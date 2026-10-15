package pg

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestRoleName(t *testing.T) {
	if got := RoleName("my-app-2"); got != "app_my_app_2" {
		t.Fatalf("RoleName = %q", got)
	}
}

func TestStatementsCreateThenRotate(t *testing.T) {
	fresh := statements("app_hello", "SECRET", false, false)
	want := []string{
		`CREATE ROLE "app_hello" LOGIN PASSWORD 'SECRET'`,
		`GRANT "app_hello" TO CURRENT_USER`,
		`CREATE DATABASE "app_hello" OWNER "app_hello"`,
		`REVOKE CONNECT ON DATABASE "app_hello" FROM PUBLIC`,
	}
	if !slices.Equal(fresh, want) {
		t.Fatalf("fresh = %q", fresh)
	}
	again := statements("app_hello", "NEW", true, true)
	if again[0] != `ALTER ROLE "app_hello" WITH LOGIN PASSWORD 'NEW'` || slices.Contains(again, want[2]) {
		t.Fatalf("re-attach = %q", again)
	}
}

func TestAppURLSwapsCredentialsAndKeepsQuery(t *testing.T) {
	got, err := appURL("postgresql://doadmin:pw@private-db.example.com:25060/defaultdb?sslmode=require", "app_hello", "SECRET")
	if err != nil || got != "postgresql://app_hello:SECRET@private-db.example.com:25060/app_hello?sslmode=require" {
		t.Fatalf("appURL = %q, %v", got, err)
	}
}

func TestProvisionAgainstRealPostgres(t *testing.T) {
	adminURL := os.Getenv("MINIRAIL_TEST_PG_URL")
	if adminURL == "" {
		t.Skip("MINIRAIL_TEST_PG_URL not set")
	}
	ctx := context.Background()
	provisioner := Provisioner{AdminURL: adminURL}
	var appURLs []string
	for range 2 {
		databaseURL, err := provisioner.Provision(ctx, "it-hello")
		if err != nil {
			t.Fatal(err)
		}
		appURLs = append(appURLs, databaseURL)
	}
	if _, err := pgx.Connect(ctx, appURLs[0]); err == nil {
		t.Fatal("re-attach must rotate the old password")
	}
	conn, err := pgx.Connect(ctx, appURLs[1])
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "CREATE TABLE IF NOT EXISTS smoke (id int)"); err != nil {
		t.Fatalf("app role cannot use its own database: %v", err)
	}
}
