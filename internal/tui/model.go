package tui

import (
	"fmt"
	"sort"
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

	agentsRegistered bool

	focusDetail  bool
	tab          tabID
	scroll       int // -1 = follow the tail
	art          artifact
	confirmAbort bool
	showHelp     bool
	flash        string
	flashUntil   time.Time

	// exitHint is printed by the watch command after the program exits —
	// the deep link out of the monitor (accept/reject stay in the CLI).
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
	if reg, err := loop.LoadAgents(m.root); err == nil {
		m.agentsRegistered = len(reg.Agents) > 0
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
	// loop that most needs eyes.
	m.selected = 0
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
	m.selected = clamp(m.selected+delta, 0, len(m.loops)-1)
	m.selectedID = m.loops[m.selected].ID
	m.scroll = -1
	m.reload()
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
		m.setTab(tabID(key[0] - '1'))
		return m, nil
	case "p":
		m.requestPause()
		return m, nil
	case "r":
		m.requestResume()
		return m, nil
	case "a":
		if v := m.current(); v != nil && !done(*v) {
			m.confirmAbort = true
		} else {
			m.say("nothing to abort")
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
// spacer, tab bar.
const detailFixedRows = 6

// bodyRows mirrors the frame's layout math: header + two rules + footer eat
// four rows; the detail header eats detailFixedRows more.
func (m model) bodyRows() int {
	return m.height - 4 - detailFixedRows
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
	if m.width >= collapseWidth && (len(m.loops) > 0 || len(m.broken) > 0) {
		return m.width - railWidth(m.loops, m.broken) - 2
	}
	return m.width - 1
}

func (m model) frameState() frameState {
	s := frameState{
		width:            m.width,
		height:           m.height,
		color:            m.color,
		loops:            m.loops,
		broken:           m.broken,
		selected:         m.selected,
		agentsRegistered: m.agentsRegistered,
		focusDetail:      m.focusDetail,
		tab:              m.tab,
		scroll:           m.scroll,
		art:              m.art,
		confirmAbort:     m.confirmAbort,
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
	v := tea.NewView(renderFrame(m.frameState()))
	v.AltScreen = true
	return v
}

func done(v loop.LoopView) bool {
	switch v.Status {
	case loop.StatusGreen, loop.StatusParked, loop.StatusAccepted, loop.StatusRejected:
		return true
	}
	return false
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
