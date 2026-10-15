package build

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"minirail/internal/apps"
)

const credentialHelper = `credential.helper=!f() { echo username=x-access-token; echo "password=$GITHUB_TOKEN"; }; f`

func LsRemote(token string) func(ctx context.Context, repo, branch string) (string, error) {
	return func(ctx context.Context, repo, branch string) (string, error) {
		command := exec.CommandContext(ctx, "git", "-c", credentialHelper, "ls-remote", repo, "refs/heads/"+branch)
		command.Env = append(os.Environ(), "GITHUB_TOKEN="+token, "GIT_TERMINAL_PROMPT=0", "HOME=/tmp")
		output, err := command.Output()
		if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
			return "", fmt.Errorf("git ls-remote %s: %s", repo, strings.TrimSpace(string(exitErr.Stderr)))
		}
		if err != nil {
			return "", fmt.Errorf("git ls-remote %s: %w", repo, err)
		}
		return parseLsRemote(string(output), branch)
	}
}

func parseLsRemote(output, branch string) (string, error) {
	for line := range strings.Lines(output) {
		sha, ref, found := strings.Cut(strings.TrimSpace(line), "\t")
		if found && ref == "refs/heads/"+branch && apps.ValidSHA(sha) {
			return sha, nil
		}
	}
	return "", fmt.Errorf("branch %q not found in repo", branch)
}
