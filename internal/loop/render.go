package loop

import (
	"fmt"
	"strings"
	"time"
)

// Plain renderers. ANSI-free by design: color is the CLI layer's concern and
// is never the only signal — verdicts always carry the ok/FAIL words and
// the ✓/✗ glyphs.

// RenderLoopLine is the one-line list form: id, status, iterations, agent,
// goal.
func RenderLoopLine(v LoopView) string {
	verdict := v.Status
	goal := v.Goal
	if len(goal) > 60 {
		goal = goal[:57] + "..."
	}
	return fmt.Sprintf("%-28s %-8s iter %d/%d  %-10s %s", v.ID, verdict, v.IterationsUsed, v.MaxIterations, v.Agent, goal)
}

// RenderStatus is the full single-loop view: header, budget, iteration
// timeline, last feedback, and the next command.
func RenderStatus(v LoopView) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s · %s · iter %d/%d · %s/%s wall\n", v.ID, v.Status, v.IterationsUsed, v.MaxIterations, v.WallClockUsed, v.MaxWallClock)
	fmt.Fprintf(&b, "goal: %s\n", v.Goal)
	fmt.Fprintf(&b, "agent: %s\n", v.Agent)
	if v.ParkedReason != "" {
		fmt.Fprintf(&b, "note: %s\n", v.ParkedReason)
	}
	b.WriteString("verifier:")
	for _, s := range v.Verifier {
		fmt.Fprintf(&b, " %s", s.Name)
	}
	b.WriteString("\n")

	if len(v.Iterations) > 0 {
		b.WriteString("\n  iter      verdict            agent      verify     diff\n")
		for _, it := range v.Iterations {
			b.WriteString("  " + renderIterationRow(it) + "\n")
		}
	}
	if v.LastFeedback != "" && v.Status != StatusGreen {
		b.WriteString("\nlast feedback tail:\n")
		for _, line := range tailLines(v.LastFeedback, 8) {
			fmt.Fprintf(&b, "  | %s\n", line)
		}
	}
	if v.NextCommand != "" {
		fmt.Fprintf(&b, "\nnext: %s\n", v.NextCommand)
	}
	return b.String()
}

func renderIterationRow(it IterationView) string {
	label := fmt.Sprintf("%d", it.Index)
	if it.Baseline {
		label = "base"
	}
	verdict := "✓ green"
	switch {
	case it.Violation:
		verdict = "✗ forbidden"
	case !it.Green && it.FailingStage != "":
		verdict = fmt.Sprintf("✗ %s", it.FailingStage)
	case !it.Green:
		verdict = "✗ red"
	}
	agent := "-"
	if it.AgentExit != nil {
		agent = fmt.Sprintf("exit %d", *it.AgentExit)
	}
	return fmt.Sprintf("%-9s %-18s %-10s %-10s %s",
		label, verdict, agent,
		(time.Duration(it.VerifyMS) * time.Millisecond).Round(time.Millisecond).String(),
		renderDiffCell(it))
}

func renderDiffCell(it IterationView) string {
	if it.DiffBytes == 0 {
		return "none"
	}
	return fmt.Sprintf("%d file(s), %s", it.FilesChanged, humanBytes(it.DiffBytes))
}

// RenderIterationDetail is `loopy log <id> --iter N`: the iteration's record
// plus pointers to its raw artifacts.
func RenderIterationDetail(root, loopID string, it Iteration) string {
	var b strings.Builder
	label := fmt.Sprintf("iteration %d", it.Index)
	if it.Index == 0 {
		label = "baseline (verify only, no agent)"
	}
	fmt.Fprintf(&b, "%s · %s → %s\n", label, it.StartedAt, it.EndedAt)
	if it.AgentExit != nil {
		fmt.Fprintf(&b, "agent: exit %d in %s\n", *it.AgentExit, (time.Duration(it.AgentMS) * time.Millisecond).Round(time.Millisecond))
	}
	if it.Violation != "" {
		fmt.Fprintf(&b, "violation: %s\n", it.Violation)
	}
	for _, s := range it.Stages {
		verdict := "ok"
		if s.ExitCode != 0 {
			verdict = fmt.Sprintf("FAIL exit %d", s.ExitCode)
		}
		fmt.Fprintf(&b, "stage %-10s %-12s %s\n", s.Name, verdict, (time.Duration(s.DurationMS) * time.Millisecond).Round(time.Millisecond))
	}
	if it.FeedbackTail != "" && !it.Green {
		b.WriteString("feedback tail:\n")
		for _, line := range tailLines(it.FeedbackTail, 20) {
			fmt.Fprintf(&b, "  | %s\n", line)
		}
	}
	if len(it.ChangedFiles) > 0 {
		fmt.Fprintf(&b, "changed files (%d):\n", len(it.ChangedFiles))
		max := len(it.ChangedFiles)
		if max > 20 {
			max = 20
		}
		for _, f := range it.ChangedFiles[:max] {
			fmt.Fprintf(&b, "  %s\n", f)
		}
		if len(it.ChangedFiles) > max {
			fmt.Fprintf(&b, "  … and %d more\n", len(it.ChangedFiles)-max)
		}
	}
	dir := IterationDir(root, loopID, it.Index)
	fmt.Fprintf(&b, "artifacts: %s\n", dir)
	return b.String()
}

func tailLines(s string, n int) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
