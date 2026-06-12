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
| Gemini CLI | `loopy agent add gemini --cmd "gemini -p {prompt} --yolo --skip-trust"` | **Tested 2026-06-12** — green loop in one iteration; see note below |
| Any script | `loopy agent add fixer --cmd "sh ./my-agent.sh {prompt_file}"` | Tested continuously — the demo and the engine test suite run scripted shell agents |

Notes:

- **Codex**: plain `codex exec {prompt}` runs in a read-only sandbox by
  default and cannot edit the worktree; use `--full-auto` (workspace-write
  sandbox, no approval prompts) for loop work.
- **Gemini**: without `--skip-trust`, gemini refuses headless work in any
  directory it hasn't interactively trusted — and every loop worktree is a
  fresh untrusted directory, so `--yolo` silently downgrades and the agent
  exits without doing anything (the loop parks "agent blocked"). The
  equivalent env var is `GEMINI_CLI_TRUST_WORKSPACE=true`.
- **Permission flags are the contract.** Register agents in their
  non-interactive, permission-scoped modes. An agent that stops to ask a
  question burns wall-clock budget until the loop parks.
- Smoke-test a registration before trusting it with a budget:
  `loopy agent check <name>` runs the agent once against a trivial prompt in
  a throwaway directory (one tiny model call) and prints the CLI's own words
  on failure. `loopy doctor` also warns when a registered agent's binary is
  missing from PATH.
- For a fuller rehearsal, run a tiny goal with a small budget:

  ```bash
  loopy run "create hello.txt containing exactly: hello" \
    --verify "grep -qx hello hello.txt" --agent <name> --max-iters 2
  ```
