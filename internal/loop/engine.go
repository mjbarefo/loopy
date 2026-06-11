package loop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// controlPollInterval is how often a running agent/verifier phase checks for
// an abort request. Pause waits for the iteration boundary; abort doesn't.
const controlPollInterval = 2 * time.Second

// CreateOptions describes a new loop. Verifier is mandatory — no verifier,
// no loop; resolving defaults/inference is the caller's job, refusing an
// empty result is CreateLoop's.
type CreateOptions struct {
	Goal           string
	Agent          string // empty = registry default
	Verifier       []Stage
	Constraints    []string
	ForbiddenPaths []string
	Budget         Budget      // zero fields filled from defaults
	Stuck          StuckPolicy // zero fields filled from defaults
	// IDHint, when set, seeds the loop ID instead of the goal (race mode
	// appends the agent name so competitors get distinct IDs).
	IDHint string
}

// CreateLoop validates options, claims an ID, creates the isolated worktree,
// and records the loop as running. It does not start the engine.
func CreateLoop(root string, opts CreateOptions) (Loop, error) {
	if err := EnsureInitialized(root); err != nil {
		return Loop{}, err
	}
	goal := strings.TrimSpace(opts.Goal)
	if goal == "" {
		return Loop{}, errors.New("a goal is required")
	}
	if len(opts.Verifier) == 0 {
		return Loop{}, errors.New("a loop cannot be created without a verifier (pass --verify or configure a default)")
	}
	for _, stage := range opts.Verifier {
		if strings.TrimSpace(stage.Cmd) == "" {
			return Loop{}, fmt.Errorf("verifier stage %q has an empty command", stage.Name)
		}
	}
	agentName, _, err := ResolveAgent(root, opts.Agent)
	if err != nil {
		return Loop{}, err
	}

	budget := opts.Budget
	if budget.MaxIterations <= 0 {
		budget.MaxIterations = DefaultBudget.MaxIterations
	}
	if budget.MaxWallClock <= 0 {
		budget.MaxWallClock = DefaultBudget.MaxWallClock
	}
	stuck := opts.Stuck
	if stuck.SameFailureRepeats <= 0 {
		stuck.SameFailureRepeats = DefaultStuckPolicy.SameFailureRepeats
	}
	if stuck.NoChangeRepeats <= 0 {
		stuck.NoChangeRepeats = DefaultStuckPolicy.NoChangeRepeats
	}

	ids, err := LoopIDs(root)
	if err != nil {
		return Loop{}, err
	}
	seed := goal
	if strings.TrimSpace(opts.IDHint) != "" {
		seed = opts.IDHint
	}
	id := UniqueLoopID(seed, ids)

	worktree, branch, base, err := CreateLoopWorktree(root, id)
	if err != nil {
		return Loop{}, err
	}
	l := Loop{
		ID:             id,
		Goal:           goal,
		Constraints:    opts.Constraints,
		ForbiddenPaths: opts.ForbiddenPaths,
		Agent:          agentName,
		Verifier:       opts.Verifier,
		Budget:         budget,
		Stuck:          stuck,
		Status:         StatusRunning,
		BaseCommit:     base,
		Branch:         branch,
		Worktree:       worktree,
		CreatedAt:      utcNowISO(),
	}
	if err := SaveLoop(root, l); err != nil {
		return Loop{}, err
	}
	return l, nil
}

// Events are optional engine progress callbacks; any field may be nil. The
// engine is the single writer of state — events are for rendering only.
type Events struct {
	LoopStarted      func(Loop)
	BaselineStarted  func()
	IterationStarted func(index, maxIterations int)
	AgentDone        func(index, exitCode int, d time.Duration)
	StageDone        func(index int, r StageResult)
	IterationDone    func(Iteration, Loop)
	Note             func(string)
	LoopEnded        func(Loop)
}

func (e Events) note(format string, args ...any) {
	if e.Note != nil {
		e.Note(fmt.Sprintf(format, args...))
	}
}

// RunEngine drives a loop until it parks: green, budget exhausted, stuck,
// aborted, or paused (pause returns with status paused; the loop is not
// done). State is flushed to disk after every phase, so a crashed engine
// resumes exactly where it stopped.
func RunEngine(root, loopID string, ev Events) (Loop, error) {
	if err := AcquireEngineLock(root, loopID); err != nil {
		return Loop{}, err
	}
	defer func() { _ = ReleaseEngineLock(root, loopID) }()

	l, err := LoadLoop(root, loopID)
	if err != nil {
		return Loop{}, err
	}
	if l.Done() {
		return l, fmt.Errorf("loop %s is already %s", l.ID, l.Status)
	}
	if info, statErr := os.Stat(l.Worktree); statErr != nil || !info.IsDir() {
		return l, fmt.Errorf("loop worktree missing: %s (see `loopy doctor`)", l.Worktree)
	}
	_, agent, err := ResolveAgent(root, l.Agent)
	if err != nil {
		return l, err
	}
	if l.Status != StatusRunning {
		l.Status = StatusRunning
		if err := SaveLoop(root, l); err != nil {
			return l, err
		}
	}
	if ev.LoopStarted != nil {
		ev.LoopStarted(l)
	}

	iterations, err := LoadIterations(root, l.ID)
	if err != nil {
		return l, err
	}

	for {
		// Phase boundary: control requests beat everything else.
		ctrl, err := ReadControl(root, l.ID)
		if err != nil {
			return l, err
		}
		if ctrl.Abort {
			return parkLoop(root, l, abortReason(ctrl), ev)
		}
		if ctrl.Pause {
			l.Status = StatusPaused
			if err := SaveLoop(root, l); err != nil {
				return l, err
			}
			ev.note("paused (resume with `loopy resume %s`)", l.ID)
			if ev.LoopEnded != nil {
				ev.LoopEnded(l)
			}
			return l, nil
		}

		// Baseline: verify before the first agent run. Seeds the first
		// prompt's feedback, and a goal that's already green costs zero
		// agent runs.
		if len(iterations) == 0 {
			if ev.BaselineStarted != nil {
				ev.BaselineStarted()
			}
			baseline, err := runBaseline(root, l, ev)
			if err != nil {
				return l, err
			}
			iterations = append(iterations, baseline)
			l.WallClockUsed += Duration(baseline.durationOrZero())
			if baseline.Green {
				return endLoop(root, l, StatusGreen, "green at baseline: the verifier already passes", ev)
			}
			if err := SaveLoop(root, l); err != nil {
				return l, err
			}
			continue
		}

		last := iterations[len(iterations)-1]
		if last.Green {
			return endLoop(root, l, StatusGreen, "", ev)
		}

		// Hard caps.
		if l.IterationsUsed >= l.Budget.MaxIterations {
			return parkLoop(root, l, fmt.Sprintf("budget exhausted: %d/%d iterations used", l.IterationsUsed, l.Budget.MaxIterations), ev)
		}
		if l.WallClockUsed >= l.Budget.MaxWallClock {
			return parkLoop(root, l, fmt.Sprintf("budget exhausted: %s of %s wall clock used", time.Duration(l.WallClockUsed).Round(time.Millisecond), time.Duration(l.Budget.MaxWallClock)), ev)
		}

		// Stuck detection: park early instead of burning budget.
		if reason, stuck := detectStuck(l, iterations); stuck {
			return parkLoop(root, l, reason, ev)
		}

		index := len(iterations)
		if ev.IterationStarted != nil {
			ev.IterationStarted(index, l.Budget.MaxIterations)
		}
		it, aborted, err := runIteration(root, l, agent, index, &last, ev)
		if err != nil {
			return l, err
		}
		iterations = append(iterations, it)
		l.IterationsUsed++
		l.WallClockUsed += Duration(it.durationOrZero())
		if err := SaveLoop(root, l); err != nil {
			return l, err
		}
		if ev.IterationDone != nil {
			ev.IterationDone(it, l)
		}
		if aborted {
			ctrl, _ := ReadControl(root, l.ID)
			return parkLoop(root, l, abortReason(ctrl), ev)
		}
		if it.Green {
			return endLoop(root, l, StatusGreen, "", ev)
		}
	}
}

// runBaseline records iteration 0: verifier only, no agent, empty diff.
func runBaseline(root string, l Loop, ev Events) (Iteration, error) {
	start := time.Now()
	it := Iteration{Index: 0, StartedAt: utcNowISO()}
	dir := IterationDir(root, l.ID, 0)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return it, err
	}
	ctx, stop := watchAbort(root, l.ID)
	defer stop()
	outcome, err := verifyToLog(ctx, l, dir, 0, ev)
	if err != nil && !errors.Is(err, context.Canceled) {
		return it, err
	}
	it.Stages = outcome.Stages
	it.Green = outcome.Green && err == nil
	it.FailingStage = outcome.FailingStage
	it.FeedbackTail = outcome.FeedbackTail
	it.TailHash = outcome.TailHash()
	it.DiffHash = hashBytes(nil)
	it.EndedAt = utcNowISO()
	it.WallMS = time.Since(start).Milliseconds()
	if err := SaveIteration(root, l.ID, it); err != nil {
		return it, err
	}
	return it, nil
}

// runIteration is one turn of the crank: compose → agent → snapshot →
// forbidden-path check → verify → record. Aborted reports whether an abort
// request interrupted the phases.
func runIteration(root string, l Loop, agent Agent, index int, prev *Iteration, ev Events) (it Iteration, aborted bool, err error) {
	start := time.Now()
	it = Iteration{Index: index, StartedAt: utcNowISO()}
	dir := IterationDir(root, l.ID, index)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return it, false, err
	}

	// Compose. The prompt is evidence: exactly what the agent was told.
	prompt := ComposePrompt(l, index, prev)
	promptPath := filepath.Join(dir, PromptFile)
	if err := writeFileAtomic(promptPath, []byte(prompt), 0o644); err != nil {
		return it, false, err
	}
	command, err := ExpandAgentCommand(agent.Cmd, TemplateContext{
		Prompt:     prompt,
		PromptFile: promptPath,
		Worktree:   l.Worktree,
		LoopID:     l.ID,
		Goal:       l.Goal,
		Iteration:  index,
	})
	if err != nil {
		return it, false, err
	}

	// Agent phase.
	ctx, stop := watchAbort(root, l.ID)
	defer stop()
	agentLog, err := os.OpenFile(filepath.Join(dir, AgentLogFile), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return it, false, err
	}
	agentStart := time.Now()
	_, agentExit, agentErr := runShell(ctx, l.Worktree, command, agentLog)
	closeErr := agentLog.Close()
	it.AgentMS = time.Since(agentStart).Milliseconds()
	it.AgentExit = &agentExit
	if ev.AgentDone != nil {
		ev.AgentDone(index, agentExit, time.Since(agentStart))
	}
	if agentErr != nil && !errors.Is(agentErr, context.Canceled) {
		return it, false, fmt.Errorf("agent command failed to run: %w", agentErr)
	}
	if closeErr != nil {
		return it, false, closeErr
	}

	// Snapshot phase: cumulative diff vs the loop's base commit, always
	// recorded — even for an aborted or failed agent run.
	diff, changed, err := Snapshot(l.Worktree, l.BaseCommit)
	if err != nil {
		return it, false, err
	}
	if err := writeFileAtomic(filepath.Join(dir, DiffFile), diff, 0o644); err != nil {
		return it, false, err
	}
	it.DiffHash = hashBytes(diff)
	it.DiffBytes = len(diff)
	it.ChangedFiles = changed

	if errors.Is(agentErr, context.Canceled) {
		it.EndedAt = utcNowISO()
		it.WallMS = time.Since(start).Milliseconds()
		return it, true, SaveIteration(root, l.ID, it)
	}

	// Forbidden paths are checked every iteration, not at the end: a
	// violation fails the iteration and the verifier is skipped.
	if violation := checkForbidden(l.ForbiddenPaths, changed); violation != "" {
		it.Violation = violation
		it.FailingStage = "forbidden-paths"
		it.FeedbackTail = violation
		it.TailHash = hashBytes([]byte("forbidden-paths\x00" + violation))
		it.EndedAt = utcNowISO()
		it.WallMS = time.Since(start).Milliseconds()
		return it, false, SaveIteration(root, l.ID, it)
	}

	// Verifier phase.
	outcome, verr := verifyToLog(ctx, l, dir, index, ev)
	if verr != nil && !errors.Is(verr, context.Canceled) {
		return it, false, verr
	}
	it.Stages = outcome.Stages
	it.Green = outcome.Green && verr == nil
	it.FailingStage = outcome.FailingStage
	it.FeedbackTail = outcome.FeedbackTail
	it.TailHash = outcome.TailHash()
	it.EndedAt = utcNowISO()
	it.WallMS = time.Since(start).Milliseconds()
	if err := SaveIteration(root, l.ID, it); err != nil {
		return it, false, err
	}
	return it, errors.Is(verr, context.Canceled), nil
}

func verifyToLog(ctx context.Context, l Loop, iterDir string, index int, ev Events) (VerifierOutcome, error) {
	logFile, err := os.OpenFile(filepath.Join(iterDir, VerifierLogFile), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return VerifierOutcome{}, err
	}
	defer logFile.Close()
	outcome, err := runVerifierWithEvents(ctx, l.Worktree, l.Verifier, logFile, index, ev)
	return outcome, err
}

// runVerifierWithEvents wraps RunVerifier to emit per-stage events as stages
// finish (RunVerifier itself stays a pure domain function).
func runVerifierWithEvents(ctx context.Context, dir string, stages []Stage, log io.Writer, index int, ev Events) (VerifierOutcome, error) {
	outcome, err := RunVerifier(ctx, dir, stages, log)
	if ev.StageDone != nil {
		for _, r := range outcome.Stages {
			ev.StageDone(index, r)
		}
	}
	return outcome, err
}

// checkForbidden matches changed paths against forbidden prefixes (a path
// equal to or under a forbidden entry violates).
func checkForbidden(forbidden, changed []string) string {
	var hits []string
	for _, f := range forbidden {
		prefix := strings.TrimSuffix(filepath.ToSlash(strings.TrimSpace(f)), "/")
		if prefix == "" {
			continue
		}
		for _, c := range changed {
			path := filepath.ToSlash(c)
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				hits = append(hits, c)
			}
		}
	}
	if len(hits) == 0 {
		return ""
	}
	return fmt.Sprintf("forbidden paths were modified: %s", strings.Join(hits, ", "))
}

// detectStuck applies the loop's escalation rules to the recorded history.
// The baseline (index 0) never counts: it measures the repo, not the agent.
func detectStuck(l Loop, iterations []Iteration) (string, bool) {
	var agentIters []Iteration
	for _, it := range iterations {
		if it.Index > 0 {
			agentIters = append(agentIters, it)
		}
	}
	if len(agentIters) == 0 {
		return "", false
	}

	// Same failure N times: the agent is going in circles.
	n := l.Stuck.SameFailureRepeats
	if len(agentIters) >= n {
		window := agentIters[len(agentIters)-n:]
		same := window[0].TailHash != ""
		for _, it := range window[1:] {
			if it.TailHash != window[0].TailHash {
				same = false
				break
			}
		}
		if same {
			return fmt.Sprintf("stuck: verifier stage %q failed identically for %d consecutive iterations", window[len(window)-1].FailingStage, n), true
		}
	}

	// No diff change for N iterations: the agent gave up or did nothing.
	m := l.Stuck.NoChangeRepeats
	if len(agentIters) >= m {
		window := agentIters[len(agentIters)-m:]
		allUnchanged := true
		for i, it := range window {
			var prevHash string
			idx := len(agentIters) - m + i
			if idx == 0 {
				prevHash = iterations[0].DiffHash // baseline
			} else {
				prevHash = agentIters[idx-1].DiffHash
			}
			if it.DiffHash != prevHash {
				allUnchanged = false
				break
			}
		}
		if allUnchanged {
			if m == 1 {
				return "stuck: the last iteration produced no change to the diff", true
			}
			return fmt.Sprintf("stuck: %d consecutive iterations produced no change to the diff", m), true
		}
	}
	return "", false
}

func parkLoop(root string, l Loop, reason string, ev Events) (Loop, error) {
	return endLoop(root, l, StatusParked, reason, ev)
}

func endLoop(root string, l Loop, status, reason string, ev Events) (Loop, error) {
	l.Status = status
	l.ParkedReason = reason
	l.EndedAt = utcNowISO()
	if err := SaveLoop(root, l); err != nil {
		return l, err
	}
	_ = ClearControl(root, l.ID)
	if ev.LoopEnded != nil {
		ev.LoopEnded(l)
	}
	return l, nil
}

// ParkAborted parks a loop that has no live engine (paused or crashed),
// recording the abort reason verbatim. It takes the engine lock to exclude a
// racing engine start.
func ParkAborted(root, loopID, reason string) error {
	if err := AcquireEngineLock(root, loopID); err != nil {
		return err
	}
	defer func() { _ = ReleaseEngineLock(root, loopID) }()
	l, err := LoadLoop(root, loopID)
	if err != nil {
		return err
	}
	if l.Done() {
		return fmt.Errorf("loop %s is already %s", l.ID, l.Status)
	}
	_, err = parkLoop(root, l, abortReason(Control{Abort: true, Reason: reason}), Events{})
	return err
}

func abortReason(c Control) string {
	if strings.TrimSpace(c.Reason) != "" {
		return "aborted: " + strings.TrimSpace(c.Reason)
	}
	return "aborted by user"
}

// watchAbort polls control.json while a phase runs and cancels the returned
// context on an abort request, which kills the running process group.
func watchAbort(root, loopID string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(controlPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ctrl, err := ReadControl(root, loopID); err == nil && ctrl.Abort {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, func() {
		close(done)
		cancel()
	}
}

// durationOrZero is the iteration's recorded wall time.
func (it Iteration) durationOrZero() time.Duration {
	if it.WallMS > 0 {
		return time.Duration(it.WallMS) * time.Millisecond
	}
	start, err1 := time.Parse(time.RFC3339, it.StartedAt)
	end, err2 := time.Parse(time.RFC3339, it.EndedAt)
	if err1 != nil || err2 != nil || end.Before(start) {
		return 0
	}
	return end.Sub(start)
}
