# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

loopy is a local tool for engineering coding-agent loops: goal + verifier +
budget → an agent iterates in an isolated worktree until green or the budget
runs out → a human reviews the diff with the full iteration history. Read
`DESIGN.md` for the design, `DECISIONS.md` for every deviation from it.
**When you deviate from the design or make a call it left open, add a
DECISIONS.md entry** — what you decided and one sentence on why.

## Invariants (not up for debate)

1. No verifier, no loop — creation refuses empty verifiers.
2. loopy never merges, commits to user branches, or pushes. Output is a
   reviewed diff plus evidence.
3. Budgets are hard caps; every gate override is recorded verbatim.
4. No model calls of its own; agents are registered external commands. The
   demo must keep working with zero API keys.
5. Layer boundaries: `internal/loop` is **stdlib only**; only `internal/tui`
   (M2+) may import the TUI framework. Rendering logic never lives in the
   domain — `LoopView` in `internal/loop/view.go` is the shared view-model.
6. Everything on disk is plain JSON / markdown / patches, inspectable without
   loopy.

## Build & test

- `make check` — canonical gate: fmt + vet + test + build. Run before every PR.
- `go test -race ./...` — required before merging.
- `scripts/demo.sh` — end-to-end loop in a temp repo, no API keys; run it when
  touching the engine, prompt composition, or CLI output.
- All builds are `CGO_ENABLED=0`; `go.mod` has zero dependencies until M2.

## Architecture

- `cmd/loopy` — CLI dispatch; `usageError`/`helpRequest` map to the exit-code
  contract: 0 success, 1 runtime failure, 2 usage. `loopy run` exits 0 only
  when the loop parks green.
- `internal/loop` — engine (`engine.go`), verifier runner, prompt composer,
  git/worktrees (temp-index snapshots), JSON store (atomic temp+rename
  writes), stuck detection, locks, doctor.
- State lives under `.loopy/` in the target repo: `loops/<id>/loop.json`,
  `iterations/NNNN/{prompt.md,agent.log,verifier.log,diff.patch,iteration.json}`,
  `control.json` (monitor→engine), `engine.lock` (pid liveness).
- Engine is the single writer of loop state; other processes write only
  `control.json`. Pause is honored at iteration boundaries; abort kills the
  running process group within ~2s.
- Agent command templates expand `{prompt}` `{prompt_file}` `{worktree}`
  `{loop_id}` `{goal}` `{iteration}` — every value shell-quoted (verifier
  output reaches the prompt; unquoted expansion is an injection vector).

## Conventions

- Branch names: `mjbarefo/<context>/<description>`; commit messages terse,
  lowercase, imperative. Small PRs; the human merges on GitHub.
- Plain vocabulary everywhere: loop, iteration, verifier, green, parked,
  review. The mascot stays in README/help text, never in the command surface.
- Engine tests script their agents as inline shell commands against real temp
  git repos — keep new engine behavior covered that way (fast, no mocks).
- Color is never the only signal; honor `NO_COLOR`; `--json` everywhere it
  makes sense.
