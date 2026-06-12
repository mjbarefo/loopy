package tui

// The fleet view: while browsing (rail focus), the detail area shows every
// loop as a compact live strip — status, convergence, and a short live tail —
// so the whole fleet breathes at once. Enter opens the selected loop's full
// detail; esc returns. herdr shows you terminals; loopy shows you
// convergence.

import (
	"fmt"
	"strings"

	"github.com/mjbarefo/loopy/internal/loop"
)

// fleetActive reports whether browsing renders the fleet: two or more loops
// (a single loop's full detail is strictly better than a strip), never in a
// --once frame (its single-loop byte contract is for scripts).
func (s frameState) fleetActive() bool {
	return !s.focusDetail && !s.once && len(s.loops) >= 2
}

// fleetLines renders the strips, windowed so the selected strip stays
// visible. Strips follow rail order (urgency first); a blank row separates
// strips and a second one separates the urgency groups — the gap is the
// label, same as the rail. Decided loops compress to one quiet line.
func fleetLines(s frameState, width, rows int) []cell {
	railW, _ := s.railArea()
	cursor := railW == 0 // no rail: the strips carry the selection cursor

	var lines []cell
	selStart, selEnd := 0, 0
	prevGroup := -1
	for i, v := range s.loops {
		if prevGroup >= 0 {
			lines = append(lines, cell{})
			if railGroup(v) != prevGroup {
				lines = append(lines, cell{})
			}
		}
		prevGroup = railGroup(v)
		strip := stripLines(s, v, i == s.selected, cursor, width)
		if i == s.selected {
			selStart = len(lines)
			selEnd = selStart + len(strip)
		}
		lines = append(lines, strip...)
	}
	for _, b := range s.broken {
		lines = append(lines, cell{}, joinCells(
			stripIndent(s, false, cursor),
			styled(s.color, sgrRed, "✗"),
			plainCell(" "+loop.TruncateDisplay(b.ID, width-16)),
			styled(s.color, sgrDim, " (unreadable)"),
		))
	}

	if len(lines) <= rows {
		return lines
	}
	start := 0
	if selEnd > rows {
		start = selEnd - rows
	}
	if selStart < start {
		start = selStart
	}
	if start+rows > len(lines) {
		start = len(lines) - rows
	}
	return lines[start : start+rows]
}

// stripIndent is the two-column cursor cell ("▶ " on the selected strip)
// when the rail is collapsed; with a rail the cursor lives there and strips
// stay flush.
func stripIndent(s frameState, selected, cursor bool) cell {
	if !cursor {
		return cell{}
	}
	if selected {
		return styled(s.color, sgrCyan, "▶ ")
	}
	return plainCell("  ")
}

// stripLines is one loop's strip: a header line (glyph, id, what it is
// doing), a convergence line (verdict run + verifier meter), and a short
// live tail. Decided loops get the header only — history stays quiet.
func stripLines(s frameState, v loop.LoopView, selected, cursor bool, width int) []cell {
	glyph, sgr := statusGlyph(v)
	idCell := plainCell(v.ID)
	if selected {
		idCell = styled(s.color, sgrBold, v.ID)
	}
	indent := stripIndent(s, selected, cursor)
	statusW := width - loop.DisplayWidth(indent.plain) - loop.DisplayWidth(v.ID) - 5
	head := joinCells(
		indent,
		styled(s.color, sgr, glyph),
		plainCell(" "),
		idCell,
		styled(s.color, sgrDim, " — "),
		plainCell(loop.TruncateDisplay(fleetStatusText(s, v), statusW)),
	)
	if v.Status == loop.StatusAccepted || v.Status == loop.StatusRejected {
		return []cell{head}
	}
	if len(v.Iterations) == 0 && !v.Live {
		// Nothing has run yet; the header says everything there is.
		return []cell{head}
	}

	bodyIndent := joinCells(stripIndent(s, false, cursor), plainCell("  "))
	lines := []cell{head, joinCells(
		bodyIndent,
		verdictRunCell(s, v),
		plainCell("   "),
		stageMeterCell(s, v),
	)}
	for _, t := range s.tails[v.ID] {
		lines = append(lines, joinCells(
			bodyIndent,
			styled(s.color, sgrDim, "| "),
			plainCell(loop.TruncateDisplay(strings.ReplaceAll(t, "\t", "    "), width-loop.DisplayWidth(bodyIndent.plain)-3)),
		))
	}
	return lines
}

// fleetStatusText answers "what is this loop doing" in one plain phrase,
// sized for a strip header. It is activityLine's plural-context sibling: no
// "now:" prefix and no key hints (those live in the footer and help).
func fleetStatusText(s frameState, v loop.LoopView) string {
	switch v.Status {
	case loop.StatusRunning:
		if !v.Live {
			return "running (no engine)"
		}
		var now string
		switch v.Phase {
		case loop.PhaseAgent:
			now = fmt.Sprintf("agent running · iter %d", v.PhaseIteration)
		case loop.PhaseVerify:
			now = fmt.Sprintf("verifying · iter %d", v.PhaseIteration)
		case loop.PhaseReview:
			now = "reviewer running"
		default:
			now = "between iterations"
		}
		if e := s.elapsedByID[v.ID]; e != "" {
			now += " · " + e
		}
		return now
	case loop.StatusPaused:
		return "paused"
	case loop.StatusGreen:
		if v.IterationsUsed == 0 {
			// The same honesty as the activity line: green with zero
			// iterations means the agent never ran.
			return "green at baseline — the verifier may not test the goal"
		}
		return fmt.Sprintf("ready for review · green in %d iteration(s)", v.IterationsUsed)
	case loop.StatusParked:
		return "parked — " + v.ParkedReason
	case loop.StatusAccepted:
		return "decided: accepted"
	case loop.StatusRejected:
		return "decided: rejected"
	}
	return v.Status
}

// verdictRunCell is the loop's convergence signature: one glyph per agent
// iteration (✗ red, ✓ green), plus a cyan ● for the iteration in flight.
// The baseline is skipped — it is the starting line, not a lap.
func verdictRunCell(s frameState, v loop.LoopView) cell {
	var cells []cell
	for _, it := range v.Iterations {
		if it.Baseline {
			continue
		}
		if len(cells) > 0 {
			cells = append(cells, plainCell(" "))
		}
		if it.Green {
			cells = append(cells, styled(s.color, sgrGreen, "✓"))
		} else {
			cells = append(cells, styled(s.color, sgrRed, "✗"))
		}
	}
	if v.Status == loop.StatusRunning && v.Live {
		if len(cells) > 0 {
			cells = append(cells, plainCell(" "))
		}
		cells = append(cells, styled(s.color, sgrCyan, "●"))
	}
	if len(cells) == 0 {
		return styled(s.color, sgrDim, "·")
	}
	return joinCells(cells...)
}

// stageMeterCell shows how far through the verifier the last verify got:
// ▮ per passed stage, ▯ per remaining, then the verdict word.
func stageMeterCell(s frameState, v loop.LoopView) cell {
	total := len(v.Verifier)
	if total == 0 {
		return cell{}
	}
	if len(v.Iterations) == 0 {
		return styled(s.color, sgrDim, "verify baseline…")
	}
	last := v.Iterations[len(v.Iterations)-1]
	label := last.FailingStage
	if last.Green {
		label = "green"
	}
	return joinCells(
		styled(s.color, sgrDim, "verify "),
		plainCell(strings.Repeat("▮", last.StagesPassed)+strings.Repeat("▯", total-last.StagesPassed)),
		styled(s.color, sgrDim, " "+label),
	)
}
