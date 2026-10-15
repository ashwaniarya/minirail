package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"minirail/internal/apps"
	"minirail/internal/build"
	"minirail/internal/pg"
	"minirail/internal/web"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	config, err := apps.LoadConfig(os.Getenv)
	if err != nil {
		return err
	}
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return err
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	store := &apps.Store{Client: client, Config: config}
	builder := &build.Builder{Client: client, Config: config, Store: store, ResolveHead: build.LsRemote(config.GitHubToken)}
	server := &web.Server{Config: config, Store: store, Builder: builder}
	if config.PGAdminURL != "" {
		server.Provision = pg.Provisioner{AdminURL: config.PGAdminURL}.Provision
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go builder.Run(ctx, 10*time.Second)

	// No WriteTimeout: the log stream stays open while it follows a pod.
	httpServer := &http.Server{Addr: config.Addr, Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpServer.Shutdown(shutdownCtx)
	}()
	log.Printf("minirail listening on %s", config.Addr)
	if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
