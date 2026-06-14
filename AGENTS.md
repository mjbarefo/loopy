# AGENTS.md

Repository-wide instructions for coding agents working on loopy.

## Project overview

loopy is a local Go CLI for running coding agents in isolated git worktrees
until an ordered verifier goes green or a hard budget is exhausted. It does
not call models itself; registered agent commands are external processes.

Read these before making substantial changes:

- `README.md` and `QUICKSTART.md` for the user-facing workflow.
- `DESIGN.md` for product intent and architecture.
- `DECISIONS.md` for accepted deviations and engineering decisions.
- `SECURITY.md` before changing command execution, prompts, worktrees, or
  persisted state.
- `docs/agents.md` for the tested external-agent command matrix. That file is
  runtime documentation, not repository contribution guidance.

## Repository map

- `cmd/loopy`: CLI parsing, commands, output, and exit-code behavior.
- `internal/loop`: domain logic, verifier execution, worktrees, persistence,
  review, and rendering view-models. This package must remain standard-library
  only.
- `internal/tui`: Bubble Tea monitor. This is the only package that may import
  the TUI framework.
- `scripts`: end-to-end, PTY, packaging, and release automation.
- `docs`: operational and design-adjacent documentation.
- `.github/workflows`: CI and release gates.

## Non-negotiable invariants

1. A loop cannot be created without at least one verifier stage.
2. loopy never merges, commits to a user's branch, or pushes. Its output is a
   reviewed diff plus inspectable evidence.
3. Budgets are hard caps, and every gate override is recorded verbatim.
4. The core product makes no model calls. The zero-API-key demo must keep
   working.
5. `internal/loop` remains standard-library only; rendering logic belongs in
   the shared `LoopView` boundary or `internal/tui`, not in domain behavior.
6. Persisted state remains plain JSON, Markdown, and patches readable without
   loopy.
7. Agent template values stay shell-quoted. Verifier output is untrusted input.

## Working practices

- Inspect the current implementation and tests before editing. Keep changes
  narrowly scoped and preserve unrelated worktree changes.
- Prefer existing patterns and standard-library APIs. Do not add a production
  dependency without a clear need and an explicit design decision.
- Use `gofmt` for Go changes. Keep errors contextual and preserve documented
  exit codes: 0 for success, 1 for runtime failure, and 2 for usage errors.
- Keep state writes atomic and respect the engine's single-writer ownership.
- Treat process cleanup, shell quoting, path handling, and worktree isolation
  as security-sensitive.
- Keep color optional and never the only signal. Honor `NO_COLOR`, and preserve
  deterministic ANSI-free output where supported.
- Maintain `--json` behavior when changing command output.
- Add or update focused tests with behavior changes. Engine tests should use
  scripted agents and real temporary git repositories rather than mocks.
- Do not edit generated release artifacts or live `.loopy/` runtime state
  unless the task explicitly targets them.

## Build and test

Use the Go version declared in `go.mod`. During development, run the narrowest
relevant test first, for example:

```sh
go test ./internal/loop -run TestName
go test ./internal/tui -run TestName
go test ./cmd/loopy -run TestName
```

Before handing off any change, run:

```sh
make check
```

`make check` runs formatting, vetting, tests, and a `CGO_ENABLED=0` build. Also
run the checks appropriate to the affected surface:

```sh
go test -race ./...  # required for shared state, concurrency, or final merge
make tui-smoke       # TUI rendering, input, or monitor behavior
scripts/demo.sh      # engine, verifier, prompt, review, or CLI workflow
```

The PTY smoke test requires `expect`. Do not weaken or delete a failing test to
make a change pass; fix the behavior or update expectations when behavior was
intentionally changed.

## Documentation and decisions

- Update user-facing docs and examples when commands or behavior change.
- Add a dated `DECISIONS.md` entry when deviating from `DESIGN.md` or resolving
  a meaningful question the design leaves open.
- Keep terminology consistent: loop, iteration, verifier, green, parked, and
  review.
- Keep claims aligned with what CI and release automation actually prove.

## Completion checklist

- The requested behavior is implemented with focused tests.
- Package and security boundaries still hold.
- `make check` passes.
- Relevant race, TUI smoke, and end-to-end demo checks pass.
- Documentation and `DECISIONS.md` are updated when required.
