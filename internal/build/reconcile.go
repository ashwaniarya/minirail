package build

import (
	"context"
	"errors"
	"log"
	"time"

	"minirail/internal/apps"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (b *Builder) Reconcile(ctx context.Context) error {
	list, err := b.Client.BatchV1().Jobs(b.Config.Namespace).List(ctx, metav1.ListOptions{LabelSelector: apps.LabelKind + "=" + KindBuild})
	if err != nil {
		return err
	}
	newestSucceeded := map[string]batchv1.Job{}
	for _, job := range list.Items {
		if Status(job) != StatusSucceeded {
			continue
		}
		appName := job.Labels[apps.LabelApp]
		if current, seen := newestSucceeded[appName]; !seen || newestFirst(job, current) < 0 {
			newestSucceeded[appName] = job
		}
	}
	var errs []error
	for appName, job := range newestSucceeded {
		sha := job.Annotations[apps.AnnotationSHA]
		app, err := b.Store.Get(ctx, appName)
		if errors.Is(err, apps.ErrNotFound) || !apps.ValidSHA(sha) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if sha == app.DeployedSHA {
			continue
		}
		if err := b.Store.Apply(ctx, app, apps.ImageRef(b.Config.Registry, app.Name, sha)); err != nil {
			errs = append(errs, err)
			continue
		}
		app.DeployedSHA = sha
		if err := b.Store.Update(ctx, app); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (b *Builder) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := b.Reconcile(ctx); err != nil {
				log.Printf("reconcile: %v", err)
			}
		}
	}
}
