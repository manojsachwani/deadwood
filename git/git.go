// Package git wraps the git binary for the operations deadwood needs.
package git

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

// run executes git in dir (the current directory when dir is empty) and
// returns stdout. On failure the error carries git's own stderr message.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := cleanStderr(stderr.String()); msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	return stdout.String(), nil
}

// cleanStderr reduces git's stderr to its last meaningful line, without the
// "fatal:"/"error:" prefix.
func cleanStderr(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "hint:") {
			continue
		}
		for _, prefix := range []string{"fatal: ", "error: "} {
			line = strings.TrimPrefix(line, prefix)
		}
		return line
	}
	return ""
}

// RepoRoot returns the top-level directory of the current worktree.
func RepoRoot() (string, error) {
	out, err := run("", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
