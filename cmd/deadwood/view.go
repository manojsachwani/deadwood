package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/manojsachwani/deadwood/git"
)

// Lines of the layout that are not the body: header, status line and help
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

// bodyHeight is the space for the table or whatever replaces it.
func (m model) bodyHeight() int {
	if h := m.height - m.chrome(); h > 3 {
		return h
	}
	return 3
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	width := m.width - appStyle.GetHorizontalFrameSize()
	line := lipgloss.NewStyle().MaxWidth(width)

	var body string
	switch {
	case m.mode == modeHelp:
		body = m.panel(width, helpText())
	case m.mode == modeDetail:
		body = m.panel(width, m.detailText(width))
	case m.view == viewWorktrees:
		body = m.worktreeTable.View()
	case len(m.visible) == 0 && m.busy == "":
		body = m.emptyView(width)
	default:
		body = m.branchTable.View()
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

// panel fits free-form text into the body area.
func (m model) panel(width int, text string) string {
	h := m.bodyHeight()
	return lipgloss.NewStyle().Width(width).Height(h).MaxHeight(h).MaxWidth(width).Render(text)
}

func (m model) emptyView(width int) string {
	msg := "No branches yet — they appear here after the first commit."
	if m.filter != "" {
		msg = fmt.Sprintf("No branch matches %q — esc clears the filter.", m.filter)
	}
	return lipgloss.Place(width, m.bodyHeight(), lipgloss.Center, lipgloss.Center, mutedStyle.Render(msg))
}

func (m model) headerView(width int) string {
	left := titleStyle.Render("deadwood") + mutedStyle.Render("  "+filepath.Base(m.mainRoot()))
	if m.opts.Base != "" {
		left += mutedStyle.Render("  ·  base " + m.opts.Base)
	}

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
	switch m.mode {
	case modeList:
	case modeHelp, modeDetail:
		return ""
	default:
		return m.promptView()
	}
	if m.status != "" {
		return m.statusText()
	}

	// Idle: show where the cursor is, since the table has no scrollbar.
	cursor, total := m.branchTable.Cursor(), len(m.visible)
	if m.view == viewWorktrees {
		cursor, total = m.worktreeTable.Cursor(), len(m.worktrees)
	}
	var parts []string
	if m.view == viewBranches {
		if len(m.selected) > 0 {
			parts = append(parts, promptStyle.Render(fmt.Sprintf("%d selected", len(m.selected))))
		}
		if m.filter != "" {
			parts = append(parts, promptStyle.Render("/"+m.filter))
		}
	}
	if total > 0 {
		parts = append(parts, mutedStyle.Render(fmt.Sprintf("%d of %d", cursor+1, total)))
	}
	if m.view == viewBranches && m.sort != sortName {
		parts = append(parts, mutedStyle.Render(m.sort.String()))
	}
	return strings.Join(parts, mutedStyle.Render("  ·  "))
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
	case modeFilter:
		return label("Filter:") + m.input.View()
	case modeDeleteScope:
		return label("Delete "+m.targetSummary()+"?") + mutedStyle.Render("Choose where to delete from.")
	case modeDeleteConfirm:
		where := "locally"
		if m.remoteDelete {
			where = "locally + remote"
		}
		warning := "Undo with u."
		if m.remoteDelete {
			warning = "Remote copy can't be undone."
		}
		if n := m.unmergedCount(); n == 1 && len(m.targets) == 1 {
			warning = "Not merged into " + m.baseName() + ". " + warning
		} else if n > 0 {
			warning = fmt.Sprintf("%d of them not merged into %s. %s", n, m.baseName(), warning)
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

func (m model) baseName() string {
	if m.opts.Base == "" {
		return "the base branch"
	}
	return m.opts.Base
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
				{"space", "select"}, {"d", "delete"}, {"M", "select merged"}, {"/", "filter"},
				{"?", "all keys"}, {"q", "quit"}, {"u", "undo"}, {"enter", "details"},
				{"tab", "worktrees"}, {"o", "sort"},
			}
		} else {
			hints = []hint{
				{"n", "new"}, {"d", "remove"}, {"enter", "open"}, {"?", "all keys"},
				{"q", "quit"}, {"l", "lock"}, {"p", "prune"}, {"tab", "branches"},
			}
		}
	case modeHelp, modeDetail:
		hints = []hint{{"any key", "back"}}
	case modeFilter:
		hints = []hint{{"enter", "keep filter"}, {"esc", "clear"}}
	case modeDeleteScope:
		hints = []hint{{"l", "local only"}, {"r", "local + remote"}, {"esc", "cancel"}}
	case modeRename, modeWorktreeBranch, modeWorktreePath:
		hints = []hint{{"enter", "confirm"}, {"esc", "cancel"}}
	case modeWorktreeRemove:
		hints = []hint{{"y", "remove"}}
		if b, ok := m.worktreeBranch(m.wtTarget); ok {
			hints = append(hints, hint{"b", "remove + delete branch " + b.Name})
		}
		hints = append(hints, hint{"n", "cancel"})
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

func helpText() string {
	section := func(title string, keys ...hint) string {
		var b strings.Builder
		b.WriteString(promptStyle.Render(title) + "\n")
		for _, k := range keys {
			b.WriteString("  " + keyStyle.Render(fmt.Sprintf("%-10s", k.key)) + mutedStyle.Render(k.desc) + "\n")
		}
		return b.String()
	}
	branches := section("Branches",
		hint{"space", "select / deselect"},
		hint{"ctrl+a", "select all / none"},
		hint{"M", "select merged, squashed and rebased"},
		hint{"S", "select stale"},
		hint{"d", "delete selection"},
		hint{"u", "undo the last delete"},
		hint{"r", "rename"},
		hint{"s", "switch to branch"},
		hint{"w", "create a worktree for it"},
		hint{"enter", "show unmerged commits"},
		hint{"/", "filter by name"},
		hint{"o", "change sort order"},
	)
	other := section("Worktrees",
		hint{"n", "new worktree"},
		hint{"d", "remove (optionally with its branch)"},
		hint{"enter", "quit and print its path"},
		hint{"l", "lock / unlock"},
		hint{"p", "prune missing entries"},
	) + "\n" + section("Everywhere",
		hint{"tab 1 2", "switch tab"},
		hint{"F", "fetch and prune remotes"},
		hint{"ctrl+r", "refresh"},
		hint{"esc", "clear selection, then filter"},
		hint{"q", "quit"},
	)
	return lipgloss.JoinHorizontal(lipgloss.Top, branches, "    ", other)
}

func (m model) detailText(width int) string {
	b := m.detailFor
	var out strings.Builder
	out.WriteString(promptStyle.Render(b.Name) + mutedStyle.Render("  "+b.Commit+"  "+b.Age+"  "+b.Author) + "\n")

	facts := []string{}
	if s := stateLabel(b); s != "" {
		facts = append(facts, s)
	}
	if b.Upstream != "" {
		facts = append(facts, b.Upstream+" "+syncLabel(b))
	} else {
		facts = append(facts, "no upstream")
	}
	if b.Stale && stateLabel(b) != "stale" {
		facts = append(facts, "stale")
	}
	out.WriteString(mutedStyle.Render(strings.Join(facts, "  ·  ")) + "\n\n")

	if m.detail == nil {
		out.WriteString(m.spinner.View() + mutedStyle.Render(" Loading commits…"))
		return out.String()
	}
	d := m.detail
	switch {
	case d.Total == 0:
		out.WriteString(successStyle.Render("Nothing here that is not already in " + m.baseName() + "."))
	case b.Merge == git.Squashed || b.Merge == git.Rebased:
		out.WriteString(successStyle.Render(fmt.Sprintf("%s — the same changes are already in %s as different commits.",
			plural(d.Total, "commit"), m.baseName())) + "\n")
	default:
		out.WriteString(warnStyle.Render(fmt.Sprintf("%s not in %s", plural(d.Total, "commit"), m.baseName())))
		if d.Stat != "" {
			out.WriteString(mutedStyle.Render("  ·  " + d.Stat))
		}
		out.WriteString("\n")
	}
	if d.Total > 0 {
		out.WriteString("\n")
		clip := lipgloss.NewStyle().MaxWidth(width)
		for _, c := range d.Commits {
			out.WriteString(clip.Render("  "+c) + "\n")
		}
		if d.Total > len(d.Commits) {
			out.WriteString(mutedStyle.Render(fmt.Sprintf("  … and %d more", d.Total-len(d.Commits))))
		}
	}
	return out.String()
}
