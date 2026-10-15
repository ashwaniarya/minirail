package apps

import (
	"reflect"
	"strings"
	"testing"
)

func validWeb() App {
	return App{Name: "hello", Repo: "https://github.com/acme/hello", Branch: "main", Type: Web, Port: 8080, Replicas: 1, Root: "."}
}

func TestValidateAcceptsEachType(t *testing.T) {
	worker := validWeb()
	worker.Type, worker.Port = Worker, 0
	cronApp := validWeb()
	cronApp.Type, cronApp.Port, cronApp.Replicas, cronApp.Schedule = Cron, 0, 0, "*/15 * * * *"
	for _, app := range []App{validWeb(), worker, cronApp} {
		if err := app.Validate(); err != nil {
			t.Errorf("%s: %v", app.Type, err)
		}
	}
}

func TestValidateRejectsBadInput(t *testing.T) {
	cases := map[string]func(*App){
		"uppercase name":      func(a *App) { a.Name = "Hello" },
		"leading dash name":   func(a *App) { a.Name = "-hello" },
		"long name":           func(a *App) { a.Name = strings.Repeat("a", 31) },
		"reserved name":       func(a *App) { a.Name = "console" },
		"gitlab repo":         func(a *App) { a.Repo = "https://gitlab.com/acme/hello" },
		"repo with path":      func(a *App) { a.Repo = "https://github.com/acme/hello/tree/main" },
		"branch dotdot":       func(a *App) { a.Branch = "feat/../main" },
		"branch flag":         func(a *App) { a.Branch = "--upload-pack=x" },
		"branch space":        func(a *App) { a.Branch = "my branch" },
		"absolute root":       func(a *App) { a.Root = "/etc" },
		"escaping root":       func(a *App) { a.Root = "apps/../../x" },
		"zero port":           func(a *App) { a.Port = 0 },
		"too many replicas":   func(a *App) { a.Replicas = 6 },
		"unknown type":        func(a *App) { a.Type = "db" },
		"six field schedule":  func(a *App) { a.Type, a.Schedule = Cron, "0 */15 * * * *" },
		"descriptor schedule": func(a *App) { a.Type, a.Schedule = Cron, "@hourly" },
		"out of range minute": func(a *App) { a.Type, a.Schedule = Cron, "61 * * * *" },
	}
	for label, mutate := range cases {
		app := validWeb()
		mutate(&app)
		if app.Validate() == nil {
			t.Errorf("%s: expected error", label)
		}
	}
}

func TestNormalizeRepo(t *testing.T) {
	for input, want := range map[string]string{
		"https://github.com/Acme/Hello.git": "https://github.com/acme/hello",
		" https://github.com/acme/hello/ ":  "https://github.com/acme/hello",
		"https://github.com/acme/hello":     "https://github.com/acme/hello",
	} {
		if got := NormalizeRepo(input); got != want {
			t.Errorf("NormalizeRepo(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestValidSHA(t *testing.T) {
	if !ValidSHA(strings.Repeat("a1", 20)) {
		t.Error("40 hex chars should be valid")
	}
	for _, bad := range []string{"", "abc", strings.Repeat("A", 40), strings.Repeat("g", 40)} {
		if ValidSHA(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestParseEnv(t *testing.T) {
	got, err := ParseEnv("# comment\r\nA=1\r\n\nB = two=2\nEMPTY=\n")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "1", "B": " two=2", "EMPTY": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for _, bad := range []string{"NOEQUALS", "1A=x", "A-B=x", "=x"} {
		if _, err := ParseEnv(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestFormatEnvRoundTrips(t *testing.T) {
	env := map[string]string{"B": "2", "A": "x=y"}
	text := FormatEnv(env)
	if text != "A=x=y\nB=2\n" {
		t.Fatalf("FormatEnv = %q", text)
	}
	parsed, err := ParseEnv(text)
	if err != nil || !reflect.DeepEqual(parsed, env) {
		t.Fatalf("round trip = %#v, %v", parsed, err)
	}
}

func TestConfigMapRoundTrip(t *testing.T) {
	app := validWeb()
	app.Postgres, app.DeployedSHA = true, strings.Repeat("b", 40)
	configMap := app.ToConfigMap("apps")
	if configMap.Name != "hello-app" || configMap.Labels[LabelApp] != "hello" {
		t.Fatalf("metadata = %+v", configMap.ObjectMeta)
	}
	got, err := FromConfigMap(configMap)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, app) {
		t.Fatalf("got %+v, want %+v", got, app)
	}
}

func TestFromConfigMapRejectsCorruptNumbers(t *testing.T) {
	configMap := validWeb().ToConfigMap("apps")
	configMap.Data["port"] = "eighty"
	if _, err := FromConfigMap(configMap); err == nil {
		t.Fatal("expected error")
	}
}
