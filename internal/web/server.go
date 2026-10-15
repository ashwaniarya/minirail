package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"errors"
	"html/template"
	"log"
	"net/http"

	"minirail/internal/apps"
	"minirail/internal/build"
)

const maxBodyBytes = 1 << 20

var errBadRequest = errors.New("bad request")

//go:embed templates
var templateFiles embed.FS

var templateFuncs = template.FuncMap{
	"short": func(sha string) string {
		if len(sha) > 7 {
			return sha[:7]
		}
		return sha
	},
	"buildStatus": build.Status,
}

func page(name string) *template.Template {
	return template.Must(template.New(name).Funcs(templateFuncs).ParseFS(templateFiles, "templates/layout.html", "templates/"+name+".html"))
}

type Server struct {
	Config    apps.Config
	Store     *apps.Store
	Builder   *build.Builder
	Provision func(ctx context.Context, appName string) (string, error)
}

func (s *Server) Handler() http.Handler {
	dashboard := http.NewServeMux()
	dashboard.HandleFunc("GET /{$}", s.index)
	dashboard.HandleFunc("GET /apps/new", s.newForm)
	dashboard.HandleFunc("POST /apps/new", s.create)
	dashboard.HandleFunc("GET /apps/{name}", s.detail)
	dashboard.HandleFunc("POST /apps/{name}/env", s.appAction(s.saveEnv))
	dashboard.HandleFunc("POST /apps/{name}/deploy", s.appAction(s.deploy))
	dashboard.HandleFunc("POST /apps/{name}/run", s.appAction(s.run))
	dashboard.HandleFunc("POST /apps/{name}/attach/postgres", s.appAction(s.attachPostgres))
	dashboard.HandleFunc("POST /apps/{name}/attach/valkey", s.appAction(s.attachValkey))
	dashboard.HandleFunc("POST /apps/{name}/delete", s.remove)
	dashboard.HandleFunc("GET /apps/{name}/logs", s.logs)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	root.HandleFunc("POST /webhook/github", s.webhook)
	root.Handle("/", s.requireAdmin(http.NewCrossOriginProtection().Handler(dashboard)))
	return http.MaxBytesHandler(root, maxBodyBytes)
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	want := sha256.Sum256([]byte("admin:" + s.Config.AdminPassword))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, _ := r.BasicAuth()
		got := sha256.Sum256([]byte(user + ":" + password))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="minirail", charset="UTF-8"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func render(w http.ResponseWriter, status int, tmpl *template.Template, data any) {
	var body bytes.Buffer
	if err := tmpl.ExecuteTemplate(&body, "layout", data); err != nil {
		log.Printf("render %s: %v", tmpl.Name(), err)
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	body.WriteTo(w)
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errBadRequest):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, apps.ErrNotFound), errors.Is(err, errNoLogs):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, apps.ErrExists), errors.Is(err, apps.ErrNotDeployed):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		log.Printf("cluster error: %v", err)
		http.Error(w, "cluster request failed: "+err.Error(), http.StatusBadGateway)
	}
}

func (s *Server) appURL(app apps.App) string {
	if app.Type != apps.Web {
		return ""
	}
	return s.Config.Scheme + "://" + apps.Host(app.Name, s.Config.Domain)
}
