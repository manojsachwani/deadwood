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
  deadwood [flags]                 open the interactive UI
  deadwood clean [flags]           delete merged branches without the UI

Run it inside a git repository.

Flags:
  --base <ref>        compare branches against this ref
                      (default: the remote's default branch, then main/master)
  --stale-days <n>    mark branches without commits for n days as stale
                      (default 90, 0 disables)
  --prs               also treat branches as merged when the GitHub CLI (gh)
                      reports their pull request as merged
  --ci                show the CI result of each branch tip (needs gh)
  --list              print the branches and their state, then exit
  --json              like --list, as JSON including worktrees
  --version           print the version
  -h, --help          show this help

Clean flags (deadwood clean):
  --yes               delete; without it the candidates are only listed
  --stale             also delete stale branches, merged or not
  --remote            delete their upstream branches too
  --json              report what was (or would be) deleted as JSON
  --base, --stale-days, --prs as above

Configuration (git config):
  deadwood.base           default for --base
  deadwood.staleDays      default for --stale-days
  deadwood.pullRequests   true to enable --prs by default
  deadwood.ci             true to enable --ci by default
  deadwood.protect        glob of branch names never deleted (repeatable)
  deadwood.theme          default, ocean, ember or mono
  deadwood.keys.<action>  remap a key, e.g. deadwood.keys.delete x
  deadwood.worktreeCopy   file to copy into new worktrees (repeatable)
  deadwood.worktreeHook   shell command to run in a new worktree

Shell integration - jump into the worktree you pick with enter:
  dw() { local dir; dir="$(deadwood "$@")" && [ -d "$dir" ] && cd "$dir"; }
`

func main() {
	if len(os.Args) > 1 && os.Args[1] == "clean" {
		runClean(os.Args[2:])
		return
	}
	var (
		base        = flag.String("base", "", "")
		staleDays   = flag.Int("stale-days", -1, "")
		prs         = flag.Bool("prs", false, "")
		ci          = flag.Bool("ci", false, "")
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
	opts.PullRequests = opts.PullRequests || *prs
	opts.CI = opts.CI || *ci

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

	if name := git.Config("deadwood.theme"); name != "" && !setTheme(name) {
		fmt.Fprintf(os.Stderr, "deadwood: unknown theme %q, using default\n", name)
	}
	cfg := viewConfig{keys: git.ConfigMap("deadwood.keys")}
	cfg.sort, cfg.filter = git.LoadView()

	final, err := tea.NewProgram(newModel(root, opts, cfg), programOpts...).Run()
	if err != nil {
		fail(err)
	}
	m := final.(model)
	git.SaveView(sortNames[m.sort], m.filter)
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

// runClean is `deadwood clean`: the non-interactive counterpart of selecting
// the merged branches and deleting them. It applies the same rules as the UI.
func runClean(args []string) {
	fs := flag.NewFlagSet("clean", flag.ExitOnError)
	var (
		base      = fs.String("base", "", "")
		staleDays = fs.Int("stale-days", -1, "")
		prs       = fs.Bool("prs", false, "")
		yes       = fs.Bool("yes", false, "")
		stale     = fs.Bool("stale", false, "")
		remote    = fs.Bool("remote", false, "")
		asJSON    = fs.Bool("json", false, "")
	)
	fs.Bool("merged", true, "") // accepted for readability; merged branches are always included
	fs.Bool("dry-run", false, "")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	fs.Parse(args)

	if _, err := git.RepoRoot(); err != nil {
		fail(err)
	}
	opts, err := git.LoadOptions(*base, *staleDays)
	if err != nil {
		fail(err)
	}
	opts.PullRequests = opts.PullRequests || *prs
	branches, err := git.ListBranches(opts)
	if err != nil {
		fail(err)
	}

	var targets []git.Branch
	for _, b := range branches {
		if ok, _ := deletable(b); ok && (b.Merge != git.NotMerged || (*stale && b.Stale)) {
			targets = append(targets, b)
		}
	}
	if *asJSON {
		cleanJSON(opts.Base, targets, *yes, *remote)
		return
	}
	if len(targets) == 0 {
		fmt.Println("Nothing to clean.")
		return
	}
	if !*yes {
		where := ""
		if *remote {
			where = " locally and on the remote"
		}
		fmt.Printf("Would delete %s%s (run again with --yes):\n", plural(len(targets), "branch"), where)
		for _, b := range targets {
			fmt.Printf("  %s  (%s, %s)\n", b.Name, stateLabel(b), b.Age)
		}
		return
	}

	failed := false
	for _, b := range targets {
		r := cleanOne(b, *remote)
		if r.Error != "" {
			failed = true
			fmt.Fprintf(os.Stderr, "deadwood: %s: %s\n", b.Name, r.Error)
		}
		if !r.Deleted {
			continue
		}
		suffix := ""
		if r.RemoteDeleted {
			suffix = " and " + b.Upstream
		}
		fmt.Printf("Deleted %s%s. Restore with: git branch %s %s\n", b.Name, suffix, b.Name, b.SHA)
	}
	if failed {
		os.Exit(1)
	}
}

// cleanResult is one branch in the output of `deadwood clean --json`.
type cleanResult struct {
	Name          string `json:"name"`
	SHA           string `json:"sha"`
	State         string `json:"state"`
	Upstream      string `json:"upstream,omitempty"`
	Deleted       bool   `json:"deleted"`
	RemoteDeleted bool   `json:"remote_deleted"`
	Error         string `json:"error,omitempty"`
}

// cleanOne deletes a branch. A branch whose remote deletion failed is still
// reported as deleted, with the error alongside.
func cleanOne(b git.Branch, remote bool) cleanResult {
	r := cleanResult{Name: b.Name, SHA: b.SHA, State: stateLabel(b), Upstream: b.Upstream}
	remoteDeleted, err := git.DeleteBranch(b, remote)
	if err != nil {
		r.Error = err.Error()
	}
	r.Deleted = err == nil || !git.BranchExists(b.Name)
	r.RemoteDeleted = remoteDeleted
	return r
}

func cleanJSON(base string, targets []git.Branch, yes, remote bool) {
	out := struct {
		Base     string        `json:"base"`
		DryRun   bool          `json:"dry_run"`
		Branches []cleanResult `json:"branches"`
	}{Base: base, DryRun: !yes, Branches: []cleanResult{}}
	failed := false
	for _, b := range targets {
		r := cleanResult{Name: b.Name, SHA: b.SHA, State: stateLabel(b), Upstream: b.Upstream}
		if yes {
			r = cleanOne(b, remote)
			failed = failed || r.Error != ""
		}
		out.Branches = append(out.Branches, r)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
	if failed {
		os.Exit(1)
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
	ciHeader := ""
	if opts.CI {
		ciHeader = "CI\t"
	}
	fmt.Fprintln(w, "BRANCH\tSTATE\tSYNC\t"+ciHeader+"UPDATED\tUPSTREAM")
	for _, b := range branches {
		ci := ""
		if opts.CI {
			ci = dash(b.CI) + "\t"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s%s\t%s\n", b.Name, dash(stateLabel(b)), syncLabel(b), ci, b.Age, dash(b.Upstream))
	}
	return w.Flush()
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
