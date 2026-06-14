package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	tea "charm.land/bubbletea/v2"
)

// Options configures one monitor session.
type Options struct {
	Root   string
	LoopID string // initial selection; empty selects the newest loop
	Color  bool
	// Welcome shows the launch splash first (bare `loopy`); any key enters
	// the monitor.
	Welcome bool
}

// Run starts the interactive monitor and blocks until it exits. The returned
// hint, when non-empty, is the next command the user asked the monitor to
// hand off to (the `o` key) — the caller prints it after the alt screen is
// torn down.
func Run(opts Options) (hint string, err error) {
	m := newModel(opts.Root, opts.LoopID, opts.Color)
	m.welcome = opts.Welcome
	res, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", err
	}
	if final, ok := res.(model); ok {
		return final.exitHint, nil
	}
	return "", nil
}

// onceWidth is the default frame width for `watch --once`; COLUMNS overrides
// it so scripts and smoke tests can pin a size.
const onceWidth = 100

// RenderOnce produces one deterministic, ANSI-free frame for scripts: the
// same renderer as the live monitor, color off, content-sized height, and
// the overview — convergence at a glance. Key hints and the live elapsed
// clock are omitted; they are interactive noise in a captured frame.
func RenderOnce(root, loopID string) (string, error) {
	m := newModel(root, loopID, false)
	if m.loadErr != "" {
		return "", fmt.Errorf("%s", m.loadErr)
	}
	s := m.frameState()
	s.once = true
	s.phaseElapsed = ""
	s.width = onceWidth
	if cols, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && cols >= minWidth {
		s.width = cols
	}
	s.tab = tabOverview
	// Content-sized: the detail header (goal and activity lines wrap, so it
	// is counted) plus the body, inside bounds.
	rows := 6 + 16 // empty/onboarding state
	if v := m.current(); v != nil {
		_, detailW := s.railArea()
		rows = len(detailHeaderLines(s, *v, detailW)) + len(overviewBody(s, *v, s.width))
	}
	height := rows + 4
	if s.width >= collapseWidth && height >= marginHeight {
		height = rows + 6 // tall enough for margins: count their rows too
	}
	s.height = clamp(height, minHeight, 64)
	s.scroll = 0
	return renderFrame(s), nil
}

// runCLI runs an audited loopy command (`delete`, `accept`, `reject`)
// synchronously — these are fast, and the monitor wants the verdict for its
// flash. The CLI is the actor; the monitor still writes no loop state itself.
func runCLI(root string, args ...string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runGit runs git in the repo root. The monitor uses it for exactly one
// thing — `git apply` of an accepted loop's durable diff onto the working
// tree (the A key). It is not a loop-state write and never commits, pushes,
// or merges; the user would otherwise paste the same command themselves.
func runGit(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// spawnResume starts a detached `loopy resume <id>` engine. The monitor
// itself never writes loop state — the child is a normal engine process with
// the usual lock; its plain progress stream is redundant with the state
// files, so stdout goes nowhere.
func spawnResume(root, loopID string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "resume", loopID)
	cmd.Dir = root
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = detachedProcAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
