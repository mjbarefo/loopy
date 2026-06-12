package loop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SynthesisTimeout bounds one verifier-synthesis run. The agent explores the
// repository before answering, so this is roomier than the smoke check.
const SynthesisTimeout = 5 * time.Minute

// SynthesisResult is a proposed verifier plus its trial verdict, gathered in
// a throwaway worktree. The proposal is never used without the human's
// confirmation — invariant 1 holds, the human just stops writing shell.
type SynthesisResult struct {
	Agent string `json:"agent"`
	Cmd   string `json:"cmd"`
	// AlreadyGreen: the proposed command passed in the throwaway worktree,
	// meaning it does not test the goal — or the goal is already done.
	// Either way the human must know before spending budget.
	AlreadyGreen bool  `json:"already_green"`
	AgentMS      int64 `json:"agent_ms"`
}

// SynthesizeVerifier asks a registered agent to propose a shell command that
// is red until the goal is achieved, then trial-runs the proposal once. Both
// happen in a throwaway worktree at HEAD, so a --yolo agent can explore (and
// even scribble) without touching the user's checkout. loopy itself still
// makes no model calls (invariant 4): the agent is the same registered
// external command that runs the loop.
func SynthesizeVerifier(root, agentName, goal string) (SynthesisResult, error) {
	name, agent, err := ResolveAgent(root, agentName)
	if err != nil {
		return SynthesisResult{}, err
	}
	res := SynthesisResult{Agent: name}

	dir, cleanup, err := throwawayWorktree(root)
	if err != nil {
		return res, err
	}
	defer cleanup()

	prompt := synthesisPrompt(goal)
	promptPath := filepath.Join(dir, ".loopy-verifier-prompt.md")
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return res, err
	}
	command, err := ExpandAgentCommand(agent.Cmd, TemplateContext{
		Prompt:     prompt,
		PromptFile: promptPath,
		Worktree:   dir,
		LoopID:     "verifier-synthesis",
		Goal:       goal,
		Iteration:  0,
	})
	if err != nil {
		return res, err
	}

	logPath := filepath.Join(dir, ".loopy-verifier-synthesis.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return res, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), SynthesisTimeout)
	defer cancel()
	start := time.Now()
	_, exit, runErr := runShell(ctx, dir, command, logFile)
	_ = logFile.Close()
	res.AgentMS = time.Since(start).Milliseconds()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return res, fmt.Errorf("agent %s timed out after %s proposing a verifier", name, SynthesisTimeout)
	}
	if runErr != nil {
		return res, runErr
	}
	if exit != 0 {
		words := lastAgentWords(logPath)
		if words == "" {
			words = "no output"
		}
		return res, fmt.Errorf("agent %s exited %d proposing a verifier: %s", name, exit, words)
	}

	res.Cmd = proposedCommand(logPath)
	if res.Cmd == "" {
		return res, fmt.Errorf("agent %s produced no usable command (last lines of its output are in the transcript)", name)
	}

	// Trial-run the proposal where the agent wrote it. Red is the desired
	// answer: the goal is not done yet, so the loop has work to do.
	trialCtx, trialCancel := context.WithTimeout(context.Background(), SynthesisTimeout)
	defer trialCancel()
	_, trialExit, trialErr := runShell(trialCtx, dir, res.Cmd, io.Discard)
	if trialErr != nil {
		return res, fmt.Errorf("trial run of the proposed verifier failed to start: %w", trialErr)
	}
	res.AlreadyGreen = trialExit == 0
	return res, nil
}

// throwawayWorktree adds a detached worktree at HEAD under the system temp
// directory; cleanup removes it. Unlike loop worktrees it creates no branch
// and keeps nothing.
func throwawayWorktree(root string) (string, func(), error) {
	if err := EnsureWorktreePreconditions(root); err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp("", "loopy-verifier-synth-")
	if err != nil {
		return "", nil, err
	}
	// git refuses to add a worktree at an existing directory; hand it a
	// child path instead.
	path := filepath.Join(dir, "worktree")
	if _, err := gitOutput(root, "worktree", "add", "--detach", path, "HEAD"); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, err
	}
	cleanup := func() {
		_, _ = gitOutput(root, "worktree", "remove", "--force", path)
		_ = os.RemoveAll(dir)
	}
	return path, cleanup, nil
}

func synthesisPrompt(goal string) string {
	return fmt.Sprintf(`You are configuring the automated verifier for a coding-agent loop.

Goal of the loop:

%s

Explore this repository as needed, then propose ONE POSIX shell command that:
- exits NONZERO right now, because the goal is not achieved yet
- will exit ZERO exactly when the goal is achieved
- never modifies the repository and is safe to run repeatedly
- chains the project's own gate when one exists and stays fast (for example: test -f docs/x.md && make check)

Do not create, edit, or delete any files. Your FINAL output line must be the bare command and nothing else: no backticks, no prose, no leading $.
`, goal)
}

// proposedCommand extracts the agent's final non-empty transcript line and
// strips decoration agents add despite instructions (fences, backticks, a
// leading "$ "). The human confirms the result before it becomes a verifier,
// so a bad parse is an inconvenience, never an execution.
func proposedCommand(logPath string) string {
	data, _, _, err := TailFile(logPath, 16*1024)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(stripANSI(lines[i]))
		line = strings.TrimPrefix(line, "$ ")
		line = strings.Trim(line, "`")
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		return line
	}
	return ""
}
