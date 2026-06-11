package loop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var agentVariablePattern = regexp.MustCompile(`\{[a-z_]+\}`)

// TemplateContext carries the values substituted into an agent command for
// one iteration.
type TemplateContext struct {
	Prompt     string // full prompt text
	PromptFile string // path to prompt.md for agents that prefer a file
	Worktree   string
	LoopID     string
	Goal       string
	Iteration  int
}

// ExpandAgentCommand substitutes {prompt}, {prompt_file}, {worktree},
// {loop_id}, {goal}, {iteration} into an agent command template. Every value
// is shell-quoted: the prompt carries verifier output, and unquoted that
// would let the code under test inject shell into the agent invocation.
func ExpandAgentCommand(template string, ctx TemplateContext) (string, error) {
	values := map[string]string{
		"prompt":      ctx.Prompt,
		"prompt_file": ctx.PromptFile,
		"worktree":    ctx.Worktree,
		"loop_id":     ctx.LoopID,
		"goal":        ctx.Goal,
		"iteration":   fmt.Sprintf("%d", ctx.Iteration),
	}
	var expandErr error
	expanded := agentVariablePattern.ReplaceAllStringFunc(template, func(token string) string {
		key := token[1 : len(token)-1]
		value, ok := values[key]
		if !ok {
			expandErr = fmt.Errorf("unknown agent template variable %s", token)
			return token
		}
		return shellQuote(value)
	})
	if expandErr != nil {
		return "", expandErr
	}
	return expanded, nil
}

// shellQuote wraps s in single quotes, escaping embedded single quotes, so
// the value passes through `sh -c` verbatim.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func agentsPath(root string) string {
	return filepath.Join(LoopyPath(root), "agents.json")
}

// LoadAgents reads the registry; a missing file is an empty registry.
func LoadAgents(root string) (AgentRegistry, error) {
	var reg AgentRegistry
	if err := ReadJSON(agentsPath(root), &reg); err != nil && !errors.Is(err, os.ErrNotExist) {
		return AgentRegistry{}, err
	}
	if reg.Agents == nil {
		reg.Agents = map[string]Agent{}
	}
	return reg, nil
}

// AddAgent registers (or replaces) a named agent command. The first agent
// registered becomes the default automatically.
func AddAgent(root, name, cmd string, makeDefault bool) error {
	name = NormalizeAgentName(name)
	if name == "" {
		return errors.New("agent name is required")
	}
	if strings.TrimSpace(cmd) == "" {
		return errors.New("agent command is required")
	}
	if _, err := ExpandAgentCommand(cmd, TemplateContext{}); err != nil {
		return err
	}
	reg, err := LoadAgents(root)
	if err != nil {
		return err
	}
	reg.Agents[name] = Agent{Cmd: cmd}
	if makeDefault || reg.Default == "" {
		reg.Default = name
	}
	return WriteJSON(agentsPath(root), reg)
}

// RemoveAgent drops a registered agent; removing the default clears it.
func RemoveAgent(root, name string) error {
	name = NormalizeAgentName(name)
	reg, err := LoadAgents(root)
	if err != nil {
		return err
	}
	if _, ok := reg.Agents[name]; !ok {
		return fmt.Errorf("agent %q is not registered", name)
	}
	delete(reg.Agents, name)
	if reg.Default == name {
		reg.Default = ""
	}
	return WriteJSON(agentsPath(root), reg)
}

// ResolveAgent returns the named agent, or the default when name is empty.
func ResolveAgent(root, name string) (string, Agent, error) {
	reg, err := LoadAgents(root)
	if err != nil {
		return "", Agent{}, err
	}
	if name == "" {
		name = reg.Default
		if name == "" {
			return "", Agent{}, errors.New("no agent registered: run `loopy agent add <name> --cmd <template>` first")
		}
	}
	name = NormalizeAgentName(name)
	agent, ok := reg.Agents[name]
	if !ok {
		return "", Agent{}, fmt.Errorf("agent %q is not registered (see `loopy agent list`)", name)
	}
	return name, agent, nil
}

// AgentNames lists registered agents sorted by name.
func AgentNames(reg AgentRegistry) []string {
	names := make([]string, 0, len(reg.Agents))
	for name := range reg.Agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// NormalizeAgentName lowercases and hyphenates an agent name.
func NormalizeAgentName(name string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(name)))
	return strings.Join(fields, "-")
}

// AgentSuggestion is a known agent CLI with its tested headless invocation
// (docs/agents.md is the evidence trail).
type AgentSuggestion struct {
	Binary string
	Name   string
	Cmd    string
}

// KnownAgentCLIs is the suggestion table behind `loopy init` and the
// monitor's onboarding: claude and codex are exercised through real loops;
// gemini is still a suggestion.
var KnownAgentCLIs = []AgentSuggestion{
	{Binary: "claude", Name: "claude", Cmd: "claude -p {prompt} --permission-mode acceptEdits"},
	// Plain `codex exec` runs read-only and can't edit the worktree;
	// --full-auto is the workspace-write, no-prompts mode.
	{Binary: "codex", Name: "codex", Cmd: "codex exec --full-auto {prompt}"},
	{Binary: "gemini", Name: "gemini", Cmd: "gemini -p {prompt} --yolo"},
}

// DetectAgentCLIs returns the known agent CLIs present on PATH that are not
// already registered.
func DetectAgentCLIs(root string) []AgentSuggestion {
	reg, err := LoadAgents(root)
	registered := map[string]bool{}
	if err == nil {
		for name := range reg.Agents {
			registered[name] = true
		}
	}
	var found []AgentSuggestion
	for _, s := range KnownAgentCLIs {
		if registered[s.Name] {
			continue
		}
		if _, err := exec.LookPath(s.Binary); err == nil {
			found = append(found, s)
		}
	}
	return found
}
