package main

import (
	"os"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// palette is the set of colours a theme defines.
type palette struct {
	accent, muted, faint, text, success, danger, warn, rowBg, onAccent lipgloss.TerminalColor
}

var none = lipgloss.NoColor{}

var themes = map[string]palette{
	"default": {
		accent:   lipgloss.AdaptiveColor{Light: "#6D28D9", Dark: "#A78BFA"},
		muted:    lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8B8FA3"},
		faint:    lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#3F4354"},
		text:     lipgloss.AdaptiveColor{Light: "#111827", Dark: "#E5E7EB"},
		success:  lipgloss.AdaptiveColor{Light: "#047857", Dark: "#34D399"},
		danger:   lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"},
		warn:     lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"},
		rowBg:    lipgloss.AdaptiveColor{Light: "#EDE9FE", Dark: "#322A5C"},
		onAccent: lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1E1B2E"},
	},
	"ocean": {
		accent:   lipgloss.AdaptiveColor{Light: "#0369A1", Dark: "#38BDF8"},
		muted:    lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#8A9BB0"},
		faint:    lipgloss.AdaptiveColor{Light: "#CBD5E1", Dark: "#334155"},
		text:     lipgloss.AdaptiveColor{Light: "#0F172A", Dark: "#E2E8F0"},
		success:  lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#2DD4BF"},
		danger:   lipgloss.AdaptiveColor{Light: "#BE123C", Dark: "#FB7185"},
		warn:     lipgloss.AdaptiveColor{Light: "#A16207", Dark: "#FACC15"},
		rowBg:    lipgloss.AdaptiveColor{Light: "#E0F2FE", Dark: "#0C3A52"},
		onAccent: lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#082F49"},
	},
	"ember": {
		accent:   lipgloss.AdaptiveColor{Light: "#C2410C", Dark: "#FB923C"},
		muted:    lipgloss.AdaptiveColor{Light: "#78716C", Dark: "#A8A29E"},
		faint:    lipgloss.AdaptiveColor{Light: "#D6D3D1", Dark: "#44403C"},
		text:     lipgloss.AdaptiveColor{Light: "#1C1917", Dark: "#F5F5F4"},
		success:  lipgloss.AdaptiveColor{Light: "#4D7C0F", Dark: "#A3E635"},
		danger:   lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"},
		warn:     lipgloss.AdaptiveColor{Light: "#A16207", Dark: "#FDE047"},
		rowBg:    lipgloss.AdaptiveColor{Light: "#FFEDD5", Dark: "#4A2A14"},
		onAccent: lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1C1917"},
	},
	// mono uses no colour at all: emphasis comes from bold and reverse video.
	"mono": {accent: none, muted: none, faint: none, text: none, success: none, danger: none, warn: none, rowBg: none, onAccent: none},
}

var (
	appStyle = lipgloss.NewStyle().Padding(0, 1)

	titleStyle, mutedStyle, tabStyle, activeTabStyle            lipgloss.Style
	keyStyle, promptStyle, successStyle, dangerStyle, warnStyle lipgloss.Style

	theme palette
	mono  bool
)

func init() { setTheme("default") }

// setTheme rebuilds every style for the named theme and reports whether the
// name is known. NO_COLOR forces the mono theme.
func setTheme(name string) bool {
	p, ok := themes[name]
	if !ok {
		p = themes["default"]
	}
	if os.Getenv("NO_COLOR") != "" {
		name, p = "mono", themes["mono"]
	}
	theme, mono = p, name == "mono"

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(p.accent)
	mutedStyle = lipgloss.NewStyle().Foreground(p.muted)
	tabStyle = lipgloss.NewStyle().Padding(0, 1).Foreground(p.muted)
	activeTabStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(p.onAccent).Background(p.accent).Reverse(mono)
	keyStyle = lipgloss.NewStyle().Bold(true).Foreground(p.text)
	promptStyle = lipgloss.NewStyle().Bold(true).Foreground(p.accent)
	successStyle = lipgloss.NewStyle().Foreground(p.success)
	dangerStyle = lipgloss.NewStyle().Foreground(p.danger).Bold(mono)
	warnStyle = lipgloss.NewStyle().Foreground(p.warn).Bold(mono)
	return ok
}

func tableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = lipgloss.NewStyle().Padding(0, 1).
		Bold(true).
		Foreground(theme.muted).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(theme.faint).
		BorderBottom(true)
	s.Cell = lipgloss.NewStyle().Padding(0, 1).Foreground(theme.text)
	s.Selected = lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.text).
		Background(theme.rowBg).
		Reverse(mono)
	return s
}
