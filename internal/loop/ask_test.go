package loop

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// askOutcome runs a single ask stage against a scripted agent and returns the
// outcome plus the verifier log. The agent command is the verdict source: it
// receives the judge prompt and is expected to print PASS or FAIL: ....
func askOutcome(t *testing.T, agentCmd, goal, question string, diff []byte) (VerifierOutcome, string) {
	t.Helper()
	root := newLoopProject(t, agentCmd)
	dir := t.TempDir()
	var log bytes.Buffer
	out, err := RunVerifier(context.Background(), dir, []Stage{
		{Name: "judge", Kind: KindAsk, Ask: question},
	}, &log, &AskContext{Root: root, Goal: goal, Agent: "scripted", Diff: diff})
	if err != nil {
		t.Fatalf("RunVerifier: %v\nlog:\n%s", err, log.String())
	}
	return out, log.String()
}

func TestAskStagePassGreens(t *testing.T) {
	out, log := askOutcome(t, `echo PASS`, "add a file", "is the goal met?", nil)
	if !out.Green {
		t.Fatalf("expected green, got %+v\nlog:\n%s", out, log)
	}
	if len(out.Stages) != 1 || out.Stages[0].Kind != KindAsk {
		t.Fatalf("stage = %+v", out.Stages)
	}
	if out.Stages[0].Cmd != "is the goal met?" {
		t.Fatalf("ask stage descriptor should be the question, got %q", out.Stages[0].Cmd)
	}
	if !strings.Contains(log, "ask verdict: PASS") {
		t.Fatalf("log missing PASS verdict:\n%s", log)
	}
}

func TestAskStageFailCarriesReason(t *testing.T) {
	out, _ := askOutcome(t,
		`echo thinking about it; echo "FAIL: AGENTS.md is missing the build step"`,
		"document the build", "is AGENTS.md accurate?", nil)
	if out.Green {
		t.Fatal("expected red")
	}
	if out.FailingStage != "judge" {
		t.Fatalf("failing stage = %q", out.FailingStage)
	}
	if !strings.Contains(out.FeedbackTail, "missing the build step") {
		t.Fatalf("the FAIL reason must reach the next prompt; tail = %q", out.FeedbackTail)
	}
	if out.TailHash() == "" {
		t.Fatal("a red ask still needs a tail hash so stuck detection has something to compare")
	}
}

func TestAskStageNoVerdictFailsClosed(t *testing.T) {
	out, _ := askOutcome(t, `echo "looks fine to me honestly"`, "g", "q", nil)
	if out.Green {
		t.Fatal("an answer that is neither PASS nor FAIL must fail closed")
	}
	if !strings.Contains(out.FeedbackTail, "PASS or FAIL") {
		t.Fatalf("feedback should explain the broken protocol; tail = %q", out.FeedbackTail)
	}
}

func TestAskStagePassAmidNoise(t *testing.T) {
	out, _ := askOutcome(t,
		`printf 'I read the files.\nThe doc is accurate and complete.\nPASS\n'`,
		"g", "q", nil)
	if !out.Green {
		t.Fatalf("a verdict on the final line after reasoning should pass; got %+v", out)
	}
}

func TestAskStageNilContextErrors(t *testing.T) {
	var log bytes.Buffer
	_, err := RunVerifier(context.Background(), t.TempDir(), []Stage{
		{Name: "judge", Kind: KindAsk, Ask: "q"},
	}, &log, nil)
	if err == nil {
		t.Fatal("an ask stage with no agent context must error, never silently pass")
	}
}

// An ask stage is the expensive, key-requiring stage; a red command gate ahead
// of it must short-circuit before any agent call.
func TestAskStageRunsOnlyAfterGatesGreen(t *testing.T) {
	root := newLoopProject(t, `touch judged.marker; echo PASS`)
	dir := t.TempDir()
	var log bytes.Buffer
	out, err := RunVerifier(context.Background(), dir, []Stage{
		{Name: "gate", Cmd: `echo nope; exit 1`},
		{Name: "judge", Kind: KindAsk, Ask: "q"},
	}, &log, &AskContext{Root: root, Goal: "g", Agent: "scripted"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Green || out.FailingStage != "gate" {
		t.Fatalf("expected the gate to fail, got %+v", out)
	}
	if len(out.Stages) != 1 {
		t.Fatalf("the ask stage must not run after a red gate: %+v", out.Stages)
	}
	if _, err := os.Stat(filepath.Join(dir, "judged.marker")); err == nil {
		t.Fatal("the ask agent ran even though a prior gate was red")
	}
}

func TestCreateLoopAskStageValidation(t *testing.T) {
	root := newLoopProject(t, `echo PASS`)
	// An ask-only verifier is allowed: no deterministic floor is required.
	if _, err := CreateLoop(root, CreateOptions{
		Goal:     "make the readme friendlier",
		Verifier: []Stage{{Name: "j", Kind: KindAsk, Ask: "is it friendlier and still accurate?"}},
	}); err != nil {
		t.Fatalf("ask-only verifier should be allowed: %v", err)
	}
	// An ask stage with no question is rejected (the ask-stage analogue of an
	// empty command).
	if _, err := CreateLoop(root, CreateOptions{
		Goal:     "real goal here",
		Verifier: []Stage{{Name: "j", Kind: KindAsk, Ask: "   "}},
	}); err == nil {
		t.Fatal("an ask stage with no question must be rejected")
	}
	// An unknown kind is rejected rather than silently treated as a command.
	if _, err := CreateLoop(root, CreateOptions{
		Goal:     "real goal here",
		Verifier: []Stage{{Name: "j", Kind: "wat", Cmd: "true"}},
	}); err == nil {
		t.Fatal("an unknown stage kind must be rejected")
	}
	// An ask-stage agent override that doesn't resolve is caught at creation.
	if _, err := CreateLoop(root, CreateOptions{
		Goal:     "real goal here",
		Verifier: []Stage{{Name: "j", Kind: KindAsk, Ask: "q", Agent: "ghost"}},
	}); err == nil {
		t.Fatal("an unresolvable ask-stage agent override must be rejected")
	}
}

// End to end: a hybrid verifier (command gate + ask stage) drives a real loop
// to green. The one scripted agent plays both roles, branching on whether it
// was handed the judge prompt.
func TestLoopGreensThroughAskStage(t *testing.T) {
	agent := `if grep -q "FINAL output line EXACTLY" {prompt_file}; then echo PASS; else echo done > done.txt; fi`
	root := newLoopProject(t, agent)
	l := mustCreate(t, root, CreateOptions{
		Goal: "create done.txt",
		Verifier: []Stage{
			{Name: "file", Cmd: `test -f done.txt || { echo missing done.txt; exit 1; }`},
			{Name: "judge", Kind: KindAsk, Ask: "Does done.txt exist and look right?"},
		},
		Budget: Budget{MaxIterations: 3},
	})
	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusGreen {
		t.Fatalf("status = %q (parked: %q)", final.Status, final.ParkedReason)
	}
	if final.IterationsUsed != 1 {
		t.Fatalf("iterations = %d, want 1 (baseline red on the gate, green after one agent run)", final.IterationsUsed)
	}
}
