package loop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// greenLoop runs a one-iteration loop to green: the agent creates done.txt,
// the verifier wants it.
func greenLoop(t *testing.T, root string) Loop {
	t.Helper()
	l := mustCreate(t, root, CreateOptions{
		Goal:     "create done.txt",
		Verifier: []Stage{{Name: "done", Cmd: `test -f done.txt || { echo "need done.txt"; exit 1; }`}},
	})
	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusGreen {
		t.Fatalf("setup loop should park green, got %s (%s)", final.Status, final.ParkedReason)
	}
	return final
}

// parkedLoop runs a do-nothing agent against an unsatisfiable verifier: the
// no-change stuck rule parks it after one iteration.
func parkedLoop(t *testing.T, root string) Loop {
	t.Helper()
	l := mustCreate(t, root, CreateOptions{
		Goal:     "never succeeds",
		Verifier: []Stage{{Name: "no", Cmd: `echo "always failing"; false`}},
	})
	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusParked {
		t.Fatalf("setup loop should park, got %s", final.Status)
	}
	return final
}

func TestAcceptGreenLoopWritesDurableRecord(t *testing.T) {
	root := newLoopProject(t, "echo done > done.txt")
	l := greenLoop(t, root)

	r, err := Accept(root, l.ID, false, "")
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if r.Decision != DecisionAccepted || !r.GreenAtDecision || r.Override {
		t.Fatalf("review record wrong: %+v", r)
	}

	reloaded, err := LoadLoop(root, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != StatusAccepted {
		t.Fatalf("loop status = %s, want accepted", reloaded.Status)
	}
	diff, err := os.ReadFile(filepath.Join(LoopDir(root, l.ID), FinalDiffFile))
	if err != nil {
		t.Fatalf("final-diff.patch must be durable: %v", err)
	}
	if !strings.Contains(string(diff), "done.txt") {
		t.Fatalf("final diff lost the change:\n%s", diff)
	}
	if _, ok, _ := LoadReview(root, l.ID); !ok {
		t.Fatal("review.json missing")
	}
	if _, err := os.Stat(LogbookPath(root)); err != nil {
		t.Fatalf("accept must append the logbook: %v", err)
	}
	// The worktree stays on accept (only reject frees it).
	if _, err := os.Stat(reloaded.Worktree); err != nil {
		t.Fatalf("accept must keep the worktree: %v", err)
	}
}

func TestAcceptParkedNeedsOverrideWithReason(t *testing.T) {
	root := newLoopProject(t, "true")
	l := parkedLoop(t, root)

	if _, err := Accept(root, l.ID, false, ""); err == nil || !strings.Contains(err.Error(), "--override") {
		t.Fatalf("accepting a parked loop without override must refuse, got %v", err)
	}
	if _, err := Accept(root, l.ID, true, "  "); err == nil || !strings.Contains(err.Error(), "--reason") {
		t.Fatalf("override without a reason must refuse, got %v", err)
	}

	reason := "known-flaky verifier; the diff is fine   (verbatim)"
	r, err := Accept(root, l.ID, true, reason)
	if err != nil {
		t.Fatalf("override accept: %v", err)
	}
	if !r.Override || r.GreenAtDecision || r.Reason != reason {
		t.Fatalf("override must be recorded verbatim: %+v", r)
	}
	stored, ok, err := LoadReview(root, l.ID)
	if err != nil || !ok || stored.Reason != reason {
		t.Fatalf("review.json must carry the reason verbatim: %+v (%v)", stored, err)
	}
}

func TestDecisionsRefuseMovingOrDecidedLoops(t *testing.T) {
	root := newLoopProject(t, "echo done > done.txt")
	l := greenLoop(t, root)

	if _, err := Accept(root, l.ID, false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Accept(root, l.ID, false, ""); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("second decision must refuse, got %v", err)
	}
	if _, err := Reject(root, l.ID, "changed my mind"); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("reject after accept must refuse, got %v", err)
	}

	// A loop that never finished cannot be decided.
	running := mustCreate(t, root, CreateOptions{
		Goal:     "still going",
		Verifier: []Stage{{Name: "no", Cmd: "false"}},
	})
	if _, err := Accept(root, running.ID, false, ""); err == nil || !strings.Contains(err.Error(), "running") {
		t.Fatalf("accepting a running loop must refuse, got %v", err)
	}
}

func TestRejectFreesWorktreeAndKeepsEvidence(t *testing.T) {
	root := newLoopProject(t, "echo done > done.txt")
	l := greenLoop(t, root)

	r, err := Reject(root, l.ID, "not the approach I wanted")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if r.Decision != DecisionRejected || r.Reason == "" {
		t.Fatalf("review record wrong: %+v", r)
	}
	reloaded, err := LoadLoop(root, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != StatusRejected {
		t.Fatalf("loop status = %s, want rejected", reloaded.Status)
	}
	if _, err := os.Stat(l.Worktree); !os.IsNotExist(err) {
		t.Fatalf("reject must free the worktree, stat err = %v", err)
	}
	// Evidence survives: every recorded iteration is still on disk.
	iterations, err := LoadIterations(root, l.ID)
	if err != nil || len(iterations) == 0 {
		t.Fatalf("evidence must be preserved: %v, %d iterations", err, len(iterations))
	}
	if _, err := os.Stat(filepath.Join(IterationDir(root, l.ID, iterations[len(iterations)-1].Index), DiffFile)); err != nil {
		t.Fatalf("iteration diffs must survive reject: %v", err)
	}
}
