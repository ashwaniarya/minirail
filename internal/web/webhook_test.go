package web

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"minirail/internal/apps"
	"minirail/internal/build"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func pushBody(repoURL, ref, after string) []byte {
	return []byte(`{"ref":"` + ref + `","after":"` + after + `","deleted":false,"repository":{"html_url":"` + repoURL + `"}}`)
}

func sendWebhook(server *Server, event string, body []byte, signature string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewReader(body))
	request.Header.Set("X-GitHub-Event", event)
	request.Header.Set("X-Hub-Signature-256", signature)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func buildJobNames(t *testing.T, server *Server) []string {
	t.Helper()
	list, err := server.Store.Client.BatchV1().Jobs("apps").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, job := range list.Items {
		names = append(names, job.Name)
	}
	slices.Sort(names)
	return names
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"zen":"hi"}`)
	if !VerifySignature("hook-secret", body, sign("hook-secret", body)) {
		t.Fatal("valid signature rejected")
	}
	for name, header := range map[string]string{
		"wrong secret":   sign("other", body),
		"missing prefix": strings.TrimPrefix(sign("hook-secret", body), "sha256="),
		"not hex":        "sha256=zz",
		"empty":          "",
	} {
		if VerifySignature("hook-secret", body, header) {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestPushBuildsEveryAppOnRepoAndBranch(t *testing.T) {
	worker := testApp()
	worker.Name, worker.Type, worker.Port = "hello-worker", apps.Worker, 0
	staging := testApp()
	staging.Name, staging.Branch = "hello-staging", "dev"
	other := testApp()
	other.Name, other.Repo = "other", "https://github.com/acme/other"
	server, _ := newTestServer(t, testApp(), worker, staging, other)
	body := pushBody("https://github.com/Acme/Hello", "refs/heads/main", shaA)
	if recorder := sendWebhook(server, "push", body, sign("hook-secret", body)); recorder.Code != http.StatusAccepted {
		t.Fatalf("push = %d %q", recorder.Code, recorder.Body)
	}
	names := strings.Join(buildJobNames(t, server), ",")
	if names != build.JobName("hello", shaA)+","+build.JobName("hello-worker", shaA) {
		t.Fatalf("build jobs = %s", names)
	}
}

func TestWebhookRejectsBadSignature(t *testing.T) {
	server, _ := newTestServer(t, testApp())
	body := pushBody("https://github.com/acme/hello", "refs/heads/main", shaA)
	if recorder := sendWebhook(server, "push", body, sign("guess", body)); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature = %d", recorder.Code)
	}
	if names := buildJobNames(t, server); len(names) != 0 {
		t.Fatalf("unsigned push built %v", names)
	}
}

func TestWebhookRejectsOversizeBody(t *testing.T) {
	server, _ := newTestServer(t, testApp())
	body := bytes.Repeat([]byte("x"), maxBodyBytes+1)
	if recorder := sendWebhook(server, "push", body, sign("hook-secret", body)); recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize body = %d", recorder.Code)
	}
}

func TestWebhookIgnoresWhatItCannotBuild(t *testing.T) {
	deleted := []byte(`{"ref":"refs/heads/main","after":"` + strings.Repeat("0", 40) + `","deleted":true,"repository":{"html_url":"https://github.com/acme/hello"}}`)
	for name, testCase := range map[string]struct {
		event string
		body  []byte
		want  int
	}{
		"ping":          {"ping", []byte(`{"zen":"hi"}`), http.StatusOK},
		"other event":   {"issues", []byte(`{}`), http.StatusNoContent},
		"tag push":      {"push", pushBody("https://github.com/acme/hello", "refs/tags/v1", shaA), http.StatusNoContent},
		"branch delete": {"push", deleted, http.StatusNoContent},
		"unknown repo":  {"push", pushBody("https://github.com/acme/nope", "refs/heads/main", shaA), http.StatusNoContent},
		"other branch":  {"push", pushBody("https://github.com/acme/hello", "refs/heads/feature", shaA), http.StatusNoContent},
		"bad after":     {"push", pushBody("https://github.com/acme/hello", "refs/heads/main", "HEAD"), http.StatusBadRequest},
		"bad json":      {"push", []byte(`{`), http.StatusBadRequest},
	} {
		server, _ := newTestServer(t, testApp())
		if recorder := sendWebhook(server, testCase.event, testCase.body, sign("hook-secret", testCase.body)); recorder.Code != testCase.want {
			t.Fatalf("%s = %d, want %d", name, recorder.Code, testCase.want)
		}
		if names := buildJobNames(t, server); len(names) != 0 {
			t.Fatalf("%s built %v", name, names)
		}
	}
}
