package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"minirail/internal/build"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createForm(name, repo string) url.Values {
	return url.Values{"name": {name}, "repo": {repo}, "branch": {"main"}, "type": {"web"}, "port": {"8080"}, "replicas": {"1"}, "root": {"."}}
}

func TestIndexLinksAppsWithTheConfiguredScheme(t *testing.T) {
	server, _ := newTestServer(t, testApp())
	server.Config.Scheme = "http"
	body := serve(server, httptest.NewRequest(http.MethodGet, "/", nil)).Body.String()
	if !strings.Contains(body, "http://hello.apps.example.com") {
		t.Fatalf("index lacks the http app URL\n%s", body)
	}
}

func TestIndexListsAppsWithURLAndBuildStatus(t *testing.T) {
	server, _ := newTestServer(t, testApp())
	if _, err := server.Builder.Trigger(context.Background(), testApp(), shaA); err != nil {
		t.Fatal(err)
	}
	recorder := serve(server, httptest.NewRequest(http.MethodGet, "/", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "https://hello.apps.example.com") || !strings.Contains(body, build.StatusRunning) {
		t.Fatalf("index = %d\n%s", recorder.Code, body)
	}
}

func TestCreateStartsFirstBuild(t *testing.T) {
	server, client := newTestServer(t)
	recorder := serve(server, postForm("/apps/new", createForm("hello", " https://github.com/Acme/Hello.git ")))
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/apps/hello" {
		t.Fatalf("create = %d %q", recorder.Code, recorder.Body)
	}
	app, err := server.Store.Get(context.Background(), "hello")
	if err != nil || app.Repo != "https://github.com/acme/hello" {
		t.Fatalf("stored app = %+v, %v", app, err)
	}
	if _, err := client.BatchV1().Jobs("apps").Get(context.Background(), build.JobName("hello", shaA), metav1.GetOptions{}); err != nil {
		t.Fatalf("first build missing: %v", err)
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	server, client := newTestServer(t)
	recorder := serve(server, postForm("/apps/new", createForm("Bad Name", "https://github.com/acme/hello")))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "name") {
		t.Fatalf("invalid create = %d %q", recorder.Code, recorder.Body)
	}
	if list, _ := client.CoreV1().ConfigMaps("apps").List(context.Background(), metav1.ListOptions{}); len(list.Items) != 0 {
		t.Fatal("invalid input must create nothing")
	}
}

func TestCreateRejectsDuplicateName(t *testing.T) {
	server, _ := newTestServer(t, testApp())
	if recorder := serve(server, postForm("/apps/new", createForm("hello", "https://github.com/acme/other"))); recorder.Code != http.StatusConflict {
		t.Fatalf("duplicate create = %d", recorder.Code)
	}
}

func TestCreateKeepsAppWhenFirstBuildFails(t *testing.T) {
	server, _ := newTestServer(t)
	server.Builder.ResolveHead = func(context.Context, string, string) (string, error) {
		return "", errors.New("repository not found")
	}
	recorder := serve(server, postForm("/apps/new", createForm("hello", "https://github.com/acme/hello")))
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), "Press Deploy") {
		t.Fatalf("create = %d %q", recorder.Code, recorder.Body)
	}
	if _, err := server.Store.Get(context.Background(), "hello"); err != nil {
		t.Fatalf("app must be kept: %v", err)
	}
}
