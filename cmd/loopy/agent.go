package main

import (
	"flag"
	"fmt"

	"github.com/mjbarefo/loopy/internal/loop"
)

const agentHelp = `usage:
  loopy agent add <name> --cmd <template> [--default]
  loopy agent list
  loopy agent remove <name>

The template runs via 'sh -c' in the loop's worktree once per iteration.
Variables (always shell-quoted on expansion):
  {prompt}       full iteration prompt text
  {prompt_file}  path to the iteration's prompt.md
  {worktree}     the loop's isolated worktree
  {loop_id} {goal} {iteration}

suggested commands (headless modes; verify against your installed versions):
  loopy agent add claude --cmd "claude -p {prompt} --permission-mode acceptEdits"
  loopy agent add codex  --cmd "codex exec {prompt}"
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
