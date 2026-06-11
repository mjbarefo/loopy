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
	return fmt.Sprintf("%-28s %-8s iter %d/%d  %-10s %s",
		v.ID, v.Status, v.IterationsUsed, v.MaxIterations, v.Agent, TruncateDisplay(v.Goal, 60))
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
		b.WriteString("\n  " + IterationRowHeader + "\n")
		for _, it := range v.Iterations {
			b.WriteString("  " + RenderIterationRow(it) + "\n")
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

// IterationRowHeader pairs with RenderIterationRow; both renderers print it
// above the timeline so the columns always carry their names.
const IterationRowHeader = "iter  result              agent      verify     diff"

func RenderIterationRow(it IterationView) string {
	label := fmt.Sprintf("%d", it.Index)
	if it.Baseline {
		label = "base"
	}
	verdict := "✓ green"
	switch {
	case it.Violation:
		verdict = "✗ forbidden"
	case !it.Green && it.FailingStage != "":
		verdict = "✗ " + it.FailingStage
		// Stage progression is the convergence signal: how far through the
		// verifier this iteration got. Noise for single-stage verifiers.
		if it.StagesTotal > 1 {
			verdict += fmt.Sprintf(" (%d/%d)", it.StagesPassed, it.StagesTotal)
		}
	case !it.Green:
		verdict = "✗ red"
	}
	agent := "—"
	if it.AgentExit != nil {
		agent = humanDuration(time.Duration(it.AgentMS) * time.Millisecond)
		if *it.AgentExit != 0 {
			agent += fmt.Sprintf("·exit %d", *it.AgentExit)
		}
	}
	return fmt.Sprintf("%-5s %s %s %s %s",
		label,
		PadDisplay(verdict, 18),
		PadDisplay(agent, 10),
		PadDisplay(humanDuration(time.Duration(it.VerifyMS)*time.Millisecond), 10),
		renderDiffCell(it))
}

func renderDiffCell(it IterationView) string {
	if it.DiffBytes == 0 {
		return "—"
	}
	files := "file"
	if it.FilesChanged != 1 {
		files = "files"
	}
	return fmt.Sprintf("%d %s · %s", it.FilesChanged, files, HumanBytes(it.DiffBytes))
}

// humanDuration renders a duration at human resolution: milliseconds under a
// second, otherwise seconds with empty trailing units trimmed ("30m0s" reads
// "30m", "1h0m0s" reads "1h"; "30s" and "1m5s" stay as they are).
func humanDuration(d time.Duration) string {
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	s := d.Round(time.Second).String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// HumanDuration is humanDuration for other packages and the view-model.
func HumanDuration(d time.Duration) string { return humanDuration(d) }

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

// RenderReview is `loopy review <id>`: the terminal human moment — final
// diff, verifier transcript, iteration history, and the exact commands that
// record a decision. transcript is the (already capped) verifier log of the
// last verified iteration; diff is the loop's cumulative patch.
func RenderReview(v LoopView, review *Review, transcript string, transcriptIter int, diff []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "review: %s · %s · %d iteration(s) · %s\n", v.ID, v.Status, v.IterationsUsed, v.WallClockUsed)
	fmt.Fprintf(&b, "goal: %s\n", v.Goal)
	fmt.Fprintf(&b, "agent: %s\n", v.Agent)
	if v.ParkedReason != "" {
		fmt.Fprintf(&b, "note: %s\n", v.ParkedReason)
	}

	if len(v.Iterations) > 0 {
		b.WriteString("\n  " + IterationRowHeader + "\n")
		for _, it := range v.Iterations {
			b.WriteString("  " + RenderIterationRow(it) + "\n")
		}
	}

	if transcript != "" {
		fmt.Fprintf(&b, "\nverifier transcript (iteration %d):\n", transcriptIter)
		for _, line := range tailLines(transcript, 40) {
			fmt.Fprintf(&b, "  | %s\n", line)
		}
	}

	if len(diff) > 0 {
		fmt.Fprintf(&b, "\ndiff (%s):\n%s", HumanBytes(len(diff)), diff)
		if diff[len(diff)-1] != '\n' {
			b.WriteString("\n")
		}
	} else {
		b.WriteString("\ndiff: none — the verifier was already green at baseline\n")
	}

	switch {
	case review != nil:
		fmt.Fprintf(&b, "\ndecision: %s at %s", review.Decision, review.DecidedAt)
		if review.Override {
			fmt.Fprintf(&b, " (override: %s)", review.Reason)
		} else if review.Reason != "" {
			fmt.Fprintf(&b, " (%s)", review.Reason)
		}
		b.WriteString("\n")
		if review.FinalDiff != "" {
			fmt.Fprintf(&b, "apply: git apply %s\n", review.FinalDiff)
		}
	case v.Status == StatusGreen:
		fmt.Fprintf(&b, "\naccept: loopy accept %s\nreject: loopy reject %s [--reason text]\n", v.ID, v.ID)
	case v.Status == StatusParked:
		fmt.Fprintf(&b, "\nthis loop is not green — accepting requires the audited override:\n  loopy accept %s --override --reason \"<why>\"\nreject: loopy reject %s [--reason text]\n", v.ID, v.ID)
	}
	return b.String()
}

// RenderVerdict is the judge's plain output: the verdict line, the ranked
// evidence table, and any overlap warnings.
func RenderVerdict(v Verdict) string {
	var b strings.Builder
	fmt.Fprintf(&b, "verdict: %s\n", v.Reason)
	b.WriteString("\n  rank  loop                              verdict   iters  diff                 flags\n")
	for i, c := range v.Candidates {
		verdict := "✗ " + c.Status
		if c.Green {
			verdict = "✓ green"
		}
		diff := "none"
		if c.DiffBytes > 0 {
			diff = fmt.Sprintf("%d file(s), %s", c.FilesChanged, HumanBytes(c.DiffBytes))
		}
		flags := strings.Join(c.Notes, "; ")
		fmt.Fprintf(&b, "  %-5d %-33s %-9s %-6d %-20s %s\n", i+1, c.LoopID, verdict, c.Iterations, diff, flags)
	}
	if len(v.Overlaps) > 0 {
		b.WriteString("\noverlapping files (apply at most one of each pair):\n")
		for _, o := range v.Overlaps {
			fmt.Fprintf(&b, "  %s ∩ %s: %s\n", o.A, o.B, strings.Join(o.Files, ", "))
		}
	}
	if v.Winner != "" {
		fmt.Fprintf(&b, "\nnext: loopy review %s\n", v.Winner)
	} else if len(v.Candidates) > 0 {
		fmt.Fprintf(&b, "\nreview the candidates yourself: loopy review %s …\n", v.Candidates[0].LoopID)
	}
	return b.String()
}

// RenderLogbookEntry is one decision in `loopy logbook` plain output.
func RenderLogbookEntry(r Review) string {
	date := r.DecidedAt
	if len(date) >= 10 {
		date = date[:10]
	}
	line := fmt.Sprintf("%-12s %s · %s · %d iteration(s) · %s", date, r.LoopID, r.Decision, r.Iterations, r.WallClock)
	if r.Override {
		line += " · OVERRIDE: " + r.Reason
	} else if r.Reason != "" {
		line += " · " + r.Reason
	}
	return line
}

func tailLines(s string, n int) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func HumanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
