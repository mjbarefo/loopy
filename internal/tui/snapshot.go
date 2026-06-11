// Package tui is the monitor: `loopy watch`. It is the only package allowed
// to import the TUI framework. The monitor renders from state files and
// writes only control.json — the engine remains the single writer of loop
// state.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mjbarefo/loopy/internal/loop"
)

// tabID indexes the detail-pane views.
type tabID int

const (
	tabLive tabID = iota
	tabIterations
	tabDiff
	tabVerifier
	tabCount
)

var tabNames = [tabCount]string{"live", "iterations", "diff", "verifier"}

// artifact is one viewer's content: a capped, tail-first load of a single
// evidence file.
type artifact struct {
	label     string // e.g. "iter 3 · agent.log"
	lines     []string
	truncated bool
	size      int64
	missing   bool
}

// loadLoops reads every loop's view-model, newest last (ListLoops order).
func loadLoops(root string) ([]loop.LoopView, error) {
	loops, err := loop.ListLoops(root)
	if err != nil {
		return nil, err
	}
	views := make([]loop.LoopView, 0, len(loops))
	for _, l := range loops {
		v, err := loop.BuildLoopView(root, l)
		if err != nil {
			return nil, err
		}
		views = append(views, v)
	}
	return views, nil
}

// loadArtifact tail-loads one file under the viewer cap.
func loadArtifact(label, path string) artifact {
	data, truncated, size, err := loop.TailFile(path, loop.ViewerCapBytes)
	if err != nil {
		return artifact{label: label, missing: true}
	}
	text := strings.TrimRight(string(data), "\n")
	var lines []string
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	return artifact{label: label, lines: lines, truncated: truncated, size: size}
}

// currentIterationIndex is the iteration the engine is working on now: one
// past the recorded history when its evidence directory already exists,
// otherwise the last recorded iteration.
func currentIterationIndex(root string, v loop.LoopView) int {
	last := len(v.Iterations) - 1
	if last < 0 {
		return 0
	}
	next := v.Iterations[last].Index + 1
	if _, err := os.Stat(loop.IterationDir(root, v.ID, next)); err == nil {
		return next
	}
	return v.Iterations[last].Index
}

// liveArtifact picks the file to tail for the live view: whichever of the
// current iteration's agent.log / verifier.log was written to most recently.
func liveArtifact(root string, v loop.LoopView) artifact {
	idx := currentIterationIndex(root, v)
	dir := loop.IterationDir(root, v.ID, idx)
	agentPath := filepath.Join(dir, loop.AgentLogFile)
	verifierPath := filepath.Join(dir, loop.VerifierLogFile)

	path, name := agentPath, loop.AgentLogFile
	agentInfo, agentErr := os.Stat(agentPath)
	verifierInfo, verifierErr := os.Stat(verifierPath)
	switch {
	case agentErr != nil && verifierErr != nil:
		return artifact{label: fmt.Sprintf("iter %d", idx), missing: true}
	case agentErr != nil:
		path, name = verifierPath, loop.VerifierLogFile
	case verifierErr == nil && verifierInfo.ModTime().After(agentInfo.ModTime()):
		path, name = verifierPath, loop.VerifierLogFile
	}
	return loadArtifact(fmt.Sprintf("iter %d · %s", idx, name), path)
}

// latestArtifact scans backwards from the current iteration for the newest
// existing copy of one evidence file (diff.patch, verifier.log).
func latestArtifact(root string, v loop.LoopView, name string) artifact {
	for idx := currentIterationIndex(root, v); idx >= 0; idx-- {
		path := filepath.Join(loop.IterationDir(root, v.ID, idx), name)
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return loadArtifact(fmt.Sprintf("iter %d · %s", idx, name), path)
		}
	}
	return artifact{label: name, missing: true}
}

// loadTabArtifact returns the artifact behind the given tab for one loop.
// The iterations tab renders from the view-model, not a file.
func loadTabArtifact(root string, v loop.LoopView, tab tabID) artifact {
	switch tab {
	case tabLive:
		return liveArtifact(root, v)
	case tabDiff:
		return latestArtifact(root, v, loop.DiffFile)
	case tabVerifier:
		return latestArtifact(root, v, loop.VerifierLogFile)
	default:
		return artifact{}
	}
}

// errText flattens a load error for in-frame display.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
