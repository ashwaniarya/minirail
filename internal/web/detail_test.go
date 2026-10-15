package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDetailShowsEnvBuildsAndWebhook(t *testing.T) {
	server, _ := newTestServer(t, testApp())
	ctx := context.Background()
	if err := server.Store.SetEnv(ctx, "hello", map[string]string{"GREETING": "hi"}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Builder.Trigger(ctx, testApp(), shaA); err != nil {
		t.Fatal(err)
	}
	recorder := serve(server, httptest.NewRequest(http.MethodGet, "/apps/hello", nil))
	body := recorder.Body.String()
	for _, want := range []string{"GREETING=hi", shaA[:7], "https://console.apps.example.com/webhook/github"} {
		if !strings.Contains(body, want) {
			t.Fatalf("detail missing %q (status %d)\n%s", want, recorder.Code, body)
		}
	}
}

func TestDetailOfUnknownAppIs404(t *testing.T) {
	server, _ := newTestServer(t)
	if recorder := serve(server, httptest.NewRequest(http.MethodGet, "/apps/ghost", nil)); recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown app = %d", recorder.Code)
	}
}

func TestDetailHidesUnavailableDatabases(t *testing.T) {
	server, _ := newTestServer(t, testApp())
	server.Config.ValkeyEnabled = false
	body := serve(server, httptest.NewRequest(http.MethodGet, "/apps/hello", nil)).Body.String()
	if strings.Contains(body, "Attach Postgres") || strings.Contains(body, "Attach Valkey") {
		t.Fatalf("attach buttons shown without databases:\n%s", body)
	}
}
