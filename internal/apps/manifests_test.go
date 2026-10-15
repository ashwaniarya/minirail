package apps

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func testConfig() Config {
	return Config{Namespace: "apps", Domain: "apps.example.com", Registry: "registry.digitalocean.com/acme"}
}

func TestBuildWorkloadGolden(t *testing.T) {
	worker := validWeb()
	worker.Type, worker.Port, worker.Replicas = Worker, 0, 2
	cronApp := validWeb()
	cronApp.Type, cronApp.Port, cronApp.Replicas, cronApp.Schedule, cronApp.Valkey = Cron, 0, 0, "0 3 * * *", true
	for _, app := range []App{validWeb(), worker, cronApp} {
		t.Run(string(app.Type), func(t *testing.T) {
			image := ImageRef(testConfig().Registry, app.Name, "abc")
			got, err := json.MarshalIndent(BuildWorkload(app, testConfig(), image, "2026-10-15T00:00:00Z"), "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join("testdata", string(app.Type)+".golden.json")
			if *update {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("workload differs from %s; rerun with -update and review the diff\n%s", golden, got)
			}
		})
	}
}

func TestBuildWorkloadObjectsPerType(t *testing.T) {
	worker := validWeb()
	worker.Type = Worker
	cronApp := validWeb()
	cronApp.Type, cronApp.Schedule = Cron, "0 3 * * *"
	web := BuildWorkload(validWeb(), testConfig(), "image", "now")
	if web.Deployment == nil || web.Service == nil || web.Ingress == nil || web.CronJob != nil {
		t.Fatalf("web objects = %+v", web)
	}
	if host := web.Ingress.Spec.Rules[0].Host; host != "hello.apps.example.com" {
		t.Fatalf("host = %q", host)
	}
	if got := BuildWorkload(worker, testConfig(), "image", "now"); got.Deployment == nil || got.Service != nil || got.Ingress != nil {
		t.Fatalf("worker objects = %+v", got)
	}
	if got := BuildWorkload(cronApp, testConfig(), "image", "now"); got.CronJob == nil || got.Deployment != nil {
		t.Fatalf("cron objects = %+v", got)
	}
}

func TestValkeySecretOnlyWhenAttached(t *testing.T) {
	app := validWeb()
	envFrom := BuildWorkload(app, testConfig(), "image", "now").Deployment.Spec.Template.Spec.Containers[0].EnvFrom
	if len(envFrom) != 2 {
		t.Fatalf("detached app envFrom = %d sources", len(envFrom))
	}
	app.Valkey = true
	envFrom = BuildWorkload(app, testConfig(), "image", "now").Deployment.Spec.Template.Spec.Containers[0].EnvFrom
	if len(envFrom) != 3 || envFrom[2].SecretRef.Name != ValkeySecret {
		t.Fatalf("attached app envFrom = %+v", envFrom)
	}
}
