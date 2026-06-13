package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/mjbarefo/loopy/internal/loop"
)

// The frame renderer is a pure function: frameState in, one string out. The
// live monitor and `watch --once` share it, so what scripts capture is what
// humans see. Color is applied after plain-text layout and is never the only
// signal — every verdict keeps its word or glyph.
//
// Layout (no box chrome; dim rules, one dim rail separator, and at roomy
// sizes a blank row inside each rule plus a two-column gutter — margins are
// structure, not decoration):
//
//	 ∞ loopy   1 running · 1 paused · 2 to review                 ? help
//	─────────────────────────────────────────────────────────────────────
//
//	  ▶ ● fix-csv-quoting      2/8 │  ● fix-csv-quoting — running
//	    ◌ paused-import        1/8 │  goal   make the importer handle …
//	                               │  agent  claude · iter 2/8 · wall …
//	    ✓ green-deploy-docs    3/6 │  ● now: agent running · iter 3 · 1m
//	    ✗ flaky-importer       8/8 │
//	                               │  ▸ overview   live   diff   verifier
//	                               │   iter  result      agent  verify …
//
//	─────────────────────────────────────────────────────────────────────
//	 n new · enter open · ? keys                       next: loopy …
const (
	minWidth      = 40
	minHeight     = 8
	collapseWidth = 80 // below this the loop rail collapses away
	marginHeight  = 20 // below this (or collapseWidth) the margins collapse
	minRailWidth  = 22
	maxRailWidth  = 34
)

// ANSI SGR codes. The TUI styles by hand instead of taking a styling
// dependency; the frame must stay byte-deterministic for --once and tests.
const (
	sgrBold   = "1"
	sgrDim    = "2"
	sgrRed    = "31"
	sgrGreen  = "32"
	sgrYellow = "33"
	sgrCyan   = "36"
)

type frameState struct {
	width, height int
	color         bool
	once          bool // deterministic single frame: no key hints, no elapsed

	loops    []loop.LoopView
	broken   []loop.BrokenLoop
	selected int

	// initialized/agentsRegistered/detected steer onboarding: the empty
	// state offers the exact next step, executable in place.
	initialized      bool
	agentsRegistered bool
	detected         []loop.AgentSuggestion

	form formState

	focusDetail bool
	tab         tabID
	scroll      int // -1 = follow the tail
	art         artifact
	// elapsed strings are precomputed by the model (they need a clock; the
	// renderer must stay pure and deterministic).
	phaseElapsed  string
	synthElapsed  string
	confirmAbort  bool
	confirmDelete bool
	confirmAccept bool
	confirmReject bool
	flash         string
	showHelp      bool
	loadErr       string
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

// joinCells concatenates cells into one, preserving styled segments.
func joinCells(cells ...cell) cell {
	var plain, stl strings.Builder
	for _, c := range cells {
		plain.WriteString(c.plain)
		if c.styled != "" {
			stl.WriteString(c.styled)
		} else {
			stl.WriteString(c.plain)
		}
	}
	return cell{plain: plain.String(), styled: stl.String()}
}

// padCell pads (or truncates) a cell to exactly width visible columns.
// Styling is dropped when truncation is needed — a cut escape sequence is
// worse than a plain line.
func padCell(c cell, width int) string {
	w := loop.DisplayWidth(c.plain)
	if w > width {
		return loop.PadDisplay(c.plain, width)
	}
	out := c.plain
	if c.styled != "" {
		out = c.styled
	}
	return out + strings.Repeat(" ", width-w)
}

func rule(n int) string { return strings.Repeat("─", n) }

// roomy reports whether the frame has the room for margins: the blank row
// inside each rule and the two-column gutter. Below it the layout collapses
// to the dense form — the 40x8 floor and the degradation ladder are
// untouched.
func (s frameState) roomy() bool {
	return s.width >= collapseWidth && s.height >= marginHeight
}

// contentRows is the rows between the rules, minus the margin rows.
func (s frameState) contentRows() int {
	if s.roomy() {
		return s.height - 6
	}
	return s.height - 4
}

// railArea is the rail width (0 when collapsed or empty) and the detail
// width that remains beside it.
func (s frameState) railArea() (railW, detailW int) {
	detailW = s.width - 1
	if s.width >= collapseWidth && (len(s.loops) > 0 || len(s.broken) > 0) {
		railW = railWidth(s.loops, s.broken)
		detailW = s.width - railW - 2 // separator column + leading space
	}
	if s.roomy() {
		if railW > 0 {
			detailW -= 2 // the gutter columns ahead of the rail
		} else {
			detailW -= 1 // the two-column gutter replaces the single leading space
		}
	}
	return railW, detailW
}

func renderFrame(s frameState) string {
	if s.width < minWidth || s.height < minHeight {
		return fmt.Sprintf("terminal too small for the monitor (need at least %dx%d)\n", minWidth, minHeight)
	}
	// The margins: a two-column gutter ahead of the rail. The gap past the
	// separator stays a single space — widening it would shave columns off
	// the timeline, and facts outrank air.
	railW, detailW := s.railArea()
	contentRows := s.contentRows()
	gutter := ""
	if s.roomy() {
		gutter = "  "
	}

	var sel *loop.LoopView
	if s.selected >= 0 && s.selected < len(s.loops) {
		sel = &s.loops[s.selected]
	}

	detail := detailLines(s, sel, detailW, contentRows)
	var rail []cell
	if railW > 0 {
		rail = railLines(s, railW, contentRows)
	}

	// Chrome recedes: rules and the rail separator are dim, and at roomy
	// sizes a blank row under each rule lets the content float.
	blank := strings.Repeat(" ", s.width) + "\n"
	var b strings.Builder
	b.WriteString(padCell(headerCell(s), s.width) + "\n")
	b.WriteString(paint(s.color, sgrDim, rule(s.width)) + "\n")
	if s.roomy() {
		b.WriteString(blank)
	}
	for i := 0; i < contentRows; i++ {
		b.WriteString(gutter)
		if railW > 0 {
			b.WriteString(padCell(lineAt(rail, i), railW))
			b.WriteString(paint(s.color, sgrDim, "│"))
			b.WriteString(" ")
		} else if gutter == "" {
			b.WriteString(" ")
		}
		b.WriteString(padCell(lineAt(detail, i), detailW) + "\n")
	}
	if s.roomy() {
		b.WriteString(blank)
	}
	b.WriteString(paint(s.color, sgrDim, rule(s.width)) + "\n")
	b.WriteString(padCell(footerCell(s, sel, s.width), s.width) + "\n")
	return b.String()
}

func lineAt(lines []cell, i int) cell {
	if i < len(lines) {
		return lines[i]
	}
	return cell{}
}

// headerCell is the identity lockup plus the project pulse: the compact
// logo mark, the wordmark, and how many loops are in each bucket — bold
// numbers, plain words, dim separators.
func headerCell(s frameState) cell {
	var counts = map[string]int{}
	live := 0
	baselineGreen := 0
	for _, v := range s.loops {
		counts[v.Status]++
		if v.Status == loop.StatusRunning && v.Live {
			live++
		}
		if v.Status == loop.StatusGreen && v.IterationsUsed == 0 {
			baselineGreen++
		}
	}
	type bucket struct {
		n    int
		word string
	}
	var parts []bucket
	if live > 0 {
		parts = append(parts, bucket{live, "running"})
	}
	if stale := counts[loop.StatusRunning] - live; stale > 0 {
		parts = append(parts, bucket{stale, "stale"})
	}
	if n := counts[loop.StatusPaused]; n > 0 {
		parts = append(parts, bucket{n, "paused"})
	}
	// Baseline green is not a win: it leaves the "green to review" bucket
	// and gets named for what it is.
	if n := counts[loop.StatusGreen] - baselineGreen; n > 0 {
		parts = append(parts, bucket{n, "green to review"})
	}
	if baselineGreen > 0 {
		parts = append(parts, bucket{baselineGreen, "already green — check the verifier"})
	}
	if n := counts[loop.StatusParked]; n > 0 {
		parts = append(parts, bucket{n, "parked"})
	}
	if n := counts[loop.StatusAccepted] + counts[loop.StatusRejected]; n > 0 {
		parts = append(parts, bucket{n, "decided"})
	}
	if len(s.broken) > 0 {
		parts = append(parts, bucket{len(s.broken), "unreadable"})
	}
	cells := []cell{
		styled(s.color, sgrCyan, " ∞ "),
		styled(s.color, sgrBold, "loopy"),
		plainCell("   "),
	}
	if s.roomy() {
		cells = append([]cell{plainCell(" ")}, cells...)
	}
	if len(parts) == 0 {
		cells = append(cells, styled(s.color, sgrDim, "no loops yet"))
	}
	for i, p := range parts {
		if i > 0 {
			cells = append(cells, styled(s.color, sgrDim, " · "))
		}
		cells = append(cells,
			styled(s.color, sgrBold, fmt.Sprintf("%d", p.n)),
			plainCell(" "+p.word),
		)
	}
	left := joinCells(cells...)
	if s.once {
		return left
	}
	// One hint; everything else lives behind ? (q included).
	hints := "? help"
	gap := s.width - loop.DisplayWidth(left.plain) - loop.DisplayWidth(hints) - 1
	if gap < 2 {
		return left
	}
	return joinCells(left, plainCell(strings.Repeat(" ", gap)), styled(s.color, sgrDim, hints), plainCell(" "))
}

// railWidth sizes the loop rail to its content within sane bounds.
func railWidth(loops []loop.LoopView, broken []loop.BrokenLoop) int {
	w := minRailWidth
	for _, v := range loops {
		// marker(2) + glyph(2) + id + gap(2) + iters(5)
		if need := 2 + 2 + loop.DisplayWidth(v.ID) + 2 + 5; need > w {
			w = need
		}
	}
	for _, b := range broken {
		// marker(2) + glyph(2) + id + " (unreadable)"(13)
		if need := 2 + 2 + loop.DisplayWidth(b.ID) + 13; need > w {
			w = need
		}
	}
	if w > maxRailWidth {
		return maxRailWidth
	}
	return w
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
	case loop.StatusGreen:
		if v.IterationsUsed == 0 {
			return "!", sgrYellow // green at baseline: the agent never ran
		}
		return "✓", sgrGreen
	case loop.StatusAccepted:
		return "✓", sgrGreen
	case loop.StatusRejected:
		return "✗", sgrRed
	default: // parked
		return "✗", sgrRed
	}
}

func statusPhrase(v loop.LoopView) string {
	switch v.Status {
	case loop.StatusRunning:
		if v.Live {
			return "running"
		}
		return "running (no engine)"
	case loop.StatusGreen:
		if v.IterationsUsed == 0 {
			return "green at baseline (the agent never ran)"
		}
		return v.Status
	default:
		return v.Status
	}
}

// railVisible: decided loops leave the rail — the header still counts them
// and the logbook holds the history. The selected loop is always rendered,
// so `loopy watch <id>` can pin a decided one.
func railVisible(v loop.LoopView) bool {
	return v.Status != loop.StatusAccepted && v.Status != loop.StatusRejected
}

// railGroup buckets a loop by urgency: live work, things waiting on the
// human, dead history. The rail separates the groups with a blank row.
func railGroup(v loop.LoopView) int {
	switch r := statusRank(v); {
	case r == 0:
		return 0 // live
	case r <= 3:
		return 1 // needs you: paused, stale, green to review
	default:
		return 2 // history: parked and decided
	}
}

// railRow pairs a rendered rail line with the loop it represents; idx is -1
// for the blank gap rows between urgency groups and for broken loops. The
// mouse hit-test maps clicks back through this.
type railRow struct {
	line cell
	idx  int
}

func railRows(s frameState, railW, rows int) []railRow {
	// One accent per row: the status glyph carries the color, the cursor is
	// cyan, the ID is bold only when selected, and the budget is metadata.
	// A blank row separates the urgency groups — the gap is the label.
	var out []railRow
	selLine := 0
	prevGroup := -1
	idW := railW - 2 - 2 - 2 - 5
	for i, v := range s.loops {
		if !railVisible(v) && i != s.selected {
			continue
		}
		g := railGroup(v)
		if prevGroup >= 0 && g != prevGroup {
			out = append(out, railRow{idx: -1})
		}
		prevGroup = g
		marker := styled(s.color, sgrCyan, "▶ ")
		if i != s.selected {
			marker = plainCell("  ")
		}
		glyph, sgr := statusGlyph(v)
		id := loop.PadDisplay(v.ID, idW)
		idCell := plainCell(id)
		if i == s.selected {
			idCell = styled(s.color, sgrBold, id)
			selLine = len(out)
		}
		iters := fmt.Sprintf("%d/%d", v.IterationsUsed, v.MaxIterations)
		if loop.DisplayWidth(iters) > 5 {
			iters = fmt.Sprintf("%d", v.IterationsUsed)
		}
		out = append(out, railRow{idx: i, line: joinCells(
			marker,
			styled(s.color, sgr, glyph),
			plainCell(" "),
			idCell,
			styled(s.color, sgrDim, "  "+fmt.Sprintf("%5s", iters)),
		)})
	}
	for _, b := range s.broken {
		if prevGroup >= 0 && prevGroup != 2 {
			out = append(out, railRow{idx: -1})
		}
		prevGroup = 2
		out = append(out, railRow{idx: -1, line: joinCells(
			plainCell("  "),
			styled(s.color, sgrRed, "✗"),
			plainCell(" "+loop.TruncateDisplay(b.ID, idW)),
			styled(s.color, sgrDim, " (unreadable)"),
		)})
	}
	// Keep the selection visible when the list outgrows the rail.
	if len(out) <= rows {
		return out
	}
	start := 0
	if selLine >= rows {
		start = selLine - rows + 1
	}
	if start+rows > len(out) {
		start = len(out) - rows
	}
	return out[start : start+rows]
}

func railLines(s frameState, railW, rows int) []cell {
	rr := railRows(s, railW, rows)
	lines := make([]cell, len(rr))
	for i, r := range rr {
		lines[i] = r.line
	}
	return lines
}

func detailLines(s frameState, sel *loop.LoopView, width, rows int) []cell {
	if s.loadErr != "" {
		return []cell{{}, styled(s.color, sgrRed, "error: "+loop.TruncateDisplay(s.loadErr, width-8))}
	}
	if s.form.active {
		return formLines(s, width)
	}
	if sel == nil {
		if len(s.broken) > 0 {
			return brokenOnlyLines(s, width)
		}
		if len(s.loops) > 0 {
			return quietStateLines(s, width)
		}
		return emptyStateLines(s, width, rows)
	}
	if s.showHelp {
		return helpLines(s)
	}

	lines := make([]cell, 0, rows)
	lines = append(lines, detailHeaderLines(s, *sel, width)...)

	bodyRows := rows - len(lines)
	var body []cell
	if s.tab == tabOverview {
		body = overviewBody(s, *sel, width)
	} else {
		body = artifactBody(s, width)
	}
	lines = append(lines, window(body, bodyRows, s.scroll)...)
	return lines
}

// detailHeaderLines is the detail pane's fixed header: the status title, the
// goal (wrapped, up to three lines), the agent line, the activity line
// (wrapped, up to two), a spacer, and the nav. The body's scroll math and the
// mouse hit-test both count these rows, so they are built in one place.
//
// The header borrows the form's typography: dim labels, plain values, the
// status accent on the glyph only — the phrase repeats the glyph in words,
// so it stays plain.
func detailHeaderLines(s frameState, sel loop.LoopView, width int) []cell {
	glyph, sgr := statusGlyph(sel)
	lines := []cell{joinCells(
		styled(s.color, sgr, glyph+" "),
		styled(s.color, sgrBold, sel.ID),
		styled(s.color, sgrDim, " — "),
		plainCell(statusPhrase(sel)),
	)}
	for _, gl := range wrapCapped(sel.Goal, width-7, 3) {
		label := styled(s.color, sgrDim, "goal   ")
		if len(lines) > 1 {
			label = plainCell("       ") // the hanging indent under the label
		}
		lines = append(lines, joinCells(label, plainCell(loop.TruncateDisplay(gl, width-7))))
	}
	lines = append(lines, joinCells(
		styled(s.color, sgrDim, "agent  "),
		plainCell(sel.Agent),
		styled(s.color, sgrDim, " · iter "),
		plainCell(fmt.Sprintf("%d/%d", sel.IterationsUsed, sel.MaxIterations)),
		styled(s.color, sgrDim, " · wall "),
		plainCell(fmt.Sprintf("%s of %s", sel.WallClockUsed, sel.MaxWallClock)),
	))
	actCode, actGlyph, actText := activityParts(s, sel)
	if actGlyph == "" {
		lines = append(lines, cell{})
	} else {
		for i, al := range wrapCapped(actText, width-2, 2) {
			mark := styled(s.color, actCode, actGlyph)
			if i > 0 {
				mark = plainCell(" ")
			}
			lines = append(lines, joinCells(mark, plainCell(" "+loop.TruncateDisplay(al, width-2))))
		}
	}
	lines = append(lines, cell{})
	lines = append(lines, navBar(s))
	return lines
}

// wrapCapped word-wraps text to at most maxLines lines; everything past the
// cap is squeezed into (and truncated on) the last line, so nothing vanishes
// silently.
func wrapCapped(text string, width, maxLines int) []string {
	lines := loop.WrapDisplay(text, width)
	if len(lines) > maxLines {
		rest := strings.Join(lines[maxLines-1:], " ")
		lines = append(lines[:maxLines-1], loop.TruncateDisplay(rest, width))
	}
	return lines
}

// activityParts is the two-second answer to "what is it doing right now".
// Status color is glyph-sized: one colored glyph, then plain words.
func activityParts(s frameState, v loop.LoopView) (code, glyph, text string) {
	switch v.Status {
	case loop.StatusRunning:
		if !v.Live {
			return sgrYellow, "!", "no engine holds this loop — r resumes it, loopy abort stops it"
		}
		var now string
		switch v.Phase {
		case loop.PhaseAgent:
			now = fmt.Sprintf("now: agent running · iter %d", v.PhaseIteration)
		case loop.PhaseVerify:
			now = fmt.Sprintf("now: verifying · iter %d", v.PhaseIteration)
		case loop.PhaseReview:
			now = "now: reviewer critiquing the green diff"
		default:
			now = "now: between iterations"
		}
		if s.phaseElapsed != "" && v.Phase != "" {
			now += " · " + s.phaseElapsed
		}
		return sgrCyan, "●", now
	case loop.StatusPaused:
		return sgrYellow, "◌", "paused — r starts an engine and resumes"
	case loop.StatusGreen:
		if v.IterationsUsed == 0 {
			// Green with zero iterations means the agent never ran: honesty
			// over celebration.
			return sgrYellow, "!", "already green at baseline — nothing to do, or the verifier may not test the goal"
		}
		return sgrGreen, "✓", fmt.Sprintf("verifier green after %d iteration(s) — ready for review", v.IterationsUsed)
	case loop.StatusParked:
		return sgrRed, "✗", v.ParkedReason
	case loop.StatusAccepted:
		return sgrGreen, "✓", "decided: accepted"
	case loop.StatusRejected:
		return sgrRed, "✗", "decided: rejected"
	}
	return "", "", ""
}

// navBar is the quiet nav: ▸ plus cyan marks the active view, the rest sit
// dim. The ▸ is the non-color signal, so color is never the only one.
func navBar(s frameState) cell {
	cells := make([]cell, 0, tabCount*2)
	for i, name := range tabNames {
		if i > 0 {
			cells = append(cells, plainCell("   "))
		}
		if tabID(i) == s.tab {
			cells = append(cells, styled(s.color, sgrCyan, "▸ "+name))
		} else {
			cells = append(cells, styled(s.color, sgrDim, name))
		}
	}
	return joinCells(cells...)
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

func overviewBody(s frameState, v loop.LoopView, width int) []cell {
	var lines []cell
	if len(v.Iterations) == 0 {
		lines = append(lines, plainCell(" waiting for the baseline verify…"))
		return lines
	}
	// The timeline accents the verdict cell only; the metrics stay plain and
	// the baseline recedes. Whole-row color is reserved for the live row.
	lines = append(lines, styled(s.color, sgrDim, " "+loop.IterationRowHeader))
	for _, it := range v.Iterations {
		label, verdict, metrics := loop.RenderIterationRowParts(it)
		if it.Baseline {
			lines = append(lines, styled(s.color, sgrDim, loop.TruncateDisplay(" "+loop.RenderIterationRow(it), width)))
			continue
		}
		sgr := sgrGreen
		if !it.Green {
			sgr = sgrRed
		}
		lines = append(lines, joinCells(
			plainCell(fmt.Sprintf(" %-5s ", label)),
			styled(s.color, sgr, loop.PadDisplay(verdict, 18)),
			plainCell(" "+metrics),
		))
	}
	if v.Status == loop.StatusRunning && v.Live {
		lines = append(lines, joinCells(
			plainCell(fmt.Sprintf(" %-5d ", len(v.Iterations))),
			styled(s.color, sgrCyan, "●"),
			plainCell(" "+runningVerb(v)+"…"),
		))
	}
	if v.LastFeedback != "" && v.Status != loop.StatusGreen && v.Status != loop.StatusAccepted {
		lines = append(lines, cell{}, styled(s.color, sgrDim, " last feedback tail:"))
		for _, fl := range strings.Split(strings.TrimRight(v.LastFeedback, "\n"), "\n") {
			lines = append(lines, quoted(s, fl, width))
		}
	}
	// A short live tail keeps "what is it doing" on the default view; the
	// live tab has the full one.
	if v.Live && !s.art.missing && len(s.art.lines) > 0 {
		lines = append(lines, cell{}, styled(s.color, sgrDim, " live tail · "+s.art.label+":"))
		tail := s.art.lines
		if len(tail) > 6 {
			tail = tail[len(tail)-6:]
		}
		for _, tl := range tail {
			lines = append(lines, quoted(s, strings.ReplaceAll(tl, "\t", "    "), width))
		}
	}
	return lines
}

// quoted is the shared gutter for quoted output (feedback and live tails):
// the same dim "| " gutter the CLI renderers quote with, then the line
// verbatim. ("│" stays reserved for the rail separator.)
func quoted(s frameState, line string, width int) cell {
	return joinCells(
		styled(s.color, sgrDim, " | "),
		plainCell(loop.TruncateDisplay(line, width-4)),
	)
}

func runningVerb(v loop.LoopView) string {
	switch v.Phase {
	case loop.PhaseAgent:
		return "agent running"
	case loop.PhaseVerify:
		return "verifying"
	case loop.PhaseReview:
		return "reviewer running"
	default:
		return "running"
	}
}

// artifactBody renders the diff/verifier/live tabs answer-first: a header in
// plain words (what changed, did it pass), then the raw evidence with per-line
// styling. Every styled line keeps a non-color signal — the diff's +/- and the
// log's === markers carry the meaning when color is off.
func artifactBody(s frameState, width int) []cell {
	art := s.art
	if art.missing {
		return []cell{styled(s.color, sgrDim, " nothing here yet ("+art.label+")")}
	}
	var lines []cell
	switch s.tab {
	case tabDiff:
		lines = append(lines, diffHeaderLines(s, width)...)
	case tabVerifier:
		lines = append(lines, verifierHeaderLines(s, width)...)
	}
	lines = append(lines, artifactBanner(s, width))
	switch s.tab {
	case tabDiff:
		for _, l := range art.lines {
			lines = append(lines, diffLine(s, l, width))
		}
	case tabVerifier:
		lines = append(lines, verifierLogLines(s, width)...)
	default:
		for _, l := range art.lines {
			lines = append(lines, plainCell(" "+loop.TruncateDisplay(strings.ReplaceAll(l, "\t", "    "), width-1)))
		}
	}
	return lines
}

// artifactBanner is the provenance line: which file, which iteration, and the
// partial-load warning when the viewer cap cut the head off.
func artifactBanner(s frameState, width int) cell {
	art := s.art
	banner := " " + art.label
	if !art.truncated {
		return styled(s.color, sgrDim, loop.TruncateDisplay(banner, width))
	}
	shown := int64(0)
	for _, l := range art.lines {
		shown += int64(len(l)) + 1
	}
	banner += fmt.Sprintf(" · truncated: showing last %s of %s", loop.HumanBytes(int(shown)), loop.HumanBytes(int(art.size)))
	// The facts stay dim; the glyph alone says "partial".
	return joinCells(
		styled(s.color, sgrYellow, " !"),
		styled(s.color, sgrDim, loop.TruncateDisplay(banner, width-2)),
	)
}

// diffHeaderLines answers "what changed" before the patch: a one-line total,
// then one line per file with a kind glyph and plain words. A truncated patch
// gets no header — counting only the visible tail would lie, and the banner
// already says the load is partial.
func diffHeaderLines(s frameState, width int) []cell {
	if s.art.truncated {
		return nil
	}
	files := loop.ParseDiff(s.art.lines)
	if len(files) == 0 {
		return nil
	}
	adds, dels := 0, 0
	for _, f := range files {
		adds += f.Adds
		dels += f.Dels
	}
	word := "files"
	if len(files) == 1 {
		word = "file"
	}
	summary := fmt.Sprintf(" %d %s changed", len(files), word)
	if c := diffCounts(adds, dels); c != "" {
		summary += " · " + c
	}
	lines := []cell{styled(s.color, sgrBold, loop.TruncateDisplay(summary, width))}
	for _, f := range files {
		glyph, sgr := fileKindGlyph(f.Kind)
		facts := f.Kind
		switch {
		case f.Binary:
			facts += " · binary"
		case diffCounts(f.Adds, f.Dels) != "":
			facts += " · " + diffCounts(f.Adds, f.Dels)
		}
		lines = append(lines, joinCells(
			plainCell(" "),
			styled(s.color, sgr, glyph),
			plainCell(" "+loop.TruncateDisplay(f.Path, width-4)),
			styled(s.color, sgrDim, "  "+facts),
		))
	}
	return append(lines, cell{})
}

// diffCounts renders "+A -D" with zero parts omitted; empty when nothing
// was counted (binary files, pure renames).
func diffCounts(adds, dels int) string {
	switch {
	case adds > 0 && dels > 0:
		return fmt.Sprintf("+%d -%d", adds, dels)
	case adds > 0:
		return fmt.Sprintf("+%d", adds)
	case dels > 0:
		return fmt.Sprintf("-%d", dels)
	}
	return ""
}

// fileKindGlyph pairs each file kind with a glyph and a color; the kind word
// follows in plain text, so color is never the only signal.
func fileKindGlyph(kind string) (glyph, sgr string) {
	switch kind {
	case "new file":
		return "+", sgrGreen
	case "deleted":
		return "-", sgrRed
	case "renamed":
		return "→", sgrCyan
	default:
		return "~", sgrYellow
	}
}

// diffLine styles one patch line: file headers bold, hunk markers dim cyan,
// adds green, removals red, context plain. The classification is stateless,
// so a tail-truncated patch starting mid-hunk still reads.
func diffLine(s frameState, l string, width int) cell {
	t := " " + loop.TruncateDisplay(strings.ReplaceAll(l, "\t", "    "), width-1)
	switch {
	case strings.HasPrefix(l, "diff --git "),
		strings.HasPrefix(l, "+++ "),
		strings.HasPrefix(l, "--- "):
		return styled(s.color, sgrBold, t)
	case strings.HasPrefix(l, "@@"):
		return styled(s.color, sgrDim+";"+sgrCyan, t)
	case strings.HasPrefix(l, "+"):
		return styled(s.color, sgrGreen, t)
	case strings.HasPrefix(l, "-"):
		return styled(s.color, sgrRed, t)
	}
	return plainCell(t)
}

// artifactIteration finds the iteration record the loaded artifact belongs
// to; nil when the artifact carries no iteration or the record is gone.
func artifactIteration(s frameState) *loop.IterationView {
	if s.art.iter < 0 || s.selected < 0 || s.selected >= len(s.loops) {
		return nil
	}
	for i := range s.loops[s.selected].Iterations {
		if it := &s.loops[s.selected].Iterations[i]; it.Index == s.art.iter {
			return it
		}
	}
	return nil
}

// verifierHeaderLines answers "did it pass" before the log: one scoreboard
// row per stage and a plain-words verdict. Stages the run never reached
// (the verifier short-circuits) say so.
func verifierHeaderLines(s frameState, width int) []cell {
	it := artifactIteration(s)
	if it == nil {
		return nil
	}
	stages := s.loops[s.selected].Verifier
	var lines []cell
	nameW := 0
	for _, st := range stages {
		if w := loop.DisplayWidth(st.Name); w > nameW {
			nameW = w
		}
	}
	for i, st := range stages {
		// An ask stage is judged by the agent, not a shell command; tag it so
		// the human sees which greens are mechanical and which are a judgment.
		// The tag is a word, never just color — NO_COLOR stays legible.
		tag := ""
		descBudget := width - nameW - 12
		if st.Kind == loop.KindAsk {
			tag = "ask "
			descBudget -= 4
		}
		if i >= len(it.Stages) {
			lines = append(lines, styled(s.color, sgrDim, loop.TruncateDisplay(
				" · "+loop.PadDisplay(st.Name, nameW)+"  "+tag+st.Descriptor()+"  did not run", width)))
			continue
		}
		r := it.Stages[i]
		glyph, sgr := "✓", sgrGreen
		if r.ExitCode != 0 {
			glyph, sgr = "✗", sgrRed
		}
		row := []cell{
			plainCell(" "),
			styled(s.color, sgr, glyph),
			plainCell(" " + loop.PadDisplay(st.Name, nameW) + "  "),
		}
		if tag != "" {
			row = append(row, styled(s.color, sgrDim, tag))
		}
		row = append(row,
			plainCell(loop.TruncateDisplay(st.Descriptor(), descBudget)),
			styled(s.color, sgrDim, "  ("+loop.HumanDuration(time.Duration(r.DurationMS)*time.Millisecond)+")"),
		)
		lines = append(lines, joinCells(row...))
	}
	switch {
	case it.Green && it.Baseline:
		// Green before the agent ever ran proves nothing about the goal.
		lines = append(lines, joinCells(
			plainCell(" "),
			styled(s.color, sgrYellow, "!"),
			plainCell(" "+loop.TruncateDisplay("green at baseline — the agent never ran; this verifier may not test the goal", width-3)),
		))
	case it.Green:
		lines = append(lines, joinCells(
			plainCell(" "),
			styled(s.color, sgrGreen, "✓"),
			plainCell(" green: the goal is met"),
		))
	default:
		verdict := "red: the verifier failed — the log below shows why"
		if it.FailingStage != "" {
			verdict = fmt.Sprintf("red: stage %s failed — the log below shows why", it.FailingStage)
		}
		lines = append(lines, joinCells(
			plainCell(" "),
			styled(s.color, sgrRed, "✗"),
			plainCell(" "+loop.TruncateDisplay(verdict, width-3)),
		))
	}
	return append(lines, cell{})
}

// verifierLogLines styles the log so the answer pops: "=== stage" markers are
// dim dividers, and on a red run the passing stages' output dims so the
// failing stage's lines read bright. A green run stays plain throughout.
func verifierLogLines(s frameState, width int) []cell {
	failing := ""
	if it := artifactIteration(s); it != nil && !it.Green {
		failing = it.FailingStage
	}
	cur := ""
	var lines []cell
	for _, l := range s.art.lines {
		t := " " + loop.TruncateDisplay(strings.ReplaceAll(l, "\t", "    "), width-1)
		if name, ok := stageMarkerName(l); ok {
			cur = name
			lines = append(lines, styled(s.color, sgrDim, t))
			continue
		}
		if failing != "" && cur != "" && cur != failing {
			lines = append(lines, styled(s.color, sgrDim, t))
			continue
		}
		lines = append(lines, plainCell(t))
	}
	return lines
}

// stageMarkerName parses the verifier runner's "=== stage <name>: …" lines.
func stageMarkerName(l string) (string, bool) {
	rest, ok := strings.CutPrefix(l, "=== stage ")
	if !ok {
		return "", false
	}
	name, _, ok := strings.Cut(rest, ":")
	return name, ok
}

// emptyStateLines is first-run onboarding: the one place the mascot lives in
// the working monitor, plus a checklist whose next step is executable in
// place — i initializes, digits register detected agents, n starts a loop.
func emptyStateLines(s frameState, width, rows int) []cell {
	rowsOut := []cell{}
	if rows >= 16 && width >= 64 {
		for i, art := range logoArt {
			side := ""
			switch i {
			case 1:
				side = "        l o o p y"
			case 3:
				side = "   " + logoTagline
			}
			rowsOut = append(rowsOut, joinCells(styled(s.color, sgrCyan, "   "+art), plainCell(side)))
		}
	}
	rowsOut = append(rowsOut,
		cell{},
		joinCells(
			styled(s.color, sgrBold, " no loops yet"),
			plainCell(" — the path to the first one:"),
		),
		cell{},
	)

	// Step 1: init.
	if s.initialized {
		rowsOut = append(rowsOut, joinCells(
			plainCell("   1. initialize the repo "),
			styled(s.color, sgrGreen, "✓"),
			styled(s.color, sgrDim, "  (.loopy/ exists)"),
		))
	} else {
		rowsOut = append(rowsOut, joinCells(
			plainCell("   1. "),
			styled(s.color, sgrCyan, "press i"),
			plainCell(" to initialize this repo (creates .loopy/, git-ignores it)"),
		))
	}

	// Step 2: an agent.
	switch {
	case s.agentsRegistered:
		rowsOut = append(rowsOut, joinCells(
			plainCell("   2. register an agent "),
			styled(s.color, sgrGreen, "✓"),
			styled(s.color, sgrDim, "  (see loopy agent list)"),
		))
	case !s.initialized:
		rowsOut = append(rowsOut, styled(s.color, sgrDim, "   2. register an agent"))
	case len(s.detected) > 0:
		rowsOut = append(rowsOut, plainCell("   2. register an agent — found on this machine:"))
		for i, d := range s.detected {
			if i >= 3 {
				break
			}
			rowsOut = append(rowsOut, joinCells(
				plainCell("        "),
				styled(s.color, sgrCyan, fmt.Sprintf("press %d", i+1)),
				plainCell(loop.TruncateDisplay(fmt.Sprintf(" for %s  (%s)", d.Name, d.Cmd), width-15)),
			))
		}
	default:
		rowsOut = append(rowsOut, plainCell("   2. register an agent:"),
			styled(s.color, sgrDim, loop.TruncateDisplay(`        loopy agent add claude --cmd "claude -p {prompt} --permission-mode acceptEdits" --default`, width)))
	}

	// Step 3: the loop.
	if s.initialized && s.agentsRegistered {
		rowsOut = append(rowsOut, joinCells(
			plainCell("   3. "),
			styled(s.color, sgrCyan, "press n"),
			plainCell(" and describe the goal — the agent iterates until your verifier passes"),
		))
	} else {
		rowsOut = append(rowsOut, styled(s.color, sgrDim, "   3. start a loop (n)"))
	}

	rowsOut = append(rowsOut,
		cell{},
		styled(s.color, sgrDim, " then watch it converge here. (? help · q quits)"),
	)
	return rowsOut
}

// quietStateLines is the rail at rest: every loop is decided, nothing needs
// eyes. The newest accepted loop keeps its apply command on screen — the
// human's next move (apply on a branch, commit, open the PR) must outlive
// a three-second flash.
func quietStateLines(s frameState, width int) []cell {
	lines := []cell{
		{},
		joinCells(
			styled(s.color, sgrBold, " all quiet"),
			plainCell(" — every loop is decided"),
		),
		{},
		joinCells(plainCell("   "), styled(s.color, sgrCyan, "n"), plainCell(" starts the next loop")),
		joinCells(plainCell("   the history: "), styled(s.color, sgrCyan, "loopy logbook")),
	}
	last := newestAcceptedWithCommand(s.loops)
	if last != nil {
		lines = append(lines,
			cell{},
			plainCell(loop.TruncateDisplay(fmt.Sprintf("   %s was accepted; its diff is durable — to ship it:", last.ID), width-1)),
			joinCells(plainCell("     "), styled(s.color, sgrCyan, loop.TruncateDisplay(last.NextCommand, width-6))),
			styled(s.color, sgrDim, "     on a branch, then commit and open your PR"),
		)
	}
	return lines
}

// newestAcceptedWithCommand is the loop whose apply command the quiet rail
// shows (and the c key copies): the most recently decided accepted loop that
// still knows its final-diff path.
func newestAcceptedWithCommand(loops []loop.LoopView) *loop.LoopView {
	var last *loop.LoopView
	for i := range loops {
		v := &loops[i]
		if v.Status == loop.StatusAccepted && v.NextCommand != "" {
			if last == nil || v.EndedAt > last.EndedAt {
				last = v
			}
		}
	}
	return last
}

func brokenOnlyLines(s frameState, width int) []cell {
	lines := []cell{{}, styled(s.color, sgrRed, " every loop here is unreadable:")}
	for _, b := range s.broken {
		lines = append(lines, plainCell("   "+loop.TruncateDisplay(b.ID+": "+b.Err, width-3)))
	}
	lines = append(lines, cell{}, styled(s.color, sgrCyan, " run: loopy doctor"))
	return lines
}

// helpLines styles keys the way onboarding styles its press-hints: the key
// is the action, so the key gets the accent.
func helpLines(s frameState) []cell {
	keys := []struct{ key, desc string }{
		{"↑/↓ or j/k", "select loop (list) · scroll (detail)"},
		{"enter", "focus the detail pane for scrolling"},
		{"esc", "back to the loop list · dismiss"},
		{"tab / 1-4", "switch view: overview, live, diff, verifier"},
		{"n", "start a new loop (goal + the project verifier)"},
		{"g / G", "jump to top / follow the tail"},
		{"pgup/pgdn", "page through the body"},
		{"mouse", "wheel scrolls · click selects loops and views"},
		{"c", "copy the next command (terminal must allow OSC 52)"},
		{"p", "pause at the next iteration boundary"},
		{"r", "resume a paused loop · reject a parked one (confirms)"},
		{"a", "abort a moving loop · accept a green one (confirms)"},
		{"d", "delete the loop and its evidence (asks for confirmation)"},
		{"o", "quit and print the next command"},
		{"q", "quit"},
	}
	lines := []cell{{}, styled(s.color, sgrBold, " keys")}
	for _, k := range keys {
		lines = append(lines, joinCells(
			styled(s.color, sgrCyan, "   "+loop.PadDisplay(k.key, 12)),
			plainCell("  "+k.desc),
		))
	}
	for _, note := range []string{
		"",
		" the monitor never writes loop state; controls go through",
		" control.json, decisions through the audited CLI.",
		" accepting a non-green loop stays there: loopy accept --override.",
	} {
		lines = append(lines, styled(s.color, sgrDim, note))
	}
	return lines
}

// The footer ships one short hint chain; every other binding lives behind ?.
// Facts (the next command) keep their place — only hints compressed.
const listHints = "n new · enter open · ? keys"
const detailHints = "esc back · ? keys"

func footerCell(s frameState, sel *loop.LoopView, width int) cell {
	margin := " "
	if s.roomy() {
		margin = "  "
	}
	switch {
	case s.form.active && s.flash == "":
		// The wizard screens carry their own affordance line.
		return cell{}
	case s.confirmAbort && sel != nil:
		return joinCells(
			plainCell(margin),
			styled(s.color, sgrRed, "✗"),
			plainCell(loop.TruncateDisplay(fmt.Sprintf(" abort %s? y to confirm · n to cancel", sel.ID), width-len(margin)-1)),
		)
	case s.confirmDelete && sel != nil:
		return joinCells(
			plainCell(margin),
			styled(s.color, sgrRed, "✗"),
			plainCell(loop.TruncateDisplay(fmt.Sprintf(" delete %s? all its evidence is removed — y to confirm · n to cancel", sel.ID), width-len(margin)-1)),
		)
	case s.confirmAccept && sel != nil:
		return joinCells(
			plainCell(margin),
			styled(s.color, sgrGreen, "✓"),
			plainCell(loop.TruncateDisplay(fmt.Sprintf(" accept %s? the decision is recorded — y to confirm · n to cancel", sel.ID), width-len(margin)-1)),
		)
	case s.confirmReject && sel != nil:
		return joinCells(
			plainCell(margin),
			styled(s.color, sgrRed, "✗"),
			plainCell(loop.TruncateDisplay(fmt.Sprintf(" reject %s? evidence kept, worktree freed — y to confirm · n to cancel", sel.ID), width-len(margin)-1)),
		)
	case s.flash != "":
		return joinCells(
			plainCell(margin),
			styled(s.color, sgrBold, loop.TruncateDisplay(s.flash, width-len(margin))),
		)
	}

	next := ""
	if sel != nil && sel.NextCommand != "" && !(sel.Status == loop.StatusRunning && !s.once) {
		// Inside the live monitor a running loop's "next" is the monitor
		// itself — pointless; --once keeps it for scripts.
		next = "next: " + sel.NextCommand
	}
	if s.once {
		return styled(s.color, sgrCyan, " "+next)
	}

	hints := listHints
	if s.focusDetail {
		hints = detailHints
	}
	if next == "" {
		return styled(s.color, sgrDim, margin+hints)
	}
	// The next command always wins the space fight. Hints yield whole —
	// first down to the bare ? (so the way back to every key stays on
	// screen), then entirely — never cut mid-word.
	for _, h := range []string{hints, "? keys"} {
		keyText := margin + h
		gap := width - loop.DisplayWidth(keyText) - loop.DisplayWidth(next) - len(margin)
		if gap < 2 {
			continue
		}
		return joinCells(
			styled(s.color, sgrDim, keyText),
			plainCell(strings.Repeat(" ", gap)),
			styled(s.color, sgrCyan, next),
			plainCell(margin),
		)
	}
	return styled(s.color, sgrCyan, loop.TruncateDisplay(margin+next, width))
}
