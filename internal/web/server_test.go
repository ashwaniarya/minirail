package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"minirail/internal/apps"
	"minirail/internal/build"

	"k8s.io/client-go/kubernetes/fake"
)

var shaA = strings.Repeat("a", 40)

func testConfig() apps.Config {
	return apps.Config{
		Namespace:        "apps",
		Domain:           "apps.example.com",
		Registry:         "registry.digitalocean.com/acme",
		ControlImage:     "registry.digitalocean.com/acme/minirail:abc",
		BuildkitImage:    "moby/buildkit:v0.33.1-rootless",
		RailpackFrontend: "ghcr.io/railwayapp/railpack-frontend:v0.40.1",
		WebhookSecret:    "hook-secret",
		AdminPassword:    "admin-pass",
		ValkeyEnabled:    true,
	}
}

func testApp() apps.App {
	return apps.App{Name: "hello", Repo: "https://github.com/acme/hello", Branch: "main", Type: apps.Web, Port: 8080, Replicas: 1, Root: "."}
}

func newTestServer(t *testing.T, existing ...apps.App) (*Server, *fake.Clientset) {
	t.Helper()
	client := fake.NewClientset()
	store := &apps.Store{Client: client, Config: testConfig()}
	for _, app := range existing {
		if err := store.Create(context.Background(), app); err != nil {
			t.Fatal(err)
		}
	}
	resolveHead := func(context.Context, string, string) (string, error) { return shaA, nil }
	builder := &build.Builder{Client: client, Config: testConfig(), Store: store, ResolveHead: resolveHead}
	return &Server{Config: testConfig(), Store: store, Builder: builder}, client
}

func serve(server *Server, request *http.Request) *httptest.ResponseRecorder {
	request.SetBasicAuth("admin", "admin-pass")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func postForm(path string, values url.Values) *http.Request {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return request
}

func TestHealthzNeedsNoAuth(t *testing.T) {
	server, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "ok" {
		t.Fatalf("healthz = %d %q", recorder.Code, recorder.Body)
	}
}

func TestDashboardRequiresAdmin(t *testing.T) {
	server, _ := newTestServer(t)
	for name, setAuth := range map[string]func(*http.Request){
		"no credentials": func(*http.Request) {},
		"wrong password": func(request *http.Request) { request.SetBasicAuth("admin", "nope") },
		"wrong user":     func(request *http.Request) { request.SetBasicAuth("root", "admin-pass") },
	} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		setAuth(request)
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized || !strings.HasPrefix(recorder.Header().Get("WWW-Authenticate"), "Basic") {
			t.Fatalf("%s: %d %v", name, recorder.Code, recorder.Header())
		}
	}
}

func TestCrossSitePostIsRejected(t *testing.T) {
	server, _ := newTestServer(t, testApp())
	request := postForm("/apps/hello/delete", url.Values{})
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	if recorder := serve(server, request); recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-site delete = %d", recorder.Code)
	}
	if _, err := server.Store.Get(context.Background(), "hello"); err != nil {
		t.Fatalf("app must survive a cross-site post: %v", err)
	}
}
