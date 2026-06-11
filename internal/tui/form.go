package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/mjbarefo/loopy/internal/loop"
)

// The new-loop form: press n, type a goal, enter. It resolves the verifier
// exactly like `loopy run` (project default, else inference confirmed by the
// explicit act of starting — stored as the default, same contract as the
// CLI's confirm-once). The loop is created with the same domain call the CLI
// uses and handed to a detached engine via the resume path; the monitor
// itself never runs an engine.

type formState struct {
	active bool
	goal   string

	// Verifier resolution, computed when the form opens.
	stages      []loop.Stage
	stagesDesc  string
	stored      bool   // already the project default
	inferSource string // non-empty when inferred (starting stores it)
	agent       string // the default agent the loop will use
	blocked     string // non-empty: why a loop can't start from the form
}

// openForm resolves what a started loop would use, so the form tells the
// truth before enter is pressed.
func openForm(root string) formState {
	f := formState{active: true}

	reg, err := loop.LoadAgents(root)
	switch {
	case err != nil || len(reg.Agents) == 0:
		f.blocked = "no agents registered — register one first (see the empty state or `loopy agent add`)"
		return f
	case reg.Default != "":
		f.agent = reg.Default
	default:
		for name := range reg.Agents {
			f.agent = name
			break
		}
	}

	cfg, err := loop.LoadConfig(root)
	if err == nil && len(cfg.DefaultVerifier) > 0 {
		f.stages = cfg.DefaultVerifier
		f.stored = true
	} else if inferred, ok := loop.InferVerifier(root); ok {
		f.stages = inferred.Stages
		f.inferSource = inferred.Source
	} else {
		f.blocked = `no verifier configured or inferable — start with: loopy run "<goal>" --verify "<cmd>"`
		return f
	}
	parts := make([]string, len(f.stages))
	for i, s := range f.stages {
		parts[i] = s.Cmd
	}
	f.stagesDesc = strings.Join(parts, " && ")
	return f
}

// startLoop creates the loop and hands it to a detached engine. Returns the
// new loop ID.
func startLoop(root string, f formState) (string, error) {
	goal := strings.TrimSpace(f.goal)
	if goal == "" {
		return "", fmt.Errorf("a goal is required")
	}
	if f.blocked != "" {
		return "", fmt.Errorf("%s", f.blocked)
	}
	// Starting the loop is the confirmation: store an inferred verifier as
	// the project default, exactly like the CLI's confirm-once prompt.
	if !f.stored {
		cfg, err := loop.LoadConfig(root)
		if err != nil {
			return "", err
		}
		cfg.DefaultVerifier = f.stages
		if err := loop.SaveConfig(root, cfg); err != nil {
			return "", err
		}
	}
	l, err := loop.CreateLoop(root, loop.CreateOptions{
		Goal:     goal,
		Agent:    f.agent,
		Verifier: f.stages,
	})
	if err != nil {
		return "", err
	}
	if err := spawnResume(root, l.ID); err != nil {
		return l.ID, fmt.Errorf("loop %s created but the engine did not start: %v — run `loopy resume %s`", l.ID, err, l.ID)
	}
	return l.ID, nil
}

// formLines renders the form into the detail pane.
func formLines(s frameState, width int) []cell {
	f := s.form
	lines := []cell{
		joinCells(styled(s.color, sgrBold, "start a loop"), styled(s.color, sgrDim, "   esc cancels")),
		{},
	}
	cursor := "▏"
	lines = append(lines, joinCells(
		plainCell("goal  "),
		plainCell(loop.TruncateDisplay(f.goal, width-8)),
		styled(s.color, sgrCyan, cursor),
	))
	lines = append(lines, cell{})

	if f.blocked != "" {
		lines = append(lines, styled(s.color, sgrYellow, loop.TruncateDisplay("✗ "+f.blocked, width)))
		return lines
	}

	verifier := f.stagesDesc
	note := "project default"
	if !f.stored {
		note = "inferred from " + f.inferSource + " — starting stores it as the project default"
	}
	lines = append(lines,
		joinCells(styled(s.color, sgrDim, "verifier  "), plainCell(loop.TruncateDisplay(verifier, width-10))),
		styled(s.color, sgrDim, loop.TruncateDisplay("          "+note, width)),
		joinCells(styled(s.color, sgrDim, "agent     "), plainCell(f.agent)),
		joinCells(styled(s.color, sgrDim, "budget    "), plainCell(fmt.Sprintf("%d iterations · %s (loopy run flags override)", loop.DefaultBudget.MaxIterations, loop.HumanDuration(time.Duration(loop.DefaultBudget.MaxWallClock))))),
		cell{},
		styled(s.color, sgrCyan, "enter starts the loop in its own worktree — your checkout is never touched"),
	)
	return lines
}
