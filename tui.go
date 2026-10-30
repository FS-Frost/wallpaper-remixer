package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type screen int

const (
	screenMenu screen = iota
	screenImages
	screenHistory
	screenFavorites
	screenBlacklist
	screenDirs
	screenSettings
	screenModes
	screenStats
	screenLogs
	screenInput
	screenConfirm
)

const (
	tuiRefreshEvery  = time.Second
	previewDebounce  = 150 * time.Millisecond
	previewCacheSize = 40
	logLinesShown    = 500
	headerHeight     = 3
	footerHeight     = 1
)

var (
	colorAccent = lipgloss.AdaptiveColor{Light: "#7D56F4", Dark: "#A78BFA"}
	colorOK     = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	colorErr    = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	colorMuted  = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
	colorFav    = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}

	styleTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(colorAccent).Padding(0, 1)
	styleOK     = lipgloss.NewStyle().Foreground(colorOK)
	styleErr    = lipgloss.NewStyle().Foreground(colorErr)
	styleMuted  = lipgloss.NewStyle().Foreground(colorMuted)
	styleFav    = lipgloss.NewStyle().Foreground(colorFav)
	styleBold   = lipgloss.NewStyle().Bold(true)
	styleHeader = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).BorderForeground(colorMuted)
	stylePane   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorMuted).Padding(0, 1)
	styleBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccent).Padding(1, 2)
)

var (
	keyEnter    = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select"))
	keyBack     = key.NewBinding(key.WithKeys("esc", "q", "backspace"), key.WithHelp("esc", "back"))
	keyQuit     = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))
	keyForce    = key.NewBinding(key.WithKeys("ctrl+c"))
	keyRemix    = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "remix"))
	keyPrev     = key.NewBinding(key.WithKeys("["), key.WithHelp("[", "previous"))
	keyNext     = key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next"))
	keyFav      = key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "favorite"))
	keyBan      = key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "blacklist"))
	keyLoop     = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "start/stop loop"))
	keyOpen     = key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open"))
	keyReveal   = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "show in folder"))
	keyPreview  = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "toggle preview"))
	keyDelete   = key.NewBinding(key.WithKeys("d", "delete"), key.WithHelp("d", "remove"))
	keyAdd      = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add"))
	keyRefresh  = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh"))
	keyYes      = key.NewBinding(key.WithKeys("y", "enter"), key.WithHelp("y", "yes"))
	keyNo       = key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n", "no"))
	keySetImage = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "set wallpaper"))
)

type entry struct {
	title  string
	desc   string
	action string
	path   string
	offset int
}

func (e entry) Title() string       { return e.title }
func (e entry) Description() string { return e.desc }
func (e entry) FilterValue() string { return e.title + " " + e.path }

type tickMsg time.Time

type snapshotMsg struct {
	state         *State
	loop          LoopInfo
	autostart     bool
	autostartPath string
	autostartErr  error
	err           error
}

type opDoneMsg struct {
	text    string
	err     error
	next    screen
	restart bool
}

type itemsMsg struct {
	screen screen
	items  []list.Item
}

type textMsg struct {
	screen  screen
	content string
}

type previewTickMsg struct{ seq int }

type previewMsg struct {
	key     string
	preview Preview
	err     error
}

type previewResult struct {
	preview Preview
	err     error
}

type model struct {
	width  int
	height int

	screen   screen
	list     list.Model
	input    textinput.Model
	viewport viewport.Model
	spinner  spinner.Model

	state         *State
	loop          LoopInfo
	autostart     bool
	autostartPath string
	autostartErr  error

	busy      string
	status    string
	statusErr bool
	loading   bool
	itemsSig  string

	inputTitle  string
	inputHint   string
	inputSubmit func(string) (string, error)
	inputParent screen

	confirmText    string
	confirmFn      func() (string, error)
	confirmParent  screen
	confirmRestart bool

	// restartExe is set after a successful update; main relaunches it once the TUI exits.
	restartTarget string
	restartExe    string

	showPreview bool
	previewKey  string
	previewSeq  int
	previews    map[string]previewResult
}

// runTUI runs the menu. restartExe is non-empty when the menu updated itself and the new
// executable must be launched in its place.
func runTUI() (restartExe string, err error) {
	logToStdout = false
	final, err := tea.NewProgram(newModel(), tea.WithAltScreen()).Run()
	if m, ok := final.(*model); ok {
		restartExe = m.restartExe
	}
	return restartExe, err
}

func newModel() *model {
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(colorAccent).BorderForeground(colorAccent)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(colorAccent).BorderForeground(colorAccent)

	l := list.New(nil, delegate, 0, 0)
	l.DisableQuitKeybindings()
	l.SetShowTitle(true)
	l.Styles.Title = styleTitle

	in := textinput.New()
	in.Prompt = "› "
	in.CharLimit = 1024

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)

	m := &model{
		list:        l,
		input:       in,
		viewport:    viewport.New(0, 0),
		spinner:     sp,
		state:       &State{},
		showPreview: true,
		previews:    map[string]previewResult{},
	}
	m.enter(screenMenu)
	return m
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(loadSnapshot, tickEvery())
}

func tickEvery() tea.Cmd {
	return tea.Tick(tuiRefreshEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func loadSnapshot() tea.Msg {
	state, err := readState()
	msg := snapshotMsg{state: state, loop: readLoopInfo(), err: err}
	msg.autostart, msg.autostartPath, msg.autostartErr = autostartStatus()
	return msg
}

// ---------- update ----------

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, m.requestPreview()

	case tickMsg:
		return m, tea.Batch(loadSnapshot, tickEvery())

	case snapshotMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}
		m.state, m.loop = msg.state, msg.loop
		m.autostart, m.autostartPath, m.autostartErr = msg.autostart, msg.autostartPath, msg.autostartErr
		return m, tea.Batch(m.rebuildStaticItems(), m.requestPreview())

	case opDoneMsg:
		m.busy = ""
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
		} else {
			m.setStatus(msg.text, false)
		}
		if msg.restart && msg.err == nil {
			m.restartExe = m.restartTarget
			return m, tea.Quit
		}
		// A failed input stays open so the value can be fixed.
		keepInput := m.screen == screenInput && msg.err != nil
		if !keepInput && msg.next != m.screen && (m.screen == screenInput || m.screen == screenConfirm || m.screen == screenModes) {
			m.enter(msg.next)
		}
		return m, tea.Batch(loadSnapshot, m.loadScreen())

	case itemsMsg:
		if msg.screen != m.screen {
			return m, nil
		}
		m.loading = false
		return m, tea.Batch(m.setItems(msg.items), m.requestPreview())

	case textMsg:
		if msg.screen != m.screen {
			return m, nil
		}
		m.loading = false
		m.viewport.SetContent(msg.content)
		if m.screen == screenLogs {
			m.viewport.GotoBottom()
		} else {
			m.viewport.GotoTop()
		}
		return m, nil

	case previewTickMsg:
		if msg.seq != m.previewSeq {
			return m, nil
		}
		return m, m.renderPreviewCmd()

	case previewMsg:
		if len(m.previews) >= previewCacheSize {
			m.previews = map[string]previewResult{}
		}
		m.previews[msg.key] = previewResult{preview: msg.preview, err: msg.err}
		return m, nil

	case spinner.TickMsg:
		if m.busy == "" && !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if key.Matches(msg, keyForce) {
			return m, tea.Quit
		}
		return m, m.handleKey(msg)
	}

	return m, m.forward(msg)
}

// forward passes a message to the component that owns the current screen.
func (m *model) forward(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch {
	case m.screen == screenInput:
		m.input, cmd = m.input.Update(msg)
	case m.isTextScreen():
		m.viewport, cmd = m.viewport.Update(msg)
	case m.isListScreen():
		m.list, cmd = m.list.Update(msg)
		return tea.Batch(cmd, m.requestPreview())
	}
	return cmd
}

func (m *model) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch m.screen {
	case screenInput:
		return m.handleInputKey(msg)
	case screenConfirm:
		return m.handleConfirmKey(msg)
	}

	if m.isListScreen() && m.list.FilterState() == list.Filtering {
		return m.forward(msg)
	}

	if m.isTextScreen() {
		switch {
		case key.Matches(msg, keyBack):
			m.enter(screenMenu)
			return m.loadScreen()
		case key.Matches(msg, keyRefresh):
			return m.loadScreen()
		}
		return m.forward(msg)
	}

	// esc clears an applied filter before it means "back".
	if msg.String() == "esc" && m.list.FilterState() == list.FilterApplied {
		return m.forward(msg)
	}

	if m.screen == screenMenu {
		if key.Matches(msg, keyQuit) {
			return tea.Quit
		}
		return m.handleMenuKey(msg)
	}

	if key.Matches(msg, keyBack) {
		parent := screenMenu
		if m.screen == screenModes {
			parent = screenSettings
		}
		m.enter(parent)
		return m.loadScreen()
	}

	switch m.screen {
	case screenSettings:
		if key.Matches(msg, keyEnter) {
			return m.settingsAction()
		}
	case screenModes:
		if key.Matches(msg, keyEnter) {
			if e, ok := m.selected(); ok {
				return m.runOp("setting fit mode", screenSettings, func() (string, error) {
					return "Fit mode set to " + modeLabel(e.action), setWallpaperMode(e.action)
				})
			}
		}
	case screenDirs:
		if cmd, handled := m.handleDirsKey(msg); handled {
			return cmd
		}
	case screenImages, screenHistory, screenFavorites, screenBlacklist:
		if cmd, handled := m.handleImageKey(msg); handled {
			return cmd
		}
	}

	return m.forward(msg)
}

func (m *model) handleMenuKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keyEnter):
		if e, ok := m.selected(); ok {
			return m.menuAction(e.action)
		}
	case key.Matches(msg, keyRemix):
		return m.menuAction("remix")
	case key.Matches(msg, keyPrev):
		return m.menuAction("prev")
	case key.Matches(msg, keyNext):
		return m.menuAction("next")
	case key.Matches(msg, keyFav):
		return m.menuAction("favcur")
	case key.Matches(msg, keyBan):
		return m.menuAction("blcur")
	case key.Matches(msg, keyLoop):
		return m.menuAction("loop")
	}

	return m.forward(msg)
}

func (m *model) menuAction(action string) tea.Cmd {
	switch action {
	case "remix":
		return m.runOp("remixing", screenMenu, func() (string, error) {
			if err := remixWallpaper(); err != nil {
				return "", err
			}
			return "New wallpaper: " + currentName(), nil
		})
	case "prev":
		return m.runOp("going back", screenMenu, func() (string, error) {
			if err := goBack(1); err != nil {
				return "", err
			}
			return "Back to: " + currentName(), nil
		})
	case "next":
		return m.runOp("going forward", screenMenu, func() (string, error) {
			if err := goForward(1); err != nil {
				return "", err
			}
			return "Forward to: " + currentName(), nil
		})
	case "favcur":
		return m.runOp("updating favorites", screenMenu, func() (string, error) {
			added, err := toggleFavorite("")
			if added {
				return "Added to favorites: " + currentName(), err
			}
			return "Removed from favorites: " + currentName(), err
		})
	case "blcur":
		return m.runOp("blacklisting", screenMenu, func() (string, error) {
			banned := currentName()
			if err := blacklistCurrent(); err != nil {
				return "", err
			}
			if err := remixWallpaper(); err != nil {
				return "", fmt.Errorf("blacklisted %s, but remix failed: %v", banned, err)
			}
			return fmt.Sprintf("Blacklisted %s, new wallpaper: %s", banned, currentName()), nil
		})
	case "loop":
		if m.loop.Running {
			return m.runOp("stopping loop", screenMenu, func() (string, error) {
				return "Background loop stopped", stopBackgroundLoop()
			})
		}
		return m.runOp("starting loop", screenMenu, func() (string, error) {
			pid, err := startBackgroundLoop()
			return fmt.Sprintf("Background loop started (pid %d)", pid), err
		})
	case "restart":
		return m.runOp("restarting loop", screenMenu, func() (string, error) {
			pid, err := restartBackgroundLoop()
			return fmt.Sprintf("Background loop restarted (pid %d)", pid), err
		})
	case "resetcycle":
		return m.confirm("Allow every already used image to be picked again?", func() (string, error) {
			return "Rotation cycle reset", resetCycle()
		})
	case "update":
		dir, err := resolveSourceDir(m.state)
		if err != nil {
			m.setStatus(err.Error(), true)
			return nil
		}
		exe, err := runningExecutable()
		if err != nil {
			m.setStatus(err.Error(), true)
			return nil
		}
		// Captured now: once the update renames the running image, its path changes.
		m.restartTarget = exe
		cmd := m.confirm(fmt.Sprintf("Run tests, rebuild from %s and restart the menu? A running loop is restarted too.", dir), selfUpdate)
		m.confirmRestart = true
		return cmd
	case "clearhistory":
		return m.confirm("Clear the whole history and redo stack? Back/next will have nothing to go to.", func() (string, error) {
			return "History cleared", clearHistory()
		})
	case "quit":
		return tea.Quit
	case "browse":
		m.enter(screenImages)
	case "history":
		m.enter(screenHistory)
	case "favorites":
		m.enter(screenFavorites)
	case "blacklist":
		m.enter(screenBlacklist)
	case "dirs":
		m.enter(screenDirs)
	case "settings":
		m.enter(screenSettings)
	case "stats":
		m.enter(screenStats)
	case "logs":
		m.enter(screenLogs)
	}

	return m.loadScreen()
}

func (m *model) settingsAction() tea.Cmd {
	e, ok := m.selected()
	if !ok {
		return nil
	}

	switch e.action {
	case "interval":
		return m.prompt("Interval between remixes", "Seconds (90) or a duration (5m, 1h30m)", formatInterval(m.state.SecondsToWait), func(value string) (string, error) {
			seconds, err := parseInterval(value)
			if err != nil {
				return "", err
			}
			return "Interval set to " + formatInterval(seconds), setInterval(seconds)
		})
	case "pool":
		next := poolFavorites
		if m.state.PoolMode == poolFavorites {
			next = poolAll
		}
		return m.runOp("switching pool", screenSettings, func() (string, error) {
			return "Pool set to " + next, setPoolMode(next)
		})
	case "mode":
		m.enter(screenModes)
		return m.loadScreen()
	case "sync":
		enable := m.state.DisableBrowserSync
		return m.runOp("updating browser sync", screenSettings, func() (string, error) {
			return "Browser sync " + onOff(enable), setBrowserSync(enable)
		})
	case "browserdir":
		return m.prompt("Browser directory", "Folder that holds the browser background image. Empty disables sync.", m.state.BrowserDir, func(value string) (string, error) {
			return "Browser directory saved", setBrowserDir(value)
		})
	case "browserimage":
		return m.prompt("Browser image file name", "File name inside the browser directory, e.g. edge_background.jpg", m.state.BrowserImage, func(value string) (string, error) {
			return "Browser image saved", setBrowserImage(value)
		})
	case "sourcedir":
		return m.prompt("Source directory", "Repository used by Update from source. Empty uses the one embedded at build time.", m.state.SourceDir, func(value string) (string, error) {
			return "Source directory saved", setSourceDir(value)
		})
	case "autostart":
		if !autostartSupported {
			m.setStatus(errAutostartText(), true)
			return nil
		}
		if m.autostart {
			return m.runOp("disabling autostart", screenSettings, func() (string, error) {
				return "Autostart disabled", disableAutostart()
			})
		}
		return m.runOp("enabling autostart", screenSettings, func() (string, error) {
			scriptPath, err := enableAutostart()
			return "Autostart enabled: " + scriptPath, err
		})
	}

	return nil
}

func (m *model) handleDirsKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, keyAdd):
		return m.prompt("Add images directory", "Absolute path, searched recursively", "", func(value string) (string, error) {
			return "Directory added", addImagesDirectory(value)
		}), true
	case key.Matches(msg, keyDelete):
		if e, ok := m.selected(); ok {
			return m.confirm(fmt.Sprintf("Remove %q from the images directories? Files are not deleted.", e.path), func() (string, error) {
				return "Directory removed", removeImagesDirectory(e.path)
			}), true
		}
	case key.Matches(msg, keyEnter), key.Matches(msg, keyReveal):
		if e, ok := m.selected(); ok {
			return m.quickOp(revealInFileManager(e.path), "Opened "+e.path), true
		}
	}

	return nil, false
}

func (m *model) handleImageKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	e, ok := m.selected()

	switch {
	case key.Matches(msg, keyPreview):
		m.showPreview = !m.showPreview
		m.resize()
		return m.requestPreview(), true
	case !ok:
		return nil, false
	case key.Matches(msg, keyOpen):
		return m.quickOp(openInDefaultApp(e.path), "Opened "+filepath.Base(e.path)), true
	case key.Matches(msg, keyReveal):
		return m.quickOp(revealInFileManager(e.path), "Revealed "+filepath.Base(e.path)), true
	}

	switch m.screen {
	case screenBlacklist:
		if key.Matches(msg, keyEnter) || key.Matches(msg, keyDelete) {
			return m.runOp("updating blacklist", m.screen, func() (string, error) {
				return "Removed from blacklist: " + filepath.Base(e.path), unblacklist(e.path)
			}), true
		}
		return nil, false
	case screenFavorites:
		if key.Matches(msg, keyDelete) {
			return m.runOp("updating favorites", m.screen, func() (string, error) {
				_, err := toggleFavorite(e.path)
				return "Removed from favorites: " + filepath.Base(e.path), err
			}), true
		}
	}

	switch {
	case key.Matches(msg, keyEnter):
		if m.screen == screenHistory {
			return m.runOp("jumping in history", m.screen, func() (string, error) {
				return "Wallpaper: " + filepath.Base(e.path), jumpHistory(e.offset)
			}), true
		}
		return m.runOp("setting wallpaper", m.screen, func() (string, error) {
			return "Wallpaper: " + filepath.Base(e.path), setImage(e.path)
		}), true
	case key.Matches(msg, keyFav):
		return m.runOp("updating favorites", m.screen, func() (string, error) {
			added, err := toggleFavorite(e.path)
			if added {
				return "Added to favorites: " + filepath.Base(e.path), err
			}
			return "Removed from favorites: " + filepath.Base(e.path), err
		}), true
	case key.Matches(msg, keyBan) && m.screen == screenImages:
		return m.runOp("blacklisting", m.screen, func() (string, error) {
			return "Blacklisted: " + filepath.Base(e.path), blacklistImage(e.path)
		}), true
	}

	return nil, false
}

func (m *model) handleInputKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.enter(m.inputParent)
		return m.loadScreen()
	case "enter":
		value, submit := m.input.Value(), m.inputSubmit
		return m.runOp("saving", m.inputParent, func() (string, error) { return submit(value) })
	}

	return m.forward(msg)
}

func (m *model) handleConfirmKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keyYes):
		if m.confirmRestart {
			return m.runOpMsg("updating: tests, build, deploy", m.confirmParent, m.confirmFn, true)
		}
		return m.runOp("working", m.confirmParent, m.confirmFn)
	case key.Matches(msg, keyNo):
		m.enter(m.confirmParent)
		return m.loadScreen()
	}

	return nil
}

// ---------- helpers ----------

func (m *model) runOp(label string, next screen, fn func() (string, error)) tea.Cmd {
	return m.runOpMsg(label, next, fn, false)
}

func (m *model) runOpMsg(label string, next screen, fn func() (string, error), restart bool) tea.Cmd {
	if m.busy != "" {
		m.setStatus("Still "+m.busy+", wait a moment", true)
		return nil
	}

	m.busy = label
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		text, err := fn()
		return opDoneMsg{text: text, err: err, next: next, restart: restart}
	})
}

func (m *model) quickOp(err error, text string) tea.Cmd {
	if err != nil {
		m.setStatus(err.Error(), true)
	} else {
		m.setStatus(text, false)
	}
	return nil
}

func (m *model) prompt(title, hint, value string, submit func(string) (string, error)) tea.Cmd {
	m.inputTitle, m.inputHint, m.inputSubmit, m.inputParent = title, hint, submit, m.screen
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.enter(screenInput)
	return m.input.Focus()
}

func (m *model) confirm(text string, fn func() (string, error)) tea.Cmd {
	m.confirmText, m.confirmFn, m.confirmParent, m.confirmRestart = text, fn, m.screen, false
	m.enter(screenConfirm)
	return nil
}

func (m *model) setStatus(text string, isErr bool) {
	m.status, m.statusErr = text, isErr
}

func (m *model) selected() (entry, bool) {
	e, ok := m.list.SelectedItem().(entry)
	return e, ok
}

func (m *model) isListScreen() bool {
	switch m.screen {
	case screenMenu, screenImages, screenHistory, screenFavorites, screenBlacklist, screenDirs, screenSettings, screenModes:
		return true
	}
	return false
}

func (m *model) isTextScreen() bool {
	return m.screen == screenStats || m.screen == screenLogs
}

func (m *model) isImageScreen() bool {
	switch m.screen {
	case screenImages, screenHistory, screenFavorites, screenBlacklist:
		return true
	}
	return false
}

// enter switches screens and configures the list for the new one.
func (m *model) enter(s screen) {
	if s != screenInput {
		m.input.Blur()
	}

	m.screen = s
	m.itemsSig = ""
	if !m.isListScreen() {
		m.resize()
		return
	}

	m.list.ResetFilter()
	m.list.SetItems(nil)
	m.list.ResetSelected()
	m.list.SetFilteringEnabled(true)
	m.list.SetShowStatusBar(s != screenMenu && s != screenSettings && s != screenModes)
	m.list.Title, m.list.AdditionalShortHelpKeys = screenTitle(s), m.helpKeys(s)
	m.list.AdditionalFullHelpKeys = m.list.AdditionalShortHelpKeys
	m.list.SetStatusBarItemName("item", "items")
	if m.isImageScreen() {
		m.list.SetStatusBarItemName("image", "images")
	}
	m.resize()
	m.rebuildStaticItems()
}

// screenTitle is the breadcrumb from the menu to s, so the way back is always visible.
func screenTitle(s screen) string {
	switch s {
	case screenMenu:
		return "Menu"
	case screenModes:
		return screenTitle(screenSettings) + " › " + screenName(s)
	}
	return "Menu › " + screenName(s)
}

func screenName(s screen) string {
	switch s {
	case screenImages:
		return "Browse images"
	case screenHistory:
		return "History"
	case screenFavorites:
		return "Favorites"
	case screenBlacklist:
		return "Blacklist"
	case screenDirs:
		return "Image directories"
	case screenSettings:
		return "Settings"
	case screenModes:
		return "Fit mode"
	case screenStats:
		return "Statistics"
	case screenLogs:
		return "Logs"
	}
	return "Menu"
}

// navHint is the always visible reminder of how to leave the current screen.
func (m *model) navHint() string {
	switch {
	case m.screen == screenInput:
		return "enter save · esc cancel"
	case m.screen == screenConfirm:
		return "y yes · n no"
	case m.isListScreen() && m.list.FilterState() == list.Filtering:
		return "enter apply filter · esc cancel filter"
	case m.isListScreen() && m.list.FilterState() == list.FilterApplied:
		return "esc clear filter"
	case m.screen == screenMenu:
		return "enter select · / filter · q quit"
	}
	return "esc back"
}

func (m *model) helpKeys(s screen) func() []key.Binding {
	var bindings []key.Binding
	switch s {
	case screenMenu:
		bindings = []key.Binding{keyRemix, keyPrev, keyNext, keyFav, keyBan, keyLoop, keyQuit}
	case screenImages:
		bindings = []key.Binding{keyBack, keySetImage, keyFav, keyBan, keyOpen, keyReveal, keyPreview}
	case screenHistory:
		bindings = []key.Binding{keyBack, key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "jump here")), keyFav, keyOpen, keyReveal, keyPreview}
	case screenFavorites:
		bindings = []key.Binding{keyBack, keySetImage, keyDelete, keyOpen, keyReveal, keyPreview}
	case screenBlacklist:
		bindings = []key.Binding{keyBack, key.NewBinding(key.WithKeys("enter", "d"), key.WithHelp("enter/d", "unblacklist")), keyOpen, keyReveal, keyPreview}
	case screenDirs:
		bindings = []key.Binding{keyBack, keyAdd, keyDelete, key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open folder"))}
	default:
		bindings = []key.Binding{keyBack, keyEnter}
	}

	return func() []key.Binding { return bindings }
}

func (m *model) setItems(items []list.Item) tea.Cmd {
	var sb strings.Builder
	for _, it := range items {
		e := it.(entry)
		sb.WriteString(e.title + "\x00" + e.desc + "\x00" + e.path + "\n")
	}

	if sig := sb.String(); sig == m.itemsSig {
		return nil
	} else {
		m.itemsSig = sig
	}

	return m.list.SetItems(items)
}

// rebuildStaticItems refreshes the screens whose items come straight from the state.
func (m *model) rebuildStaticItems() tea.Cmd {
	switch m.screen {
	case screenMenu:
		return m.setItems(menuEntries(m))
	case screenSettings:
		return m.setItems(settingsEntries(m))
	case screenModes:
		return m.setItems(modeEntries(m.state))
	case screenHistory:
		return m.setItems(historyEntries(m.state))
	case screenFavorites:
		return m.setItems(pathEntries(m.state.Favorites, m.state, "favorite"))
	case screenBlacklist:
		return m.setItems(pathEntries(m.state.Blacklist, m.state, "blacklisted"))
	}
	return nil
}

// loadScreen fetches the data a screen needs; slow scans run in the background.
func (m *model) loadScreen() tea.Cmd {
	state := *m.state
	s := m.screen

	switch s {
	case screenImages:
		m.loading = true
		return tea.Batch(m.spinner.Tick, func() tea.Msg {
			fresh, err := readState()
			if err == nil {
				state = *fresh
			}
			return itemsMsg{screen: s, items: imageEntries(&state)}
		})
	case screenDirs:
		m.loading = true
		return tea.Batch(m.spinner.Tick, func() tea.Msg {
			fresh, err := readState()
			if err == nil {
				state = *fresh
			}
			return itemsMsg{screen: s, items: dirEntries(&state)}
		})
	case screenStats:
		m.loading = true
		return tea.Batch(m.spinner.Tick, func() tea.Msg {
			fresh, err := readState()
			if err == nil {
				state = *fresh
			}
			return textMsg{screen: s, content: statsText(&state)}
		})
	case screenLogs:
		return func() tea.Msg {
			lines, err := tailLog(logLinesShown)
			if err != nil {
				return textMsg{screen: s, content: styleErr.Render(err.Error())}
			}
			if len(lines) == 0 {
				return textMsg{screen: s, content: styleMuted.Render("Log is empty.")}
			}
			return textMsg{screen: s, content: strings.Join(lines, "\n")}
		}
	}

	return m.rebuildStaticItems()
}

// previewPaneWidth is the total width of the preview pane, border included.
func (m *model) previewPaneWidth() int {
	return max(24, m.width*2/5)
}

// previewSize is the art area inside the pane: minus border and padding, and two
// rows for the info line and its gap.
func (m *model) previewSize() (cols, rows int) {
	return m.previewPaneWidth() - 2 - 2, m.bodyHeight() - 2 - 2
}

func (m *model) previewVisible() bool {
	return m.isImageScreen() && m.showPreview && m.width >= 80 && m.bodyHeight() >= 8
}

func (m *model) previewCacheKey(path string) string {
	cols, rows := m.previewSize()
	return fmt.Sprintf("%s|%d|%d", path, cols, rows)
}

// requestPreview schedules a debounced preview render for the selected image.
func (m *model) requestPreview() tea.Cmd {
	if !m.previewVisible() {
		return nil
	}

	e, ok := m.selected()
	if !ok {
		return nil
	}

	cacheKey := m.previewCacheKey(e.path)
	if cacheKey == m.previewKey {
		return nil
	}

	m.previewKey = cacheKey
	m.previewSeq++
	if _, cached := m.previews[cacheKey]; cached {
		return nil
	}

	seq := m.previewSeq
	return tea.Tick(previewDebounce, func(time.Time) tea.Msg { return previewTickMsg{seq: seq} })
}

func (m *model) renderPreviewCmd() tea.Cmd {
	e, ok := m.selected()
	if !ok {
		return nil
	}

	cacheKey := m.previewCacheKey(e.path)
	if _, cached := m.previews[cacheKey]; cached {
		return nil
	}

	cols, rows := m.previewSize()
	return func() tea.Msg {
		p, err := renderPreview(e.path, cols, rows)
		return previewMsg{key: cacheKey, preview: p, err: err}
	}
}

func (m *model) bodyHeight() int {
	return max(1, m.height-headerHeight-footerHeight)
}

func (m *model) listWidth() int {
	if m.previewVisible() {
		return m.width - m.previewPaneWidth()
	}
	return m.width
}

func (m *model) resize() {
	m.list.SetSize(m.listWidth(), m.bodyHeight())
	m.viewport.Width, m.viewport.Height = m.width, m.bodyHeight()
	m.input.Width = max(10, m.width-12)
}

// ---------- entries ----------

func menuEntries(m *model) []list.Item {
	s := m.state

	loopEntry := entry{title: "Start background loop", desc: fmt.Sprintf("Remix every %s in a hidden process", formatInterval(s.SecondsToWait)), action: "loop"}
	if m.loop.Running {
		loopEntry = entry{title: "Stop background loop", desc: "Running" + pidText(m.loop.PID) + ", " + countdownText(s), action: "loop"}
	}

	favEntry := entry{title: "Favorite current", desc: "Add the current wallpaper to favorites", action: "favcur"}
	if containsPath(s.Favorites, currentImage(s)) {
		favEntry = entry{title: "Unfavorite current", desc: "Remove the current wallpaper from favorites", action: "favcur"}
	}

	return []list.Item{
		entry{title: "Remix now", desc: fmt.Sprintf("Random image from the %q pool", s.PoolMode), action: "remix"},
		entry{title: "Previous wallpaper", desc: fmt.Sprintf("Go back one image (%d in history)", max(0, len(s.ImagePaths)-1)), action: "prev"},
		entry{title: "Next wallpaper", desc: fmt.Sprintf("Redo after going back (%d available)", len(s.Forward)), action: "next"},
		entry{title: "Browse images", desc: "Fuzzy search every image with preview", action: "browse"},
		entry{title: "History", desc: "Jump to any previous wallpaper", action: "history"},
		entry{title: "Favorites", desc: fmt.Sprintf("%d favorite images", len(s.Favorites)), action: "favorites"},
		favEntry,
		entry{title: "Blacklist current", desc: "Never show the current image again and remix", action: "blcur"},
		entry{title: "Blacklist", desc: fmt.Sprintf("%d blacklisted images", len(s.Blacklist)), action: "blacklist"},
		entry{title: "Image directories", desc: fmt.Sprintf("%d directories configured", len(s.ImagesDirectories)), action: "dirs"},
		loopEntry,
		entry{title: "Restart background loop", desc: "Stop and start again, e.g. after rebuilding", action: "restart"},
		entry{title: "Settings", desc: "Interval, pool, fit mode, browser sync, autostart", action: "settings"},
		entry{title: "Statistics", desc: "Images per directory, used and remaining", action: "stats"},
		entry{title: "Logs", desc: "Last lines of " + logFileName, action: "logs"},
		updateEntry(s),
		entry{title: "Reset rotation", desc: fmt.Sprintf("Let used images be picked again (%d used)", len(s.UsedInCycle)), action: "resetcycle"},
		entry{title: "Clear history", desc: fmt.Sprintf("Forget history and redo stack (%d entries)", len(s.ImagePaths)+len(s.Forward)), action: "clearhistory"},
		entry{title: "Quit", desc: "Exit the menu, the background loop keeps running", action: "quit"},
	}
}

func updateEntry(s *State) entry {
	desc := "Run tests, rebuild and restart"
	if dir, err := resolveSourceDir(s); err != nil {
		desc = err.Error()
	} else {
		desc += " from " + dir
	}
	return entry{title: "Update from source", desc: desc, action: "update"}
}

func sourceDirText(s *State) string {
	switch {
	case s.SourceDir != "":
		return s.SourceDir
	case sourceDir != "":
		return cleanPath(sourceDir) + " (embedded at build time)"
	}
	return "(not set)"
}

func settingsEntries(m *model) []list.Item {
	s := m.state

	pool := "All images in the directories"
	if s.PoolMode == poolFavorites {
		pool = "Favorites only"
	}

	browserDir := s.BrowserDir
	if browserDir == "" {
		browserDir = "(not set, sync skipped)"
	}

	browserImage := s.BrowserImage
	if browserImage == "" {
		browserImage = "(not set)"
	}

	autostart := "Off"
	switch {
	case !autostartSupported:
		autostart = errAutostartText()
	case m.autostartErr != nil:
		autostart = m.autostartErr.Error()
	case m.autostart:
		autostart = "On: " + m.autostartPath
	}

	return []list.Item{
		entry{title: "Interval", desc: "Every " + formatInterval(s.SecondsToWait), action: "interval"},
		entry{title: "Pool", desc: pool + " (enter to toggle)", action: "pool"},
		entry{title: "Fit mode", desc: modeLabel(s.WallpaperMode), action: "mode"},
		entry{title: "Browser sync", desc: onOff(!s.DisableBrowserSync) + " (enter to toggle)", action: "sync"},
		entry{title: "Browser directory", desc: browserDir, action: "browserdir"},
		entry{title: "Browser image file", desc: browserImage, action: "browserimage"},
		entry{title: "Start with Windows", desc: autostart, action: "autostart"},
		entry{title: "Source directory", desc: sourceDirText(s), action: "sourcedir"},
	}
}

func modeEntries(state *State) []list.Item {
	items := []list.Item{}
	for _, name := range wallpaperModeNames {
		title := modeLabel(name)
		if name == state.WallpaperMode {
			title = "● " + title
		}
		items = append(items, entry{title: title, desc: modeDescription(name), action: name})
	}
	return items
}

func imageEntries(state *State) []list.Item {
	images := getImagesFromDirectories(nil, state.Blacklist, state.ImagesDirectories)
	sort.Slice(images, func(i, j int) bool { return pathKey(images[i]) < pathKey(images[j]) })

	favs := newPathSet(state.Favorites)
	used := newPathSet(state.UsedInCycle)
	current := pathKey(currentImage(state))

	items := make([]list.Item, 0, len(images))
	for _, img := range images {
		items = append(items, entry{title: imageTitle(img, favs, current), desc: imageDesc(img, used), path: img})
	}
	return items
}

func historyEntries(state *State) []list.Item {
	favs := newPathSet(state.Favorites)
	items := []list.Item{}

	for j := len(state.Forward) - 1; j >= 0; j-- {
		p := state.Forward[j]
		items = append(items, entry{title: "↷ " + imageTitle(p, favs, ""), desc: fmt.Sprintf("redo +%d · %s", j+1, filepath.Dir(p)), path: p, offset: j + 1})
	}

	n := len(state.ImagePaths)
	for i := n - 1; i >= 0; i-- {
		p := state.ImagePaths[i]
		offset := i - (n - 1)
		desc := fmt.Sprintf("%d · %s", offset, filepath.Dir(p))
		title := imageTitle(p, favs, "")
		if offset == 0 {
			title, desc = "● "+title, "current · "+filepath.Dir(p)
		}
		items = append(items, entry{title: title, desc: desc, path: p, offset: offset})
	}

	return items
}

func pathEntries(paths []string, state *State, kind string) []list.Item {
	favs := newPathSet(state.Favorites)
	current := pathKey(currentImage(state))
	items := []list.Item{}

	for _, p := range paths {
		desc := filepath.Dir(p)
		if !fileExists(p) {
			desc = "missing · " + desc
		}
		title := filepath.Base(p)
		if kind == "favorite" {
			title = imageTitle(p, favs, current)
		}
		items = append(items, entry{title: title, desc: desc, path: p})
	}

	return items
}

func dirEntries(state *State) []list.Item {
	items := []list.Item{}
	for _, d := range computeStats(state).Dirs {
		desc := fmt.Sprintf("%d images · %d used · %d remaining", d.Total, d.Used, d.Remaining)
		if !d.Exists {
			desc = "missing directory"
		}
		items = append(items, entry{title: d.Dir, desc: desc, path: d.Dir})
	}
	return items
}

func imageTitle(p string, favs map[string]bool, current string) string {
	title := filepath.Base(p)
	if favs[pathKey(p)] {
		title = "★ " + title
	}
	if current != "" && pathKey(p) == current {
		title += "  (current)"
	}
	return title
}

func imageDesc(p string, used map[string]bool) string {
	if used[pathKey(p)] {
		return "used · " + filepath.Dir(p)
	}
	return filepath.Dir(p)
}

func statsText(state *State) string {
	st := computeStats(state)

	var sb strings.Builder
	row := func(label string, value interface{}) {
		fmt.Fprintf(&sb, "  %-22s %v\n", styleMuted.Render(label), value)
	}

	sb.WriteString(styleBold.Render("Overview") + "\n\n")
	row("Images", st.Total)
	row("Used this cycle", st.Used)
	row("Remaining this cycle", st.Remaining)
	row("Favorites", st.Favorites)
	row("Blacklisted", st.Blacklisted)
	row("History entries", st.History)
	row("Redo entries", st.Forward)
	row("Pool", state.PoolMode)
	row("Interval", formatInterval(state.SecondsToWait))
	row("Last remix", formatTime(state.LastRunAt))

	if st.Total > 0 {
		row("Cycle progress", progressBar(st.Used, st.Total, 30))
	}

	sb.WriteString("\n" + styleBold.Render("Directories") + "\n\n")
	if len(st.Dirs) == 0 {
		sb.WriteString(styleMuted.Render("  No directories configured.") + "\n")
	}
	for _, d := range st.Dirs {
		if !d.Exists {
			fmt.Fprintf(&sb, "  %s %s\n", styleErr.Render("missing"), d.Dir)
			continue
		}
		fmt.Fprintf(&sb, "  %s  %d images, %d used, %d remaining\n  %s\n\n", d.Dir, d.Total, d.Used, d.Remaining, progressBar(d.Used, d.Total, 30))
	}

	return sb.String()
}

// ---------- view ----------

func (m *model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	return lipgloss.JoinVertical(lipgloss.Left, m.headerView(), m.bodyView(), m.footerView())
}

func (m *model) headerView() string {
	s := m.state

	loop := styleMuted.Render("○ loop stopped")
	if m.loop.Running {
		loop = styleOK.Render("● loop running"+pidText(m.loop.PID)) + styleMuted.Render(" · "+countdownText(s))
	}

	current := currentImage(s)
	currentText := styleMuted.Render("no wallpaper set yet")
	if current != "" {
		currentText = styleBold.Render(filepath.Base(current)) + styleMuted.Render("  "+filepath.Dir(current))
		if containsPath(s.Favorites, current) {
			currentText = styleFav.Render("★ ") + currentText
		}
	}

	settings := styleMuted.Render(fmt.Sprintf("every %s · pool %s · sync %s · fit %s",
		formatInterval(s.SecondsToWait), s.PoolMode, onOff(browserSyncEnabled(s)), modeLabel(s.WallpaperMode)))

	line1 := styleTitle.Render("wallpaper-remixer") + "  " + loop
	line2 := currentText + "   " + settings
	return styleHeader.Width(m.width).Render(ansi.Truncate(line1, m.width, "…") + "\n" + ansi.Truncate(line2, m.width, "…"))
}

func (m *model) footerView() string {
	var text string
	switch {
	case m.busy != "":
		text = m.spinner.View() + " " + m.busy + "..."
	case m.loading:
		text = m.spinner.View() + " loading..."
	case m.status != "" && m.statusErr:
		text = styleErr.Render("✗ " + m.status)
	case m.status != "":
		text = styleOK.Render("✓ " + m.status)
	}

	hint := styleMuted.Render(m.navHint())
	textWidth := max(0, m.width-lipgloss.Width(hint)-2)
	text = ansi.Truncate(text, textWidth, "…")
	gap := strings.Repeat(" ", max(1, m.width-lipgloss.Width(text)-lipgloss.Width(hint)))
	return ansi.Truncate(text+gap+hint, m.width, "")
}

func (m *model) bodyView() string {
	height := m.bodyHeight()
	// Max* truncate instead of wrapping, so an overflowing line never spills onto the next row.
	style := lipgloss.NewStyle().Height(height).MaxHeight(height).MaxWidth(m.width)

	switch m.screen {
	case screenInput:
		box := styleBox.Render(styleBold.Render(m.inputTitle) + "\n" + styleMuted.Render(m.inputHint) + "\n\n" + m.input.View() + "\n\n" + styleMuted.Render("enter save · esc cancel"))
		return style.Render(lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box))
	case screenConfirm:
		box := styleBox.Render(styleBold.Render(m.confirmText) + "\n\n" + styleMuted.Render("y/enter yes · n/esc no"))
		return style.Render(lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box))
	case screenStats, screenLogs:
		hint := styleMuted.Render("  r refresh · esc back · ↑/↓ scroll")
		m.viewport.Height = height - 1
		return style.Render(m.viewport.View() + "\n" + hint)
	}

	if !m.previewVisible() {
		return style.Render(m.list.View())
	}

	listWidth := m.listWidth()
	listView := lipgloss.NewStyle().Width(listWidth).MaxWidth(listWidth).Height(height).MaxHeight(height).Render(m.list.View())
	return style.Render(lipgloss.JoinHorizontal(lipgloss.Top, listView, m.previewView()))
}

// previewView renders the preview in its own bordered pane, the art centered inside.
func (m *model) previewView() string {
	paneWidth, height := m.previewPaneWidth(), m.bodyHeight()
	cols, rows := m.previewSize()
	style := stylePane.Width(paneWidth - 2).Height(height - 2).MaxWidth(paneWidth).MaxHeight(height)
	center := func(s string) string { return lipgloss.Place(cols, rows+2, lipgloss.Center, lipgloss.Center, s) }

	e, ok := m.selected()
	if !ok {
		return style.Render(center(styleMuted.Render("No image selected")))
	}

	res, cached := m.previews[m.previewCacheKey(e.path)]
	switch {
	case !cached:
		return style.Render(center(styleMuted.Render("Rendering preview...")))
	case res.err != nil:
		return style.Render(center(styleErr.Width(cols).Render("Preview unavailable: " + res.err.Error())))
	}

	art := lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center, res.preview.Art)
	info := lipgloss.PlaceHorizontal(cols, lipgloss.Center, styleMuted.Render(fmt.Sprintf("%d×%d · %s", res.preview.Width, res.preview.Height, formatBytes(res.preview.Size))))
	return style.Render(art + "\n\n" + info)
}

// ---------- formatting ----------

func currentName() string {
	state, err := readState()
	if err != nil || currentImage(state) == "" {
		return "(none)"
	}
	return filepath.Base(currentImage(state))
}

func countdownText(state *State) string {
	left := time.Until(nextRemixAt(state)).Round(time.Second)
	if left <= 0 {
		return "remix due"
	}
	return "next in " + formatInterval(int(left/time.Second))
}

func pidText(pid int) string {
	if pid <= 0 {
		return ""
	}
	return fmt.Sprintf(" (pid %d)", pid)
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func modeLabel(mode string) string {
	if mode == "" {
		return "system default"
	}
	return mode
}

func modeDescription(mode string) string {
	switch mode {
	case "":
		return "Do not touch the fit mode configured in the system"
	case "fill":
		return "Cover the screen, cropping the edges"
	case "fit":
		return "Show the whole image, with bars if needed"
	case "stretch":
		return "Stretch to the screen, ignoring the aspect ratio"
	case "center":
		return "Original size, centered"
	case "tile":
		return "Repeat the image"
	case "span":
		return "One image across all monitors"
	}
	return ""
}

func errAutostartText() string {
	return "Not supported on this OS"
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format("2006-01-02 15:04:05")
}

func progressBar(done, total, width int) string {
	if total <= 0 {
		return ""
	}
	filled := done * width / total
	return styleOK.Render(strings.Repeat("█", filled)) + styleMuted.Render(strings.Repeat("░", width-filled)) + fmt.Sprintf(" %d%%", done*100/total)
}
