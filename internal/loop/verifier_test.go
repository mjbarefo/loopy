package loop

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunVerifierGreen(t *testing.T) {
	var log bytes.Buffer
	outcome, err := RunVerifier(context.Background(), t.TempDir(), []Stage{
		{Name: "a", Cmd: "echo one"},
		{Name: "b", Cmd: "echo two"},
	}, &log, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Green || len(outcome.Stages) != 2 {
		t.Fatalf("outcome = %+v", outcome)
	}
	if outcome.TailHash() != "" {
		t.Fatal("green outcome should have empty tail hash")
	}
	for _, want := range []string{"=== stage a", "one", "=== stage b", "two", "exit 0"} {
		if !strings.Contains(log.String(), want) {
			t.Fatalf("log missing %q:\n%s", want, log.String())
		}
	}
}

func TestRunVerifierShortCircuits(t *testing.T) {
	var log bytes.Buffer
	outcome, err := RunVerifier(context.Background(), t.TempDir(), []Stage{
		{Name: "fmt", Cmd: "echo fine"},
		{Name: "vet", Cmd: "echo broken hint; exit 3"},
		{Name: "test", Cmd: "echo never runs"},
	}, &log, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Green {
		t.Fatal("expected red")
	}
	if outcome.FailingStage != "vet" {
		t.Fatalf("failing stage = %q", outcome.FailingStage)
	}
	if len(outcome.Stages) != 2 {
		t.Fatalf("stages = %+v (test stage should not run)", outcome.Stages)
	}
	if outcome.Stages[1].ExitCode != 3 {
		t.Fatalf("exit = %d", outcome.Stages[1].ExitCode)
	}
	if !strings.Contains(outcome.FeedbackTail, "broken hint") {
		t.Fatalf("feedback tail = %q", outcome.FeedbackTail)
	}
	if strings.Contains(log.String(), "never runs") {
		t.Fatal("third stage ran after failure")
	}
	if outcome.TailHash() == "" {
		t.Fatal("red outcome needs a tail hash")
	}
}

func TestRunVerifierEmptyStages(t *testing.T) {
	if _, err := RunVerifier(context.Background(), t.TempDir(), nil, &bytes.Buffer{}, nil); err == nil {
		t.Fatal("expected error for empty verifier")
	}
}

func TestRunVerifierCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := RunVerifier(ctx, t.TempDir(), []Stage{{Name: "slow", Cmd: "sleep 30"}}, &bytes.Buffer{}, nil)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("cancellation took %v; process group kill is broken", elapsed)
	}
}

func TestTailString(t *testing.T) {
	long := strings.Repeat("x", 100) + "\nimportant tail line"
	got := tailString([]byte(long), 30)
	if got != "important tail line" {
		t.Fatalf("tail = %q", got)
	}
	if got := tailString([]byte("short\n"), 100); got != "short" {
		t.Fatalf("tail = %q", got)
	}
}
