package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mjbarefo/loopy/internal/loop"
)

const runHelp = `usage:
  loopy "<goal>"
  loopy run "<goal>" [flags]

flags:
  --verify <cmd>          verifier stage; repeatable, runs in order (fast→slow)
  --agent <name>          registered agent to use (default: registry default)
  --max-iters <n>         iteration budget (default 8)
  --max-time <dur>        wall-clock budget, e.g. 30m (default 30m)
  --constraint <text>     goal constraint; repeatable
  --forbidden-path <p>    path the agent must not touch; repeatable

Without --verify, loopy uses the project's stored default verifier, or infers
one from the repo (make check, go test, npm test, ...) and asks once before
storing it. A loop cannot be created without a verifier.

exit codes: 0 loop parked green · 1 parked red or failed · 2 usage error`

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ", ") }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func handleRun(cwd string, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(discard{})
	var verify, constraints, forbidden stringList
	fs.Var(&verify, "verify", "verifier stage command (repeatable)")
	fs.Var(&constraints, "constraint", "constraint (repeatable)")
	fs.Var(&forbidden, "forbidden-path", "forbidden path (repeatable)")
	agent := fs.String("agent", "", "agent name")
	maxIters := fs.Int("max-iters", 0, "max iterations")
	maxTime := fs.Duration("max-time", 0, "max wall clock")

	// Accept both `loopy run "goal" --flags` and `loopy run --flags "goal"`.
	var goal string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		goal = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return helpRequest{runHelp}
		}
		return usageError{err}
	}
	if goal == "" && fs.NArg() > 0 {
		goal = fs.Arg(0)
	}
	if strings.TrimSpace(goal) == "" {
		return usagef("a goal is required: loopy run \"<goal>\"")
	}

	root, err := projectRoot(cwd)
	if err != nil {
		return err
	}
	if err := loop.EnsureInitialized(root); err != nil {
		return err
	}

	stages, err := resolveVerifier(root, verify)
	if err != nil {
		return err
	}

	l, err := loop.CreateLoop(root, loop.CreateOptions{
		Goal:           goal,
		Agent:          *agent,
		Verifier:       stages,
		Constraints:    constraints,
		ForbiddenPaths: forbidden,
		Budget: loop.Budget{
			MaxIterations: *maxIters,
			MaxWallClock:  loop.Duration(*maxTime),
		},
	})
	if err != nil {
		return err
	}
	return driveEngine(root, l.ID)
}

// driveEngine runs the engine in the foreground with progress lines, shared
// by run and resume. The exit-code contract: green nil, anything else error.
func driveEngine(root, loopID string) error {
	final, err := loop.RunEngine(root, loopID, progressEvents())
	if err != nil {
		return err
	}
	switch final.Status {
	case loop.StatusGreen:
		return nil
	case loop.StatusPaused:
		// An intentional pause is not a failure.
		return nil
	default:
		return fmt.Errorf("loop %s parked: %s", final.ID, final.ParkedReason)
	}
}

// progressEvents renders engine progress as plain lines: one per phase,
// CI-friendly, color only as a secondary signal.
func progressEvents() loop.Events {
	return loop.Events{
		LoopStarted: func(l loop.Loop) {
			fmt.Printf("loop %s started · agent %s · budget %d iters / %s\n",
				colorize(cyan, l.ID), l.Agent, l.Budget.MaxIterations, time.Duration(l.Budget.MaxWallClock))
			fmt.Printf("worktree %s (branch %s)\n", l.Worktree, l.Branch)
		},
		BaselineStarted: func() {
			fmt.Println("baseline: verifying before the first agent run…")
		},
		IterationStarted: func(index, max int) {
			fmt.Printf("iter %d/%d: agent running…\n", index, max)
		},
		AgentDone: func(index, exitCode int, d time.Duration) {
			verdict := fmt.Sprintf("exit %d", exitCode)
			if exitCode != 0 {
				verdict = colorize(red, verdict)
			}
			fmt.Printf("iter %d: agent done (%s, %s)\n", index, verdict, d.Round(time.Second))
		},
		StageDone: func(index int, r loop.StageResult) {
			if r.ExitCode == 0 {
				fmt.Printf("iter %d: %s %s (%s)\n", index, colorize(green, "✓"), r.Name, (time.Duration(r.DurationMS) * time.Millisecond).Round(time.Millisecond))
			} else {
				fmt.Printf("iter %d: %s %s failed, exit %d (%s)\n", index, colorize(red, "✗"), r.Name, r.ExitCode, (time.Duration(r.DurationMS) * time.Millisecond).Round(time.Millisecond))
			}
		},
		IterationDone: func(it loop.Iteration, l loop.Loop) {
			if it.Violation != "" {
				fmt.Printf("iter %d: %s %s\n", it.Index, colorize(red, "✗"), it.Violation)
			}
		},
		Note: func(s string) {
			fmt.Printf("note: %s\n", s)
		},
		LoopEnded: func(l loop.Loop) {
			switch l.Status {
			case loop.StatusGreen:
				note := ""
				if l.ParkedReason != "" {
					note = " (" + l.ParkedReason + ")"
				}
				fmt.Printf("%s loop %s is green%s — parked for review\n", colorize(green, "✓"), l.ID, note)
				if view, err := loop.BuildLoopView(root(), l); err == nil && view.NextCommand != "" {
					fmt.Printf("next: %s\n", view.NextCommand)
				}
			case loop.StatusParked:
				fmt.Printf("%s loop %s parked: %s\n", colorize(red, "✗"), l.ID, l.ParkedReason)
				fmt.Printf("next: loopy log %s\n", l.ID)
			}
		},
	}
}

// root is a lazy lookup for event rendering; events fire from engine context
// where the project root is already established.
func root() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	r, err := loop.DetectGitRoot(cwd)
	if err != nil {
		return cwd
	}
	return r
}

// resolveVerifier applies the precedence: --verify flags > stored project
// default > inference confirmed once and stored. No verifier, no loop.
func resolveVerifier(root string, cmds []string) ([]loop.Stage, error) {
	if len(cmds) > 0 {
		stages := make([]loop.Stage, len(cmds))
		for i, cmd := range cmds {
			stages[i] = loop.Stage{Name: fmt.Sprintf("verify-%d", i+1), Cmd: cmd}
		}
		if len(stages) == 1 {
			stages[0].Name = "verify"
		}
		return stages, nil
	}
	cfg, err := loop.LoadConfig(root)
	if err != nil {
		return nil, err
	}
	if len(cfg.DefaultVerifier) > 0 {
		return cfg.DefaultVerifier, nil
	}

	inferred, ok := loop.InferVerifier(root)
	if !ok {
		return nil, fmt.Errorf("no verifier: pass --verify <cmd>, or set a default in .loopy/config.json (a loop cannot exist without one)")
	}
	if !isTTY(os.Stdin) {
		return nil, fmt.Errorf("no verifier configured; inferred %q from %s but won't use it unconfirmed — pass --verify <cmd> or confirm interactively once", describeStages(inferred.Stages), inferred.Source)
	}
	fmt.Printf("no default verifier configured. Detected from %s:\n", inferred.Source)
	for i, s := range inferred.Stages {
		fmt.Printf("  %d. %s: %s\n", i+1, s.Name, s.Cmd)
	}
	fmt.Print("use this as the project's default verifier? [Y/n] ")
	line, readErr := bufio.NewReader(os.Stdin).ReadString('\n')
	if readErr != nil && line == "" {
		return nil, fmt.Errorf("no confirmation; pass --verify <cmd>")
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "" && answer != "y" && answer != "yes" {
		return nil, fmt.Errorf("declined; pass --verify <cmd> to define the verifier explicitly")
	}
	cfg.DefaultVerifier = inferred.Stages
	if err := loop.SaveConfig(root, cfg); err != nil {
		return nil, err
	}
	fmt.Println("stored as default verifier in .loopy/config.json")
	return inferred.Stages, nil
}

func describeStages(stages []loop.Stage) string {
	parts := make([]string, len(stages))
	for i, s := range stages {
		parts[i] = s.Cmd
	}
	return strings.Join(parts, " && ")
}
