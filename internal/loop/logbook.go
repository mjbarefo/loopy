package loop

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The logbook is the project's durable memory of human decisions:
// .loopy/logbook.md, one appended markdown entry per accept/reject, written
// for humans; review.json stays the structured record. LogbookEntries
// aggregates those records so `loopy logbook --json` needs no second store.

// LogbookFile is the logbook's name inside .loopy/.
const LogbookFile = "logbook.md"

// LogbookPath returns .loopy/logbook.md.
func LogbookPath(root string) string {
	return filepath.Join(root, LoopyDir, LogbookFile)
}

// appendLogbook appends one human-readable markdown entry for a decision.
// Override reasons are recorded verbatim.
func appendLogbook(root string, l Loop, r Review) error {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s — %s — %s\n\n", r.LoopID, r.Decision, r.DecidedAt)
	fmt.Fprintf(&b, "- goal: %s\n", r.Goal)
	fmt.Fprintf(&b, "- agent: %s (%d iterations, %s)\n", r.Agent, r.Iterations, r.WallClock)
	fmt.Fprintf(&b, "- green at decision: %v\n", r.GreenAtDecision)
	if r.Override {
		b.WriteString("- **override** (accepted while not green)\n")
	}
	if r.Reason != "" {
		fmt.Fprintf(&b, "- reason: %s\n", r.Reason)
	}
	b.WriteString("\n")

	f, err := os.OpenFile(LogbookPath(root), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// LogbookEntries returns every recorded decision across loops, ordered by
// decision time (oldest first), ties broken by loop ID.
func LogbookEntries(root string) ([]Review, error) {
	loops, err := ListLoops(root)
	if err != nil {
		return nil, err
	}
	var entries []Review
	for _, l := range loops {
		r, ok, err := LoadReview(root, l.ID)
		if err != nil {
			return nil, err
		}
		if ok {
			entries = append(entries, r)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].DecidedAt != entries[j].DecidedAt {
			return entries[i].DecidedAt < entries[j].DecidedAt
		}
		return entries[i].LoopID < entries[j].LoopID
	})
	return entries, nil
}
