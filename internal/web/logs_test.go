package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
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

var (
	waitingState    = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}}
	terminatedState = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}
)

func containerStatuses(state corev1.ContainerState, names ...string) []corev1.ContainerStatus {
	var statuses []corev1.ContainerStatus
	for _, name := range names {
		statuses = append(statuses, corev1.ContainerStatus{Name: name, State: state})
	}
	return statuses
}

func setPodStatus(t *testing.T, client *fake.Clientset, name string, status corev1.PodStatus) {
	t.Helper()
	pods := client.CoreV1().Pods("apps")
	pod, err := pods.Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Error(err)
		return
	}
	pod.Status = status
	if _, err := pods.UpdateStatus(context.Background(), pod, metav1.UpdateOptions{}); err != nil {
		t.Error(err)
	}
}

func echoContainerLogs(client *fake.Clientset, podName string) {
	client.PrependReactor("get", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "log" {
			return false, nil, nil
		}
		options := action.(clienttesting.GenericAction).GetValue().(*corev1.PodLogOptions)
		object, err := client.Tracker().Get(corev1.SchemeGroupVersion.WithResource("pods"), "apps", podName)
		if err != nil {
			return true, nil, err
		}
		pod := object.(*corev1.Pod)
		for _, status := range slices.Concat(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses) {
			if status.Name == options.Container && (status.State.Waiting == nil || status.LastTerminationState.Terminated != nil) {
				return true, &runtime.Unknown{Raw: []byte("log of " + options.Container + "\n")}, nil
			}
		}
		return true, nil, fmt.Errorf("container %q in pod %q is waiting to start", options.Container, podName)
	})
}

func streamLogs(server *Server, source string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return serve(server, httptest.NewRequest(http.MethodGet, "/apps/hello/logs?source="+source, nil).WithContext(ctx)).Body.String()
}

const endEvent = "event: end\ndata: \n\n"

func TestBuildLogsStreamEveryContainerInOrder(t *testing.T) {
	server, client := newTestServer(t, testApp())
	if _, err := server.Builder.Trigger(context.Background(), testApp(), shaA); err != nil {
		t.Fatal(err)
	}
	createPod(t, client, "hello-build-pod", map[string]string{batchv1.JobNameLabel: build.JobName("hello", shaA)}, time.Now())
	setPodStatus(t, client, "hello-build-pod", corev1.PodStatus{InitContainerStatuses: containerStatuses(terminatedState, buildContainers...)})
	echoContainerLogs(client, "hello-build-pod")
	recorder := serve(server, httptest.NewRequest(http.MethodGet, "/apps/hello/logs?source=build", nil))
	want := "data: log of clone\n\ndata: log of prepare\n\ndata: log of build\n\nevent: end\ndata: \n\n"
	if recorder.Body.String() != want || recorder.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream = %q", recorder.Body)
	}
}

func TestBuildLogsWaitForEachContainerToStart(t *testing.T) {
	server, client := newTestServer(t, testApp())
	if _, err := server.Builder.Trigger(context.Background(), testApp(), shaA); err != nil {
		t.Fatal(err)
	}
	createPod(t, client, "hello-build-pod", map[string]string{batchv1.JobNameLabel: build.JobName("hello", shaA)}, time.Now())
	setPodStatus(t, client, "hello-build-pod", corev1.PodStatus{InitContainerStatuses: containerStatuses(waitingState, buildContainers...)})
	echoContainerLogs(client, "hello-build-pod")
	started := make(chan struct{})
	go func() {
		defer close(started)
		time.Sleep(300 * time.Millisecond)
		setPodStatus(t, client, "hello-build-pod", corev1.PodStatus{InitContainerStatuses: containerStatuses(terminatedState, buildContainers...)})
	}()
	body := streamLogs(server, "build")
	<-started
	if want := "data: log of clone\n\ndata: log of prepare\n\ndata: log of build\n\n" + endEvent; body != want {
		t.Fatalf("stream = %q", body)
	}
}

func TestBuildLogsStopWaitingOnceThePodFinished(t *testing.T) {
	server, client := newTestServer(t, testApp())
	if _, err := server.Builder.Trigger(context.Background(), testApp(), shaA); err != nil {
		t.Fatal(err)
	}
	createPod(t, client, "hello-build-pod", map[string]string{batchv1.JobNameLabel: build.JobName("hello", shaA)}, time.Now())
	setPodStatus(t, client, "hello-build-pod", corev1.PodStatus{
		Phase:                 corev1.PodFailed,
		InitContainerStatuses: slices.Concat(containerStatuses(terminatedState, "clone"), containerStatuses(waitingState, "prepare", "build")),
	})
	echoContainerLogs(client, "hello-build-pod")
	body := streamLogs(server, "build")
	if !strings.HasPrefix(body, "data: log of clone\n\n") || strings.Count(body, "is waiting to start") != 2 || !strings.HasSuffix(body, endEvent) {
		t.Fatalf("stream = %q", body)
	}
}

func TestRuntimeLogsOfCrashLoopingContainerStreamAtOnce(t *testing.T) {
	server, client := newTestServer(t, testApp())
	createPod(t, client, "hello-pod", apps.Labels("hello"), time.Now())
	crashLooping := corev1.ContainerStatus{
		Name:                 "app",
		State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		LastTerminationState: terminatedState,
	}
	setPodStatus(t, client, "hello-pod", corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{crashLooping}})
	echoContainerLogs(client, "hello-pod")
	if body := streamLogs(server, "runtime"); body != "data: log of app\n\n"+endEvent {
		t.Fatalf("stream = %q", body)
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
