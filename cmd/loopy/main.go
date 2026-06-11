// Command loopy is a local tool for engineering coding-agent loops: define a
// goal, a verifier, and a budget; an agent iterates in an isolated worktree
// until the verifier goes green or the budget runs out; a human reviews the
// result with the full iteration history.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

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
	case "run":
		return handleRun(cwd, args[1:])
	case "list":
		return handleList(cwd, args[1:])
	case "status":
		return handleStatus(cwd, args[1:])
	case "log":
		return handleLog(cwd, args[1:])
	case "watch":
		return handleWatch(cwd, args[1:])
	case "pause":
		return handlePause(cwd, args[1:])
	case "resume":
		return handleResume(cwd, args[1:])
	case "abort":
		return handleAbort(cwd, args[1:])
	case "review":
		return handleReview(cwd, args[1:])
	case "accept":
		return handleAccept(cwd, args[1:])
	case "reject":
		return handleReject(cwd, args[1:])
	case "logbook":
		return handleLogbook(cwd, args[1:])
	case "judge":
		return handleJudge(cwd, args[1:])
	case "doctor":
		return handleDoctor(cwd, args[1:])
	default:
		// The happy path: loopy "<goal>". Only multi-word arguments read as
		// goals, so a typo'd command can't silently become a loop.
		if len(args) == 1 && strings.ContainsAny(args[0], " \t") {
			return handleRun(cwd, args)
		}
		return usagef("unknown command %q (see `loopy help`; to start a loop: loopy \"<goal>\")", args[0])
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

start a loop:
  loopy "<goal>"                        the happy path: defaults for everything
  loopy run "<goal>" [flags]            engineer the loop deliberately
                                        (see loopy run --help)

watch and steer:
  loopy watch [loop-id] [--once]        the monitor: live view, timeline,
                                        drill-downs (default: newest loop)
  loopy list [--json]                   all loops, one line each
  loopy status [loop-id] [--json]       one loop in depth (default: newest)
  loopy log <loop-id> [--iter N]        the recorded iteration history
  loopy pause | resume | abort <id>     control a running loop

judge:
  loopy review <loop-id>                final diff, verifier transcript,
                                        iteration history
  loopy accept <loop-id>                record the decision; non-green needs
                                        --override --reason (kept verbatim)
  loopy reject <loop-id> [--reason]     decline; evidence kept, worktree freed
  loopy judge <id> <id> [...]           rank finished loops by their evidence
                                        (deterministic; used by --race)
  loopy logbook [--json]                durable memory of every decision

setup:
  loopy init                            prepare this repository for loops
  loopy agent add <name> --cmd <tmpl>   register an agent command
  loopy agent list | remove <name>      manage registered agents
  loopy doctor [--json]                 diagnose environment and state
  loopy version                         print version

agent command templates substitute (always shell-quoted):
  {prompt} {prompt_file} {worktree} {loop_id} {goal} {iteration}

exit codes: 0 success · 1 runtime failure · 2 usage error
(loopy run: 0 means the loop parked green)`
