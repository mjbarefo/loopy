package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mjbarefo/loopy/internal/loop"
)

// dirtyRepo makes a temp git repo with one uncommitted change to a tracked file.
func dirtyRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	git("config", "user.email", "t@example.com")
	git("config", "user.name", "tester")
	f := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(f, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "base")
	if err := os.WriteFile(f, []byte("v2\n"), 0o644); err != nil { // uncommitted
		t.Fatal(err)
	}
	return dir
}

// On the confirm step, uncommitted changes turn enter into a stash offer
// rather than the dead-end refusal: the screen advertises it, and cancelling
// leaves the working tree untouched.
func TestWizardConfirmStashOffer(t *testing.T) {
	root := dirtyRepo(t)
	m := model{root: root, form: formState{
		active: true, step: stepConfirm, goal: "g",
		agents: []string{"claude"}, picked: map[int]bool{},
		ask: "is it done?", iters: "5", wall: "30m",
	}}

	res, _ := m.handleFormKey(press(tea.KeyEnter, ""))
	m = res.(model)
	if !m.form.confirmStash {
		t.Fatal("enter on a dirty confirm step should offer to stash, not start blindly or fail")
	}
	s := wideState()
	s.form = m.form
	if !strings.Contains(renderFrame(s), "stash them and start") {
		t.Error("the confirm screen should render the stash offer")
	}

	// n cancels: the offer clears, the wizard says so, and the tree is untouched.
	res, _ = m.handleFormKey(press('n', "n"))
	m = res.(model)
	if m.form.confirmStash || m.flash == "" {
		t.Fatalf("n should cancel the stash offer and say so (confirmStash=%v flash=%q)", m.form.confirmStash, m.flash)
	}
	if dirty, _ := loop.IsGitDirty(root); !dirty {
		t.Error("cancelling the offer must not stash — the working tree should stay dirty")
	}
}

// A clean tree never sees the offer: the start path runs as before. (The dirty
// check returns false, so confirmStash is never armed.)
func TestWizardConfirmCleanTreeNoOffer(t *testing.T) {
	root := dirtyRepo(t)
	// Commit the change so the tree is clean.
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "wip"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if dirty, _ := loop.IsGitDirty(root); dirty {
		t.Fatal("repo should be clean after committing")
	}
}
