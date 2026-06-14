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

// A on an accepted loop arms the apply confirmation and captures the target,
// even though accepted loops have left the rail (the target is the newest
// accepted loop, not the current selection).
func TestApplyKeyArmsConfirm(t *testing.T) {
	loops := sampleLoops()
	loops[1].Status = loop.StatusAccepted
	loops[1].FinalDiffPath = "/tmp/loops/flaky-importer/final-diff.patch"
	loops[1].NextCommand = "git apply " + loops[1].FinalDiffPath
	// Selection sits on the still-running loop; the apply target is the
	// accepted one regardless.
	m := model{loops: loops, selected: 0}
	res, _ := m.handleKey(press('A', "A"))
	m = res.(model)
	if !m.confirmApply {
		t.Fatal("A should arm the apply confirmation when something is accepted")
	}
	if m.applyID != "flaky-importer" || m.applyPath != loops[1].FinalDiffPath {
		t.Fatalf("apply target not captured: id=%q path=%q", m.applyID, m.applyPath)
	}
	res, _ = m.handleKey(press(tea.KeyEscape, ""))
	m = res.(model)
	if m.confirmApply || m.flash == "" {
		t.Fatal("esc should cancel the apply and say so")
	}
}

// A with nothing accepted yet is a no-op with a pointer, not an armed confirm.
func TestApplyKeyNoTarget(t *testing.T) {
	m := model{loops: sampleLoops(), selected: 0} // running + parked, none accepted
	res, _ := m.handleKey(press('A', "A"))
	m = res.(model)
	if m.confirmApply {
		t.Fatal("A with nothing accepted must not arm a confirmation")
	}
	if m.flash == "" {
		t.Fatal("A with nothing accepted should explain why")
	}
}

// runApply actually applies the durable diff to the working tree — and only
// that: it is git apply, never a commit. The patch applies atomically.
func TestRunApplyAppliesPatchToWorkingTree(t *testing.T) {
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
	target := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(target, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "base")

	// Produce a patch that changes the file, then restore the tree to v1.
	if err := os.WriteFile(target, []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := exec.Command("git", "diff")
	diff.Dir = dir
	patch, err := diff.Output()
	if err != nil {
		t.Fatal(err)
	}
	git("checkout", "--", "file.txt")
	patchPath := filepath.Join(dir, "change.patch")
	if err := os.WriteFile(patchPath, patch, 0o644); err != nil {
		t.Fatal(err)
	}

	m := &model{root: dir, applyID: "demo", applyPath: patchPath}
	m.runApply()

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v2\n" {
		t.Fatalf("file not patched: %q (flash: %q)", got, m.flash)
	}
	if m.flash == "" {
		t.Fatal("a successful apply should confirm in the flash")
	}

	// The commit history is untouched — loopy applies, it never commits.
	head := exec.Command("git", "log", "--oneline")
	head.Dir = dir
	out, _ := head.Output()
	if n := len(string(out)); n == 0 {
		t.Fatal("expected the base commit to remain")
	}
	status := exec.Command("git", "status", "--porcelain")
	status.Dir = dir
	st, _ := status.Output()
	if len(st) == 0 {
		t.Fatal("the patch should be an uncommitted working-tree change")
	}
}

// A failing apply (a patch that does not fit) leaves the tree untouched and
// surfaces the failure rather than swallowing it.
func TestRunApplyFailureKeepsTreeAndReports(t *testing.T) {
	dir := t.TempDir()
	patchPath := filepath.Join(dir, "bad.patch")
	if err := os.WriteFile(patchPath, []byte("not a patch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &model{root: dir, applyID: "demo", applyPath: patchPath}
	m.runApply()
	if !strings.Contains(m.flash, "git apply failed") {
		t.Fatalf("a failed apply should report it, got flash %q", m.flash)
	}
}
