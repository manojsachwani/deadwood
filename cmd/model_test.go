package main

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manojsachwani/deadwood/git"
)

func testModel(t *testing.T, width, height int) model {
	t.Helper()
	m := newModel("/src/app")
	m = send(m, tea.WindowSizeMsg{Width: width, Height: height})
	return send(m, loadedMsg{
		branches: []git.Branch{
			{Name: "main", Current: true, Commit: "87c33da", Author: "Ada", Age: "2 days ago", Subject: "Add things", Upstream: "origin/main", Remote: "origin"},
			{Name: "feature/login", Commit: "1a30a4e", Author: "Ada", Age: "3 weeks ago", Subject: "Login form", Upstream: "origin/feature/login", Remote: "origin", Ahead: 2, Behind: 1},
			{Name: "old-spike", Commit: "93ab520", Author: "Grace", Age: "8 months ago", Subject: "Try a thing", Upstream: "origin/old-spike", Remote: "origin", Gone: true, Merged: true},
			{Name: "hotfix", Commit: "884dcb3", Author: "Grace", Age: "5 hours ago", Subject: "Fix crash", Worktree: "/src/app-hotfix"},
		},
		worktrees: []git.Worktree{
			{Path: "/src/app", Branch: "main", Commit: "87c33da", Main: true},
			{Path: "/src/app-hotfix", Branch: "hotfix", Commit: "884dcb3", Dirty: true},
			{Path: "/src/app-gone", Commit: "93ab520", Detached: true, Prunable: true},
		},
	})
}

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
	if !strings.Contains(m.View(), "Not merged") {
		t.Error("deleting an unmerged branch should warn")
	}
	m = send(m, key("n"))
	if m.mode != modeList {
		t.Errorf("n should cancel, mode = %v", m.mode)
	}
	m = send(m, key("down"))
	m = send(m, key("d"))
	m = send(m, key("l"))
	if strings.Contains(m.View(), "may be lost") {
		t.Error("deleting a merged branch should not warn about lost commits")
	}
}

func TestEmptyRepository(t *testing.T) {
	m := newModel("/src/app")
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
	m := newModel("/src/app")
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
	}, false)
	if !msg.failed || !strings.Contains(msg.status, "current branch") || !strings.Contains(msg.status, "checked out in a worktree") {
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
