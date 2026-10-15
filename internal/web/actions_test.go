package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"minirail/internal/apps"
	"minirail/internal/build"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func storedApp(t *testing.T, server *Server, name string) apps.App {
	t.Helper()
	app, err := server.Store.Get(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func cronApp() apps.App {
	app := testApp()
	app.Name, app.Type, app.Port, app.Schedule = "nightly", apps.Cron, 0, "0 3 * * *"
	return app
}

func TestSaveEnvReplacesSecret(t *testing.T) {
	server, _ := newTestServer(t, testApp())
	recorder := serve(server, postForm("/apps/hello/env", url.Values{"env": {"# comment\nA=1\nB=two words\n"}}))
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("save env = %d %q", recorder.Code, recorder.Body)
	}
	env, _ := server.Store.Env(context.Background(), "hello")
	if len(env) != 2 || env["A"] != "1" || env["B"] != "two words" {
		t.Fatalf("env = %v", env)
	}
}

func TestSaveEnvRejectsBadLine(t *testing.T) {
	server, _ := newTestServer(t, testApp())
	recorder := serve(server, postForm("/apps/hello/env", url.Values{"env": {"A=1\nnot a pair"}}))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "line 2") {
		t.Fatalf("bad env = %d %q", recorder.Code, recorder.Body)
	}
}

func TestSaveEnvBeforeFirstDeployCreatesNoWorkload(t *testing.T) {
	server, client := newTestServer(t, testApp())
	if recorder := serve(server, postForm("/apps/hello/env", url.Values{"env": {"A=1"}})); recorder.Code != http.StatusSeeOther {
		t.Fatalf("save env = %d %q", recorder.Code, recorder.Body)
	}
	if _, err := client.AppsV1().Deployments("apps").Get(context.Background(), "hello", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("undeployed app got a deployment: %v", err)
	}
}

func TestSaveEnvRestartsDeployedApp(t *testing.T) {
	server, client := newTestServer(t, testApp())
	ctx := context.Background()
	app := testApp()
	app.DeployedSHA = shaA
	if err := server.Store.Update(ctx, app); err != nil {
		t.Fatal(err)
	}
	if recorder := serve(server, postForm("/apps/hello/env", url.Values{"env": {"A=1"}})); recorder.Code != http.StatusSeeOther {
		t.Fatalf("save env = %d", recorder.Code)
	}
	deployment, err := client.AppsV1().Deployments("apps").Get(ctx, "hello", metav1.GetOptions{})
	if err != nil || deployment.Spec.Template.Annotations[apps.AnnotationRestartedAt] == "" {
		t.Fatalf("deployment = %+v, %v", deployment, err)
	}
}

func TestDeployStartsBuildOfBranchHead(t *testing.T) {
	server, client := newTestServer(t, testApp())
	if recorder := serve(server, postForm("/apps/hello/deploy", nil)); recorder.Code != http.StatusSeeOther {
		t.Fatalf("deploy = %d", recorder.Code)
	}
	if _, err := client.BatchV1().Jobs("apps").Get(context.Background(), build.JobName("hello", shaA), metav1.GetOptions{}); err != nil {
		t.Fatalf("build missing: %v", err)
	}
}

func TestRunNow(t *testing.T) {
	server, client := newTestServer(t, testApp(), cronApp())
	ctx := context.Background()
	if recorder := serve(server, postForm("/apps/hello/run", nil)); recorder.Code != http.StatusBadRequest {
		t.Fatalf("run web app = %d", recorder.Code)
	}
	if recorder := serve(server, postForm("/apps/nightly/run", nil)); recorder.Code != http.StatusConflict {
		t.Fatalf("run undeployed cron = %d", recorder.Code)
	}
	if err := server.Store.Apply(ctx, cronApp(), apps.ImageRef(testConfig().Registry, "nightly", shaA)); err != nil {
		t.Fatal(err)
	}
	if recorder := serve(server, postForm("/apps/nightly/run", nil)); recorder.Code != http.StatusSeeOther {
		t.Fatalf("run cron = %d %q", recorder.Code, recorder.Body)
	}
	jobs, _ := client.BatchV1().Jobs("apps").List(ctx, metav1.ListOptions{LabelSelector: apps.LabelApp + "=nightly"})
	if len(jobs.Items) != 1 || !strings.HasPrefix(jobs.Items[0].Name, "nightly-run-") {
		t.Fatalf("jobs = %+v", jobs.Items)
	}
}

func TestAttachPostgresWritesDatabaseURL(t *testing.T) {
	server, client := newTestServer(t, testApp())
	server.Provision = func(_ context.Context, appName string) (string, error) {
		return "postgres://app_" + appName + ":secret@db:25060/app_" + appName + "?sslmode=require", nil
	}
	if recorder := serve(server, postForm("/apps/hello/attach/postgres", nil)); recorder.Code != http.StatusSeeOther {
		t.Fatalf("attach = %d %q", recorder.Code, recorder.Body)
	}
	secret, err := client.CoreV1().Secrets("apps").Get(context.Background(), apps.DBSecretName("hello"), metav1.GetOptions{})
	if err != nil || !strings.HasPrefix(string(secret.Data["DATABASE_URL"]), "postgres://app_hello:") {
		t.Fatalf("db secret = %+v, %v", secret, err)
	}
	if !storedApp(t, server, "hello").Postgres {
		t.Fatal("app must record the attached postgres")
	}
}

func TestAttachPostgresFailureWritesNoSecret(t *testing.T) {
	server, client := newTestServer(t, testApp())
	server.Provision = func(context.Context, string) (string, error) { return "", errors.New("connection refused") }
	if recorder := serve(server, postForm("/apps/hello/attach/postgres", nil)); recorder.Code != http.StatusBadGateway {
		t.Fatalf("failed attach = %d", recorder.Code)
	}
	if _, err := client.CoreV1().Secrets("apps").Get(context.Background(), apps.DBSecretName("hello"), metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("failed attach wrote a secret: %v", err)
	}
	if storedApp(t, server, "hello").Postgres {
		t.Fatal("failed attach must not flag postgres")
	}
}

func TestAttachValkeyFlagsApp(t *testing.T) {
	server, _ := newTestServer(t, testApp())
	if recorder := serve(server, postForm("/apps/hello/attach/valkey", nil)); recorder.Code != http.StatusSeeOther {
		t.Fatalf("attach valkey = %d", recorder.Code)
	}
	if !storedApp(t, server, "hello").Valkey {
		t.Fatal("app must record the attached valkey")
	}
}

func TestAttachUnavailableDatabase(t *testing.T) {
	server, _ := newTestServer(t, testApp())
	server.Config.ValkeyEnabled = false
	for path, want := range map[string]int{
		"/apps/hello/attach/postgres": http.StatusBadRequest,
		"/apps/hello/attach/valkey":   http.StatusBadRequest,
		"/apps/hello/attach/mysql":    http.StatusNotFound,
	} {
		if recorder := serve(server, postForm(path, nil)); recorder.Code != want {
			t.Fatalf("%s = %d, want %d", path, recorder.Code, want)
		}
	}
}

func TestDeleteKeepsSharedSecrets(t *testing.T) {
	server, client := newTestServer(t, testApp())
	ctx := context.Background()
	for _, name := range []string{apps.RegistrySecret, apps.ValkeySecret, build.GitHubTokenSecret} {
		shared := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "apps"}}
		if _, err := client.CoreV1().Secrets("apps").Create(ctx, shared, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := server.Store.SetEnv(ctx, "hello", map[string]string{"A": "1"}); err != nil {
		t.Fatal(err)
	}
	recorder := serve(server, postForm("/apps/hello/delete", nil))
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/" {
		t.Fatalf("delete = %d %v", recorder.Code, recorder.Header())
	}
	if _, err := server.Store.Get(ctx, "hello"); !errors.Is(err, apps.ErrNotFound) {
		t.Fatalf("app still present: %v", err)
	}
	secrets, _ := client.CoreV1().Secrets("apps").List(ctx, metav1.ListOptions{})
	if len(secrets.Items) != 3 {
		t.Fatalf("secrets after delete = %d, want the 3 shared ones", len(secrets.Items))
	}
}
