package tui

import "testing"

// TestHitTestWideGeometry holds the hit-test to renderFrame's roomy layout:
// content starts at row 3 behind a two-column gutter, the rail rows map to
// loop indices (gap rows to none), and the nav names switch views.
func TestHitTestWideGeometry(t *testing.T) {
	s := wideState() // 120x36: roomy, rail visible
	railW, _ := s.railArea()
	detailX := 2 + railW + 2

	// Rail rows: live loop, group gap, parked loop.
	if got := hitTest(s, 4, 3); got.kind != hitRail || got.loopIdx != 0 {
		t.Errorf("rail row 0 = %+v, want loop 0", got)
	}
	if got := hitTest(s, 4, 4); got.kind != hitRail || got.loopIdx != -1 {
		t.Errorf("the group gap = %+v, want rail with no loop", got)
	}
	if got := hitTest(s, 4, 5); got.kind != hitRail || got.loopIdx != 1 {
		t.Errorf("rail row 2 = %+v, want loop 1", got)
	}
	// Below the rail's rows: still the rail (a wheel target), no loop.
	if got := hitTest(s, 4, 20); got.kind != hitRail || got.loopIdx != -1 {
		t.Errorf("empty rail area = %+v", got)
	}

	// The nav bar is detail row 5 (y=8): "▸ overview   live   diff   verifier".
	if got := hitTest(s, detailX, 8); got.kind != hitTab || got.tab != tabOverview {
		t.Errorf("nav start = %+v, want overview", got)
	}
	if got := hitTest(s, detailX+13, 8); got.kind != hitTab || got.tab != tabLive {
		t.Errorf("nav live = %+v", got)
	}
	if got := hitTest(s, detailX+20, 8); got.kind != hitTab || got.tab != tabDiff {
		t.Errorf("nav diff = %+v", got)
	}
	if got := hitTest(s, detailX+27, 8); got.kind != hitTab || got.tab != tabVerifier {
		t.Errorf("nav verifier = %+v", got)
	}
	// Past the last name: plain detail.
	if got := hitTest(s, detailX+60, 8); got.kind != hitDetail {
		t.Errorf("past the nav = %+v, want detail", got)
	}

	// Body rows are detail; chrome rows and the separator column are nothing.
	if got := hitTest(s, detailX+5, 12); got.kind != hitDetail {
		t.Errorf("detail body = %+v", got)
	}
	for _, pt := range [][2]int{{10, 0}, {10, 1}, {10, 2}, {10, 35}, {2 + railW, 7}, {2 + railW + 1, 7}, {0, 7}} {
		if got := hitTest(s, pt[0], pt[1]); got.kind != hitNothing {
			t.Errorf("chrome at (%d,%d) = %+v, want nothing", pt[0], pt[1], got)
		}
	}
}

// TestHitTestDenseNoRail: below the collapse width the rail is gone — the
// whole pane is detail, the nav sits at row 7, and column 0 is the margin.
func TestHitTestDenseNoRail(t *testing.T) {
	s := wideState()
	s.width, s.height = 60, 18 // not roomy, rail collapsed

	if got := hitTest(s, 0, 7); got.kind != hitNothing {
		t.Errorf("the margin column = %+v, want nothing", got)
	}
	if got := hitTest(s, 1, 7); got.kind != hitTab || got.tab != tabOverview {
		t.Errorf("dense nav = %+v, want overview", got)
	}
	if got := hitTest(s, 10, 10); got.kind != hitDetail {
		t.Errorf("dense body = %+v, want detail", got)
	}
}

// TestHitTestNavNeedsTheLoopDetail: when the detail pane shows something
// else — help, the wizard, a load error, the quiet rail — there is no nav
// bar to click.
func TestHitTestNavNeedsTheLoopDetail(t *testing.T) {
	base := wideState()
	railW, _ := base.railArea()
	x, y := 2+railW+2, 8

	if got := hitTest(base, x, y); got.kind != hitTab {
		t.Fatalf("baseline should hit the nav, got %+v", got)
	}
	for name, mod := range map[string]func(*frameState){
		"help":      func(s *frameState) { s.showHelp = true },
		"wizard":    func(s *frameState) { s.form.active = true },
		"loadErr":   func(s *frameState) { s.loadErr = "boom" },
		"quietRail": func(s *frameState) { s.selected = -1 },
	} {
		s := wideState()
		mod(&s)
		if got := hitTest(s, x, y); got.kind == hitTab {
			t.Errorf("%s: the nav should not be clickable, got %+v", name, got)
		}
	}
}

func TestHitTestTinyTerminal(t *testing.T) {
	if got := hitTest(frameState{width: 30, height: 5}, 2, 2); got.kind != hitNothing {
		t.Errorf("tiny terminal = %+v, want nothing", got)
	}
}
