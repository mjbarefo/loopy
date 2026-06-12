package loop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// AgentCheckTimeout bounds one smoke run; a healthy agent CLI answers a
// trivial prompt well inside this.
const AgentCheckTimeout = 2 * time.Minute

// AgentBinary extracts the executable a command template invokes: the first
// token that is not a VAR=value environment assignment. Empty when the
// template has no tokens.
func AgentBinary(template string) string {
	for _, tok := range strings.Fields(template) {
		if i := strings.IndexByte(tok, '='); i > 0 && !strings.ContainsAny(tok[:i], `/'"{$`) {
			continue // a FOO=bar prefix assignment, not the binary
		}
		return tok
	}
	return ""
}

// MissingAgentBinary names the template's executable when it cannot be found
// on PATH; empty when present, or when the token is shell-quoted/expanded
// and no verdict is safe.
func MissingAgentBinary(template string) string {
	bin := AgentBinary(template)
	if bin == "" || strings.ContainsAny(bin, "'\"$`(") {
		return ""
	}
	if strings.Contains(bin, "/") {
		if _, err := os.Stat(bin); err != nil {
			return bin
		}
		return ""
	}
	if _, err := exec.LookPath(bin); err != nil {
		return bin
	}
	return ""
}

// AgentCheckResult is one smoke run's verdict.
type AgentCheckResult struct {
	Name   string `json:"name"`
	Cmd    string `json:"cmd"`
	OK     bool   `json:"ok"`
	Exit   int    `json:"exit"`
	Words  string `json:"words,omitempty"` // the transcript's last non-empty line
	WallMS int64  `json:"wall_ms"`
}

// CheckAgent smoke-runs one registered agent outside any loop: a trivial
// prompt in a throwaway directory, so trust prompts, dead auth, and missing
// binaries surface in seconds instead of parking a real loop as "agent
// blocked". It spends one (tiny) model call, which is why it is an explicit
// command rather than a side effect of registration.
func CheckAgent(root, name string) (AgentCheckResult, error) {
	name, agent, err := ResolveAgent(root, name)
	if err != nil {
		return AgentCheckResult{}, err
	}
	res := AgentCheckResult{Name: name, Cmd: agent.Cmd}

	dir, err := os.MkdirTemp("", "loopy-agent-check-")
	if err != nil {
		return res, err
	}
	defer os.RemoveAll(dir)
	// A loop worktree is always a git repository, and some agent CLIs refuse
	// to run outside one (codex). The throwaway must be representative.
	if _, err := runGitChecked(dir, nil, "init", "-q"); err != nil {
		return res, err
	}

	prompt := "This is a connectivity check. Reply with the single word OK. Do not run tools and do not create or modify any files."
	promptPath := filepath.Join(dir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte(prompt+"\n"), 0o644); err != nil {
		return res, err
	}
	command, err := ExpandAgentCommand(agent.Cmd, TemplateContext{
		Prompt:     prompt,
		PromptFile: promptPath,
		Worktree:   dir,
		LoopID:     "agent-check",
		Goal:       "agent connectivity check",
		Iteration:  0,
	})
	if err != nil {
		return res, err
	}

	logPath := filepath.Join(dir, "agent.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return res, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), AgentCheckTimeout)
	defer cancel()
	start := time.Now()
	_, exit, runErr := runShell(ctx, dir, command, logFile)
	closeErr := logFile.Close()
	res.WallMS = time.Since(start).Milliseconds()
	res.Exit = exit
	res.Words = lastAgentWords(logPath)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		res.Words = fmt.Sprintf("timed out after %s", AgentCheckTimeout)
		return res, nil
	}
	if runErr != nil {
		return res, runErr
	}
	if closeErr != nil {
		return res, closeErr
	}
	res.OK = exit == 0
	return res, nil
}
