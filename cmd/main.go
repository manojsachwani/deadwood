package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojsachwani/deadwood/git"
)

func main() {
	root, err := git.RepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "deadwood:", err)
		os.Exit(1)
	}
	p := tea.NewProgram(newModel(root), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "deadwood:", err)
		os.Exit(1)
	}
}
