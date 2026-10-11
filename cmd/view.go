package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Lines of the layout that are not the table: header, status line and help
// line, plus two spacers that are dropped on very short windows.
const (
	chromeHeight        = 5
	compactChromeHeight = 3
	compactBelow        = 10
)

func (m model) chrome() int {
	if m.height < compactBelow {
		return compactChromeHeight
	}
	return chromeHeight
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	width := m.width - appStyle.GetHorizontalFrameSize()
	line := lipgloss.NewStyle().MaxWidth(width)

	body := m.branchTable.View()
	if m.view == viewWorktrees {
		body = m.worktreeTable.View()
	} else if len(m.branches) == 0 && m.busy == "" {
		body = m.emptyView(width, lipgloss.Height(body))
	}

	lines := []string{
		m.headerView(width),
		"",
		line.Render(body),
		"",
		line.Render(m.statusView()),
		m.helpView(width),
	}
	if m.chrome() == compactChromeHeight {
		lines = []string{lines[0], lines[2], lines[4], lines[5]}
	}
	return appStyle.Render(strings.Join(lines, "\n"))
}

func (m model) emptyView(width, height int) string {
	msg := mutedStyle.Render("No branches yet — they appear here after the first commit.")
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, msg)
}

func (m model) headerView(width int) string {
	left := titleStyle.Render("deadwood") + mutedStyle.Render("  "+filepath.Base(m.mainRoot()))

	tab := func(id viewID, label string, count int) string {
		style := tabStyle
		if m.view == id {
			style = activeTabStyle
		}
		return style.Render(fmt.Sprintf("%s %d", label, count))
	}
	right := tab(viewBranches, "Branches", len(m.branches)) + " " +
		tab(viewWorktrees, "Worktrees", len(m.worktrees))

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return right
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m model) statusView() string {
	if m.busy != "" {
		return m.spinner.View() + " " + mutedStyle.Render(m.busy+"…")
	}
	if m.mode != modeList {
		return m.promptView()
	}
	if m.status != "" {
		return m.statusText()
	}
	// Idle: show where the cursor is, since the table has no scrollbar.
	cursor, total := m.branchTable.Cursor(), len(m.branches)
	if m.view == viewWorktrees {
		cursor, total = m.worktreeTable.Cursor(), len(m.worktrees)
	}
	if total == 0 {
		return ""
	}
	position := mutedStyle.Render(fmt.Sprintf("%d of %d", cursor+1, total))
	if m.view == viewBranches && len(m.selected) > 0 {
		return promptStyle.Render(fmt.Sprintf("%d selected", len(m.selected))) + mutedStyle.Render("  ·  ") + position
	}
	return position
}

func (m model) statusText() string {
	switch m.statusKind {
	case statusSuccess:
		return successStyle.Render("✓ " + m.status)
	case statusError:
		return dangerStyle.Render("✗ " + m.status)
	}
	return mutedStyle.Render(m.status)
}

func (m model) promptView() string {
	label := func(s string) string { return promptStyle.Render(s) + " " }

	switch m.mode {
	case modeDeleteScope:
		return label("Delete "+m.targetSummary()+"?") + mutedStyle.Render("Choose where to delete from.")
	case modeDeleteConfirm:
		where := "locally"
		if m.remoteDelete {
			where = "locally and on the remote"
		}
		warning := "This cannot be undone."
		if n := m.unmergedCount(); n == 1 && len(m.targets) == 1 {
			warning = "Not merged into the current branch — its commits may be lost."
		} else if n > 0 {
			warning = fmt.Sprintf("%d of them not merged into the current branch — commits may be lost.", n)
		}
		return label("Delete "+m.targetSummary()+" "+where+"?") + warnStyle.Render(warning)
	case modeRename:
		return label("Rename "+m.targets[0].Name+" →") + m.input.View()
	case modeWorktreeBranch:
		return label("New worktree · branch:") + m.input.View()
	case modeWorktreePath:
		return label("New worktree for "+m.wtBranch+" · path:") + m.input.View()
	case modeWorktreeRemove:
		q := label("Remove worktree " + m.showPath(m.wtTarget.Path) + "?")
		switch {
		case m.wtTarget.Dirty:
			q += warnStyle.Render("Uncommitted changes will be lost.")
		case m.wtTarget.Locked:
			q += warnStyle.Render("It is locked.")
		}
		return q
	case modeWorktreePrune:
		return label("Prune " + plural(m.prunableCount(), "stale entry") + "?")
	}
	return ""
}

func (m model) targetSummary() string {
	if len(m.targets) == 1 {
		return m.targets[0].Name
	}
	return plural(len(m.targets), "branch")
}

type hint struct{ key, desc string }

// helpView renders the key hints for the current mode. Hints are listed by
// importance and dropped from the end until the line fits.
func (m model) helpView(width int) string {
	// A validation error raised while a prompt is open takes over the help line.
	if m.mode != modeList && m.status != "" {
		return lipgloss.NewStyle().MaxWidth(width).Render(m.statusText())
	}

	var hints []hint
	switch m.mode {
	case modeList:
		if m.view == viewBranches {
			hints = []hint{
				{"space", "select"}, {"d", "delete"}, {"r", "rename"}, {"w", "worktree"},
				{"tab", "worktrees"}, {"q", "quit"}, {"ctrl+a", "all"}, {"ctrl+r", "refresh"},
			}
		} else {
			hints = []hint{
				{"n", "new"}, {"d", "remove"}, {"l", "lock"}, {"p", "prune"},
				{"tab", "branches"}, {"q", "quit"}, {"ctrl+r", "refresh"},
			}
		}
	case modeDeleteScope:
		hints = []hint{{"l", "local only"}, {"r", "local + remote"}, {"esc", "cancel"}}
	case modeRename, modeWorktreeBranch, modeWorktreePath:
		hints = []hint{{"enter", "confirm"}, {"esc", "cancel"}}
	default:
		hints = []hint{{"y", "confirm"}, {"n", "cancel"}}
	}

	sep := mutedStyle.Render("  ·  ")
	out := ""
	for i, h := range hints {
		next := keyStyle.Render(h.key) + " " + mutedStyle.Render(h.desc)
		if i > 0 {
			next = out + sep + next
		}
		if lipgloss.Width(next) > width {
			break
		}
		out = next
	}
	return out
}
