package web

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"

	"minirail/internal/apps"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var errNoLogs = errors.New("no pod to read logs from yet")

var buildContainers = []string{"clone", "prepare", "build"}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app, err := s.Store.Get(ctx, r.PathValue("name"))
	if err != nil {
		fail(w, err)
		return
	}
	podName, containers, err := s.logTarget(ctx, app, r.URL.Query().Get("source"))
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher := http.NewResponseController(w)
	pods := s.Store.Client.CoreV1().Pods(s.Config.Namespace)
	for _, container := range containers {
		stream, err := pods.GetLogs(podName, &corev1.PodLogOptions{Container: container, Follow: true, TailLines: ptr.To[int64](200)}).Stream(ctx)
		if err != nil {
			fmt.Fprintf(w, "data: [%s] %v\n\n", container, err)
			flusher.Flush()
			continue
		}
		scanner := bufio.NewScanner(stream)
		scanner.Buffer(make([]byte, 64*1024), 1<<20)
		for scanner.Scan() {
			fmt.Fprintf(w, "data: %s\n\n", scanner.Text())
			flusher.Flush()
		}
		stream.Close()
	}
	fmt.Fprint(w, "event: end\ndata: \n\n")
	flusher.Flush()
}

func (s *Server) logTarget(ctx context.Context, app apps.App, source string) (string, []string, error) {
	pods := s.Store.Client.CoreV1().Pods(s.Config.Namespace)
	switch source {
	case "build":
		builds, err := s.Builder.Builds(ctx, app.Name)
		if err != nil {
			return "", nil, err
		}
		if len(builds) == 0 {
			return "", nil, errNoLogs
		}
		list, err := pods.List(ctx, metav1.ListOptions{LabelSelector: batchv1.JobNameLabel + "=" + builds[0].Name})
		if err != nil {
			return "", nil, err
		}
		pod, err := newestPod(list.Items)
		return pod, buildContainers, err
	case "runtime":
		list, err := pods.List(ctx, metav1.ListOptions{LabelSelector: runtimeSelector(app.Name)})
		if err != nil {
			return "", nil, err
		}
		pod, err := newestPod(list.Items)
		return pod, []string{"app"}, err
	default:
		return "", nil, fmt.Errorf("%w: source must be build or runtime", errBadRequest)
	}
}

func newestPod(pods []corev1.Pod) (string, error) {
	if len(pods) == 0 {
		return "", errNoLogs
	}
	newest := pods[0]
	for _, pod := range pods[1:] {
		if newest.CreationTimestamp.Before(&pod.CreationTimestamp) {
			newest = pod
		}
	}
	return newest.Name, nil
}
