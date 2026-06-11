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
}

// Run starts the interactive monitor and blocks until it exits. The returned
// hint, when non-empty, is the next command the user asked the monitor to
// hand off to (the `o` key) — the caller prints it after the alt screen is
// torn down.
func Run(opts Options) (hint string, err error) {
	m := newModel(opts.Root, opts.LoopID, opts.Color)
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
// the iterations view — convergence at a glance.
func RenderOnce(root, loopID string) (string, error) {
	m := newModel(root, loopID, false)
	if m.loadErr != "" {
		return "", fmt.Errorf("%s", m.loadErr)
	}
	s := m.frameState()
	s.width = onceWidth
	if cols, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && cols >= minWidth {
		s.width = cols
	}
	s.tab = tabIterations
	// Content-sized: tab bar + goal + spacer + body, inside minimum bounds.
	rows := 3
	if v := m.current(); v != nil {
		rows += len(iterationsBody(s, *v, s.width))
	}
	s.height = clamp(rows+4, minHeight, 64)
	s.scroll = 0
	return renderFrame(s), nil
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
