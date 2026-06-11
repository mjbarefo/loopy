package loop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// FeedbackTailBytes bounds how much failing-stage output is carried into the
// next prompt (and into iteration.json). Full output always lands in
// verifier.log.
const FeedbackTailBytes = 4096

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
func RunVerifier(ctx context.Context, dir string, stages []Stage, log io.Writer) (VerifierOutcome, error) {
	if len(stages) == 0 {
		// Defense in depth: creation refuses empty verifiers, so this is a
		// corrupted loop.json, not a usable state.
		return VerifierOutcome{}, fmt.Errorf("loop has no verifier stages")
	}
	outcome := VerifierOutcome{Green: true}
	for _, stage := range stages {
		fmt.Fprintf(log, "=== stage %s: %s\n", stage.Name, stage.Cmd)
		start := time.Now()
		output, exitCode, err := runShell(ctx, dir, stage.Cmd, log)
		if err != nil {
			return outcome, err
		}
		result := StageResult{
			Name:       stage.Name,
			Cmd:        stage.Cmd,
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
