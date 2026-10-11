package git

import (
	"path/filepath"
	"strings"
)

// Worktree represents one entry of `git worktree list`
type Worktree struct {
	Path     string `json:"path"`
	Branch   string `json:"branch,omitempty"` // short branch name, empty when detached or bare
	Commit   string `json:"commit,omitempty"` // short SHA
	Main     bool   `json:"main"`             // the main working tree, which cannot be removed
	Bare     bool   `json:"bare"`
	Detached bool   `json:"detached"`
	Locked   bool   `json:"locked"`
	Prunable bool   `json:"prunable"` // the directory is gone and the entry can be pruned
	Dirty    bool   `json:"dirty"`    // has uncommitted or untracked changes
}

// ListWorktrees returns all worktrees of the repository, main one first
func ListWorktrees() ([]Worktree, error) {
	out, err := run("", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	worktrees := parseWorktrees(out)
	for i := range worktrees {
		wt := &worktrees[i]
		if wt.Bare || wt.Prunable {
			continue
		}
		if status, err := run(wt.Path, "status", "--porcelain"); err == nil {
			wt.Dirty = strings.TrimSpace(status) != ""
		}
	}
	return worktrees, nil
}

func parseWorktrees(out string) []Worktree {
	worktrees := []Worktree{}
	var cur *Worktree
	flush := func() {
		if cur != nil {
			worktrees = append(worktrees, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			flush()
			continue
		}
		key, value, _ := strings.Cut(line, " ")
		if key == "worktree" {
			flush()
			cur = &Worktree{Path: value, Main: len(worktrees) == 0}
			continue
		}
		if cur == nil {
			continue
		}
		switch key {
		case "HEAD":
			// An all-zero id means the branch has no commits yet.
			if strings.Trim(value, "0") == "" {
				break
			}
			cur.Commit = value
			if len(value) > 7 {
				cur.Commit = value[:7]
			}
		case "branch":
			cur.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare":
			cur.Bare = true
		case "detached":
			cur.Detached = true
		case "locked":
			cur.Locked = true
		case "prunable":
			cur.Prunable = true
		}
	}
	flush()
	return worktrees
}

// AddWorktree creates a worktree at path. An existing branch is checked out
// there; otherwise a new branch with that name is created from HEAD.
func AddWorktree(path, branch string) error {
	args := []string{"worktree", "add"}
	if BranchExists(branch) {
		args = append(args, path, branch)
	} else {
		args = append(args, "-b", branch, path)
	}
	_, err := run("", args...)
	return err
}

// RemoveWorktree removes a worktree. force is required when it has
// uncommitted changes or is locked.
func RemoveWorktree(wt Worktree, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		// A locked worktree needs --force twice.
		args = append(args, "--force")
		if wt.Locked {
			args = append(args, "--force")
		}
	}
	_, err := run("", append(args, wt.Path)...)
	return err
}

// PruneWorktrees drops entries whose directories no longer exist
func PruneWorktrees() error {
	_, err := run("", "worktree", "prune")
	return err
}

// SetWorktreeLocked locks or unlocks a worktree
func SetWorktreeLocked(path string, locked bool) error {
	verb := "unlock"
	if locked {
		verb = "lock"
	}
	_, err := run("", "worktree", verb, path)
	return err
}

// DefaultWorktreePath suggests a sibling directory of the main worktree,
// e.g. /src/app + feature/login -> /src/app-feature-login.
func DefaultWorktreePath(mainRoot, branch string) string {
	slug := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(branch)
	return filepath.Join(filepath.Dir(mainRoot), filepath.Base(mainRoot)+"-"+slug)
}
