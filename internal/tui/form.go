package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mjbarefo/loopy/internal/loop"
)

// The new-loop wizard: press n and the monitor walks through every decision
// a loop needs — goal, agent, verifier, budget, confirm — one question per
// screen, plain words, defaults prefilled, esc steps back. It resolves the
// verifier exactly like `loopy run` (project default, else inference;
// starting with an untouched inferred verifier stores it as the project
// default — the CLI's confirm-once contract). Loops are created with the
// same domain call the CLI uses and handed to detached engines via the
// resume path; the monitor itself never runs an engine. Marking more than
// one agent races them: one loop per agent, ranked by `loopy judge` when
// they have all parked.

type wizardStep int

const (
	stepGoal wizardStep = iota
	stepAgent
	stepVerifier
	stepBudget
	stepConfirm
	stepCount
)

type formState struct {
	active bool
	step   wizardStep

	goal string

	// Agent selection: every registered agent, default first. When none are
	// registered yet, detected CLIs can be registered without leaving the
	// wizard.
	agents       []string
	defaultAgent string
	detected     []loop.AgentSuggestion
	cursor       int
	picked       map[int]bool // marked agents; more than one races

	// Verifier: an editable command, prefilled from the project default or
	// inference. The prefill keeps its multi-stage form until edited.
	verifier      string
	prefillStages []loop.Stage
	stored        bool   // prefill is already the project default
	inferSource   string // non-empty when the prefill was inferred
	edited        bool

	// Budget fields as text, validated when the step advances.
	iters       string
	wall        string
	budgetField int // 0 iterations, 1 wall clock
}

// openForm resolves everything a started loop would use, so every wizard
// screen tells the truth before enter is pressed.
func openForm(root string) formState {
	f := formState{
		active: true,
		picked: map[int]bool{},
		iters:  strconv.Itoa(loop.DefaultBudget.MaxIterations),
		wall:   loop.HumanDuration(time.Duration(loop.DefaultBudget.MaxWallClock)),
	}

	if reg, err := loop.LoadAgents(root); err == nil && len(reg.Agents) > 0 {
		f.defaultAgent = reg.Default
		for name := range reg.Agents {
			f.agents = append(f.agents, name)
		}
		sort.Slice(f.agents, func(i, j int) bool {
			if (f.agents[i] == f.defaultAgent) != (f.agents[j] == f.defaultAgent) {
				return f.agents[i] == f.defaultAgent
			}
			return f.agents[i] < f.agents[j]
		})
	} else {
		f.detected = loop.DetectAgentCLIs(root)
	}

	cfg, err := loop.LoadConfig(root)
	if err == nil && len(cfg.DefaultVerifier) > 0 {
		f.prefillStages = cfg.DefaultVerifier
		f.stored = true
	} else if inferred, ok := loop.InferVerifier(root); ok {
		f.prefillStages = inferred.Stages
		f.inferSource = inferred.Source
	}
	parts := make([]string, len(f.prefillStages))
	for i, s := range f.prefillStages {
		parts[i] = s.Cmd
	}
	f.verifier = strings.Join(parts, " && ")
	return f
}

// selectedAgents returns the marked agents in list order, falling back to
// the agent under the cursor.
func (f formState) selectedAgents() []string {
	var out []string
	for i, a := range f.agents {
		if f.picked[i] {
			out = append(out, a)
		}
	}
	if len(out) == 0 && f.cursor < len(f.agents) {
		out = append(out, f.agents[f.cursor])
	}
	return out
}

// resolvedStages is the verifier the loop will actually run: the prefilled
// stages while untouched, otherwise one stage from the edited command.
func (f formState) resolvedStages() []loop.Stage {
	cmd := strings.TrimSpace(f.verifier)
	if cmd == "" {
		return nil
	}
	if !f.edited && len(f.prefillStages) > 0 {
		return f.prefillStages
	}
	return []loop.Stage{{Name: "verify", Cmd: cmd}}
}

// startLoops creates one loop per selected agent and hands each to a
// detached engine. Returns the new loop IDs.
func startLoops(root string, f formState) ([]string, error) {
	goal := strings.TrimSpace(f.goal)
	if goal == "" {
		return nil, fmt.Errorf("a goal is required")
	}
	agents := f.selectedAgents()
	if len(agents) == 0 {
		return nil, fmt.Errorf("pick an agent first")
	}
	stages := f.resolvedStages()
	if len(stages) == 0 {
		return nil, fmt.Errorf("a loop needs a verifier — no verifier, no loop")
	}
	iters, err := strconv.Atoi(strings.TrimSpace(f.iters))
	if err != nil || iters < 1 {
		return nil, fmt.Errorf("iterations must be a number of at least 1")
	}
	wall, err := time.ParseDuration(strings.TrimSpace(f.wall))
	if err != nil || wall <= 0 {
		return nil, fmt.Errorf("wall clock must be a duration like 30m or 2h")
	}

	// Starting with an untouched inferred verifier is the confirmation that
	// stores it as the project default — the CLI's confirm-once contract.
	if f.inferSource != "" && !f.edited {
		cfg, err := loop.LoadConfig(root)
		if err != nil {
			return nil, err
		}
		cfg.DefaultVerifier = stages
		if err := loop.SaveConfig(root, cfg); err != nil {
			return nil, err
		}
	}

	var ids []string
	for _, agent := range agents {
		l, err := loop.CreateLoop(root, loop.CreateOptions{
			Goal:     goal,
			Agent:    agent,
			Verifier: stages,
			Budget: loop.Budget{
				MaxIterations: iters,
				MaxWallClock:  loop.Duration(wall),
			},
		})
		if err != nil {
			return ids, err
		}
		if err := spawnResume(root, l.ID); err != nil {
			return ids, fmt.Errorf("loop %s created but the engine did not start: %v — run `loopy resume %s`", l.ID, err, l.ID)
		}
		ids = append(ids, l.ID)
	}
	return ids, nil
}

// formLines renders the current wizard screen into the detail pane. This is
// the launch-screen moment of the working monitor: headroom above the title,
// blank rows between regions, one accent per screen (the input cursor, or
// the action line on confirm), and one dim affordance line at the bottom.
func formLines(s frameState, width int) []cell {
	f := s.form
	var lines []cell
	if s.roomy() {
		lines = append(lines, cell{})
	}
	lines = append(lines,
		joinCells(
			styled(s.color, sgrBold, "start a loop"),
			styled(s.color, sgrDim, fmt.Sprintf("   step %d of %d", f.step+1, stepCount)),
		),
		cell{},
	)
	switch f.step {
	case stepGoal:
		lines = append(lines, goalLines(s, width)...)
	case stepAgent:
		lines = append(lines, agentLines(s, width)...)
	case stepVerifier:
		lines = append(lines, verifierLines(s, width)...)
	case stepBudget:
		lines = append(lines, budgetLines(s)...)
	case stepConfirm:
		lines = append(lines, confirmLines(s, width)...)
	}
	return lines
}

// affordance is the one dim hint line each wizard screen ends with.
func affordance(s frameState, text string) cell {
	return styled(s.color, sgrDim, text)
}

func inputCell(s frameState, label, value string, active bool, width int) cell {
	cells := []cell{
		styled(s.color, sgrDim, label),
		plainCell(loop.TruncateDisplay(value, width-loop.DisplayWidth(label)-2)),
	}
	if active {
		cells = append(cells, styled(s.color, sgrCyan, "▏"))
	}
	return joinCells(cells...)
}

func goalLines(s frameState, width int) []cell {
	return []cell{
		inputCell(s, "goal  ", s.form.goal, true, width),
		{},
		styled(s.color, sgrDim, "describe what done looks like — the agent iterates until the verifier passes."),
		{},
		affordance(s, "enter continues · esc cancels"),
	}
}

func agentLines(s frameState, width int) []cell {
	f := s.form
	if len(f.agents) == 0 && len(f.detected) == 0 {
		return []cell{
			joinCells(
				styled(s.color, sgrYellow, "✗"),
				plainCell(" no agent CLIs registered or found on this machine"),
			),
			{},
			styled(s.color, sgrDim, loop.TruncateDisplay(`register one first:  loopy agent add claude --cmd "claude -p {prompt} --permission-mode acceptEdits" --default`, width)),
		}
	}

	lines := []cell{styled(s.color, sgrDim, "who does the work — space marks more than one to race them.")}
	lines = append(lines, cell{})
	if len(f.agents) > 0 {
		for i, a := range f.agents {
			marker := plainCell("   ")
			name := plainCell(a)
			if i == f.cursor {
				marker = styled(s.color, sgrCyan, " ▶ ")
				name = styled(s.color, sgrBold, a)
			}
			// The race mark is a plain glyph: the cursor keeps the accent.
			mark := plainCell("  ")
			if f.picked[i] {
				mark = plainCell("✓ ")
			}
			note := ""
			if a == f.defaultAgent {
				note = "  (default)"
			}
			lines = append(lines, joinCells(marker, mark, name, styled(s.color, sgrDim, note)))
		}
		lines = append(lines, cell{}, affordance(s, "enter continues with "+strings.Join(f.selectedAgents(), " + ")+" · esc goes back"))
		return lines
	}

	// Nothing registered yet: offer what the machine already has.
	lines[0] = styled(s.color, sgrDim, "no agents registered yet — found on this machine:")
	for i, d := range f.detected {
		marker := plainCell("   ")
		name := plainCell(d.Name)
		if i == f.cursor {
			marker = styled(s.color, sgrCyan, " ▶ ")
			name = styled(s.color, sgrBold, d.Name)
		}
		lines = append(lines, joinCells(marker, name, styled(s.color, sgrDim, loop.TruncateDisplay("  ("+d.Cmd+")", width-20))))
	}
	lines = append(lines, cell{}, affordance(s, "enter registers it and continues · esc goes back"))
	return lines
}

func verifierLines(s frameState, width int) []cell {
	f := s.form
	source := "type the command that proves the goal is met"
	switch {
	case f.edited:
		source = "edited — used as a single stage for this loop"
	case f.stored:
		source = "the project default — edit to override for this loop"
	case f.inferSource != "":
		source = "inferred from " + f.inferSource + " — starting stores it as the project default"
	}
	return []cell{
		inputCell(s, "verifier  ", f.verifier, true, width),
		styled(s.color, sgrDim, loop.TruncateDisplay("          "+source, width)),
		{},
		styled(s.color, sgrDim, "this command decides green: exit 0 means the goal is met."),
		{},
		affordance(s, "enter continues · esc goes back"),
	}
}

func budgetLines(s frameState) []cell {
	f := s.form
	return []cell{
		inputCell(s, "iterations  ", f.iters, f.budgetField == 0, 40),
		inputCell(s, "wall clock  ", f.wall, f.budgetField == 1, 40),
		{},
		styled(s.color, sgrDim, "hard caps — the loop parks when either runs out (↑↓ switches fields)."),
		{},
		affordance(s, "enter continues · esc goes back"),
	}
}

func confirmLines(s frameState, width int) []cell {
	f := s.form
	agents := strings.Join(f.selectedAgents(), " + ")
	stages := f.resolvedStages()
	parts := make([]string, len(stages))
	for i, st := range stages {
		parts[i] = st.Cmd
	}
	action := "enter starts the loop in its own worktree — your checkout is never touched"
	if len(f.selectedAgents()) > 1 {
		action = fmt.Sprintf("enter races %d agents in parallel worktrees — your checkout is never touched", len(f.selectedAgents()))
	}
	return []cell{
		joinCells(styled(s.color, sgrDim, "goal      "), plainCell(loop.TruncateDisplay(f.goal, width-10))),
		joinCells(styled(s.color, sgrDim, "agent     "), plainCell(agents)),
		joinCells(styled(s.color, sgrDim, "verifier  "), plainCell(loop.TruncateDisplay(strings.Join(parts, " && "), width-10))),
		joinCells(styled(s.color, sgrDim, "budget    "), plainCell(f.iters+" iterations · "+f.wall)),
		{},
		// The action line is this screen's one accent and one affordance;
		// esc was the same on every screen before it.
		styled(s.color, sgrCyan, loop.TruncateDisplay(action, width)),
	}
}
