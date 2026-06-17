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
	if m.confirm != confirmApply {
		t.Fatal("A should arm the apply confirmation when something is accepted")
	}
	if m.applyID != "flaky-importer" || m.applyPath != loops[1].FinalDiffPath {
		t.Fatalf("apply target not captured: id=%q path=%q", m.applyID, m.applyPath)
	}
	res, _ = m.handleKey(press(tea.KeyEscape, ""))
	m = res.(model)
	if m.confirm != confirmNone || m.flash == "" {
		t.Fatal("esc should cancel the apply and say so")
	}
}

// A with nothing accepted yet is a no-op with a pointer, not an armed confirm.
func TestApplyKeyNoTarget(t *testing.T) {
	m := model{loops: sampleLoops(), selected: 0} // running + parked, none accepted
	res, _ := m.handleKey(press('A', "A"))
	m = res.(model)
	if m.confirm == confirmApply {
		t.Fatal("A with nothing accepted must not arm a confirmation")
	}
	if m.flash == "" {
		t.Fatal("A with nothing accepted should explain why")
	}
}

// gitRepoWithPatch builds a temp repo whose change.patch turns file.txt from
// v1 to v2, with the tree restored to v1 — applying the patch reintroduces v2.
func gitRepoWithPatch(t *testing.T) (dir, patchPath, target string) {
	t.Helper()
	dir = t.TempDir()
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
	target = filepath.Join(dir, "file.txt")
	if err := os.WriteFile(target, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "base")
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
	patchPath = filepath.Join(dir, "change.patch")
	if err := os.WriteFile(patchPath, patch, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, patchPath, target
}

// applyPatch actually applies the durable diff to the working tree — and only
// that: it is git apply, never a commit. The patch applies atomically.
func TestApplyPatchToWorkingTree(t *testing.T) {
	dir, patchPath, target := gitRepoWithPatch(t)
	if out, err := applyPatch(dir, patchPath); err != nil {
		t.Fatalf("applyPatch: %v\n%s", err, out)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v2\n" {
		t.Fatalf("file not patched: %q", got)
	}
	// History untouched, change uncommitted: loopy applies, never commits.
	log := exec.Command("git", "log", "--oneline")
	log.Dir = dir
	if out, _ := log.Output(); strings.Count(string(out), "\n") != 1 {
		t.Fatalf("expected exactly the base commit, got:\n%s", out)
	}
	status := exec.Command("git", "status", "--porcelain")
	status.Dir = dir
	if st, _ := status.Output(); len(st) == 0 {
		t.Fatal("the patch should be an uncommitted working-tree change")
	}
}

func TestApplyPatchFailureReturnsError(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.patch")
	if err := os.WriteFile(bad, []byte("not a patch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := applyPatch(dir, bad); err == nil {
		t.Fatal("a malformed patch must fail rather than silently no-op")
	}
}

// The whole point of this change: a successful apply removes the loop. A failed
// apply leaves it. The delete seam stands in for the audited CLI.
func TestRunApplyDeletesLoopOnSuccess(t *testing.T) {
	dir, patchPath, target := gitRepoWithPatch(t)
	deleted := ""
	m := &model{
		root:       dir,
		applyID:    "shipped-loop",
		applyPath:  patchPath,
		deleteLoop: func(root, id string) (string, error) { deleted = id; return "", nil },
	}
	m.runApply()

	if got, _ := os.ReadFile(target); string(got) != "v2\n" {
		t.Fatalf("apply did not run before delete: %q", got)
	}
	if deleted != "shipped-loop" {
		t.Fatalf("a clean apply must remove the loop, deleted=%q", deleted)
	}
	if !strings.Contains(m.flash, "removed the loop") {
		t.Fatalf("flash should report the removal: %q", m.flash)
	}
}

func TestRunApplyKeepsLoopWhenApplyFails(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.patch")
	if err := os.WriteFile(bad, []byte("not a patch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deleted := ""
	m := &model{
		root:       dir,
		applyID:    "kept-loop",
		applyPath:  bad,
		deleteLoop: func(root, id string) (string, error) { deleted = id; return "", nil },
	}
	m.runApply()

	if deleted != "" {
		t.Fatalf("a failed apply must NOT remove the loop, deleted=%q", deleted)
	}
	if !strings.Contains(m.flash, "git apply failed") {
		t.Fatalf("a failed apply should report it, got flash %q", m.flash)
	}
}

// A delete that fails after a clean apply must not erase the fact that the diff
// landed — the user still needs to commit it.
func TestRunApplyReportsAppliedWhenDeleteFails(t *testing.T) {
	dir, patchPath, _ := gitRepoWithPatch(t)
	m := &model{
		root:      dir,
		applyID:   "half",
		applyPath: patchPath,
		deleteLoop: func(root, id string) (string, error) {
			return "live engine holds it", os.ErrPermission
		},
	}
	m.runApply()
	if !strings.Contains(m.flash, "applied half") {
		t.Fatalf("a clean apply must be reported even when delete fails: %q", m.flash)
	}
}
