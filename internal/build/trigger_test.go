package build

import (
	"context"
	"strings"
	"testing"
	"time"

	"minirail/internal/apps"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	shaB = strings.Repeat("b", 40)
	shaC = strings.Repeat("c", 40)
)

func newTestBuilder(t *testing.T, appsToCreate ...apps.App) (*Builder, *fake.Clientset) {
	t.Helper()
	client := fake.NewClientset()
	store := &apps.Store{Client: client, Config: testConfig()}
	for _, app := range appsToCreate {
		if err := store.Create(context.Background(), app); err != nil {
			t.Fatal(err)
		}
	}
	resolveHead := func(context.Context, string, string) (string, error) { return shaA, nil }
	return &Builder{Client: client, Config: testConfig(), Store: store, ResolveHead: resolveHead}, client
}

func finishJob(t *testing.T, client *fake.Clientset, name string, condition batchv1.JobConditionType, created time.Time) {
	t.Helper()
	jobs := client.BatchV1().Jobs("apps")
	job, err := jobs.Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	job.CreationTimestamp = metav1.NewTime(created)
	job.Status.Conditions = []batchv1.JobCondition{{Type: condition, Status: corev1.ConditionTrue}}
	if _, err := jobs.Update(context.Background(), job, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func deployedSHA(t *testing.T, builder *Builder, appName string) string {
	t.Helper()
	app, err := builder.Store.Get(context.Background(), appName)
	if err != nil {
		t.Fatal(err)
	}
	return app.DeployedSHA
}

func TestTriggerStartsOneBuildPerSHA(t *testing.T) {
	ctx := context.Background()
	builder, client := newTestBuilder(t, testApp())
	for attempt, wantCreated := range []bool{true, false} {
		created, err := builder.Trigger(ctx, testApp(), shaA)
		if err != nil || created != wantCreated {
			t.Fatalf("attempt %d: created = %v, %v", attempt, created, err)
		}
	}
	list, _ := client.BatchV1().Jobs("apps").List(ctx, metav1.ListOptions{})
	if len(list.Items) != 1 {
		t.Fatalf("jobs = %d", len(list.Items))
	}
}

func TestTriggerSkipsDeployedSHA(t *testing.T) {
	builder, _ := newTestBuilder(t)
	app := testApp()
	app.DeployedSHA = shaA
	if created, err := builder.Trigger(context.Background(), app, shaA); err != nil || created {
		t.Fatalf("created = %v, %v", created, err)
	}
}

func TestTriggerRecreatesFailedBuild(t *testing.T) {
	ctx := context.Background()
	builder, client := newTestBuilder(t, testApp())
	if _, err := builder.Trigger(ctx, testApp(), shaA); err != nil {
		t.Fatal(err)
	}
	finishJob(t, client, JobName("hello", shaA), batchv1.JobFailed, time.Now())
	created, err := builder.Trigger(ctx, testApp(), shaA)
	if err != nil || !created {
		t.Fatalf("retry created = %v, %v", created, err)
	}
	job, _ := client.BatchV1().Jobs("apps").Get(ctx, JobName("hello", shaA), metav1.GetOptions{})
	if Status(*job) != StatusRunning {
		t.Fatalf("retried build status = %s", Status(*job))
	}
}

func TestRedeployBuildsBranchHead(t *testing.T) {
	ctx := context.Background()
	builder, client := newTestBuilder(t, testApp())
	if _, err := builder.Redeploy(ctx, testApp()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.BatchV1().Jobs("apps").Get(ctx, JobName("hello", shaA), metav1.GetOptions{}); err != nil {
		t.Fatalf("head build missing: %v", err)
	}
}

func TestReconcileDeploysNewestSucceededBuild(t *testing.T) {
	ctx := context.Background()
	builder, client := newTestBuilder(t, testApp())
	start := time.Now()
	for index, build := range []struct {
		sha       string
		condition batchv1.JobConditionType
	}{{shaA, batchv1.JobComplete}, {shaB, batchv1.JobComplete}, {shaC, batchv1.JobFailed}} {
		if _, err := builder.Trigger(ctx, testApp(), build.sha); err != nil {
			t.Fatal(err)
		}
		finishJob(t, client, JobName("hello", build.sha), build.condition, start.Add(time.Duration(index)*time.Minute))
	}
	if err := builder.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if got := deployedSHA(t, builder, "hello"); got != shaB {
		t.Fatalf("deployed = %s, want newest succeeded build", got)
	}
	deployment, err := client.AppsV1().Deployments("apps").Get(ctx, "hello", metav1.GetOptions{})
	if err != nil || deployment.Spec.Template.Spec.Containers[0].Image != apps.ImageRef(testConfig().Registry, "hello", shaB) {
		t.Fatalf("deployment = %+v, %v", deployment, err)
	}
}

func TestReconcileDeploysEveryAppOnSharedRepo(t *testing.T) {
	ctx := context.Background()
	worker := testApp()
	worker.Name, worker.Type, worker.Port = "hello-worker", apps.Worker, 0
	builder, client := newTestBuilder(t, testApp(), worker)
	for _, app := range []apps.App{testApp(), worker} {
		if _, err := builder.Trigger(ctx, app, shaA); err != nil {
			t.Fatal(err)
		}
		finishJob(t, client, JobName(app.Name, shaA), batchv1.JobComplete, time.Now())
	}
	if err := builder.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"hello", "hello-worker"} {
		if got := deployedSHA(t, builder, name); got != shaA {
			t.Fatalf("%s deployed = %q", name, got)
		}
	}
}

func TestForcePushBackToOldSHARedeploysIt(t *testing.T) {
	ctx := context.Background()
	builder, client := newTestBuilder(t, testApp())
	start := time.Now()
	for index, sha := range []string{shaA, shaC} {
		if _, err := builder.Trigger(ctx, testApp(), sha); err != nil {
			t.Fatal(err)
		}
		finishJob(t, client, JobName("hello", sha), batchv1.JobComplete, start.Add(time.Duration(index)*time.Minute))
	}
	if err := builder.Reconcile(ctx); err != nil || deployedSHA(t, builder, "hello") != shaC {
		t.Fatalf("setup: deployed = %s, %v", deployedSHA(t, builder, "hello"), err)
	}
	app, _ := builder.Store.Get(ctx, "hello")
	if created, err := builder.Trigger(ctx, app, shaA); err != nil || !created {
		t.Fatalf("old sha rebuild created = %v, %v", created, err)
	}
	if err := builder.Reconcile(ctx); err != nil || deployedSHA(t, builder, "hello") != shaC {
		t.Fatal("running rebuild must not change the deployed sha")
	}
	finishJob(t, client, JobName("hello", shaA), batchv1.JobComplete, start.Add(5*time.Minute))
	if err := builder.Reconcile(ctx); err != nil || deployedSHA(t, builder, "hello") != shaA {
		t.Fatalf("deployed = %s after force-push rebuild, want old sha", deployedSHA(t, builder, "hello"))
	}
}

func TestReconcileIgnoresBuildsOfDeletedApps(t *testing.T) {
	ctx := context.Background()
	builder, client := newTestBuilder(t)
	if _, err := builder.Trigger(ctx, testApp(), shaA); err != nil {
		t.Fatal(err)
	}
	finishJob(t, client, JobName("hello", shaA), batchv1.JobComplete, time.Now())
	if err := builder.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if list, _ := client.AppsV1().Deployments("apps").List(ctx, metav1.ListOptions{}); len(list.Items) != 0 {
		t.Fatal("build of a deleted app must not deploy")
	}
}

func TestBuildsNewestFirst(t *testing.T) {
	ctx := context.Background()
	builder, client := newTestBuilder(t, testApp())
	start := time.Now()
	for index, sha := range []string{shaA, shaB} {
		if _, err := builder.Trigger(ctx, testApp(), sha); err != nil {
			t.Fatal(err)
		}
		finishJob(t, client, JobName("hello", sha), batchv1.JobComplete, start.Add(time.Duration(index)*time.Minute))
	}
	builds, err := builder.Builds(ctx, "hello")
	if err != nil || len(builds) != 2 || builds[0].Name != JobName("hello", shaB) {
		t.Fatalf("builds = %v, %v", builds, err)
	}
}
