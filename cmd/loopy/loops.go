package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/mjbarefo/loopy/internal/loop"
)

func handleList(cwd string, args []string) error {
	asJSON, err := parseJSONFlag("list", args)
	if err != nil {
		return err
	}
	root, err := projectRoot(cwd)
	if err != nil {
		return err
	}
	if err := loop.EnsureInitialized(root); err != nil {
		return err
	}
	loops, err := loop.ListLoops(root)
	if err != nil {
		return err
	}
	views := make([]loop.LoopView, 0, len(loops))
	for _, l := range loops {
		v, err := loop.BuildLoopView(root, l)
		if err != nil {
			return err
		}
		views = append(views, v)
	}
	if asJSON {
		return printJSON(views)
	}
	if len(views) == 0 {
		fmt.Println("no loops yet — start one: loopy \"<goal>\"")
		return nil
	}
	for _, v := range views {
		fmt.Println(loop.RenderLoopLine(v))
	}
	return nil
}

func handleStatus(cwd string, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(discard{})
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return usageError{err}
	}
	root, err := projectRoot(cwd)
	if err != nil {
		return err
	}
	if err := loop.EnsureInitialized(root); err != nil {
		return err
	}

	loopID := fs.Arg(0)
	if loopID == "" {
		// No ID: most recent loop, the one you're most likely watching.
		loops, err := loop.ListLoops(root)
		if err != nil {
			return err
		}
		if len(loops) == 0 {
			return fmt.Errorf("no loops yet — start one: loopy \"<goal>\"")
		}
		loopID = loops[len(loops)-1].ID
	}
	l, err := loop.LoadLoop(root, loopID)
	if err != nil {
		return err
	}
	view, err := loop.BuildLoopView(root, l)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(view)
	}
	fmt.Print(loop.RenderStatus(view))
	return nil
}

func handleLog(cwd string, args []string) error {
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	fs.SetOutput(discard{})
	iterFlag := fs.Int("iter", -1, "show one iteration in detail")
	asJSON := fs.Bool("json", false, "machine-readable output")
	var loopID string
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		loopID = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return usageError{err}
	}
	if loopID == "" && fs.NArg() > 0 {
		loopID = fs.Arg(0)
	}
	if loopID == "" {
		return usagef("usage: loopy log <loop-id> [--iter N]")
	}
	root, err := projectRoot(cwd)
	if err != nil {
		return err
	}
	if err := loop.EnsureInitialized(root); err != nil {
		return err
	}
	l, err := loop.LoadLoop(root, loopID)
	if err != nil {
		return err
	}
	iterations, err := loop.LoadIterations(root, l.ID)
	if err != nil {
		return err
	}

	if *iterFlag >= 0 {
		for _, it := range iterations {
			if it.Index == *iterFlag {
				if *asJSON {
					return printJSON(it)
				}
				fmt.Print(loop.RenderIterationDetail(root, l.ID, it))
				return nil
			}
		}
		return fmt.Errorf("iteration %d not found (loop has %d)", *iterFlag, len(iterations))
	}

	if *asJSON {
		return printJSON(iterations)
	}
	for _, it := range iterations {
		fmt.Print(loop.RenderIterationDetail(root, l.ID, it))
		fmt.Println()
	}
	return nil
}

func handlePause(cwd string, args []string) error {
	root, loopID, err := loadControlTarget(cwd, args, "pause")
	if err != nil {
		return err
	}
	l, err := loop.LoadLoop(root, loopID)
	if err != nil {
		return err
	}
	if l.Done() {
		return fmt.Errorf("loop %s is already %s", l.ID, l.Status)
	}
	if l.Status == loop.StatusPaused {
		fmt.Printf("loop %s is already paused\n", l.ID)
		return nil
	}
	if err := loop.WriteControl(root, l.ID, loop.Control{Pause: true}); err != nil {
		return err
	}
	fmt.Printf("pause requested; the engine honors it at the next iteration boundary\n")
	return nil
}

func handleResume(cwd string, args []string) error {
	root, loopID, err := loadControlTarget(cwd, args, "resume")
	if err != nil {
		return err
	}
	l, err := loop.LoadLoop(root, loopID)
	if err != nil {
		return err
	}
	if l.Done() {
		return fmt.Errorf("loop %s is already %s", l.ID, l.Status)
	}
	if _, held, _ := loop.EngineLockState(root, l.ID); held {
		return fmt.Errorf("loop %s already has a live engine", l.ID)
	}
	if err := loop.ClearControl(root, l.ID); err != nil {
		return err
	}
	return driveEngine(root, l.ID)
}

func handleAbort(cwd string, args []string) error {
	fs := flag.NewFlagSet("abort", flag.ContinueOnError)
	fs.SetOutput(discard{})
	reason := fs.String("reason", "", "why the loop is being aborted (recorded verbatim)")
	var loopID string
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		loopID = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return usageError{err}
	}
	if loopID == "" && fs.NArg() > 0 {
		loopID = fs.Arg(0)
	}
	if loopID == "" {
		return usagef("usage: loopy abort <loop-id> [--reason text]")
	}
	root, err := projectRoot(cwd)
	if err != nil {
		return err
	}
	l, err := loop.LoadLoop(root, loopID)
	if err != nil {
		return err
	}
	if l.Done() {
		return fmt.Errorf("loop %s is already %s", l.ID, l.Status)
	}
	ctrl := loop.Control{Abort: true, Reason: *reason}
	if err := loop.WriteControl(root, l.ID, ctrl); err != nil {
		return err
	}
	if _, held, _ := loop.EngineLockState(root, l.ID); held {
		fmt.Printf("abort requested; the engine kills the running phase within seconds\n")
		return nil
	}
	// No live engine (paused or crashed): park directly so the state is
	// consistent without waiting for a resume.
	if err := loop.ParkAborted(root, l.ID, *reason); err != nil {
		return err
	}
	fmt.Printf("loop %s parked (no engine was running)\n", l.ID)
	return nil
}

func loadControlTarget(cwd string, args []string, verb string) (root, loopID string, err error) {
	if len(args) != 1 || args[0] == "" || args[0][0] == '-' {
		return "", "", usagef("usage: loopy %s <loop-id>", verb)
	}
	root, err = projectRoot(cwd)
	if err != nil {
		return "", "", err
	}
	if err := loop.EnsureInitialized(root); err != nil {
		return "", "", err
	}
	return root, args[0], nil
}

func parseJSONFlag(cmd string, args []string) (bool, error) {
	asJSON := false
	for _, arg := range args {
		if arg == "--json" {
			asJSON = true
		} else {
			return false, usagef("usage: loopy %s [--json]", cmd)
		}
	}
	return asJSON, nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
