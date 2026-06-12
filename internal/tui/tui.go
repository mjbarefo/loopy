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
	// Content-sized: the fixed detail header plus the body, inside bounds.
	rows := detailFixedRows
	if v := m.current(); v != nil {
		rows += len(overviewBody(s, *v, s.width))
	} else {
		rows += 16 // empty/onboarding state
	}
	height := rows + 4
	if s.width >= collapseWidth && height >= marginHeight {
		height = rows + 6 // tall enough for margins: count their rows too
	}
	s.height = clamp(height, minHeight, 64)
	s.scroll = 0
	return renderFrame(s), nil
}

// runDelete runs `loopy delete <id>` synchronously — deletion is fast, and
// the monitor wants the verdict for its flash. The CLI is the actor; the
// monitor still writes no loop state itself.
func runDelete(root, loopID string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(exe, "delete", loopID)
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
