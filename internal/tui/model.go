package tui

import (
	"context"
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

// synthDoneMsg carries a verifier proposal back into the wizard. seq pairs
// it with the request that started it; stale results (after esc) are dropped.
type synthDoneMsg struct {
	seq int
	res loop.SynthesisResult
	err error
}

func synthesizeCmd(root, agent, goal string, seq int) tea.Cmd {
	return func() tea.Msg {
		res, err := loop.SynthesizeVerifier(context.Background(), root, agent, goal)
		return synthDoneMsg{seq: seq, res: res, err: err}
	}
}

// confirmKind names the one confirmation that may be pending at any time.
// The values are mutually exclusive: each key handler sets at most one and
// handleKey clears it on the next keypress.
type confirmKind int

const (
	confirmNone   confirmKind = iota
	confirmAbort              // 'a' on a live loop
	confirmDelete             // 'd' on a stopped loop
	confirmAccept             // 'a' on a green loop
	confirmReject             // 'r' on a parked/green loop
	confirmApply              // 'A' to apply an accepted diff
)

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

	focusDetail bool
	tab         tabID
	scroll      int // -1 = follow the tail
	art         artifact
	confirm     confirmKind
	// applyID/applyPath capture the apply target when confirm == confirmApply:
	// an accepted loop leaves the rail, so the target is usually the newest
	// accepted loop, not the current selection.
	applyID    string
	applyPath  string
	showHelp   bool
	flash      string
	flashUntil time.Time

	// exitHint is printed by the watch command after the program exits —
	// the deep link out of the monitor (o hands off the next command).
	exitHint string

	// deleteLoop is the seam tests use to observe the apply→delete coupling
	// without shelling out. Nil in production: the real delete goes through the
	// audited CLI (`loopy delete`), like every other monitor decision.
	deleteLoop func(root, id string) (string, error)
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
	if m.flash != "" && time.Now().After(m.flashUntil) {
		m.flash = ""
	}
	if len(m.loops) == 0 {
		m.selected = -1
		m.selectedID = ""
		m.art = artifact{}
		return
	}
	m.selected = reselect(m.loops, m.selectedID)
	if m.selected < 0 {
		// Every loop is decided and none is pinned: nothing needs eyes.
		// The rail goes quiet instead of re-pinning the last decision —
		// a just-accepted loop lingering looked like the accept failed.
		m.selectedID = ""
		m.art = artifact{}
		return
	}
	m.selectedID = m.loops[m.selected].ID
	m.art = loadTabArtifact(m.root, m.loops[m.selected], m.tab)
}

// reselect re-finds the sticky selection by ID (so `loopy watch <id>` can
// pin a decided loop); otherwise the top of the rail — the first loop that
// needs eyes. -1 when every loop is decided and nothing is pinned.
func reselect(loops []loop.LoopView, stickyID string) int {
	for i, v := range loops {
		if stickyID != "" && v.ID == stickyID {
			return i
		}
	}
	for i, v := range loops {
		if railVisible(v) {
			return i
		}
	}
	return -1
}

func (m *model) selectLoop(delta int) {
	if len(m.loops) == 0 {
		return
	}
	next := nextVisible(m.loops, m.selected, delta)
	if next < 0 || next >= len(m.loops) {
		return // a quiet rail has nothing to select
	}
	m.selectTo(next)
}

// selectTo moves the selection to a specific loop (rail clicks land here).
// It re-points the artifact only; the tick owns re-reading the loops.
func (m *model) selectTo(i int) {
	if i < 0 || i >= len(m.loops) {
		return
	}
	m.selected = i
	m.selectedID = m.loops[i].ID
	m.resetScroll()
	m.reloadArtifact()
}

// reloadArtifact refreshes the detail pane's artifact for the current
// selection and tab.
func (m *model) reloadArtifact() {
	m.art = artifact{}
	if v := m.current(); v != nil {
		m.art = loadTabArtifact(m.root, *v, m.tab)
	}
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
	m.resetScroll()
	m.reloadArtifact()
}

// resetScroll is the tab's home position: live (and the overview's tail)
// follow the tail; the diff and verifier tabs lead with their answer-first
// header, so they open at the top.
func (m *model) resetScroll() {
	m.scroll = -1
	if m.tab == tabDiff || m.tab == tabVerifier {
		m.scroll = 0
	}
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
	case synthDoneMsg:
		return m.handleSynthDone(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseWheelMsg:
		return m.handleWheel(msg)
	case tea.MouseClickMsg:
		return m.handleClick(msg)
	}
	return m, nil
}

// handleWheel scrolls whatever pane sits under the pointer: the detail body
// by three lines per notch, the rail by one loop.
func (m model) handleWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.welcome || m.form.active {
		return m, nil
	}
	dir := 0
	switch msg.Button {
	case tea.MouseWheelUp:
		dir = -1
	case tea.MouseWheelDown:
		dir = 1
	default:
		return m, nil
	}
	switch hitTest(m.frameState(), msg.X, msg.Y).kind {
	case hitRail:
		m.selectLoop(dir)
	case hitDetail, hitTab:
		m.scrollBy(dir * 3)
	}
	return m, nil
}

// handleClick: a rail row selects its loop, a nav name switches the view, the
// detail body takes scroll focus. Decisions stay explicit — while a y/n
// confirm is pending, clicks are ignored; the wizard stays keyboard-driven.
func (m model) handleClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	if m.welcome {
		m.welcome = false
		return m, nil
	}
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}
	if m.form.active || m.confirm != confirmNone {
		return m, nil
	}
	switch t := hitTest(m.frameState(), msg.X, msg.Y); t.kind {
	case hitRail:
		if t.loopIdx >= 0 {
			m.focusDetail = false
			m.selectTo(t.loopIdx)
		}
	case hitTab:
		m.setTab(t.tab)
	case hitDetail:
		m.focusDetail = true
	}
	return m, nil
}

// handleSynthDone lands an agent's verifier proposal in the wizard's
// editable field. The human's enter remains the signature; a synthesized
// verifier is goal-specific and never stored as the project default
// (edited=true keeps it out of the confirm-once store path).
func (m model) handleSynthDone(msg synthDoneMsg) (tea.Model, tea.Cmd) {
	if !m.form.active || m.form.step != stepVerifier || msg.seq != m.form.synthSeq || !m.form.synthesizing {
		return m, nil // the wizard moved on; drop the stale result
	}
	m.form.synthesizing = false
	if msg.err != nil {
		m.say("verifier proposal failed: %v", msg.err)
		return m, nil
	}
	m.form.verifier = msg.res.Cmd
	m.form.edited = true
	m.form.proposedBy = msg.res.Agent
	m.form.synthGoal = strings.TrimSpace(m.form.goal)
	if msg.res.AlreadyGreen {
		m.say("warning: the proposal already passes — the goal may be done, or it may not test the goal")
	} else {
		m.say("proposed by %s — red right now, as it should be; enter is your sign-off", msg.res.Agent)
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

	if m.confirm != confirmNone {
		type confirmEntry struct {
			action func()
			cancel string
		}
		dispatch := map[confirmKind]confirmEntry{
			confirmAbort:  {m.requestAbort, "abort cancelled"},
			confirmDelete: {m.requestDelete, "delete cancelled"},
			confirmAccept: {m.requestAccept, "accept cancelled"},
			confirmReject: {m.requestReject, "reject cancelled"},
			confirmApply:  {m.runApply, "apply cancelled"},
		}
		if e, ok := dispatch[m.confirm]; ok {
			switch key {
			case "y":
				e.action()
			case "n", "esc", "q", "ctrl+c":
				m.say("%s", e.cancel)
			}
		}
		m.confirm = confirmNone
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
			m.confirm = confirmReject
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
			m.confirm = confirmAbort
		case v.Status == loop.StatusGreen:
			m.confirm = confirmAccept
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
			m.confirm = confirmDelete
		}
		return m, nil
	case "o":
		if v := m.current(); v != nil && v.NextCommand != "" {
			m.exitHint = v.NextCommand
		}
		return m, tea.Quit
	case "c":
		// Copy the next command via OSC 52. A running loop's next command is
		// this monitor itself — nothing worth copying; the quiet rail copies
		// the newest accepted loop's apply command (the one it shows).
		cmd := ""
		if v := m.current(); v != nil {
			if v.Status != loop.StatusRunning {
				cmd = v.NextCommand
			}
		} else if v := newestAcceptedWithCommand(m.loops); v != nil {
			cmd = v.NextCommand
		}
		if cmd == "" {
			m.say("nothing to copy — no next command here")
			return m, nil
		}
		m.say("sent to the clipboard: %s", cmd)
		return m, tea.SetClipboard(cmd)
	case "A":
		// Apply an accepted loop's diff to the working tree. This is the one
		// place the monitor touches the user's checkout, so it confirms first —
		// and it is git apply only: loopy never commits, pushes, or merges.
		m.requestApply()
		return m, nil
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
	id := v.ID
	if out, err := runCLI(m.root, "accept", id); err != nil {
		m.say("accept failed: %s", firstLine(out, err))
		return
	}
	m.say("accepted %s — final-diff.patch is the durable record", id)
	m.selectedID = ""
	m.reload()
	// The accepted loop left the rail; keep its next move in the flash —
	// the apply command is how the diff reaches a branch and a PR.
	for _, lv := range m.loops {
		if lv.ID == id && lv.NextCommand != "" {
			m.say("accepted %s — apply it: %s", id, lv.NextCommand)
			break
		}
	}
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

// applyTarget is the accepted loop whose durable diff the A key applies: a
// pinned accepted selection if there is one, otherwise the newest accepted
// loop (the same one the quiet rail shows and c copies). Nil when nothing has
// been accepted yet.
func (m *model) applyTarget() *loop.LoopView {
	if v := m.current(); v != nil && v.Status == loop.StatusAccepted && v.FinalDiffPath != "" {
		return v
	}
	return newestAcceptedWithCommand(m.loops)
}

// requestApply stages the apply target for a y/n confirmation. The actual
// git apply runs in runApply, so the captured path survives a selection that
// shifts under a background reload.
func (m *model) requestApply() {
	v := m.applyTarget()
	if v == nil || v.FinalDiffPath == "" {
		m.say("nothing to apply — accept a green loop first (a)")
		return
	}
	m.applyID = v.ID
	m.applyPath = v.FinalDiffPath
	m.confirm = confirmApply
}

// runApply applies the accepted loop's durable diff to the user's working
// tree, then removes the loop — applying is shipping, so the loop's job is
// done and it should leave the (already quiet) rail. The apply is loopy's only
// write to the checkout and deliberately the *weakest* one: git apply, never a
// commit, push, or merge (invariant 2). The delete runs ONLY after a clean
// apply, so a conflict leaves both the tree and the loop untouched; the logbook
// keeps a line either way. The diff is now in the working tree (and reaches git
// history on commit), so the removed evidence is no longer the only copy.
func (m *model) runApply() {
	if m.applyPath == "" {
		m.say("nothing to apply")
		return
	}
	if out, err := applyPatch(m.root, m.applyPath); err != nil {
		m.say("git apply failed (your tree is untouched): %s", firstLine(out, err))
		return
	}
	id := m.applyID
	if out, err := m.deleteViaCLI(id); err != nil {
		// The apply succeeded; only the cleanup didn't. Don't lose that.
		m.say("applied %s — review and commit it; removing the loop failed: %s", id, firstLine(out, err))
	} else {
		m.say("applied %s and removed the loop — review, commit, and open your PR", id)
	}
	m.selectedID = ""
	m.reload()
}

// applyPatch applies a patch to the working tree at root. Split out from
// runApply so the git mechanics are unit-testable without the delete (which
// shells out) that follows a clean apply.
func applyPatch(root, path string) (string, error) {
	return runGit(root, "apply", path)
}

// deleteViaCLI removes a loop through the audited CLI, the same path as the d
// key — the monitor never writes loop state itself. Tests inject deleteLoop to
// observe the call without spawning a process.
func (m *model) deleteViaCLI(id string) (string, error) {
	if m.deleteLoop != nil {
		return m.deleteLoop(m.root, id)
	}
	return runCLI(m.root, "delete", id)
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

// bodyRows asks the frame's own geometry how many body rows remain after
// the chrome and the detail header (whose goal and activity lines wrap, so
// its height is counted, not assumed).
func (m model) bodyRows() int {
	s := m.frameState()
	rows := s.contentRows()
	if v := m.current(); v != nil {
		_, detailW := s.railArea()
		return rows - len(detailHeaderLines(s, *v, detailW))
	}
	return rows - 6
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
		confirm:          m.confirm,
		applyID:          m.applyID,
		flash:            m.flash,
		showHelp:         m.showHelp,
		loadErr:          m.loadErr,
	}
	// The elapsed clocks are the model's: the renderer stays deterministic.
	if m.form.synthesizing {
		s.synthElapsed = time.Since(m.form.synthStarted).Round(time.Second).String()
	}
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
	// Cell-motion mouse: wheel scrolls, clicks select. The terminal's native
	// text selection needs a modifier while this is on (Option/Shift); the c
	// key covers the main copy need (the next command) via OSC 52.
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// handleFormKey drives the new-loop wizard. Typing uses the key's text (so
// letters that are commands elsewhere, like q and p, spell the goal); enter
// advances a step, esc walks back, and esc on the first step cancels.
// startSynth kicks an async verifier-synthesis run for the goal and bumps the
// sequence so a late result from a cancelled run is dropped.
func (m *model) startSynth(agent, goal string) tea.Cmd {
	m.form.synthesizing = true
	m.form.synthStarted = time.Now()
	m.form.synthSeq++
	return synthesizeCmd(m.root, agent, goal, m.form.synthSeq)
}

// enterVerifierStep advances to the verifier step and composes a hybrid
// instantly: the ask question defaults to the goal so the agent judges
// goal-completion each iteration, and the checks field is blank by default so
// the engine designs a deterministic gate in the background once the loop runs
// (AutoGate) — the agent, not a guessed command, is the gate. No agent call
// here; loop creation never blocks. tab designs the gate up front instead.
func (m *model) enterVerifierStep() tea.Cmd {
	m.form.step = stepVerifier
	if !m.form.askEdited {
		m.form.ask = strings.TrimSpace(m.form.goal)
	}
	return nil
}

func (m model) handleFormKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.form.confirmStash {
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "y":
			return m.startFormLoops(true)
		case "n", "esc":
			m.form.confirmStash = false
			m.say("cancelled — commit or stash your changes, then start")
		}
		return m, nil
	}
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.form.synthesizing {
			// Cancel the pending proposal; its late result is dropped by seq.
			m.form.synthesizing = false
			m.form.synthSeq++
			m.say("proposal cancelled")
			return m, nil
		}
		if m.form.step == stepGoal {
			m.form = formState{}
		} else {
			m.form.step--
		}
		return m, nil
	case "enter":
		if m.form.synthesizing {
			m.say("still asking %s — esc cancels", strings.Join(m.form.selectedAgents(), "+"))
			return m, nil
		}
		return m.wizardAdvance()
	}
	if m.form.synthesizing {
		return m, nil // typing is parked while the agent thinks
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
		switch key {
		case "up", "down":
			m.form.verifierField = 1 - m.form.verifierField
			return m, nil
		case "tab":
			agents := m.form.selectedAgents()
			if len(agents) == 0 {
				m.say("pick an agent first — it designs the checks")
				return m, nil
			}
			m.form.verifierField = 1 // the proposal lands in the checks field
			return m, m.startSynth(agents[0], strings.TrimSpace(m.form.goal))
		}
		if m.form.verifierField == 0 {
			if next := editText(m.form.ask, key, msg.Text, 500); next != m.form.ask {
				m.form.ask = next
				m.form.askEdited = true
			}
			break
		}
		if next := editText(m.form.verifier, key, msg.Text, 500); next != m.form.verifier {
			m.form.verifier = next
			m.form.edited = true
			m.form.proposedBy = ""
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
		// The agent is chosen and the goal is set: hand the goal to the agent
		// to design the verifier up front, rather than landing on a blind
		// inferred default.
		return m, m.enterVerifierStep()
	case stepVerifier:
		if len(f.resolvedStages()) == 0 {
			m.say("no verifier, no loop — set a check, an ask question, or both")
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
		// Uncommitted changes turn enter into a stash offer instead of the
		// dead-end refusal; the confirm screen renders it, y stashes and starts.
		if dirty, err := loop.IsGitDirty(m.root); err == nil && dirty {
			m.form.confirmStash = true
			return m, nil
		}
		return m.startFormLoops(false)
	}
	f.step++
	return m, nil
}

// startFormLoops creates the wizard's loop(s), optionally stashing uncommitted
// changes first (the confirmStash offer). loopy never pops the stash — the
// flash points the user at `git stash pop`, and the loop runs from HEAD either
// way, so they can restore whenever.
func (m model) startFormLoops(stashFirst bool) (tea.Model, tea.Cmd) {
	stashed := false
	if stashFirst {
		var err error
		stashed, err = loop.StashTracked(m.root, "loopy: set aside before starting a loop")
		if err != nil {
			m.form.confirmStash = false
			m.say("could not stash: %v", err)
			return m, nil
		}
	}
	ids, err := startLoops(m.root, m.form)
	if err != nil {
		m.form.confirmStash = false
		m.say("%v", err)
		return m, nil
	}
	m.form = formState{}
	m.selectedID = ids[0]
	m.tab = tabOverview
	m.scroll = -1
	switch {
	case len(ids) > 1:
		m.say("racing %d loops — when all park: loopy judge %s", len(ids), strings.Join(ids, " "))
	case stashed:
		m.say("loop %s started — your changes are stashed (git stash pop to restore)", ids[0])
	default:
		m.say("loop %s started", ids[0])
	}
	m.reload()
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
	return loop.IsTerminalStatus(v.Status)
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
