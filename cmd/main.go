package main

import (
	"fmt"
	"os"
	"strings"
	tea "github.com/charmbracelet/bubbletea"
	list "github.com/charmbracelet/bubbles/list"
	textinput "github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/yourusername/deadwood/git"
)

type branchItem struct {
	branch git.Branch
}

func (b branchItem) Title() string    { return b.branch.Name }
func (b branchItem) Description() string { if b.branch.Current { return "(current)" } else { return "" } }
func (b branchItem) FilterValue() string { return b.branch.Name }
type model struct {
	list         list.Model
	branches     []git.Branch
	quitting     bool
	multi        map[int]struct{}
	mode         string // "list", "confirm-delete", "input-rename", "confirm-remote"
	input        textinput.Model
	action       string // "delete" or "rename"
	errMsg       string
	confirmMsg   string
	remoteDelete bool
}

func initialModel() model {
	branches, err := git.ListBranches()
	if err != nil {
		fmt.Println("Error listing branches:", err)
		os.Exit(1)
	}
	items := make([]list.Item, len(branches))
	for i, b := range branches {
		items[i] = branchItem{branch: b}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select branches (space to select, d: delete, r: rename, q: quit)"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	ti := textinput.New()
	ti.Placeholder = "New branch name"
	ti.CharLimit = 64
	return model{list: l, branches: branches, multi: make(map[int]struct{}), mode: "list", input: ti}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case "list":
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			case " ":
				idx := m.list.Index()
				if _, ok := m.multi[idx]; ok {
					delete(m.multi, idx)
				} else {
					m.multi[idx] = struct{}{}
				}
				return m, nil
			case "d":
				if len(m.multi) == 0 {
					m.errMsg = "No branches selected. Use space to select."
					return m, nil
				}
				m.mode = "confirm-remote"
				m.action = "delete"
				m.confirmMsg = "Delete selected branches locally only (l) or also remotely (r)?"
				return m, nil
			case "r":
				if len(m.multi) != 1 {
					m.errMsg = "Select exactly one branch to rename."
					return m, nil
				}
				m.mode = "input-rename"
				m.action = "rename"
				m.input.SetValue("")
				return m, textinput.Blink
			}
		}
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	case "confirm-remote":
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "l":
				m.remoteDelete = false
				m.mode = "confirm-delete"
				m.confirmMsg = "Are you sure you want to delete selected branches locally? (y/n)"
				return m, nil
			case "r":
				m.remoteDelete = true
				m.mode = "confirm-delete"
				m.confirmMsg = "Are you sure you want to delete selected branches locally and remotely? (y/n)"
				return m, nil
			case "q", "esc":
				m.mode = "list"
				return m, nil
			}
		}
	case "confirm-delete":
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "y":
				// Perform delete
				var failed []string
				for idx := range m.multi {
					branch := m.branches[idx]
					if branch.Current {
						failed = append(failed, branch.Name+" (current)")
						continue
					}
					err := git.DeleteBranch(branch.Name, m.remoteDelete)
					if err != nil {
						failed = append(failed, branch.Name)
					}
				}
				if len(failed) > 0 {
					m.errMsg = "Failed to delete: " + strings.Join(failed, ", ")
				} else {
					m.errMsg = "Deleted selected branches."
				}
				m.mode = "list"
				m.multi = make(map[int]struct{})
				// Refresh list
				branches, _ := git.ListBranches()
				m.branches = branches
				items := make([]list.Item, len(branches))
				for i, b := range branches {
					items[i] = branchItem{branch: b}
				}
				m.list.SetItems(items)
				return m, nil
			case "n", "q", "esc":
				m.mode = "list"
				return m, nil
			}
		}
	case "input-rename":
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "enter":
				// Perform rename
				var idx int
				for i := range m.multi { idx = i }
				oldName := m.branches[idx].Name
				newName := m.input.Value()
				if newName == "" {
					m.errMsg = "Branch name cannot be empty."
					return m, nil
				}
				err := git.RenameBranch(oldName, newName)
				if err != nil {
					m.errMsg = "Rename failed: " + err.Error()
				} else {
					m.errMsg = "Renamed branch."
				}
				m.mode = "list"
				m.multi = make(map[int]struct{})
				// Refresh list
				branches, _ := git.ListBranches()
				m.branches = branches
				items := make([]list.Item, len(branches))
				for i, b := range branches {
					items[i] = branchItem{branch: b}
				}
				m.list.SetItems(items)
				return m, nil
			case "esc", "q":
				m.mode = "list"
				return m, nil
			}
		}
		return m, cmd
	}
	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Padding(1, 2).Render(m.list.View()))
	// Show multi-select marks
	if m.mode == "list" && len(m.multi) > 0 {
		b.WriteString("\nSelected: ")
		for idx := range m.multi {
			b.WriteString(m.branches[idx].Name + ", ")
		}
		b.WriteString("\n")
	}
	if m.errMsg != "" {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.errMsg) + "\n")
	}
	switch m.mode {
	case "confirm-remote", "confirm-delete":
		b.WriteString("\n" + m.confirmMsg + "\n")
	case "input-rename":
		b.WriteString("\nRename branch to: " + m.input.View() + "\n")
	}
	return b.String()
}

func main() {
       p := tea.NewProgram(initialModel())
       if err := p.Start(); err != nil {
	       fmt.Println("Error running program:", err)
	       os.Exit(1)
       }
}
