package loop

import (
	"fmt"
	"path/filepath"
	"time"
)

// IterationView is one iteration row for rendering.
type IterationView struct {
	Index        int    `json:"index"`
	Baseline     bool   `json:"baseline"`
	Green        bool   `json:"green"`
	FailingStage string `json:"failing_stage,omitempty"`
	Violation    bool   `json:"violation,omitempty"`
	AgentExit    *int   `json:"agent_exit,omitempty"`
	AgentMS      int64  `json:"agent_ms,omitempty"`
	VerifyMS     int64  `json:"verify_ms,omitempty"`
	DiffBytes    int    `json:"diff_bytes"`
	FilesChanged int    `json:"files_changed"`
}

// LoopView is the shared view-model: everything the plain renderer (and the
// M2 monitor) needs, with no rendering logic attached.
type LoopView struct {
	ID             string          `json:"id"`
	Goal           string          `json:"goal"`
	Agent          string          `json:"agent"`
	Status         string          `json:"status"`
	ParkedReason   string          `json:"parked_reason,omitempty"`
	IterationsUsed int             `json:"iterations_used"`
	MaxIterations  int             `json:"max_iterations"`
	WallClockUsed  string          `json:"wall_clock_used"`
	MaxWallClock   string          `json:"max_wall_clock"`
	Verifier       []Stage         `json:"verifier"`
	Iterations     []IterationView `json:"iterations,omitempty"`
	LastFeedback   string          `json:"last_feedback,omitempty"`
	FinalDiffPath  string          `json:"final_diff_path,omitempty"`
	Worktree       string          `json:"worktree,omitempty"`
	NextCommand    string          `json:"next_command,omitempty"`
	CreatedAt      string          `json:"created_at"`
	EndedAt        string          `json:"ended_at,omitempty"`
}

// BuildLoopView assembles the view-model for one loop from disk.
func BuildLoopView(root string, l Loop) (LoopView, error) {
	iterations, err := LoadIterations(root, l.ID)
	if err != nil {
		return LoopView{}, err
	}
	view := LoopView{
		ID:             l.ID,
		Goal:           l.Goal,
		Agent:          l.Agent,
		Status:         l.Status,
		ParkedReason:   l.ParkedReason,
		IterationsUsed: l.IterationsUsed,
		MaxIterations:  l.Budget.MaxIterations,
		WallClockUsed:  time.Duration(l.WallClockUsed).Round(time.Second).String(),
		MaxWallClock:   time.Duration(l.Budget.MaxWallClock).String(),
		Verifier:       l.Verifier,
		Worktree:       l.Worktree,
		CreatedAt:      l.CreatedAt,
		EndedAt:        l.EndedAt,
	}
	for _, it := range iterations {
		var verifyMS int64
		for _, s := range it.Stages {
			verifyMS += s.DurationMS
		}
		view.Iterations = append(view.Iterations, IterationView{
			Index:        it.Index,
			Baseline:     it.Index == 0,
			Green:        it.Green,
			FailingStage: it.FailingStage,
			Violation:    it.Violation != "",
			AgentExit:    it.AgentExit,
			AgentMS:      it.AgentMS,
			VerifyMS:     verifyMS,
			DiffBytes:    it.DiffBytes,
			FilesChanged: len(it.ChangedFiles),
		})
	}
	if len(iterations) > 0 {
		last := iterations[len(iterations)-1]
		view.LastFeedback = last.FeedbackTail
		if last.DiffBytes > 0 {
			view.FinalDiffPath = filepath.Join(IterationDir(root, l.ID, last.Index), DiffFile)
		}
	}
	view.NextCommand = nextCommand(l, view.FinalDiffPath)
	return view, nil
}

// nextCommand is the footer hint: the one command that moves this loop
// forward.
func nextCommand(l Loop, finalDiff string) string {
	switch l.Status {
	case StatusRunning:
		return fmt.Sprintf("loopy status %s", l.ID)
	case StatusPaused:
		return fmt.Sprintf("loopy resume %s", l.ID)
	case StatusGreen:
		if finalDiff != "" {
			// `loopy review` arrives at M3; until then the diff is the review.
			return fmt.Sprintf("review the diff: less %s", finalDiff)
		}
		return fmt.Sprintf("loopy log %s", l.ID)
	case StatusParked:
		return fmt.Sprintf("loopy log %s", l.ID)
	default:
		return ""
	}
}
