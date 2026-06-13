package tui

import "github.com/mjbarefo/loopy/internal/loop"

// Mouse support is hit-testing over the frame's geometry. renderFrame is a
// pure function of frameState, so "what is under (x, y)" is one too — the
// arithmetic here must mirror renderFrame's layout exactly, and the tests
// hold the two together.

type hitKind int

const (
	hitNothing hitKind = iota
	hitRail            // the loop rail; loopIdx >= 0 on a loop's row
	hitDetail          // the detail pane body (a wheel-scroll target)
	hitTab             // one of the nav bar's view names
)

type hitTarget struct {
	kind    hitKind
	loopIdx int   // index into s.loops when kind == hitRail, else -1
	tab     tabID // valid when kind == hitTab
}

// hitTest reports what sits under a zero-based screen coordinate.
func hitTest(s frameState, x, y int) hitTarget {
	none := hitTarget{kind: hitNothing, loopIdx: -1}
	if s.width < minWidth || s.height < minHeight {
		return none
	}
	// Content rows start under the header and rule (plus the roomy blank).
	top := 2
	if s.roomy() {
		top = 3
	}
	row := y - top
	if row < 0 || row >= s.contentRows() {
		return none
	}
	gutterW := 0
	if s.roomy() {
		gutterW = 2
	}
	railW, _ := s.railArea()
	if railW > 0 && x >= gutterW && x < gutterW+railW {
		rows := railRows(s, railW, s.contentRows())
		idx := -1
		if row < len(rows) {
			idx = rows[row].idx
		}
		return hitTarget{kind: hitRail, loopIdx: idx}
	}
	// The detail pane starts past the separator and its space (or just the
	// gutter when the rail is collapsed) — renderFrame's exact spelling.
	detailStart := 1
	switch {
	case railW > 0:
		detailStart = gutterW + railW + 2
	case gutterW > 0:
		detailStart = gutterW
	}
	if x < detailStart {
		return none
	}
	if row == navBarRow && detailShowsLoop(s) {
		if t, ok := navTabAt(s, x-detailStart); ok {
			return hitTarget{kind: hitTab, loopIdx: -1, tab: t}
		}
	}
	return hitTarget{kind: hitDetail, loopIdx: -1}
}

// navBarRow is the nav bar's position inside the detail pane: title, goal,
// agent, activity, spacer, then the nav (detailFixedRows keeps the same
// count for scrolling).
const navBarRow = 5

// detailShowsLoop mirrors detailLines' branching: the loop detail (and so
// the nav bar) renders only when a loop is selected and nothing covers it.
func detailShowsLoop(s frameState) bool {
	return s.loadErr == "" && !s.form.active && !s.showHelp &&
		s.selected >= 0 && s.selected < len(s.loops)
}

// navTabAt maps an x offset inside the detail pane to the nav tab under it,
// mirroring navBar's spelling: names three spaces apart, the active one
// behind a "▸ " marker.
func navTabAt(s frameState, x int) (tabID, bool) {
	pos := 0
	for i, name := range tabNames {
		if i > 0 {
			pos += 3
		}
		w := loop.DisplayWidth(name)
		if tabID(i) == s.tab {
			w += 2 // the "▸ " marker
		}
		if x >= pos && x < pos+w {
			return tabID(i), true
		}
		pos += w
	}
	return 0, false
}
