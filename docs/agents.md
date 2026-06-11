# Headless agent matrix

loopy runs whatever command you register; these are the invocations we have
actually exercised through real loops, and when. Re-test after a CLI you use
ships a major version — headless flags churn.

Template variables (`{prompt}`, `{prompt_file}`, `{worktree}`, `{loop_id}`,
`{goal}`, `{iteration}`) are always shell-quoted on expansion. The agent runs
with its working directory set to the loop's worktree.

| Agent CLI | Registration | Status |
| --- | --- | --- |
| Claude Code | `loopy agent add claude --cmd "claude -p {prompt} --permission-mode acceptEdits" --default` | **Tested 2026-06-11** — multi-iteration loop, feedback-driven convergence (see the `implement-displaywidth…` loop in this repo's logbook) |
| Codex CLI | `loopy agent add codex --cmd "codex exec --full-auto {prompt}"` | **Tested 2026-06-11** — see note below |
| Gemini CLI | `loopy agent add gemini --cmd "gemini -p {prompt} --yolo"` | Suggestion only — not yet exercised; verify the flags before trusting it with a budget |
| Any script | `loopy agent add fixer --cmd "sh ./my-agent.sh {prompt_file}"` | Tested continuously — the demo and the engine test suite run scripted shell agents |

Notes:

- **Codex**: plain `codex exec {prompt}` runs in a read-only sandbox by
  default and cannot edit the worktree; use `--full-auto` (workspace-write
  sandbox, no approval prompts) for loop work.
- **Permission flags are the contract.** Register agents in their
  non-interactive, permission-scoped modes. An agent that stops to ask a
  question burns wall-clock budget until the loop parks.
- Test a new registration once with a tiny goal and `--max-iters 2` before
  trusting it with a real budget:

  ```bash
  loopy run "create hello.txt containing exactly: hello" \
    --verify "grep -qx hello hello.txt" --agent <name> --max-iters 2
  ```
