package build

import (
	"minirail/internal/apps"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	KindBuild         = "build"
	GitHubTokenSecret = "github-token"
)

const cloneScript = `set -eu
mkdir -p /work/src /work/plan
cd /work/src
git init -q
git remote add origin "$REPO"
git -c 'credential.helper=!f() { echo username=x-access-token; echo "password=$GITHUB_TOKEN"; }; f' fetch -q --depth 1 origin "$SHA"
git checkout -q FETCH_HEAD
`

const prepareScript = `set -eu
if [ -f "/work/src/$ROOT/Dockerfile" ]; then echo "Dockerfile found, skipping railpack"; exit 0; fi
railpack prepare "/work/src/$ROOT" --plan-out /work/plan/railpack-plan.json --info-out /work/plan/railpack-info.json
`

const buildScript = `set -eu
CTX="/work/src/$ROOT"
if [ -f /work/plan/railpack-plan.json ]; then
  set -- --frontend=gateway.v0 --opt source="$RAILPACK_FRONTEND" --local context="$CTX" --local dockerfile=/work/plan
else
  set -- --frontend=dockerfile.v0 --local context="$CTX" --local dockerfile="$CTX"
fi
exec buildctl-daemonless.sh build "$@" \
  --output "type=image,name=$IMAGE,push=true" \
  --export-cache "type=registry,ref=$CACHE,mode=max" \
  --import-cache "type=registry,ref=$CACHE"
`

func JobName(appName, sha string) string { return appName + "-build-" + sha[:7] }

func CacheRef(registry, appName string) string { return registry + "/" + appName + ":buildcache" }

func NewJob(config apps.Config, app apps.App, sha string) *batchv1.Job {
	labels := map[string]string{apps.LabelApp: app.Name, apps.LabelKind: KindBuild}
	work := corev1.VolumeMount{Name: "work", MountPath: "/work"}
	unconfined := &corev1.SecurityContext{
		RunAsUser:       ptr.To[int64](1000),
		RunAsGroup:      ptr.To[int64](1000),
		SeccompProfile:  &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined},
		AppArmorProfile: &corev1.AppArmorProfile{Type: corev1.AppArmorProfileTypeUnconfined},
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        JobName(app.Name, sha),
			Namespace:   config.Namespace,
			Labels:      labels,
			Annotations: map[string]string{apps.AnnotationSHA: sha},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To[int32](0),
			ActiveDeadlineSeconds:   ptr.To[int64](1800),
			TTLSecondsAfterFinished: ptr.To[int32](86400),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy:    corev1.RestartPolicyNever,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: apps.RegistrySecret}},
					InitContainers: []corev1.Container{
						{
							Name:    "clone",
							Image:   config.ControlImage,
							Command: []string{"sh", "-c", cloneScript},
							Env: []corev1.EnvVar{
								{Name: "REPO", Value: app.Repo},
								{Name: "SHA", Value: sha},
								{Name: "HOME", Value: "/tmp"},
								{Name: "GITHUB_TOKEN", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: GitHubTokenSecret},
									Key:                  "token",
								}}},
							},
							VolumeMounts: []corev1.VolumeMount{work},
						},
						{
							Name:         "prepare",
							Image:        config.ControlImage,
							Command:      []string{"sh", "-c", prepareScript},
							Env:          []corev1.EnvVar{{Name: "ROOT", Value: app.Root}, {Name: "HOME", Value: "/tmp"}},
							VolumeMounts: []corev1.VolumeMount{work},
						},
					},
					Containers: []corev1.Container{{
						Name:            "build",
						Image:           config.BuildkitImage,
						Command:         []string{"sh", "-c", buildScript},
						SecurityContext: unconfined,
						Env: []corev1.EnvVar{
							{Name: "BUILDKITD_FLAGS", Value: "--oci-worker-no-process-sandbox"},
							{Name: "ROOT", Value: app.Root},
							{Name: "IMAGE", Value: apps.ImageRef(config.Registry, app.Name, sha)},
							{Name: "CACHE", Value: CacheRef(config.Registry, app.Name)},
							{Name: "RAILPACK_FRONTEND", Value: config.RailpackFrontend},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m"), corev1.ResourceMemory: resource.MustParse("512Mi")},
							Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("3Gi")},
						},
						VolumeMounts: []corev1.VolumeMount{
							work,
							{Name: "registry", MountPath: "/home/user/.docker", ReadOnly: true},
							{Name: "buildkitd", MountPath: "/home/user/.local/share/buildkit"},
						},
					}},
					Volumes: []corev1.Volume{
						{Name: "work", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						{Name: "buildkitd", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						{Name: "registry", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
							SecretName: apps.RegistrySecret,
							Items:      []corev1.KeyToPath{{Key: corev1.DockerConfigJsonKey, Path: "config.json"}},
						}}},
					},
				},
			},
		},
	}
}
