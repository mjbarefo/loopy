package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mjbarefo/loopy/internal/loop"
	"github.com/mjbarefo/loopy/internal/tui"
)

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

  The monitor: loop list, live agent/verifier tailing, the iteration
  timeline, and drill-down viewers (diff, verifier log). Defaults to the
  newest loop.

  Control from the monitor is limited to the safe, reversible moves —
  pause, resume, abort (with confirmation), and handing you the next
  command. Accept and reject stay in the CLI.

flags:
  --once       print one deterministic ANSI-free frame and exit (for
               scripts; honors COLUMNS for width)
  --no-color   disable color (NO_COLOR is also honored)

keys: ↑↓ select · enter drill in · tab/1-4 views · p pause · r resume ·
      a abort · o review hand-off · ? help · q quit`
