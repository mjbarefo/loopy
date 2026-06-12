package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/mjbarefo/loopy/internal/loop"
)

const agentHelp = `usage:
  loopy agent add <name> --cmd <template> [--default]
  loopy agent check [name]   smoke-run agents with a trivial prompt (one
                             tiny model call each); no name checks them all
  loopy agent list
  loopy agent remove <name>

The template runs via 'sh -c' in the loop's worktree once per iteration.
Variables (always shell-quoted on expansion):
  {prompt}       full iteration prompt text
  {prompt_file}  path to the iteration's prompt.md
  {worktree}     the loop's isolated worktree
  {loop_id} {goal} {iteration}

suggested commands (headless modes proven by real loops; see docs/agents.md):
  loopy agent add claude --cmd "claude -p {prompt} --permission-mode acceptEdits"
  loopy agent add codex  --cmd "codex exec --full-auto {prompt}"
  loopy agent add gemini --cmd "gemini -p {prompt} --yolo --skip-trust"
  loopy agent add shell  --cmd "sh my-fix-script.sh {prompt_file} {worktree}"`

func handleAgent(cwd string, args []string) error {
	root, err := projectRoot(cwd)
	if err != nil {
		return err
	}
	if err := loop.EnsureInitialized(root); err != nil {
		return err
	}
	if len(args) == 0 {
		return helpRequest{agentHelp}
	}
	switch args[0] {
	case "add":
		return agentAdd(root, args[1:])
	case "check":
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		return agentCheck(root, name)
	case "list":
		return agentList(root)
	case "remove":
		if len(args) != 2 {
			return usagef("usage: loopy agent remove <name>")
		}
		if err := loop.RemoveAgent(root, args[1]); err != nil {
			return err
		}
		fmt.Printf("removed agent %s\n", loop.NormalizeAgentName(args[1]))
		return nil
	case "help", "--help", "-h":
		return helpRequest{agentHelp}
	default:
		return usagef("unknown agent subcommand %q (see `loopy agent help`)", args[0])
	}
}

func agentAdd(root string, args []string) error {
	fs := flag.NewFlagSet("agent add", flag.ContinueOnError)
	fs.SetOutput(discard{})
	cmd := fs.String("cmd", "", "agent command template")
	makeDefault := fs.Bool("default", false, "make this the default agent")
	// Allow `loopy agent add claude --cmd ...` (name first) and
	// `loopy agent add --cmd ... claude` alike.
	var name string
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		name = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return usageError{err}
	}
	if name == "" && fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	if name == "" {
		return usagef("agent name is required: loopy agent add <name> --cmd <template>")
	}
	if *cmd == "" {
		return usagef("--cmd is required: loopy agent add <name> --cmd <template>")
	}
	if err := loop.AddAgent(root, name, *cmd, *makeDefault); err != nil {
		return err
	}
	reg, err := loop.LoadAgents(root)
	if err != nil {
		return err
	}
	normalized := loop.NormalizeAgentName(name)
	suffix := ""
	if reg.Default == normalized {
		suffix = " (default)"
	}
	fmt.Printf("registered agent %s%s\n", normalized, suffix)
	if bin := loop.MissingAgentBinary(*cmd); bin != "" {
		fmt.Printf("warning: %q is not on PATH — loops with this agent will park \"agent blocked\"\n", bin)
	}
	fmt.Printf("smoke-test it (one tiny model call): loopy agent check %s\n", normalized)
	return nil
}

// agentCheck smoke-runs one agent, or all of them when name is empty. A
// failing agent makes the command exit 1, so setup scripts can gate on it.
func agentCheck(root, name string) error {
	var names []string
	if name != "" {
		names = []string{name}
	} else {
		reg, err := loop.LoadAgents(root)
		if err != nil {
			return err
		}
		names = loop.AgentNames(reg)
		if len(names) == 0 {
			return fmt.Errorf("no agents registered (see `loopy agent help`)")
		}
	}
	failed := 0
	for _, n := range names {
		res, err := loop.CheckAgent(root, n)
		if err != nil {
			return err
		}
		took := (time.Duration(res.WallMS) * time.Millisecond).Round(100 * time.Millisecond)
		if res.OK {
			fmt.Printf("✓ %s ok (%s)\n", res.Name, took)
			continue
		}
		failed++
		fmt.Printf("✗ %s failed (exit %d, %s)\n", res.Name, res.Exit, took)
		if res.Words != "" {
			fmt.Printf("    %s\n", res.Words)
		}
		fmt.Printf("    fix the command and re-register: loopy agent add %s --cmd '…'\n", res.Name)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d agent(s) failed the check", failed, len(names))
	}
	return nil
}

func agentList(root string) error {
	reg, err := loop.LoadAgents(root)
	if err != nil {
		return err
	}
	if len(reg.Agents) == 0 {
		fmt.Println("no agents registered (see `loopy agent help`)")
		return nil
	}
	for _, name := range loop.AgentNames(reg) {
		marker := " "
		if name == reg.Default {
			marker = "*"
		}
		fmt.Printf("%s %-12s %s\n", marker, name, reg.Agents[name].Cmd)
	}
	return nil
}

// discard silences flag's default error printing; we format usage errors
// ourselves through the exit-code contract.
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
