package build

import (
	"strings"
	"testing"

	"minirail/internal/apps"
)

func testConfig() apps.Config {
	return apps.Config{
		Namespace:        "apps",
		Domain:           "apps.example.com",
		Registry:         "registry.digitalocean.com/acme",
		ControlImage:     "registry.digitalocean.com/acme/minirail:abc",
		BuildkitImage:    "moby/buildkit:v0.33.1-rootless",
		RailpackFrontend: "ghcr.io/railwayapp/railpack-frontend:v0.40.1",
	}
}

func testApp() apps.App {
	return apps.App{Name: "hello", Repo: "https://github.com/acme/hello", Branch: "main", Type: apps.Web, Port: 8080, Replicas: 1, Root: "examples/hello"}
}

const shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func envValue(t *testing.T, containerEnv map[string]string, key string) string {
	t.Helper()
	value, found := containerEnv[key]
	if !found {
		t.Fatalf("env %s missing", key)
	}
	return value
}

func TestNewJobWiresSourceImageAndCache(t *testing.T) {
	job := NewJob(testConfig(), testApp(), shaA)
	if job.Name != "hello-build-aaaaaaa" || job.Annotations[apps.AnnotationSHA] != shaA {
		t.Fatalf("job meta = %+v", job.ObjectMeta)
	}
	for _, labels := range []map[string]string{job.Labels, job.Spec.Template.Labels} {
		if labels[apps.LabelApp] != "hello" || labels[apps.LabelKind] != KindBuild {
			t.Fatalf("labels = %v", labels)
		}
	}
	if *job.Spec.BackoffLimit != 0 || *job.Spec.ActiveDeadlineSeconds != 1800 {
		t.Fatal("build must fail fast and time out after 30 minutes")
	}
	pod := job.Spec.Template.Spec
	if len(pod.InitContainers) != 2 || pod.InitContainers[0].Name != "clone" || pod.InitContainers[1].Name != "prepare" {
		t.Fatalf("init containers = %+v", pod.InitContainers)
	}
	clone := pod.InitContainers[0]
	cloneEnv := map[string]string{}
	for _, variable := range clone.Env {
		cloneEnv[variable.Name] = variable.Value
		if variable.Name == "GITHUB_TOKEN" && variable.ValueFrom.SecretKeyRef.Name != GitHubTokenSecret {
			t.Fatal("token must come from the github-token secret")
		}
	}
	if envValue(t, cloneEnv, "SHA") != shaA || envValue(t, cloneEnv, "REPO") != "https://github.com/acme/hello" {
		t.Fatalf("clone env = %v", cloneEnv)
	}
	build := pod.Containers[0]
	buildEnv := map[string]string{}
	for _, variable := range build.Env {
		buildEnv[variable.Name] = variable.Value
	}
	if envValue(t, buildEnv, "IMAGE") != "registry.digitalocean.com/acme/hello:"+shaA {
		t.Fatalf("image = %q", buildEnv["IMAGE"])
	}
	if envValue(t, buildEnv, "CACHE") != "registry.digitalocean.com/acme/hello:buildcache" || envValue(t, buildEnv, "ROOT") != "examples/hello" || envValue(t, buildEnv, "REGISTRY_OPTS") != "" {
		t.Fatalf("build env = %v", buildEnv)
	}
	if build.Image != "moby/buildkit:v0.33.1-rootless" || *build.SecurityContext.RunAsUser != 1000 {
		t.Fatalf("build container = %+v", build)
	}
}

func TestNewJobUsesPlainHTTPForAnInsecureRegistry(t *testing.T) {
	config := testConfig()
	config.RegistryInsecure = true
	build := NewJob(config, testApp(), shaA).Spec.Template.Spec.Containers[0]
	buildEnv := map[string]string{}
	for _, variable := range build.Env {
		buildEnv[variable.Name] = variable.Value
	}
	if envValue(t, buildEnv, "REGISTRY_OPTS") != ",registry.insecure=true" {
		t.Fatalf("build env = %v", buildEnv)
	}
	if strings.Count(buildScript, "$REGISTRY_OPTS") != 3 {
		t.Fatal("push, cache export and cache import must all carry REGISTRY_OPTS")
	}
}
