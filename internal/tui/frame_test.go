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
// header, dim chrome, the ▸+cyan nav (no brackets, no inverse video), and
// status color in exactly one place per row — the rail glyph and the verdict
// cell, never the whole line.
func TestFrameIdentityAccents(t *testing.T) {
	s := wideState()
	s.color = true
	frame := renderFrame(s)

	if !strings.Contains(frame, "\x1b[36m ∞ \x1b[0m\x1b[1mloopy\x1b[0m") {
		t.Error("header missing the cyan ∞ mark + bold wordmark lockup")
	}
	if strings.Contains(frame, "\x1b[7m") {
		t.Error("inverse video crept back in; the active nav item is ▸ + cyan")
	}
	if !strings.Contains(frame, "\x1b[36m▸ overview\x1b[0m") {
		t.Error("active nav item should be ▸ + cyan")
	}
	if strings.Contains(frame, "[overview]") {
		t.Error("bracketed tabs are retired; the nav marks the active view with ▸")
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

// TestFrameDiffTabAnswersFirst: the diff tab opens with what changed in plain
// words — a file count, a per-file list — then the patch, styled per line.
func TestFrameDiffTabAnswersFirst(t *testing.T) {
	s := wideState()
	s.tab = tabDiff
	s.art = artifact{label: "iter 2 · diff.patch", iter: 2, lines: []string{
		"diff --git a/docs/fable/README.md b/docs/fable/README.md",
		"new file mode 100644",
		"index 0000000..52ad8d6",
		"--- /dev/null",
		"+++ b/docs/fable/README.md",
		"@@ -0,0 +1,3 @@",
		"+# fable",
		"+",
		"+a readme written by a loop",
	}}
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 120, 36)
	for _, want := range []string{
		"1 file changed · +3",
		"+ docs/fable/README.md",
		"new file · +3",
		"iter 2 · diff.patch",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("diff tab missing %q\n%s", want, frame)
		}
	}

	s.color = true
	frame = renderFrame(s)
	for _, want := range []string{
		"\x1b[1m 1 file changed · +3\x1b[0m", // the answer leads, bold
		"\x1b[1m diff --git",                 // file headers bold
		"\x1b[2;36m @@ -0,0 +1,3 @@\x1b[0m",  // hunk markers dim cyan
		"\x1b[32m +# fable\x1b[0m",           // adds green
		"\x1b[1m --- /dev/null\x1b[0m",       // old-side header bold, not red
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("styled diff missing %q\n%s", want, frame)
		}
	}
}

// TestFrameVerifierTabScoreboard: the verifier tab opens with a per-stage
// scoreboard and a plain-words verdict; in the log, markers are dim dividers
// and on a red run the passing stages' output dims so the failure reads.
func TestFrameVerifierTabScoreboard(t *testing.T) {
	s := wideState()
	s.tab = tabVerifier
	s.loops[0].Verifier = append(s.loops[0].Verifier, loop.Stage{Name: "build", Cmd: "go build ./..."})
	s.loops[0].Iterations[1].Stages = []loop.StageResult{
		{Name: "vet", Cmd: "go vet ./...", ExitCode: 0, DurationMS: 300},
		{Name: "test", Cmd: "go test ./...", ExitCode: 1, DurationMS: 900},
	}
	s.art = artifact{label: "iter 1 · verifier.log", iter: 1, lines: []string{
		"=== stage vet: go vet ./...",
		"vet chatter",
		"=== stage vet: exit 0 (300ms)",
		"=== stage test: go test ./...",
		"--- FAIL: TestQuotedNewlines (0.00s)",
		"=== stage test: exit 1 (900ms)",
	}}
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 120, 36)
	for _, want := range []string{
		"✓ vet",
		"✗ test",
		"300ms",
		"· build  go build ./...  not yet run",
		"red — test failed; the log below shows why",
		"--- FAIL: TestQuotedNewlines",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("verifier tab missing %q\n%s", want, frame)
		}
	}

	s.color = true
	frame = renderFrame(s)
	if !strings.Contains(frame, "\x1b[2m === stage vet: go vet ./...\x1b[0m") {
		t.Error("stage markers should be dim dividers")
	}
	if !strings.Contains(frame, "\x1b[2m vet chatter\x1b[0m") {
		t.Error("a passing stage's output should dim on a red run")
	}
	if strings.Contains(frame, "\x1b[2m --- FAIL") {
		t.Error("the failing stage's output must stay bright")
	}
}

// TestFrameVerifierTabAskStage: an ask stage in the scoreboard wears an "ask"
// tag (a word, not just color) and shows its question, so the human sees which
// greens are a mechanical check and which are the agent's judgment.
func TestFrameVerifierTabAskStage(t *testing.T) {
	s := wideState()
	s.tab = tabVerifier
	s.loops[0].Verifier = append(s.loops[0].Verifier,
		loop.Stage{Name: "judge", Kind: loop.KindAsk, Ask: "does the importer handle quoted newlines?"})
	s.loops[0].Iterations[2].Stages = []loop.StageResult{
		{Name: "vet", Cmd: "go vet ./...", ExitCode: 0, DurationMS: 300},
		{Name: "test", Cmd: "go test ./...", ExitCode: 0, DurationMS: 900},
		{Name: "judge", Kind: loop.KindAsk, Cmd: "does the importer handle quoted newlines?", ExitCode: 0, DurationMS: 4200},
	}
	s.art = artifact{label: "iter 2 · verifier.log", iter: 2, lines: []string{
		"=== stage judge (ask claude): does the importer handle quoted newlines?",
		"=== ask verdict: PASS",
	}}
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 120, 36)
	for _, want := range []string{"✓ judge", "asks claude: does the importer handle quoted newlines?", "green — the goal is met"} {
		if !strings.Contains(frame, want) {
			t.Errorf("ask-stage scoreboard missing %q\n%s", want, frame)
		}
	}
}

// TestFrameVerifierTabCommandNotFound: a command stage that exits 127 is almost
// always prose typed into the checks field, not a real test — the verdict points
// at the fix (clear it, let the agent judge) instead of a generic failure.
func TestFrameVerifierTabCommandNotFound(t *testing.T) {
	s := wideState()
	s.tab = tabVerifier
	s.loops[0].Verifier = []loop.Stage{{Name: "verify", Cmd: "agents.md in repo root"}}
	s.loops[0].Iterations[1].Stages = []loop.StageResult{
		{Name: "verify", Cmd: "agents.md in repo root", ExitCode: 127, DurationMS: 16},
	}
	s.loops[0].Iterations[1].FailingStage = "verify"
	s.art = artifact{label: "iter 1 · verifier.log", iter: 1, lines: []string{
		"=== stage verify: agents.md in repo root",
		"sh: agents.md: command not found",
	}}
	frame := renderFrame(s)
	for _, want := range []string{"not a runnable command (exit 127)", "let the agent judge"} {
		if !strings.Contains(frame, want) {
			t.Errorf("a 127 verdict should point at the fix, missing %q\n%s", want, frame)
		}
	}
}

// TestFrameVerifierTabGreen: a green run says so in plain words and keeps the
// whole log bright — only the markers recede.
func TestFrameVerifierTabGreen(t *testing.T) {
	s := wideState()
	s.tab = tabVerifier
	s.loops[0].Iterations[2].Stages = []loop.StageResult{
		{Name: "vet", Cmd: "go vet ./...", ExitCode: 0, DurationMS: 250},
		{Name: "test", Cmd: "go test ./...", ExitCode: 0, DurationMS: 1100},
	}
	s.art = artifact{label: "iter 2 · verifier.log", iter: 2, lines: []string{
		"=== stage test: go test ./...",
		"ok  	github.com/example/pkg	0.4s",
	}}
	frame := renderFrame(s)
	if !strings.Contains(frame, "green — the goal is met") {
		t.Errorf("green verifier tab needs its verdict\n%s", frame)
	}
	s.color = true
	frame = renderFrame(s)
	if strings.Contains(frame, "\x1b[2m ok  ") {
		t.Error("a green run's output should not dim")
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

func TestFrameDecisionConfirmsInFooter(t *testing.T) {
	s := wideState()
	s.confirmAccept = true
	if frame := renderFrame(s); !strings.Contains(frame, "accept fix-csv-quoting? the decision is recorded") {
		t.Error("footer should ask for accept confirmation")
	}
	s.confirmAccept = false
	s.confirmReject = true
	if frame := renderFrame(s); !strings.Contains(frame, "reject fix-csv-quoting? evidence kept, worktree freed") {
		t.Error("footer should ask for reject confirmation")
	}
}

// TestFrameQuietRail: every loop decided and nothing selected — the detail
// pane says so and keeps the newest accepted loop's apply command visible.
func TestFrameQuietRail(t *testing.T) {
	s := wideState()
	for i := range s.loops {
		s.loops[i].Status = loop.StatusAccepted
	}
	s.loops[1].EndedAt = "2026-06-12T20:00:00Z"
	s.loops[1].NextCommand = "git apply .loopy/loops/flaky-importer/final-diff.patch"
	s.selected = -1
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 120, 36)
	for _, want := range []string{
		"all quiet",
		"starts the next loop",
		"flaky-importer was accepted",
		"git apply .loopy/loops/flaky-importer/final-diff.patch",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("quiet rail missing %q\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "no loops yet") {
		t.Error("a quiet rail is not the onboarding empty state")
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

func TestRenderPickerScanStates(t *testing.T) {
	s := pickerState{width: 100, height: 30, start: "/tmp/nowhere", scanning: true}
	frame := renderPicker(s)
	for _, want := range []string{
		"engineer loops, not prompts",
		"/tmp/nowhere is not a git repository",
		"scanning /tmp/nowhere for repositories…",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("scanning picker missing %q\n%s", want, frame)
		}
	}

	// Scan done, nothing found: the guidance replaces the dead end.
	s.scanning = false
	frame = renderPicker(s)
	for _, want := range []string{
		"no git repositories found under /tmp/nowhere",
		"cd into the repo you want loops in, then run: loopy",
		"press g — git init right here",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("empty picker missing %q\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "could not read") {
		t.Error("no denied dirs, no privacy hint")
	}

	// Unreadable dirs get the macOS privacy hint.
	s.denied = []string{"Documents", "Desktop"}
	frame = renderPicker(s)
	for _, want := range []string{
		"could not read: Documents, Desktop",
		"Privacy & Security",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("denied picker missing %q\n%s", want, frame)
		}
	}

	// A scan still running behind found repos says so.
	s = pickerState{
		width: 100, height: 30, start: "/tmp/nowhere", scanning: true,
		repos: []loop.RepoCandidate{{Path: "/tmp/projects/alpha"}},
	}
	if frame := renderPicker(s); !strings.Contains(frame, "…still scanning") {
		t.Errorf("streaming picker should admit the scan is still running\n%s", frame)
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
		"pick a project to run loops in:",
		"loop state lives inside the repo it works on, under .loopy/",
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

func TestFrameNewLoopWizard(t *testing.T) {
	base := formState{
		active: true, goal: "fix the importer",
		agents: []string{"claude", "codex"}, defaultAgent: "claude",
		picked:        map[int]bool{},
		prefillStages: []loop.Stage{{Name: "test", Cmd: "go test ./..."}},
		verifier:      "go test ./...", inferSource: "go.mod",
		iters: "8", wall: "30m",
	}

	// Step 1: the goal, with the input and a plain-words hint.
	s := wideState()
	s.form = base
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 120, 36)
	for _, want := range []string{"start a loop", "step 1 of 5", "fix the importer", "describe what done looks like", "enter continues"} {
		if !strings.Contains(frame, want) {
			t.Errorf("goal step missing %q\n%s", want, frame)
		}
	}

	// Step 2: agents, default labeled, race marking explained.
	s.form.step = stepAgent
	s.form.picked = map[int]bool{0: true, 1: true}
	frame = renderFrame(s)
	for _, want := range []string{"step 2 of 5", "claude", "(default)", "space marks more than one to race", "enter continues with claude + codex"} {
		if !strings.Contains(frame, want) {
			t.Errorf("agent step missing %q\n%s", want, frame)
		}
	}

	// Step 3: the hybrid verifier — command gates plus an ask question, both
	// editable, composed instantly (no blocking synthesis on arrival).
	s.form = base
	s.form.step = stepVerifier
	s.form.ask = "is the importer fixed and tests green?"
	frame = renderFrame(s)
	for _, want := range []string{
		"checks  go test ./...", "ask     is the importer fixed",
		"claude judges this in plain English", "the agent judges", "↑↓ switches",
		"tab asks claude to design the checks",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("hybrid verifier step missing %q\n%s", want, frame)
		}
	}

	// tab is the optional polish: it still asks the agent to design the gates,
	// and while it works the step says so.
	s.form = base
	s.form.step = stepVerifier
	s.form.synthesizing = true
	s.synthElapsed = "12s"
	frame = renderFrame(s)
	for _, want := range []string{"is designing the verifier for your goal", "throwaway worktree"} {
		if !strings.Contains(frame, want) {
			t.Errorf("synthesizing verifier step missing %q\n%s", want, frame)
		}
	}

	// A landed proposal is attributed in the checks field.
	s.form = base
	s.form.step = stepVerifier
	s.form.proposedBy = "claude"
	s.form.edited = true
	s.form.verifier = "test -f AGENTS.md && make check"
	if !strings.Contains(renderFrame(s), "designed by claude for this goal") {
		t.Error("a landed proposal should be attributed to its agent")
	}

	// No proposal yet: the gates are the inferred fallback, tab can tighten them.
	s.form = base
	s.form.step = stepVerifier
	if !strings.Contains(renderFrame(s), "inferred from go.mod") {
		t.Error("an inferred verifier should name its source")
	}

	// Step 4: budget, hard caps named.
	s.form = base
	s.form.step = stepBudget
	frame = renderFrame(s)
	for _, want := range []string{"iterations  8", "wall clock  30m", "hard caps"} {
		if !strings.Contains(frame, want) {
			t.Errorf("budget step missing %q\n%s", want, frame)
		}
	}

	// Step 5: the summary and the start action.
	s.form.step = stepConfirm
	frame = renderFrame(s)
	for _, want := range []string{"goal      fix the importer", "agent     claude", "verifier  go test ./...", "8 iterations · 30m", "enter starts the loop in its own worktree"} {
		if !strings.Contains(frame, want) {
			t.Errorf("confirm step missing %q\n%s", want, frame)
		}
	}
	s.form.picked = map[int]bool{0: true, 1: true}
	if !strings.Contains(renderFrame(s), "enter races 2 agents") {
		t.Error("a multi-agent confirm must say it races")
	}

	// A hybrid confirm lists both parts — the command verbatim and the ask
	// stage as the judgment it is.
	s.form = base
	s.form.step = stepConfirm
	s.form.ask = "is the importer fixed?"
	if frame := renderFrame(s); !strings.Contains(frame, "go test ./...") || !strings.Contains(frame, "ask: is the importer fixed?") {
		t.Errorf("hybrid confirm must show both the command and the ask stage\n%s", frame)
	}

	// No agents registered, none detected: the step says how to proceed.
	s.form = formState{active: true, step: stepAgent, picked: map[int]bool{}}
	if !strings.Contains(renderFrame(s), "no agent CLIs registered or found") {
		t.Error("the agent step must say why it is stuck and what to do")
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

func footerLine(t *testing.T, frame string) string {
	t.Helper()
	lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
	return lines[len(lines)-1]
}

// TestFooterNextCommandWins: when the hints and the next command cannot
// share the footer, the hints vanish entirely (they live behind ?) — never
// cut mid-word. The next command is a fact and always stays.
func TestFooterNextCommandWins(t *testing.T) {
	s := wideState()
	s.selected = 1 // parked: footer carries next: loopy review flaky-importer
	s.width = 48
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 48, 36)
	if !strings.Contains(frame, "next: loopy review flaky-importer") {
		t.Errorf("the next command always wins the space fight\n%s", frame)
	}
	if strings.Contains(footerLine(t, frame), "n new") {
		t.Errorf("hints should yield entirely when the next command needs the room: %q", footerLine(t, frame))
	}

	// With room, hints and the command share the line.
	s.width = 80
	frame = renderFrame(s)
	last := footerLine(t, frame)
	if !strings.Contains(last, "n new · enter open · ? keys") || !strings.Contains(last, "next: loopy review flaky-importer") {
		t.Errorf("a roomy footer carries both hints and the next command: %q", last)
	}
}

// TestFrameFooterDiet pins the hint budget: three hints in the footer, one
// in the header, everything else behind ?.
func TestFrameFooterDiet(t *testing.T) {
	frame := renderFrame(wideState())
	last := footerLine(t, frame)
	if !strings.Contains(last, "n new · enter open · ? keys") {
		t.Errorf("footer should ship the three-hint chain, got %q", last)
	}
	for _, gone := range []string{"↑↓ loop", "tab view", "p pause", "r resume", "a abort", "q quit"} {
		if strings.Contains(last, gone) {
			t.Errorf("footer hint %q should live behind ?", gone)
		}
	}
	header := strings.SplitN(frame, "\n", 2)[0]
	if strings.Contains(header, "q quit") {
		t.Error("the header dropped q quit; ? retains it")
	}
	if !strings.Contains(header, "? help") {
		t.Error("the header keeps its single ? hint")
	}

	s := wideState()
	s.focusDetail = true
	if !strings.Contains(footerLine(t, renderFrame(s)), "esc back · ? keys") {
		t.Error("detail focus footer should be esc back · ? keys")
	}
}

// TestFrameMargins: at roomy sizes a blank row sits inside each rule and the
// content floats behind a two-column gutter; below ~80x20 the dense layout
// is byte-identical to the old one.
func TestFrameMargins(t *testing.T) {
	s := wideState() // 120x36 is roomy
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 120, 36)
	lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
	if strings.TrimSpace(lines[2]) != "" {
		t.Errorf("roomy frames keep a blank row under the header rule, got %q", lines[2])
	}
	if strings.TrimSpace(lines[len(lines)-3]) != "" {
		t.Errorf("roomy frames keep a blank row above the footer rule, got %q", lines[len(lines)-3])
	}
	if !strings.HasPrefix(lines[3], "  ▶") {
		t.Errorf("the rail floats behind a two-column gutter, got %q", lines[3])
	}

	// Short terminals spend no rows on margins.
	s.height = 18
	frame = renderFrame(s)
	checkFrameGeometry(t, frame, 120, 18)
	lines = strings.Split(strings.TrimRight(frame, "\n"), "\n")
	if !strings.HasPrefix(lines[2], "▶") {
		t.Errorf("short frames keep the dense rail at the edge, got %q", lines[2])
	}
}

// TestRailGroupGaps: the rail separates live work, loops needing the human,
// and history with a blank row — the gap is the label.
func TestRailGroupGaps(t *testing.T) {
	s := wideState() // one live loop, one parked: two groups
	railW := railWidth(s.loops, s.broken)
	lines := railLines(s, railW, 20)
	if len(lines) != 3 {
		t.Fatalf("want live row, gap, history row; got %d lines", len(lines))
	}
	if lines[1].plain != "" {
		t.Errorf("urgency groups should be separated by a blank row, got %q", lines[1].plain)
	}
	if !strings.Contains(lines[2].plain, "flaky-importer") {
		t.Errorf("history group missing its loop, got %q", lines[2].plain)
	}
}

// TestFrameColorDiet: status hues are glyph-sized. The activity line is a
// colored glyph plus plain words; the title's status phrase is plain; the
// live timeline row colors only its dot. The verdict cell (tested above)
// stays the one permitted block of status color.
func TestFrameColorDiet(t *testing.T) {
	s := wideState()
	s.color = true
	frame := renderFrame(s)
	if !strings.Contains(frame, "\x1b[36m●\x1b[0m now: agent running") {
		t.Error("running activity should be a cyan glyph + plain text")
	}
	if !strings.Contains(frame, "\x1b[2m — \x1b[0mrunning") {
		t.Error("the title's status phrase should be plain — the glyph says it")
	}
	if !strings.Contains(frame, "\x1b[36m●\x1b[0m agent running…") {
		t.Error("the live timeline row should color only its dot")
	}

	s.selected = 1 // parked
	frame = renderFrame(s)
	if !strings.Contains(frame, "\x1b[31m✗\x1b[0m stuck: no change") {
		t.Error("parked activity should be a red glyph + the plain reason")
	}
}

// TestFrameBaselineGreenHonesty: green after zero iterations means the agent
// never ran — the monitor says so everywhere instead of celebrating: the
// header bucket, the rail glyph, the title phrase, and the activity line.
func TestFrameBaselineGreenHonesty(t *testing.T) {
	s := wideState()
	s.loops[0] = loop.LoopView{
		ID: "already-green", Goal: "a goal the verifier may not test",
		Agent: "claude", Status: loop.StatusGreen,
		IterationsUsed: 0, MaxIterations: 5,
		WallClockUsed: "1s", MaxWallClock: "30m",
		Iterations:  []loop.IterationView{{Index: 0, Baseline: true, Green: true}},
		NextCommand: "loopy review already-green",
	}
	frame := renderFrame(s)
	for _, want := range []string{
		"1 already green — check the verifier",    // the header pulse names it
		"green at baseline (the agent never ran)", // the title phrase
		"already green at baseline — nothing to do, or the verifier may not test the goal",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("baseline green honesty missing %q\n%s", want, frame)
		}
	}
	for _, gone := range []string{"ready for review", "1 green to review"} {
		if strings.Contains(frame, gone) {
			t.Errorf("baseline green is not a win; %q must not appear", gone)
		}
	}

	s.color = true
	frame = renderFrame(s)
	if !strings.Contains(frame, "\x1b[33m!\x1b[0m already green at baseline") {
		t.Error("baseline green carries the caution glyph")
	}
	if !strings.Contains(frame, "\x1b[33m!\x1b[0m \x1b[1malready-green") {
		t.Error("the rail and title glyph should be the yellow !, not the green ✓")
	}
	if strings.Contains(frame, "\x1b[32m✓\x1b[0m verifier green") {
		t.Error("baseline green must not reuse the green success line")
	}
}

// TestFrameVerifierTabBaselineGreen: the verifier tab's verdict must not
// celebrate a baseline-green run — green before the agent ran proves nothing.
func TestFrameVerifierTabBaselineGreen(t *testing.T) {
	s := wideState()
	s.tab = tabVerifier
	s.loops[0] = loop.LoopView{
		ID: "already-green", Goal: "a goal the verifier may not test",
		Agent: "codex", Status: loop.StatusGreen,
		IterationsUsed: 0, MaxIterations: 3,
		WallClockUsed: "26s", MaxWallClock: "30m",
		Verifier: []loop.Stage{{Name: "check", Cmd: "make check"}},
		Iterations: []loop.IterationView{{
			Index: 0, Baseline: true, Green: true,
			Stages: []loop.StageResult{{Name: "check", Cmd: "make check", ExitCode: 0, DurationMS: 26000}},
		}},
	}
	s.art = artifact{label: "iter 0 · verifier.log", iter: 0, lines: []string{
		"=== stage check: make check",
		"=== stage check: exit 0 (26.0s)",
	}}
	frame := renderFrame(s)
	if !strings.Contains(frame, "green at baseline — the agent never ran; this verifier may not test the goal") {
		t.Errorf("baseline-green verifier tab needs the honest verdict\n%s", frame)
	}
	if strings.Contains(frame, "green: the goal is met") {
		t.Error("a baseline-green run must not claim the goal is met")
	}
}

// TestFrameGoalWraps: a long goal wraps under a hanging indent (up to three
// lines, the last truncated) instead of vanishing behind an ellipsis.
func TestFrameGoalWraps(t *testing.T) {
	s := wideState()
	s.loops[0].Goal = "Add an AGENTS.md in the root of this repo that follows AGENTS.md best practice - sub 200 lines is preferred with executive / summary language that is easy to understand."
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 120, 36)
	if !strings.Contains(frame, "best practice") {
		t.Errorf("the goal's second line should be visible, not truncated away\n%s", frame)
	}

	_, detailW := s.railArea()
	header := detailHeaderLines(s, s.loops[0], detailW)
	goalLines := len(loop.WrapDisplay(s.loops[0].Goal, detailW-7))
	if goalLines < 2 || goalLines > 3 {
		t.Fatalf("fixture goal should wrap to 2-3 lines at this width, got %d", goalLines)
	}
	// title + goal lines + agent + activity + spacer + nav.
	if want := 5 + goalLines; len(header) != want {
		t.Fatalf("header rows = %d, want %d", len(header), want)
	}
	if !strings.HasPrefix(header[2].plain, "       ") {
		t.Errorf("goal continuation needs the hanging indent, got %q", header[2].plain)
	}

	// An absurd goal caps at three lines, the last marked truncated.
	s.loops[0].Goal = strings.Repeat("ten chars ", 60)
	header = detailHeaderLines(s, s.loops[0], detailW)
	if want := 5 + 3; len(header) != want {
		t.Fatalf("header rows = %d, want %d (goal capped at 3 lines)", len(header), want)
	}
	if !strings.HasSuffix(header[3].plain, "…") {
		t.Errorf("the capped goal line should end in an ellipsis, got %q", header[3].plain)
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

// TestFrameDecidedLoopsLeaveTheRail: history lives in the header count and
// the logbook, not the rail — unless a decided loop is explicitly selected.
func TestFrameDecidedLoopsLeaveTheRail(t *testing.T) {
	s := wideState()
	s.loops[1].Status = loop.StatusAccepted
	frame := renderFrame(s)
	checkFrameGeometry(t, frame, 120, 36)
	if strings.Contains(frame, "✗ flaky-importer") || strings.Contains(frame, "✓ flaky-importer") {
		t.Errorf("decided loop should not be in the rail\n%s", frame)
	}
	if !strings.Contains(frame, "1 decided") {
		t.Error("the header should still count the decided loop")
	}

	// Explicit selection (loopy watch <id>) pins it back into view.
	s.selected = 1
	frame = renderFrame(s)
	if !strings.Contains(frame, "flaky-importer") {
		t.Errorf("a selected decided loop must stay visible\n%s", frame)
	}
}
