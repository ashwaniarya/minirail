package web

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"minirail/internal/apps"
	"minirail/internal/build"
)

var (
	indexPage  = page("index")
	newAppPage = page("new")
)

type appRow struct {
	App       apps.App
	URL       string
	LastBuild string
}

type newAppView struct {
	App   apps.App
	Error string
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.List(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	rows := make([]appRow, 0, len(list))
	for _, app := range list {
		builds, err := s.Builder.Builds(r.Context(), app.Name)
		if err != nil {
			fail(w, err)
			return
		}
		row := appRow{App: app, URL: s.appURL(app), LastBuild: "none"}
		if len(builds) > 0 {
			row.LastBuild = build.Status(builds[0])
		}
		rows = append(rows, row)
	}
	render(w, http.StatusOK, indexPage, rows)
}

func (s *Server) newForm(w http.ResponseWriter, r *http.Request) {
	defaults := apps.App{Branch: "main", Type: apps.Web, Port: 8080, Replicas: 1, Root: "."}
	render(w, http.StatusOK, newAppPage, newAppView{App: defaults})
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	app := appFromForm(r)
	if err := app.Validate(); err != nil {
		render(w, http.StatusBadRequest, newAppPage, newAppView{App: app, Error: err.Error()})
		return
	}
	if err := s.Store.Create(r.Context(), app); errors.Is(err, apps.ErrExists) {
		render(w, http.StatusConflict, newAppPage, newAppView{App: app, Error: err.Error()})
		return
	} else if err != nil {
		fail(w, err)
		return
	}
	if _, err := s.Builder.Redeploy(r.Context(), app); err != nil {
		log.Printf("first build of %s: %v", app.Name, err)
		http.Error(w, "App created, but the first build could not start: "+err.Error()+". Press Deploy on the app page to retry.", http.StatusBadGateway)
		return
	}
	http.Redirect(w, r, "/apps/"+app.Name, http.StatusSeeOther)
}

func appFromForm(r *http.Request) apps.App {
	number := func(key string) int {
		value, _ := strconv.Atoi(r.PostFormValue(key))
		return value
	}
	return apps.App{
		Name:     strings.TrimSpace(r.PostFormValue("name")),
		Repo:     apps.NormalizeRepo(r.PostFormValue("repo")),
		Branch:   strings.TrimSpace(r.PostFormValue("branch")),
		Type:     apps.Type(r.PostFormValue("type")),
		Port:     number("port"),
		Schedule: strings.TrimSpace(r.PostFormValue("schedule")),
		Replicas: number("replicas"),
		Root:     strings.TrimSpace(r.PostFormValue("root")),
	}
}
