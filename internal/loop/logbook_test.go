package loop

import (
	"os"
	"strings"
	"testing"
)

// fabricatedDecision writes a decided loop's state directly (no git needed):
// loop.json + review.json, the way Accept/Reject leave them.
func fabricatedDecision(t *testing.T, root, id, decision, reason string, green, override bool, decidedAt string) (Loop, Review) {
	t.Helper()
	l := Loop{
		ID: id, Goal: "goal for " + id, Agent: "scripted",
		Verifier: []Stage{{Name: "test", Cmd: "true"}},
		Budget:   DefaultBudget, Status: decision,
		IterationsUsed: 2, WallClockUsed: Duration(90 * 1e9),
		CreatedAt: decidedAt, EndedAt: decidedAt,
	}
	r := Review{
		LoopID: id, Decision: decision, GreenAtDecision: green,
		Override: override, Reason: reason, DecidedAt: decidedAt,
		Goal: l.Goal, Agent: l.Agent, Iterations: 2, WallClock: "1m30s",
	}
	if err := SaveLoop(root, l); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(ReviewPath(root, id), r); err != nil {
		t.Fatal(err)
	}
	return l, r
}

func TestLogbookPathIsInsideLoopyDir(t *testing.T) {
	if got := LogbookPath("/repo"); !strings.HasSuffix(got, "logbook.md") || !strings.Contains(got, LoopyDir) {
		t.Fatalf("LogbookPath = %q, want .loopy/logbook.md under the root", got)
	}
}

func TestLogbookAppendIsHumanReadableAndCumulative(t *testing.T) {
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}

	first, firstReview := fabricatedDecision(t, root, "fix-alpha", DecisionAccepted, "", true, false, "2026-06-11T10:00:00Z")
	if err := appendLogbook(root, first, firstReview); err != nil {
		t.Fatalf("appendLogbook: %v", err)
	}
	second, secondReview := fabricatedDecision(t, root, "fix-beta", DecisionRejected, "diff touched generated code", false, false, "2026-06-11T11:00:00Z")
	if err := appendLogbook(root, second, secondReview); err != nil {
		t.Fatalf("appendLogbook (second): %v", err)
	}

	data, err := os.ReadFile(LogbookPath(root))
	if err != nil {
		t.Fatalf("logbook.md missing after append: %v", err)
	}
	text := string(data)

	// Both entries, in append order, each carrying the essentials a human
	// needs months later: id, decision, goal, and the why.
	firstIdx := strings.Index(text, "fix-alpha")
	secondIdx := strings.Index(text, "fix-beta")
	if firstIdx == -1 || secondIdx == -1 || secondIdx < firstIdx {
		t.Fatalf("logbook entries missing or out of order:\n%s", text)
	}
	for _, want := range []string{DecisionAccepted, DecisionRejected, "goal for fix-alpha", "diff touched generated code", "2026-06-11"} {
		if !strings.Contains(text, want) {
			t.Errorf("logbook.md missing %q:\n%s", want, text)
		}
	}
}

func TestLogbookRecordsOverrideVerbatim(t *testing.T) {
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	reason := "shipping anyway: the failing stage is a known-flaky e2e   <verbatim, spaces kept>"
	l, r := fabricatedDecision(t, root, "fix-gamma", DecisionAccepted, reason, false, true, "2026-06-11T12:00:00Z")
	if err := appendLogbook(root, l, r); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(LogbookPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), reason) {
		t.Fatalf("override reason must appear verbatim in the logbook:\n%s", data)
	}
	if !strings.Contains(strings.ToLower(string(data)), "override") {
		t.Fatalf("an override must be labeled as one:\n%s", data)
	}
}

func TestLogbookEntriesAggregatesReviews(t *testing.T) {
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	// Decision times deliberately out of directory order.
	fabricatedDecision(t, root, "zeta-loop", DecisionAccepted, "", true, false, "2026-06-11T09:00:00Z")
	fabricatedDecision(t, root, "alpha-loop", DecisionRejected, "too broad", false, false, "2026-06-11T10:30:00Z")
	// An undecided loop must not appear.
	undecided := Loop{ID: "still-running", Goal: "g", Agent: "a", Status: StatusRunning, Verifier: []Stage{{Name: "t", Cmd: "true"}}, Budget: DefaultBudget, CreatedAt: "2026-06-11T09:30:00Z"}
	if err := SaveLoop(root, undecided); err != nil {
		t.Fatal(err)
	}

	entries, err := LogbookEntries(root)
	if err != nil {
		t.Fatalf("LogbookEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].LoopID != "zeta-loop" || entries[1].LoopID != "alpha-loop" {
		t.Fatalf("entries must be ordered by decision time, got %s then %s", entries[0].LoopID, entries[1].LoopID)
	}
}

func TestLogbookEntriesEmptyProject(t *testing.T) {
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	entries, err := LogbookEntries(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("empty project: want no entries and no error, got %v, %v", entries, err)
	}
}
