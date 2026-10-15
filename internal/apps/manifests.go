package apps

import (
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

const (
	RegistrySecret = "registry"
	ValkeySecret   = "valkey"
)

type Workload struct {
	Deployment *appsv1.Deployment    `json:"deployment,omitempty"`
	Service    *corev1.Service       `json:"service,omitempty"`
	Ingress    *networkingv1.Ingress `json:"ingress,omitempty"`
	CronJob    *batchv1.CronJob      `json:"cronJob,omitempty"`
}

func ImageRef(registry, name, sha string) string {
	return registry + "/" + name + ":" + sha
}

func Host(name, domain string) string { return name + "." + domain }

func BuildWorkload(app App, config Config, image, restartedAt string) Workload {
	meta := metav1.ObjectMeta{Name: app.Name, Namespace: config.Namespace, Labels: Labels(app.Name)}
	podLabels := Labels(app.Name)
	podLabels[LabelKind] = string(app.Type)
	template := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: podLabels, Annotations: map[string]string{AnnotationRestartedAt: restartedAt}},
		Spec:       podSpec(app, image),
	}
	if app.Type == Cron {
		template.Spec.RestartPolicy = corev1.RestartPolicyNever
		return Workload{CronJob: &batchv1.CronJob{
			ObjectMeta: meta,
			Spec: batchv1.CronJobSpec{
				Schedule:          app.Schedule,
				ConcurrencyPolicy: batchv1.ForbidConcurrent,
				JobTemplate: batchv1.JobTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
					Spec:       batchv1.JobSpec{BackoffLimit: ptr.To[int32](0), Template: template},
				},
			},
		}}
	}
	workload := Workload{Deployment: &appsv1.Deployment{
		ObjectMeta: meta,
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(app.Replicas)),
			Selector: &metav1.LabelSelector{MatchLabels: podLabels},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: ptr.To(intstr.FromInt32(0)),
					MaxSurge:       ptr.To(intstr.FromInt32(1)),
				},
			},
			Template: template,
		},
	}}
	if app.Type == Web {
		workload.Service = &corev1.Service{
			ObjectMeta: meta,
			Spec: corev1.ServiceSpec{
				Selector: podLabels,
				Ports:    []corev1.ServicePort{{Name: "http", Port: 80, TargetPort: intstr.FromInt32(int32(app.Port))}},
			},
		}
		workload.Ingress = &networkingv1.Ingress{
			ObjectMeta: meta,
			Spec: networkingv1.IngressSpec{
				IngressClassName: ptr.To("traefik"),
				Rules: []networkingv1.IngressRule{{
					Host: Host(app.Name, config.Domain),
					IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/",
							PathType: ptr.To(networkingv1.PathTypePrefix),
							Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{
								Name: app.Name,
								Port: networkingv1.ServiceBackendPort{Number: 80},
							}},
						}},
					}},
				}},
			},
		}
	}
	return workload
}

func podSpec(app App, image string) corev1.PodSpec {
	container := corev1.Container{
		Name:  "app",
		Image: image,
		EnvFrom: []corev1.EnvFromSource{
			{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: EnvSecretName(app.Name)}, Optional: ptr.To(true)}},
			{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: DBSecretName(app.Name)}, Optional: ptr.To(true)}},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")},
		},
	}
	if app.Valkey {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: ValkeySecret}},
		})
	}
	if app.Type == Web {
		container.Env = []corev1.EnvVar{{Name: "PORT", Value: strconv.Itoa(app.Port)}}
		container.Ports = []corev1.ContainerPort{{Name: "http", ContainerPort: int32(app.Port)}}
		container.ReadinessProbe = &corev1.Probe{
			ProbeHandler:  corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(int32(app.Port))}},
			PeriodSeconds: 5,
		}
	}
	return corev1.PodSpec{
		Containers:       []corev1.Container{container},
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: RegistrySecret}},
	}
}
