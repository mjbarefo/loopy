// Command loopy is a local tool for engineering coding-agent loops: define a
// goal, a verifier, and a budget; an agent iterates in an isolated worktree
// until the verifier goes green or the budget runs out; a human reviews the
// result with the full iteration history.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/mjbarefo/loopy/internal/loop"
)

// Exit codes: 0 success (including help), 1 runtime failure, 2 usage error.
// `loopy run` additionally exits 1 when the loop parks without green, so
// scripts can gate on the verdict.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

// usageError marks errors caused by invalid invocation rather than a failed
// operation, so scripts can distinguish the two via the exit code.
type usageError struct{ err error }

func (u usageError) Error() string { return u.err.Error() }
func (u usageError) Unwrap() error { return u.err }

func usagef(format string, args ...any) error {
	return usageError{fmt.Errorf(format, args...)}
}

// helpRequest carries usage text the user explicitly asked for; it prints to
// stdout and exits 0.
type helpRequest struct{ usage string }

func (h helpRequest) Error() string { return h.usage }

func main() {
	os.Exit(runWithExitCode(os.Args[1:]))
}

func runWithExitCode(args []string) int {
	err := run(args)
	if err == nil {
		return exitOK
	}
	var help helpRequest
	if errors.As(err, &help) {
		fmt.Println(help.usage)
		return exitOK
	}
	fmt.Fprintf(os.Stderr, "loopy: %v\n", err)
	var usage usageError
	if errors.As(err, &usage) {
		return exitUsage
	}
	return exitFailure
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Println(rootHelp)
		return nil
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Println(rootHelp)
		return nil
	case "version", "--version", "-version":
		fmt.Printf("loopy %s\n", loop.ResolvedVersion())
		return nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	switch args[0] {
	case "init":
		return handleInit(cwd, args[1:])
	case "agent":
		return handleAgent(cwd, args[1:])
	case "doctor":
		return handleDoctor(cwd, args[1:])
	default:
		return usagef("unknown command %q (see `loopy help`)", args[0])
	}
}

// projectRoot resolves the git top level; loopy state lives at the repo root
// no matter where in the tree it is invoked.
func projectRoot(cwd string) (string, error) {
	return loop.DetectGitRoot(cwd)
}

const rootHelp = `loopy — engineer loops, not prompts

  An agent iterates in an isolated git worktree until your verifier goes
  green or the budget runs out. You review the result; loopy never ships.

usage:
  loopy init                            prepare this repository for loops
  loopy agent add <name> --cmd <tmpl>   register an agent command
  loopy agent list | remove <name>      manage registered agents
  loopy doctor [--json]                 diagnose environment and state
  loopy version                         print version

agent command templates substitute (always shell-quoted):
  {prompt} {prompt_file} {worktree} {loop_id} {goal} {iteration}

exit codes: 0 success · 1 runtime failure · 2 usage error`
