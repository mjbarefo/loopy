package loop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// FeedbackTailBytes bounds how much failing-stage output is carried into the
// next prompt (and into iteration.json). Full output always lands in
// verifier.log.
const FeedbackTailBytes = 4096

// AskTimeout bounds one ask stage's agent call. It reads the worktree and
// answers a yes/no question — far cheaper than synthesis's explore-and-design,
// so the cap is tighter. A slow agent fails the stage closed; it never hangs
// the loop.
const AskTimeout = 2 * time.Minute

// askPromptFile is the throwaway prompt an ask stage hands its agent, written
// into the worktree and removed after.
const askPromptFile = ".loopy-ask-prompt.md"

// AskContext carries what an ask stage needs to run an agent. The engine
// always supplies one; a nil context with an ask stage present is a config
// error (RunVerifier fails that stage closed). Command-only verifiers ignore
// it, so tests may pass nil.
type AskContext struct {
	Root  string // resolves the registered agent
	Goal  string // the loop's goal, for the judge prompt
	Agent string // default agent for ask stages: the loop's own agent
	Diff  []byte // cumulative diff vs base, included as orientation
}

// VerifierOutcome is the result of running a loop's verifier once.
type VerifierOutcome struct {
	Stages       []StageResult
	Green        bool
	FailingStage string
	// FeedbackTail is the bounded tail of the first failing stage's output.
	FeedbackTail string
}

// TailHash fingerprints the failure for stuck detection: same failing stage +
// same output tail across iterations means the agent is going in circles.
func (v VerifierOutcome) TailHash() string {
	if v.Green {
		return ""
	}
	return hashBytes([]byte(v.FailingStage + "\x00" + v.FeedbackTail))
}

// RunVerifier executes stages in order inside dir, short-circuiting on the
// first failure. Full per-stage output is streamed to log; the failing
// stage's bounded tail comes back in the outcome. Cancelling ctx kills the
// running stage's process group.
func RunVerifier(ctx context.Context, dir string, stages []Stage, log io.Writer, ask *AskContext) (VerifierOutcome, error) {
	if len(stages) == 0 {
		// Defense in depth: creation refuses empty verifiers, so this is a
		// corrupted loop.json, not a usable state.
		return VerifierOutcome{}, fmt.Errorf("loop has no verifier stages")
	}
	outcome := VerifierOutcome{Green: true}
	for _, stage := range stages {
		start := time.Now()
		var (
			output   []byte
			exitCode int
			err      error
		)
		switch stage.kind() {
		case KindAsk:
			output, exitCode, err = runAskStage(ctx, dir, stage, ask, log)
		default:
			fmt.Fprintf(log, "=== stage %s: %s\n", stage.Name, stage.Cmd)
			output, exitCode, err = runShell(ctx, dir, stage.Cmd, log)
		}
		if err != nil {
			return outcome, err
		}
		result := StageResult{
			Name:       stage.Name,
			Kind:       stage.kind(),
			Cmd:        stage.Descriptor(),
			ExitCode:   exitCode,
			DurationMS: time.Since(start).Milliseconds(),
		}
		outcome.Stages = append(outcome.Stages, result)
		fmt.Fprintf(log, "=== stage %s: exit %d (%s)\n", stage.Name, exitCode, time.Since(start).Round(time.Millisecond))
		if exitCode != 0 {
			outcome.Green = false
			outcome.FailingStage = stage.Name
			outcome.FeedbackTail = tailString(output, FeedbackTailBytes)
			break
		}
	}
	return outcome, nil
}

// runAskStage asks a registered agent the stage's question about the current
// worktree and maps its verdict to an exit code (PASS -> 0, FAIL -> 1). The
// agent's full reasoning streams to log; the returned bytes are the FAIL
// reason, which becomes the next prompt's feedback. It fails closed: a timeout,
// an unrunnable agent, or a missing verdict all return FAIL — loopy never
// makes a model call of its own, so a stuck judge can't hang the loop.
//
// Parent-context cancellation (abort/pause) propagates as an error exactly like
// a command stage; only the per-stage AskTimeout fails closed.
func runAskStage(ctx context.Context, dir string, stage Stage, ask *AskContext, log io.Writer) ([]byte, int, error) {
	if ask == nil {
		return nil, 0, fmt.Errorf("verifier stage %q is an ask stage but no agent context was provided", stage.Name)
	}
	agentName := strings.TrimSpace(stage.Agent)
	if agentName == "" {
		agentName = ask.Agent
	}
	name, agent, err := ResolveAgent(ask.Root, agentName)
	if err != nil {
		return nil, 0, fmt.Errorf("ask stage %q: %w", stage.Name, err)
	}

	prompt := askPrompt(ask.Goal, stage.Ask, ask.Diff)
	promptPath := filepath.Join(dir, askPromptFile)
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return nil, 0, err
	}
	defer func() { _ = os.Remove(promptPath) }()

	command, err := ExpandAgentCommand(agent.Cmd, TemplateContext{
		Prompt:     prompt,
		PromptFile: promptPath,
		Worktree:   dir,
		LoopID:     "verifier-ask",
		Goal:       ask.Goal,
		Iteration:  0,
	})
	if err != nil {
		return nil, 0, err
	}

	fmt.Fprintf(log, "=== stage %s (ask %s): %s\n", stage.Name, name, stage.Ask)
	actx, cancel := context.WithTimeout(ctx, AskTimeout)
	defer cancel()
	out, _, runErr := runShell(actx, dir, command, log)

	// Parent cancellation outranks everything: surface it as an error so the
	// engine treats it as an abort, not a verdict.
	if ctx.Err() != nil {
		return out, -1, ctx.Err()
	}
	if errors.Is(actx.Err(), context.DeadlineExceeded) {
		return failClosed(log, fmt.Sprintf("agent %s did not answer within %s", name, AskTimeout))
	}
	if runErr != nil {
		return failClosed(log, fmt.Sprintf("agent %s could not be run: %v", name, runErr))
	}

	if pass, reason := parseVerdict(out); pass {
		fmt.Fprintln(log, "=== ask verdict: PASS")
		return nil, 0, nil
	} else {
		return failClosed(log, reason)
	}
}

// failClosed records a FAIL verdict and returns it as the stage's feedback.
func failClosed(log io.Writer, reason string) ([]byte, int, error) {
	fmt.Fprintf(log, "=== ask verdict: FAIL (%s)\n", reason)
	return []byte("FAIL: " + reason), 1, nil
}

// askPrompt composes the judge prompt: the goal, the question, and the
// cumulative diff for orientation (the worktree already contains the changes).
func askPrompt(goal, question string, diff []byte) string {
	const diffCap = 16 * 1024
	d := string(diff)
	note := ""
	if len(d) > diffCap {
		d = d[len(d)-diffCap:]
		note = "(diff truncated to the last 16 KiB; read the files directly for the rest)\n"
	}
	if strings.TrimSpace(d) == "" {
		d = "(no changes yet — the working tree matches the base commit)"
	}
	return fmt.Sprintf(`You are the verifier for a coding-agent loop. Judge whether the goal is met in the CURRENT state of this worktree. Do NOT modify anything; this is a read-only check.

Goal of the loop:

%s

Question to answer:

%s

You may read any file in this worktree and run read-only commands to check your answer. The cumulative diff since the loop started, for orientation (the worktree already contains these changes):

%s%s

Decide, then make your FINAL output line EXACTLY one of:
  PASS
  FAIL: <one concrete sentence on what is still missing or wrong>

Put nothing after the verdict line — no backticks, no prose. When in doubt, answer FAIL.
`, goal, question, note, d)
}

// parseVerdict reads the agent's final non-empty line and maps it to a verdict.
// Anything that is not a clear PASS/FAIL fails closed, with the offending line
// quoted so the next iteration knows the agent broke protocol.
func parseVerdict(out []byte) (pass bool, reason string) {
	lines := strings.Split(string(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(stripANSI(lines[i]))
		line = strings.TrimPrefix(line, "$ ")
		line = strings.TrimSpace(strings.Trim(line, "`"))
		if line == "" {
			continue
		}
		up := strings.ToUpper(line)
		switch {
		case up == "PASS" || strings.HasPrefix(up, "PASS:") || strings.HasPrefix(up, "PASS "):
			return true, ""
		case strings.HasPrefix(up, "FAIL"):
			r := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line[len("FAIL"):]), ":"))
			if r == "" {
				r = "the agent reported the goal is not met"
			}
			return false, r
		default:
			return false, "agent did not end with a PASS or FAIL verdict (last line: " + truncateInline(line, 120) + ")"
		}
	}
	return false, "agent produced no verdict"
}

// truncateInline shortens s to n runes for a one-line reason.
func truncateInline(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// runShell runs cmd via `sh -c` in dir, streaming combined output to stream
// while retaining a bounded tail for feedback. The process gets its own
// process group so cancellation kills the whole tree.
func runShell(ctx context.Context, dir, cmd string, stream io.Writer) (tail []byte, exitCode int, err error) {
	var buf tailBuffer
	out := io.MultiWriter(stream, &buf)
	c := exec.Command("sh", "-c", cmd)
	c.Dir = dir
	c.Stdout = out
	c.Stderr = out
	setProcessGroup(c)
	if err := c.Start(); err != nil {
		return nil, 0, err
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case <-ctx.Done():
		killProcessGroup(c)
		<-done
		return buf.Bytes(), -1, ctx.Err()
	case waitErr := <-done:
		if waitErr != nil {
			var exitErr *exec.ExitError
			if errors.As(waitErr, &exitErr) {
				return buf.Bytes(), exitErr.ExitCode(), nil
			}
			return buf.Bytes(), 1, waitErr
		}
		return buf.Bytes(), 0, nil
	}
}

// tailBuffer keeps only the last FeedbackTailBytes*2 bytes written, enough to
// extract a clean tail without holding giant outputs in memory.
type tailBuffer struct {
	data []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	if max := FeedbackTailBytes * 2; len(b.data) > max {
		b.data = append([]byte(nil), b.data[len(b.data)-max:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) Bytes() []byte { return b.data }

// tailString returns the last n bytes of data as a string, starting at a line
// boundary when one is available.
func tailString(data []byte, n int) string {
	if len(data) > n {
		data = data[len(data)-n:]
		if idx := strings.IndexByte(string(data), '\n'); idx >= 0 && idx+1 < len(data) {
			data = data[idx+1:]
		}
	}
	return strings.TrimRight(string(data), "\n")
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}
