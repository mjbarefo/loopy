package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/mjbarefo/loopy/internal/loop"
	"github.com/mjbarefo/loopy/internal/tui"
)

// launchMonitor is bare `loopy`: the monitor with the welcome splash. It
// works in an uninitialized repo — the empty state walks through setup —
// and outside any repo it becomes the front door: pick a nearby repository
// (or git-init in place) and flow straight into that repo's monitor.
func launchMonitor() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	welcome := true
	root, err := projectRoot(cwd)
	if err != nil {
		// Not a git repo: the monitor has nothing to watch — offer nearby
		// repos instead of a dead end. (Pipes never reach here; run()
		// routes them to the help text before launchMonitor.)
		repos := loop.FindRepos(cwd)
		if len(repos) == 0 {
			fmt.Print(tui.FrontDoor(colorEnabled, cwd))
			return nil
		}
		choice, initHere, err := tui.PickRepo(cwd, repos, colorEnabled)
		if err != nil {
			return err
		}
		switch {
		case initHere:
			// An explicit, labeled choice — never an accidental keypress.
			if out, err := exec.Command("git", "init").CombinedOutput(); err != nil {
				return fmt.Errorf("git init: %v\n%s", err, out)
			}
			fmt.Printf("initialized a git repository in %s\n", cwd)
			root = cwd
		case choice != "":
			root = choice
		default:
			return nil // the user looked and left; that is an answer
		}
		welcome = false // the picker was the branded moment; skip the splash
	}
	hint, err := tui.Run(tui.Options{
		Root:    root,
		Color:   colorEnabled,
		Welcome: welcome,
	})
	if err != nil {
		return err
	}
	if hint != "" {
		// The hint must work from where the user actually is.
		if root != cwd {
			hint = fmt.Sprintf("cd %s && %s", root, hint)
		}
		fmt.Printf("next: %s\n", hint)
	}
	return nil
}

func handleWatch(cwd string, args []string) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(discard{})
	once := fs.Bool("once", false, "print one plain ANSI-free frame and exit")
	noColor := fs.Bool("no-color", false, "disable color")
	var loopID string
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		loopID = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return helpRequest{watchHelp}
		}
		return usageError{err}
	}
	if loopID == "" && fs.NArg() > 0 {
		loopID = fs.Arg(0)
	}
	root, err := projectRoot(cwd)
	if err != nil {
		return err
	}
	if err := loop.EnsureInitialized(root); err != nil {
		return err
	}
	if loopID != "" {
		if _, err := loop.LoadLoop(root, loopID); err != nil {
			return err
		}
	}

	if *once {
		frame, err := tui.RenderOnce(root, loopID)
		if err != nil {
			return err
		}
		fmt.Print(frame)
		return nil
	}

	if !isTTY(os.Stdout) || !isTTY(os.Stdin) {
		return fmt.Errorf("loopy watch needs a terminal (use --once for a single plain frame)")
	}
	hint, err := tui.Run(tui.Options{
		Root:   root,
		LoopID: loopID,
		Color:  colorEnabled && !*noColor,
	})
	if err != nil {
		return err
	}
	if hint != "" {
		fmt.Printf("next: %s\n", hint)
	}
	return nil
}

const watchHelp = `usage: loopy watch [loop-id] [--once] [--no-color]

  The monitor. The rail lists every loop, most urgent first; the overview
  answers the live questions at a glance — what is running, whether it is
  converging (the iteration timeline), what the engine is doing right now,
  why a loop stopped, and the next command. Tabs switch to the full live
  tail, the cumulative diff, and the verifier log (tail-first, capped,
  truncation always labeled). Defaults to the loop that most needs eyes.

  Control from the monitor is limited to the safe, reversible moves —
  pause, resume, abort (with confirmation), and handing you the next
  command. Accept and reject stay in the CLI.

flags:
  --once       print one deterministic ANSI-free frame and exit (for
               scripts; honors COLUMNS for width, minimum 40)
  --no-color   disable color (NO_COLOR is also honored)

keys: ↑↓ select · enter drill in · tab/1-4 views · p pause · r resume ·
      a abort · o next-command hand-off · ? help · q quit`
