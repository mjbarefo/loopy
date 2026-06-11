package loop

import (
	"testing"
)

// fabricatedCandidate writes a finished loop plus one recorded iteration —
// the judge only reads evidence, so no git is needed.
func fabricatedCandidate(t *testing.T, root, id, status string, diffBytes int, files []string, iters int) {
	t.Helper()
	l := Loop{
		ID: id, Goal: "shared goal", Agent: "agent-" + id, Status: status,
		Verifier: []Stage{{Name: "t", Cmd: "true"}}, Budget: DefaultBudget,
		IterationsUsed: iters, WallClockUsed: Duration(int64(iters) * 1e9),
		CreatedAt: "2026-06-11T10:00:00Z", EndedAt: "2026-06-11T10:05:00Z",
	}
	if status == StatusParked {
		l.ParkedReason = "budget exhausted"
	}
	if err := SaveLoop(root, l); err != nil {
		t.Fatal(err)
	}
	it := Iteration{
		Index: 1, Green: status == StatusGreen,
		DiffBytes: diffBytes, ChangedFiles: files, DiffHash: "h-" + id,
		StartedAt: "2026-06-11T10:00:00Z", EndedAt: "2026-06-11T10:05:00Z",
	}
	if err := SaveIteration(root, id, it); err != nil {
		t.Fatal(err)
	}
}

func judgeProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestJudgePrefersSmallestCleanGreen(t *testing.T) {
	root := judgeProject(t)
	fabricatedCandidate(t, root, "clean-small", StatusGreen, 400, []string{"a.go"}, 2)
	fabricatedCandidate(t, root, "clean-large", StatusGreen, 4000, []string{"a.go", "b.go", "c.go"}, 2)
	fabricatedCandidate(t, root, "manifest-small", StatusGreen, 200, []string{"go.mod"}, 1)

	v, err := Judge(root, []string{"manifest-small", "clean-large", "clean-small"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Winner != "clean-small" {
		t.Fatalf("winner = %q (%s), want clean-small", v.Winner, v.Reason)
	}
	if v.Candidates[0].LoopID != "clean-small" || v.Candidates[1].LoopID != "clean-large" || v.Candidates[2].LoopID != "manifest-small" {
		t.Fatalf("ranking wrong: %s, %s, %s", v.Candidates[0].LoopID, v.Candidates[1].LoopID, v.Candidates[2].LoopID)
	}
	if len(v.Candidates[2].Manifests) == 0 {
		t.Fatal("the manifest-touching candidate must be flagged")
	}
}

func TestJudgeNoGreenIsNoSafeWinner(t *testing.T) {
	root := judgeProject(t)
	fabricatedCandidate(t, root, "red-one", StatusParked, 500, []string{"a.go"}, 8)
	fabricatedCandidate(t, root, "red-two", StatusParked, 100, []string{"b.go"}, 8)

	v, err := Judge(root, []string{"red-one", "red-two"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Winner != "" || v.Reason == "" {
		t.Fatalf("no green loops must yield no safe winner, got %q (%s)", v.Winner, v.Reason)
	}
}

func TestJudgeAllManifestGreensIsNoSafeWinner(t *testing.T) {
	root := judgeProject(t)
	fabricatedCandidate(t, root, "m-one", StatusGreen, 300, []string{"go.mod", "x.go"}, 1)
	fabricatedCandidate(t, root, "m-two", StatusGreen, 700, []string{"package.json"}, 2)

	v, err := Judge(root, []string{"m-one", "m-two"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Winner != "" {
		t.Fatalf("all-manifest greens must yield no safe winner, got %q", v.Winner)
	}
}

func TestJudgeFlagsOverlap(t *testing.T) {
	root := judgeProject(t)
	fabricatedCandidate(t, root, "overlap-a", StatusGreen, 300, []string{"shared.go", "a.go"}, 1)
	fabricatedCandidate(t, root, "overlap-b", StatusGreen, 500, []string{"shared.go", "b.go"}, 1)

	v, err := Judge(root, []string{"overlap-a", "overlap-b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Overlaps) != 1 || len(v.Overlaps[0].Files) != 1 || v.Overlaps[0].Files[0] != "shared.go" {
		t.Fatalf("overlap on shared.go must be flagged, got %+v", v.Overlaps)
	}
}

func TestJudgeIsDeterministicUnderInputOrder(t *testing.T) {
	root := judgeProject(t)
	fabricatedCandidate(t, root, "d-one", StatusGreen, 400, []string{"a.go"}, 2)
	fabricatedCandidate(t, root, "d-two", StatusGreen, 400, []string{"b.go"}, 2)
	fabricatedCandidate(t, root, "d-three", StatusParked, 100, []string{"c.go"}, 8)

	orders := [][]string{
		{"d-one", "d-two", "d-three"},
		{"d-three", "d-two", "d-one"},
		{"d-two", "d-three", "d-one"},
	}
	var winner string
	var ranking [3]string
	for i, order := range orders {
		v, err := Judge(root, order)
		if err != nil {
			t.Fatal(err)
		}
		var got [3]string
		for j, c := range v.Candidates {
			got[j] = c.LoopID
		}
		if i == 0 {
			winner, ranking = v.Winner, got
			continue
		}
		if v.Winner != winner || got != ranking {
			t.Fatalf("verdict depends on input order: %v vs %v (winner %q vs %q)", got, ranking, v.Winner, winner)
		}
	}
	// Equal evidence ties break on loop ID, lexicographically.
	if winner != "d-one" {
		t.Fatalf("tie-break must be lexicographic, got %q", winner)
	}
}

func TestJudgeRefusesUnfinishedLoops(t *testing.T) {
	root := judgeProject(t)
	fabricatedCandidate(t, root, "done-loop", StatusGreen, 100, []string{"a.go"}, 1)
	running := Loop{ID: "running-loop", Goal: "g", Agent: "a", Status: StatusRunning, Verifier: []Stage{{Name: "t", Cmd: "true"}}, Budget: DefaultBudget, CreatedAt: "2026-06-11T10:00:00Z"}
	if err := SaveLoop(root, running); err != nil {
		t.Fatal(err)
	}
	if _, err := Judge(root, []string{"done-loop", "running-loop"}); err == nil {
		t.Fatal("judging a running loop must refuse")
	}
}
