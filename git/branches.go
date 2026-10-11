package git

import (
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MergeKind says how a branch ended up in the base branch.
type MergeKind string

const (
	NotMerged MergeKind = ""
	Merged    MergeKind = "merged"   // ancestor of the base
	Squashed  MergeKind = "squashed" // its combined diff is already in the base
	Rebased   MergeKind = "rebased"  // every commit's patch is already in the base
)

// Branch represents a local git branch
type Branch struct {
	Name      string    `json:"name"`
	Current   bool      `json:"current"`
	SHA       string    `json:"sha"`
	Commit    string    `json:"-"` // short SHA
	Author    string    `json:"author"`
	Age       string    `json:"-"` // relative date of the last commit
	Updated   time.Time `json:"updated"`
	Subject   string    `json:"subject"`
	Upstream  string    `json:"upstream,omitempty"` // e.g. "origin/main"
	Remote    string    `json:"-"`                  // remote name of the upstream
	Ahead     int       `json:"ahead"`
	Behind    int       `json:"behind"`
	Gone      bool      `json:"gone"`               // upstream is configured but no longer exists
	Worktree  string    `json:"worktree,omitempty"` // worktree that has this branch checked out
	Merge     MergeKind `json:"merged,omitempty"`
	Stale     bool      `json:"stale"`
	Protected bool      `json:"protected"`
}

// Options control how branches are classified
type Options struct {
	Base      string   // ref that branches are compared against
	StaleDays int      // branches without commits for this long are stale; 0 disables
	Protect   []string // glob patterns of branch names that are never deleted

	// PullRequests also asks the GitHub CLI which branches were merged
	// through a pull request. It needs gh and the network, so it is opt-in.
	PullRequests bool
}

const DefaultStaleDays = 90

// alwaysProtected are never deleted, whatever the configuration says.
var alwaysProtected = []string{"main", "master", "develop"}

// LoadOptions combines command-line overrides with `git config deadwood.*`.
// An empty base or a negative staleDays means "not given".
func LoadOptions(base string, staleDays int) (Options, error) {
	opts := Options{StaleDays: DefaultStaleDays}
	if v := config("deadwood.staleDays"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			opts.StaleDays = n
		}
	}
	if staleDays >= 0 {
		opts.StaleDays = staleDays
	}
	opts.PullRequests = config("deadwood.pullRequests") == "true"
	if out, err := run("", "config", "--get-all", "deadwood.protect"); err == nil {
		opts.Protect = strings.Fields(out)
	}
	if base == "" {
		base = config("deadwood.base")
	}
	if base != "" {
		if _, err := run("", "rev-parse", "--verify", "--quiet", base+"^{commit}"); err != nil {
			return opts, fmt.Errorf("base %q is not a valid ref", base)
		}
		opts.Base = base
		return opts, nil
	}
	opts.Base = detectBase()
	return opts, nil
}

func config(key string) string {
	out, _ := run("", "config", "--get", key)
	return strings.TrimSpace(out)
}

// detectBase picks the remote's default branch, falling back to the usual
// names. It returns "" when the repository has none of them.
func detectBase() string {
	if out, err := run("", "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if ref := strings.TrimSpace(out); ref != "" {
			return ref
		}
	}
	for _, ref := range []string{"origin/main", "origin/master", "main", "master"} {
		if _, err := run("", "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err == nil {
			return ref
		}
	}
	return ""
}

func (o Options) protects(name string) bool {
	if name == o.Base || strings.HasSuffix(o.Base, "/"+name) {
		return true
	}
	for _, p := range alwaysProtected {
		if name == p {
			return true
		}
	}
	for _, pattern := range o.Protect {
		if ok, _ := path.Match(pattern, name); ok {
			return true
		}
	}
	return false
}

const fieldSep = "\x1f"

var branchFormat = strings.Join([]string{
	"%(refname:short)",
	"%(HEAD)",
	"%(objectname)",
	"%(objectname:short)",
	"%(authorname)",
	"%(committerdate:relative)",
	"%(committerdate:unix)",
	"%(upstream:short)",
	"%(upstream:remotename)",
	"%(upstream:track)",
	"%(worktreepath)",
	"%(contents:subject)",
}, "%1f")

// ListBranches returns the local branches, classified against opts
func ListBranches(opts Options) ([]Branch, error) {
	out, err := run("", "for-each-ref", "--format="+branchFormat, "refs/heads")
	if err != nil {
		return nil, err
	}
	branches := parseBranches(out)

	// Without a base (a repository with no commits, say) nothing is merged.
	merged := map[string]bool{}
	if opts.Base != "" {
		if out, err := run("", "for-each-ref", "--merged", opts.Base, "--format=%(refname:short)", "refs/heads"); err == nil {
			for _, name := range strings.Fields(out) {
				merged[name] = true
			}
		}
	}
	now := time.Now()
	for i := range branches {
		b := &branches[i]
		b.Protected = opts.protects(b.Name)
		if merged[b.Name] {
			b.Merge = Merged
		}
		if opts.StaleDays > 0 && !b.Updated.IsZero() {
			b.Stale = now.Sub(b.Updated) > time.Duration(opts.StaleDays)*24*time.Hour
		}
	}
	if opts.Base != "" {
		detectRewrittenMerges(opts.Base, branches)
	}
	if opts.PullRequests {
		applyPullRequests(branches)
	}
	return branches, nil
}

func parseBranches(out string) []Branch {
	branches := []Branch{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, fieldSep)
		if len(f) < 12 {
			continue
		}
		b := Branch{
			Name:     f[0],
			Current:  f[1] == "*",
			SHA:      f[2],
			Commit:   f[3],
			Author:   f[4],
			Age:      f[5],
			Upstream: f[7],
			Remote:   f[8],
			Worktree: f[10],
			Subject:  f[11],
		}
		if unix, err := strconv.ParseInt(f[6], 10, 64); err == nil {
			b.Updated = time.Unix(unix, 0)
		}
		b.Ahead, b.Behind, b.Gone = parseTrack(f[9])
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

// detectRewrittenMerges finds branches that landed in base through a squash
// or rebase merge, which leave no ancestry behind for --merged to see.
func detectRewrittenMerges(base string, branches []Branch) {
	var wg sync.WaitGroup
	slots := make(chan struct{}, 8)
	for i := range branches {
		b := &branches[i]
		if b.Merge != NotMerged || b.Protected {
			continue
		}
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			b.Merge = rewrittenMerge(base, b.Name)
		}()
	}
	wg.Wait()
}

func rewrittenMerge(base, branch string) MergeKind {
	// Rebase merge: every commit of the branch has a patch-equivalent in base.
	if out, err := run("", "cherry", base, branch); err == nil {
		lines := strings.Fields(strings.ReplaceAll(out, " ", ""))
		rebased := len(lines) > 0
		for _, line := range lines {
			if !strings.HasPrefix(line, "-") {
				rebased = false
			}
		}
		if rebased {
			return Rebased
		}
	}

	// Squash merge: collapse the branch into one throwaway commit on top of
	// the merge base and ask whether base already contains that patch.
	mergeBase, err := run("", "merge-base", base, branch)
	if err != nil {
		return NotMerged
	}
	squash, err := run("",
		"-c", "user.name=deadwood", "-c", "user.email=deadwood@localhost",
		"commit-tree", branch+"^{tree}", "-p", strings.TrimSpace(mergeBase), "-m", "squash")
	if err != nil {
		return NotMerged
	}
	if out, err := run("", "cherry", base, strings.TrimSpace(squash)); err == nil && strings.HasPrefix(out, "-") {
		return Squashed
	}
	return NotMerged
}

// ErrMoved is returned when a branch changed after it was listed.
var ErrMoved = errors.New("branch moved since it was listed; refresh and try again")

// DeleteBranch force-deletes a branch locally, provided it still points at
// the commit it was listed with. When remote is true and the branch tracks
// an upstream that still exists, that upstream branch is deleted too, unless
// it has moved since the last fetch. remoteDeleted reports whether it was.
func DeleteBranch(b Branch, remote bool) (remoteDeleted bool, err error) {
	if b.SHA != "" {
		out, err := run("", "rev-parse", "--verify", "--quiet", "refs/heads/"+b.Name)
		if err != nil {
			return false, err
		}
		if strings.TrimSpace(out) != b.SHA {
			return false, ErrMoved
		}
	}
	if _, err := run("", "branch", "-D", b.Name); err != nil {
		return false, err
	}
	if !remote || b.Upstream == "" || b.Remote == "" || b.Gone {
		return false, nil
	}
	remoteBranch := strings.TrimPrefix(b.Upstream, b.Remote+"/")
	args := []string{"push"}
	if out, err := run("", "rev-parse", "--verify", "--quiet", "refs/remotes/"+b.Upstream); err == nil {
		args = append(args, "--force-with-lease="+remoteBranch+":"+strings.TrimSpace(out))
	}
	args = append(args, b.Remote, ":refs/heads/"+remoteBranch)
	if _, err := run("", args...); err != nil {
		return false, fmt.Errorf("deleted locally, remote failed: %w", err)
	}
	return true, nil
}

// RestoreBranch recreates a deleted branch at the commit it pointed to and
// re-attaches its upstream when that still exists.
func RestoreBranch(b Branch) error {
	if _, err := run("", "branch", b.Name, b.SHA); err != nil {
		return err
	}
	if b.Upstream != "" {
		if _, err := run("", "rev-parse", "--verify", "--quiet", "refs/remotes/"+b.Upstream); err == nil {
			run("", "branch", "--quiet", "--set-upstream-to="+b.Upstream, b.Name)
		}
	}
	return nil
}

// RenameBranch renames a local branch
func RenameBranch(oldName, newName string) error {
	_, err := run("", "branch", "-m", oldName, newName)
	return err
}

// SwitchBranch checks a branch out in the current worktree
func SwitchBranch(name string) error {
	_, err := run("", "switch", "--quiet", name)
	return err
}

// Fetch updates all remotes and drops remote-tracking refs that are gone
func Fetch() error {
	_, err := run("", "fetch", "--all", "--prune", "--quiet")
	return err
}

// BranchExists reports whether a local branch with this name exists
func BranchExists(name string) bool {
	_, err := run("", "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// Detail is what a branch would add to the base
type Detail struct {
	Commits []string // one line each, newest first, capped
	Total   int      // number of commits not in base
	Stat    string   // diff --shortstat against the merge base
}

const detailCommits = 30

// BranchDetail lists the commits and changes of a branch that are not in base
func BranchDetail(base, name string) (Detail, error) {
	var d Detail
	if base == "" {
		base = "HEAD"
	}
	out, err := run("", "log", "--format=%h %s (%cr)", "-n", strconv.Itoa(detailCommits), base+".."+name)
	if err != nil {
		return d, err
	}
	for _, line := range strings.Split(out, "\n") {
		if line != "" {
			d.Commits = append(d.Commits, line)
		}
	}
	if out, err := run("", "rev-list", "--count", base+".."+name); err == nil {
		d.Total, _ = strconv.Atoi(strings.TrimSpace(out))
	}
	if out, err := run("", "diff", "--shortstat", base+"..."+name); err == nil {
		d.Stat = strings.TrimSpace(out)
	}
	return d, nil
}

// CreateBranch creates a branch at start without checking it out
func CreateBranch(name, start string) error {
	_, err := run("", "branch", name, start)
	return err
}

// defaultRemote is where a branch without an upstream gets pushed: origin
// when it exists, otherwise the first remote.
func defaultRemote() (string, error) {
	out, err := run("", "remote")
	if err != nil {
		return "", err
	}
	remotes := strings.Fields(out)
	if len(remotes) == 0 {
		return "", errors.New("this repository has no remote")
	}
	for _, r := range remotes {
		if r == "origin" {
			return r, nil
		}
	}
	return remotes[0], nil
}

// PushBranch pushes a branch to its upstream, or publishes it on the default
// remote and starts tracking it when it has none. It returns the remote ref.
func PushBranch(b Branch) (string, error) {
	if b.Upstream != "" && b.Remote != "" {
		remoteBranch := strings.TrimPrefix(b.Upstream, b.Remote+"/")
		_, err := run("", "push", "--quiet", b.Remote, b.Name+":refs/heads/"+remoteBranch)
		return b.Upstream, err
	}
	remote, err := defaultRemote()
	if err != nil {
		return "", err
	}
	_, err = run("", "push", "--quiet", "--set-upstream", remote, b.Name)
	return remote + "/" + b.Name, err
}

// PullBranch fast-forwards a branch from its upstream. The current branch is
// pulled in place; any other branch is updated without checking it out.
func PullBranch(b Branch) error {
	if b.Upstream == "" || b.Remote == "" {
		return errors.New("branch has no upstream")
	}
	if b.Gone {
		return errors.New("upstream branch is gone")
	}
	if b.Current {
		_, err := run("", "pull", "--quiet", "--ff-only")
		return err
	}
	remoteBranch := strings.TrimPrefix(b.Upstream, b.Remote+"/")
	_, err := run("", "fetch", "--quiet", b.Remote, remoteBranch+":"+b.Name)
	return err
}

// RenameBranchRemote renames a branch locally and on its remote: the new
// name is pushed and tracked, then the old remote branch is deleted unless
// it moved since the last fetch.
func RenameBranchRemote(b Branch, newName string) error {
	if b.Upstream == "" || b.Remote == "" || b.Gone {
		return errors.New("branch has no upstream to rename")
	}
	oldRemoteBranch := strings.TrimPrefix(b.Upstream, b.Remote+"/")
	lease := ""
	if out, err := run("", "rev-parse", "--verify", "--quiet", "refs/remotes/"+b.Upstream); err == nil {
		lease = "--force-with-lease=" + oldRemoteBranch + ":" + strings.TrimSpace(out)
	}
	if err := RenameBranch(b.Name, newName); err != nil {
		return err
	}
	if _, err := run("", "push", "--quiet", "--set-upstream", b.Remote, newName+":refs/heads/"+newName); err != nil {
		return fmt.Errorf("renamed locally, push failed: %w", err)
	}
	args := []string{"push", "--quiet"}
	if lease != "" {
		args = append(args, lease)
	}
	if _, err := run("", append(args, b.Remote, ":refs/heads/"+oldRemoteBranch)...); err != nil {
		return fmt.Errorf("renamed and pushed, but the old remote branch remains: %w", err)
	}
	return nil
}

// ArchivePrefix is the tag namespace archived branches are kept under
const ArchivePrefix = "archive/"

// ArchiveBranch tags the branch tip as archive/<name> and deletes the local
// branch, so its commits stay reachable.
func ArchiveBranch(b Branch) error {
	tag := ArchivePrefix + b.Name
	if _, err := run("", "tag", tag, b.SHA); err != nil {
		return err
	}
	if _, err := DeleteBranch(b, false); err != nil {
		run("", "tag", "-d", tag)
		return err
	}
	return nil
}

// Unarchive removes the archive tag of a branch, provided it still points at
// the commit the branch was archived at.
func Unarchive(b Branch) {
	tag := ArchivePrefix + b.Name
	if out, err := run("", "rev-parse", "--verify", "--quiet", "refs/tags/"+tag+"^{commit}"); err == nil && strings.TrimSpace(out) == b.SHA {
		run("", "tag", "-d", tag)
	}
}

// View settings remembered per repository in git config
const (
	configSort   = "deadwood.sort"
	configFilter = "deadwood.filter"
)

// LoadView returns the sort order and filter saved by SaveView
func LoadView() (sort, filter string) {
	return config(configSort), config(configFilter)
}

// SaveView remembers the sort order and filter for the next run
func SaveView(sort, filter string) {
	for key, value := range map[string]string{configSort: sort, configFilter: filter} {
		if value == "" {
			run("", "config", "--local", "--unset-all", key)
		} else {
			run("", "config", "--local", key, value)
		}
	}
}

// ConfigMap returns every `git config <prefix>.<name>` as name -> value
func ConfigMap(prefix string) map[string]string {
	values := map[string]string{}
	out, err := run("", "config", "--get-regexp", "^"+strings.ReplaceAll(prefix, ".", `\.`)+`\.`)
	if err != nil {
		return values
	}
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		values[strings.ToLower(key[len(prefix)+1:])] = strings.TrimSpace(value)
	}
	return values
}

// Config returns one git config value, or "" when it is unset
func Config(key string) string { return config(key) }
