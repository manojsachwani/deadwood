package main

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

var (
	colorAccent  = lipgloss.AdaptiveColor{Light: "#6D28D9", Dark: "#A78BFA"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8B8FA3"}
	colorFaint   = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#3F4354"}
	colorText    = lipgloss.AdaptiveColor{Light: "#111827", Dark: "#E5E7EB"}
	colorSuccess = lipgloss.AdaptiveColor{Light: "#047857", Dark: "#34D399"}
	colorDanger  = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	colorWarn    = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}
	colorRowBg   = lipgloss.AdaptiveColor{Light: "#EDE9FE", Dark: "#322A5C"}

	appStyle   = lipgloss.NewStyle().Padding(0, 1)
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)

	tabStyle       = lipgloss.NewStyle().Padding(0, 1).Foreground(colorMuted)
	activeTabStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1E1B2E"}).
			Background(colorAccent)

	keyStyle     = lipgloss.NewStyle().Bold(true).Foreground(colorText)
	promptStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	successStyle = lipgloss.NewStyle().Foreground(colorSuccess)
	dangerStyle  = lipgloss.NewStyle().Foreground(colorDanger)
	warnStyle    = lipgloss.NewStyle().Foreground(colorWarn)
)

func tableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		Bold(true).
		Foreground(colorMuted).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colorFaint).
		BorderBottom(true)
	s.Cell = s.Cell.Foreground(colorText)
	s.Selected = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorText).
		Background(colorRowBg)
	return s
}
