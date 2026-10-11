package main

import (
	"fmt"
	"path/filepath"
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
		m.refreshTables()
		return m, nil

	case opDoneMsg:
		kind := statusSuccess
		if msg.failed {
			kind = statusError
		}
		if len(msg.deleted) > 0 {
			m.undo = msg.deleted
			m.deleted = append(m.deleted, msg.deleted...)
		}
		if msg.restored {
			m.forgetDeleted(m.undo)
			m.undo = nil
		}
		m.selected = make(map[string]struct{})
		m.busy = "Refreshing"
		return m, tea.Batch(m.setStatus(kind, msg.status), m.loadCmd())

	case detailMsg:
		if m.mode != modeDetail || msg.name != m.detailFor.Name {
			return m, nil
		}
		if msg.err != nil {
			m.mode = modeList
			return m, m.setStatus(statusError, msg.err.Error())
		}
		m.detail = &msg.detail
		return m, nil

	case clearStatusMsg:
		if msg.id == m.statusID {
			m.status = ""
		}
		return m, nil

	case spinner.TickMsg:
		if m.busy == "" && !(m.mode == modeDetail && m.detail == nil) {
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
		switch {
		case m.mode == modeList:
			return m.updateList(msg)
		case m.mode == modeDetail || m.mode == modeHelp:
			m.mode = modeList
			return m, nil
		case m.mode.isInput():
			return m.updateInput(msg)
		default:
			return m.updateConfirm(msg)
		}
	}

	if m.mode.isInput() {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

// forgetDeleted removes restored branches from the session's delete log.
func (m *model) forgetDeleted(restored []deletedBranch) {
	names := make(map[string]bool, len(restored))
	for _, d := range restored {
		names[d.Name] = true
	}
	kept := m.deleted[:0:0]
	for _, d := range m.deleted {
		if !names[d.Name] {
			kept = append(kept, d)
		}
	}
	m.deleted = kept
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := m.canonical(msg.String())
	switch key {
	case "q":
		return m, tea.Quit
	case "?":
		m.mode = modeHelp
		m.status = ""
		return m, nil
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
		return m, tea.Batch(m.loadCmd(), m.spinner.Tick)
	case "F":
		return m, m.run("Fetching", func() opDoneMsg {
			if err := git.Fetch(); err != nil {
				return opDoneMsg{status: "Fetch failed: " + err.Error(), failed: true}
			}
			return opDoneMsg{status: "Fetched and pruned remotes"}
		})
	}
	if key == "" {
		return m, nil
	}
	if m.view == viewWorktrees {
		return m.updateWorktreeList(msg, key)
	}
	return m.updateBranchList(msg, key)
}

func (m model) updateBranchList(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
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
		if len(m.selected) == len(m.visible) {
			m.selected = make(map[string]struct{})
		} else {
			for _, b := range m.visible {
				m.selected[b.Name] = struct{}{}
			}
		}
		m.refreshTables()
		return m, nil

	case "M":
		n := m.selectWhere(func(b git.Branch) bool { return b.Merge != git.NotMerged })
		m.refreshTables()
		if n == 0 {
			return m, m.setStatus(statusInfo, "No merged branches to select")
		}
		return m, m.setStatus(statusInfo, "Selected "+plural(n, "merged branch"))

	case "S":
		n := m.selectWhere(func(b git.Branch) bool { return b.Stale })
		m.refreshTables()
		if n == 0 {
			return m, m.setStatus(statusInfo, fmt.Sprintf("No branches older than %d days", m.opts.StaleDays))
		}
		return m, m.setStatus(statusInfo, "Selected "+plural(n, "stale branch"))

	case "esc":
		switch {
		case len(m.selected) > 0:
			m.selected = make(map[string]struct{})
		case m.filter != "":
			m.filter = ""
		}
		m.refreshTables()
		return m, nil

	case "/":
		return m, m.startInput(modeFilter, "type to filter", m.filter)

	case "o":
		m.sort = (m.sort + 1) % sortModes
		m.refreshTables()
		m.branchTable.GotoTop()
		return m, m.setStatus(statusInfo, "Sorted by "+m.sort.String())

	case "d":
		m.targets = m.actionBranches()
		if len(m.targets) == 0 {
			return m, nil
		}
		if len(m.targets) == 1 {
			if ok, reason := deletable(m.targets[0]); !ok {
				return m, m.setStatus(statusError, fmt.Sprintf("%s cannot be deleted: %s", m.targets[0].Name, reason))
			}
		}
		m.mode = modeDeleteScope
		m.status = ""
		return m, nil

	case "u":
		if len(m.undo) == 0 {
			return m, m.setStatus(statusInfo, "Nothing to undo")
		}
		batch := m.undo
		return m, m.run("Restoring", func() opDoneMsg { return restoreBranches(batch) })

	case "r":
		targets := m.actionBranches()
		if len(targets) != 1 {
			return m, m.setStatus(statusError, "Select exactly one branch to rename")
		}
		m.targets = targets
		return m, m.startInput(modeRename, "new branch name", targets[0].Name)

	case "s":
		b, ok := m.cursorBranch()
		if !ok || b.Current {
			return m, nil
		}
		if b.Worktree != "" {
			return m, m.setStatus(statusError, b.Name+" is checked out at "+m.showPath(b.Worktree))
		}
		return m, m.run("Switching", func() opDoneMsg {
			if err := git.SwitchBranch(b.Name); err != nil {
				return opDoneMsg{status: "Switch failed: " + err.Error(), failed: true}
			}
			return opDoneMsg{status: "Switched to " + b.Name}
		})

	case "enter", "i":
		b, ok := m.cursorBranch()
		if !ok {
			return m, nil
		}
		m.mode = modeDetail
		m.detailFor = b
		m.detail = nil
		m.status = ""
		base := m.opts.Base
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			d, err := git.BranchDetail(base, b.Name)
			return detailMsg{name: b.Name, detail: d, err: err}
		})

	case "A":
		b, ok := m.cursorBranch()
		if !ok {
			return m, nil
		}
		n := m.selectWhere(func(o git.Branch) bool { return o.Author == b.Author })
		m.refreshTables()
		if n == 0 {
			return m, m.setStatus(statusInfo, "No deletable branches by "+b.Author)
		}
		return m, m.setStatus(statusInfo, fmt.Sprintf("Selected %s by %s", plural(n, "branch"), b.Author))

	case "a":
		m.targets = m.actionBranches()
		if len(m.targets) == 0 {
			return m, nil
		}
		if len(m.targets) == 1 {
			if ok, reason := deletable(m.targets[0]); !ok {
				return m, m.setStatus(statusError, fmt.Sprintf("%s cannot be archived: %s", m.targets[0].Name, reason))
			}
		}
		m.mode = modeArchiveConfirm
		m.status = ""
		return m, nil

	case "n":
		b, ok := m.cursorBranch()
		if !ok {
			return m, nil
		}
		m.targets = []git.Branch{b}
		return m, m.startInput(modeNewBranch, "new branch name", "")

	case "p":
		b, ok := m.cursorBranch()
		if !ok {
			return m, nil
		}
		return m, m.run("Pushing", func() opDoneMsg {
			ref, err := git.PushBranch(b)
			if err != nil {
				return opDoneMsg{status: "Push failed: " + err.Error(), failed: true}
			}
			return opDoneMsg{status: fmt.Sprintf("Pushed %s to %s", b.Name, ref)}
		})

	case "P":
		b, ok := m.cursorBranch()
		if !ok {
			return m, nil
		}
		if b.Worktree != "" && !b.Current {
			return m, m.setStatus(statusError, b.Name+" is checked out at "+m.showPath(b.Worktree)+"; pull it there")
		}
		return m, m.run("Pulling", func() opDoneMsg {
			if err := git.PullBranch(b); err != nil {
				return opDoneMsg{status: "Pull failed: " + err.Error(), failed: true}
			}
			return opDoneMsg{status: "Updated " + b.Name + " from " + b.Upstream}
		})

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

func (m model) updateWorktreeList(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "n":
		return m, m.startInput(modeWorktreeBranch, "existing or new branch name", "")

	case "enter":
		wt, ok := m.cursorWorktree()
		if !ok {
			return m, nil
		}
		if wt.Prunable || wt.Bare {
			return m, m.setStatus(statusError, "This worktree has no directory to open")
		}
		m.chosenPath = wt.Path
		return m, tea.Quit

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

	case "M":
		if len(m.mergedWorktrees()) == 0 {
			return m, m.setStatus(statusInfo, "No clean worktrees on merged branches")
		}
		m.mode = modeWorktreePruneMerged
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
		if m.mode == modeFilter {
			m.filter = ""
			m.refreshTables()
		}
		m.cancel()
		return m, nil
	case "enter":
		if m.mode == modeFilter {
			m.cancel()
			return m, nil
		}
		return m.submitInput()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.mode == modeFilter && m.filter != m.input.Value() {
		m.filter = m.input.Value()
		m.refreshTables()
		m.branchTable.GotoTop()
	}
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
		if b := m.targets[0]; b.Upstream != "" && !b.Gone {
			m.newName = value
			m.mode = modeRenameScope
			m.status = ""
			m.input.Blur()
			return m, nil
		}
		return m, m.run("Renaming", func() opDoneMsg {
			if err := git.RenameBranch(old, value); err != nil {
				return opDoneMsg{status: "Rename failed: " + err.Error(), failed: true}
			}
			return opDoneMsg{status: fmt.Sprintf("Renamed %s → %s", old, value)}
		})

	case modeNewBranch:
		start := m.targets[0].Name
		return m, m.run("Creating branch", func() opDoneMsg {
			if err := git.CreateBranch(value, start); err != nil {
				return opDoneMsg{status: "Create failed: " + err.Error(), failed: true}
			}
			return opDoneMsg{status: fmt.Sprintf("Created %s from %s", value, start)}
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
		branch, path, mainRoot := m.wtBranch, expandPath(value), m.mainRoot()
		return m, m.run("Creating worktree", func() opDoneMsg {
			if err := git.AddWorktree(path, branch); err != nil {
				return opDoneMsg{status: "Worktree failed: " + err.Error(), failed: true}
			}
			done := opDoneMsg{status: fmt.Sprintf("Created worktree for %s at %s", branch, m.showPath(path))}
			abs, err := filepath.Abs(path)
			if err != nil {
				return done
			}
			steps, err := git.SetupWorktree(mainRoot, abs)
			if len(steps) > 0 {
				done.status += " · " + strings.Join(steps, ", ")
			}
			if err != nil {
				done.status += " · " + err.Error()
				done.failed = true
			}
			return done
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

	case modeRenameScope:
		b, name := m.targets[0], m.newName
		switch key {
		case "l":
			return m, m.run("Renaming", func() opDoneMsg {
				if err := git.RenameBranch(b.Name, name); err != nil {
					return opDoneMsg{status: "Rename failed: " + err.Error(), failed: true}
				}
				return opDoneMsg{status: fmt.Sprintf("Renamed %s → %s", b.Name, name)}
			})
		case "r":
			return m, m.run("Renaming", func() opDoneMsg {
				if err := git.RenameBranchRemote(b, name); err != nil {
					return opDoneMsg{status: "Rename: " + err.Error(), failed: true}
				}
				return opDoneMsg{status: fmt.Sprintf("Renamed %s → %s locally and on %s", b.Name, name, b.Remote)}
			})
		}

	case modeArchiveConfirm:
		if key == "y" {
			targets := m.targets
			return m, m.run("Archiving", func() opDoneMsg { return archiveBranches(targets) })
		}

	case modeWorktreePruneMerged:
		if key == "y" || key == "b" {
			targets, withBranches := m.mergedWorktrees(), key == "b"
			branches := make([]git.Branch, len(targets))
			for i, wt := range targets {
				branches[i], _ = m.worktreeBranch(wt)
			}
			return m, m.run("Removing worktrees", func() opDoneMsg {
				return pruneMergedWorktrees(targets, branches, withBranches)
			})
		}

	case modeDeleteConfirm:
		if key == "y" {
			targets, remote := m.targets, m.remoteDelete
			return m, m.run("Deleting", func() opDoneMsg { return deleteBranches(targets, remote) })
		}

	case modeWorktreeRemove:
		wt := m.wtTarget
		branch, withBranch := m.worktreeBranch(wt)
		if key == "y" || (key == "b" && withBranch) {
			alsoBranch := key == "b"
			return m, m.run("Removing worktree", func() opDoneMsg {
				if err := git.RemoveWorktree(wt, wt.Dirty || wt.Locked); err != nil {
					return opDoneMsg{status: "Remove failed: " + err.Error(), failed: true}
				}
				done := opDoneMsg{status: "Removed worktree " + m.showPath(wt.Path)}
				if !alsoBranch {
					return done
				}
				if _, err := git.DeleteBranch(branch, false); err != nil {
					done.status += ", but not branch " + branch.Name + ": " + err.Error()
					done.failed = true
					return done
				}
				done.status += " and branch " + branch.Name + " · u to undo the branch"
				done.deleted = []deletedBranch{{Branch: branch}}
				return done
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

// worktreeBranch returns the branch a worktree has checked out and whether
// it may be deleted together with the worktree.
func (m model) worktreeBranch(wt git.Worktree) (git.Branch, bool) {
	for _, b := range m.branches {
		if b.Name == wt.Branch && wt.Branch != "" {
			return b, !b.Protected
		}
	}
	return git.Branch{}, false
}

func deleteBranches(targets []git.Branch, remote bool) opDoneMsg {
	var deleted []deletedBranch
	var failed []string
	for _, b := range targets {
		ok, reason := deletable(b)
		if ok {
			remoteDeleted, err := git.DeleteBranch(b, remote)
			if err == nil || !git.BranchExists(b.Name) {
				deleted = append(deleted, deletedBranch{Branch: b, remote: remoteDeleted})
			}
			if err == nil {
				continue
			}
			reason = err.Error()
		}
		failed = append(failed, fmt.Sprintf("%s (%s)", b.Name, reason))
	}
	if len(failed) == 0 {
		return opDoneMsg{status: "Deleted " + plural(len(deleted), "branch") + " · u to undo", deleted: deleted}
	}
	return opDoneMsg{
		status:  fmt.Sprintf("Deleted %d · not done: %s", len(deleted), strings.Join(failed, ", ")),
		failed:  true,
		deleted: deleted,
	}
}

func archiveBranches(targets []git.Branch) opDoneMsg {
	var deleted []deletedBranch
	var failed []string
	for _, b := range targets {
		ok, reason := deletable(b)
		if ok {
			err := git.ArchiveBranch(b)
			if err == nil {
				deleted = append(deleted, deletedBranch{Branch: b, archived: true})
				continue
			}
			reason = err.Error()
		}
		failed = append(failed, fmt.Sprintf("%s (%s)", b.Name, reason))
	}
	status := fmt.Sprintf("Archived %s under %s", plural(len(deleted), "branch"), git.ArchivePrefix)
	if len(failed) == 0 {
		return opDoneMsg{status: status + " · u to undo", deleted: deleted}
	}
	return opDoneMsg{status: status + " · not done: " + strings.Join(failed, ", "), failed: true, deleted: deleted}
}

func pruneMergedWorktrees(targets []git.Worktree, branches []git.Branch, withBranches bool) opDoneMsg {
	removed := 0
	var deleted []deletedBranch
	var failed []string
	for i, wt := range targets {
		if err := git.RemoveWorktree(wt, false); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%s)", wt.Branch, err))
			continue
		}
		removed++
		if !withBranches {
			continue
		}
		if _, err := git.DeleteBranch(branches[i], false); err != nil {
			failed = append(failed, fmt.Sprintf("branch %s (%s)", wt.Branch, err))
			continue
		}
		deleted = append(deleted, deletedBranch{Branch: branches[i]})
	}
	status := "Removed " + plural(removed, "worktree")
	if len(deleted) > 0 {
		status += " and " + plural(len(deleted), "branch") + " · u to undo the branches"
	}
	if len(failed) > 0 {
		return opDoneMsg{status: status + " · not done: " + strings.Join(failed, ", "), failed: true, deleted: deleted}
	}
	return opDoneMsg{status: status, deleted: deleted}
}

func restoreBranches(batch []deletedBranch) opDoneMsg {
	var failed []string
	remote := 0
	for _, d := range batch {
		if err := git.RestoreBranch(d.Branch); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%s)", d.Name, err))
			continue
		}
		if d.archived {
			git.Unarchive(d.Branch)
		}
		if d.remote {
			remote++
		}
	}
	if len(failed) > 0 {
		return opDoneMsg{status: "Could not restore " + strings.Join(failed, ", "), failed: true}
	}
	status := "Restored " + plural(len(batch), "branch")
	if remote > 0 {
		status += " locally · remote copies stay deleted, push to recreate them"
	}
	return opDoneMsg{status: status, restored: true}
}
