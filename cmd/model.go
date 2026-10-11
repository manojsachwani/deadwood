package main

import (
	"fmt"
	"os"
	"path/filepath"
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
	modeDeleteScope
	modeDeleteConfirm
	modeRename
	modeWorktreeBranch
	modeWorktreePath
	modeWorktreeRemove
	modeWorktreePrune
)

type statusKind int

const (
	statusInfo statusKind = iota
	statusSuccess
	statusError
)

const statusTimeout = 6 * time.Second

type model struct {
	root string // worktree deadwood was started in

	view viewID
	mode mode

	branches      []git.Branch
	worktrees     []git.Worktree
	branchTable   table.Model
	worktreeTable table.Model
	selected      map[string]struct{} // branch names

	input   textinput.Model
	spinner spinner.Model
	busy    string // label of the git operation in flight

	status     string
	statusKind statusKind
	statusID   int

	// Pending operation, filled in while a prompt is open.
	targets      []git.Branch
	remoteDelete bool
	wtBranch     string
	wtTarget     git.Worktree

	width  int
	height int
}

type loadedMsg struct {
	branches  []git.Branch
	worktrees []git.Worktree
	err       error
}

type opDoneMsg struct {
	status string
	failed bool
}

type clearStatusMsg struct{ id int }

func newModel(root string) model {
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
	return model{
		root:          root,
		branchTable:   newTable(),
		worktreeTable: newTable(),
		selected:      make(map[string]struct{}),
		input:         ti,
		spinner:       sp,
		busy:          "Loading",
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(loadCmd, m.spinner.Tick)
}

func loadCmd() tea.Msg {
	branches, err := git.ListBranches()
	if err != nil {
		return loadedMsg{err: err}
	}
	worktrees, err := git.ListWorktrees()
	return loadedMsg{branches: branches, worktrees: worktrees, err: err}
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
	if i < 0 || i >= len(m.branches) {
		return git.Branch{}, false
	}
	return m.branches[i], true
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
	for _, b := range m.branches {
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

// unmergedCount is the number of pending delete targets with commits that
// are not on the current branch.
func (m model) unmergedCount() int {
	n := 0
	for _, b := range m.targets {
		if !b.Merged && !b.Current {
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

// colSpec describes a table column. Columns with flex share leftover width;
// a column is dropped when the content area is narrower than minTotal.
type colSpec struct {
	title    string
	width    int
	flex     int
	minTotal int
}

var branchCols = []colSpec{
	{title: " ", width: 1},
	{title: "Branch", width: 16, flex: 3},
	{title: "State", width: 8},
	{title: "Sync", width: 8, minTotal: 48},
	{title: "Upstream", width: 16, flex: 2, minTotal: 100},
	{title: "Commit", width: 7, minTotal: 64},
	{title: "Updated", width: 14, minTotal: 80},
	{title: "Author", width: 16, minTotal: 116},
	{title: "Subject", width: 20, flex: 3, minTotal: 132},
}

var worktreeCols = []colSpec{
	{title: "Path", width: 20, flex: 3},
	{title: "Branch", width: 14, flex: 2},
	{title: "Commit", width: 7, minTotal: 64},
	{title: "State", width: 14, flex: 1},
}

const (
	cellPadding = 2 // horizontal padding the table adds to every cell
	minColWidth = 4
)

func layoutCols(specs []colSpec, total int) ([]table.Column, []bool) {
	visible := make([]bool, len(specs))
	used, flex := 0, 0
	for i, s := range specs {
		if total < s.minTotal {
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

func (m model) branchRow(b git.Branch) []string {
	mark := ""
	if _, ok := m.selected[b.Name]; ok {
		mark = "●"
	}
	state := ""
	switch {
	case b.Current:
		state = "current"
	case b.Worktree != "":
		state = "worktree"
	case b.Merged:
		state = "merged"
	}
	sync := "local"
	switch {
	case b.Upstream == "":
	case b.Gone:
		sync = "gone"
	case b.Ahead > 0 && b.Behind > 0:
		sync = fmt.Sprintf("↑%d ↓%d", b.Ahead, b.Behind)
	case b.Ahead > 0:
		sync = fmt.Sprintf("↑%d", b.Ahead)
	case b.Behind > 0:
		sync = fmt.Sprintf("↓%d", b.Behind)
	default:
		sync = "synced"
	}
	return []string{mark, b.Name, state, sync, b.Upstream, b.Commit, b.Age, b.Author, b.Subject}
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
	return []string{m.inputPath(wt.Path), branch, wt.Commit, strings.Join(state, " · ")}
}

// refreshTables rebuilds columns and rows of both tables for the current
// data, selection and window size.
func (m *model) refreshTables() {
	total := m.width - appStyle.GetHorizontalFrameSize()
	height := m.height - m.chrome()
	if height < 3 {
		height = 3
	}

	cols, visible := layoutCols(branchCols, total)
	rows := make([]table.Row, len(m.branches))
	for i, b := range m.branches {
		rows[i] = pick(m.branchRow(b), visible)
	}
	setTable(&m.branchTable, cols, rows, total, height)

	cols, visible = layoutCols(worktreeCols, total)
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

// run puts the UI into its busy state while cmd executes.
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

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	if strings.HasSuffix(word, "h") || strings.HasSuffix(word, "y") {
		if strings.HasSuffix(word, "y") {
			return fmt.Sprintf("%d %sies", n, word[:len(word)-1])
		}
		return fmt.Sprintf("%d %ses", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
