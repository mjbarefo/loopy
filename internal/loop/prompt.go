package loop

import (
	"fmt"
	"strings"
)

// promptMaxChangedFiles caps the changed-files list carried into a prompt.
const promptMaxChangedFiles = 50

// ComposePrompt builds the full prompt for one agent iteration: goal,
// constraints, the verifier definition, the previous iteration's failure
// feedback, and a bounded summary of changes so far. Everything the agent
// needs, nothing unbounded — the model forgets between runs; this document is
// its entire context from loopy.
func ComposePrompt(l Loop, index int, prev *Iteration) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# loopy — iteration %d of %d in loop %q\n\n", index, l.Budget.MaxIterations, l.ID)

	b.WriteString("## Goal\n\n")
	b.WriteString(strings.TrimSpace(l.Goal))
	b.WriteString("\n")

	if len(l.Constraints) > 0 {
		b.WriteString("\n## Constraints\n\n")
		for _, c := range l.Constraints {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}

	if len(l.ForbiddenPaths) > 0 {
		b.WriteString("\n## Forbidden paths\n\n")
		b.WriteString("Do not create, modify, or delete anything under these paths; an iteration that touches them fails:\n\n")
		for _, p := range l.ForbiddenPaths {
			fmt.Fprintf(&b, "- %s\n", p)
		}
	}

	b.WriteString("\n## Verifier\n\n")
	b.WriteString("The loop is green when all of these commands exit 0, run in order from the worktree root. They run automatically after you exit; you may also run them yourself:\n\n")
	for i, stage := range l.Verifier {
		fmt.Fprintf(&b, "%d. %s: `%s`\n", i+1, stage.Name, stage.Cmd)
	}

	if prev != nil {
		b.WriteString("\n## Feedback from the last verification\n\n")
		switch {
		case prev.Violation != "":
			fmt.Fprintf(&b, "Iteration %d failed before verification: %s\nUndo those changes and stay within the allowed paths.\n", prev.Index, prev.Violation)
		case prev.FailingStage != "":
			label := fmt.Sprintf("iteration %d", prev.Index)
			if prev.Index == 0 {
				label = "the baseline check (before any agent ran)"
			}
			fmt.Fprintf(&b, "In %s, verifier stage `%s` failed. Output tail:\n\n```text\n%s\n```\n", label, prev.FailingStage, prev.FeedbackTail)
		}
		if len(prev.ChangedFiles) > 0 {
			fmt.Fprintf(&b, "\n## Changes so far (cumulative, vs base commit %s)\n\n", shortCommit(l.BaseCommit))
			files := prev.ChangedFiles
			truncated := 0
			if len(files) > promptMaxChangedFiles {
				truncated = len(files) - promptMaxChangedFiles
				files = files[:promptMaxChangedFiles]
			}
			for _, f := range files {
				fmt.Fprintf(&b, "- %s\n", f)
			}
			if truncated > 0 {
				fmt.Fprintf(&b, "- … and %d more\n", truncated)
			}
		}
	}

	b.WriteString(`
## Rules

- You are one iteration of an autonomous loop. Make concrete progress toward the goal, then exit.
- Work only inside this directory (the loop's isolated git worktree).
- Do not commit, push, switch branches, or create branches. loopy snapshots your changes automatically.
- Do not edit anything under .loopy/.
- When the verifier passes, the loop ends and a human reviews the full diff.
`)
	return b.String()
}

func shortCommit(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}
