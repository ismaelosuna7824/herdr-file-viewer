// Package ui composes the file browser, fuzzy finder, content search, git log
// and file viewer into a single Bubble Tea program following the
// Model-Update-View architecture. The domain packages (explorer, finder,
// search, gitlog, gitstatus, gitdiff, viewer) know nothing about Bubble Tea;
// this package is the only place they meet.
//
// The browse screen is a fixed multi-panel layout: the file tree and file view
// on top, and two always-visible docked panels below — a search panel (fuzzy
// file find / content search) on the left and the git-log panel on the right.
// Tab cycles focus through the four panels.
package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ismaelosuna7824/herdr-file-viewer/internal/editor"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/explorer"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitdiff"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitlog"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/gitstatus"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/herdr"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/reveal"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/search"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/update"
	"github.com/ismaelosuna7824/herdr-file-viewer/internal/viewer"
)

// version is the running build's version, injected via -ldflags and set from
// main. "dev" (the default) suppresses the update check.
var version = "dev"

// SetVersion records the running build version (called from main at startup).
func SetVersion(v string) {
	if v != "" {
		version = v
	}
}

type mode int

const (
	modeBrowse mode = iota
	modeDiff
	modeHelp
)

type browseFocus int

const (
	focusExplorer browseFocus = iota
	focusViewer
	focusSearch
	focusLog
)

// searchKind selects what the left docked panel searches.
type searchKind int

const (
	kindFind    searchKind = iota // fuzzy file finder
	kindContent                   // content search across files
)

const explorerWidth = 34

// Model is the root application state.
type Model struct {
	root          string
	width         int
	height        int
	ready         bool
	treeStatePath string
	st            styles

	mode   mode
	bfocus browseFocus
	skind  searchKind

	tree   *explorer.Tree
	viewer viewer.Model
	finder finderPanel
	search searchPanel
	diff   diffPanel
	log    logPanel

	git          gitstatus.Status
	currentFile  string       // absolute path of the file shown in the viewer
	diffReturn   mode         // where Esc from the diff view returns to
	diffRel      string       // slash path of the file in the review view
	diffMode     gitdiff.Mode // which boundary the review view shows (for hunk staging)
	updateLatest string       // newer release tag, if one is available
	statusNote   string       // transient footer note (e.g. "no editor set")

	// Editor picker overlay (shown when no default editor is configured).
	pickerActive  bool
	pickerEditors []editor.Editor
	pickerCursor  int
	pickerTarget  string // file or project dir to open
	pickerLabel   string // what's being opened, for the title

	// Git-operation overlays (new branch / commit prompt, and confirm dialogs).
	prompt      gitOp
	promptInput textinput.Model
	confirm     gitOp
	confirmName string // branch involved in a confirm dialog

	searchGen    int
	cancel       context.CancelFunc
	highlightGen int
}

// gitOp identifies a pending git operation driven by a prompt or confirm dialog.
type gitOp int

const (
	opNone gitOp = iota
	opNewBranch
	opCommit
	opMerge
	opRebase
	opDelete
	opForcePush
	opAmend
	opUndo
	opTag
	opCherryPick
	opResetHardHEAD
	opResetHardCommit
)

// --- messages ---------------------------------------------------------------

type filesLoadedMsg struct{ files []string }

type gitStatusMsg struct{ status gitstatus.Status }

type gitLogMsg struct{ commits []gitlog.Commit }

type gitBranchesMsg struct{ branches []gitlog.Branch }

type gitChangesMsg struct{ changes []gitstatus.Change }

type branchSwitchedMsg struct{ err error }

// gitOpDoneMsg carries the result of a create/commit/merge/rebase/delete op.
type gitOpDoneMsg struct{ err error }

type diffLoadedMsg struct{ diff gitdiff.FileDiff }

// diffReloadedMsg carries the re-fetched diff after a hunk was staged. Unlike
// diffLoadedMsg it lets the review view pop back to the list when nothing
// remains to stage.
type diffReloadedMsg struct{ diff gitdiff.FileDiff }

// hunkStagedMsg reports the result of staging/unstaging a single hunk.
type hunkStagedMsg struct{ err error }

// updateMsg carries a newer release tag (empty = up to date).
type updateMsg struct{ latest string }

// editorClosedMsg fires when the external editor exits (err set if it failed to
// launch, e.g. the command isn't on PATH).
type editorClosedMsg struct{ err error }

// fileTabOpenedMsg reports the result of opening a file in a Herdr tab.
type fileTabOpenedMsg struct{ err error }

// diffTabOpenedMsg reports the result of opening a diff review in a Herdr tab.
type diffTabOpenedMsg struct{ err error }

// highlightMsg carries the result of asynchronous syntax highlighting.
type highlightMsg struct {
	gen   int
	path  string
	lines []string
}

type searchResultMsg struct {
	gen int
	res search.Result
	err error
}

// New constructs the application rooted at the given directory.
func New(root string) (Model, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Model{}, err
	}
	tree, err := explorer.New(abs)
	if err != nil {
		return Model{}, err
	}
	model := Model{
		root:          abs,
		treeStatePath: treeStatePathFromEnv(),
		st:            newStyles(),
		tree:          tree,
		viewer:        viewer.New(),
		finder:        newFinderPanel(),
		search:        newSearchPanel(),
		diff:          newDiffPanel(),
		log:           newLogPanel(),
		promptInput:   newPromptInput(),
	}
	model.restoreTreeState()
	return model, nil
}

// Init kicks off the background file index, git status scan, and git log load.
func (m Model) Init() tea.Cmd {
	return tea.Batch(loadFilesCmd(m.root), loadGitStatusCmd(m.root),
		loadGitLogCmd(m.root), loadBranchesCmd(m.root), loadChangesCmd(m.root),
		checkUpdateCmd(), tickCmd())
}

// --- commands ---------------------------------------------------------------

func loadFilesCmd(root string) tea.Cmd {
	return func() tea.Msg {
		files, _ := search.ListFiles(context.Background(), root, 50000)
		return filesLoadedMsg{files: files}
	}
}

func loadGitStatusCmd(root string) tea.Cmd {
	return func() tea.Msg {
		return gitStatusMsg{status: gitstatus.Load(context.Background(), root)}
	}
}

func loadDiffCmd(root, rel string, untracked bool) tea.Cmd {
	return func() tea.Msg {
		return diffLoadedMsg{diff: gitdiff.Load(context.Background(), root, rel, untracked)}
	}
}

// loadDiffModeCmd fetches a specific Source Control boundary (used by the
// interactive review view so its hunks are exactly the ones that can be staged).
func loadDiffModeCmd(root, rel string, mode gitdiff.Mode) tea.Cmd {
	return func() tea.Msg {
		return diffLoadedMsg{diff: gitdiff.LoadMode(context.Background(), root, rel, mode)}
	}
}

// loadDiffReloadCmd re-fetches the review diff after a hunk was staged, tagged
// so the view can return to the staging list once nothing remains.
func loadDiffReloadCmd(root, rel string, mode gitdiff.Mode) tea.Cmd {
	return func() tea.Msg {
		return diffReloadedMsg{diff: gitdiff.LoadMode(context.Background(), root, rel, mode)}
	}
}

func loadGitLogCmd(root string) tea.Cmd {
	return func() tea.Msg {
		return gitLogMsg{commits: gitlog.Load(context.Background(), root, 200)}
	}
}

func loadBranchesCmd(root string) tea.Cmd {
	return func() tea.Msg {
		return gitBranchesMsg{branches: gitlog.Branches(context.Background(), root)}
	}
}

func loadChangesCmd(root string) tea.Cmd {
	return func() tea.Msg {
		return gitChangesMsg{changes: gitstatus.Changes(context.Background(), root)}
	}
}

// checkUpdateCmd asks GitHub for the latest release and reports it if it's newer
// than the running build. Best-effort: errors just yield "no update".
func checkUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		latest, err := update.Latest(context.Background())
		if err != nil || !update.IsNewer(latest, version) {
			return updateMsg{}
		}
		return updateMsg{latest: latest}
	}
}

func switchBranchCmd(root, name string) tea.Cmd {
	return func() tea.Msg {
		return branchSwitchedMsg{err: gitlog.Switch(context.Background(), root, name)}
	}
}

// gitOpCmd runs a git-mutating operation off the UI thread.
func gitOpCmd(op func() error) tea.Cmd {
	return func() tea.Msg {
		return gitOpDoneMsg{err: op()}
	}
}

// runGitOp is a convenience for the no-argument git operations (fetch, pull,
// push, stash, …): it clears the last error and runs fn against the root.
func (m *Model) runGitOp(fn func(context.Context, string) error) tea.Cmd {
	m.log.branchErr = ""
	root := m.root
	return gitOpCmd(func() error { return fn(context.Background(), root) })
}

// autoRefreshInterval is how often the viewer polls disk/git for changes so new
// files, edits and staging updates appear without a manual refresh.
const autoRefreshInterval = 2 * time.Second

type tickMsg struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(autoRefreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// autoRefreshCmd reloads the cheap "what changed on disk" signals every tick:
// git status, the staging list, and the file index. History and branches change
// rarely, so they refresh on `r` and after git operations instead.
func autoRefreshCmd(root string) tea.Cmd {
	return tea.Batch(loadGitStatusCmd(root), loadChangesCmd(root), loadFilesCmd(root))
}

// reloadGitCmd re-scans status, history, branches, and the file index — used
// after a branch switch or a manual refresh.
func reloadGitCmd(root string) tea.Cmd {
	return tea.Batch(
		loadGitStatusCmd(root),
		loadGitLogCmd(root),
		loadBranchesCmd(root),
		loadChangesCmd(root),
		loadFilesCmd(root),
	)
}

func loadCommitDiffCmd(root, hash, label string) tea.Cmd {
	return func() tea.Msg {
		return diffLoadedMsg{diff: gitdiff.LoadRef(context.Background(), root, hash, label)}
	}
}

func runSearchCmd(ctx context.Context, gen int, root string, opts search.Options) tea.Cmd {
	return func() tea.Msg {
		res, err := search.Search(ctx, root, opts)
		if ctx.Err() != nil {
			return searchResultMsg{gen: gen, err: ctx.Err()}
		}
		return searchResultMsg{gen: gen, res: res, err: err}
	}
}

// --- update -----------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		m.layout()
		return m, nil

	case filesLoadedMsg:
		m.finder.setFiles(msg.files)
		return m, nil

	case gitStatusMsg:
		m.git = msg.status
		return m, nil

	case gitLogMsg:
		m.log.setCommits(msg.commits)
		return m, nil

	case gitBranchesMsg:
		m.log.setBranches(msg.branches)
		return m, nil

	case gitChangesMsg:
		m.log.setChanges(msg.changes)
		return m, nil

	case branchSwitchedMsg:
		if msg.err != nil {
			m.log.branchErr = msg.err.Error()
			return m, nil
		}
		// Branch changed: everything git-derived is now stale.
		m.log.branchErr = ""
		return m, reloadGitCmd(m.root)

	case gitOpDoneMsg:
		if msg.err != nil {
			m.log.branchErr = msg.err.Error()
			return m, nil
		}
		m.log.branchErr = ""
		return m, reloadGitCmd(m.root)

	case diffLoadedMsg:
		m.diff.SetDiff(msg.diff)
		return m, nil

	case diffReloadedMsg:
		m.diff.SetDiff(msg.diff)
		// Nothing left to stage in this file — drop back to the staging list.
		if m.mode == modeDiff && (msg.diff.Empty || m.diff.hunkCount() == 0) {
			m.mode = m.diffReturn
		}
		return m, nil

	case hunkStagedMsg:
		if msg.err != nil {
			m.statusNote = "stage failed: " + msg.err.Error()
			return m, nil
		}
		m.statusNote = ""
		// Refresh the review diff (same file+mode) and the git/staging signals.
		return m, tea.Batch(
			loadDiffReloadCmd(m.root, m.diffRel, m.diffMode),
			reloadGitCmd(m.root),
		)

	case updateMsg:
		m.updateLatest = msg.latest
		return m, nil

	case editorClosedMsg:
		if msg.err != nil {
			m.statusNote = "editor failed: " + msg.err.Error()
			return m, nil
		}
		// The file may have changed in the editor — reload the view and git.
		m.tree.Refresh()
		var cmd tea.Cmd
		if m.currentFile != "" {
			cmd = m.showFile(m.currentFile)
		}
		return m, tea.Batch(cmd, reloadGitCmd(m.root))

	case fileTabOpenedMsg:
		if msg.err != nil {
			m.statusNote = "file tab failed: " + msg.err.Error()
		}
		return m, nil

	case diffTabOpenedMsg:
		if msg.err != nil {
			m.statusNote = "diff tab failed: " + msg.err.Error()
		}
		return m, nil

	case highlightMsg:
		if msg.gen == m.highlightGen {
			m.viewer.SetHighlighted(msg.path, msg.lines)
		}
		return m, nil

	case searchResultMsg:
		if msg.gen == m.searchGen {
			if msg.err != nil && msg.err != context.Canceled {
				m.search.setResult(search.Result{}, msg.err)
			} else if msg.err == nil {
				m.search.setResult(msg.res, nil)
			}
		}
		return m, nil

	case tickMsg:
		// Live refresh: re-read the tree from disk and reload git/file signals,
		// then schedule the next tick.
		m.tree.Refresh()
		return m, tea.Batch(autoRefreshCmd(m.root), tickCmd())

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	return m.forwardToActive(msg)
}

// handleMouse maps a left click in the visible explorer region back to the
// exact tree row. Directories toggle; files open as read-only Herdr tabs. Every
// other mouse event — wheel scroll, drag, or any event outside plain browse mode
// (e.g. the inline diff view or an overlay) — is forwarded to the active panel
// so its viewport handles scrolling. This keeps click-to-open composed with
// native mouse-wheel scrolling.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.ready || m.mode != modeBrowse || m.pickerActive || m.prompt != opNone || m.confirm != opNone {
		return m.forwardToActive(msg)
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m.forwardToActive(msg)
	}
	topH, _, _, _ := m.browseDims()
	if topH < 1 {
		topH = 1
	}
	row := msg.Y - 1 // terminal row 0 is the app header
	if msg.X < 0 || msg.X >= explorerWidth || row < 0 || row >= topH {
		return m.forwardToActive(msg)
	}
	nodes := m.tree.Visible()
	index := scrollStart(m.tree.Cursor(), len(nodes), topH) + row
	if index < 0 || index >= len(nodes) {
		return m, nil
	}
	m.tree.SetCursor(index)
	m.bfocus = focusExplorer
	n := m.tree.Selected()
	if n == nil {
		m.persistTreeState()
		return m, nil
	}
	if n.IsDir {
		m.tree.Toggle()
		m.persistTreeState()
		return m, nil
	}
	m.persistTreeState()
	return m, m.openSelectedFileTab()
}

// openSelectedFileTab opens the selected file as a read-only Herdr tab with the
// project tree attached on the right. Deduplication and locking live in herdr.
func (m Model) openSelectedFileTab() tea.Cmd {
	n := m.tree.Selected()
	if n == nil || n.IsDir {
		return nil
	}
	workspaceID := os.Getenv("HERDR_WORKSPACE_ID")
	path := n.Path
	root := m.root
	return func() tea.Msg {
		return fileTabOpenedMsg{err: herdr.OpenFileTab(workspaceID, path, root)}
	}
}

// openDiffTab opens the selected changed file's diff as a read-only Herdr Diff
// tab. Deduplication and locking (per path+mode) live in herdr.
func (m Model) openDiffTab(change gitstatus.Change) tea.Cmd {
	workspaceID := os.Getenv("HERDR_WORKSPACE_ID")
	path := filepath.Join(m.root, filepath.FromSlash(change.Path))
	root := m.root
	mode := changeMode(change)
	return func() tea.Msg {
		return diffTabOpenedMsg{err: herdr.OpenDiffTab(workspaceID, path, root, mode)}
	}
}

// changeMode maps a git status entry to the diff boundary to review: untracked
// files show the whole file, staged files show the index diff, and everything
// else shows the worktree diff.
func changeMode(c gitstatus.Change) gitdiff.Mode {
	switch {
	case c.Untracked():
		return gitdiff.ModeUntracked
	case c.Staged():
		return gitdiff.ModeStaged
	default:
		return gitdiff.ModeWorktree
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	// Overlays take priority over everything else.
	if m.pickerActive {
		return m.handlePickerKey(msg)
	}
	if m.prompt != opNone {
		return m.handlePromptKey(msg)
	}
	if m.confirm != opNone {
		return m.handleConfirmKey(msg)
	}
	switch m.mode {
	case modeDiff:
		return m.handleDiffKey(msg)
	case modeHelp:
		m.mode = modeBrowse // any key closes help
		return m, nil
	default:
		return m.handleBrowseKey(msg)
	}
}

func newPromptInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	return ti
}

// startPrompt opens the text-input overlay for a create/commit operation.
func (m *Model) startPrompt(op gitOp, placeholder string) tea.Cmd {
	m.prompt = op
	m.promptInput.SetValue("")
	m.promptInput.Placeholder = placeholder
	return m.promptInput.Focus()
}

func (m Model) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.prompt = opNone
		m.promptInput.Blur()
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.promptInput.Value())
		op := m.prompt
		m.prompt = opNone
		m.promptInput.Blur()
		if val == "" {
			return m, nil
		}
		m.log.branchErr = ""
		root := m.root
		switch op {
		case opNewBranch:
			return m, gitOpCmd(func() error { return gitlog.CreateBranch(context.Background(), root, val) })
		case opCommit:
			// Commit only what's staged — never `git add -A`. Stage with space/A.
			return m, gitOpCmd(func() error { return gitlog.CommitStaged(context.Background(), root, val) })
		case opTag:
			return m, gitOpCmd(func() error { return gitlog.CreateTag(context.Background(), root, val) })
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.promptInput, cmd = m.promptInput.Update(msg)
	return m, cmd
}

func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		op := m.confirm
		name := m.confirmName
		m.confirm = opNone
		m.log.branchErr = ""
		root := m.root
		switch op {
		case opMerge:
			return m, gitOpCmd(func() error { return gitlog.Merge(context.Background(), root, name) })
		case opRebase:
			return m, gitOpCmd(func() error { return gitlog.Rebase(context.Background(), root, name) })
		case opDelete:
			return m, gitOpCmd(func() error { return gitlog.DeleteBranch(context.Background(), root, name) })
		case opForcePush:
			return m, m.runGitOp(gitlog.ForcePush)
		case opAmend:
			return m, m.runGitOp(gitlog.Amend)
		case opUndo:
			return m, m.runGitOp(gitlog.UndoLastCommit)
		case opCherryPick:
			m.log.branchErr = ""
			return m, gitOpCmd(func() error { return gitlog.CherryPick(context.Background(), root, name) })
		case opResetHardCommit:
			m.log.branchErr = ""
			return m, gitOpCmd(func() error { return gitlog.ResetHard(context.Background(), root, name) })
		case opResetHardHEAD:
			return m, m.runGitOp(func(ctx context.Context, r string) error { return gitlog.ResetHard(ctx, r, "HEAD") })
		}
		return m, nil
	default: // n, esc, anything else cancels
		m.confirm = opNone
		return m, nil
	}
}

func (m Model) handleBrowseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.statusNote = "" // transient — cleared on the next keypress

	// Directional panel movement (Alt+h/j/k/l and Alt+arrows) works from any
	// panel, including while the search input is focused.
	if dir, ok := focusDirection(msg.String()); ok {
		return m, m.moveFocus(dir)
	}

	// The search and git panels own the keyboard while focused, so typed letters
	// never trigger the single-key shortcuts below.
	if m.bfocus == focusSearch {
		return m.handleSearchFocus(msg)
	}
	if m.bfocus == focusLog {
		return m.handleLogFocus(msg)
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "ctrl+p":
		m.skind = kindFind
		return m, m.setFocus(focusSearch)
	case "ctrl+f":
		m.skind = kindContent
		return m, tea.Batch(m.setFocus(focusSearch), m.triggerSearch())
	case "r":
		m.tree.Refresh()
		return m, reloadGitCmd(m.root)
	case "d":
		if target := m.diffTarget(); target != "" {
			return m.openDiff(target)
		}
		return m, nil
	case "g":
		return m, m.setFocus(focusLog)
	case "L":
		// Locate: jump the tree to the file currently shown and focus it there.
		if m.currentFile != "" {
			m.tree.RevealPath(m.currentFile)
			m.bfocus = focusExplorer
		}
		return m, nil
	case "o":
		// Open the file's location in the OS file manager (Finder/Explorer/…).
		if t := m.revealTarget(); t != "" {
			return m, revealCmd(t)
		}
		return m, nil
	case "e":
		// Open the selected/open file in a configured editor.
		return m.openInEditor(m.editTarget(), false)
	case "E":
		// Open the whole project (root) in a configured editor.
		return m.openInEditor(m.root, true)
	case "?":
		m.mode = modeHelp
		return m, nil
	case "m":
		m.viewer.ToggleMarkdown()
		return m, m.highlightCmd() // highlight the source view when toggled off
	case "tab":
		return m, m.cycleFocus()
	}

	// File view focused: arrows scroll it; one keystroke returns to the tree.
	if m.bfocus == focusViewer {
		switch msg.String() {
		case "esc", "left", "h":
			return m, m.setFocus(focusExplorer)
		}
		var cmd tea.Cmd
		m.viewer, cmd = m.viewer.Update(msg)
		return m, cmd
	}

	// Explorer navigation with live preview.
	switch msg.String() {
	case "up", "k":
		m.tree.MoveUp()
		return m, m.previewSelected()
	case "down", "j":
		m.tree.MoveDown()
		return m, m.previewSelected()
	case "left", "h":
		if n := m.tree.Selected(); n != nil && n.IsDir && n.Expanded {
			m.tree.Toggle()
		}
	case "right", "l":
		n := m.tree.Selected()
		if n != nil && n.IsDir && !n.Expanded {
			m.tree.Toggle()
		} else if n != nil && !n.IsDir {
			cmd := m.showFile(n.Path)
			m.bfocus = focusViewer
			return m, cmd
		}
	case "enter", " ":
		if n := m.tree.Selected(); n != nil {
			if n.IsDir {
				m.tree.Toggle()
			} else {
				cmd := m.showFile(n.Path)
				m.bfocus = focusViewer
				return m, cmd
			}
		}
	}
	return m, nil
}

// handleSearchFocus drives the left docked panel while it holds focus.
func (m Model) handleSearchFocus(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, m.setFocus(focusExplorer)
	case "tab":
		return m, m.cycleFocus()
	case "ctrl+p": // toggle to fuzzy file find
		m.skind = kindFind
		return m, m.setFocus(focusSearch)
	case "ctrl+f": // toggle to content search
		m.skind = kindContent
		return m, tea.Batch(m.setFocus(focusSearch), m.triggerSearch())
	case "up", "ctrl+k":
		m.searchMoveUp()
		return m, nil
	case "down", "ctrl+j":
		m.searchMoveDown()
		return m, nil
	case "enter":
		// Only on enter do we touch the file view / tree — never live, so the
		// left panels don't flicker while you type.
		cmd := m.openSearchSelection()
		m.bfocus = focusViewer
		m.finder.blur()
		m.search.blur()
		return m, cmd
	}

	if m.skind == kindContent {
		// Ctrl combos work everywhere (Mac's Option key can't send Alt in a
		// terminal); Alt aliases are kept for Linux/Windows.
		switch msg.String() {
		case "ctrl+t", "alt+c":
			m.search.caseSensitive = !m.search.caseSensitive
			return m, m.triggerSearch()
		case "ctrl+w", "alt+w":
			m.search.wholeWord = !m.search.wholeWord
			return m, m.triggerSearch()
		case "ctrl+r", "alt+r":
			m.search.regex = !m.search.regex
			return m, m.triggerSearch()
		}
		var cmd tea.Cmd
		m.search.input, cmd = m.search.input.Update(msg)
		return m, tea.Batch(cmd, m.triggerSearch())
	}

	var cmd tea.Cmd
	m.finder.input, cmd = m.finder.input.Update(msg)
	m.finder.refresh()
	m.finder.resetCursor() // new query → jump to the top match
	return m, cmd
}

// handleLogFocus drives the git panel while focused: navigate, toggle
// history/branches, and run git operations from the branch view.
func (m Model) handleLogFocus(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, m.setFocus(focusExplorer)
	case "tab":
		return m, m.cycleFocus()
	case "g":
		m.log.toggleKind()
		return m, nil
	case "up", "k":
		m.log.moveUp()
		return m, nil
	case "down", "j":
		m.log.moveDown()
		return m, nil
	case "?":
		m.mode = modeHelp
		return m, nil
	case "enter":
		switch m.log.kind {
		case logChanges:
			// View the selected file's working diff (directories do nothing).
			if r, ok := m.log.selectedRow(); ok && !r.isDir {
				return m.openDiff(filepath.Join(m.root, filepath.FromSlash(r.change.Path)))
			}
			return m, nil
		case logBranches:
			if br, ok := m.log.selectedBranch(); ok && !br.Current {
				return m, switchBranchCmd(m.root, br.Name)
			}
			return m, nil
		default:
			if c, ok := m.log.selectedCommit(); ok {
				label := c.Short + "  " + c.Subject
				m.diff.beginLoad(label)
				m.diffReturn = modeBrowse
				m.mode = modeDiff
				return m, loadCommitDiffCmd(m.root, c.Hash, label)
			}
			return m, nil
		}
	}

	// Staging operations, available in the changes view.
	if m.log.kind == logChanges {
		switch msg.String() {
		case " ": // toggle stage/unstage the selected file or directory subtree
			if row, ok := m.log.selectedRow(); ok {
				paths := row.paths()
				if row.staged == stageAll {
					return m, m.runGitOp(func(ctx context.Context, r string) error {
						return gitlog.UnstagePaths(ctx, r, paths...)
					})
				}
				return m, m.runGitOp(func(ctx context.Context, r string) error {
					return gitlog.StagePaths(ctx, r, paths...)
				})
			}
			return m, nil
		case "A": // stage everything
			return m, m.runGitOp(gitlog.StageAll)
		case "U": // unstage everything
			return m, m.runGitOp(gitlog.UnstageAll)
		case "c": // commit staged changes
			return m, m.startPrompt(opCommit, "commit message")
		case "d": // open the selected changed file's diff in a read-only Diff tab
			if r, ok := m.log.selectedRow(); ok && !r.isDir {
				return m, m.openDiffTab(r.change)
			}
			return m, nil
		}
		return m, nil
	}

	// Git operations, available in the branch view.
	if m.log.kind == logBranches {
		switch msg.String() {
		// Local, non-destructive — run immediately.
		case "n":
			return m, m.startPrompt(opNewBranch, "new branch name")
		case "c":
			return m, m.startPrompt(opCommit, "commit message")
		case "A":
			return m, m.runGitOp(gitlog.StageAll)
		case "t":
			return m, m.startPrompt(opTag, "tag name (at HEAD)")
		case "f":
			return m, m.runGitOp(gitlog.Fetch)
		case "p":
			return m, m.runGitOp(gitlog.Pull)
		case "P":
			return m, m.runGitOp(gitlog.Push)
		case "s":
			return m, m.runGitOp(gitlog.Stash)
		case "S":
			return m, m.runGitOp(gitlog.StashPop)

		// Destructive / history-rewriting / remote-clobbering — confirm first.
		case "F":
			m.confirm = opForcePush
			return m, nil
		case "a":
			m.confirm = opAmend
			return m, nil
		case "u":
			m.confirm = opUndo
			return m, nil
		case "H":
			m.confirm = opResetHardHEAD
			return m, nil
		case "m":
			if br, ok := m.log.selectedBranch(); ok && !br.Current {
				m.confirm, m.confirmName = opMerge, br.Name
			}
			return m, nil
		case "r":
			if br, ok := m.log.selectedBranch(); ok && !br.Current {
				m.confirm, m.confirmName = opRebase, br.Name
			}
			return m, nil
		case "x":
			if br, ok := m.log.selectedBranch(); ok && !br.Current {
				m.confirm, m.confirmName = opDelete, br.Name
			}
			return m, nil
		}
	}

	// Commit operations, available in the history view.
	if m.log.kind == logHistory {
		switch msg.String() {
		case "y":
			if c, ok := m.log.selectedCommit(); ok {
				m.confirm, m.confirmName = opCherryPick, c.Hash
			}
			return m, nil
		case "R":
			if c, ok := m.log.selectedCommit(); ok {
				m.confirm, m.confirmName = opResetHardCommit, c.Hash
			}
			return m, nil
		}
	}
	return m, nil
}

// setFocus moves focus to the given panel, managing text-input focus so the
// cursor blinks only in the active search input.
func (m *Model) setFocus(f browseFocus) tea.Cmd {
	m.bfocus = f
	if f == focusSearch {
		if m.skind == kindContent {
			m.finder.blur()
			return m.search.focus()
		}
		m.search.blur()
		m.finder.refresh()
		return m.finder.focus()
	}
	m.finder.blur()
	m.search.blur()
	return nil
}

type focusDir int

const (
	dirLeft focusDir = iota
	dirRight
	dirUp
	dirDown
)

// focusDirection maps Alt+h/j/k/l and Alt+arrows to a direction.
func focusDirection(s string) (focusDir, bool) {
	switch s {
	case "alt+h", "alt+left":
		return dirLeft, true
	case "alt+l", "alt+right":
		return dirRight, true
	case "alt+k", "alt+up":
		return dirUp, true
	case "alt+j", "alt+down":
		return dirDown, true
	}
	return 0, false
}

// moveFocus shifts focus to the neighbouring panel in the given direction,
// following the 2×2 layout (tree | file / search | log).
func (m *Model) moveFocus(d focusDir) tea.Cmd {
	next := m.bfocus
	switch m.bfocus {
	case focusExplorer:
		if d == dirRight {
			next = focusViewer
		} else if d == dirDown {
			next = focusSearch
		}
	case focusViewer:
		if d == dirLeft {
			next = focusExplorer
		} else if d == dirDown {
			next = focusLog
		}
	case focusSearch:
		if d == dirUp {
			next = focusExplorer
		} else if d == dirRight {
			next = focusLog
		}
	case focusLog:
		if d == dirUp {
			next = focusViewer
		} else if d == dirLeft {
			next = focusSearch
		}
	}
	if next == m.bfocus {
		return nil
	}
	return m.setFocus(next)
}

// cycleFocus advances focus through the four panels: tree → file → search → log.
func (m *Model) cycleFocus() tea.Cmd {
	var next browseFocus
	switch m.bfocus {
	case focusExplorer:
		next = focusViewer
	case focusViewer:
		next = focusSearch
	case focusSearch:
		next = focusLog
	default:
		next = focusExplorer
	}
	return m.setFocus(next)
}

func (m *Model) searchMoveUp() {
	if m.skind == kindContent {
		m.search.moveUp()
	} else {
		m.finder.moveUp()
	}
}

func (m *Model) searchMoveDown() {
	if m.skind == kindContent {
		m.search.moveDown()
	} else {
		m.finder.moveDown()
	}
}

// openSearchSelection loads the currently-selected search result into the file
// view and reveals it in the tree. Called only on enter, never while typing, so
// the left panels stay stable during a search.
func (m *Model) openSearchSelection() tea.Cmd {
	if m.skind == kindContent {
		if hit, ok := m.search.selected(); ok {
			return m.openRelative(hit.Path, hit.Line)
		}
		return nil
	}
	if rel := m.finder.selected(); rel != "" {
		return m.openRelative(rel, 0)
	}
	return nil
}

// showFile loads a file into the viewer (plain, instantly) and returns a command
// that highlights it in the background. Every file-open path goes through here.
func (m *Model) showFile(abs string) tea.Cmd {
	m.viewer.Load(abs)
	m.currentFile = abs
	return m.highlightCmd()
}

// highlightCmd kicks off async syntax highlighting for the current file, guarded
// by a generation counter so a fast scroll only ever applies the latest result.
func (m *Model) highlightCmd() tea.Cmd {
	if !m.viewer.ShouldHighlight() {
		return nil
	}
	m.highlightGen++
	gen := m.highlightGen
	path := m.viewer.Path()
	src := m.viewer.Raw()
	return func() tea.Msg {
		return highlightMsg{gen: gen, path: path, lines: viewer.Highlight(path, src)}
	}
}

// previewSelected loads the file under the tree cursor into the viewer without
// changing focus, so tree navigation doubles as a live preview.
func (m *Model) previewSelected() tea.Cmd {
	if n := m.tree.Selected(); n != nil && !n.IsDir {
		return m.showFile(n.Path)
	}
	return nil
}

// openInEditor opens target in a configured editor. With a default (or a single
// configured editor) it opens immediately; otherwise it shows the picker.
func (m Model) openInEditor(target string, isProject bool) (tea.Model, tea.Cmd) {
	if target == "" {
		return m, nil
	}
	eds := editor.Load()
	if len(eds) == 0 {
		if p := editor.ConfigPath(); p != "" {
			m.statusNote = "no editors configured — add them to " + p
		} else {
			m.statusNote = "no editors configured (set $EDITOR or the editors config)"
		}
		return m, nil
	}
	if e, ok := editor.Preferred(eds); ok {
		return m, editorExec(e, target)
	}
	// No default and several configured → ask which one.
	m.pickerEditors = eds
	m.pickerCursor = 0
	m.pickerTarget = target
	m.pickerLabel = filepath.Base(target)
	if isProject {
		m.pickerLabel += "/  (project)"
	}
	m.pickerActive = true
	return m, nil
}

func editorExec(e editor.Editor, target string) tea.Cmd {
	return tea.ExecProcess(e.Command(target), func(err error) tea.Msg { return editorClosedMsg{err: err} })
}

// handlePickerKey drives the editor picker overlay.
func (m Model) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.pickerActive = false
		return m, nil
	case "up", "k":
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}
		return m, nil
	case "down", "j":
		if m.pickerCursor < len(m.pickerEditors)-1 {
			m.pickerCursor++
		}
		return m, nil
	case "enter":
		e := m.pickerEditors[m.pickerCursor]
		target := m.pickerTarget
		m.pickerActive = false
		return m, editorExec(e, target)
	}
	// Number keys pick directly (1-9).
	if len(msg.String()) == 1 {
		if d := int(msg.String()[0] - '1'); d >= 0 && d < len(m.pickerEditors) && d < 9 {
			e := m.pickerEditors[d]
			target := m.pickerTarget
			m.pickerActive = false
			return m, editorExec(e, target)
		}
	}
	return m, nil
}

// editTarget returns the file to open in the editor: the selected tree node if
// it's a file, otherwise the file open in the viewer.
func (m Model) editTarget() string {
	if m.bfocus == focusExplorer {
		if n := m.tree.Selected(); n != nil && !n.IsDir {
			return n.Path
		}
	}
	return m.currentFile
}

// revealTarget returns the path to reveal in the OS file manager: the selected
// tree node (file or directory) when the explorer is focused, otherwise the
// file currently open in the viewer.
func (m Model) revealTarget() string {
	if m.bfocus == focusExplorer {
		if n := m.tree.Selected(); n != nil {
			return n.Path
		}
	}
	return m.currentFile
}

// revealCmd opens the OS file manager at path off the UI thread (fire-and-forget;
// a failure — e.g. no xdg-open — is silently ignored).
func revealCmd(path string) tea.Cmd {
	return func() tea.Msg {
		_ = reveal.Reveal(path)
		return nil
	}
}

// diffTarget returns the absolute path of the file to review.
func (m Model) diffTarget() string {
	if m.bfocus == focusExplorer {
		if n := m.tree.Selected(); n != nil && !n.IsDir {
			return n.Path
		}
	}
	return m.currentFile
}

// openDiff enters the review view for an absolute file path.
func (m Model) openDiff(abs string) (tea.Model, tea.Cmd) {
	rel, err := filepath.Rel(m.root, abs)
	if err != nil {
		return m, nil
	}
	rel = filepath.ToSlash(rel)
	// Review the UNSTAGED (worktree) changes so the visible hunks are exactly
	// the ones `space` can stage. Untracked files have no index entry, so they
	// diff against an empty file and are staged whole.
	mode := gitdiff.ModeWorktree
	if m.git.FileCode(abs) == gitstatus.Untracked {
		mode = gitdiff.ModeUntracked
	}
	m.diffRel = rel
	m.diffMode = mode
	m.diff.beginLoad(rel)
	m.diffReturn = modeBrowse
	m.mode = modeDiff
	return m, loadDiffModeCmd(m.root, rel, mode)
}

// handleDiffKey drives the review view: scroll, toggle layout, move the hunk
// cursor, and stage the hunk under it.
func (m Model) handleDiffKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "d", "q":
		m.mode = m.diffReturn
		return m, nil
	case "s":
		m.diff.toggleLayout()
		return m, nil
	case "]", "n":
		m.diff.moveHunk(1)
		return m, nil
	case "[", "N":
		m.diff.moveHunk(-1)
		return m, nil
	case " ":
		return m.stageCurrentHunk()
	}
	var cmd tea.Cmd
	m.diff, cmd = m.diff.Update(msg)
	return m, cmd
}

// stageCurrentHunk stages the hunk under the review cursor into the index. It
// mutates only the index via `git apply --cached` (worktree files) or `git add`
// (untracked files); the working tree is never touched. On success the diff and
// git status reload; when the diff shows a staged review the same key unstages.
func (m Model) stageCurrentHunk() (tea.Model, tea.Cmd) {
	if m.diff.diff.Binary {
		m.statusNote = "binary file — cannot stage by hunk"
		return m, nil
	}
	if m.diff.hunkCount() == 0 {
		m.statusNote = "no hunk to stage"
		return m, nil
	}
	root, rel, mode, idx := m.root, m.diffRel, m.diffMode, m.diff.currentHunk()

	// An untracked file is one synthetic whole-file hunk; stage it with `git add`.
	if mode == gitdiff.ModeUntracked {
		m.statusNote = ""
		return m, func() tea.Msg {
			return hunkStagedMsg{err: gitlog.StageFile(context.Background(), root, rel)}
		}
	}

	// A staged review unstages (apply --reverse); a worktree review stages.
	reverse := mode == gitdiff.ModeStaged
	m.statusNote = ""
	return m, func() tea.Msg {
		raw, err := gitdiff.LoadRaw(context.Background(), root, rel, mode)
		if err != nil {
			return hunkStagedMsg{err: err}
		}
		patch := raw.Patch(idx)
		if patch == "" {
			return hunkStagedMsg{err: errors.New("hunk is no longer present")}
		}
		return hunkStagedMsg{err: gitlog.ApplyCachedPatch(context.Background(), root, patch, reverse)}
	}
}

// triggerSearch cancels any in-flight content search and starts a fresh one.
func (m *Model) triggerSearch() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	if strings.TrimSpace(m.search.input.Value()) == "" {
		m.search.setResult(search.Result{}, nil)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.searchGen++
	m.search.searching = true
	return runSearchCmd(ctx, m.searchGen, m.root, m.search.options())
}

// openRelative loads a file (slash path relative to root) into the viewer and
// reveals it in the tree, optionally jumping to a 1-based line. It does not
// change focus — callers decide. Returns the async-highlight command.
func (m *Model) openRelative(rel string, line int) tea.Cmd {
	abs := filepath.Join(m.root, filepath.FromSlash(rel))
	cmd := m.showFile(abs)
	if line > 0 {
		m.viewer.GoToLine(line)
	}
	m.tree.RevealPath(abs)
	return cmd
}

func (m Model) forwardToActive(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.mode == modeDiff {
		m.diff, cmd = m.diff.Update(msg)
		return m, cmd
	}
	if m.bfocus == focusSearch {
		if m.skind == kindContent {
			m.search.input, cmd = m.search.input.Update(msg)
		} else {
			m.finder.input, cmd = m.finder.input.Update(msg)
		}
		return m, cmd
	}
	m.viewer, cmd = m.viewer.Update(msg)
	return m, cmd
}

// layout sizes child components once per resize — never per frame — so View()
// stays cheap (viewport rebuilds its content only when the size actually
// changes).
func (m *Model) layout() {
	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}
	topH, _, _, _ := m.browseDims()
	vw := m.width - explorerWidth - 1
	if vw < 10 {
		vw = 10
	}
	m.viewer.SetSize(vw, topH) // the file view lives in the top region
	m.diff.SetSize(m.width, bodyH)
}

// --- view -------------------------------------------------------------------

func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}
	if m.pickerActive {
		return m.viewPicker()
	}
	if m.prompt != opNone {
		return m.viewPrompt()
	}
	if m.confirm != opNone {
		return m.viewConfirm()
	}
	switch m.mode {
	case modeDiff:
		return m.viewDiff()
	case modeHelp:
		return m.viewHelp()
	default:
		return m.viewBrowse()
	}
}

// viewPrompt renders the centered text-input overlay for new-branch / commit.
func (m Model) viewPrompt() string {
	var title string
	switch m.prompt {
	case opNewBranch:
		title = "New branch"
		if m.git.Branch != "" {
			title += " from " + m.git.Branch
		}
	case opCommit:
		title = "Commit message"
	case opTag:
		title = "New tag at HEAD"
	}
	m.promptInput.Width = min(64, m.width-14)
	inner := m.st.modalTitle.Render(title) + "\n\n" +
		m.st.prompt.Render("› ") + m.promptInput.View()
	box := modalBox(min(72, m.width-6)).Render(inner)
	body := centerModal(m.width, m.height-2, box)
	return m.frame(body, "enter confirm · esc cancel")
}

// viewPicker renders the editor chooser shown when no default editor is set.
func (m Model) viewPicker() string {
	lines := []string{m.st.modalTitle.Render("Open  " + m.pickerLabel + "  in…"), ""}
	for i, e := range m.pickerEditors {
		num := string(rune('1' + i))
		if i >= 9 {
			num = " "
		}
		row := num + "  " + e.Name
		if i == m.pickerCursor {
			row = m.st.modalSel.Render("▸ " + num + "  " + e.Name)
		} else {
			row = "  " + row
		}
		lines = append(lines, row)
	}
	box := modalBox(min(60, m.width-6)).Render(strings.Join(lines, "\n"))
	body := centerModal(m.width, m.height-2, box)
	return m.frame(body, "↑↓ move · 1-9 / enter open · esc cancel")
}

// viewConfirm renders the centered yes/no dialog for merge / rebase / delete.
func (m Model) viewConfirm() string {
	cur := m.git.Branch
	var q string
	switch m.confirm {
	case opMerge:
		q = "Merge  " + m.confirmName + "  into  " + cur + "  ?"
	case opRebase:
		q = "Rebase  " + cur + "  onto  " + m.confirmName + "  ?"
	case opDelete:
		q = "Delete branch  " + m.confirmName + "  ?"
	case opForcePush:
		q = "Force-push  " + cur + "  to remote?  (--force-with-lease)"
	case opAmend:
		q = "Amend last commit with all current changes?"
	case opUndo:
		q = "Undo last commit?  (changes stay staged)"
	case opCherryPick:
		q = "Cherry-pick  " + shortHash(m.confirmName) + "  onto  " + cur + "  ?"
	case opResetHardCommit:
		q = "Reset --hard  " + cur + "  to  " + shortHash(m.confirmName) + "  ?  (discards commits & changes)"
	case opResetHardHEAD:
		q = "Discard ALL uncommitted changes?  (reset --hard HEAD)"
	}
	inner := m.st.modalTitle.Render("Confirm") + "\n\n" + q
	box := modalBox(min(72, m.width-6)).Render(inner)
	body := centerModal(m.width, m.height-2, box)
	return m.frame(body, "y confirm · n / esc cancel")
}

// browseDims computes the browse-screen geometry once so layout() (which sizes
// components) and viewBrowse() (which draws them) always agree. The docked
// panels take ~a third of the height, leaving the tree/file view the majority.
func (m Model) browseDims() (topH, bottomH, leftW, rightW int) {
	bodyH := m.height - 2
	bottomH = bodyH / 3
	if bottomH < 8 {
		bottomH = 8
	}
	if bottomH > bodyH-6 {
		bottomH = bodyH - 6
	}
	if bottomH < 3 {
		bottomH = 3
	}
	topH = bodyH - bottomH - 1 // -1 for the panel label rules row
	if topH < 1 {
		topH = 1
	}
	leftW = (m.width - 1) / 2
	rightW = m.width - leftW - 1
	return topH, bottomH, leftW, rightW
}

func (m Model) viewBrowse() string {
	topH, bottomH, leftW, rightW := m.browseDims()

	top := m.browseBody(topH)

	rows := bottomH - 3
	if rows < 2 {
		rows = 2
	}

	// Left docked panel: fuzzy file find or content search.
	var sLabel, sContent string
	if m.skind == kindContent {
		sLabel = "SEARCH IN FILES"
		sContent = m.search.view(m.st, leftW-2, rows)
	} else {
		sLabel = "FIND FILE"
		sContent = m.finder.view(m.st, leftW-2, rows)
	}
	leftPanel := lipgloss.JoinVertical(lipgloss.Left,
		panelRule(sLabel, m.bfocus == focusSearch, leftW),
		clampHeight(lipgloss.NewStyle().PaddingLeft(1).Render(sContent), bottomH),
	)

	// Right docked panel: git log or branches, titled with the branch.
	logLabel := m.log.title()
	if m.git.Branch != "" {
		logLabel += " · " + m.git.Branch
	}
	logContent := m.log.view(m.st, rightW-2, bottomH, m.bfocus == focusLog)
	rightPanel := lipgloss.JoinVertical(lipgloss.Left,
		panelRule(logLabel, m.bfocus == focusLog, rightW),
		clampHeight(lipgloss.NewStyle().PaddingLeft(1).Render(logContent), bottomH),
	)

	gap := lipgloss.NewStyle().Width(1).Height(bottomH + 1).Render("")
	bottom := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, gap, rightPanel)

	body := lipgloss.JoinVertical(lipgloss.Left, top, bottom)
	help := "tab · ^p find · ^f search · g log · d diff · e edit · L locate · o os · m md · ? help · q quit"
	if m.statusNote != "" {
		help = m.statusNote
	}
	return m.frame(body, help)
}

// panelRule renders a labeled horizontal divider (a "tab") of a given width for
// a docked panel; the label brightens when its panel holds focus.
func panelRule(label string, focused bool, width int) string {
	tabColor := colBorder
	if focused {
		tabColor = colAccent
	}
	tab := lipgloss.NewStyle().Foreground(tabColor).Bold(true).Render(" " + label + " ")
	fill := width - lipgloss.Width(tab) - 2
	if fill < 0 {
		fill = 0
	}
	return lipgloss.NewStyle().Foreground(colAccent).Render("──") + tab +
		lipgloss.NewStyle().Foreground(colBorder).Render(strings.Repeat("─", fill))
}

// browseBody renders the [explorer | file view] split at a given height. The
// viewer is sized once in layout(); here we only draw it, so no per-frame
// content rebuild happens.
func (m Model) browseBody(bodyH int) string {
	if bodyH < 1 {
		bodyH = 1
	}
	left := m.renderExplorer(explorerWidth, bodyH)
	// Exactly bodyH lines — a trailing newline here would add a phantom row that
	// pushes the layout one line too tall and clips the bottom of the panels.
	sep := lipgloss.NewStyle().Foreground(colBorder).
		Render(strings.TrimSuffix(strings.Repeat("│\n", bodyH), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, sep, m.viewer.View())
}

func (m Model) viewDiff() string {
	help := "↑↓/pgup/pgdn scroll · ][/nN hunk · space stage hunk · s split/inline · d/esc/q back"
	return m.frame(m.diff.View(), help)
}

// viewHelp renders a centered card listing every keybinding.
func (m Model) viewHelp() string {
	w := helpWidth(m.st) + 4 // + horizontal padding
	if w > m.width-4 {
		w = m.width - 4
	}
	box := modalBox(w).Render(helpContent(m.st))
	body := centerModal(m.width, m.height-2, box)
	return m.frame(body, "any key to close")
}

// frame wraps body content with the header title bar and footer help line.
func (m Model) frame(body, help string) string {
	// Truncate header/footer to one line each — a string wider than the pane
	// would wrap and push the layout a row too tall (clipping the bottom).
	title := m.st.header.Width(m.width).Render(truncateLine(m.headerText(), m.width))
	footer := m.st.footer.Width(m.width).Render(truncateLine(" "+help, m.width))
	body = clampHeight(body, m.height-2)
	return lipgloss.JoinVertical(lipgloss.Left, title, body, footer)
}

func (m Model) headerText() string {
	name := filepath.Base(m.root)
	branch := ""
	if m.git.Branch != "" {
		branch = "  ⎇ " + m.git.Branch
	}
	title := "  File Viewer — " + name + branch
	if m.mode == modeDiff {
		title = "  Review — " + name + branch
	}
	if m.updateLatest != "" {
		title += "   ⬆ " + m.updateLatest + " available (reinstall to update)"
	}
	return title
}

// renderExplorer draws the file tree column with git decorations and a
// highlighted cursor row when the explorer holds focus.
func (m Model) renderExplorer(width, height int) string {
	nodes := m.tree.Visible()
	cursor := m.tree.Cursor()
	start := scrollStart(cursor, len(nodes), height)

	var b strings.Builder
	for i := start; i < start+height && i < len(nodes); i++ {
		n := nodes[i]
		indent := strings.Repeat("  ", n.Depth)

		color, badge, decorated := lipgloss.Color(""), " ", false
		if n.IsDir {
			decorated = m.git.DirDirty(n.Path)
			color = colGitDirtyDir
		} else {
			color, badge, decorated = gitDecor(m.git.FileCode(n.Path))
		}

		var icon, label string
		name := n.Name
		if n.IsDir {
			if n.Expanded {
				icon = "▾ "
			} else {
				icon = "▸ "
			}
			name += "/"
			if decorated {
				label = lipgloss.NewStyle().Foreground(color).Bold(true).Render(name)
			} else {
				label = m.st.dir.Render(name)
			}
		} else {
			icon = "  "
			if decorated {
				label = lipgloss.NewStyle().Foreground(color).Render(name)
			} else {
				label = m.st.file.Render(name)
			}
		}

		badgeCell := " "
		if !n.IsDir && decorated {
			badgeCell = lipgloss.NewStyle().Foreground(color).Bold(true).Render(badge)
		}

		row := indent + m.st.muted.Render(icon) + label
		if i == cursor && m.bfocus == focusExplorer {
			plain := indent + icon + name
			row = m.st.rowSelected.Width(width).Render(plain)
		} else {
			row = padRow(row, width-1) + badgeCell
		}
		b.WriteString(truncateLine(row, width))
		if i < start+height-1 && i < len(nodes)-1 {
			b.WriteByte('\n')
		}
	}
	return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
}

// clampHeight pads or trims a block to exactly h rows.
func clampHeight(s string, h int) string {
	if h < 0 {
		h = 0
	}
	lines := strings.Split(s, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}
