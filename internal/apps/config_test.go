package apps

import (
	"strings"
	"testing"
)

func fakeEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func requiredEnv() map[string]string {
	return map[string]string{
		"MINIRAIL_DOMAIN":        "apps.example.com",
		"MINIRAIL_REGISTRY":      "registry.digitalocean.com/acme",
		"MINIRAIL_CONTROL_IMAGE": "registry.digitalocean.com/acme/minirail:abc",
		"GITHUB_TOKEN":           "token",
		"WEBHOOK_SECRET":         "secret",
		"ADMIN_PASSWORD":         "password",
	}
}

func TestLoadConfigAppliesDefaults(t *testing.T) {
	config, err := LoadConfig(fakeEnv(requiredEnv()))
	if err != nil {
		t.Fatal(err)
	}
	if config.Namespace != "apps" || config.Addr != ":8080" || config.ValkeyEnabled || config.Scheme != "https" || config.RegistryInsecure {
		t.Fatalf("defaults not applied: %+v", config)
	}
	if config.BuildkitImage != "moby/buildkit:v0.33.1-rootless" {
		t.Fatalf("buildkit image = %q", config.BuildkitImage)
	}
	if config.RailpackFrontend != "ghcr.io/railwayapp/railpack-frontend:v0.40.1" {
		t.Fatalf("railpack frontend = %q", config.RailpackFrontend)
	}
}

func TestLoadConfigNamesEveryMissingKey(t *testing.T) {
	_, err := LoadConfig(fakeEnv(map[string]string{}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, key := range []string{"MINIRAIL_DOMAIN", "MINIRAIL_REGISTRY", "MINIRAIL_CONTROL_IMAGE", "WEBHOOK_SECRET", "ADMIN_PASSWORD"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not name %s", err, key)
		}
	}
}

func TestLoadConfigValkeyFlag(t *testing.T) {
	env := requiredEnv()
	env["MINIRAIL_VALKEY"] = "true"
	config, err := LoadConfig(fakeEnv(env))
	if err != nil {
		t.Fatal(err)
	}
	if !config.ValkeyEnabled {
		t.Fatal("valkey should be enabled")
	}
}

func TestLoadConfigAllowsNoGitHubTokenForPublicRepos(t *testing.T) {
	env := requiredEnv()
	delete(env, "GITHUB_TOKEN")
	if _, err := LoadConfig(fakeEnv(env)); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfigLocalFlags(t *testing.T) {
	env := requiredEnv()
	env["MINIRAIL_SCHEME"] = "http"
	env["MINIRAIL_REGISTRY_INSECURE"] = "true"
	config, err := LoadConfig(fakeEnv(env))
	if err != nil {
		t.Fatal(err)
	}
	if config.Scheme != "http" || !config.RegistryInsecure {
		t.Fatalf("local flags not applied: %+v", config)
	}
}

func TestLoadConfigRejectsUnknownScheme(t *testing.T) {
	env := requiredEnv()
	env["MINIRAIL_SCHEME"] = "ftp"
	if _, err := LoadConfig(fakeEnv(env)); err == nil || !strings.Contains(err.Error(), "MINIRAIL_SCHEME") {
		t.Fatalf("err = %v", err)
	}
}
