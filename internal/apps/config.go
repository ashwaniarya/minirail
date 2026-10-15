package apps

import (
	"fmt"
	"strings"
)

type Config struct {
	Namespace        string
	Domain           string
	Scheme           string
	Registry         string
	RegistryInsecure bool
	ControlImage     string
	BuildkitImage    string
	RailpackFrontend string
	GitHubToken      string
	WebhookSecret    string
	AdminPassword    string
	PGAdminURL       string
	ValkeyEnabled    bool
	Addr             string
}

func LoadConfig(getenv func(string) string) (Config, error) {
	valueOr := func(key, fallback string) string {
		if value := getenv(key); value != "" {
			return value
		}
		return fallback
	}
	config := Config{
		Namespace:        valueOr("MINIRAIL_NAMESPACE", "apps"),
		Domain:           getenv("MINIRAIL_DOMAIN"),
		Scheme:           valueOr("MINIRAIL_SCHEME", "https"),
		Registry:         getenv("MINIRAIL_REGISTRY"),
		RegistryInsecure: getenv("MINIRAIL_REGISTRY_INSECURE") == "true",
		ControlImage:     getenv("MINIRAIL_CONTROL_IMAGE"),
		BuildkitImage:    valueOr("MINIRAIL_BUILDKIT_IMAGE", "moby/buildkit:v0.33.1-rootless"),
		RailpackFrontend: valueOr("MINIRAIL_RAILPACK_FRONTEND", "ghcr.io/railwayapp/railpack-frontend:v0.40.1"),
		GitHubToken:      getenv("GITHUB_TOKEN"),
		WebhookSecret:    getenv("WEBHOOK_SECRET"),
		AdminPassword:    getenv("ADMIN_PASSWORD"),
		PGAdminURL:       getenv("PG_ADMIN_URL"),
		ValkeyEnabled:    getenv("MINIRAIL_VALKEY") == "true",
		Addr:             valueOr("MINIRAIL_ADDR", ":8080"),
	}
	var missing []string
	for _, required := range []struct{ key, value string }{
		{"MINIRAIL_DOMAIN", config.Domain},
		{"MINIRAIL_REGISTRY", config.Registry},
		{"MINIRAIL_CONTROL_IMAGE", config.ControlImage},
		{"WEBHOOK_SECRET", config.WebhookSecret},
		{"ADMIN_PASSWORD", config.AdminPassword},
	} {
		if required.value == "" {
			missing = append(missing, required.key)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}
	if config.Scheme != "https" && config.Scheme != "http" {
		return Config{}, fmt.Errorf("MINIRAIL_SCHEME must be http or https, got %q", config.Scheme)
	}
	return config, nil
}
