package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mjbarefo/loopy/internal/loop"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newEventTestProject mirrors the engine tests' pattern: a committed repo,
// .loopy initialized, and an inline shell agent — no mocks, no API keys.
func newEventTestProject(t *testing.T, agentCmd string) string {
	t.Helper()
	root := t.TempDir()
	gitIn(t, root, "init", "-q", "-b", "main")
	gitIn(t, root, "config", "user.email", "test@loopy.local")
	gitIn(t, root, "config", "user.name", "loopy test")
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("events\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "init")
	if _, _, err := loop.InitProject(root); err != nil {
		t.Fatal(err)
	}
	if err := loop.AddAgent(root, "scripted", agentCmd, true); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "loopy setup")
	return root
}

// TestRunJSONEventStream drives a real engine through the NDJSON emitter and
// checks the stream contract: every line parses, the events arrive in phase
// order, and the final result event carries the loop view.
func TestRunJSONEventStream(t *testing.T) {
	root := newEventTestProject(t, `echo working; echo done > done.txt`)
	l, err := loop.CreateLoop(root, loop.CreateOptions{
		Goal: "create done.txt",
		Verifier: []loop.Stage{
			{Name: "done", Cmd: `test -f done.txt || { echo "need done.txt"; exit 1; }`},
		},
		Budget: loop.Budget{MaxIterations: 3},
	})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	em := newJSONEmitter(&buf)
	final, err := loop.RunEngine(root, l.ID, em.events(l.ID))
	if err != nil {
		t.Fatal(err)
	}
	em.result(root, final)

	var events []runEvent
	for i, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var ev runEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %d is not valid JSON: %v\n%s", i+1, err, line)
		}
		if ev.TS == "" {
			t.Fatalf("line %d has no timestamp: %s", i+1, line)
		}
		events = append(events, ev)
	}

	var order []string
	for _, ev := range events {
		order = append(order, ev.Event)
		if ev.LoopID != l.ID {
			t.Fatalf("event %s has loop_id %q, want %q", ev.Event, ev.LoopID, l.ID)
		}
	}
	// The phase order of a one-iteration green loop. stage_done appears for
	// the baseline verify and the iteration's verify.
	want := []string{"loop_started", "baseline_started", "stage_done", "iteration_started", "agent_done", "stage_done", "iteration_done", "loop_ended", "result"}
	if strings.Join(order, " ") != strings.Join(want, " ") {
		t.Fatalf("event order = %v, want %v", order, want)
	}

	first, last := events[0], events[len(events)-1]
	if first.Agent != "scripted" || first.MaxIterations != 3 {
		t.Fatalf("loop_started = %+v, want agent scripted, max_iterations 3", first)
	}
	if last.Status != loop.StatusGreen || last.Result == nil {
		t.Fatalf("result event = %+v, want status green with a loop view", last)
	}
	if last.Result.Status != loop.StatusGreen || last.Result.IterationsUsed != 1 {
		t.Fatalf("result view = status %s after %d iteration(s), want green after 1", last.Result.Status, last.Result.IterationsUsed)
	}
}

// TestRunJSONFlagRefusesUnconfirmedVerifier pins the script-safety rule:
// --json never mixes an interactive confirmation into the stream, even when
// inference has a confident guess.
func TestRunJSONFlagRefusesUnconfirmedVerifier(t *testing.T) {
	root := newEventTestProject(t, `true`)
	if err := os.WriteFile(filepath.Join(root, "Makefile"), []byte("check:\n\ttrue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if inferred, ok := loop.InferVerifier(root); !ok || len(inferred.Stages) == 0 {
		t.Fatal("test setup: expected the Makefile check target to be inferrable")
	}
	if _, err := resolveVerifier(root, nil, false); err == nil {
		t.Fatal("resolveVerifier(interactive=false) accepted an unconfirmed inferred verifier")
	}
}
