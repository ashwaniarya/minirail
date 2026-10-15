package apps

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestApplyCreatesThenUpdatesWebWorkload(t *testing.T) {
	ctx := context.Background()
	store, client := newTestStore(t)
	app := validWeb()
	for _, image := range []string{"registry/hello:one", "registry/hello:two"} {
		if err := store.Apply(ctx, app, image); err != nil {
			t.Fatal(err)
		}
	}
	deployment, err := client.AppsV1().Deployments("apps").Get(ctx, "hello", metav1.GetOptions{})
	if err != nil || deployment.Spec.Template.Spec.Containers[0].Image != "registry/hello:two" {
		t.Fatalf("deployment = %+v, %v", deployment, err)
	}
	if _, err := client.CoreV1().Services("apps").Get(ctx, "hello", metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.NetworkingV1().Ingresses("apps").Get(ctx, "hello", metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestRestartSkipsUndeployedAppAndReusesDeployedImage(t *testing.T) {
	ctx := context.Background()
	store, client := newTestStore(t)
	app := validWeb()
	if err := store.Restart(ctx, app); err != nil {
		t.Fatal(err)
	}
	if list, _ := client.AppsV1().Deployments("apps").List(ctx, metav1.ListOptions{}); len(list.Items) != 0 {
		t.Fatal("restart of an undeployed app must not create a workload")
	}
	app.DeployedSHA = "abc"
	if err := store.Restart(ctx, app); err != nil {
		t.Fatal(err)
	}
	deployment, _ := client.AppsV1().Deployments("apps").Get(ctx, "hello", metav1.GetOptions{})
	if image := deployment.Spec.Template.Spec.Containers[0].Image; image != "registry.digitalocean.com/acme/hello:abc" {
		t.Fatalf("image = %q", image)
	}
}

func TestRunNowCopiesCronJobTemplate(t *testing.T) {
	ctx := context.Background()
	store, client := newTestStore(t)
	app := validWeb()
	app.Type, app.Schedule = Cron, "0 3 * * *"
	if _, err := store.RunNow(ctx, app); !errors.Is(err, ErrNotDeployed) {
		t.Fatalf("run before deploy err = %v", err)
	}
	if err := store.Apply(ctx, app, "registry/hello:one"); err != nil {
		t.Fatal(err)
	}
	name, err := store.RunNow(ctx, app)
	if err != nil {
		t.Fatal(err)
	}
	job, err := client.BatchV1().Jobs("apps").Get(ctx, name, metav1.GetOptions{})
	if err != nil || job.Spec.Template.Spec.Containers[0].Image != "registry/hello:one" || job.Labels[LabelApp] != "hello" {
		t.Fatalf("job = %+v, %v", job, err)
	}
}

func TestDeleteRemovesOnlyThatAppsObjects(t *testing.T) {
	ctx := context.Background()
	store, client := newTestStore(t)
	other := validWeb()
	other.Name = "other"
	for _, app := range []App{validWeb(), other} {
		if err := store.Create(ctx, app); err != nil {
			t.Fatal(err)
		}
		if err := store.Apply(ctx, app, "image"); err != nil {
			t.Fatal(err)
		}
		if err := store.SetEnv(ctx, app.Name, map[string]string{"A": "1"}); err != nil {
			t.Fatal(err)
		}
	}
	shared := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: RegistrySecret, Namespace: "apps"}}
	if _, err := client.CoreV1().Secrets("apps").Create(ctx, shared, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "hello"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted app still readable: %v", err)
	}
	if _, err := client.AppsV1().Deployments("apps").Get(ctx, "hello", metav1.GetOptions{}); err == nil {
		t.Fatal("deployment survived delete")
	}
	if _, err := store.Get(ctx, "other"); err != nil {
		t.Fatalf("other app removed: %v", err)
	}
	if _, err := client.AppsV1().Deployments("apps").Get(ctx, "other", metav1.GetOptions{}); err != nil {
		t.Fatalf("other deployment removed: %v", err)
	}
	if _, err := client.CoreV1().Secrets("apps").Get(ctx, RegistrySecret, metav1.GetOptions{}); err != nil {
		t.Fatalf("shared registry secret removed: %v", err)
	}
}
