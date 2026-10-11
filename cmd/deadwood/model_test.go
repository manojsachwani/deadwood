package main

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manojsachwani/deadwood/git"
)

func testModel(t *testing.T, width, height int) model {
	t.Helper()
	m := newModel("/src/app", git.Options{Base: "origin/main", StaleDays: 90}, viewConfig{})
	m = send(m, tea.WindowSizeMsg{Width: width, Height: height})
	return send(m, loadedMsg{
		branches: []git.Branch{
			{Name: "main", Current: true, Protected: true, Merge: git.Merged, Commit: "87c33da", Author: "Ada", Age: "2 days ago", Updated: day(2), Subject: "Add things", Upstream: "origin/main", Remote: "origin"},
			{Name: "feature/login", Commit: "1a30a4e", Author: "Ada", Age: "3 weeks ago", Updated: day(21), Subject: "Login form", Upstream: "origin/feature/login", Remote: "origin", Ahead: 2, Behind: 1},
			{Name: "old-spike", Commit: "93ab520", Author: "Grace", Age: "8 months ago", Subject: "Try a thing", Upstream: "origin/old-spike", Remote: "origin", Gone: true, Merge: git.Squashed, Stale: true, Updated: day(240)},
			{Name: "hotfix", Commit: "884dcb3", Author: "Grace", Age: "5 hours ago", Updated: day(0), Subject: "Fix crash", Worktree: "/src/app-hotfix"},
		},
		worktrees: []git.Worktree{
			{Path: "/src/app", Branch: "main", Commit: "87c33da", Main: true},
			{Path: "/src/app-hotfix", Branch: "hotfix", Commit: "884dcb3", Dirty: true},
			{Path: "/src/app-gone", Commit: "93ab520", Detached: true, Prunable: true},
		},
	})
}

func day(n int) time.Time { return time.Now().Add(-time.Duration(n) * 24 * time.Hour) }

func send(m model, msg tea.Msg) model {
	next, _ := m.Update(msg)
	return next.(model)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestViewFitsWindow(t *testing.T) {
	for _, size := range [][2]int{{140, 30}, {100, 24}, {80, 20}, {50, 12}, {30, 6}} {
		m := testModel(t, size[0], size[1])
		for _, v := range []viewID{viewBranches, viewWorktrees} {
			m.view = v
			out := m.View()
			if os.Getenv("DEADWOOD_SHOW") != "" {
				t.Logf("%dx%d view %d\n%s", size[0], size[1], v, out)
			}
			if h := lipgloss.Height(out); h > size[1] {
				t.Errorf("%dx%d view %d: height %d overflows", size[0], size[1], v, h)
			}
			for _, line := range strings.Split(out, "\n") {
				if w := lipgloss.Width(line); w > size[0] {
					t.Errorf("%dx%d view %d: line width %d overflows: %q", size[0], size[1], v, w, line)
				}
			}
		}
	}
}

func TestRenameAcceptsQ(t *testing.T) {
	m := testModel(t, 100, 24)
	m = send(m, key("down"))
	m = send(m, key("r"))
	if m.mode != modeRename {
		t.Fatalf("mode = %v, want rename", m.mode)
	}
	m.input.SetValue("")
	for _, r := range "quick" {
		m = send(m, key(string(r)))
	}
	if m.mode != modeRename || m.input.Value() != "quick" {
		t.Errorf("mode = %v, input = %q; q must be typeable", m.mode, m.input.Value())
	}
	m = send(m, key("esc"))
	if m.mode != modeList {
		t.Errorf("esc should cancel, mode = %v", m.mode)
	}
}

func TestSelectionSurvivesReload(t *testing.T) {
	m := testModel(t, 100, 24)
	m = send(m, key("down"))
	m = send(m, key(" "))
	m = send(m, key(" "))
	if len(m.selected) != 2 || m.branchTable.Cursor() != 3 {
		t.Fatalf("selected = %v, cursor = %d", m.selected, m.branchTable.Cursor())
	}
	// feature/login disappears and the order changes.
	m = send(m, loadedMsg{branches: []git.Branch{{Name: "old-spike"}, {Name: "main", Current: true}}})
	if _, ok := m.selected["old-spike"]; !ok || len(m.selected) != 1 {
		t.Errorf("selected = %v, want only old-spike", m.selected)
	}
}

func TestDeleteFlow(t *testing.T) {
	m := testModel(t, 100, 24)
	m = send(m, key("down"))
	m = send(m, key("d"))
	if m.mode != modeDeleteScope || len(m.targets) != 1 || m.targets[0].Name != "feature/login" {
		t.Fatalf("mode = %v, targets = %+v", m.mode, m.targets)
	}
	m = send(m, key("r"))
	if m.mode != modeDeleteConfirm || !m.remoteDelete {
		t.Fatalf("mode = %v, remote = %v", m.mode, m.remoteDelete)
	}
	if !strings.Contains(m.View(), "Not merged into origin/main") || !strings.Contains(m.View(), "can't be undone") {
		t.Error("deleting an unmerged branch on the remote should warn")
	}
	m = send(m, key("n"))
	if m.mode != modeList {
		t.Errorf("n should cancel, mode = %v", m.mode)
	}
	m = send(m, key("down"))
	m = send(m, key("d"))
	m = send(m, key("l"))
	if strings.Contains(m.View(), "ot merged into") || !strings.Contains(m.View(), "Undo with u") {
		t.Error("deleting a squashed branch locally should not warn about unmerged commits")
	}
}

func TestEmptyRepository(t *testing.T) {
	m := newModel("/src/app", git.Options{Base: "origin/main", StaleDays: 90}, viewConfig{})
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 20})
	m = send(m, loadedMsg{worktrees: []git.Worktree{{Path: "/src/app", Branch: "main", Main: true}}})
	if !strings.Contains(m.View(), "No branches yet") {
		t.Error("empty repository should explain the empty table")
	}
	for _, k := range []string{"d", "r", "w", " ", "tab", "d", "l", "p"} {
		m = send(m, key(k))
	}
	if h := lipgloss.Height(m.View()); h > 20 {
		t.Errorf("height %d overflows", h)
	}
}

func TestPaths(t *testing.T) {
	m := newModel("/src/app", git.Options{Base: "origin/main", StaleDays: 90}, viewConfig{})
	far := "/var/folders/some/really/long/temporary/location/for/worktrees/app-feature"
	if got := m.inputPath("/src/app-x"); got != "../app-x" {
		t.Errorf("inputPath sibling = %q", got)
	}
	if got := m.inputPath(far); got != far {
		t.Errorf("inputPath must stay resolvable, got %q", got)
	}
	if got := m.showPath(far); got != "…/worktrees/app-feature" {
		t.Errorf("showPath = %q", got)
	}
}

func TestCancelClearsValidationError(t *testing.T) {
	m := testModel(t, 100, 24)
	m = send(m, key("tab"))
	m = send(m, key("n"))
	m = send(m, key("enter"))
	if m.mode != modeWorktreeBranch || m.statusKind != statusError || m.status == "" {
		t.Fatalf("empty input should be rejected in place: mode %v, status %q", m.mode, m.status)
	}
	m = send(m, key("esc"))
	if m.mode != modeList || m.status != "" {
		t.Errorf("esc should clear the error: mode %v, status %q", m.mode, m.status)
	}
}

func TestTruncateLeft(t *testing.T) {
	if got := truncateLeft("/a/very/long/path", 9); got != "…ong/path" {
		t.Errorf("got %q", got)
	}
	if got := truncateLeft("short", 9); got != "short" {
		t.Errorf("got %q", got)
	}
}

func TestDeleteSkipsProtectedBranches(t *testing.T) {
	msg := deleteBranches([]git.Branch{
		{Name: "main", Current: true},
		{Name: "hotfix", Worktree: "/src/app-hotfix"},
		{Name: "develop", Protected: true},
	}, false)
	if !msg.failed || len(msg.deleted) != 0 || !strings.Contains(msg.status, "current branch") ||
		!strings.Contains(msg.status, "checked out in a worktree") || !strings.Contains(msg.status, "protected") {
		t.Errorf("unexpected result %+v", msg)
	}
}

func TestWorktreeGuards(t *testing.T) {
	m := testModel(t, 100, 24)
	m = send(m, key("tab"))
	if m.view != viewWorktrees {
		t.Fatalf("view = %v", m.view)
	}
	m = send(m, key("d"))
	if m.mode != modeList || m.statusKind != statusError {
		t.Errorf("main worktree must not be removable: mode %v, status %q", m.mode, m.status)
	}
	m = send(m, key("down"))
	m = send(m, key("d"))
	if m.mode != modeWorktreeRemove || !strings.Contains(m.View(), "Uncommitted changes") {
		t.Errorf("dirty worktree should warn before removal: mode %v", m.mode)
	}
	m = send(m, key("esc"))
	m = send(m, key("p"))
	if m.mode != modeWorktreePrune {
		t.Errorf("prune should ask for confirmation, mode = %v", m.mode)
	}
}

func TestWorktreeFromBranch(t *testing.T) {
	m := testModel(t, 100, 24)
	m = send(m, key("down"))
	m = send(m, key("w"))
	if m.mode != modeWorktreePath || m.wtBranch != "feature/login" || m.input.Value() != "../app-feature-login" {
		t.Errorf("mode = %v, branch = %q, path = %q", m.mode, m.wtBranch, m.input.Value())
	}
	m = send(m, key("esc"))
	m = send(m, key("down"))
	m = send(m, key("down"))
	m = send(m, key("w"))
	if m.mode != modeList || m.statusKind != statusError {
		t.Errorf("branch already in a worktree should be refused: mode %v", m.mode)
	}
}

func TestFilterAndSort(t *testing.T) {
	m := testModel(t, 100, 24)
	m = send(m, key(" "))
	m = send(m, key("/"))
	for _, r := range "FEAT" {
		m = send(m, key(string(r)))
	}
	if len(m.visible) != 1 || m.visible[0].Name != "feature/login" || len(m.selected) != 0 {
		t.Fatalf("visible = %+v, selected = %v", m.visible, m.selected)
	}
	m = send(m, key("enter"))
	if m.mode != modeList || m.filter != "FEAT" {
		t.Fatalf("enter should keep the filter: mode %v, filter %q", m.mode, m.filter)
	}
	m = send(m, key("d"))
	if len(m.targets) != 1 || m.targets[0].Name != "feature/login" {
		t.Errorf("actions must use the filtered list, got %+v", m.targets)
	}
	m = send(m, key("esc"))
	m = send(m, key("esc"))
	if m.filter != "" || len(m.visible) != 4 {
		t.Errorf("esc should clear the filter: %q, %d visible", m.filter, len(m.visible))
	}

	m = send(m, key("/"))
	m = send(m, key("z"))
	if !strings.Contains(m.View(), "No branch matches") {
		t.Error("a filter without matches should say so")
	}
	m = send(m, key("esc"))

	m = send(m, key("o"))
	if m.visible[0].Name != "hotfix" {
		t.Errorf("newest first should start with hotfix, got %s", m.visible[0].Name)
	}
	m = send(m, key("o"))
	if m.visible[0].Name != "old-spike" {
		t.Errorf("oldest first should start with old-spike, got %s", m.visible[0].Name)
	}
}

func TestSmartSelect(t *testing.T) {
	m := testModel(t, 100, 24)
	m = send(m, key("M"))
	if _, ok := m.selected["old-spike"]; !ok || len(m.selected) != 1 {
		t.Errorf("M should select only deletable merged branches, got %v", m.selected)
	}
	m = send(m, key("esc"))
	m = send(m, key("S"))
	if _, ok := m.selected["old-spike"]; !ok || len(m.selected) != 1 {
		t.Errorf("S should select stale branches, got %v", m.selected)
	}
}

func TestProtectedBranchIsRefused(t *testing.T) {
	m := testModel(t, 100, 24)
	m = send(m, key("d"))
	if m.mode != modeList || m.statusKind != statusError || !strings.Contains(m.status, "cannot be deleted") {
		t.Errorf("mode %v, status %q", m.mode, m.status)
	}
}

func TestUndoBookkeeping(t *testing.T) {
	m := testModel(t, 100, 24)
	m = send(m, key("u"))
	if m.busy != "" || !strings.Contains(m.status, "Nothing to undo") {
		t.Errorf("busy %q, status %q", m.busy, m.status)
	}
	gone := []deletedBranch{{Branch: git.Branch{Name: "a", SHA: "1"}}, {Branch: git.Branch{Name: "b", SHA: "2"}}}
	m = send(m, opDoneMsg{status: "Deleted", deleted: gone[:1]})
	m = send(m, loadedMsg{})
	m = send(m, opDoneMsg{status: "Deleted", deleted: gone[1:]})
	m = send(m, loadedMsg{})
	if len(m.undo) != 1 || m.undo[0].Name != "b" || len(m.deleted) != 2 {
		t.Fatalf("undo = %+v, deleted = %+v", m.undo, m.deleted)
	}
	m = send(m, opDoneMsg{status: "Restored", restored: true})
	if len(m.undo) != 0 || len(m.deleted) != 1 || m.deleted[0].Name != "a" {
		t.Errorf("undo = %+v, deleted = %+v", m.undo, m.deleted)
	}
}

func TestDetailAndHelp(t *testing.T) {
	m := testModel(t, 100, 24)
	m = send(m, key("down"))
	m = send(m, key("enter"))
	if m.mode != modeDetail || !strings.Contains(m.View(), "Loading commits") {
		t.Fatalf("mode %v", m.mode)
	}
	m = send(m, detailMsg{name: "other", detail: git.Detail{Total: 9}})
	if m.detail != nil {
		t.Error("a detail for another branch must be ignored")
	}
	m = send(m, detailMsg{name: "feature/login", detail: git.Detail{Total: 2, Commits: []string{"abc one", "def two"}, Stat: "1 file changed"}})
	view := m.View()
	if !strings.Contains(view, "2 commits not in origin/main") || !strings.Contains(view, "abc one") {
		t.Errorf("detail view:\n%s", view)
	}
	if h := lipgloss.Height(view); h > 24 {
		t.Errorf("detail height %d overflows", h)
	}
	m = send(m, key("q"))
	if m.mode != modeList {
		t.Error("any key should close the detail view")
	}
	m = send(m, key("?"))
	if m.mode != modeHelp || !strings.Contains(m.View(), "undo the last delete") || lipgloss.Height(m.View()) > 24 {
		t.Error("help overlay should list the keys and fit")
	}
}

func TestWorktreeOpenAndBranchRemoval(t *testing.T) {
	m := testModel(t, 100, 24)
	m = send(m, key("tab"))
	m = send(m, key("down"))
	m = send(m, key("d"))
	if !strings.Contains(m.View(), "remove + delete branch hotfix") {
		t.Errorf("removing a worktree should offer its branch:\n%s", m.View())
	}
	m = send(m, key("esc"))
	next, cmd := m.Update(key("enter"))
	if next.(model).chosenPath != "/src/app-hotfix" || cmd == nil {
		t.Errorf("enter should pick the worktree, got %q", next.(model).chosenPath)
	}
}

func TestShortAge(t *testing.T) {
	const day = 24 * time.Hour
	for d, want := range map[time.Duration]string{
		30 * time.Second: "just now",
		5 * time.Hour:    "5h ago",
		3 * day:          "3d ago",
		21 * day:         "3w ago",
		240 * day:        "8mo ago",
		645 * day:        "1y 9mo ago",
		730 * day:        "2y ago",
	} {
		if got := shortAge(d); got != want {
			t.Errorf("shortAge(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestKeyRemapping(t *testing.T) {
	m := newModel("/src/app", git.Options{Base: "origin/main"}, viewConfig{
		keys: map[string]string{"delete": "x", "select": "space", "bogus": "z", "quit": "q"},
	})
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m = send(m, loadedMsg{branches: []git.Branch{{Name: "topic"}}})
	if m = send(m, key("d")); m.mode != modeList {
		t.Error("the default key must stop working once it is remapped")
	}
	if m = send(m, key("x")); m.mode != modeDeleteScope {
		t.Errorf("the custom key should delete, mode = %v", m.mode)
	}
	m = send(m, key("esc"))
	if view := m.View(); !strings.Contains(view, "x delete") || strings.Contains(view, "d delete") {
		t.Errorf("hints should show the custom key:\n%s", view)
	}
	if m.k(" ") != "space" || len(m.keyTo) != 1 {
		t.Errorf("unknown actions and no-op remaps must be ignored: %v", m.keyTo)
	}
}

func TestSavedViewIsApplied(t *testing.T) {
	m := newModel("/src/app", git.Options{}, viewConfig{sort: "oldest", filter: "feat*"})
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m = send(m, loadedMsg{branches: []git.Branch{
		{Name: "feature/b", Updated: day(1)}, {Name: "feature/a", Updated: day(9)}, {Name: "fix/feat"},
	}})
	if m.sort != sortOldest || len(m.visible) != 2 || m.visible[0].Name != "feature/a" {
		t.Errorf("sort = %v, visible = %+v", m.sort, m.visible)
	}
	if sortNames[m.sort] != "oldest" || parseSort("nonsense") != sortName {
		t.Error("sort names should round-trip and unknown ones fall back to name")
	}
}

func TestGlobFilter(t *testing.T) {
	for _, c := range []struct {
		filter, name string
		want         bool
	}{
		{"", "anything", true},
		{"LOG", "feature/login", true},
		{"feature/*", "feature/login", true},
		{"feature/*", "fix/feature", false},
		{"*fix?", "hotfix1", true},
	} {
		if got := matchesFilter(c.filter, c.name); got != c.want {
			t.Errorf("matchesFilter(%q, %q) = %v", c.filter, c.name, got)
		}
	}
}

func TestBranchPrompts(t *testing.T) {
	m := testModel(t, 110, 24)
	m = send(m, key("down"))

	m = send(m, key("n"))
	if m.mode != modeNewBranch || !strings.Contains(m.View(), "New branch from feature/login") {
		t.Fatalf("mode = %v", m.mode)
	}
	m = send(m, key("esc"))

	// feature/login tracks a live upstream, so a rename asks about the remote.
	m = send(m, key("r"))
	m.input.SetValue("feature/signin")
	m = send(m, key("enter"))
	if m.mode != modeRenameScope || m.newName != "feature/signin" || !strings.Contains(m.View(), "Also rename origin/feature/login?") {
		t.Fatalf("mode = %v, newName = %q", m.mode, m.newName)
	}
	m = send(m, key("esc"))

	m = send(m, key("a"))
	if m.mode != modeArchiveConfirm || !strings.Contains(m.View(), "archive/<name>") {
		t.Fatalf("mode = %v", m.mode)
	}
	m = send(m, key("n"))

	m = send(m, key("A"))
	if _, ok := m.selected["feature/login"]; !ok || len(m.selected) != 1 {
		t.Errorf("A should select deletable branches by the same author, got %v", m.selected)
	}
}

func TestPruneMergedWorktrees(t *testing.T) {
	m := testModel(t, 110, 24)
	m = send(m, key("tab"))
	if m = send(m, key("M")); m.mode != modeList || !strings.Contains(m.status, "No clean worktrees") {
		t.Fatalf("hotfix is dirty and unmerged: mode %v, status %q", m.mode, m.status)
	}
	branches := append([]git.Branch{}, m.branches...)
	for i := range branches {
		if branches[i].Name == "hotfix" {
			branches[i].Merge = git.Squashed
		}
	}
	worktrees := []git.Worktree{
		{Path: "/src/app", Branch: "main", Main: true},
		{Path: "/src/app-hotfix", Branch: "hotfix"},
		{Path: "/src/app-locked", Branch: "hotfix", Locked: true},
	}
	m = send(m, loadedMsg{branches: branches, worktrees: worktrees})
	if got := m.mergedWorktrees(); len(got) != 1 || got[0].Path != "/src/app-hotfix" {
		t.Fatalf("merged worktrees = %+v", got)
	}
	if m = send(m, key("M")); m.mode != modeWorktreePruneMerged || !strings.Contains(m.View(), "Remove 1 clean worktree") {
		t.Errorf("mode = %v", m.mode)
	}
}

func TestThemes(t *testing.T) {
	defer setTheme("default")
	for name := range themes {
		if !setTheme(name) {
			t.Errorf("theme %s should be known", name)
		}
		m := testModel(t, 100, 24)
		if h := lipgloss.Height(m.View()); h > 24 {
			t.Errorf("theme %s: height %d", name, h)
		}
	}
	if setTheme("nope") {
		t.Error("an unknown theme should be reported")
	}
}
