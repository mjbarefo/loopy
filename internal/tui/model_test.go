package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
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
	m := model{initialized: false, agentsRegistered: false}
	res, _ := m.handleKey(press('n', "n"))
	m = res.(model)
	if m.form.active {
		t.Fatal("n must not open the form before init + an agent")
	}
	if m.flash == "" {
		t.Fatal("the guard should say what to do instead")
	}
}
