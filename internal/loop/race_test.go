package loop

import (
	"os"
	"strings"
	"testing"
)

// TestRaceJudgesParallelLoops races two scripted agents on one goal: both
// go green, but one drags a dependency manifest along. The judge must pick
// the clean diff and the verdict must be durable on disk.
func TestRaceJudgesParallelLoops(t *testing.T) {
	root := newLoopProject(t, "true") // registers "scripted"; we add the racers
	if err := AddAgent(root, "tidy", "echo alpha >> target.txt", false); err != nil {
		t.Fatal(err)
	}
	if err := AddAgent(root, "messy", `echo alpha >> target.txt && echo '{"dep":"new"}' > package.json`, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/target.txt", []byte("start\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "racers")

	record, err := RunRace(root, CreateOptions{
		Goal:     "make target contain alpha",
		Verifier: []Stage{{Name: "alpha", Cmd: `grep -q alpha target.txt || { echo "need alpha"; exit 1; }`}},
		Budget:   Budget{MaxIterations: 2},
	}, []string{"tidy", "messy"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(record.Loops) != 2 {
		t.Fatalf("want 2 racing loops, got %v", record.Loops)
	}
	for _, id := range record.Loops {
		l, err := LoadLoop(root, id)
		if err != nil {
			t.Fatal(err)
		}
		if l.Status != StatusGreen {
			t.Fatalf("loop %s should be green, got %s (%s)", id, l.Status, l.ParkedReason)
		}
	}

	if record.Verdict.Winner == "" || !strings.Contains(record.Verdict.Winner, "tidy") {
		t.Fatalf("judge should pick the clean diff, got %q (%s)", record.Verdict.Winner, record.Verdict.Reason)
	}
	// The race record is durable and inspectable.
	var stored RaceRecord
	if err := ReadJSON(racePath(root, record.ID), &stored); err != nil {
		t.Fatalf("race.json must be on disk: %v", err)
	}
	if stored.Verdict.Winner != record.Verdict.Winner {
		t.Fatalf("stored verdict differs: %q vs %q", stored.Verdict.Winner, record.Verdict.Winner)
	}
}

func TestRaceNeedsTwoDistinctAgents(t *testing.T) {
	root := newLoopProject(t, "true")
	if _, err := RunRace(root, CreateOptions{Goal: "g", Verifier: []Stage{{Name: "t", Cmd: "true"}}}, []string{"scripted"}, nil); err == nil {
		t.Fatal("a one-agent race must refuse")
	}
	if _, err := RunRace(root, CreateOptions{Goal: "g", Verifier: []Stage{{Name: "t", Cmd: "true"}}}, []string{"scripted", "scripted"}, nil); err == nil {
		t.Fatal("a duplicate-agent race must refuse")
	}
}
