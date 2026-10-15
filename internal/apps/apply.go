package apps

import (
	"context"
	"errors"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var ErrNotDeployed = errors.New("app has not been deployed yet")

func (s *Store) Apply(ctx context.Context, app App, image string) error {
	return s.apply(ctx, app, image, false)
}

func (s *Store) apply(ctx context.Context, app App, image string, restart bool) error {
	workload := BuildWorkload(app, s.Config, image, time.Now().UTC().Format(time.RFC3339))
	namespace := s.Config.Namespace
	if desired := workload.Deployment; desired != nil {
		deployments := s.Client.AppsV1().Deployments(namespace)
		existing, err := deployments.Get(ctx, desired.Name, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			_, err = deployments.Create(ctx, desired, metav1.CreateOptions{})
		case err == nil:
			if !restart {
				keepRestartedAt(&desired.Spec.Template, existing.Spec.Template)
			}
			existing.Spec = desired.Spec
			_, err = deployments.Update(ctx, existing, metav1.UpdateOptions{})
		}
		if err != nil {
			return fmt.Errorf("deployment %s: %w", desired.Name, err)
		}
	}
	if desired := workload.CronJob; desired != nil {
		cronJobs := s.Client.BatchV1().CronJobs(namespace)
		existing, err := cronJobs.Get(ctx, desired.Name, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			_, err = cronJobs.Create(ctx, desired, metav1.CreateOptions{})
		case err == nil:
			if !restart {
				keepRestartedAt(&desired.Spec.JobTemplate.Spec.Template, existing.Spec.JobTemplate.Spec.Template)
			}
			existing.Spec = desired.Spec
			_, err = cronJobs.Update(ctx, existing, metav1.UpdateOptions{})
		}
		if err != nil {
			return fmt.Errorf("cronjob %s: %w", desired.Name, err)
		}
	}
	if desired := workload.Service; desired != nil {
		_, err := s.Client.CoreV1().Services(namespace).Create(ctx, desired, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("service %s: %w", desired.Name, err)
		}
	}
	if desired := workload.Ingress; desired != nil {
		_, err := s.Client.NetworkingV1().Ingresses(namespace).Create(ctx, desired, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("ingress %s: %w", desired.Name, err)
		}
	}
	return nil
}

func (s *Store) Restart(ctx context.Context, app App) error {
	if app.DeployedSHA == "" {
		return nil
	}
	return s.apply(ctx, app, ImageRef(s.Config.Registry, app.Name, app.DeployedSHA), true)
}

func keepRestartedAt(desired *corev1.PodTemplateSpec, existing corev1.PodTemplateSpec) {
	if stamp, ok := existing.Annotations[AnnotationRestartedAt]; ok {
		desired.Annotations[AnnotationRestartedAt] = stamp
	}
}

func (s *Store) RunNow(ctx context.Context, app App) (string, error) {
	cronJob, err := s.Client.BatchV1().CronJobs(s.Config.Namespace).Get(ctx, app.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", ErrNotDeployed
	}
	if err != nil {
		return "", err
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-run-%d", app.Name, time.Now().Unix()),
			Namespace: s.Config.Namespace,
			Labels:    Labels(app.Name),
		},
		Spec: cronJob.Spec.JobTemplate.Spec,
	}
	created, err := s.Client.BatchV1().Jobs(s.Config.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return created.Name, nil
}
