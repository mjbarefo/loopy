package tui

import (
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

	// goal → agent → verifier → budget → confirm.
	for want := stepAgent; want <= stepConfirm; want++ {
		res, _ := m.handleFormKey(enter)
		m = res.(model)
		if m.form.step != want {
			t.Fatalf("after enter, step = %d, want %d", m.form.step, want)
		}
	}

	// esc walks back without losing anything.
	res, _ := m.handleFormKey(press(tea.KeyEscape, ""))
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

	// Editing the verifier marks it as such.
	m.form.step = stepVerifier
	res, _ = m.handleFormKey(press('x', "x"))
	m = res.(model)
	if !m.form.edited {
		t.Fatal("typing on the verifier step must set edited")
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
