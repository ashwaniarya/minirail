package apps

import (
	"context"
	"errors"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestStore(t *testing.T) (*Store, *fake.Clientset) {
	t.Helper()
	client := fake.NewClientset()
	return &Store{Client: client, Config: testConfig()}, client
}

func TestStoreCreateGetList(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestStore(t)
	second := validWeb()
	second.Name = "api"
	for _, app := range []App{validWeb(), second} {
		if err := store.Create(ctx, app); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Create(ctx, validWeb()); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate create err = %v", err)
	}
	got, err := store.Get(ctx, "hello")
	if err != nil || !reflect.DeepEqual(got, validWeb()) {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if _, err := store.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing app err = %v", err)
	}
	all, err := store.List(ctx)
	if err != nil || len(all) != 2 || all[0].Name != "api" || all[1].Name != "hello" {
		t.Fatalf("List = %+v, %v", all, err)
	}
}

func TestStoreUpdatePersistsFields(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestStore(t)
	app := validWeb()
	if err := store.Create(ctx, app); err != nil {
		t.Fatal(err)
	}
	app.Postgres, app.DeployedSHA = true, "abc"
	if err := store.Update(ctx, app); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(ctx, "hello"); !got.Postgres || got.DeployedSHA != "abc" {
		t.Fatalf("after update = %+v", got)
	}
}

func TestStoreEnvAndDatabaseURLAreSeparate(t *testing.T) {
	ctx := context.Background()
	store, client := newTestStore(t)
	if env, err := store.Env(ctx, "hello"); err != nil || len(env) != 0 {
		t.Fatalf("empty env = %v, %v", env, err)
	}
	if err := store.SetDatabaseURL(ctx, "hello", "postgres://db"); err != nil {
		t.Fatal(err)
	}
	for _, env := range []map[string]string{{"A": "1", "B": "2"}, {"A": "3"}} {
		if err := store.SetEnv(ctx, "hello", env); err != nil {
			t.Fatal(err)
		}
	}
	env, err := store.Env(ctx, "hello")
	if err != nil || !reflect.DeepEqual(env, map[string]string{"A": "3"}) {
		t.Fatalf("env = %v, %v", env, err)
	}
	dbSecret, err := client.CoreV1().Secrets("apps").Get(ctx, "hello-db", metav1.GetOptions{})
	if err != nil || string(dbSecret.Data["DATABASE_URL"]) != "postgres://db" || dbSecret.Labels[LabelApp] != "hello" {
		t.Fatalf("db secret = %+v, %v", dbSecret, err)
	}
}
