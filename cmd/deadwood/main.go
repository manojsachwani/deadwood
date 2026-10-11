package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"text/tabwriter"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manojsachwani/deadwood/git"
	"github.com/muesli/termenv"
)

// version is set by the release build; other builds report the module version.
var version = ""

const usage = `deadwood - clear out git branches and worktrees

Usage:
  deadwood [flags]

Run it inside a git repository to open the interactive UI.

Flags:
  --base <ref>        compare branches against this ref
                      (default: the remote's default branch, then main/master)
  --stale-days <n>    mark branches without commits for n days as stale
                      (default 90, 0 disables)
  --list              print the branches and their state, then exit
  --json              like --list, as JSON including worktrees
  --version           print the version
  -h, --help          show this help

Configuration (git config):
  deadwood.base       default for --base
  deadwood.staleDays  default for --stale-days
  deadwood.protect    glob of branch names that are never deleted (repeatable)

Shell integration - jump into the worktree you pick with enter:
  dw() { local dir; dir="$(deadwood "$@")" && [ -d "$dir" ] && cd "$dir"; }
`

func main() {
	var (
		base        = flag.String("base", "", "")
		staleDays   = flag.Int("stale-days", -1, "")
		list        = flag.Bool("list", false, "")
		asJSON      = flag.Bool("json", false, "")
		showVersion = flag.Bool("version", false, "")
	)
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	if *showVersion {
		fmt.Println("deadwood", buildVersion())
		return
	}

	root, err := git.RepoRoot()
	if err != nil {
		fail(err)
	}
	opts, err := git.LoadOptions(*base, *staleDays)
	if err != nil {
		fail(err)
	}

	if *list || *asJSON {
		if err := printList(opts, *asJSON); err != nil {
			fail(err)
		}
		return
	}

	// With stdout captured (the shell wrapper reads the chosen worktree from
	// it), the interface is drawn on stderr instead.
	programOpts := []tea.ProgramOption{tea.WithAltScreen()}
	if info, err := os.Stdout.Stat(); err == nil && info.Mode()&os.ModeCharDevice == 0 {
		programOpts = append(programOpts, tea.WithOutput(os.Stderr))
		lipgloss.DefaultRenderer().SetOutput(termenv.NewOutput(os.Stderr))
	}

	final, err := tea.NewProgram(newModel(root, opts), programOpts...).Run()
	if err != nil {
		fail(err)
	}
	m := final.(model)
	if len(m.deleted) > 0 {
		fmt.Fprintln(os.Stderr, "Branches deleted in this session can be restored with:")
		for _, d := range m.deleted {
			fmt.Fprintf(os.Stderr, "  git branch %s %s\n", d.Name, d.SHA)
		}
	}
	if m.chosenPath != "" {
		fmt.Println(m.chosenPath)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "deadwood:", err)
	os.Exit(1)
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

func printList(opts git.Options, asJSON bool) error {
	branches, err := git.ListBranches(opts)
	if err != nil {
		return err
	}
	if asJSON {
		worktrees, err := git.ListWorktrees()
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Base      string         `json:"base"`
			Branches  []git.Branch   `json:"branches"`
			Worktrees []git.Worktree `json:"worktrees"`
		}{opts.Base, branches, worktrees})
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "BRANCH\tSTATE\tSYNC\tUPDATED\tUPSTREAM")
	for _, b := range branches {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", b.Name, dash(stateLabel(b)), syncLabel(b), b.Age, dash(b.Upstream))
	}
	return w.Flush()
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
