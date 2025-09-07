package main

import (
	"strings"
	"github.com/charmbracelet/lipgloss"
)

func (m model) View() string {
	if m.quitting {
		goodbyeStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6B9D")).
			Background(lipgloss.Color("#1A1A2E")).
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF6B9D"))
		return goodbyeStyle.Render("✨ Goodbye! Thanks for using Deadwood! ✨") + "\n"
	}
	var b strings.Builder

	// App title with gradient-like effect
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#6C5CE7")).
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#A29BFE")).
		MarginBottom(1)
	b.WriteString(titleStyle.Render("🌲 Deadwood - Git Branch Manager 🌲"))
	b.WriteString("\n")

	// Description with vibrant colors
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00CEC9")).
		Bold(true).
		Padding(0, 1).
		MarginBottom(1)
	b.WriteString(descStyle.Render("🚀 A beautiful TUI for managing git branches: select, delete, rename, batch ops 🚀"))
	b.WriteString("\n\n")

	// Table styling with colorful border
	tableStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#74B9FF")).
		Background(lipgloss.Color("#2D3436")).
		Padding(1, 2)
	b.WriteString(tableStyle.Render(m.table.View()))
	b.WriteString("\n")

	// Error/info message with better styling
	if m.errMsg != "" {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF7675")).
			Background(lipgloss.Color("#2D3436")).
			Bold(true).
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF7675")).
			MarginTop(1)
		b.WriteString("\n" + errorStyle.Render("⚠️  " + m.errMsg + "  ⚠️") + "\n")
	}

	// Help box with colorful styling
	help := "💫 [SPACE] select  [d] delete  [r] rename  [q] quit 💫\n🎯 Navigate: ↑/k ↓/j  Confirm: [y]  Cancel: [n]/[esc] 🎯"
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FDCB6E")).
		Background(lipgloss.Color("#2D3436")).
		Bold(true).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FDCB6E")).
		MarginTop(1)
	b.WriteString(helpStyle.Render(help))

	// Confirmation or input with enhanced styling
	switch m.mode {
	case "confirm-remote", "confirm-delete":
		confirmStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E17055")).
			Background(lipgloss.Color("#2D3436")).
			Bold(true).
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#E17055")).
			MarginTop(1)
		b.WriteString("\n" + confirmStyle.Render("🔥 " + m.confirmMsg + " 🔥") + "\n")
	case "input-rename":
		inputStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#55A3FF")).
			Background(lipgloss.Color("#2D3436")).
			Bold(true).
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#55A3FF")).
			MarginTop(1)
		b.WriteString("\n" + inputStyle.Render("✏️  Rename branch to: " + m.input.View() + "  ✏️") + "\n")
	}
	return b.String()
}
