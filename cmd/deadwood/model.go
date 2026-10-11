package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojsachwani/deadwood/git"
)

type viewID int

const (
	viewBranches viewID = iota
	viewWorktrees
)

type mode int

const (
	modeList mode = iota
	modeFilter
	modeDetail
	modeHelp
	modeDeleteScope
	modeDeleteConfirm
	modeRename
	modeRenameScope
	modeNewBranch
	modeArchiveConfirm
	modeWorktreePruneMerged
	modeWorktreeBranch
	modeWorktreePath
	modeWorktreeRemove
	modeWorktreePrune
)

// isInput reports whether the mode has the text input focused.
func (md mode) isInput() bool {
	switch md {
	case modeFilter, modeRename, modeNewBranch, modeWorktreeBranch, modeWorktreePath:
		return true
	}
	return false
}

type sortMode int

const (
	sortName sortMode = iota
	sortNewest
	sortOldest
	sortModes
)

func (s sortMode) String() string {
	return [...]string{"name", "newest first", "oldest first"}[s]
}

// sortNames are the values stored in git config between runs.
var sortNames = [...]string{"", "newest", "oldest"}

func parseSort(name string) sortMode {
	for i, n := range sortNames {
		if n == name {
			return sortMode(i)
		}
	}
	return sortName
}

// keyAction maps the action names used in `git config deadwood.keys.<action>`
// to the key each has by default. Keys are shared between the tabs, so
// remapping "delete" also moves "remove worktree".
var keyAction = map[string]string{
	"select": " ", "selectall": "ctrl+a", "selectmerged": "M", "selectstale": "S", "selectauthor": "A",
	"delete": "d", "archive": "a", "undo": "u", "rename": "r", "switch": "s", "worktree": "w",
	"new": "n", "push": "p", "pull": "P", "details": "enter", "filter": "/", "sort": "o",
	"lock": "l", "fetch": "F", "refresh": "ctrl+r", "help": "?", "quit": "q",
}

type statusKind int

const (
	statusInfo statusKind = iota
	statusSuccess
	statusError
)

const statusTimeout = 6 * time.Second

// deletedBranch is a branch deadwood removed, kept so it can be restored.
type deletedBranch struct {
	git.Branch
	remote   bool // its upstream branch was deleted too
	archived bool // an archive tag was created for it
}

type model struct {
	root string // worktree deadwood was started in
	opts git.Options

	view viewID
	mode mode

	branches      []git.Branch // everything git reported
	visible       []git.Branch // filtered and sorted; what the table shows
	worktrees     []git.Worktree
	branchTable   table.Model
	worktreeTable table.Model
	selected      map[string]struct{} // branch names
	filter        string
	sort          sortMode

	input   textinput.Model
	spinner spinner.Model
	busy    string // label of the git operation in flight

	status     string
	statusKind statusKind
	statusID   int

	// Pending operation, filled in while a prompt is open.
	targets      []git.Branch
	remoteDelete bool
	newName      string // rename target while asking about the remote
	wtBranch     string
	wtTarget     git.Worktree

	detailFor git.Branch
	detail    *git.Detail // nil while loading

	focus      string          // branch to move the cursor to once it is loaded
	undo       []deletedBranch // the last delete, restorable with u
	deleted    []deletedBranch // everything deleted and not restored this session
	chosenPath string          // worktree picked with enter, printed on exit

	// Remapped keys: default key -> custom key, and the reverse.
	keyTo   map[string]string
	keyFrom map[string]string

	width  int
	height int
}

type loadedMsg struct {
	branches  []git.Branch
	worktrees []git.Worktree
	err       error
}

type opDoneMsg struct {
	status   string
	failed   bool
	deleted  []deletedBranch
	restored bool   // the undo batch was put back
	focus    string // branch to put the cursor on after the reload
}

type detailMsg struct {
	name   string
	detail git.Detail
	err    error
}

type clearStatusMsg struct{ id int }

// viewConfig is what newModel takes from git config besides the options.
type viewConfig struct {
	sort   string
	filter string
	keys   map[string]string // action name -> custom key
}

func newModel(root string, opts git.Options, cfg viewConfig) model {
	newTable := func() table.Model {
		t := table.New(table.WithFocused(true))
		t.SetStyles(tableStyles())
		return t
	}
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 255
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	sp.Style = promptStyle
	keyTo, keyFrom := map[string]string{}, map[string]string{}
	for action, custom := range cfg.keys {
		def, ok := keyAction[strings.ToLower(action)]
		if custom == "space" {
			custom = " "
		}
		if !ok || custom == "" || custom == def {
			continue
		}
		keyTo[def], keyFrom[custom] = custom, def
	}
	return model{
		keyTo:         keyTo,
		keyFrom:       keyFrom,
		sort:          parseSort(cfg.sort),
		filter:        cfg.filter,
		root:          root,
		opts:          opts,
		branchTable:   newTable(),
		worktreeTable: newTable(),
		selected:      make(map[string]struct{}),
		input:         ti,
		spinner:       sp,
		busy:          "Loading",
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.loadCmd(), m.spinner.Tick)
}

func (m model) loadCmd() tea.Cmd {
	opts := m.opts
	return func() tea.Msg {
		branches, err := git.ListBranches(opts)
		if err != nil {
			return loadedMsg{err: err}
		}
		worktrees, err := git.ListWorktrees()
		return loadedMsg{branches: branches, worktrees: worktrees, err: err}
	}
}

// canonical translates a pressed key into the default key the handlers
// switch on. A default key that was remapped away does nothing.
func (m model) canonical(pressed string) string {
	if def, ok := m.keyFrom[pressed]; ok {
		return def
	}
	if _, moved := m.keyTo[pressed]; moved {
		return ""
	}
	return pressed
}

// k is the key to show in hints for an action's default key.
func (m model) k(def string) string {
	if custom, ok := m.keyTo[def]; ok {
		def = custom
	}
	if def == " " {
		return "space"
	}
	return def
}

// mainRoot is the main working tree, which new worktrees are placed next to.
func (m model) mainRoot() string {
	if len(m.worktrees) > 0 {
		return m.worktrees[0].Path
	}
	return m.root
}

func (m model) cursorBranch() (git.Branch, bool) {
	i := m.branchTable.Cursor()
	if i < 0 || i >= len(m.visible) {
		return git.Branch{}, false
	}
	return m.visible[i], true
}

func (m model) cursorWorktree() (git.Worktree, bool) {
	i := m.worktreeTable.Cursor()
	if i < 0 || i >= len(m.worktrees) {
		return git.Worktree{}, false
	}
	return m.worktrees[i], true
}

// actionBranches returns the selected branches, or the one under the cursor
// when nothing is selected.
func (m model) actionBranches() []git.Branch {
	var out []git.Branch
	for _, b := range m.visible {
		if _, ok := m.selected[b.Name]; ok {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		if b, ok := m.cursorBranch(); ok {
			out = append(out, b)
		}
	}
	return out
}

// deletable reports whether deadwood will delete the branch at all, and if
// not, why.
func deletable(b git.Branch) (bool, string) {
	switch {
	case b.Current:
		return false, "current branch"
	case b.Worktree != "":
		return false, "checked out in a worktree"
	case b.Protected:
		return false, "protected"
	}
	return true, ""
}

// unmergedCount is the number of pending delete targets whose commits are
// not in the base branch.
func (m model) unmergedCount() int {
	n := 0
	for _, b := range m.targets {
		if ok, _ := deletable(b); ok && b.Merge == git.NotMerged {
			n++
		}
	}
	return n
}

func (m model) prunableCount() int {
	n := 0
	for _, wt := range m.worktrees {
		if wt.Prunable {
			n++
		}
	}
	return n
}

// applyView recomputes the visible branches from the filter and sort order,
// and drops selections that are no longer visible.
func (m *model) applyView() {
	m.visible = m.visible[:0:0]
	for _, b := range m.branches {
		if matchesFilter(m.filter, b.Name) {
			m.visible = append(m.visible, b)
		}
	}
	switch m.sort {
	case sortNewest:
		sort.SliceStable(m.visible, func(i, j int) bool { return m.visible[i].Updated.After(m.visible[j].Updated) })
	case sortOldest:
		sort.SliceStable(m.visible, func(i, j int) bool { return m.visible[i].Updated.Before(m.visible[j].Updated) })
	}
	names := make(map[string]struct{}, len(m.visible))
	for _, b := range m.visible {
		names[b.Name] = struct{}{}
	}
	for name := range m.selected {
		if _, ok := names[name]; !ok {
			delete(m.selected, name)
		}
	}
}

// matchesFilter does a case-insensitive substring match, or a glob match
// against the whole name when the filter contains * or ?. Unlike a path
// glob, * also matches the slashes in branch names.
func matchesFilter(filter, name string) bool {
	filter, name = strings.ToLower(filter), strings.ToLower(name)
	if !strings.ContainsAny(filter, "*?") {
		return strings.Contains(name, filter)
	}
	pattern := regexp.QuoteMeta(filter)
	pattern = strings.ReplaceAll(pattern, `\*`, ".*")
	pattern = strings.ReplaceAll(pattern, `\?`, ".")
	ok, err := regexp.MatchString("^"+pattern+"$", name)
	return ok && err == nil
}

// mergedWorktrees are the worktrees that can go without losing anything:
// clean, unlocked, not the main or current one, on a merged branch.
func (m model) mergedWorktrees() []git.Worktree {
	var out []git.Worktree
	for _, wt := range m.worktrees {
		if wt.Main || wt.Path == m.root || wt.Locked || wt.Dirty || wt.Prunable || wt.Bare {
			continue
		}
		if b, ok := m.worktreeBranch(wt); ok && b.Merge != git.NotMerged {
			out = append(out, wt)
		}
	}
	return out
}

// selectWhere replaces the selection with the deletable visible branches
// that match, and returns how many there are.
func (m *model) selectWhere(match func(git.Branch) bool) int {
	m.selected = make(map[string]struct{})
	for _, b := range m.visible {
		if ok, _ := deletable(b); ok && match(b) {
			m.selected[b.Name] = struct{}{}
		}
	}
	return len(m.selected)
}

// colSpec describes a table column. Columns with flex share leftover width;
// a column is dropped when the content area is narrower than minTotal.
type colSpec struct {
	title    string
	width    int
	flex     int
	minTotal int
	ci       bool // only shown when CI status is enabled
}

var branchCols = []colSpec{
	{title: " ", width: 1},
	{title: "Branch", width: 16, flex: 3},
	{title: "State", width: 9},
	{title: "Sync", width: 8, minTotal: 48},
	{title: "CI", width: 7, minTotal: 60, ci: true},
	{title: "Upstream", width: 16, flex: 2, minTotal: 100},
	{title: "Commit", width: 7, minTotal: 64},
	{title: "Updated", width: 10, minTotal: 80},
	{title: "Author", width: 16, minTotal: 116},
	{title: "Subject", width: 20, flex: 3, minTotal: 132},
}

var worktreeCols = []colSpec{
	{title: "Path", width: 20, flex: 3},
	{title: "Branch", width: 14, flex: 2},
	{title: "Commit", width: 7, minTotal: 64},
	{title: "CI", width: 7, minTotal: 60, ci: true},
	{title: "State", width: 14, flex: 1},
}

const (
	cellPadding = 2 // horizontal padding the table adds to every cell
	minColWidth = 4
)

func layoutCols(specs []colSpec, total int, showCI bool) ([]table.Column, []bool) {
	visible := make([]bool, len(specs))
	used, flex := 0, 0
	for i, s := range specs {
		if total < s.minTotal || (s.ci && !showCI) {
			continue
		}
		visible[i] = true
		used += s.width + cellPadding
		flex += s.flex
	}
	// spare is negative on very narrow windows, which shrinks the flex
	// columns below their preferred width.
	spare := total - used
	var cols []table.Column
	for i, s := range specs {
		if !visible[i] {
			continue
		}
		w := s.width
		if flex > 0 {
			w += spare * s.flex / flex
		}
		if s.flex > 0 && w < minColWidth {
			w = minColWidth
		}
		cols = append(cols, table.Column{Title: s.title, Width: w})
	}
	return cols, visible
}

func pick(cells []string, visible []bool) table.Row {
	row := make(table.Row, 0, len(cells))
	for i, c := range cells {
		if visible[i] {
			row = append(row, c)
		}
	}
	return row
}

// stateLabel is the single most relevant fact about a branch.
func stateLabel(b git.Branch) string {
	switch {
	case b.Current:
		return "current"
	case b.Worktree != "":
		return "worktree"
	case b.Protected:
		return "protected"
	case b.Merge != git.NotMerged:
		return string(b.Merge)
	case b.Stale:
		return "stale"
	}
	return ""
}

func syncLabel(b git.Branch) string {
	switch {
	case b.Upstream == "":
		return "local"
	case b.Gone:
		return "gone"
	case b.Ahead > 0 && b.Behind > 0:
		return fmt.Sprintf("↑%d ↓%d", b.Ahead, b.Behind)
	case b.Ahead > 0:
		return fmt.Sprintf("↑%d", b.Ahead)
	case b.Behind > 0:
		return fmt.Sprintf("↓%d", b.Behind)
	}
	return "synced"
}

func (m model) branchRow(b git.Branch) []string {
	mark := ""
	if _, ok := m.selected[b.Name]; ok {
		mark = "●"
	}
	return []string{mark, b.Name, stateLabel(b), syncLabel(b), b.CI, b.Upstream, b.Commit, shortAge(time.Since(b.Updated)), b.Author, b.Subject}
}

func (m model) worktreeRow(wt git.Worktree) []string {
	var state []string
	if wt.Main {
		state = append(state, "main")
	}
	if wt.Path == m.root {
		state = append(state, "here")
	}
	if wt.Bare {
		state = append(state, "bare")
	}
	if wt.Locked {
		state = append(state, "locked")
	}
	if wt.Prunable {
		state = append(state, "missing")
	}
	if wt.Dirty {
		state = append(state, "dirty")
	}
	branch := wt.Branch
	if wt.Detached {
		branch = "(detached)"
	}
	ci := ""
	if b, ok := m.worktreeBranch(wt); ok {
		ci = b.CI
	}
	return []string{m.inputPath(wt.Path), branch, wt.Commit, ci, strings.Join(state, " · ")}
}

// refreshTables rebuilds columns and rows of both tables for the current
// data, filter, selection and window size.
func (m *model) refreshTables() {
	m.applyView()
	total := m.width - appStyle.GetHorizontalFrameSize()
	height := m.bodyHeight()

	cols, visible := layoutCols(branchCols, total, m.opts.CI)
	rows := make([]table.Row, len(m.visible))
	for i, b := range m.visible {
		rows[i] = pick(m.branchRow(b), visible)
	}
	setTable(&m.branchTable, cols, rows, total, height)

	cols, visible = layoutCols(worktreeCols, total, m.opts.CI)
	rows = make([]table.Row, len(m.worktrees))
	for i, wt := range m.worktrees {
		cells := m.worktreeRow(wt)
		// Keep the end of a long path, which is the part that identifies it.
		cells[0] = truncateLeft(cells[0], cols[0].Width)
		rows[i] = pick(cells, visible)
	}
	setTable(&m.worktreeTable, cols, rows, total, height)
}

func setTable(t *table.Model, cols []table.Column, rows []table.Row, width, height int) {
	// Rows are cleared first: the table renders on every setter and would
	// index past the column list while the two disagree.
	cursor := t.Cursor()
	t.SetRows(nil)
	t.SetColumns(cols)
	t.SetWidth(width)
	t.SetHeight(height)
	t.SetRows(rows)
	if cursor >= len(rows) {
		cursor = len(rows) - 1
	}
	if cursor < 0 {
		cursor = 0
	}
	t.SetCursor(cursor)
}

func (m *model) setStatus(kind statusKind, text string) tea.Cmd {
	m.statusID++
	m.status = text
	m.statusKind = kind
	id := m.statusID
	return tea.Tick(statusTimeout, func(time.Time) tea.Msg { return clearStatusMsg{id} })
}

// startInput opens a text prompt in the given mode.
func (m *model) startInput(md mode, placeholder, value string) tea.Cmd {
	m.mode = md
	m.status = ""
	m.input.Placeholder = placeholder
	m.input.SetValue(value)
	m.input.CursorEnd()
	return m.input.Focus()
}

func (m *model) cancel() {
	m.mode = modeList
	m.input.Blur()
	m.targets = nil
	m.status = ""
}

// run puts the UI into its busy state while op executes.
func (m *model) run(label string, op func() opDoneMsg) tea.Cmd {
	m.mode = modeList
	m.input.Blur()
	m.busy = label
	m.status = ""
	return tea.Batch(func() tea.Msg { return op() }, m.spinner.Tick)
}

// inputPath is the editable form of a path used to prefill prompts:
// relative to the directory deadwood runs in when that is short, otherwise
// ~-abbreviated. It always resolves back to the same location.
func (m model) inputPath(p string) string {
	rel, err := filepath.Rel(m.root, p)
	if err != nil || rel == "." || strings.Count(rel, "..") > 1 {
		return tildePath(p)
	}
	return rel
}

// showPath is inputPath for display only: long paths keep just their last
// two elements.
func (m model) showPath(p string) string {
	const maxLen = 48
	s := m.inputPath(p)
	if len([]rune(s)) <= maxLen {
		return s
	}
	return "…/" + filepath.Join(filepath.Base(filepath.Dir(p)), filepath.Base(p))
}

// shortAge renders a duration the way the Updated column shows it, e.g.
// "5h ago", "3w ago" or "1y 9mo ago".
func shortAge(d time.Duration) string {
	const day = 24 * time.Hour
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", d/time.Minute)
	case d < day:
		return fmt.Sprintf("%dh ago", d/time.Hour)
	case d < 14*day:
		return fmt.Sprintf("%dd ago", d/day)
	case d < 60*day:
		return fmt.Sprintf("%dw ago", d/(7*day))
	case d < 365*day:
		return fmt.Sprintf("%dmo ago", d/(30*day))
	}
	years, months := d/(365*day), (d%(365*day))/(30*day)
	if months == 0 || months > 11 {
		return fmt.Sprintf("%dy ago", years)
	}
	return fmt.Sprintf("%dy %dmo ago", years, months)
}

func truncateLeft(s string, width int) string {
	if r := []rune(s); len(r) > width && width > 1 {
		return "…" + string(r[len(r)-width+1:])
	}
	return s
}

func tildePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// plural formats a count with "branch", "entry" or "worktree".
func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	switch {
	case strings.HasSuffix(word, "y"):
		word = word[:len(word)-1] + "ies"
	case strings.HasSuffix(word, "h"):
		word += "es"
	default:
		word += "s"
	}
	return fmt.Sprintf("%d %s", n, word)
}
