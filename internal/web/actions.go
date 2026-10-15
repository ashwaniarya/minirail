package web

import (
	"context"
	"fmt"
	"net/http"

	"minirail/internal/apps"
)

func (s *Server) appAction(handle func(r *http.Request, app apps.App) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		app, err := s.Store.Get(r.Context(), r.PathValue("name"))
		if err == nil {
			err = handle(r, app)
		}
		if err != nil {
			fail(w, err)
			return
		}
		http.Redirect(w, r, "/apps/"+app.Name, http.StatusSeeOther)
	}
}

func (s *Server) saveEnv(r *http.Request, app apps.App) error {
	env, err := apps.ParseEnv(r.PostFormValue("env"))
	if err != nil {
		return fmt.Errorf("%w: %w", errBadRequest, err)
	}
	if err := s.Store.SetEnv(r.Context(), app.Name, env); err != nil {
		return err
	}
	return s.Store.Restart(r.Context(), app)
}

func (s *Server) deploy(r *http.Request, app apps.App) error {
	_, err := s.Builder.Redeploy(r.Context(), app)
	return err
}

func (s *Server) run(r *http.Request, app apps.App) error {
	if app.Type != apps.Cron {
		return fmt.Errorf("%w: only cron apps can run on demand", errBadRequest)
	}
	_, err := s.Store.RunNow(r.Context(), app)
	return err
}

func (s *Server) attachPostgres(r *http.Request, app apps.App) error {
	if s.Provision == nil {
		return fmt.Errorf("%w: postgres is not configured", errBadRequest)
	}
	databaseURL, err := s.Provision(r.Context(), app.Name)
	if err != nil {
		return fmt.Errorf("provision postgres for %s: %w", app.Name, err)
	}
	if err := s.Store.SetDatabaseURL(r.Context(), app.Name, databaseURL); err != nil {
		return err
	}
	app.Postgres = true
	return s.saveAndRestart(r.Context(), app)
}

func (s *Server) attachValkey(r *http.Request, app apps.App) error {
	if !s.Config.ValkeyEnabled {
		return fmt.Errorf("%w: valkey is not enabled", errBadRequest)
	}
	app.Valkey = true
	return s.saveAndRestart(r.Context(), app)
}

func (s *Server) saveAndRestart(ctx context.Context, app apps.App) error {
	if err := s.Store.Update(ctx, app); err != nil {
		return err
	}
	return s.Store.Restart(ctx, app)
}

func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Delete(r.Context(), r.PathValue("name")); err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
