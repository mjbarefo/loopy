package loop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSynthesizeVerifier: the scripted agent emits reasoning noise and a
// final command line; synthesis extracts the command, trial-runs it in the
// throwaway worktree, and reports red-at-baseline for unfinished work.
func TestSynthesizeVerifier(t *testing.T) {
	agent := `printf 'thinking about the goal...\nexploring the repo...\ntest -f done.txt\n'`
	root := newLoopProject(t, agent)
	res, err := SynthesizeVerifier(context.Background(), root, "scripted", "create done.txt")
	if err != nil {
		t.Fatal(err)
	}
	if res.Cmd != "test -f done.txt" {
		t.Fatalf("cmd = %q, want the agent's final line", res.Cmd)
	}
	if res.AlreadyGreen {
		t.Fatal("done.txt does not exist; the trial run must be red")
	}
	if res.Agent != "scripted" {
		t.Fatalf("agent = %q", res.Agent)
	}
}

// TestSynthesizeVerifierAlreadyGreen: a proposal that passes immediately is
// flagged — it does not test the goal, or the goal is already done.
func TestSynthesizeVerifierAlreadyGreen(t *testing.T) {
	root := newLoopProject(t, `echo true`)
	res, err := SynthesizeVerifier(context.Background(), root, "scripted", "anything")
	if err != nil {
		t.Fatal(err)
	}
	if !res.AlreadyGreen {
		t.Fatalf("a trivially-true proposal must be flagged, got %+v", res)
	}
}

// TestSynthesizeVerifierStripsDecoration: agents decorate despite
// instructions; fences, backticks, and a leading "$ " come off.
func TestSynthesizeVerifierStripsDecoration(t *testing.T) {
	agent := `printf 'Here is the command:\n` + "```" + `sh\n$ test -f x.txt\n` + "```" + `\n'`
	root := newLoopProject(t, agent)
	res, err := SynthesizeVerifier(context.Background(), root, "scripted", "create x.txt")
	if err != nil {
		t.Fatal(err)
	}
	if res.Cmd != "test -f x.txt" {
		t.Fatalf("cmd = %q, want decoration stripped", res.Cmd)
	}
}

// TestSynthesizeVerifierFailingAgent: an agent that errors out surfaces its
// own words, and the throwaway worktree is gone afterwards.
func TestSynthesizeVerifierFailingAgent(t *testing.T) {
	root := newLoopProject(t, `echo "not authenticated" >&2; exit 7`)
	_, err := SynthesizeVerifier(context.Background(), root, "scripted", "anything")
	if err == nil || !strings.Contains(err.Error(), "not authenticated") {
		t.Fatalf("err = %v, want the agent's words", err)
	}
}

// TestSynthesisLeavesTheRepoAlone: synthesis runs in a throwaway worktree —
// an agent that scribbles there must not touch the user's checkout, and no
// worktree may linger.
func TestSynthesisLeavesTheRepoAlone(t *testing.T) {
	agent := `echo scribble > scribble.txt; printf 'test -f done.txt\n'`
	root := newLoopProject(t, agent)
	if _, err := SynthesizeVerifier(context.Background(), root, "scripted", "anything"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "scribble.txt")); !os.IsNotExist(err) {
		t.Fatal("the agent's scribble reached the user's checkout")
	}
	out, err := gitOutput(root, "worktree", "list", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "loopy-verifier-synth-") {
		t.Fatalf("throwaway worktree lingers:\n%s", out)
	}
}
