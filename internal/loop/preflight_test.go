package loop

import (
	"strings"
	"testing"
)

func TestAgentBinary(t *testing.T) {
	cases := []struct{ template, want string }{
		{"claude -p {prompt} --permission-mode acceptEdits", "claude"},
		{"GEMINI_CLI_TRUST_WORKSPACE=true gemini -p {prompt} --yolo", "gemini"},
		{"FOO=1 BAR=2 ./scripts/agent.sh {prompt_file}", "./scripts/agent.sh"},
		{"   ", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := AgentBinary(c.template); got != c.want {
			t.Errorf("AgentBinary(%q) = %q, want %q", c.template, got, c.want)
		}
	}
}

func TestMissingAgentBinary(t *testing.T) {
	if got := MissingAgentBinary("sh -c 'true'"); got != "" {
		t.Errorf("sh should be present, got missing %q", got)
	}
	if got := MissingAgentBinary("definitely-not-a-real-binary-xyz {prompt}"); got != "definitely-not-a-real-binary-xyz" {
		t.Errorf("missing binary not reported, got %q", got)
	}
	// Quoted or shell-expanded tokens get no verdict rather than a wrong one.
	if got := MissingAgentBinary(`"$AGENT" {prompt}`); got != "" {
		t.Errorf("no verdict expected for shell-expanded binaries, got %q", got)
	}
}

// TestCheckAgent smoke-runs scripted agents: a healthy one passes, a
// refusing one fails with its own words carried in the result.
func TestCheckAgent(t *testing.T) {
	root := newLoopProject(t, "printf OK")
	res, err := CheckAgent(root, "scripted")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Exit != 0 {
		t.Fatalf("healthy agent failed the check: %+v", res)
	}

	if err := AddAgent(root, "refuser", `echo "demo CLI: workspace not trusted" >&2; exit 55`, false); err != nil {
		t.Fatal(err)
	}
	res, err = CheckAgent(root, "refuser")
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.Exit != 55 {
		t.Fatalf("refusing agent passed the check: %+v", res)
	}
	if !strings.Contains(res.Words, "workspace not trusted") {
		t.Fatalf("words = %q, want the agent's own refusal", res.Words)
	}
}

// TestDoctorWarnsOnMissingAgentBinary: a registered agent whose binary is
// absent from PATH is a doctor warning, named per agent.
func TestDoctorWarnsOnMissingAgentBinary(t *testing.T) {
	root := newLoopProject(t, "true")
	if err := AddAgent(root, "ghost", "definitely-not-a-real-binary-xyz {prompt}", false); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range Doctor(root) {
		if c.Status == DoctorWarn && strings.Contains(c.Detail, "ghost") && strings.Contains(c.Detail, "definitely-not-a-real-binary-xyz") {
			found = true
		}
	}
	if !found {
		t.Fatal("doctor should warn about the ghost agent's missing binary")
	}
}
