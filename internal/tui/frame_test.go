package tui

import (
	"strings"
	"testing"

	"github.com/mjbarefo/loopy/internal/loop"
)

func exitCode(n int) *int { return &n }

func sampleLoops() []loop.LoopView {
	return []loop.LoopView{
		{
			ID: "fix-csv-quoting", Goal: "make the CSV importer handle quoted newlines",
			Agent: "claude", Status: loop.StatusRunning, Live: true,
			IterationsUsed: 2, MaxIterations: 8,
			WallClockUsed: "4m10s", MaxWallClock: "30m",
			Verifier: []loop.Stage{{Name: "test", Cmd: "go test ./..."}},
			Iterations: []loop.IterationView{
				{Index: 0, Baseline: true, FailingStage: "test"},
				{Index: 1, FailingStage: "test", AgentExit: exitCode(0), VerifyMS: 1200, DiffBytes: 800, FilesChanged: 2},
				{Index: 2, Green: true, AgentExit: exitCode(0), VerifyMS: 900, DiffBytes: 1600, FilesChanged: 3},
			},
			NextCommand: "loopy watch fix-csv-quoting",
		},
		{
			ID: "flaky-importer", Goal: "fix the flaky importer test",
			Agent: "claude", Status: loop.StatusParked, ParkedReason: "stuck: no change",
			IterationsUsed: 3, MaxIterations: 8,
			WallClockUsed: "9m", MaxWallClock: "30m",
			NextCommand: "loopy log flaky-importer",
		},
	}
}

func wideState() frameState {
	return frameState{
		width: 120, height: 36, loops: sampleLoops(), selected: 0, tab: tabIterations, scroll: -1,
	}
}

// visibleWidth counts runes, which is what the layout promises: every line
// exactly as wide as the terminal.
func checkFrameGeometry(t *testing.T, frame string, width, height int) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
	if len(lines) != height {
		t.Fatalf("frame has %d lines, want %d", len(lines), height)
	}
	for i, line := range lines {
		if got := len([]rune(line)); got != width {
			t.Errorf("line %d is %d columns, want %d: %q", i, got, width, line)
		}
	}
}

func TestFrameWideGeometry(t *testing.T) {
	s := wideState()
	checkFrameGeometry(t, renderFrame(s), 120, 36)
}

func TestFrameNarrowCollapsesList(t *testing.T) {
	s := wideState()
	s.width, s.height = 60, 24
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 60, 24)
	if strings.Contains(frame, "loops ─") {
		t.Error("narrow frame should not render the loop list pane")
	}
	if !strings.Contains(frame, "fix-csv-quoting") {
		t.Error("narrow frame should still show the selected loop")
	}
}

func TestFrameColorOffHasNoANSI(t *testing.T) {
	s := wideState()
	s.color = false
	if frame := renderFrame(s); strings.Contains(frame, "\x1b[") {
		t.Error("color-off frame contains ANSI escapes")
	}
}

func TestFrameColorOnKeepsGlyphs(t *testing.T) {
	s := wideState()
	s.color = true
	frame := renderFrame(s)
	if !strings.Contains(frame, "\x1b[") {
		t.Error("color-on frame has no ANSI escapes")
	}
	// Color is never the only signal: glyphs and words survive.
	for _, want := range []string{"●", "✗", "✓ green", "running"} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame missing non-color signal %q", want)
		}
	}
}

func TestFrameShowsIterationTimeline(t *testing.T) {
	frame := renderFrame(wideState())
	for _, want := range []string{"base", "✗ test", "✓ green", "● running…", "next: loopy watch fix-csv-quoting"} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame missing %q", want)
		}
	}
}

func TestFrameTruncationBanner(t *testing.T) {
	s := wideState()
	s.tab = tabDiff
	s.art = artifact{
		label: "iter 2 · diff.patch", truncated: true, size: 1 << 20,
		lines: []string{"+added line", "-removed line"},
	}
	frame := renderFrame(s)
	if !strings.Contains(frame, "truncated: showing last") || !strings.Contains(frame, "1.0 MiB") {
		t.Errorf("truncated artifact needs a banner, got:\n%s", frame)
	}
}

func TestFrameAbortConfirmInFooter(t *testing.T) {
	s := wideState()
	s.confirmAbort = true
	if frame := renderFrame(s); !strings.Contains(frame, "abort fix-csv-quoting? y to confirm") {
		t.Error("footer should ask for abort confirmation")
	}
}

func TestFrameNoLoops(t *testing.T) {
	s := frameState{width: 100, height: 20}
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 100, 20)
	if !strings.Contains(frame, "no loops yet") {
		t.Error("empty state should say so")
	}
}

func TestFrameTooSmall(t *testing.T) {
	if frame := renderFrame(frameState{width: 30, height: 5}); !strings.Contains(frame, "too small") {
		t.Error("tiny terminals get a plain message")
	}
}

func TestFrameRunningStaleIsAlarming(t *testing.T) {
	s := wideState()
	s.loops[0].Live = false
	frame := renderFrame(s)
	if !strings.Contains(frame, "running (no engine)") {
		t.Error("a running loop without an engine must be visibly wrong")
	}
}

func TestWindowFollowsTail(t *testing.T) {
	lines := []cell{plainCell("1"), plainCell("2"), plainCell("3"), plainCell("4")}
	got := window(lines, 2, -1)
	if len(got) != 2 || got[0].plain != "3" {
		t.Fatalf("follow-tail window = %v", got)
	}
	got = window(lines, 2, 0)
	if got[0].plain != "1" {
		t.Fatalf("scroll-to-top window = %v", got)
	}
}
