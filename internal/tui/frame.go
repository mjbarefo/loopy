package tui

import (
	"fmt"
	"strings"

	"github.com/mjbarefo/loopy/internal/loop"
)

// The frame renderer is a pure function: frameState in, one string out. The
// live monitor and `watch --once` share it, so what scripts capture is what
// humans see. Color is applied after plain-text layout and is never the only
// signal — every verdict keeps its word or glyph.

const (
	minWidth      = 40
	minHeight     = 8
	collapseWidth = 80 // below this the loop list pane collapses away
	leftPaneWidth = 21
)

// ANSI SGR codes. The TUI styles by hand instead of taking a styling
// dependency; the frame must stay byte-deterministic for --once and tests.
const (
	sgrBold   = "1"
	sgrDim    = "2"
	sgrInvert = "7"
	sgrRed    = "31"
	sgrGreen  = "32"
	sgrYellow = "33"
	sgrCyan   = "36"
)

type frameState struct {
	width, height int
	color         bool

	loops    []loop.LoopView
	selected int

	focusDetail  bool
	tab          tabID
	scroll       int // -1 = follow the tail
	art          artifact
	confirmAbort bool
	flash        string
	showHelp     bool
	loadErr      string
}

// cell is a piece of text with an optional styled form; layout always
// measures the plain form.
type cell struct {
	plain  string
	styled string
}

func plainCell(s string) cell { return cell{plain: s} }

func paint(color bool, code, s string) string {
	if !color || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func styled(color bool, code, s string) cell {
	return cell{plain: s, styled: paint(color, code, s)}
}

func renderFrame(s frameState) string {
	if s.width < minWidth || s.height < minHeight {
		return fmt.Sprintf("terminal too small for the monitor (need at least %dx%d)\n", minWidth, minHeight)
	}
	wide := s.width >= collapseWidth
	rightW := s.width - 2
	if wide {
		rightW = s.width - leftPaneWidth - 3
	}
	contentRows := s.height - 4

	var sel *loop.LoopView
	if s.selected >= 0 && s.selected < len(s.loops) {
		sel = &s.loops[s.selected]
	}

	right := rightLines(s, sel, rightW, contentRows)
	var left []cell
	if wide {
		left = leftLines(s, contentRows)
	}

	var b strings.Builder
	// Top border carries the pane titles, like the design sketch.
	title := titleCell(s, sel)
	if wide {
		b.WriteString("┌" + borderLabel(s, plainCell(" loops "), leftPaneWidth) + "┬" + borderLabel(s, title, rightW) + "┐\n")
	} else {
		b.WriteString("┌" + borderLabel(s, title, rightW) + "┐\n")
	}
	for i := 0; i < contentRows; i++ {
		b.WriteString("│")
		if wide {
			b.WriteString(padCell(lineAt(left, i), leftPaneWidth) + "│")
		}
		b.WriteString(padCell(lineAt(right, i), rightW) + "│\n")
	}
	if wide {
		b.WriteString("├" + dashes(leftPaneWidth) + "┴" + dashes(rightW) + "┤\n")
	} else {
		b.WriteString("├" + dashes(rightW) + "┤\n")
	}
	b.WriteString("│" + footerCell(s, sel, s.width-2) + "│\n")
	b.WriteString("└" + dashes(s.width-2) + "┘\n")
	return b.String()
}

func lineAt(lines []cell, i int) cell {
	if i < len(lines) {
		return lines[i]
	}
	return cell{}
}

func dashes(n int) string { return strings.Repeat("─", n) }

// borderLabel inlays a label into a horizontal border segment of the given
// width (the label is truncated rather than ever widening the frame).
func borderLabel(s frameState, label cell, width int) string {
	runes := []rune(label.plain)
	if len(runes) > width {
		label = plainCell(string(runes[:width-1]) + "…")
		runes = []rune(label.plain)
	}
	out := label.plain
	if label.styled != "" {
		out = label.styled
	}
	return out + dashes(width-len(runes))
}

// padCell pads (or truncates) a cell to exactly width visible columns.
func padCell(c cell, width int) string {
	runes := []rune(c.plain)
	if len(runes) > width {
		return string(runes[:width-1]) + "…"
	}
	out := c.plain
	if c.styled != "" {
		out = c.styled
	}
	return out + strings.Repeat(" ", width-len(runes))
}

func truncRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(runes[:n-1]) + "…"
}

// statusGlyph pairs every status with a non-color signal.
func statusGlyph(v loop.LoopView) (glyph, sgr string) {
	switch v.Status {
	case loop.StatusRunning:
		if v.Live {
			return "●", sgrCyan
		}
		return "!", sgrYellow // says running, but no engine holds the lock
	case loop.StatusPaused:
		return "◌", sgrYellow
	case loop.StatusGreen, loop.StatusAccepted:
		return "✓", sgrGreen
	default: // parked, rejected
		return "✗", sgrRed
	}
}

func statusText(v loop.LoopView) string {
	if v.Status == loop.StatusRunning && !v.Live {
		return "running (no engine)"
	}
	return v.Status
}

func titleCell(s frameState, sel *loop.LoopView) cell {
	if sel == nil {
		return plainCell(" loopy — no loops yet ")
	}
	_, sgr := statusGlyph(*sel)
	status := statusText(*sel)
	rest := fmt.Sprintf(" · iter %d/%d · %s/%s ", sel.IterationsUsed, sel.MaxIterations, sel.WallClockUsed, sel.MaxWallClock)
	plain := " " + sel.ID + " · " + status + rest
	styledTitle := " " + sel.ID + " · " + paint(s.color, sgr, status) + rest
	return cell{plain: plain, styled: styledTitle}
}

func leftLines(s frameState, rows int) []cell {
	lines := make([]cell, 0, rows)
	if len(s.loops) == 0 {
		lines = append(lines, styled(s.color, sgrDim, " (none)"))
		return lines
	}
	// Keep the selection visible when the list outgrows the pane.
	start := 0
	if len(s.loops) > rows && s.selected >= rows {
		start = s.selected - rows + 1
	}
	for i := start; i < len(s.loops) && len(lines) < rows; i++ {
		v := s.loops[i]
		marker := "  "
		if i == s.selected {
			marker = "▶ "
		}
		glyph, sgr := statusGlyph(v)
		name := truncRunes(v.ID, leftPaneWidth-6)
		plain := " " + marker + name + strings.Repeat(" ", leftPaneWidth-6-len([]rune(name))) + " " + glyph
		text := paint(s.color, sgr, plain)
		if i == s.selected {
			text = paint(s.color, sgrBold, text)
		}
		lines = append(lines, cell{plain: plain, styled: text})
	}
	return lines
}

func rightLines(s frameState, sel *loop.LoopView, width, rows int) []cell {
	if s.loadErr != "" {
		return []cell{{}, styled(s.color, sgrRed, " error: "+truncRunes(s.loadErr, width-8))}
	}
	if sel == nil {
		return []cell{
			{},
			plainCell(" no loops yet — start one:"),
			{},
			styled(s.color, sgrCyan, `   loopy "<goal>"`),
		}
	}
	if s.showHelp {
		return helpLines(s)
	}

	lines := make([]cell, 0, rows)
	lines = append(lines, tabBar(s))
	lines = append(lines, styled(s.color, sgrDim, " goal: "+truncRunes(sel.Goal, width-7)))
	lines = append(lines, cell{})

	bodyRows := rows - len(lines)
	var body []cell
	switch s.tab {
	case tabIterations:
		body = iterationsBody(s, *sel, width)
	default:
		body = artifactBody(s, width)
	}
	lines = append(lines, window(body, bodyRows, s.scroll)...)
	return lines
}

func tabBar(s frameState) cell {
	var plain, styledBar strings.Builder
	plain.WriteString(" ")
	styledBar.WriteString(" ")
	for i, name := range tabNames {
		var p string
		if tabID(i) == s.tab {
			p = "[" + name + "]"
			styledBar.WriteString(paint(s.color, sgrInvert, p))
		} else {
			p = " " + name + " "
			styledBar.WriteString(p)
		}
		plain.WriteString(p)
		plain.WriteString(" ")
		styledBar.WriteString(" ")
	}
	return cell{plain: plain.String(), styled: styledBar.String()}
}

// window slices body lines to the visible rows. scroll < 0 follows the tail
// (live behavior); otherwise it is a clamped offset from the top.
func window(lines []cell, rows, scroll int) []cell {
	if rows <= 0 {
		return nil
	}
	if len(lines) <= rows {
		return lines
	}
	maxStart := len(lines) - rows
	start := maxStart
	if scroll >= 0 && scroll < maxStart {
		start = scroll
	}
	return lines[start : start+rows]
}

func iterationsBody(s frameState, v loop.LoopView, width int) []cell {
	var lines []cell
	if len(v.Iterations) == 0 {
		return []cell{plainCell(" waiting for the baseline verify…")}
	}
	lines = append(lines, styled(s.color, sgrDim, "  iter      verdict            agent      verify     diff"))
	for _, it := range v.Iterations {
		row := "  " + loop.RenderIterationRow(it)
		sgr := sgrGreen
		if !it.Green {
			sgr = sgrRed
		}
		if it.Baseline {
			sgr = sgrDim
		}
		lines = append(lines, styled(s.color, sgr, row))
	}
	if v.Status == loop.StatusRunning {
		label := fmt.Sprintf("  %-9d ● running…", len(v.Iterations))
		if !v.Live {
			label = "  (no live engine — resume to continue)"
		}
		lines = append(lines, styled(s.color, sgrCyan, label))
	}
	if v.ParkedReason != "" {
		lines = append(lines, cell{}, styled(s.color, sgrYellow, " note: "+truncRunes(v.ParkedReason, width-8)))
	}
	if v.LastFeedback != "" && v.Status != loop.StatusGreen && v.Status != loop.StatusAccepted {
		lines = append(lines, cell{}, styled(s.color, sgrDim, " last feedback tail:"))
		for _, fl := range strings.Split(strings.TrimRight(v.LastFeedback, "\n"), "\n") {
			lines = append(lines, plainCell("  | "+truncRunes(fl, width-5)))
		}
	}
	return lines
}

func artifactBody(s frameState, width int) []cell {
	art := s.art
	if art.missing {
		return []cell{styled(s.color, sgrDim, " nothing here yet ("+art.label+")")}
	}
	banner := " " + art.label
	code := sgrDim
	if art.truncated {
		shown := int64(0)
		for _, l := range art.lines {
			shown += int64(len(l)) + 1
		}
		banner += fmt.Sprintf(" · truncated: showing last %s of %s", loop.HumanBytes(int(shown)), loop.HumanBytes(int(art.size)))
		code = sgrYellow
	}
	lines := []cell{styled(s.color, code, truncRunes(banner, width))}
	for _, l := range art.lines {
		lines = append(lines, plainCell(" "+truncRunes(strings.ReplaceAll(l, "\t", "    "), width-1)))
	}
	return lines
}

func helpLines(s frameState) []cell {
	rows := []string{
		"",
		" keys",
		"   ↑/↓ or j/k     select loop (list) · scroll (detail)",
		"   enter          drill into the detail pane",
		"   esc            back to the loop list · dismiss",
		"   tab / 1-4      switch view: live, iterations, diff, verifier",
		"   g / G          jump to top / follow the tail",
		"   p              pause at the next iteration boundary",
		"   r              resume a paused loop (spawns the engine)",
		"   a              abort (asks for confirmation)",
		"   o              quit and print the review command",
		"   q              quit",
		"",
		" the monitor never writes loop state; controls go through",
		" control.json and the engine honors them at phase boundaries.",
	}
	lines := make([]cell, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, plainCell(r))
	}
	return lines
}

func footerCell(s frameState, sel *loop.LoopView, width int) string {
	switch {
	case s.confirmAbort && sel != nil:
		return padCell(styled(s.color, sgrRed, fmt.Sprintf(" abort %s? y to confirm · n to cancel", sel.ID)), width)
	case s.flash != "":
		return padCell(styled(s.color, sgrYellow, " "+s.flash), width)
	}
	keys := " ↑↓ loop · enter drill · tab view · p pause · r resume · a abort · ? help · q quit"
	if s.focusDetail {
		keys = " ↑↓ scroll · g top · G follow · esc back · tab view · ? help · q quit"
	}
	next := ""
	if sel != nil && sel.NextCommand != "" {
		next = "next: " + sel.NextCommand + " "
	}
	// The exact next command always wins the space fight.
	keyW := width - len([]rune(next))
	if keyW < 0 {
		return padCell(plainCell(" "+truncRunes(strings.TrimSpace(next), width-1)), width)
	}
	plain := padCell(plainCell(keys), keyW) + next
	stl := padCell(styled(s.color, sgrDim, truncRunes(keys, keyW)), keyW) + paint(s.color, sgrCyan, next)
	return padCell(cell{plain: plain, styled: stl}, width)
}
