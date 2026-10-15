package apps

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
)

func (s *Store) Delete(ctx context.Context, name string) error {
	namespace := s.Config.Namespace
	selector := metav1.ListOptions{LabelSelector: LabelApp + "=" + name}
	background := metav1.DeleteOptions{PropagationPolicy: ptr.To(metav1.DeletePropagationBackground)}
	deployments, services, ingresses := s.Client.AppsV1().Deployments(namespace), s.Client.CoreV1().Services(namespace), s.Client.NetworkingV1().Ingresses(namespace)
	cronJobs, jobs := s.Client.BatchV1().CronJobs(namespace), s.Client.BatchV1().Jobs(namespace)
	secrets, configMaps := s.Client.CoreV1().Secrets(namespace), s.Client.CoreV1().ConfigMaps(namespace)
	kinds := []struct {
		list   func() (runtime.Object, error)
		remove func(context.Context, string, metav1.DeleteOptions) error
	}{
		{func() (runtime.Object, error) { return deployments.List(ctx, selector) }, deployments.Delete},
		{func() (runtime.Object, error) { return services.List(ctx, selector) }, services.Delete},
		{func() (runtime.Object, error) { return ingresses.List(ctx, selector) }, ingresses.Delete},
		{func() (runtime.Object, error) { return cronJobs.List(ctx, selector) }, cronJobs.Delete},
		{func() (runtime.Object, error) { return jobs.List(ctx, selector) }, jobs.Delete},
		{func() (runtime.Object, error) { return secrets.List(ctx, selector) }, secrets.Delete},
		// The app record goes last so a partial failure leaves the app listed and the delete retryable.
		{func() (runtime.Object, error) { return configMaps.List(ctx, selector) }, configMaps.Delete},
	}
	var errs []error
	for _, kind := range kinds {
		list, err := kind.list()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		items, err := meta.ExtractList(list)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, item := range items {
			object, err := meta.Accessor(item)
			if err == nil {
				err = kind.remove(ctx, object.GetName(), background)
			}
			if err != nil && !apierrors.IsNotFound(err) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
