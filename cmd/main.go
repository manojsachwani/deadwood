



package main

import (
	"fmt"
	"os"
	"strings"
	tea "github.com/charmbracelet/bubbletea"
	table "github.com/charmbracelet/bubbles/table"
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
	table        table.Model
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
       rows := make([]table.Row, len(branches))
       for i, b := range branches {
	       status := ""
	       if b.Current {
		       status = "👑 current"
	       }
	       rows[i] = table.Row{
		       "⚪",
		       b.Name,
		       status,
		       b.Commit,
		       b.Author,
	       }
       }
       columns := []table.Column{
	       {Title: "Select", Width: 8},
	       {Title: "Branch", Width: 30},
	       {Title: "Status", Width: 10},
	       {Title: "Commit", Width: 10},
	       {Title: "Author", Width: 18},
       }
       t := table.New(
	       table.WithColumns(columns),
	       table.WithRows(rows),
	       table.WithFocused(true),
	       table.WithHeight(12),
	       table.WithWidth(80),
       )
       // Custom colorful table styles
       s := table.DefaultStyles()
       s.Header = s.Header.
               BorderStyle(lipgloss.NormalBorder()).
               BorderForeground(lipgloss.Color("#74B9FF")).
               BorderBottom(true).
               Bold(true).
               Foreground(lipgloss.Color("#FFFFFF")).
               Background(lipgloss.Color("#6C5CE7"))
       s.Selected = s.Selected.
               Foreground(lipgloss.Color("#FFFFFF")).
               Background(lipgloss.Color("#00B894")).
               Bold(true)
       s.Cell = s.Cell.
               Foreground(lipgloss.Color("#DDD6FE"))
       t.SetStyles(s)
       ti := textinput.New()
       ti.Placeholder = "New branch name"
       ti.CharLimit = 64
       return model{table: t, branches: branches, multi: make(map[int]struct{}), mode: "list", input: ti}
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
		       idx := m.table.Cursor()
		       if _, ok := m.multi[idx]; ok {
			       delete(m.multi, idx)
		       } else {
			       m.multi[idx] = struct{}{}
		       }
		       // Update rows to reflect selection
		       rows := make([]table.Row, len(m.branches))
		       for i, b := range m.branches {
			       checked := "⚪"
			       if _, ok := m.multi[i]; ok {
				       checked = "🟢"
			       }
			       status := ""
			       if b.Current {
				       status = "👑 current"
			       }
			       rows[i] = table.Row{checked, b.Name, status, b.Commit, b.Author}
		       }
		       m.table.SetRows(rows)
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
	       m.table, cmd = m.table.Update(msg)
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
		       // Refresh table
		       branches, _ := git.ListBranches()
		       m.branches = branches
		       rows := make([]table.Row, len(branches))
		       for i, b := range branches {
			       checked := "⚪"
			       if _, ok := m.multi[i]; ok {
				       checked = "🟢"
			       }
			       status := ""
			       if b.Current {
				       status = "👑 current"
			       }
			       rows[i] = table.Row{checked, b.Name, status, b.Commit, b.Author}
		       }
		       m.table.SetRows(rows)
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
		       // Refresh table
		       branches, _ := git.ListBranches()
		       m.branches = branches
		       rows := make([]table.Row, len(branches))
		       for i, b := range branches {
			       checked := "⚪"
			       if _, ok := m.multi[i]; ok {
				       checked = "🟢"
			       }
			       status := ""
			       if b.Current {
				       status = "👑 current"
			       }
			       rows[i] = table.Row{checked, b.Name, status, b.Commit, b.Author}
		       }
		       m.table.SetRows(rows)
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



func main() {
       p := tea.NewProgram(initialModel())
       if err := p.Start(); err != nil {
	       fmt.Println("Error running program:", err)
	       os.Exit(1)
       }
}
