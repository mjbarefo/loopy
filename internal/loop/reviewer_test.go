package loop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// reviewerProject is a one-iteration-green project (the agent creates
// done.txt, the verifier checks for it) with a second "critic" agent
// registered for reviewing.
func reviewerProject(t *testing.T, criticCmd string) string {
	t.Helper()
	root := newLoopProject(t, `echo done > done.txt`)
	// agents.json lives under the gitignored .loopy/ — no commit needed.
	if err := AddAgent(root, "critic", criticCmd, false); err != nil {
		t.Fatal(err)
	}
	return root
}

func reviewerLoopOpts(reviewer string) CreateOptions {
	return CreateOptions{
		Goal:     "create done.txt",
		Reviewer: reviewer,
		Verifier: []Stage{{Name: "done", Cmd: `test -f done.txt || { echo "need done.txt"; exit 1; }`}},
		Budget:   Budget{MaxIterations: 3},
	}
}

func TestReviewerCritiqueIsEvidence(t *testing.T) {
	root := reviewerProject(t, `cat {prompt_file} >/dev/null; echo "the diff matches the goal"; echo "verdict: looks right"`)
	l := mustCreate(t, root, reviewerLoopOpts("critic"))

	reviewerExit := -1
	final, err := RunEngine(root, l.ID, Events{
		ReviewerDone: func(exitCode int, _ time.Duration) { reviewerExit = exitCode },
	})
	if err != nil {
		t.Fatal(err)
	}
	if reviewerExit != 0 {
		t.Fatalf("ReviewerDone exit = %d, want 0", reviewerExit)
	}
	if final.Status != StatusGreen {
		t.Fatalf("status = %s, want green", final.Status)
	}
	if final.ReviewerExit == nil || *final.ReviewerExit != 0 {
		t.Fatalf("reviewer exit = %v, want 0", final.ReviewerExit)
	}

	critique, err := os.ReadFile(filepath.Join(LoopDir(root, l.ID), CritiqueFile))
	if err != nil {
		t.Fatalf("critique missing: %v", err)
	}
	if !strings.Contains(string(critique), "verdict: looks right") {
		t.Fatalf("critique = %q, want the reviewer's verdict", critique)
	}

	prompt, err := os.ReadFile(filepath.Join(LoopDir(root, l.ID), ReviewPromptFile))
	if err != nil {
		t.Fatalf("review prompt missing: %v", err)
	}
	for _, want := range []string{"create done.txt", "reviewer, not the author", DiffFile} {
		if !strings.Contains(string(prompt), want) {
			t.Errorf("review prompt missing %q", want)
		}
	}

	view, err := BuildLoopView(root, final)
	if err != nil {
		t.Fatal(err)
	}
	if view.CritiquePath == "" || view.Reviewer != "critic" {
		t.Fatalf("view should surface the critique: reviewer=%q path=%q", view.Reviewer, view.CritiquePath)
	}
}

func TestReviewerCannotShip(t *testing.T) {
	// A misbehaving reviewer edits the verified work and plants a new file;
	// the engine restores the exact verified state before parking.
	root := reviewerProject(t, `echo sabotage > done.txt; echo planted > extra.txt; echo "verdict: looks right"`)
	l := mustCreate(t, root, reviewerLoopOpts("critic"))

	var notes []string
	final, err := RunEngine(root, l.ID, Events{Note: func(s string) { notes = append(notes, s) }})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusGreen {
		t.Fatalf("status = %s, want green", final.Status)
	}

	data, err := os.ReadFile(filepath.Join(final.Worktree, "done.txt"))
	if err != nil || strings.TrimSpace(string(data)) != "done" {
		t.Fatalf("done.txt = %q, %v — the verified state was not restored", data, err)
	}
	if _, err := os.Stat(filepath.Join(final.Worktree, "extra.txt")); err == nil {
		t.Fatal("the reviewer's planted file survived the restore")
	}

	iterations, err := LoadIterations(root, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	last := iterations[len(iterations)-1]
	diff, _, err := Snapshot(final.Worktree, final.BaseCommit)
	if err != nil {
		t.Fatal(err)
	}
	if hashBytes(diff) != last.DiffHash {
		t.Fatal("the worktree diff no longer matches the verified iteration")
	}

	restored := false
	for _, n := range notes {
		if strings.Contains(n, "restored") {
			restored = true
		}
	}
	if !restored {
		t.Fatalf("the restore should be reported, notes: %v", notes)
	}
}

func TestReviewerFailureNeverBlocksGreen(t *testing.T) {
	root := reviewerProject(t, `echo "reviewer crashed"; exit 3`)
	l := mustCreate(t, root, reviewerLoopOpts("critic"))

	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusGreen {
		t.Fatalf("status = %s, want green — the critique is never a gate", final.Status)
	}
	if final.ReviewerExit == nil || *final.ReviewerExit != 3 {
		t.Fatalf("reviewer exit = %v, want 3", final.ReviewerExit)
	}
}

func TestReviewerMustBeADifferentAgent(t *testing.T) {
	root := reviewerProject(t, `true`)
	opts := reviewerLoopOpts("scripted") // same as the author
	if _, err := CreateLoop(root, opts); err == nil || !strings.Contains(err.Error(), "different agent") {
		t.Fatalf("self-review must be refused at creation, got %v", err)
	}
}

func TestReviewerSkipsBaselineGreen(t *testing.T) {
	// A verifier that already passes parks green with an empty diff —
	// nothing to review.
	root := reviewerProject(t, `echo "verdict: looks right"`)
	l := mustCreate(t, root, CreateOptions{
		Goal:     "already done",
		Reviewer: "critic",
		Verifier: []Stage{{Name: "true", Cmd: "true"}},
		Budget:   Budget{MaxIterations: 2},
	})

	var notes []string
	final, err := RunEngine(root, l.ID, Events{Note: func(s string) { notes = append(notes, s) }})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusGreen || final.IterationsUsed != 0 {
		t.Fatalf("want baseline green, got %s after %d iterations", final.Status, final.IterationsUsed)
	}
	if _, err := os.Stat(filepath.Join(LoopDir(root, l.ID), CritiqueFile)); err == nil {
		t.Fatal("baseline green has an empty diff; no critique should exist")
	}
	skipped := false
	for _, n := range notes {
		if strings.Contains(n, "nothing to review") {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("the skip should be reported, notes: %v", notes)
	}
}
