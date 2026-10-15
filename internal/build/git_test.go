package build

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestParseLsRemote(t *testing.T) {
	output := shaA + "\trefs/heads/main\n" + strings.Repeat("b", 40) + "\trefs/heads/main-old\n"
	if sha, err := parseLsRemote(output, "main"); err != nil || sha != shaA {
		t.Fatalf("main = %q, %v", sha, err)
	}
	for _, bad := range []string{"", "nonsense\trefs/heads/main\n", shaA + "\trefs/heads/other\n"} {
		if _, err := parseLsRemote(bad, "main"); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestLsRemoteReadsBranchHeadFromRealRepo(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main", repo},
		{"-C", repo, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "--allow-empty", "-m", "first"},
	} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, output)
		}
	}
	head, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	sha, err := LsRemote("unused")(context.Background(), repo, "main")
	if err != nil || sha != strings.TrimSpace(string(head)) {
		t.Fatalf("LsRemote = %q, %v", sha, err)
	}
	if _, err := LsRemote("unused")(context.Background(), repo, "missing"); err == nil {
		t.Fatal("missing branch must error")
	}
}
