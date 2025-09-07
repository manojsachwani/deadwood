package git

import (
	"os/exec"
	"strings"
)

// Branch represents a git branch
type Branch struct {
	Name    string
	Current bool
	Commit  string // short SHA
	Author  string
}

// ListBranches returns a list of local branches
func ListBranches() ([]Branch, error) {
       cmd := exec.Command("git", "branch")
       out, err := cmd.Output()
       if err != nil {
	       return nil, err
       }
       lines := strings.Split(string(out), "\n")
       branches := []Branch{}
       for _, line := range lines {
	       line = strings.TrimSpace(line)
	       if line == "" {
		       continue
	       }
	       current := false
	       if strings.HasPrefix(line, "*") {
		       current = true
		       line = strings.TrimSpace(line[1:])
	       }
	       // Get last commit short SHA and author for this branch
	       sha := ""
	       author := ""
	       if line != "" {
		       shaCmd := exec.Command("git", "rev-parse", "--short", line)
		       shaOut, shaErr := shaCmd.Output()
		       if shaErr == nil {
			       sha = strings.TrimSpace(string(shaOut))
		       }
		       authorCmd := exec.Command("git", "log", "-1", "--pretty=format:%an", line)
		       authorOut, authorErr := authorCmd.Output()
		       if authorErr == nil {
			       author = strings.TrimSpace(string(authorOut))
		       }
	       }
	       branches = append(branches, Branch{Name: line, Current: current, Commit: sha, Author: author})
       }
       return branches, nil
}

// DeleteBranch deletes a branch locally and optionally remotely
func DeleteBranch(branch string, remote bool) error {
	cmd := exec.Command("git", "branch", "-D", branch)
	if err := cmd.Run(); err != nil {
		return err
	}
	if remote {
		cmd = exec.Command("git", "push", "origin", "--delete", branch)
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

// RenameBranch renames a branch
func RenameBranch(oldName, newName string) error {
	cmd := exec.Command("git", "branch", "-m", oldName, newName)
	return cmd.Run()
}
