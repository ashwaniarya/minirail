package apps

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Type string

const (
	Web    Type = "web"
	Worker Type = "worker"
	Cron   Type = "cron"
)

const (
	LabelApp              = "paas/app"
	LabelKind             = "paas/kind"
	AnnotationSHA         = "paas/sha"
	AnnotationRestartedAt = "paas/restarted-at"
)

type App struct {
	Name        string
	Repo        string
	Branch      string
	Type        Type
	Port        int
	Schedule    string
	Replicas    int
	Root        string
	Postgres    bool
	Valkey      bool
	DeployedSHA string
}

var (
	namePattern   = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	repoPattern   = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	branchPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	rootPattern   = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	shaPattern    = regexp.MustCompile(`^[0-9a-f]{40}$`)
	reservedNames = []string{"console", "www", "minirail"}
)

func Labels(name string) map[string]string {
	return map[string]string{LabelApp: name}
}

func ConfigMapName(name string) string { return name + "-app" }
func EnvSecretName(name string) string { return name + "-env" }
func DBSecretName(name string) string  { return name + "-db" }

func (a App) Validate() error {
	switch {
	case len(a.Name) > 30 || !namePattern.MatchString(a.Name):
		return errors.New("name must be 1-30 lowercase letters, digits or dashes, starting and ending with a letter or digit")
	case slices.Contains(reservedNames, a.Name):
		return fmt.Errorf("name %q is reserved", a.Name)
	case !repoPattern.MatchString(a.Repo):
		return errors.New("repo must look like https://github.com/<owner>/<repo>")
	case len(a.Branch) > 100 || !branchPattern.MatchString(a.Branch) || strings.Contains(a.Branch, "..") || strings.HasPrefix(a.Branch, "-"):
		return errors.New("branch may only contain letters, digits, '.', '_', '/' and '-'")
	case !rootPattern.MatchString(a.Root) || path.IsAbs(a.Root) || slices.Contains(strings.Split(a.Root, "/"), ".."):
		return errors.New("root must be a relative path inside the repo")
	}
	switch a.Type {
	case Web:
		if a.Port < 1 || a.Port > 65535 {
			return errors.New("port must be between 1 and 65535")
		}
		return validateReplicas(a.Replicas)
	case Worker:
		return validateReplicas(a.Replicas)
	case Cron:
		return validateSchedule(a.Schedule)
	}
	return errors.New("type must be web, worker or cron")
}

func validateReplicas(replicas int) error {
	if replicas < 1 || replicas > 5 {
		return errors.New("replicas must be between 1 and 5")
	}
	return nil
}

func validateSchedule(schedule string) error {
	if len(strings.Fields(schedule)) != 5 {
		return errors.New("schedule must have exactly 5 fields, e.g. */15 * * * *")
	}
	if _, err := cron.ParseStandard(schedule); err != nil {
		return fmt.Errorf("schedule: %w", err)
	}
	return nil
}

func NormalizeRepo(repo string) string {
	repo = strings.TrimRight(strings.ToLower(strings.TrimSpace(repo)), "/")
	return strings.TrimSuffix(repo, ".git")
}

func ValidEnvKey(key string) bool { return envKeyPattern.MatchString(key) }
func ValidSHA(sha string) bool    { return shaPattern.MatchString(sha) }

func ParseEnv(text string) (map[string]string, error) {
	env := map[string]string{}
	for index, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || !ValidEnvKey(key) {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE where KEY is letters, digits and '_'", index+1)
		}
		env[key] = value
	}
	return env, nil
}

func FormatEnv(env map[string]string) string {
	var builder strings.Builder
	for _, key := range slices.Sorted(maps.Keys(env)) {
		builder.WriteString(key + "=" + env[key] + "\n")
	}
	return builder.String()
}

func (a App) ToConfigMap(namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName(a.Name), Namespace: namespace, Labels: Labels(a.Name)},
		Data: map[string]string{
			"repo":         a.Repo,
			"branch":       a.Branch,
			"type":         string(a.Type),
			"port":         strconv.Itoa(a.Port),
			"schedule":     a.Schedule,
			"replicas":     strconv.Itoa(a.Replicas),
			"root":         a.Root,
			"postgres":     strconv.FormatBool(a.Postgres),
			"valkey":       strconv.FormatBool(a.Valkey),
			"deployed-sha": a.DeployedSHA,
		},
	}
}

func FromConfigMap(configMap *corev1.ConfigMap) (App, error) {
	data := configMap.Data
	port, portErr := strconv.Atoi(data["port"])
	replicas, replicasErr := strconv.Atoi(data["replicas"])
	if err := errors.Join(portErr, replicasErr); err != nil {
		return App{}, fmt.Errorf("app record %s is corrupt: %w", configMap.Name, err)
	}
	return App{
		Name:        configMap.Labels[LabelApp],
		Repo:        data["repo"],
		Branch:      data["branch"],
		Type:        Type(data["type"]),
		Port:        port,
		Schedule:    data["schedule"],
		Replicas:    replicas,
		Root:        data["root"],
		Postgres:    data["postgres"] == "true",
		Valkey:      data["valkey"] == "true",
		DeployedSHA: data["deployed-sha"],
	}, nil
}
