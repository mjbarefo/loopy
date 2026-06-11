package loop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Review is the audited record of the terminal human moment, stored at
// .loopy/loops/<id>/review.json. Overrides are recorded verbatim — same
// discipline as crux's seal.
type Review struct {
	LoopID   string `json:"loop_id"`
	Decision string `json:"decision"` // accepted | rejected
	// GreenAtDecision is the loop's verifier verdict when the human decided;
	// accepting without it requires (and records) an override.
	GreenAtDecision bool   `json:"green_at_decision"`
	Override        bool   `json:"override,omitempty"`
	Reason          string `json:"reason,omitempty"`
	DecidedAt       string `json:"decided_at"`

	Goal       string `json:"goal"`
	Agent      string `json:"agent"`
	Iterations int    `json:"iterations"`
	WallClock  string `json:"wall_clock"`

	// FinalDiff is the durable patch path (accept only); it survives
	// worktree removal.
	FinalDiff    string   `json:"final_diff,omitempty"`
	DiffBytes    int      `json:"diff_bytes,omitempty"`
	ChangedFiles []string `json:"changed_files,omitempty"`
}

const (
	DecisionAccepted = "accepted"
	DecisionRejected = "rejected"

	// FinalDiffFile is the durable copy of the loop's cumulative diff,
	// written into the loop directory at accept.
	FinalDiffFile = "final-diff.patch"
	reviewFile    = "review.json"
)

// ReviewPath returns where a loop's decision record lives.
func ReviewPath(root, loopID string) string {
	return filepath.Join(LoopDir(root, loopID), reviewFile)
}

// LoadReview reads a loop's decision record; ok is false when no decision
// has been made.
func LoadReview(root, loopID string) (Review, bool, error) {
	var r Review
	err := ReadJSON(ReviewPath(root, loopID), &r)
	if errors.Is(err, os.ErrNotExist) {
		return Review{}, false, nil
	}
	if err != nil {
		return Review{}, false, err
	}
	return r, true, nil
}

// Accept records the human accepting a loop's result. The loop must be
// green; accepting a parked (non-green) loop requires override with a
// non-empty reason, recorded verbatim. Accept writes final-diff.patch and
// review.json, marks the loop accepted, and appends the logbook. The
// worktree is kept (the design frees it only on reject).
func Accept(root, loopID string, override bool, reason string) (Review, error) {
	if err := AcquireEngineLock(root, loopID); err != nil {
		return Review{}, err
	}
	defer func() { _ = ReleaseEngineLock(root, loopID) }()

	l, err := reviewableLoop(root, loopID)
	if err != nil {
		return Review{}, err
	}
	green := l.Status == StatusGreen
	if !green {
		if !override {
			return Review{}, fmt.Errorf("loop %s is %s, not green: accepting requires --override --reason", l.ID, l.Status)
		}
		if strings.TrimSpace(reason) == "" {
			return Review{}, errors.New("--override requires --reason: the override is recorded verbatim")
		}
	}

	r := newReview(l, DecisionAccepted, green, override && !green, reason)

	it, ok, err := lastChangedIteration(root, l)
	if err != nil {
		return Review{}, err
	}
	if ok {
		diff, err := os.ReadFile(filepath.Join(IterationDir(root, l.ID, it.Index), DiffFile))
		if err != nil {
			return Review{}, err
		}
		finalPath := filepath.Join(LoopDir(root, l.ID), FinalDiffFile)
		if err := writeFileAtomic(finalPath, diff, 0o644); err != nil {
			return Review{}, err
		}
		r.FinalDiff = finalPath
		r.DiffBytes = it.DiffBytes
		r.ChangedFiles = it.ChangedFiles
	}

	return r, recordDecision(root, l, r)
}

// Reject records the human declining a loop's result: review.json and the
// logbook are written, every piece of iteration evidence is preserved, and
// the worktree (plus its loopy/<id> branch) is freed.
func Reject(root, loopID, reason string) (Review, error) {
	if err := AcquireEngineLock(root, loopID); err != nil {
		return Review{}, err
	}
	defer func() { _ = ReleaseEngineLock(root, loopID) }()

	l, err := reviewableLoop(root, loopID)
	if err != nil {
		return Review{}, err
	}
	r := newReview(l, DecisionRejected, l.Status == StatusGreen, false, reason)
	if err := recordDecision(root, l, r); err != nil {
		return Review{}, err
	}
	if err := RemoveLoopWorktree(root, l.ID); err != nil {
		return r, fmt.Errorf("decision recorded, but freeing the worktree failed (see `loopy doctor`): %w", err)
	}
	return r, nil
}

// reviewableLoop loads a loop and refuses decisions on loops that are still
// moving or already decided.
func reviewableLoop(root, loopID string) (Loop, error) {
	l, err := LoadLoop(root, loopID)
	if err != nil {
		return Loop{}, err
	}
	switch l.Status {
	case StatusGreen, StatusParked:
		return l, nil
	case StatusAccepted, StatusRejected:
		return Loop{}, fmt.Errorf("loop %s was already %s (see %s)", l.ID, l.Status, ReviewPath(root, l.ID))
	case StatusPaused:
		return Loop{}, fmt.Errorf("loop %s is paused: `loopy resume %s` to finish it or `loopy abort %s` to park it first", l.ID, l.ID, l.ID)
	default:
		return Loop{}, fmt.Errorf("loop %s is still running: wait for it to park or `loopy abort %s`", l.ID, l.ID)
	}
}

func newReview(l Loop, decision string, green, override bool, reason string) Review {
	return Review{
		LoopID:          l.ID,
		Decision:        decision,
		GreenAtDecision: green,
		Override:        override,
		Reason:          reason,
		DecidedAt:       utcNowISO(),
		Goal:            l.Goal,
		Agent:           l.Agent,
		Iterations:      l.IterationsUsed,
		WallClock:       time.Duration(l.WallClockUsed).Round(time.Second).String(),
	}
}

// recordDecision flushes the review, the loop status, and the logbook entry.
func recordDecision(root string, l Loop, r Review) error {
	if err := WriteJSON(ReviewPath(root, l.ID), r); err != nil {
		return err
	}
	l.Status = r.Decision
	if l.EndedAt == "" {
		l.EndedAt = r.DecidedAt
	}
	if err := SaveLoop(root, l); err != nil {
		return err
	}
	return appendLogbook(root, l, r)
}

// lastChangedIteration finds the newest iteration that actually changed the
// worktree (a green-at-baseline loop has none).
func lastChangedIteration(root string, l Loop) (Iteration, bool, error) {
	iterations, err := LoadIterations(root, l.ID)
	if err != nil {
		return Iteration{}, false, err
	}
	for i := len(iterations) - 1; i >= 0; i-- {
		if iterations[i].DiffBytes > 0 {
			return iterations[i], true, nil
		}
	}
	return Iteration{}, false, nil
}
