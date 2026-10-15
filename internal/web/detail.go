package web

import (
	"net/http"

	"minirail/internal/apps"
	"minirail/internal/build"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var detailPage = page("detail")

type podRow struct {
	Name     string
	Phase    string
	Restarts int32
}

type detailView struct {
	App         apps.App
	URL         string
	Env         string
	Builds      []batchv1.Job
	Pods        []podRow
	WebhookURL  string
	CanPostgres bool
	CanValkey   bool
}

func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app, err := s.Store.Get(ctx, r.PathValue("name"))
	if err != nil {
		fail(w, err)
		return
	}
	env, err := s.Store.Env(ctx, app.Name)
	if err != nil {
		fail(w, err)
		return
	}
	builds, err := s.Builder.Builds(ctx, app.Name)
	if err != nil {
		fail(w, err)
		return
	}
	pods, err := s.Store.Client.CoreV1().Pods(s.Config.Namespace).List(ctx, metav1.ListOptions{LabelSelector: runtimeSelector(app.Name)})
	if err != nil {
		fail(w, err)
		return
	}
	podRows := make([]podRow, 0, len(pods.Items))
	for _, pod := range pods.Items {
		row := podRow{Name: pod.Name, Phase: string(pod.Status.Phase)}
		for _, container := range pod.Status.ContainerStatuses {
			row.Restarts += container.RestartCount
		}
		podRows = append(podRows, row)
	}
	render(w, http.StatusOK, detailPage, detailView{
		App:         app,
		URL:         s.appURL(app),
		Env:         apps.FormatEnv(env),
		Builds:      builds,
		Pods:        podRows,
		WebhookURL:  "https://" + apps.Host("console", s.Config.Domain) + "/webhook/github",
		CanPostgres: s.Provision != nil,
		CanValkey:   s.Config.ValkeyEnabled,
	})
}

func runtimeSelector(appName string) string {
	return apps.LabelApp + "=" + appName + "," + apps.LabelKind + "!=" + build.KindBuild
}
