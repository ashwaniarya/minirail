package build

import (
	"context"
	"slices"
	"strings"

	"minirail/internal/apps"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

const (
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

type Builder struct {
	Client      kubernetes.Interface
	Config      apps.Config
	Store       *apps.Store
	ResolveHead func(ctx context.Context, repo, branch string) (string, error)
}

func Status(job batchv1.Job) string {
	for _, condition := range job.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case batchv1.JobComplete:
			return StatusSucceeded
		case batchv1.JobFailed:
			return StatusFailed
		}
	}
	return StatusRunning
}

func (b *Builder) Trigger(ctx context.Context, app apps.App, sha string) (bool, error) {
	if sha == app.DeployedSHA {
		return false, nil
	}
	jobs := b.Client.BatchV1().Jobs(b.Config.Namespace)
	name := JobName(app.Name, sha)
	existing, err := jobs.Get(ctx, name, metav1.GetOptions{})
	switch {
	case err == nil && Status(*existing) == StatusRunning:
		return false, nil
	case err == nil:
		// A finished Job is rebuilt rather than reused so it becomes the newest build and Reconcile deploys it.
		background := metav1.DeleteOptions{PropagationPolicy: ptr.To(metav1.DeletePropagationBackground)}
		if err := jobs.Delete(ctx, name, background); err != nil && !apierrors.IsNotFound(err) {
			return false, err
		}
	case !apierrors.IsNotFound(err):
		return false, err
	}
	if _, err := jobs.Create(ctx, NewJob(b.Config, app, sha), metav1.CreateOptions{}); err != nil {
		return false, err
	}
	return true, nil
}

func (b *Builder) Redeploy(ctx context.Context, app apps.App) (bool, error) {
	sha, err := b.ResolveHead(ctx, app.Repo, app.Branch)
	if err != nil {
		return false, err
	}
	return b.Trigger(ctx, app, sha)
}

func (b *Builder) Builds(ctx context.Context, appName string) ([]batchv1.Job, error) {
	selector := apps.LabelApp + "=" + appName + "," + apps.LabelKind + "=" + KindBuild
	list, err := b.Client.BatchV1().Jobs(b.Config.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(list.Items, newestFirst)
	return list.Items, nil
}

func newestFirst(left, right batchv1.Job) int {
	if order := right.CreationTimestamp.Compare(left.CreationTimestamp.Time); order != 0 {
		return order
	}
	return strings.Compare(right.Name, left.Name)
}
