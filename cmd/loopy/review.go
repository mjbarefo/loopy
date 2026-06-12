package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mjbarefo/loopy/internal/loop"
)

func handleReview(cwd string, args []string) error {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	fs.SetOutput(discard{})
	asJSON := fs.Bool("json", false, "machine-readable output")
	loopID, rest := splitLeadingID(args)
	if err := fs.Parse(rest); err != nil {
		return usageError{err}
	}
	if loopID == "" && fs.NArg() > 0 {
		loopID = fs.Arg(0)
	}
	if loopID == "" {
		return usagef("usage: loopy review <loop-id> [--json]")
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
	view, err := loop.BuildLoopView(root, l)
	if err != nil {
		return err
	}
	review, decided, err := loop.LoadReview(root, l.ID)
	if err != nil {
		return err
	}
	var reviewPtr *loop.Review
	if decided {
		reviewPtr = &review
	}

	// The verifier transcript of the last iteration that ran stages.
	transcript, transcriptIter := "", 0
	iterations, err := loop.LoadIterations(root, l.ID)
	if err != nil {
		return err
	}
	for i := len(iterations) - 1; i >= 0; i-- {
		if len(iterations[i].Stages) == 0 {
			continue
		}
		path := filepath.Join(loop.IterationDir(root, l.ID, iterations[i].Index), loop.VerifierLogFile)
		if data, _, _, err := loop.TailFile(path, loop.ViewerCapBytes); err == nil {
			transcript, transcriptIter = string(data), iterations[i].Index
		}
		break
	}

	// The diff under review: the durable copy once accepted, otherwise the
	// last cumulative diff.
	var diff []byte
	if view.FinalDiffPath != "" {
		if data, err := os.ReadFile(view.FinalDiffPath); err == nil {
			diff = data
		}
	}

	critique := ""
	if view.CritiquePath != "" {
		if data, _, _, err := loop.TailFile(view.CritiquePath, loop.ViewerCapBytes); err == nil {
			critique = string(data)
		}
	}

	if *asJSON {
		return printJSON(struct {
			Loop   loop.LoopView `json:"loop"`
			Review *loop.Review  `json:"review,omitempty"`
		}{view, reviewPtr})
	}
	fmt.Print(loop.RenderReview(view, reviewPtr, transcript, transcriptIter, diff, critique))
	return nil
}

func handleAccept(cwd string, args []string) error {
	fs := flag.NewFlagSet("accept", flag.ContinueOnError)
	fs.SetOutput(discard{})
	override := fs.Bool("override", false, "accept a non-green loop (requires --reason, recorded verbatim)")
	reason := fs.String("reason", "", "why this override is justified")
	loopID, rest := splitLeadingID(args)
	if err := fs.Parse(rest); err != nil {
		return usageError{err}
	}
	if loopID == "" && fs.NArg() > 0 {
		loopID = fs.Arg(0)
	}
	if loopID == "" {
		return usagef("usage: loopy accept <loop-id> [--override --reason text]")
	}
	root, err := projectRoot(cwd)
	if err != nil {
		return err
	}
	if err := loop.EnsureInitialized(root); err != nil {
		return err
	}
	r, err := loop.Accept(root, loopID, *override, *reason)
	if err != nil {
		return err
	}
	if r.Override {
		fmt.Printf("accepted %s with override (recorded verbatim): %s\n", r.LoopID, r.Reason)
	} else {
		fmt.Printf("accepted %s\n", r.LoopID)
	}
	if r.FinalDiff != "" {
		fmt.Printf("durable diff: %s\napply it: git apply %s\n", r.FinalDiff, r.FinalDiff)
	} else {
		fmt.Println("no diff to apply (green at baseline)")
	}
	fmt.Printf("logbook: %s\n", loop.LogbookPath(root))
	return nil
}

func handleReject(cwd string, args []string) error {
	fs := flag.NewFlagSet("reject", flag.ContinueOnError)
	fs.SetOutput(discard{})
	reason := fs.String("reason", "", "why the result was declined (recorded in the logbook)")
	loopID, rest := splitLeadingID(args)
	if err := fs.Parse(rest); err != nil {
		return usageError{err}
	}
	if loopID == "" && fs.NArg() > 0 {
		loopID = fs.Arg(0)
	}
	if loopID == "" {
		return usagef("usage: loopy reject <loop-id> [--reason text]")
	}
	root, err := projectRoot(cwd)
	if err != nil {
		return err
	}
	if err := loop.EnsureInitialized(root); err != nil {
		return err
	}
	r, err := loop.Reject(root, loopID, *reason)
	if err != nil {
		return err
	}
	fmt.Printf("rejected %s — evidence preserved under %s, worktree freed\n", r.LoopID, loop.LoopDir(root, r.LoopID))
	fmt.Printf("logbook: %s\n", loop.LogbookPath(root))
	return nil
}

func handleLogbook(cwd string, args []string) error {
	asJSON, err := parseJSONFlag("logbook", args)
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
	entries, err := loop.LogbookEntries(root)
	if err != nil {
		return err
	}
	if asJSON {
		if entries == nil {
			entries = []loop.Review{}
		}
		return printJSON(entries)
	}
	if len(entries) == 0 {
		fmt.Println("no decisions recorded yet — loops land here after `loopy accept` or `loopy reject`")
		return nil
	}
	for _, r := range entries {
		fmt.Println(loop.RenderLogbookEntry(r))
	}
	fmt.Printf("\nfull narrative: %s\n", loop.LogbookPath(root))
	return nil
}

func handleJudge(cwd string, args []string) error {
	asJSON := false
	var ids []string
	for _, arg := range args {
		if arg == "--json" {
			asJSON = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return usagef("usage: loopy judge <loop-id> <loop-id> [...] [--json]")
		}
		ids = append(ids, arg)
	}
	if len(ids) < 2 {
		return usagef("usage: loopy judge <loop-id> <loop-id> [...] — the judge compares finished loops")
	}
	root, err := projectRoot(cwd)
	if err != nil {
		return err
	}
	if err := loop.EnsureInitialized(root); err != nil {
		return err
	}
	verdict, err := loop.Judge(root, ids)
	if err != nil {
		return err
	}
	if asJSON {
		return printJSON(verdict)
	}
	fmt.Print(loop.RenderVerdict(verdict))
	return nil
}

// splitLeadingID peels a leading non-flag argument (the loop ID) so both
// `loopy accept <id> --reason x` and `loopy accept --reason x <id>` work.
func splitLeadingID(args []string) (string, []string) {
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		return args[0], args[1:]
	}
	return "", args
}
