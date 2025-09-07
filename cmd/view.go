package main

import (
	"strings"
	"github.com/charmbracelet/lipgloss"
)

func (m model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}
	var b strings.Builder

	// App title
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Background(lipgloss.Color("236")).Padding(0, 1)
	b.WriteString(titleStyle.Render(" Deadwood "))
	b.WriteString("\n")

	// Description
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	b.WriteString(descStyle.Render("A beautiful TUI for managing git branches: select, delete, rename, batch ops."))
	b.WriteString("\n\n")

	// Table styling
	tableStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Background(lipgloss.Color("236")).Padding(1, 2)
	b.WriteString(tableStyle.Render(m.table.View()))
	b.WriteString("\n")

	// Error/info message
	if m.errMsg != "" {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.errMsg) + "\n")
	}

	// Help box
	help := "[SPACE] select  [d] delete  [r] rename  [q] quit\nNavigate: ↑/k ↓/j  Confirm: [y]  Cancel: [n]/[esc]"
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Background(lipgloss.Color("234")).Padding(0, 1).MarginTop(1)
	b.WriteString(helpStyle.Render(help))

	// Confirmation or input
	switch m.mode {
	case "confirm-remote", "confirm-delete":
		b.WriteString("\n" + m.confirmMsg + "\n")
	case "input-rename":
		b.WriteString("\nRename branch to: " + m.input.View() + "\n")
	}
	return b.String()
}
