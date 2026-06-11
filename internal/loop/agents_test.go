package loop

import (
	"strings"
	"testing"
)

func TestExpandAgentCommand(t *testing.T) {
	ctx := TemplateContext{
		Prompt:     "fix it; echo `pwned` 'quotes'",
		PromptFile: "/tmp/prompt.md",
		Worktree:   "/tmp/wt",
		LoopID:     "fix-it",
		Goal:       "fix it",
		Iteration:  3,
	}
	got, err := ExpandAgentCommand("claude -p {prompt} --dir {worktree} --iter {iteration}", ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := `claude -p 'fix it; echo ` + "`pwned`" + ` '\''quotes'\''' --dir '/tmp/wt' --iter '3'`
	if got != want {
		t.Errorf("expanded:\n  got  %s\n  want %s", got, want)
	}
}

func TestExpandAgentCommandUnknownVariable(t *testing.T) {
	_, err := ExpandAgentCommand("run {nope}", TemplateContext{})
	if err == nil || !strings.Contains(err.Error(), "{nope}") {
		t.Fatalf("expected unknown-variable error, got %v", err)
	}
}

func TestAgentRegistry(t *testing.T) {
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	if err := AddAgent(root, "Claude Code", "claude -p {prompt}", false); err != nil {
		t.Fatal(err)
	}
	// First agent becomes default even without --default.
	name, agent, err := ResolveAgent(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if name != "claude-code" || agent.Cmd != "claude -p {prompt}" {
		t.Fatalf("resolved %q %q", name, agent.Cmd)
	}

	if err := AddAgent(root, "codex", "codex exec {prompt}", true); err != nil {
		t.Fatal(err)
	}
	name, _, err = ResolveAgent(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if name != "codex" {
		t.Fatalf("default after --default add = %q, want codex", name)
	}

	if _, _, err := ResolveAgent(root, "ghost"); err == nil {
		t.Fatal("expected error for unregistered agent")
	}

	if err := RemoveAgent(root, "codex"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ResolveAgent(root, ""); err == nil {
		t.Fatal("expected error: default was removed")
	}
	if err := RemoveAgent(root, "ghost"); err == nil {
		t.Fatal("expected error removing unknown agent")
	}
}

func TestAddAgentValidation(t *testing.T) {
	root := t.TempDir()
	if _, _, err := InitProject(root); err != nil {
		t.Fatal(err)
	}
	if err := AddAgent(root, "", "cmd", false); err == nil {
		t.Fatal("expected error for empty name")
	}
	if err := AddAgent(root, "x", " ", false); err == nil {
		t.Fatal("expected error for empty command")
	}
	if err := AddAgent(root, "x", "run {bogus}", false); err == nil {
		t.Fatal("expected error for unknown template variable")
	}
}
