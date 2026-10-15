package pg

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
)

type Provisioner struct {
	AdminURL string
}

func RoleName(appName string) string { return "app_" + strings.ReplaceAll(appName, "-", "_") }

func (p Provisioner) Provision(ctx context.Context, appName string) (string, error) {
	role, password := RoleName(appName), rand.Text()
	conn, err := pgx.Connect(ctx, p.AdminURL)
	if err != nil {
		return "", fmt.Errorf("connect to postgres: %w", err)
	}
	defer conn.Close(ctx)
	var roleExists, databaseExists bool
	err = conn.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1), EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)",
		role).Scan(&roleExists, &databaseExists)
	if err != nil {
		return "", err
	}
	for _, statement := range statements(role, password, roleExists, databaseExists) {
		if _, err := conn.Exec(ctx, statement); err != nil {
			return "", fmt.Errorf("%s: %w", strings.Fields(statement)[0], err)
		}
	}
	return appURL(p.AdminURL, role, password)
}

func statements(role, password string, roleExists, databaseExists bool) []string {
	quoted := pgx.Identifier{role}.Sanitize()
	list := []string{fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD '%s'", quoted, password)}
	if roleExists {
		list = []string{fmt.Sprintf("ALTER ROLE %s WITH LOGIN PASSWORD '%s'", quoted, password)}
	}
	// The managed admin is not a superuser, so it must be a member of the role to make it a database owner.
	list = append(list, fmt.Sprintf("GRANT %s TO CURRENT_USER", quoted))
	if !databaseExists {
		list = append(list, fmt.Sprintf("CREATE DATABASE %s OWNER %s", quoted, quoted))
	}
	return append(list, fmt.Sprintf("REVOKE CONNECT ON DATABASE %s FROM PUBLIC", quoted))
}

func appURL(adminURL, role, password string) (string, error) {
	parsed, err := url.Parse(adminURL)
	if err != nil {
		return "", fmt.Errorf("admin url: %w", err)
	}
	parsed.User = url.UserPassword(role, password)
	parsed.Path = "/" + role
	return parsed.String(), nil
}
