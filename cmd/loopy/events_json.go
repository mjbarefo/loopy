package main

import (
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/mjbarefo/loopy/internal/loop"
)

// runEvent is one NDJSON line on stdout when `loopy run --json` (or
// `resume --json`) streams progress: the machine face of the plain progress
// lines, for outer loops that drive loopy. The exit code stays the verdict
// (0 only green); this stream is observability. Fields are omitted when they
// don't apply to the event.
type runEvent struct {
	Event         string            `json:"event"`
	TS            string            `json:"ts"`
	LoopID        string            `json:"loop_id,omitempty"`
	Agent         string            `json:"agent,omitempty"`
	Branch        string            `json:"branch,omitempty"`
	Worktree      string            `json:"worktree,omitempty"`
	MaxIterations int               `json:"max_iterations,omitempty"`
	MaxWallClock  string            `json:"max_wall_clock,omitempty"`
	Iteration     *int              `json:"iteration,omitempty"`
	ExitCode      *int              `json:"exit_code,omitempty"`
	DurationMS    *int64            `json:"duration_ms,omitempty"`
	Stage         *loop.StageResult `json:"stage,omitempty"`
	Green         *bool             `json:"green,omitempty"`
	FailingStage  string            `json:"failing_stage,omitempty"`
	Violation     string            `json:"violation,omitempty"`
	DiffBytes     *int              `json:"diff_bytes,omitempty"`
	Status        string            `json:"status,omitempty"`
	ParkedReason  string            `json:"parked_reason,omitempty"`
	Note          string            `json:"note,omitempty"`
	Result        *loop.LoopView    `json:"result,omitempty"`
	RaceID        string            `json:"race_id,omitempty"`
	Verdict       *loop.Verdict     `json:"verdict,omitempty"`
}

// jsonEmitter serializes events onto one writer. The mutex keeps lines
// atomic when a race streams several loops' events concurrently.
type jsonEmitter struct {
	mu  sync.Mutex
	enc *json.Encoder
	now func() time.Time
}

func newJSONEmitter(w io.Writer) *jsonEmitter {
	return &jsonEmitter{enc: json.NewEncoder(w), now: time.Now}
}

func (e *jsonEmitter) emit(ev runEvent) {
	ev.TS = e.now().UTC().Format(time.RFC3339)
	e.mu.Lock()
	defer e.mu.Unlock()
	// A write error here means stdout is gone; the engine's own state is on
	// disk either way, same as the plain stream.
	_ = e.enc.Encode(ev)
}

// events maps one loop's engine callbacks onto the emitter.
func (e *jsonEmitter) events(loopID string) loop.Events {
	return loop.Events{
		LoopStarted: func(l loop.Loop) {
			e.emit(runEvent{
				Event:         "loop_started",
				LoopID:        l.ID,
				Agent:         l.Agent,
				Branch:        l.Branch,
				Worktree:      l.Worktree,
				MaxIterations: l.Budget.MaxIterations,
				MaxWallClock:  time.Duration(l.Budget.MaxWallClock).String(),
			})
		},
		BaselineStarted: func() {
			e.emit(runEvent{Event: "baseline_started", LoopID: loopID})
		},
		IterationStarted: func(index, max int) {
			e.emit(runEvent{Event: "iteration_started", LoopID: loopID, Iteration: &index, MaxIterations: max})
		},
		AgentDone: func(index, exitCode int, d time.Duration) {
			ms := d.Milliseconds()
			e.emit(runEvent{Event: "agent_done", LoopID: loopID, Iteration: &index, ExitCode: &exitCode, DurationMS: &ms})
		},
		StageDone: func(index int, r loop.StageResult) {
			e.emit(runEvent{Event: "stage_done", LoopID: loopID, Iteration: &index, Stage: &r})
		},
		IterationDone: func(it loop.Iteration, l loop.Loop) {
			idx, green, diff := it.Index, it.Green, it.DiffBytes
			e.emit(runEvent{
				Event:        "iteration_done",
				LoopID:       l.ID,
				Iteration:    &idx,
				Green:        &green,
				FailingStage: it.FailingStage,
				Violation:    it.Violation,
				DiffBytes:    &diff,
			})
		},
		ReviewerDone: func(exitCode int, d time.Duration) {
			ms := d.Milliseconds()
			e.emit(runEvent{Event: "reviewer_done", LoopID: loopID, ExitCode: &exitCode, DurationMS: &ms})
		},
		Note: func(s string) {
			e.emit(runEvent{Event: "note", LoopID: loopID, Note: s})
		},
		LoopEnded: func(l loop.Loop) {
			e.emit(runEvent{Event: "loop_ended", LoopID: l.ID, Status: l.Status, ParkedReason: l.ParkedReason})
		},
	}
}

// result emits the terminal event: the loop's full view-model — the same
// object `loopy status --json` serves, so consumers learn one shape.
func (e *jsonEmitter) result(root string, l loop.Loop) {
	view, err := loop.BuildLoopView(root, l)
	if err != nil {
		e.emit(runEvent{Event: "result", LoopID: l.ID, Status: l.Status, ParkedReason: l.ParkedReason, Note: "view unavailable: " + err.Error()})
		return
	}
	e.emit(runEvent{Event: "result", LoopID: l.ID, Status: l.Status, Result: &view})
}
