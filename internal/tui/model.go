package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mjbarefo/loopy/internal/loop"
)

// refreshInterval is how often the monitor re-reads state files and log
// tails. The engine flushes at phase boundaries; half a second keeps tailing
// lively without hammering the disk.
const refreshInterval = 500 * time.Millisecond

const flashDuration = 3 * time.Second

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type model struct {
	root  string
	color bool

	width, height int

	loops      []loop.LoopView
	broken     []loop.BrokenLoop
	selected   int
	selectedID string // selection is sticky by ID across reloads
	loadErr    string

	welcome          bool
	initialized      bool
	agentsRegistered bool
	detected         []loop.AgentSuggestion
	form             formState

	focusDetail   bool
	tab           tabID
	scroll        int // -1 = follow the tail
	art           artifact
	confirmAbort  bool
	confirmDelete bool
	confirmAccept bool
	confirmReject bool
	showHelp      bool
	flash         string
	flashUntil    time.Time

	// exitHint is printed by the watch command after the program exits —
	// the deep link out of the monitor (o hands off the next command).
	exitHint string
}

func newModel(root, loopID string, color bool) model {
	m := model{
		root:       root,
		color:      color,
		selectedID: loopID,
		scroll:     -1,
		width:      80,
		height:     24,
	}
	m.reload()
	return m
}

// statusRank orders the rail by what needs eyes first: live work, then loops
// waiting on the human (paused, dead engines, green to review), then the
// parked and decided history.
func statusRank(v loop.LoopView) int {
	switch v.Status {
	case loop.StatusRunning:
		if v.Live {
			return 0
		}
		return 2 // says running but nothing is — needs attention
	case loop.StatusPaused:
		return 1
	case loop.StatusGreen:
		return 3
	case loop.StatusParked:
		return 4
	case loop.StatusAccepted:
		return 5
	default: // rejected
		return 6
	}
}

// reload re-reads every loop and the selected tab's artifact from disk. The
// monitor holds no state of its own — disk is the truth, every time.
func (m *model) reload() {
	views, broken, err := loadLoops(m.root)
	if err != nil {
		m.loadErr = errText(err)
		return
	}
	m.loadErr = ""
	m.loops = views
	m.broken = broken
	m.initialized = loop.EnsureInitialized(m.root) == nil
	if reg, err := loop.LoadAgents(m.root); err == nil {
		m.agentsRegistered = len(reg.Agents) > 0
	}
	if len(views) == 0 && m.initialized && !m.agentsRegistered {
		m.detected = loop.DetectAgentCLIs(m.root)
	} else {
		m.detected = nil
	}
	sort.SliceStable(m.loops, func(i, j int) bool {
		ri, rj := statusRank(m.loops[i]), statusRank(m.loops[j])
		if ri != rj {
			return ri < rj
		}
		// Newest first within a group; ListLoops is oldest-first.
		return m.loops[i].CreatedAt > m.loops[j].CreatedAt
	})
	if len(m.loops) == 0 {
		m.selected = 0
		m.selectedID = ""
		m.art = artifact{}
		return
	}
	// Re-find the sticky selection; default to the top of the rail — the
	// first loop that needs eyes (decided history is hidden there).
	m.selected = 0
	for i, v := range m.loops {
		if railVisible(v) {
			m.selected = i
			break
		}
	}
	for i, v := range m.loops {
		if v.ID == m.selectedID {
			m.selected = i
			break
		}
	}
	m.selectedID = m.loops[m.selected].ID
	m.art = loadTabArtifact(m.root, m.loops[m.selected], m.tab)
	if m.flash != "" && time.Now().After(m.flashUntil) {
		m.flash = ""
	}
}

func (m *model) selectLoop(delta int) {
	if len(m.loops) == 0 {
		return
	}
	m.selected = nextVisible(m.loops, m.selected, delta)
	m.selectedID = m.loops[m.selected].ID
	m.scroll = -1
	m.reload()
}

// nextVisible moves the selection delta steps over rail-visible loops —
// decided history is skipped, matching what the rail renders.
func nextVisible(loops []loop.LoopView, from, delta int) int {
	step := 1
	if delta < 0 {
		step = -1
		delta = -delta
	}
	idx := from
	for n := delta; n > 0; n-- {
		j := idx + step
		for j >= 0 && j < len(loops) && !railVisible(loops[j]) {
			j += step
		}
		if j < 0 || j >= len(loops) {
			break
		}
		idx = j
	}
	return idx
}

func (m *model) setTab(t tabID) {
	m.tab = t
	m.scroll = -1
	m.reload()
}

func (m *model) say(format string, args ...any) {
	m.flash = fmt.Sprintf(format, args...)
	m.flashUntil = time.Now().Add(flashDuration)
}

func (m model) current() *loop.LoopView {
	if m.selected >= 0 && m.selected < len(m.loops) {
		return &m.loops[m.selected]
	}
	return nil
}

func (m model) Init() tea.Cmd { return tick() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		m.reload()
		return m, tick()
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// The welcome splash: any key enters the monitor (q still quits).
	if m.welcome {
		m.welcome = false
		if key == "q" || key == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	if m.form.active {
		return m.handleFormKey(msg)
	}

	if m.confirmAbort {
		switch key {
		case "y":
			m.requestAbort()
		case "n", "esc", "q", "ctrl+c":
			m.say("abort cancelled")
		}
		m.confirmAbort = false
		return m, nil
	}
	if m.confirmDelete {
		switch key {
		case "y":
			m.requestDelete()
		case "n", "esc", "q", "ctrl+c":
			m.say("delete cancelled")
		}
		m.confirmDelete = false
		return m, nil
	}
	if m.confirmAccept {
		switch key {
		case "y":
			m.requestAccept()
		case "n", "esc", "q", "ctrl+c":
			m.say("accept cancelled")
		}
		m.confirmAccept = false
		return m, nil
	}
	if m.confirmReject {
		switch key {
		case "y":
			m.requestReject()
		case "n", "esc", "q", "ctrl+c":
			m.say("reject cancelled")
		}
		m.confirmReject = false
		return m, nil
	}

	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "esc":
		switch {
		case m.showHelp:
			m.showHelp = false
		case m.focusDetail:
			m.focusDetail = false
		}
		return m, nil
	case "enter":
		m.focusDetail = true
		return m, nil
	case "up", "k":
		if m.focusDetail {
			m.scrollBy(-1)
		} else {
			m.selectLoop(-1)
		}
		return m, nil
	case "down", "j":
		if m.focusDetail {
			m.scrollBy(1)
		} else {
			m.selectLoop(1)
		}
		return m, nil
	case "pgup":
		m.scrollBy(-m.bodyRows())
		return m, nil
	case "pgdown":
		m.scrollBy(m.bodyRows())
		return m, nil
	case "g", "home":
		m.scroll = 0
		return m, nil
	case "G", "shift+g", "end":
		m.scroll = -1
		return m, nil
	case "tab":
		m.setTab((m.tab + 1) % tabCount)
		return m, nil
	case "shift+tab":
		m.setTab((m.tab + tabCount - 1) % tabCount)
		return m, nil
	case "1", "2", "3", "4":
		if m.current() != nil {
			m.setTab(tabID(key[0] - '1'))
			return m, nil
		}
		// Onboarding: digits register the detected agent CLIs.
		idx := int(key[0] - '1')
		if idx < len(m.detected) {
			d := m.detected[idx]
			if err := loop.AddAgent(m.root, d.Name, d.Cmd, !m.agentsRegistered); err != nil {
				m.say("could not register %s: %v", d.Name, err)
				return m, nil
			}
			m.say("registered agent %s", d.Name)
			m.reload()
		}
		return m, nil
	case "n":
		if !m.initialized {
			m.say("initialize the repo first — press i")
			return m, nil
		}
		// No agent yet is fine: the wizard's agent step registers one.
		m.form = openForm(m.root)
		return m, nil
	case "i":
		if m.initialized {
			return m, nil
		}
		if _, _, err := loop.InitProject(m.root); err != nil {
			m.say("init failed: %v", err)
			return m, nil
		}
		m.say("initialized .loopy/ (and git-ignored it — commit that)")
		m.reload()
		return m, nil
	case "p":
		m.requestPause()
		return m, nil
	case "r":
		// Contextual: reject judges a parked loop; everything else is resume.
		switch v := m.current(); {
		case v != nil && (v.Status == loop.StatusGreen || v.Status == loop.StatusParked):
			m.confirmReject = true
		case v != nil && decided(*v):
			m.say("%s is already %s", v.ID, v.Status)
		default:
			m.requestResume()
		}
		return m, nil
	case "a":
		// Contextual: abort stops a moving loop; accept judges a green one.
		// Accepting a parked red loop is an override and stays in the CLI.
		switch v := m.current(); {
		case v == nil:
			m.say("nothing to abort")
		case !done(*v):
			m.confirmAbort = true
		case v.Status == loop.StatusGreen:
			m.confirmAccept = true
		case v.Status == loop.StatusParked:
			m.say("%s is not green — accepting it is an override: loopy accept %s --override --reason …", v.ID, v.ID)
		default:
			m.say("%s is already %s", v.ID, v.Status)
		}
		return m, nil
	case "d":
		switch v := m.current(); {
		case v == nil:
			m.say("nothing to delete")
		case v.Live:
			m.say("a live engine holds %s — abort it first (a)", v.ID)
		default:
			m.confirmDelete = true
		}
		return m, nil
	case "o":
		if v := m.current(); v != nil && v.NextCommand != "" {
			m.exitHint = v.NextCommand
		}
		return m, tea.Quit
	}
	return m, nil
}

// Control actions. The monitor's only writes are control.json; everything
// else is a hint pointing at the audited CLI.

func (m *model) requestPause() {
	v := m.current()
	switch {
	case v == nil || done(*v):
		m.say("nothing to pause")
	case v.Status == loop.StatusPaused:
		m.say("%s is already paused", v.ID)
	case !v.Live:
		m.say("no live engine — nothing is running")
	default:
		if err := loop.WriteControl(m.root, v.ID, loop.Control{Pause: true}); err != nil {
			m.say("pause failed: %v", err)
			return
		}
		m.say("pause requested — honored at the next iteration boundary")
	}
}

func (m *model) requestResume() {
	v := m.current()
	switch {
	case v == nil || done(*v):
		m.say("nothing to resume")
	case v.Live:
		// A live engine with a pending pause: cancelling the request is the
		// resume.
		if ctrl, err := loop.ReadControl(m.root, v.ID); err == nil && ctrl.Pause {
			if err := loop.ClearControl(m.root, v.ID); err != nil {
				m.say("cancel failed: %v", err)
				return
			}
			m.say("pending pause cancelled")
			return
		}
		m.say("%s is already running", v.ID)
	default:
		// Paused (or crashed) with no engine: spawn one. The child is a
		// normal `loopy resume` — the engine lock keeps this race-safe.
		if err := spawnResume(m.root, v.ID); err != nil {
			m.say("resume failed: %v", err)
			return
		}
		m.say("engine started: loopy resume %s", v.ID)
	}
}

func (m *model) requestAbort() {
	v := m.current()
	if v == nil || done(*v) {
		m.say("nothing to abort")
		return
	}
	if !v.Live {
		// No engine to honor the request; parking is loop-state surgery that
		// belongs to the audited CLI path.
		m.say("no live engine — run: loopy abort %s", v.ID)
		return
	}
	if err := loop.WriteControl(m.root, v.ID, loop.Control{Abort: true, Reason: "aborted from the monitor"}); err != nil {
		m.say("abort failed: %v", err)
		return
	}
	m.say("abort requested — the engine stops within seconds")
}

// requestDelete shells out to the audited CLI (`loopy delete <id>`): the
// monitor itself never writes loop state, same rule as resume spawning a
// normal engine.
func (m *model) requestDelete() {
	v := m.current()
	if v == nil {
		m.say("nothing to delete")
		return
	}
	if out, err := runCLI(m.root, "delete", v.ID); err != nil {
		m.say("delete failed: %s", firstLine(out, err))
		return
	}
	m.say("deleted %s — the logbook keeps the record", v.ID)
	m.selectedID = ""
	m.reload()
}

// requestAccept and requestReject record the judgment through the audited
// CLI, same rule as delete. Decided loops leave the rail; the selection
// falls back to the next loop that needs eyes.
func (m *model) requestAccept() {
	v := m.current()
	if v == nil {
		m.say("nothing to accept")
		return
	}
	if out, err := runCLI(m.root, "accept", v.ID); err != nil {
		m.say("accept failed: %s", firstLine(out, err))
		return
	}
	m.say("accepted %s — final-diff.patch is the durable record", v.ID)
	m.selectedID = ""
	m.reload()
}

func (m *model) requestReject() {
	v := m.current()
	if v == nil {
		m.say("nothing to reject")
		return
	}
	if out, err := runCLI(m.root, "reject", v.ID); err != nil {
		m.say("reject failed: %s", firstLine(out, err))
		return
	}
	m.say("rejected %s — evidence kept, worktree freed", v.ID)
	m.selectedID = ""
	m.reload()
}

// firstLine flattens a child command's output (or its error) to one flash.
func firstLine(out string, err error) string {
	if t := strings.TrimSpace(out); t != "" {
		if i := strings.IndexByte(t, '\n'); i >= 0 {
			t = t[:i]
		}
		return t
	}
	return err.Error()
}

func (m *model) scrollBy(delta int) {
	lines := m.bodyLineCount()
	rows := m.bodyRows()
	maxStart := lines - rows
	if maxStart < 0 {
		maxStart = 0
	}
	cur := m.scroll
	if cur < 0 {
		cur = maxStart
	}
	next := clamp(cur+delta, 0, maxStart)
	if next >= maxStart {
		next = -1 // back on the tail
	}
	m.scroll = next
}

// detailFixedRows mirrors the frame's layout: status, goal, meta, activity,
// spacer, nav bar.
const detailFixedRows = 6

// bodyRows asks the frame's own geometry how many body rows remain after
// the chrome and the fixed detail header.
func (m model) bodyRows() int {
	return m.frameState().contentRows() - detailFixedRows
}

func (m model) bodyLineCount() int {
	s := m.frameState()
	if m.tab == tabOverview {
		if v := m.current(); v != nil {
			return len(overviewBody(s, *v, m.detailWidth()))
		}
		return 0
	}
	return len(artifactBody(s, m.detailWidth())) // banner + lines
}

func (m model) detailWidth() int {
	_, detailW := m.frameState().railArea()
	return detailW
}

func (m model) frameState() frameState {
	s := frameState{
		width:            m.width,
		height:           m.height,
		color:            m.color,
		loops:            m.loops,
		broken:           m.broken,
		selected:         m.selected,
		initialized:      m.initialized,
		agentsRegistered: m.agentsRegistered,
		detected:         m.detected,
		form:             m.form,
		focusDetail:      m.focusDetail,
		tab:              m.tab,
		scroll:           m.scroll,
		art:              m.art,
		confirmAbort:     m.confirmAbort,
		confirmDelete:    m.confirmDelete,
		confirmAccept:    m.confirmAccept,
		confirmReject:    m.confirmReject,
		flash:            m.flash,
		showHelp:         m.showHelp,
		loadErr:          m.loadErr,
	}
	// The elapsed clock is the model's: the renderer stays deterministic.
	if v := m.current(); v != nil && v.Live && v.PhaseStartedAt != "" {
		if started, err := time.Parse(time.RFC3339, v.PhaseStartedAt); err == nil {
			if d := time.Since(started); d > 0 {
				s.phaseElapsed = d.Round(time.Second).String()
			}
		}
	}
	return s
}

func (m model) View() tea.View {
	frame := renderFrame(m.frameState())
	if m.welcome {
		frame = welcomeFrame(m.frameState(), m.root)
	}
	v := tea.NewView(frame)
	v.AltScreen = true
	return v
}

// handleFormKey drives the new-loop wizard. Typing uses the key's text (so
// letters that are commands elsewhere, like q and p, spell the goal); enter
// advances a step, esc walks back, and esc on the first step cancels.
func (m model) handleFormKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.form.step == stepGoal {
			m.form = formState{}
		} else {
			m.form.step--
		}
		return m, nil
	case "enter":
		return m.wizardAdvance()
	}

	switch m.form.step {
	case stepGoal:
		m.form.goal = editText(m.form.goal, key, msg.Text, 300)
	case stepAgent:
		n := len(m.form.agents)
		if n == 0 {
			n = len(m.form.detected)
		}
		switch key {
		case "up", "k":
			m.form.cursor = clamp(m.form.cursor-1, 0, max(n-1, 0))
		case "down", "j":
			m.form.cursor = clamp(m.form.cursor+1, 0, max(n-1, 0))
		case "space":
			if len(m.form.agents) > 0 {
				m.form.picked[m.form.cursor] = !m.form.picked[m.form.cursor]
			}
		}
	case stepVerifier:
		if next := editText(m.form.verifier, key, msg.Text, 500); next != m.form.verifier {
			m.form.verifier = next
			m.form.edited = true
		}
	case stepBudget:
		switch key {
		case "up", "down", "tab", "k", "j":
			m.form.budgetField = 1 - m.form.budgetField
		default:
			if m.form.budgetField == 0 {
				m.form.iters = editText(m.form.iters, key, msg.Text, 6)
			} else {
				m.form.wall = editText(m.form.wall, key, msg.Text, 12)
			}
		}
	}
	return m, nil
}

// wizardAdvance validates the current step and moves forward; the last step
// starts the loop(s).
func (m model) wizardAdvance() (tea.Model, tea.Cmd) {
	f := &m.form
	switch f.step {
	case stepGoal:
		if strings.TrimSpace(f.goal) == "" {
			m.say("a goal is required — describe what done looks like")
			return m, nil
		}
	case stepAgent:
		switch {
		case len(f.agents) > 0:
			// The cursor is the choice when nothing is marked.
		case f.cursor < len(f.detected):
			d := f.detected[f.cursor]
			if err := loop.AddAgent(m.root, d.Name, d.Cmd, true); err != nil {
				m.say("could not register %s: %v", d.Name, err)
				return m, nil
			}
			f.agents = []string{d.Name}
			f.defaultAgent = d.Name
			f.cursor = 0
			m.agentsRegistered = true
			m.say("registered agent %s (default)", d.Name)
		default:
			m.say("no agent to continue with — register one: loopy agent add …")
			return m, nil
		}
	case stepVerifier:
		if strings.TrimSpace(f.verifier) == "" {
			m.say("no verifier, no loop — type the command that proves the goal")
			return m, nil
		}
	case stepBudget:
		if _, err := strconv.Atoi(strings.TrimSpace(f.iters)); err != nil {
			m.say("iterations must be a number")
			return m, nil
		}
		if _, err := time.ParseDuration(strings.TrimSpace(f.wall)); err != nil {
			m.say("wall clock must be a duration like 30m or 2h")
			return m, nil
		}
	case stepConfirm:
		ids, err := startLoops(m.root, m.form)
		if err != nil {
			m.say("%v", err)
			return m, nil
		}
		m.form = formState{}
		m.selectedID = ids[0]
		m.tab = tabOverview
		if len(ids) > 1 {
			m.say("racing %d loops — when all park: loopy judge %s", len(ids), strings.Join(ids, " "))
		} else {
			m.say("loop %s started", ids[0])
		}
		m.reload()
		return m, nil
	}
	f.step++
	return m, nil
}

// editText is the wizard's shared one-line text editing: printable text
// appends, backspace deletes a rune, ctrl+u clears.
func editText(value, key, text string, limit int) string {
	switch key {
	case "backspace":
		r := []rune(value)
		if len(r) > 0 {
			return string(r[:len(r)-1])
		}
		return value
	case "ctrl+u":
		return ""
	case "space":
		text = " "
	}
	if text != "" && len(value) < limit {
		return value + text
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func done(v loop.LoopView) bool {
	switch v.Status {
	case loop.StatusGreen, loop.StatusParked, loop.StatusAccepted, loop.StatusRejected:
		return true
	}
	return false
}

func decided(v loop.LoopView) bool {
	return v.Status == loop.StatusAccepted || v.Status == loop.StatusRejected
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
