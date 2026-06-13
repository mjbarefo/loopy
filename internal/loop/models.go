// Package loop is loopy's domain layer: loop definitions, iteration records,
// the verifier runner, git/worktree plumbing, and the engine. Stdlib only —
// no TUI imports allowed here.
package loop

import (
	"encoding/json"
	"fmt"
	"time"
)

// Loop statuses. There is no draft state: loops are created running.
const (
	StatusRunning  = "running"
	StatusPaused   = "paused"
	StatusGreen    = "green"    // verifier passed; parked for human review
	StatusParked   = "parked"   // stopped without green: budget, stuck, abort
	StatusAccepted = "accepted" // human accepted at review
	StatusRejected = "rejected" // human rejected at review
)

// StageKind distinguishes how a verifier stage produces its verdict. The zero
// value ("") is a command stage, so existing loop.json files keep parsing.
//
// "judge" is deliberately NOT reused here: that word is the deterministic,
// no-API-key ranking of competing green loops (see judge.go). An ask stage is
// the opposite — it spends an agent call (and its keys) and is not
// reproducible — so it gets its own plain word.
type StageKind string

const (
	// KindCommand runs a shell command; exit zero is green. Fast, reproducible,
	// needs no API key — the only kind inference and the demo produce.
	KindCommand StageKind = "command"
	// KindAsk asks a registered agent a yes/no question about the worktree; a
	// PASS verdict is green. Use it only where a shell command can't express
	// "done". The human's accept/reject is the backstop for its fuzziness.
	KindAsk StageKind = "ask"
)

// Stage is one verifier stage. Stages run in order and short-circuit on the
// first failure, so order them fast-to-slow (and cheap-to-expensive: an ask
// stage only runs once the command gates ahead of it are green).
type Stage struct {
	Name string    `json:"name"`
	Kind StageKind `json:"kind,omitempty"` // "" == command
	Cmd  string    `json:"cmd,omitempty"`  // command stages: the shell command
	Ask  string    `json:"ask,omitempty"`  // ask stages: the yes/no question
	// Agent optionally overrides which registered agent answers an ask stage;
	// empty means the loop's own agent. Reserved for a future opt-in — the
	// wizard never sets it today.
	Agent string `json:"agent,omitempty"`
}

// kind returns the stage's effective kind, treating the zero value as a
// command stage.
func (s Stage) kind() StageKind {
	if s.Kind == "" {
		return KindCommand
	}
	return s.Kind
}

// Descriptor is the human-facing one-liner recorded in StageResult.Cmd and
// shown in the monitor: the shell command for a command stage, the question
// for an ask stage.
func (s Stage) Descriptor() string {
	if s.kind() == KindAsk {
		return s.Ask
	}
	return s.Cmd
}

// Budget holds the loop's hard caps. Exhaustion parks the loop; budgets are
// never advisory.
type Budget struct {
	MaxIterations int      `json:"max_iterations"`
	MaxWallClock  Duration `json:"max_wall_clock"`
}

// StuckPolicy controls early escalation: parking a loop that is burning
// budget without converging.
type StuckPolicy struct {
	// SameFailureRepeats parks the loop when the first failing stage and the
	// hash of its output tail are identical for this many consecutive
	// iterations.
	SameFailureRepeats int `json:"same_failure_repeats"`
	// NoChangeRepeats parks the loop when this many consecutive iterations
	// leave the cumulative diff unchanged.
	NoChangeRepeats int `json:"no_change_repeats"`
}

// DefaultBudget and DefaultStuckPolicy are applied when a loop or the project
// config doesn't say otherwise.
//
// MaxIterations 5: self-refinement returns fall off steeply after the first
// few feedback rounds, and stuck detection parks degenerate loops before any
// cap — but real loops in this repo have needed 4 productive iterations, so
// 3 would cut genuine work short. The cap is a ceiling on slow progress, not
// a target: green stops the loop the moment it lands.
var (
	DefaultBudget      = Budget{MaxIterations: 5, MaxWallClock: Duration(30 * time.Minute)}
	DefaultStuckPolicy = StuckPolicy{SameFailureRepeats: 3, NoChangeRepeats: 1}
)

// Loop is the central object: a goal, a verifier, a budget, and an agent,
// iterating in an isolated worktree.
type Loop struct {
	ID             string   `json:"id"`
	Goal           string   `json:"goal"`
	Constraints    []string `json:"constraints,omitempty"`
	ForbiddenPaths []string `json:"forbidden_paths,omitempty"`
	Agent          string   `json:"agent"`
	// Reviewer is an optional second registered agent that critiques the
	// green diff before the loop parks. The critique is evidence, never a
	// gate; it must be a different agent than the author.
	Reviewer string      `json:"reviewer,omitempty"`
	Verifier []Stage     `json:"verifier"`
	Budget   Budget      `json:"budget"`
	Stuck    StuckPolicy `json:"stuck"`

	Status         string   `json:"status"`
	ParkedReason   string   `json:"parked_reason,omitempty"`
	IterationsUsed int      `json:"iterations_used"`
	WallClockUsed  Duration `json:"wall_clock_used"`
	// ReviewerExit records the reviewer agent's exit code, when one ran.
	ReviewerExit *int `json:"reviewer_exit,omitempty"`

	BaseCommit string `json:"base_commit"`
	Branch     string `json:"branch"`
	Worktree   string `json:"worktree"`

	CreatedAt string `json:"created_at"`
	EndedAt   string `json:"ended_at,omitempty"`
}

// Done reports whether the loop has reached a terminal state for the engine.
func (l Loop) Done() bool {
	switch l.Status {
	case StatusGreen, StatusParked, StatusAccepted, StatusRejected:
		return true
	}
	return false
}

// StageResult records one verifier stage's outcome inside an iteration.
type StageResult struct {
	Name string    `json:"name"`
	Kind StageKind `json:"kind,omitempty"` // "" == command
	// Cmd is the stage descriptor: the shell command for a command stage, the
	// question for an ask stage.
	Cmd        string `json:"cmd"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
}

// Iteration is one fully recorded turn of the crank. Index 0 is the baseline:
// a verify-only pass before the first agent run that seeds the first prompt's
// feedback (and parks the loop green immediately if the repo already passes).
type Iteration struct {
	Index     int    `json:"index"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at"`
	// WallMS is the iteration's precise wall time; the RFC3339 timestamps
	// above are second-resolution and for humans.
	WallMS int64 `json:"wall_ms"`

	// AgentExit is nil for the baseline iteration (no agent ran).
	AgentExit *int  `json:"agent_exit,omitempty"`
	AgentMS   int64 `json:"agent_ms,omitempty"`

	Stages       []StageResult `json:"stages"`
	Green        bool          `json:"green"`
	FailingStage string        `json:"failing_stage,omitempty"`
	// FeedbackTail is the bounded output tail of the first failing stage (or
	// the forbidden-path violation message) — exactly what the next prompt
	// will carry.
	FeedbackTail string `json:"feedback_tail,omitempty"`
	TailHash     string `json:"tail_hash,omitempty"`

	DiffHash     string   `json:"diff_hash"`
	DiffBytes    int      `json:"diff_bytes"`
	ChangedFiles []string `json:"changed_files,omitempty"`
	// Violation is set when the iteration touched a forbidden path; the
	// verifier is skipped and the violation is fed back instead.
	Violation string `json:"violation,omitempty"`
}

// Config is the project-level default store at .loopy/config.json.
type Config struct {
	DefaultAgent    string  `json:"default_agent,omitempty"`
	DefaultVerifier []Stage `json:"default_verifier,omitempty"`
	DefaultBudget   *Budget `json:"default_budget,omitempty"`
}

// Agent is a registered external command template. Loopy never calls a model
// itself; agents are how work happens.
type Agent struct {
	Cmd string `json:"cmd"`
}

// AgentRegistry is the .loopy/agents.json document.
type AgentRegistry struct {
	Default string           `json:"default,omitempty"`
	Agents  map[string]Agent `json:"agents"`
}

// Engine phases, recorded in .loopy/loops/<id>/phase.json while a phase runs.
const (
	PhaseAgent  = "agent"
	PhaseVerify = "verify"
	PhaseReview = "review"
)

// Phase is the engine's live activity record: which iteration it is on, what
// it is doing, and since when. Ephemeral — cleared at iteration boundaries
// and engine exit; only meaningful while an engine holds the loop's lock.
type Phase struct {
	Iteration int    `json:"iteration"`
	Phase     string `json:"phase"`
	StartedAt string `json:"started_at"`
}

// Control is the monitor→engine channel at .loopy/loops/<id>/control.json.
// The engine polls it between phases; abort is additionally watched during
// agent and verifier runs.
type Control struct {
	Pause  bool   `json:"pause,omitempty"`
	Abort  bool   `json:"abort,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Duration marshals as a human-readable string ("30m", "1h30m") so state
// files stay inspectable without loopy.
type Duration time.Duration

func (d Duration) String() string { return time.Duration(d).String() }

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("duration must be a string like \"30m\": %w", err)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

func utcNowISO() string { return time.Now().UTC().Format(time.RFC3339) }
