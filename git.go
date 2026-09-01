package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func getGitCommitHash(workDir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	if workDir != "" {
		cmd.Dir = workDir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git commit hash: %w (not a git repository?)", err)
	}
	return strings.TrimSpace(string(out)), nil
}
