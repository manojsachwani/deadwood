package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// initRepo creates a repository with one commit on main and chdirs into it.
func initRepo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(dir, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })

	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	mustGit(t, "init", "-q", "-b", "main")
	mustGit(t, "config", "user.name", "Tester")
	mustGit(t, "config", "user.email", "tester@example.com")
	mustGit(t, "commit", "-q", "--allow-empty", "-m", "initial commit")
	return repo
}

func mustGit(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestEmptyRepository(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
	mustGit(t, "init", "-q", "-b", "main")

	branches, err := ListBranches(Options{Base: "main"})
	if err != nil || len(branches) != 0 {
		t.Errorf("branches = %+v, err = %v", branches, err)
	}
	worktrees, err := ListWorktrees()
	if err != nil || len(worktrees) != 1 || worktrees[0].Commit != "" {
		t.Errorf("worktrees = %+v, err = %v", worktrees, err)
	}
}

func TestParseTrack(t *testing.T) {
	tests := []struct {
		in            string
		ahead, behind int
		gone          bool
	}{
		{"", 0, 0, false},
		{"[ahead 3]", 3, 0, false},
		{"[behind 12]", 0, 12, false},
		{"[ahead 1, behind 2]", 1, 2, false},
		{"[gone]", 0, 0, true},
	}
	for _, tt := range tests {
		ahead, behind, gone := parseTrack(tt.in)
		if ahead != tt.ahead || behind != tt.behind || gone != tt.gone {
			t.Errorf("parseTrack(%q) = %d, %d, %v", tt.in, ahead, behind, gone)
		}
	}
}

func TestParseWorktrees(t *testing.T) {
	out := "worktree /src/app\nHEAD 1234567890abcdef\nbranch refs/heads/main\n\n" +
		"worktree /src/app-feat\nHEAD abcdef1234567890\nbranch refs/heads/feat/x\nlocked reason\n\n" +
		"worktree /src/gone\nHEAD abcdef1234567890\ndetached\nprunable gitdir file points to non-existent location\n"
	got := parseWorktrees(out)
	if len(got) != 3 {
		t.Fatalf("got %d worktrees, want 3", len(got))
	}
	if !got[0].Main || got[0].Branch != "main" || got[0].Commit != "1234567" {
		t.Errorf("main worktree parsed as %+v", got[0])
	}
	if got[1].Main || got[1].Branch != "feat/x" || !got[1].Locked {
		t.Errorf("locked worktree parsed as %+v", got[1])
	}
	if !got[2].Detached || !got[2].Prunable || got[2].Branch != "" {
		t.Errorf("prunable worktree parsed as %+v", got[2])
	}
}

func TestDefaultWorktreePath(t *testing.T) {
	got := DefaultWorktreePath(filepath.Join("/src", "app"), "feature/login")
	if want := filepath.Join("/src", "app-feature-login"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBranchLifecycle(t *testing.T) {
	initRepo(t)
	mustGit(t, "branch", "feature/a")
	mustGit(t, "branch", "old")

	branches, err := ListBranches(Options{Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Branch{}
	for _, b := range branches {
		byName[b.Name] = b
	}
	if len(branches) != 3 {
		t.Fatalf("got %d branches, want 3: %+v", len(branches), branches)
	}
	main := byName["main"]
	if !main.Current || main.Commit == "" || main.Author != "Tester" || main.Subject != "initial commit" || main.Age == "" {
		t.Errorf("main parsed as %+v", main)
	}
	if byName["old"].Current || byName["old"].Upstream != "" {
		t.Errorf("old parsed as %+v", byName["old"])
	}

	if byName["old"].Merge != Merged || !byName["main"].Protected || byName["old"].Protected {
		t.Errorf("old points at HEAD and should be merged: %+v", byName["old"])
	}
	mustGit(t, "checkout", "-q", "feature/a")
	mustGit(t, "commit", "-q", "--allow-empty", "-m", "ahead")
	mustGit(t, "checkout", "-q", "main")
	branches, _ = ListBranches(Options{Base: "main"})
	for _, b := range branches {
		if b.Name == "feature/a" && b.Merge != NotMerged {
			t.Error("feature/a has an extra commit and should not be merged")
		}
	}

	if err := RenameBranch("old", "renamed"); err != nil {
		t.Fatal(err)
	}
	if BranchExists("old") || !BranchExists("renamed") {
		t.Error("rename did not take effect")
	}
	if _, err := DeleteBranch(Branch{Name: "renamed"}, true); err != nil {
		t.Fatal(err)
	}
	if BranchExists("renamed") {
		t.Error("branch still exists after delete")
	}

	_, err = DeleteBranch(Branch{Name: "main"}, false)
	if err == nil {
		t.Fatal("deleting the current branch should fail")
	}
	if err.Error() == "exit status 1" {
		t.Errorf("error should carry git's message, got %q", err)
	}
}

func TestDeleteBranchRemote(t *testing.T) {
	repo := initRepo(t)
	remote := filepath.Join(filepath.Dir(repo), "remote.git")
	mustGit(t, "init", "-q", "--bare", remote)
	mustGit(t, "remote", "add", "upstream", remote)
	mustGit(t, "branch", "feat")
	mustGit(t, "push", "-q", "-u", "upstream", "feat:feat-remote")

	branches, err := ListBranches(Options{Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	var feat Branch
	for _, b := range branches {
		if b.Name == "feat" {
			feat = b
		}
	}
	if feat.Upstream != "upstream/feat-remote" || feat.Remote != "upstream" {
		t.Fatalf("feat parsed as %+v", feat)
	}
	// Someone else pushes to the remote branch: the lease must refuse.
	mustGit(t, "branch", "other", "feat")
	other := filepath.Join(filepath.Dir(repo), "other")
	mustGit(t, "clone", "-q", remote, other, "-b", "feat-remote")
	mustGit(t, "-C", other, "-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "newer")
	mustGit(t, "-C", other, "push", "-q", "origin", "feat-remote")
	if deleted, err := DeleteBranch(feat, true); err == nil || deleted {
		t.Fatalf("stale lease should refuse the remote delete, got deleted=%v err=%v", deleted, err)
	}
	if out, _ := exec.Command("git", "ls-remote", "--heads", remote).Output(); len(out) == 0 {
		t.Fatal("remote branch was deleted despite having moved")
	}

	// After a fetch the lease is current again.
	if err := RestoreBranch(feat); err != nil {
		t.Fatal(err)
	}
	if err := Fetch(); err != nil {
		t.Fatal(err)
	}
	branches, _ = ListBranches(Options{Base: "main"})
	for _, b := range branches {
		if b.Name == "feat" {
			feat = b
		}
	}
	if feat.Upstream != "upstream/feat-remote" {
		t.Fatalf("restore should re-attach the upstream: %+v", feat)
	}
	deleted, err := DeleteBranch(feat, true)
	if err != nil || !deleted {
		t.Fatalf("deleted=%v err=%v", deleted, err)
	}
	if out, _ := exec.Command("git", "ls-remote", "--heads", remote, "feat-remote").Output(); len(out) != 0 {
		t.Errorf("remote branch still exists: %s", out)
	}
}

func commitFile(t *testing.T, name, content, msg string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, "add", name)
	mustGit(t, "commit", "-q", "-m", msg)
}

func TestSquashAndRebaseDetection(t *testing.T) {
	initRepo(t)
	commitFile(t, "base.txt", "base\n", "base file")

	mustGit(t, "checkout", "-q", "-b", "squashed")
	commitFile(t, "a.txt", "one\n", "a1")
	commitFile(t, "a.txt", "one\ntwo\n", "a2")
	mustGit(t, "checkout", "-q", "main")

	mustGit(t, "checkout", "-q", "-b", "rebased")
	commitFile(t, "b.txt", "one\n", "b1")
	commitFile(t, "c.txt", "one\n", "b2")
	mustGit(t, "checkout", "-q", "main")

	mustGit(t, "checkout", "-q", "-b", "unmerged")
	commitFile(t, "d.txt", "one\n", "d1")
	mustGit(t, "checkout", "-q", "main")

	// Main moves on, then lands the two branches the way GitHub would.
	commitFile(t, "main.txt", "x\n", "main moves on")
	mustGit(t, "merge", "-q", "--squash", "squashed")
	mustGit(t, "commit", "-q", "-m", "squashed (#1)")
	mustGit(t, "cherry-pick", "rebased~1", "rebased")

	branches, err := ListBranches(Options{Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]MergeKind{"squashed": Squashed, "rebased": Rebased, "unmerged": NotMerged, "main": Merged}
	for _, b := range branches {
		if b.Merge != want[b.Name] {
			t.Errorf("%s: merge = %q, want %q", b.Name, b.Merge, want[b.Name])
		}
	}
}

func TestDeleteRefusesMovedBranch(t *testing.T) {
	initRepo(t)
	mustGit(t, "branch", "topic")
	branches, _ := ListBranches(Options{Base: "main"})
	var topic Branch
	for _, b := range branches {
		if b.Name == "topic" {
			topic = b
		}
	}
	mustGit(t, "checkout", "-q", "topic")
	mustGit(t, "commit", "-q", "--allow-empty", "-m", "new work")
	mustGit(t, "checkout", "-q", "main")
	if _, err := DeleteBranch(topic, false); err != ErrMoved {
		t.Fatalf("err = %v, want ErrMoved", err)
	}
	if !BranchExists("topic") {
		t.Error("moved branch must survive")
	}
}

func TestOptions(t *testing.T) {
	initRepo(t)
	mustGit(t, "config", "--add", "deadwood.protect", "release/*")
	mustGit(t, "config", "deadwood.staleDays", "30")
	mustGit(t, "branch", "release/1.0")
	mustGit(t, "branch", "topic")

	opts, err := LoadOptions("", -1)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Base != "main" || opts.StaleDays != 30 {
		t.Errorf("opts = %+v", opts)
	}
	if !opts.protects("release/1.0") || !opts.protects("main") || opts.protects("topic") {
		t.Error("protection patterns not applied")
	}
	if opts, _ := LoadOptions("", 7); opts.StaleDays != 7 {
		t.Errorf("flag should override config, got %d", opts.StaleDays)
	}
	if _, err := LoadOptions("nope", -1); err == nil {
		t.Error("an unknown base should be rejected")
	}

	branches, _ := ListBranches(opts)
	for _, b := range branches {
		if b.Stale {
			t.Errorf("%s was committed just now and is not stale", b.Name)
		}
	}
	t.Setenv("GIT_COMMITTER_DATE", time.Now().Add(-40*24*time.Hour).Format(time.RFC3339))
	mustGit(t, "checkout", "-q", "topic")
	mustGit(t, "commit", "-q", "--allow-empty", "-m", "old work")
	branches, _ = ListBranches(opts)
	for _, b := range branches {
		if b.Name == "topic" && !b.Stale {
			t.Error("topic was last committed 40 days ago and should be stale at 30")
		}
	}
}

func TestBranchDetail(t *testing.T) {
	initRepo(t)
	mustGit(t, "checkout", "-q", "-b", "topic")
	commitFile(t, "a.txt", "one\n", "first")
	commitFile(t, "a.txt", "one\ntwo\n", "second")
	mustGit(t, "checkout", "-q", "main")
	d, err := BranchDetail("main", "topic")
	if err != nil {
		t.Fatal(err)
	}
	if d.Total != 2 || len(d.Commits) != 2 || !strings.Contains(d.Commits[0], "second") || !strings.Contains(d.Stat, "1 file changed") {
		t.Errorf("detail = %+v", d)
	}
}

func TestWorktreeLifecycle(t *testing.T) {
	repo := initRepo(t)
	mustGit(t, "branch", "existing")
	wtExisting := filepath.Join(filepath.Dir(repo), "wt-existing")
	wtNew := filepath.Join(filepath.Dir(repo), "wt-new")

	if err := AddWorktree(wtExisting, "existing"); err != nil {
		t.Fatal(err)
	}
	if err := AddWorktree(wtNew, "brand-new"); err != nil {
		t.Fatal(err)
	}
	if !BranchExists("brand-new") {
		t.Error("AddWorktree should create a missing branch")
	}
	if err := os.WriteFile(filepath.Join(wtNew, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	worktrees, err := ListWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	if len(worktrees) != 3 {
		t.Fatalf("got %d worktrees, want 3: %+v", len(worktrees), worktrees)
	}
	if !worktrees[0].Main || worktrees[0].Path != repo {
		t.Errorf("main worktree parsed as %+v", worktrees[0])
	}
	byPath := map[string]Worktree{}
	for _, wt := range worktrees {
		byPath[wt.Path] = wt
	}
	if byPath[wtExisting].Branch != "existing" || byPath[wtExisting].Dirty {
		t.Errorf("existing worktree parsed as %+v", byPath[wtExisting])
	}
	if !byPath[wtNew].Dirty {
		t.Errorf("worktree with untracked file should be dirty: %+v", byPath[wtNew])
	}

	branches, err := ListBranches(Options{Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range branches {
		if b.Name == "existing" && b.Worktree != wtExisting {
			t.Errorf("branch worktree = %q, want %q", b.Worktree, wtExisting)
		}
	}

	if err := RemoveWorktree(byPath[wtNew], false); err == nil {
		t.Error("removing a dirty worktree without force should fail")
	}
	if err := RemoveWorktree(byPath[wtNew], true); err != nil {
		t.Fatal(err)
	}

	if err := SetWorktreeLocked(wtExisting, true); err != nil {
		t.Fatal(err)
	}
	worktrees, _ = ListWorktrees()
	locked := worktrees[len(worktrees)-1]
	if !locked.Locked {
		t.Fatalf("worktree should be locked: %+v", locked)
	}
	if err := RemoveWorktree(locked, true); err != nil {
		t.Fatal(err)
	}

	stale := filepath.Join(filepath.Dir(repo), "wt-stale")
	if err := AddWorktree(stale, "stale"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(stale); err != nil {
		t.Fatal(err)
	}
	worktrees, _ = ListWorktrees()
	if len(worktrees) != 2 || !worktrees[1].Prunable {
		t.Fatalf("stale worktree should be prunable: %+v", worktrees)
	}
	if err := PruneWorktrees(); err != nil {
		t.Fatal(err)
	}
	if worktrees, _ = ListWorktrees(); len(worktrees) != 1 {
		t.Errorf("got %d worktrees after prune, want 1", len(worktrees))
	}
}

// withRemote adds a bare remote named origin next to the repository.
func withRemote(t *testing.T, repo string) string {
	t.Helper()
	remote := filepath.Join(filepath.Dir(repo), "origin.git")
	mustGit(t, "init", "-q", "--bare", remote)
	mustGit(t, "remote", "add", "origin", remote)
	return remote
}

func remoteHeads(t *testing.T, remote string) string {
	t.Helper()
	out, err := exec.Command("git", "ls-remote", "--heads", remote).Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func find(t *testing.T, name string) Branch {
	t.Helper()
	branches, err := ListBranches(Options{Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range branches {
		if b.Name == name {
			return b
		}
	}
	t.Fatalf("branch %s not found", name)
	return Branch{}
}

func TestCreatePushPull(t *testing.T) {
	repo := initRepo(t)
	remote := withRemote(t, repo)

	if err := CreateBranch("topic", "main"); err != nil {
		t.Fatal(err)
	}
	if err := CreateBranch("topic", "main"); err == nil {
		t.Error("creating an existing branch should fail")
	}
	ref, err := PushBranch(find(t, "topic"))
	if err != nil || ref != "origin/topic" {
		t.Fatalf("ref = %q, err = %v", ref, err)
	}
	topic := find(t, "topic")
	if topic.Upstream != "origin/topic" || !strings.Contains(remoteHeads(t, remote), "refs/heads/topic") {
		t.Fatalf("push should publish and track: %+v", topic)
	}

	// Someone else pushes; pulling a branch that is not checked out fast-forwards it.
	other := filepath.Join(filepath.Dir(repo), "other")
	mustGit(t, "clone", "-q", remote, other, "-b", "topic")
	mustGit(t, "-C", other, "-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "theirs")
	mustGit(t, "-C", other, "push", "-q", "origin", "topic")
	if err := PullBranch(topic); err != nil {
		t.Fatal(err)
	}
	if after := find(t, "topic"); after.SHA == topic.SHA || after.Subject != "theirs" {
		t.Errorf("pull did not fast-forward: %+v", after)
	}
	if err := PullBranch(Branch{Name: "main"}); err == nil {
		t.Error("pulling a branch without an upstream should fail")
	}
}

func TestRenameBranchRemote(t *testing.T) {
	repo := initRepo(t)
	remote := withRemote(t, repo)
	mustGit(t, "branch", "old-name")
	if _, err := PushBranch(find(t, "old-name")); err != nil {
		t.Fatal(err)
	}
	if err := RenameBranchRemote(find(t, "old-name"), "new-name"); err != nil {
		t.Fatal(err)
	}
	heads := remoteHeads(t, remote)
	if strings.Contains(heads, "old-name") || !strings.Contains(heads, "refs/heads/new-name") {
		t.Errorf("remote heads after rename:\n%s", heads)
	}
	if b := find(t, "new-name"); b.Upstream != "origin/new-name" {
		t.Errorf("renamed branch should track the new remote branch: %+v", b)
	}
	if err := RenameBranchRemote(find(t, "main"), "trunk"); err == nil {
		t.Error("a branch without an upstream cannot be renamed remotely")
	}
}

func TestArchiveBranch(t *testing.T) {
	initRepo(t)
	mustGit(t, "branch", "feature/x")
	b := find(t, "feature/x")
	if err := ArchiveBranch(b); err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command("git", "rev-parse", "archive/feature/x").Output()
	if BranchExists("feature/x") || strings.TrimSpace(string(out)) != b.SHA {
		t.Errorf("archive should tag %s and delete the branch, tag = %s", b.SHA, out)
	}
	Unarchive(Branch{Name: "feature/x", SHA: "0000"})
	if out, _ := exec.Command("git", "tag", "-l", "archive/*").Output(); len(out) == 0 {
		t.Error("Unarchive must leave a tag that points elsewhere")
	}
	mustGit(t, "branch", "feature/x")
	if err := ArchiveBranch(find(t, "feature/x")); err == nil || !BranchExists("feature/x") {
		t.Errorf("an existing archive tag should stop the archive and keep the branch, err = %v", err)
	}
	Unarchive(b)
	if out, _ := exec.Command("git", "tag", "-l", "archive/*").Output(); len(out) != 0 {
		t.Errorf("Unarchive should remove the tag, still have %s", out)
	}
}

func TestSavedViewAndConfigMap(t *testing.T) {
	initRepo(t)
	SaveView("newest", "feat")
	if sort, filter := LoadView(); sort != "newest" || filter != "feat" {
		t.Errorf("got %q, %q", sort, filter)
	}
	SaveView("", "")
	if sort, filter := LoadView(); sort != "" || filter != "" {
		t.Errorf("cleared view should be empty, got %q, %q", sort, filter)
	}
	mustGit(t, "config", "deadwood.keys.delete", "x")
	mustGit(t, "config", "deadwood.keys.selectMerged", "m")
	keys := ConfigMap("deadwood.keys")
	if keys["delete"] != "x" || keys["selectmerged"] != "m" || len(keys) != 2 {
		t.Errorf("keys = %v", keys)
	}
}

func TestSetupWorktree(t *testing.T) {
	repo := initRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".env"), []byte("SECRET=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustGit(t, "config", "--add", "deadwood.worktreeCopy", ".env")
	mustGit(t, "config", "--add", "deadwood.worktreeCopy", "missing.txt")
	mustGit(t, "config", "--add", "deadwood.worktreeCopy", "../outside")
	mustGit(t, "config", "deadwood.worktreeHook", "echo \"$DEADWOOD_MAIN_WORKTREE\" > hook.txt")

	wt := filepath.Join(filepath.Dir(repo), "wt")
	if err := AddWorktree(wt, "topic"); err != nil {
		t.Fatal(err)
	}
	done, err := SetupWorktree(repo, wt)
	if err != nil || len(done) != 2 {
		t.Fatalf("done = %v, err = %v", done, err)
	}
	if data, _ := os.ReadFile(filepath.Join(wt, ".env")); string(data) != "SECRET=1\n" {
		t.Errorf(".env not copied: %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(wt, "hook.txt")); strings.TrimSpace(string(data)) != repo {
		t.Errorf("hook did not run in the worktree with the main path: %q", data)
	}

	mustGit(t, "config", "deadwood.worktreeHook", "echo broken >&2; exit 3")
	if _, err := SetupWorktree(repo, wt); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("a failing hook should report its output, err = %v", err)
	}
}

func TestPullRequestMergeSignal(t *testing.T) {
	initRepo(t)
	mustGit(t, "checkout", "-q", "-b", "handmerged")
	commitFile(t, "a.txt", "one\n", "work")
	mustGit(t, "checkout", "-q", "-b", "moved-on")
	commitFile(t, "b.txt", "one\n", "more work")
	mustGit(t, "checkout", "-q", "main")
	merged := find(t, "handmerged")

	// A stand-in for the GitHub CLI that reports one merged pull request
	// per branch; moved-on has gained a commit since its PR was merged.
	bin := t.TempDir()
	script := "#!/bin/sh\necho '[{\"headRefName\":\"handmerged\",\"headRefOid\":\"" + merged.SHA +
		"\"},{\"headRefName\":\"moved-on\",\"headRefOid\":\"" + merged.SHA + "\"}]'\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	branches, err := ListBranches(Options{Base: "main", PullRequests: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range branches {
		switch b.Name {
		case "handmerged":
			if b.Merge != PRMerged {
				t.Errorf("handmerged: merge = %q", b.Merge)
			}
		case "moved-on":
			if b.Merge != NotMerged {
				t.Errorf("moved-on has commits after its PR and must stay unmerged, got %q", b.Merge)
			}
		}
	}
	if b := find(t, "handmerged"); b.Merge != NotMerged {
		t.Errorf("without the option gh must not be consulted, got %q", b.Merge)
	}
}
