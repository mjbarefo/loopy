package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mjbarefo/loopy/internal/loop"
)

func press(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text}
}

func TestFormTyping(t *testing.T) {
	m := model{form: formState{active: true}}

	for _, c := range "fix" {
		res, _ := m.handleFormKey(press(c, string(c)))
		m = res.(model)
	}
	res, _ := m.handleFormKey(press(tea.KeySpace, " "))
	m = res.(model)
	for _, c := range "it" {
		res, _ := m.handleFormKey(press(c, string(c)))
		m = res.(model)
	}
	if m.form.goal != "fix it" {
		t.Fatalf("goal = %q, want %q", m.form.goal, "fix it")
	}

	res, _ = m.handleFormKey(press(tea.KeyBackspace, ""))
	m = res.(model)
	if m.form.goal != "fix i" {
		t.Fatalf("backspace: goal = %q", m.form.goal)
	}

	// Unicode travels whole.
	res, _ = m.handleFormKey(press('引', "引"))
	m = res.(model)
	if m.form.goal != "fix i引" {
		t.Fatalf("unicode: goal = %q", m.form.goal)
	}

	res, _ = m.handleFormKey(press(tea.KeyEscape, ""))
	m = res.(model)
	if m.form.active {
		t.Fatal("esc should close the form")
	}
}

func TestWelcomeDismissal(t *testing.T) {
	m := model{welcome: true}
	res, cmd := m.handleKey(press('x', "x"))
	m = res.(model)
	if m.welcome || cmd != nil {
		t.Fatal("any key should dismiss the welcome and stay running")
	}

	m = model{welcome: true}
	_, cmd = m.handleKey(press('q', "q"))
	if cmd == nil {
		t.Fatal("q on the welcome should quit")
	}
}

func TestFormGuardNeedsSetup(t *testing.T) {
	m := model{initialized: false}
	res, _ := m.handleKey(press('n', "n"))
	m = res.(model)
	if m.form.active {
		t.Fatal("n must not open the wizard before init")
	}
	if m.flash == "" {
		t.Fatal("the guard should say what to do instead")
	}
}

func TestWizardWalksTheSteps(t *testing.T) {
	m := model{form: formState{
		active: true, goal: "fix it",
		agents: []string{"claude", "codex"}, defaultAgent: "claude",
		picked:   map[int]bool{},
		verifier: "go test ./...",
		iters:    "8", wall: "30m",
	}}
	enter := press(tea.KeyEnter, "")

	// goal → agent.
	res, _ := m.handleFormKey(enter)
	m = res.(model)
	if m.form.step != stepAgent {
		t.Fatalf("goal should advance to the agent step, got %d", m.form.step)
	}
	// agent → verifier lands instantly on the hybrid: no synthesis, no agent
	// call, and the ask question defaults to the goal.
	res, cmd := m.handleFormKey(enter)
	m = res.(model)
	if m.form.step != stepVerifier || m.form.synthesizing || cmd != nil {
		t.Fatalf("advancing past the agent should land on the verifier without an agent call: step=%d synth=%v", m.form.step, m.form.synthesizing)
	}
	if m.form.ask != "fix it" {
		t.Fatalf("the ask question should default to the goal, got %q", m.form.ask)
	}
	// verifier → budget → confirm.
	for want := stepBudget; want <= stepConfirm; want++ {
		res, _ := m.handleFormKey(enter)
		m = res.(model)
		if m.form.step != want {
			t.Fatalf("after enter, step = %d, want %d", m.form.step, want)
		}
	}

	// esc walks back without losing anything.
	res, _ = m.handleFormKey(press(tea.KeyEscape, ""))
	m = res.(model)
	if m.form.step != stepBudget || m.form.goal != "fix it" {
		t.Fatalf("esc should step back keeping state, step=%d goal=%q", m.form.step, m.form.goal)
	}

	// Bad budget input does not advance.
	m.form.iters = "lots"
	res, _ = m.handleFormKey(enter)
	m = res.(model)
	if m.form.step != stepBudget {
		t.Fatal("a non-numeric budget must not advance")
	}
	if m.flash == "" {
		t.Fatal("the wizard should say what is wrong with the budget")
	}

	// The verifier step focuses the ask field first; typing edits the question.
	m.form.step = stepVerifier
	m.form.verifierField = 0
	res, _ = m.handleFormKey(press('x', "x"))
	m = res.(model)
	if !m.form.askEdited {
		t.Fatal("typing on the verifier step should edit the ask question by default")
	}

	// Space on the agent step marks for racing.
	m.form.step = stepAgent
	res, _ = m.handleFormKey(press(tea.KeySpace, " "))
	m = res.(model)
	if !m.form.picked[0] {
		t.Fatal("space should mark the agent under the cursor")
	}

	// An empty goal refuses to advance.
	m.form = formState{active: true, picked: map[int]bool{}}
	res, _ = m.handleFormKey(enter)
	m = res.(model)
	if m.form.step != stepGoal {
		t.Fatal("an empty goal must not advance")
	}
}

// TestSelectSkipsDecided: selection moves over the rail's visible loops
// only — decided history is skipped in both directions, and a wall of
// decided loops at the edge doesn't strand the cursor.
func TestSelectSkipsDecided(t *testing.T) {
	loops := sampleLoops()
	loops = append(loops, loops[1])
	loops[1].Status = loop.StatusAccepted
	loops[1].ID = "decided-loop"
	loops[2].ID = "parked-loop"

	if got := nextVisible(loops, 0, 1); loops[got].ID != "parked-loop" {
		t.Fatalf("down should skip the decided loop, landed on %s", loops[got].ID)
	}
	if got := nextVisible(loops, 2, -1); got != 0 {
		t.Fatalf("up should skip back over it, landed on %s", loops[got].ID)
	}
	// Nothing visible below: the cursor stays put.
	loops[2].Status = loop.StatusRejected
	if got := nextVisible(loops, 0, 1); got != 0 {
		t.Fatalf("with only decided loops below, selection should stay, got %d", got)
	}
}

// TestDeleteKeyConfirms: d arms the confirmation; live loops are refused;
// n cancels without touching anything.
func TestDeleteKeyConfirms(t *testing.T) {
	m := model{loops: sampleLoops(), selected: 1} // parked loop
	res, _ := m.handleKey(press('d', "d"))
	m = res.(model)
	if !m.confirmDelete {
		t.Fatal("d on a parked loop should ask for confirmation")
	}
	res, _ = m.handleKey(press('n', "n"))
	m = res.(model)
	if m.confirmDelete || m.flash == "" {
		t.Fatal("n should cancel and say so")
	}

	m = model{loops: sampleLoops(), selected: 0} // live loop
	res, _ = m.handleKey(press('d', "d"))
	m = res.(model)
	if m.confirmDelete {
		t.Fatal("a live loop must not be deletable from the monitor")
	}
	if m.flash == "" {
		t.Fatal("the refusal should say what to do instead")
	}
}

// TestWizardSynthesis: tab on the verifier step asks the agent (async);
// typing and enter are parked while it thinks; the proposal lands in the
// editable field marked edited (so it is never stored as project default);
// esc cancels and the stale result is dropped by sequence.
func TestWizardSynthesis(t *testing.T) {
	m := model{form: formState{
		active: true, step: stepVerifier, goal: "make x",
		agents: []string{"claude"}, picked: map[int]bool{},
	}}
	res, cmd := m.handleFormKey(press(tea.KeyTab, ""))
	m = res.(model)
	if !m.form.synthesizing || cmd == nil {
		t.Fatal("tab should start synthesis and return its command")
	}
	seq := m.form.synthSeq

	// Enter and typing are parked while the agent thinks.
	res, _ = m.handleFormKey(press(tea.KeyEnter, ""))
	m = res.(model)
	if m.form.step != stepVerifier {
		t.Fatal("enter must not advance during synthesis")
	}
	res, _ = m.handleFormKey(press('x', "x"))
	m = res.(model)
	if m.form.verifier != "" {
		t.Fatal("typing must be parked during synthesis")
	}

	// The proposal lands: prefilled, editable, marked edited.
	res2, _ := m.Update(synthDoneMsg{seq: seq, res: loop.SynthesisResult{Agent: "claude", Cmd: "test -f x.txt"}})
	m = res2.(model)
	if m.form.synthesizing || m.form.verifier != "test -f x.txt" || !m.form.edited {
		t.Fatalf("proposal did not land: %+v", m.form)
	}
	if m.form.proposedBy != "claude" || m.flash == "" {
		t.Fatal("the proposal should be attributed and announced")
	}

	// Esc cancels a pending ask; the late result is dropped by sequence.
	res, _ = m.handleFormKey(press(tea.KeyTab, ""))
	m = res.(model)
	staleSeq := m.form.synthSeq
	res, _ = m.handleFormKey(press(tea.KeyEscape, ""))
	m = res.(model)
	if m.form.synthesizing || m.form.step != stepVerifier {
		t.Fatal("esc during synthesis should cancel in place")
	}
	res2, _ = m.Update(synthDoneMsg{seq: staleSeq, res: loop.SynthesisResult{Agent: "claude", Cmd: "rm -rf /"}})
	m = res2.(model)
	if m.form.verifier == "rm -rf /" {
		t.Fatal("a cancelled proposal must be dropped")
	}
}

// TestMouseWheel: the wheel scrolls whatever sits under it — the detail body
// by lines, the rail by loops.
func TestMouseWheel(t *testing.T) {
	// 120x14 is dense: content starts at row 2, the rail at column 0, and the
	// overview body (8 lines) outgrows its 4 rows — so it can actually scroll.
	m := model{loops: sampleLoops(), selected: 0, width: 120, height: 14, scroll: -1}
	railW, _ := m.frameState().railArea()
	detailX := railW + 2

	res, _ := m.Update(tea.MouseWheelMsg{X: detailX + 5, Y: 9, Button: tea.MouseWheelUp})
	m = res.(model)
	if m.scroll < 0 {
		t.Fatal("wheel up over the detail body should leave the tail")
	}
	res, _ = m.Update(tea.MouseWheelMsg{X: detailX + 5, Y: 9, Button: tea.MouseWheelDown})
	m = res.(model)
	if m.scroll != -1 {
		t.Fatalf("wheel down to the bottom should re-engage tail follow, scroll=%d", m.scroll)
	}

	res, _ = m.Update(tea.MouseWheelMsg{X: 3, Y: 2, Button: tea.MouseWheelDown})
	m = res.(model)
	if m.selected != 1 {
		t.Fatalf("wheel over the rail should move the selection, selected=%d", m.selected)
	}
}

// TestMouseClick: rail rows select, nav names switch views (landing on the
// tab's home scroll), confirms ignore clicks entirely.
func TestMouseClick(t *testing.T) {
	m := model{loops: sampleLoops(), selected: 0, width: 120, height: 36, scroll: -1}
	res, _ := m.Update(tea.MouseClickMsg{X: 4, Y: 5, Button: tea.MouseLeft})
	m = res.(model)
	if m.selected != 1 {
		t.Fatalf("clicking the parked rail row should select it, selected=%d", m.selected)
	}

	m = model{loops: sampleLoops(), selected: 0, width: 120, height: 36, scroll: -1}
	railW, _ := m.frameState().railArea()
	res, _ = m.Update(tea.MouseClickMsg{X: 2 + railW + 2 + 20, Y: 10, Button: tea.MouseLeft})
	m = res.(model)
	if m.tab != tabDiff {
		t.Fatalf("clicking the diff name should switch the view, tab=%d", m.tab)
	}
	if m.scroll != 0 {
		t.Fatalf("the diff tab opens answer-first at the top, scroll=%d", m.scroll)
	}

	m = model{loops: sampleLoops(), selected: 0, width: 120, height: 36, confirmAccept: true}
	res, _ = m.Update(tea.MouseClickMsg{X: 4, Y: 5, Button: tea.MouseLeft})
	m = res.(model)
	if m.selected != 0 || !m.confirmAccept {
		t.Fatal("a pending confirm must ignore clicks — decisions stay explicit")
	}

	m = model{welcome: true, width: 120, height: 36}
	res, _ = m.Update(tea.MouseClickMsg{X: 4, Y: 5, Button: tea.MouseLeft})
	m = res.(model)
	if m.welcome {
		t.Fatal("a click should dismiss the welcome like any key")
	}
}

// TestCopyNextCommand: c sends the next command via OSC 52 — the selected
// loop's, or on a quiet rail the newest accepted loop's apply command. A
// running loop's "next" is this monitor; nothing to copy.
func TestCopyNextCommand(t *testing.T) {
	m := model{loops: sampleLoops(), selected: 1} // parked
	res, cmd := m.handleKey(press('c', "c"))
	m = res.(model)
	if cmd == nil {
		t.Fatal("c should produce the clipboard command")
	}
	if !strings.Contains(m.flash, "loopy review flaky-importer") {
		t.Fatalf("the flash should name what was copied, got %q", m.flash)
	}

	m = model{loops: sampleLoops(), selected: 0} // running
	res, cmd = m.handleKey(press('c', "c"))
	m = res.(model)
	if cmd != nil || m.flash == "" {
		t.Fatal("a running loop has nothing worth copying; say so")
	}

	loops := sampleLoops()
	for i := range loops {
		loops[i].Status = loop.StatusAccepted
	}
	loops[1].NextCommand = "git apply .loopy/loops/flaky-importer/final-diff.patch"
	loops[1].EndedAt = "2026-06-13T10:00:00Z"
	loops[0].NextCommand = ""
	m = model{loops: loops, selected: -1} // quiet rail
	res, cmd = m.handleKey(press('c', "c"))
	m = res.(model)
	if cmd == nil || !strings.Contains(m.flash, "git apply") {
		t.Fatalf("the quiet rail should copy the shown apply command, flash=%q", m.flash)
	}
}

// TestWizardComposesHybridInstantly: advancing past the agent step lands the
// verifier instantly — command gates plus an ask question defaulting to the
// goal — with no agent call. tab is the optional polish that designs gates,
// and ↑↓ switches which field the keystrokes edit.
func TestWizardComposesHybridInstantly(t *testing.T) {
	m := model{form: formState{
		active: true, step: stepAgent, goal: "write an AGENTS.md",
		agents: []string{"codex"}, picked: map[int]bool{},
		verifier: "make check",
	}}
	enter := press(tea.KeyEnter, "")

	res, cmd := m.handleFormKey(enter)
	m = res.(model)
	if m.form.step != stepVerifier || m.form.synthesizing || cmd != nil {
		t.Fatalf("agent→verifier should be instant, no agent call: step=%d synth=%v cmd=%v", m.form.step, m.form.synthesizing, cmd != nil)
	}
	if m.form.ask != "write an AGENTS.md" {
		t.Fatalf("the ask question should default to the goal, got %q", m.form.ask)
	}
	// The resolved verifier is a hybrid: the command gate plus the ask stage.
	stages := m.form.resolvedStages()
	if len(stages) != 2 || stages[0].Kind == loop.KindAsk || stages[1].Kind != loop.KindAsk {
		t.Fatalf("expected a command gate then an ask stage, got %+v", stages)
	}

	// The ask field is focused first (the hero); typing edits the question.
	res, _ = m.handleFormKey(press('?', "?"))
	m = res.(model)
	if m.form.ask != "write an AGENTS.md?" || m.form.edited {
		t.Fatalf("typing should edit the ask question by default, not the gates: ask=%q edited=%v", m.form.ask, m.form.edited)
	}

	// ↓ switches to the checks field; typing there edits the command, not the ask.
	res, _ = m.handleFormKey(press(tea.KeyDown, ""))
	m = res.(model)
	if m.form.verifierField != 1 {
		t.Fatalf("down should switch to the checks field, got field %d", m.form.verifierField)
	}
	res, _ = m.handleFormKey(press('x', "x"))
	m = res.(model)
	if !m.form.edited || m.form.ask != "write an AGENTS.md?" {
		t.Fatalf("typing on the checks field should edit the command only: verifier=%q edited=%v", m.form.verifier, m.form.edited)
	}

	// tab is still the optional polish: it asks the agent to design the gates.
	res, cmd = m.handleFormKey(press(tea.KeyTab, ""))
	m = res.(model)
	if !m.form.synthesizing || cmd == nil {
		t.Fatal("tab on the verifier step should still ask the agent to design the checks")
	}
	res2, _ := m.Update(synthDoneMsg{seq: m.form.synthSeq, res: loop.SynthesisResult{Agent: "codex", Cmd: "test -f AGENTS.md && make check"}})
	m = res2.(model)
	if m.form.verifier != "test -f AGENTS.md && make check" || !m.form.edited {
		t.Fatalf("a tab proposal should land in the checks field: %+v", m.form)
	}
}

// TestWizardAskOnlyVerifier: clearing the checks leaves an ask-only verifier,
// which is allowed — the loop still has a verifier.
func TestWizardAskOnlyVerifier(t *testing.T) {
	f := formState{verifier: "", ask: "is the README friendlier and still accurate?"}
	stages := f.resolvedStages()
	if len(stages) != 1 || stages[0].Kind != loop.KindAsk {
		t.Fatalf("an empty checks field with an ask question is a valid ask-only verifier, got %+v", stages)
	}
	// Both empty is no verifier — wizardAdvance must refuse it.
	m := model{form: formState{active: true, step: stepVerifier}}
	res, _ := m.wizardAdvance()
	m = res.(model)
	if m.form.step != stepVerifier || m.flash == "" {
		t.Fatal("an empty checks and empty ask must not advance, and should say why")
	}
}

// TestReselect: the sticky ID wins (loopy watch <id> pins decided loops);
// otherwise the first loop that needs eyes; -1 when everything is decided —
// the rail goes quiet rather than re-pinning the loop just decided.
func TestReselect(t *testing.T) {
	loops := sampleLoops()
	if got := reselect(loops, ""); got != 0 {
		t.Fatalf("no sticky id: want the first visible loop, got %d", got)
	}
	if got := reselect(loops, "flaky-importer"); got != 1 {
		t.Fatalf("sticky id should win, got %d", got)
	}

	for i := range loops {
		loops[i].Status = loop.StatusAccepted
	}
	if got := reselect(loops, ""); got != -1 {
		t.Fatalf("all decided and nothing pinned: want -1, got %d", got)
	}
	if got := reselect(loops, "flaky-importer"); got != 1 {
		t.Fatalf("a pinned decided loop must stay selectable, got %d", got)
	}
}

// TestAcceptKeyIsContextual: a means abort while the loop moves and accept
// once it parks green; a parked red loop points at the CLI override path.
func TestAcceptKeyIsContextual(t *testing.T) {
	loops := sampleLoops()
	loops[1].Status = loop.StatusGreen

	m := model{loops: loops, selected: 0} // running loop
	res, _ := m.handleKey(press('a', "a"))
	m = res.(model)
	if !m.confirmAbort || m.confirmAccept {
		t.Fatal("a on a running loop should arm abort, not accept")
	}

	m = model{loops: loops, selected: 1} // green loop
	res, _ = m.handleKey(press('a', "a"))
	m = res.(model)
	if !m.confirmAccept || m.confirmAbort {
		t.Fatal("a on a green loop should arm accept, not abort")
	}
	res, _ = m.handleKey(press('n', "n"))
	m = res.(model)
	if m.confirmAccept || m.flash == "" {
		t.Fatal("n should cancel the accept and say so")
	}

	m = model{loops: sampleLoops(), selected: 1} // parked red loop
	res, _ = m.handleKey(press('a', "a"))
	m = res.(model)
	if m.confirmAccept || m.confirmAbort {
		t.Fatal("a parked red loop is neither abortable nor acceptable here")
	}
	if m.flash == "" {
		t.Fatal("the refusal should point at loopy accept --override")
	}
}

// TestRejectKeyIsContextual: r rejects a parked loop (green or red) and
// stays resume everywhere else.
func TestRejectKeyIsContextual(t *testing.T) {
	for _, status := range []string{loop.StatusGreen, loop.StatusParked} {
		loops := sampleLoops()
		loops[1].Status = status
		m := model{loops: loops, selected: 1}
		res, _ := m.handleKey(press('r', "r"))
		m = res.(model)
		if !m.confirmReject {
			t.Fatalf("r on a %s loop should arm the reject confirmation", status)
		}
		res, _ = m.handleKey(press(tea.KeyEscape, ""))
		m = res.(model)
		if m.confirmReject || m.flash == "" {
			t.Fatal("esc should cancel the reject and say so")
		}
	}

	m := model{loops: sampleLoops(), selected: 0} // live loop: r is resume
	res, _ := m.handleKey(press('r', "r"))
	m = res.(model)
	if m.confirmReject {
		t.Fatal("r on a running loop must stay resume, not reject")
	}
}
