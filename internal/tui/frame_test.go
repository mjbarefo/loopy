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
			Phase: loop.PhaseAgent, PhaseIteration: 3, PhaseStartedAt: "2026-06-11T10:00:00Z",
			IterationsUsed: 2, MaxIterations: 8,
			WallClockUsed: "4m10s", MaxWallClock: "30m",
			Verifier: []loop.Stage{{Name: "vet", Cmd: "go vet ./..."}, {Name: "test", Cmd: "go test ./..."}},
			Iterations: []loop.IterationView{
				{Index: 0, Baseline: true, FailingStage: "test", StagesPassed: 1, StagesTotal: 2},
				{Index: 1, FailingStage: "test", AgentExit: exitCode(0), AgentMS: 92000, VerifyMS: 1200, DiffBytes: 800, FilesChanged: 2, StagesPassed: 1, StagesTotal: 2},
				{Index: 2, Green: true, AgentExit: exitCode(0), AgentMS: 61000, VerifyMS: 900, DiffBytes: 1600, FilesChanged: 3, StagesPassed: 2, StagesTotal: 2},
			},
			LastFeedback: "importer_test.go:88 TestQuotedNewlines failed",
			NextCommand:  "loopy watch fix-csv-quoting",
		},
		{
			ID: "flaky-importer", Goal: "fix the flaky importer test",
			Agent: "claude", Status: loop.StatusParked, ParkedReason: "stuck: no change",
			IterationsUsed: 3, MaxIterations: 8,
			WallClockUsed: "9m", MaxWallClock: "30m",
			NextCommand: "loopy review flaky-importer",
		},
	}
}

func wideState() frameState {
	return frameState{
		width: 120, height: 36, loops: sampleLoops(), selected: 0, tab: tabOverview, scroll: -1,
	}
}

// checkFrameGeometry asserts every line is exactly as wide as the terminal,
// in display columns (CJK and emoji count two).
func checkFrameGeometry(t *testing.T, frame string, width, height int) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
	if len(lines) != height {
		t.Fatalf("frame has %d lines, want %d", len(lines), height)
	}
	for i, line := range lines {
		if got := loop.DisplayWidth(line); got != width {
			t.Errorf("line %d is %d columns, want %d: %q", i, got, width, line)
		}
	}
}

func TestFrameWideGeometry(t *testing.T) {
	checkFrameGeometry(t, renderFrame(wideState()), 120, 36)
}

func TestFrameNarrowCollapsesRail(t *testing.T) {
	s := wideState()
	s.width, s.height = 60, 24
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 60, 24)
	if strings.Contains(frame, "│") {
		t.Error("narrow frame should not render the rail separator")
	}
	if !strings.Contains(frame, "fix-csv-quoting") {
		t.Error("narrow frame should still show the selected loop")
	}
}

func TestFrameWideRunesKeepGeometry(t *testing.T) {
	s := wideState()
	s.loops[0].Goal = "引用符付き改行を正しく処理する 🌀 and never regress the Windows path handling in the importer pipeline"
	s.loops[0].ID = "处理引用符-handle-quoting"
	s.selected = 0
	checkFrameGeometry(t, renderFrame(s), 120, 36)
	s.width, s.height = 60, 20
	checkFrameGeometry(t, renderFrame(s), 60, 20)
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

// TestFrameIdentityAccents pins the visual identity: the ∞ lockup in the
// header, dim chrome, cyan-bold active tab (no inverse video), and status
// color in exactly one place per row — the rail glyph and the verdict cell,
// never the whole line.
func TestFrameIdentityAccents(t *testing.T) {
	s := wideState()
	s.color = true
	frame := renderFrame(s)

	if !strings.Contains(frame, "\x1b[36m ∞ \x1b[0m\x1b[1mloopy\x1b[0m") {
		t.Error("header missing the cyan ∞ mark + bold wordmark lockup")
	}
	if strings.Contains(frame, "\x1b[7m") {
		t.Error("inverse video crept back in; the active tab is cyan-bold")
	}
	if !strings.Contains(frame, "\x1b[1;36m[overview]\x1b[0m") {
		t.Error("active tab should be cyan-bold")
	}
	if !strings.Contains(frame, "\x1b[2m"+rule(8)) {
		t.Error("rules should be dim chrome")
	}
	// Selected rail row: cyan cursor, colored glyph, bold ID — three cells,
	// not one painted line.
	if !strings.Contains(frame, "\x1b[36m▶ \x1b[0m") {
		t.Error("selection cursor should be its own cyan cell")
	}
	if !strings.Contains(frame, "\x1b[1mfix-csv-quoting") {
		t.Error("selected rail ID should be bold, outside the status color")
	}
	// Unselected parked row: red glyph, plain ID.
	if !strings.Contains(frame, "\x1b[31m✗\x1b[0m flaky-importer") {
		t.Error("rail status color belongs on the glyph only")
	}
	// Timeline rows: the verdict cell carries the color, the metrics do not.
	if !strings.Contains(frame, "\x1b[32m✓ green") {
		t.Error("green verdict cell should be green")
	}
	if !strings.Contains(frame, "\x1b[0m 1m1s") {
		t.Error("iteration metrics should sit outside the verdict's color span")
	}
}

func TestFrameOverviewAnswersTheQuestions(t *testing.T) {
	frame := renderFrame(wideState())
	for _, want := range []string{
		"1 running",                   // header: what is here
		"✗ test (1/2)",                // timeline + convergence signal
		"✓ green",                     // verdicts
		"now: agent running · iter 3", // what is it doing right now
		"last feedback tail:",         // why is it red
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame missing %q\n%s", want, frame)
		}
	}
}

func TestFrameRunningLoopHasNoUselessNext(t *testing.T) {
	// Inside the monitor, "next: loopy watch <id>" for the watched loop is
	// circular; the footer omits it (only --once keeps it, for scripts).
	frame := renderFrame(wideState())
	if strings.Contains(frame, "next: loopy watch") {
		t.Errorf("running loop footer should not point back at the monitor\n%s", frame)
	}
	s := wideState()
	s.selected = 1 // parked loop
	if !strings.Contains(renderFrame(s), "next: loopy review flaky-importer") {
		t.Error("parked loop footer should carry the review command")
	}
}

func TestFrameParkedReasonIsTheActivityLine(t *testing.T) {
	s := wideState()
	s.selected = 1
	if !strings.Contains(renderFrame(s), "✗ stuck: no change") {
		t.Error("parked loop should lead with why it stopped")
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

func TestFrameNoLoopsOnboarding(t *testing.T) {
	// Uninitialized repo: the first step is executable in place.
	s := frameState{width: 100, height: 24}
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 100, 24)
	for _, want := range []string{"no loops yet", "press i", "engineer loops, not prompts"} {
		if !strings.Contains(frame, want) {
			t.Errorf("onboarding missing %q\n%s", want, frame)
		}
	}

	// Initialized with detected agent CLIs: digits register them.
	s.initialized = true
	s.detected = []loop.AgentSuggestion{{Binary: "claude", Name: "claude", Cmd: "claude -p {prompt}"}}
	frame = renderFrame(s)
	for _, want := range []string{"initialize the repo ✓", "press 1", "claude"} {
		if !strings.Contains(frame, want) {
			t.Errorf("onboarding missing %q\n%s", want, frame)
		}
	}

	// Fully set up: n starts the first loop.
	s.agentsRegistered = true
	s.detected = nil
	frame = renderFrame(s)
	for _, want := range []string{"register an agent ✓", "press n"} {
		if !strings.Contains(frame, want) {
			t.Errorf("onboarding missing %q\n%s", want, frame)
		}
	}

	// Short terminals drop the mascot, never the checklist.
	s.height = 12
	frame = renderFrame(s)
	checkFrameGeometry(t, frame, 100, 12)
	if strings.Contains(frame, "██") {
		t.Error("mascot must yield to information on short terminals")
	}
}

func TestWelcomeFrame(t *testing.T) {
	s := frameState{width: 100, height: 24, initialized: true, agentsRegistered: true, loops: sampleLoops()}
	frame := welcomeFrame(s, "/tmp/myproject")
	lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
	if len(lines) > 24 {
		t.Fatalf("welcome frame has %d lines, must fit 24", len(lines))
	}
	for i, line := range lines {
		if w := loop.DisplayWidth(line); w > 100 {
			t.Errorf("welcome line %d is %d columns, over 100", i, w)
		}
	}
	for _, want := range []string{"l o o p y", "engineer loops, not prompts", "repo myproject", "2 loop(s)", "press any key"} {
		if !strings.Contains(frame, want) {
			t.Errorf("welcome missing %q\n%s", want, frame)
		}
	}
	s.color = false
	if strings.Contains(welcomeFrame(s, "/tmp/x"), "\x1b[") {
		t.Error("color-off welcome contains ANSI escapes")
	}
}

func TestFrontDoor(t *testing.T) {
	out := FrontDoor(false, "~")
	for _, want := range []string{
		"l o o p y",
		"engineer loops, not prompts",
		"~ is not a git repository",
		"cd into the repo you want loops in, then run: loopy",
		"git init first",
		"loopy help",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("front door missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("color-off front door contains ANSI escapes")
	}
	if colored := FrontDoor(true, "~"); !strings.Contains(colored, "\x1b[36m") {
		t.Error("color-on front door should carry the cyan identity")
	}
}

func TestRenderPicker(t *testing.T) {
	s := pickerState{
		width: 100, height: 30, start: "/tmp/nowhere",
		repos: []loop.RepoCandidate{
			{Path: "/tmp/projects/alpha", Loops: 3},
			{Path: "/tmp/projects/beta"},
		},
		selected: 0,
	}
	frame := renderPicker(s)
	lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
	if len(lines) != 30 {
		t.Fatalf("picker frame has %d lines, want 30", len(lines))
	}
	for i, line := range lines {
		if got := loop.DisplayWidth(line); got != 100 {
			t.Errorf("picker line %d is %d columns, want 100: %q", i, got, line)
		}
	}
	for _, want := range []string{
		"engineer loops, not prompts",
		"/tmp/nowhere is not a git repository",
		"pick where loops should live:",
		"▶ /tmp/projects/alpha",
		"3 loop(s)",
		"/tmp/projects/beta",
		"enter opens the monitor in /tmp/projects/alpha",
		"g git init here",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("picker missing %q\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "\x1b[") {
		t.Error("color-off picker contains ANSI escapes")
	}

	// Short terminals drop the logo, never the list.
	s.height = 12
	frame = renderPicker(s)
	if strings.Contains(frame, "██") {
		t.Error("picker logo must yield to the list on short terminals")
	}
	if !strings.Contains(frame, "alpha") {
		t.Error("short picker lost the repo list")
	}

	// Selection moves the accent.
	s.height = 30
	s.selected = 1
	if frame := renderPicker(s); !strings.Contains(frame, "▶ /tmp/projects/beta") {
		t.Error("cursor did not follow the selection")
	}
}

func TestFrameNewLoopForm(t *testing.T) {
	s := wideState()
	s.form = formState{
		active: true, goal: "fix the importer",
		stages: []loop.Stage{{Name: "test", Cmd: "go test ./..."}}, stagesDesc: "go test ./...",
		inferSource: "go.mod", agent: "claude",
	}
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 120, 36)
	for _, want := range []string{"start a loop", "fix the importer", "go test ./...", "inferred from go.mod", "enter start", "esc cancel"} {
		if !strings.Contains(frame, want) {
			t.Errorf("form missing %q\n%s", want, frame)
		}
	}
	// A blocked form says why and how to proceed instead.
	s.form = formState{active: true, blocked: `no verifier configured or inferable — start with: loopy run "<goal>" --verify "<cmd>"`}
	if !strings.Contains(renderFrame(s), "no verifier configured") {
		t.Error("blocked form must say why")
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
	if !strings.Contains(frame, "no engine holds this loop") {
		t.Error("the activity line must say what is wrong and what to do")
	}
}

func TestFrameBrokenLoopVisible(t *testing.T) {
	s := wideState()
	s.broken = []loop.BrokenLoop{{ID: "mangled-loop", Path: "x/loop.json", Err: "invalid character"}}
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 120, 36)
	if !strings.Contains(frame, "mangled-loop (unreadable)") {
		t.Errorf("broken loop must appear in the rail\n%s", frame)
	}
	if !strings.Contains(frame, "1 unreadable") {
		t.Error("broken count belongs in the header")
	}
}

func TestFooterDropsWholeKeyHints(t *testing.T) {
	s := wideState()
	s.selected = 1 // parked: footer carries next: loopy review flaky-importer
	s.width = 64
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 64, 36)
	if !strings.Contains(frame, "next: loopy review flaky-importer") {
		t.Errorf("the next command always wins the space fight\n%s", frame)
	}
	// No key hint is ever cut mid-word: every hint present is complete.
	footer := strings.Split(strings.TrimRight(frame, "\n"), "\n")
	last := footer[len(footer)-1]
	for _, hint := range []string{"↑↓ loop", "enter drill", "tab view", "p pause", "r resume", "a abort", "? help", "q quit"} {
		for _, word := range strings.Fields(hint) {
			if i := strings.Index(last, word[:1]); i >= 0 {
				continue // presence is fine; we only forbid partial words below
			}
		}
	}
	for _, partial := range []string{"dril…", "pa…", "res…", "abo…", "h…"} {
		if strings.Contains(last, partial) {
			t.Errorf("footer cut a key hint mid-word: %q", last)
		}
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
