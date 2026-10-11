package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojsachwani/deadwood/git"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.refreshTables()
		return m, nil

	case loadedMsg:
		m.busy = ""
		if msg.err != nil {
			return m, m.setStatus(statusError, msg.err.Error())
		}
		m.branches = msg.branches
		m.worktrees = msg.worktrees
		m.pruneSelection()
		m.refreshTables()
		return m, nil

	case opDoneMsg:
		kind := statusSuccess
		if msg.failed {
			kind = statusError
		}
		m.selected = make(map[string]struct{})
		m.busy = "Refreshing"
		return m, tea.Batch(m.setStatus(kind, msg.status), loadCmd)

	case clearStatusMsg:
		if msg.id == m.statusID {
			m.status = ""
		}
		return m, nil

	case spinner.TickMsg:
		if m.busy == "" {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.busy != "" {
			return m, nil
		}
		switch m.mode {
		case modeList:
			return m.updateList(msg)
		case modeRename, modeWorktreeBranch, modeWorktreePath:
			return m.updateInput(msg)
		default:
			return m.updateConfirm(msg)
		}
	}

	if m.mode == modeRename || m.mode == modeWorktreeBranch || m.mode == modeWorktreePath {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

// pruneSelection drops selected names that no longer exist.
func (m *model) pruneSelection() {
	names := make(map[string]struct{}, len(m.branches))
	for _, b := range m.branches {
		names[b.Name] = struct{}{}
	}
	for name := range m.selected {
		if _, ok := names[name]; !ok {
			delete(m.selected, name)
		}
	}
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "tab", "shift+tab":
		m.view = 1 - m.view
		m.status = ""
		return m, nil
	case "1":
		m.view = viewBranches
		return m, nil
	case "2":
		m.view = viewWorktrees
		return m, nil
	case "ctrl+r":
		m.busy = "Refreshing"
		return m, tea.Batch(loadCmd, m.spinner.Tick)
	}
	if m.view == viewWorktrees {
		return m.updateWorktreeList(msg)
	}
	return m.updateBranchList(msg)
}

func (m model) updateBranchList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case " ":
		b, ok := m.cursorBranch()
		if !ok {
			return m, nil
		}
		if _, sel := m.selected[b.Name]; sel {
			delete(m.selected, b.Name)
		} else {
			m.selected[b.Name] = struct{}{}
		}
		m.refreshTables()
		m.branchTable.MoveDown(1)
		return m, nil

	case "ctrl+a":
		if len(m.selected) == len(m.branches) {
			m.selected = make(map[string]struct{})
		} else {
			for _, b := range m.branches {
				m.selected[b.Name] = struct{}{}
			}
		}
		m.refreshTables()
		return m, nil

	case "esc":
		if len(m.selected) > 0 {
			m.selected = make(map[string]struct{})
			m.refreshTables()
		}
		return m, nil

	case "d":
		m.targets = m.actionBranches()
		if len(m.targets) == 0 {
			return m, nil
		}
		m.mode = modeDeleteScope
		m.status = ""
		return m, nil

	case "r":
		targets := m.actionBranches()
		if len(targets) != 1 {
			return m, m.setStatus(statusError, "Select exactly one branch to rename")
		}
		m.targets = targets
		return m, m.startInput(modeRename, "new branch name", targets[0].Name)

	case "w":
		b, ok := m.cursorBranch()
		if !ok {
			return m, nil
		}
		if b.Worktree != "" {
			return m, m.setStatus(statusError, b.Name+" is already checked out at "+m.showPath(b.Worktree))
		}
		m.wtBranch = b.Name
		return m, m.startInput(modeWorktreePath, "path", m.inputPath(git.DefaultWorktreePath(m.mainRoot(), b.Name)))
	}

	var cmd tea.Cmd
	m.branchTable, cmd = m.branchTable.Update(msg)
	return m, cmd
}

func (m model) updateWorktreeList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "n":
		return m, m.startInput(modeWorktreeBranch, "existing or new branch name", "")

	case "d":
		wt, ok := m.cursorWorktree()
		if !ok {
			return m, nil
		}
		if wt.Main {
			return m, m.setStatus(statusError, "The main worktree cannot be removed")
		}
		if wt.Path == m.root {
			return m, m.setStatus(statusError, "Cannot remove the worktree deadwood is running in")
		}
		if wt.Prunable {
			return m, m.setStatus(statusInfo, "This worktree's directory is missing — press p to prune it")
		}
		m.wtTarget = wt
		m.mode = modeWorktreeRemove
		m.status = ""
		return m, nil

	case "p":
		if m.prunableCount() == 0 {
			return m, m.setStatus(statusInfo, "Nothing to prune")
		}
		m.mode = modeWorktreePrune
		m.status = ""
		return m, nil

	case "l":
		wt, ok := m.cursorWorktree()
		if !ok {
			return m, nil
		}
		if wt.Main {
			return m, m.setStatus(statusError, "The main worktree cannot be locked")
		}
		lock := !wt.Locked
		label, done := "Unlocking", "Unlocked "
		if lock {
			label, done = "Locking", "Locked "
		}
		return m, m.run(label, func() opDoneMsg {
			if err := git.SetWorktreeLocked(wt.Path, lock); err != nil {
				return opDoneMsg{status: err.Error(), failed: true}
			}
			return opDoneMsg{status: done + m.showPath(wt.Path)}
		})
	}

	var cmd tea.Cmd
	m.worktreeTable, cmd = m.worktreeTable.Update(msg)
	return m, cmd
}

func (m model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.cancel()
		return m, nil
	case "enter":
		return m.submitInput()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) submitInput() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.input.Value())
	if value == "" {
		return m, m.setStatus(statusError, "A value is required")
	}

	switch m.mode {
	case modeRename:
		old := m.targets[0].Name
		if value == old {
			m.cancel()
			return m, nil
		}
		return m, m.run("Renaming", func() opDoneMsg {
			if err := git.RenameBranch(old, value); err != nil {
				return opDoneMsg{status: "Rename failed: " + err.Error(), failed: true}
			}
			return opDoneMsg{status: fmt.Sprintf("Renamed %s → %s", old, value)}
		})

	case modeWorktreeBranch:
		for _, b := range m.branches {
			if b.Name == value && b.Worktree != "" {
				return m, m.setStatus(statusError, value+" is already checked out at "+m.showPath(b.Worktree))
			}
		}
		m.wtBranch = value
		return m, m.startInput(modeWorktreePath, "path", m.inputPath(git.DefaultWorktreePath(m.mainRoot(), value)))

	case modeWorktreePath:
		branch, path := m.wtBranch, expandPath(value)
		return m, m.run("Creating worktree", func() opDoneMsg {
			if err := git.AddWorktree(path, branch); err != nil {
				return opDoneMsg{status: "Worktree failed: " + err.Error(), failed: true}
			}
			return opDoneMsg{status: fmt.Sprintf("Created worktree for %s at %s", branch, m.showPath(path))}
		})
	}
	return m, nil
}

func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "esc" || key == "n" || key == "q" {
		m.cancel()
		return m, nil
	}

	switch m.mode {
	case modeDeleteScope:
		switch key {
		case "l":
			m.remoteDelete = false
			m.mode = modeDeleteConfirm
		case "r":
			m.remoteDelete = true
			m.mode = modeDeleteConfirm
		}

	case modeDeleteConfirm:
		if key == "y" {
			targets, remote := m.targets, m.remoteDelete
			return m, m.run("Deleting", func() opDoneMsg { return deleteBranches(targets, remote) })
		}

	case modeWorktreeRemove:
		if key == "y" {
			wt := m.wtTarget
			return m, m.run("Removing worktree", func() opDoneMsg {
				if err := git.RemoveWorktree(wt, wt.Dirty || wt.Locked); err != nil {
					return opDoneMsg{status: "Remove failed: " + err.Error(), failed: true}
				}
				return opDoneMsg{status: "Removed worktree " + m.showPath(wt.Path)}
			})
		}

	case modeWorktreePrune:
		if key == "y" {
			n := m.prunableCount()
			return m, m.run("Pruning", func() opDoneMsg {
				if err := git.PruneWorktrees(); err != nil {
					return opDoneMsg{status: "Prune failed: " + err.Error(), failed: true}
				}
				return opDoneMsg{status: "Pruned " + plural(n, "stale entry")}
			})
		}
	}
	return m, nil
}

func deleteBranches(targets []git.Branch, remote bool) opDoneMsg {
	deleted := 0
	var failed []string
	for _, b := range targets {
		var reason string
		switch {
		case b.Current:
			reason = "current branch"
		case b.Worktree != "":
			reason = "checked out in a worktree"
		default:
			if err := git.DeleteBranch(b, remote); err != nil {
				reason = err.Error()
			}
		}
		if reason == "" {
			deleted++
			continue
		}
		failed = append(failed, fmt.Sprintf("%s (%s)", b.Name, reason))
	}
	if len(failed) == 0 {
		return opDoneMsg{status: "Deleted " + plural(deleted, "branch")}
	}
	return opDoneMsg{
		status: fmt.Sprintf("Deleted %d, skipped %s", deleted, strings.Join(failed, ", ")),
		failed: true,
	}
}
