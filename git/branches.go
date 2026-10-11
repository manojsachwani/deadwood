package git

import (
	"fmt"
	"strings"
)

// Branch represents a local git branch
type Branch struct {
	Name     string
	Current  bool
	Commit   string // short SHA
	Author   string
	Age      string // relative date of the last commit
	Subject  string // subject line of the last commit
	Upstream string // e.g. "origin/main", empty when not tracking
	Remote   string // remote name of the upstream
	Ahead    int
	Behind   int
	Gone     bool   // upstream is configured but no longer exists
	Worktree string // path of the worktree that has this branch checked out
	Merged   bool   // fully merged into HEAD, so deleting it loses no commits
}

const fieldSep = "\x1f"

var branchFormat = strings.Join([]string{
	"%(refname:short)",
	"%(HEAD)",
	"%(objectname:short)",
	"%(authorname)",
	"%(committerdate:relative)",
	"%(upstream:short)",
	"%(upstream:remotename)",
	"%(upstream:track)",
	"%(worktreepath)",
	"%(contents:subject)",
}, "%1f")

// ListBranches returns the local branches
func ListBranches() ([]Branch, error) {
	out, err := run("", "for-each-ref", "--format="+branchFormat, "refs/heads")
	if err != nil {
		return nil, err
	}
	branches := parseBranches(out)

	// Fails on a repository without commits, where nothing is merged anyway.
	merged := map[string]bool{}
	if out, err := run("", "for-each-ref", "--merged", "HEAD", "--format=%(refname:short)", "refs/heads"); err == nil {
		for _, name := range strings.Fields(out) {
			merged[name] = true
		}
	}
	for i := range branches {
		branches[i].Merged = merged[branches[i].Name]
	}
	return branches, nil
}

func parseBranches(out string) []Branch {
	branches := []Branch{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, fieldSep)
		if len(f) < 10 {
			continue
		}
		b := Branch{
			Name:     f[0],
			Current:  f[1] == "*",
			Commit:   f[2],
			Author:   f[3],
			Age:      f[4],
			Upstream: f[5],
			Remote:   f[6],
			Worktree: f[8],
			Subject:  f[9],
		}
		b.Ahead, b.Behind, b.Gone = parseTrack(f[7])
		branches = append(branches, b)
	}
	return branches
}

// parseTrack reads %(upstream:track), e.g. "[ahead 1, behind 2]" or "[gone]".
func parseTrack(s string) (ahead, behind int, gone bool) {
	s = strings.Trim(s, "[]")
	for _, part := range strings.Split(s, ", ") {
		switch {
		case part == "gone":
			gone = true
		case strings.HasPrefix(part, "ahead "):
			fmt.Sscanf(part, "ahead %d", &ahead)
		case strings.HasPrefix(part, "behind "):
			fmt.Sscanf(part, "behind %d", &behind)
		}
	}
	return ahead, behind, gone
}

// DeleteBranch force-deletes a branch locally. When remote is true and the
// branch tracks an upstream that still exists, that upstream branch is
// deleted as well.
func DeleteBranch(b Branch, remote bool) error {
	if _, err := run("", "branch", "-D", b.Name); err != nil {
		return err
	}
	if !remote || b.Upstream == "" || b.Remote == "" || b.Gone {
		return nil
	}
	remoteBranch := strings.TrimPrefix(b.Upstream, b.Remote+"/")
	if _, err := run("", "push", b.Remote, "--delete", remoteBranch); err != nil {
		return fmt.Errorf("deleted locally, remote failed: %w", err)
	}
	return nil
}

// RenameBranch renames a local branch
func RenameBranch(oldName, newName string) error {
	_, err := run("", "branch", "-m", oldName, newName)
	return err
}

// BranchExists reports whether a local branch with this name exists
func BranchExists(name string) bool {
	_, err := run("", "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}
