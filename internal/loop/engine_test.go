package loop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newLoopProject builds a committed repo with .loopy initialized and a shell
// agent registered, returning the root. The agent command template is the
// caller's: this is how each test scripts its "agent" behavior, no API keys
// anywhere.
func newLoopProject(t *testing.T, agentCmd string) string {
	t.Helper()
	root := newTestRepo(t)
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	if err := AddAgent(root, "scripted", agentCmd, true); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "register loopy")
	return root
}

func mustCreate(t *testing.T, root string, opts CreateOptions) Loop {
	t.Helper()
	l, err := CreateLoop(root, opts)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// TestEngineConvergesToGreen is the product in one test: a two-bug repo, a
// scripted agent that fixes whatever the verifier feedback names, and a loop
// that converges in two iterations.
func TestEngineConvergesToGreen(t *testing.T) {
	// The "code under test": target.txt must contain both "alpha" and "beta".
	// The agent reads the *feedback section* of its prompt and fixes exactly
	// the failure named there — proof that feedback composition closes the
	// loop. (Grepping the whole prompt would also match the verifier
	// commands themselves.)
	agent := `fb=$(sed -n '/## Feedback/,/## Changes/p' {prompt_file}); ` +
		`case "$fb" in *"need alpha"*) echo alpha >> target.txt;; *"need beta"*) echo beta >> target.txt;; esac`
	root := newLoopProject(t, agent)
	if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte("start\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "add target")

	l := mustCreate(t, root, CreateOptions{
		Goal: "make target.txt contain alpha and beta",
		Verifier: []Stage{
			{Name: "alpha", Cmd: `grep -q alpha target.txt || { echo "need alpha in target.txt"; exit 1; }`},
			{Name: "beta", Cmd: `grep -q beta target.txt || { echo "need beta in target.txt"; exit 1; }`},
		},
	})

	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusGreen {
		t.Fatalf("status = %s (%s), want green", final.Status, final.ParkedReason)
	}
	if final.IterationsUsed != 2 {
		t.Fatalf("iterations used = %d, want 2", final.IterationsUsed)
	}

	iterations, err := LoadIterations(root, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(iterations) != 3 { // baseline + 2 agent iterations
		t.Fatalf("recorded %d iterations, want 3", len(iterations))
	}
	if iterations[0].AgentExit != nil || iterations[0].Green {
		t.Fatalf("baseline = %+v", iterations[0])
	}
	if iterations[0].FailingStage != "alpha" {
		t.Fatalf("baseline failing stage = %q", iterations[0].FailingStage)
	}
	if !iterations[2].Green {
		t.Fatalf("final iteration not green: %+v", iterations[2])
	}

	// The iteration-2 prompt must carry iteration-1's failure feedback.
	prompt, err := os.ReadFile(filepath.Join(IterationDir(root, l.ID, 2), PromptFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "need beta") {
		t.Fatalf("iteration 2 prompt missing feedback:\n%s", prompt)
	}
	if !strings.Contains(string(prompt), "target.txt") {
		t.Fatal("prompt missing changed-files summary")
	}

	// Evidence on disk: diff.patch, agent.log, verifier.log per iteration.
	for _, name := range []string{DiffFile, AgentLogFile, VerifierLogFile, PromptFile} {
		if _, err := os.Stat(filepath.Join(IterationDir(root, l.ID, 2), name)); err != nil {
			t.Errorf("iteration 2 missing %s: %v", name, err)
		}
	}
	final2, err := LoadLoop(root, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final2.Status != StatusGreen || final2.EndedAt == "" {
		t.Fatalf("persisted loop = %+v", final2)
	}
}

func TestEngineGreenAtBaseline(t *testing.T) {
	root := newLoopProject(t, `echo "should never run" > should-not-exist.txt`)
	l := mustCreate(t, root, CreateOptions{
		Goal:     "nothing to do",
		Verifier: []Stage{{Name: "ok", Cmd: "true"}},
	})
	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusGreen {
		t.Fatalf("status = %s", final.Status)
	}
	if final.IterationsUsed != 0 {
		t.Fatalf("iterations used = %d, want 0 (no agent run)", final.IterationsUsed)
	}
	if !strings.Contains(final.ParkedReason, "baseline") {
		t.Fatalf("reason = %q", final.ParkedReason)
	}
	if _, err := os.Stat(filepath.Join(final.Worktree, "should-not-exist.txt")); !os.IsNotExist(err) {
		t.Fatal("agent ran despite green baseline")
	}
}

func TestEngineParksWhenAgentDoesNothing(t *testing.T) {
	root := newLoopProject(t, "true") // agent exits 0, changes nothing
	l := mustCreate(t, root, CreateOptions{
		Goal:     "unachievable",
		Verifier: []Stage{{Name: "always-red", Cmd: "echo still broken; false"}},
	})
	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusParked {
		t.Fatalf("status = %s", final.Status)
	}
	if !strings.Contains(final.ParkedReason, "no change") {
		t.Fatalf("reason = %q, want no-change rule", final.ParkedReason)
	}
	if final.IterationsUsed != 1 {
		t.Fatalf("iterations used = %d, want 1", final.IterationsUsed)
	}
}

// TestEngineParksAgentBlocked: a nonzero agent exit with an untouched
// worktree is an environment failure (trust prompt, dead auth, missing CLI),
// parked as "agent blocked" with the CLI's own last words — never as stuck.
func TestEngineParksAgentBlocked(t *testing.T) {
	agent := `printf '\033[31mdemo CLI: directory not trusted, refusing to run\033[0m\n' >&2; exit 55`
	root := newLoopProject(t, agent)
	l := mustCreate(t, root, CreateOptions{
		Goal:     "unachievable",
		Verifier: []Stage{{Name: "red", Cmd: "test -f done.txt"}},
	})
	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusParked {
		t.Fatalf("status = %s", final.Status)
	}
	if !strings.Contains(final.ParkedReason, "agent blocked (exit 55)") {
		t.Fatalf("reason = %q, want agent blocked", final.ParkedReason)
	}
	if !strings.Contains(final.ParkedReason, "directory not trusted") {
		t.Fatalf("reason = %q, want the agent's own words", final.ParkedReason)
	}
	if strings.Contains(final.ParkedReason, "stuck") {
		t.Fatalf("reason = %q must not read as stuck", final.ParkedReason)
	}
	if strings.Contains(final.ParkedReason, "\x1b") {
		t.Fatalf("reason = %q must be ANSI-free", final.ParkedReason)
	}
	if final.IterationsUsed != 1 {
		t.Fatalf("iterations used = %d, want 1 — a blocked agent must not burn budget", final.IterationsUsed)
	}
}

// TestEngineNonzeroExitWithProgressIsNotBlocked: an agent that did real work
// before exiting nonzero is judged by its verifier, not parked as blocked.
func TestEngineNonzeroExitWithProgressIsNotBlocked(t *testing.T) {
	root := newLoopProject(t, `echo done > done.txt; exit 1`)
	l := mustCreate(t, root, CreateOptions{
		Goal:     "create done.txt",
		Verifier: []Stage{{Name: "done", Cmd: "test -f done.txt"}},
	})
	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusGreen {
		t.Fatalf("status = %s (%s), want green — the verifier outranks the exit code", final.Status, final.ParkedReason)
	}
}

func TestEngineParksOnRepeatedIdenticalFailure(t *testing.T) {
	// The agent always changes something (so no-change never fires) but never
	// fixes the failure, whose output is identical every time.
	root := newLoopProject(t, `date +%N >> churn.txt`)
	l := mustCreate(t, root, CreateOptions{
		Goal:     "unachievable",
		Verifier: []Stage{{Name: "red", Cmd: "echo same failure forever; false"}},
		Stuck:    StuckPolicy{SameFailureRepeats: 3, NoChangeRepeats: 99},
	})
	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusParked {
		t.Fatalf("status = %s", final.Status)
	}
	if !strings.Contains(final.ParkedReason, "identically for 3") {
		t.Fatalf("reason = %q, want same-failure rule", final.ParkedReason)
	}
	if final.IterationsUsed != 3 {
		t.Fatalf("iterations used = %d, want 3", final.IterationsUsed)
	}
}

func TestEngineParksOnIterationBudget(t *testing.T) {
	// Failure output varies (defeats same-failure) and the diff churns
	// (defeats no-change): only the hard cap can stop this loop.
	root := newLoopProject(t, `date +%N >> churn.txt`)
	l := mustCreate(t, root, CreateOptions{
		Goal:     "unachievable",
		Verifier: []Stage{{Name: "red", Cmd: "cat churn.txt 2>/dev/null; echo broken; false"}},
		Budget:   Budget{MaxIterations: 2, MaxWallClock: Duration(time.Hour)},
		Stuck:    StuckPolicy{SameFailureRepeats: 99, NoChangeRepeats: 99},
	})
	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusParked {
		t.Fatalf("status = %s", final.Status)
	}
	if !strings.Contains(final.ParkedReason, "budget exhausted: 2/2 iterations") {
		t.Fatalf("reason = %q", final.ParkedReason)
	}
}

func TestEngineForbiddenPathViolationFailsIteration(t *testing.T) {
	// First iteration touches vendor/ (forbidden); the violation is fed back
	// and the agent then fixes properly.
	agent := `if grep -q "forbidden paths were modified" {prompt_file}; then ` +
		`rm -rf vendor; echo fixed > target.txt; ` +
		`else mkdir -p vendor && echo bad > vendor/lib.txt; fi`
	root := newLoopProject(t, agent)
	l := mustCreate(t, root, CreateOptions{
		Goal:           "fix target without touching vendor",
		Verifier:       []Stage{{Name: "fixed", Cmd: "grep -q fixed target.txt 2>/dev/null || { echo need fixed; exit 1; }"}},
		ForbiddenPaths: []string{"vendor/"},
	})
	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusGreen {
		t.Fatalf("status = %s (%s)", final.Status, final.ParkedReason)
	}
	iterations, err := LoadIterations(root, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	first := iterations[1]
	if first.Violation == "" || first.FailingStage != "forbidden-paths" {
		t.Fatalf("iteration 1 should record the violation: %+v", first)
	}
	if len(first.Stages) != 0 {
		t.Fatal("verifier should be skipped on violation")
	}
}

func TestEnginePauseAndResume(t *testing.T) {
	agent := `echo fixed > target.txt`
	root := newLoopProject(t, agent)
	l := mustCreate(t, root, CreateOptions{
		Goal:     "fix target",
		Verifier: []Stage{{Name: "fixed", Cmd: "grep -q fixed target.txt 2>/dev/null || { echo need fixed; exit 1; }"}},
	})
	// Pause requested before the engine starts: it parks as paused at the
	// first boundary, before the baseline.
	if err := WriteControl(root, l.ID, Control{Pause: true}); err != nil {
		t.Fatal(err)
	}
	paused, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if paused.Status != StatusPaused {
		t.Fatalf("status = %s, want paused", paused.Status)
	}
	if n, _ := LoadIterations(root, l.ID); len(n) != 0 {
		t.Fatalf("recorded %d iterations while paused", len(n))
	}

	// Resume: clear control, run again, converge.
	if err := ClearControl(root, l.ID); err != nil {
		t.Fatal(err)
	}
	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusGreen {
		t.Fatalf("status = %s (%s)", final.Status, final.ParkedReason)
	}
}

func TestEngineAbortWithoutEngineParksDirectly(t *testing.T) {
	root := newLoopProject(t, "true")
	l := mustCreate(t, root, CreateOptions{
		Goal:     "whatever",
		Verifier: []Stage{{Name: "red", Cmd: "false"}},
	})
	if err := ParkAborted(root, l.ID, "changed my mind"); err != nil {
		t.Fatal(err)
	}
	final, err := LoadLoop(root, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusParked || !strings.Contains(final.ParkedReason, "changed my mind") {
		t.Fatalf("loop = %+v", final)
	}
	// Terminal loops refuse both engines and second aborts.
	if _, err := RunEngine(root, l.ID, Events{}); err == nil {
		t.Fatal("engine should refuse a parked loop")
	}
	if err := ParkAborted(root, l.ID, "again"); err == nil {
		t.Fatal("second abort should refuse")
	}
}

func TestEngineRefusesConcurrentRun(t *testing.T) {
	root := newLoopProject(t, "true")
	l := mustCreate(t, root, CreateOptions{
		Goal:     "whatever",
		Verifier: []Stage{{Name: "red", Cmd: "false"}},
	})
	// Simulate a live engine: lock held by this (alive) process.
	if err := AcquireEngineLock(root, l.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ReleaseEngineLock(root, l.ID) })
	// A second engine in another process would be refused; same-pid locks
	// are re-entrant by design, so assert on the lock state instead.
	if _, held, stale := EngineLockState(root, l.ID); !held || stale {
		t.Fatalf("lock should read as held: held=%v stale=%v", held, stale)
	}
}

func TestCreateLoopValidation(t *testing.T) {
	root := newLoopProject(t, "true")
	if _, err := CreateLoop(root, CreateOptions{Goal: "g"}); err == nil {
		t.Fatal("expected refusal without verifier")
	}
	if _, err := CreateLoop(root, CreateOptions{Goal: " ", Verifier: []Stage{{Name: "x", Cmd: "true"}}}); err == nil {
		t.Fatal("expected refusal without goal")
	}
	if _, err := CreateLoop(root, CreateOptions{Goal: "g", Verifier: []Stage{{Name: "x", Cmd: "  "}}}); err == nil {
		t.Fatal("expected refusal for empty stage command")
	}
	if _, err := CreateLoop(root, CreateOptions{Goal: "g", Verifier: []Stage{{Name: "x", Cmd: "true"}}, Agent: "ghost"}); err == nil {
		t.Fatal("expected refusal for unknown agent")
	}
	// Defaults fill in.
	l := mustCreate(t, root, CreateOptions{Goal: "real goal here", Verifier: []Stage{{Name: "x", Cmd: "true"}}})
	if l.Budget.MaxIterations != DefaultBudget.MaxIterations || l.Stuck.SameFailureRepeats != DefaultStuckPolicy.SameFailureRepeats {
		t.Fatalf("defaults not applied: %+v", l)
	}
	if l.Agent != "scripted" {
		t.Fatalf("agent = %q, want default", l.Agent)
	}
	// Same goal → disambiguated ID.
	l2 := mustCreate(t, root, CreateOptions{Goal: "real goal here", Verifier: []Stage{{Name: "x", Cmd: "true"}}})
	if l2.ID != l.ID+"-2" {
		t.Fatalf("second ID = %q", l2.ID)
	}
}

func TestDetectStuckIgnoresBaseline(t *testing.T) {
	l := Loop{Stuck: StuckPolicy{SameFailureRepeats: 2, NoChangeRepeats: 2}}
	baseline := Iteration{Index: 0, TailHash: "h1", DiffHash: hashBytes(nil)}
	// One agent iteration matching the baseline failure: not stuck yet.
	one := Iteration{Index: 1, TailHash: "h1", DiffHash: "changed"}
	if reason, stuck := detectStuck(l, []Iteration{baseline, one}); stuck {
		t.Fatalf("stuck too early: %s", reason)
	}
	two := Iteration{Index: 2, TailHash: "h1", DiffHash: "changed2"}
	if _, stuck := detectStuck(l, []Iteration{baseline, one, two}); !stuck {
		t.Fatal("two identical agent failures should be stuck at threshold 2")
	}
}

// TestEnginePhaseRecording: the engine publishes phase.json while a phase
// runs (the agent observes "agent") and clears it when the loop ends.
func TestEnginePhaseRecording(t *testing.T) {
	// The agent copies the live phase record into the worktree, where the
	// snapshot preserves it as evidence the test can assert on. The path
	// travels via the environment: template variables expand shell-quoted,
	// which would defeat the nested double quotes here.
	agent := `cp "$LOOPY_PHASE_FILE" phase-seen.json 2>/dev/null; echo done >> log.txt`
	root := newLoopProject(t, agent)

	l := mustCreate(t, root, CreateOptions{
		Goal:     "observe the phase file",
		Verifier: []Stage{{Name: "check", Cmd: "test -f phase-seen.json"}},
		Budget:   Budget{MaxIterations: 2},
	})
	t.Setenv("LOOPY_PHASE_FILE", filepath.Join(LoopDir(root, l.ID), "phase.json"))
	final, err := RunEngine(root, l.ID, Events{})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != StatusGreen {
		t.Fatalf("status = %s (%s), want green", final.Status, final.ParkedReason)
	}

	var seen Phase
	if err := ReadJSON(filepath.Join(WorktreePath(root, l.ID), "phase-seen.json"), &seen); err != nil {
		t.Fatal(err)
	}
	if seen.Phase != PhaseAgent || seen.Iteration != 1 || seen.StartedAt == "" {
		t.Fatalf("agent observed phase %+v, want agent/iteration 1", seen)
	}
	if _, found, _ := ReadPhase(root, l.ID); found {
		t.Fatal("phase.json should be cleared after the loop ends")
	}
}
