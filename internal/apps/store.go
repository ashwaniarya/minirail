package apps

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	ErrExists   = errors.New("app already exists")
	ErrNotFound = errors.New("app not found")
)

type Store struct {
	Client kubernetes.Interface
	Config Config
}

func (s *Store) Create(ctx context.Context, app App) error {
	_, err := s.Client.CoreV1().ConfigMaps(s.Config.Namespace).Create(ctx, app.ToConfigMap(s.Config.Namespace), metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("%w: %s", ErrExists, app.Name)
	}
	return err
}

func (s *Store) Get(ctx context.Context, name string) (App, error) {
	configMap, err := s.Client.CoreV1().ConfigMaps(s.Config.Namespace).Get(ctx, ConfigMapName(name), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return App{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return App{}, err
	}
	return FromConfigMap(configMap)
}

func (s *Store) List(ctx context.Context) ([]App, error) {
	configMaps, err := s.Client.CoreV1().ConfigMaps(s.Config.Namespace).List(ctx, metav1.ListOptions{LabelSelector: LabelApp})
	if err != nil {
		return nil, err
	}
	var all []App
	for _, configMap := range configMaps.Items {
		app, err := FromConfigMap(&configMap)
		if err != nil {
			return nil, err
		}
		all = append(all, app)
	}
	slices.SortFunc(all, func(left, right App) int { return strings.Compare(left.Name, right.Name) })
	return all, nil
}

func (s *Store) Update(ctx context.Context, app App) error {
	configMaps := s.Client.CoreV1().ConfigMaps(s.Config.Namespace)
	existing, err := configMaps.Get(ctx, ConfigMapName(app.Name), metav1.GetOptions{})
	if err != nil {
		return err
	}
	existing.Data = app.ToConfigMap(s.Config.Namespace).Data
	_, err = configMaps.Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (s *Store) Env(ctx context.Context, name string) (map[string]string, error) {
	secret, err := s.Client.CoreV1().Secrets(s.Config.Namespace).Get(ctx, EnvSecretName(name), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	for key, value := range secret.Data {
		env[key] = string(value)
	}
	return env, nil
}

func (s *Store) SetEnv(ctx context.Context, name string, env map[string]string) error {
	return s.upsertSecret(ctx, name, EnvSecretName(name), env)
}

func (s *Store) SetDatabaseURL(ctx context.Context, name, databaseURL string) error {
	return s.upsertSecret(ctx, name, DBSecretName(name), map[string]string{"DATABASE_URL": databaseURL})
}

func (s *Store) upsertSecret(ctx context.Context, appName, secretName string, values map[string]string) error {
	data := map[string][]byte{}
	for key, value := range values {
		data[key] = []byte(value)
	}
	secrets := s.Client.CoreV1().Secrets(s.Config.Namespace)
	existing, err := secrets.Get(ctx, secretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = secrets.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: s.Config.Namespace, Labels: Labels(appName)},
			Data:       data,
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Data = data
	_, err = secrets.Update(ctx, existing, metav1.UpdateOptions{})
	return err
}
