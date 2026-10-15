package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"minirail/internal/apps"
	"minirail/internal/build"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func createPod(t *testing.T, client *fake.Clientset, name string, labels map[string]string, created time.Time) {
	t.Helper()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "apps", Labels: labels, CreationTimestamp: metav1.NewTime(created)}}
	if _, err := client.CoreV1().Pods("apps").Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func echoContainerLogs(client *fake.Clientset) {
	client.PrependReactor("get", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "log" {
			return false, nil, nil
		}
		options := action.(clienttesting.GenericAction).GetValue().(*corev1.PodLogOptions)
		return true, &runtime.Unknown{Raw: []byte("log of " + options.Container + "\n")}, nil
	})
}

func TestBuildLogsStreamEveryContainerInOrder(t *testing.T) {
	server, client := newTestServer(t, testApp())
	if _, err := server.Builder.Trigger(context.Background(), testApp(), shaA); err != nil {
		t.Fatal(err)
	}
	createPod(t, client, "hello-build-pod", map[string]string{batchv1.JobNameLabel: build.JobName("hello", shaA)}, time.Now())
	echoContainerLogs(client)
	recorder := serve(server, httptest.NewRequest(http.MethodGet, "/apps/hello/logs?source=build", nil))
	want := "data: log of clone\n\ndata: log of prepare\n\ndata: log of build\n\nevent: end\ndata: \n\n"
	if recorder.Body.String() != want || recorder.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream = %q", recorder.Body)
	}
}

func TestRuntimeLogsUseNewestAppPod(t *testing.T) {
	server, client := newTestServer(t, testApp())
	start := time.Now()
	createPod(t, client, "hello-old", apps.Labels("hello"), start)
	createPod(t, client, "hello-new", apps.Labels("hello"), start.Add(time.Minute))
	createPod(t, client, "hello-build-pod", map[string]string{apps.LabelApp: "hello", apps.LabelKind: build.KindBuild}, start.Add(2*time.Minute))
	podName, containers, err := server.logTarget(context.Background(), testApp(), "runtime")
	if err != nil || podName != "hello-new" || len(containers) != 1 || containers[0] != "app" {
		t.Fatalf("target = %s %v %v", podName, containers, err)
	}
}

func TestLogsWithoutPodsOrSource(t *testing.T) {
	server, _ := newTestServer(t, testApp())
	if _, _, err := server.logTarget(context.Background(), testApp(), "build"); !errors.Is(err, errNoLogs) {
		t.Fatalf("no builds = %v", err)
	}
	if recorder := serve(server, httptest.NewRequest(http.MethodGet, "/apps/hello/logs?source=runtime", nil)); recorder.Code != http.StatusNotFound {
		t.Fatalf("no pods = %d", recorder.Code)
	}
	if recorder := serve(server, httptest.NewRequest(http.MethodGet, "/apps/hello/logs?source=disk", nil)); recorder.Code != http.StatusBadRequest || strings.Contains(recorder.Header().Get("Content-Type"), "event-stream") {
		t.Fatalf("bad source = %d", recorder.Code)
	}
}
