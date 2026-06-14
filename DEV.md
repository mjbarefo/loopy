# DEV.md — loopy contributor guide

Practical reference for building, testing, and extending loopy. For invariants
and design rationale, see `CLAUDE.md` (AI/conventions) and `DESIGN.md`
(architecture and thesis). For every deviation from the design, see
`DECISIONS.md`.

## Prerequisites

- **Go 1.26+** (`go.mod` declares `go 1.26`)
- **git** (the engine shells out to git; tests spin real temp repos)
- **expect** (for the PTY smoke test — `make tui-smoke` needs it)
- All builds are `CGO_ENABLED=0`; the binary is a single static executable

The module has one direct dependency: `charm.land/bubbletea/v2 v2.0.7`, used
exclusively by `internal/tui`. The domain layer (`internal/loop`) is stdlib
only.

## Build & test gates

```sh
make check        # canonical gate: gofmt -w + go vet + go test + CGO_ENABLED=0 build
                  # run before every PR

go test -race ./... # data-race check; required before merging (= make race)

scripts/demo.sh   # end-to-end loop in a throwaway repo, zero API keys
                  # run when touching engine, prompt composition, or CLI output

make tui-smoke    # PTY smoke test: builds the real binary, drives it with expect
                  # covers audited CLI actions the monitor delegates to the binary
```

### All Makefile targets

| Target | What it does |
| --- | --- |
| `make check` | fmt + vet + test + build (the PR gate) |
| `make test` | `go test ./...` |
| `make race` | `go test -race ./...` |
| `make vet` | `go vet ./...` |
| `make fmt` | `gofmt -w cmd internal` |
| `make build` | `CGO_ENABLED=0 go build ./cmd/loopy` |
| `make tui-smoke` | builds binary to `/tmp/loopy-tui-smoke-bin`, runs `scripts/tui-smoke.exp` |
| `make dist` | six-target cross-compile (`scripts/dist.sh`) — release use only |

## Repository layout

```
cmd/loopy/          CLI dispatch
  main.go           entry point; usageError/helpRequest → exit codes
  init.go           loopy init (creates .loopy/, git-ignores it)
  run.go            loopy run / resume
  watch.go          loopy watch → TUI monitor
  review.go         loopy review / accept / reject
  agent.go          loopy agent add / list / remove / check
  loops.go          loopy list / status / log / delete / judge
  doctor.go         loopy doctor
  events_json.go    NDJSON event stream for --json

internal/loop/      domain layer — stdlib only; no TUI imports
  engine.go         the loop engine: CreateLoop, RunEngine
  verifier.go       verifier runner (command + ask stages)
  prompt.go         feedback composition for the agent's next prompt
  git.go            worktree creation, diff, restore
  store.go          atomic JSON reads/writes (temp+rename)
  models.go         Loop, Stage, Budget, StuckPolicy, LoopView types
  view.go           LoopView — the shared view-model (plain + TUI renderers)
  render.go         plain-text renderer
  judge.go          deterministic race ranking (no API keys)
  gate.go           background gate synthesis (AutoGate / SynthesizeVerifier)
  synth.go          --verify auto / wizard tab synthesis
  infer.go          project-gate inference (make check, go test, npm test …)
  repos.go          ScanRepos / FindRepos for the front-door picker
  diffstat.go       pure diff stat parser
  width.go          DisplayWidth / TruncateDisplay / PadDisplay / HardWrapDisplay
  logbook.go        logbook read/write
  review.go         accept / reject / override logic
  preflight.go      agent preflight (loopy agent check)
  slug.go           loop ID generation from the goal
  tail.go           bounded log tailing
  doctor.go         lock liveness, stale-state diagnosis

internal/tui/       monitor — only package that may import Bubble Tea
  model.go          Bubble Tea model + update + key handlers
  frame.go          deterministic frame renderer
  hittest.go        pure hit-test over frameState (unit-testable)
  tui.go            Elm-arch entry point, runGit helper
  welcome.go        welcome splash + logo art (single source of truth)
  picker.go         front-door repo picker
  form.go           new-loop wizard
  snapshot.go       --once frame capture

scripts/
  demo.sh           end-to-end demo (no API keys)
  tui-smoke.exp     expect script for the PTY smoke
  dist.sh           cross-compile release archives
  tap-bootstrap.sh  one-time homebrew tap setup

docs/
  releasing.md      RC-first release runbook
  agents.md         tested headless agent invocations
  verifier-spectrum.md  command · ask · hybrid verifier design
  orchestration.md  --json event stream contract for outer loops
```

## The invariants (summary)

Canonical source: `CLAUDE.md`.

1. No verifier, no loop — `CreateLoop` refuses an empty verifier.
2. loopy never merges, commits to user branches, or pushes.
3. Budgets are hard caps; every gate override is recorded verbatim in
   `review.json`.
4. No model calls of its own — agents are registered external commands; the
   demo works with zero API keys.
5. Layer boundary: `internal/loop` is stdlib only; only `internal/tui` may
   import Bubble Tea. Rendering logic never lives in the domain — `LoopView`
   in `internal/loop/view.go` is the shared view-model.
6. Everything on disk is plain JSON / markdown / patches, inspectable without
   loopy.

## How tests are written

### Engine tests — real repos, inline shell agents, no mocks

Engine tests create throwaway git repos, register a scripted shell command as
the "agent", and run the real engine against it. There are no mocks of the
filesystem, git, or the verifier runner. The "agent" sees only what loopy
wrote to the prompt file (template variables like `{prompt_file}` expand on
expansion), so it stays fully offline. The shape, drawn from
`internal/loop/ask_test.go`:

```go
agent := `test -f done.txt || echo done > done.txt`
root := newLoopProject(t, agent)        // real temp git repo + .loopy/
l := mustCreate(t, root, CreateOptions{
    Goal:     "create done.txt",
    Verifier: []Stage{{Name: "file", Cmd: `test -f done.txt`}},
})
final, err := RunEngine(root, l.ID, Events{})  // real engine, real worktree
// assert on final.Status and the recorded iterations
```

Add new engine behavior with this pattern: keep agents as inline shell
one-liners, run against a real temp repo, assert on the final loop and its
recorded iterations.

### TUI frame tests

`internal/tui/frame_test.go` renders frames at fixed dimensions and asserts
exact byte output. `internal/tui/hittest_test.go` asserts click → action
mapping as a pure function without a running model. Color discipline is
enforced by `TestFrameColorDiet`.

### PTY smoke

`scripts/tui-smoke.exp` drives the real binary through `expect`, injecting
keystrokes including SGR mouse sequences, and asserts on terminal output. It
covers interactive paths (monitor, wizard, control keys) that unit tests can't
reach.

## Extending loopy

### Add a CLI subcommand

1. Add a handler function in `cmd/loopy/` (follow the existing file split:
   one file per command group).
2. Wire it into the dispatch table in `cmd/loopy/main.go`.
3. Return `usageError` for bad flags/args (exits 2) and `helpRequest` for
   `--help` (exits 0 with help text); all other errors return a plain `error`
   (exits 1).
4. `loopy run` exits 0 only when the loop parks green; everything else exits 1.

### Agent command templates

Templates registered with `loopy agent add --cmd "..."` expand these variables:

| Variable | Value |
| --- | --- |
| `{prompt}` | full prompt text (inline) |
| `{prompt_file}` | path to `prompt.md` — prefer this for agents with an arg length limit |
| `{worktree}` | absolute path to the loop's isolated worktree |
| `{loop_id}` | the loop's ID string |
| `{goal}` | the loop's goal text |
| `{iteration}` | current iteration number |

**Every value is shell-quoted on expansion.** Unquoted expansion is a shell
injection vector: verifier output reaches the next iteration's prompt, which
feeds `{prompt}` — a single unquoted variable would let a verifier stage
inject arbitrary shell commands. Do not change the quoting behavior without
auditing every template variable path.

The agent runs with its working directory set to the loop's worktree.

### Verifier stages

A verifier is an ordered `[]Stage`. Stages run fast-to-slow, short-circuit on
the first failure. Two kinds (`StageKind` in `internal/loop/models.go`):

- **command** (zero value, `Kind: ""`): `Cmd` is a shell command; exit 0 = pass.
  Free, deterministic, no API key.
- **ask** (`Kind: "ask"`): `Ask` is a yes/no question posed to the loop's agent
  (or an override agent via `Agent`). The agent's final output line must be
  `PASS` or `FAIL: <reason>`. Fail-closed: timeout, unrunnable agent, or no
  verdict = fail. Costs an agent call + keys; use only where a shell command
  can't express "done".

A **hybrid** is the default wizard output: command gates first, an ask stage
last. The ask stage only runs once the cheap gates are green.

Stuck detection (`SameFailureRepeats`, `NoChangeRepeats`) works against
command stages; ask loops rely on `NoChangeRepeats` and the hard budget, since
ask output varies.

### Add a domain primitive

Work only in `internal/loop`. Import nothing from `internal/tui` or any
third-party library. All JSON writes must go through the atomic temp+rename
pattern in `store.go`. If the change touches the view-model shape, update
`LoopView` in `view.go` and its plain renderer in `render.go` before touching
the TUI.

### Decisions log

When you deviate from `DESIGN.md` or make a call the design left open, add a
`DECISIONS.md` entry at the bottom: what you decided and one sentence on why.
The log is the project's institutional memory for non-obvious choices.

## Conventions

- Branch names: `mjbarefo/<context>/<description>`
- Commit messages: terse, lowercase, imperative (`tui: fix rail group gap`)
- Small PRs; the human merges on GitHub
- `--json` on every command where it makes sense
- Color is never the only signal; honor `NO_COLOR` and `--no-color`
- Plain vocabulary: loop, iteration, verifier, green, parked, review. The
  mascot belongs in README/help text, never in the command surface.
- Display-width–aware helpers only: `TruncateDisplay`, `PadDisplay`,
  `WrapDisplay`, `HardWrapDisplay` in `internal/loop/width.go`. Never
  byte-slice or rune-count for layout — CJK and emoji break both.

## State layout (quick reference)

```
.loopy/
  config.json                      default verifier, default agent, default budget
  agents.json
  logbook.md
  loops/<id>/
    loop.json                      definition + live status + budget accounting
    control.json                   pause/abort requests (monitor → engine only)
    engine.lock                    pid liveness; stale lock = resumable
    phase.json                     ephemeral: current phase while engine runs
    iterations/
      0000/                        baseline (agent-free verifier run)
        prompt.md  agent.log  verifier.log  diff.patch  iteration.json
      0001/ …
    final-diff.patch               written at accept; durable after worktree gone
    review.json                    audited accept/reject, overrides verbatim
    critique.md                    reviewer agent output (if --reviewer was set)
  worktrees/<id>/                  git worktree on branch loopy/<id>
```

Engine is the single writer of `loop.json` and `iterations/`. Other processes
write only `control.json`. The monitor writes nothing to loop state; it shells
out to the audited CLI for any action that changes state.

## Release

See `docs/releasing.md` for the full runbook: RC-first tags, the full gate
re-runs on release, `make dist` produces six CGO-free archives, and the
Homebrew formula is a release asset pushed to the tap. RCs (`v0.x.y-rc.N`)
publish as prereleases and never reach the tap; only stable tags do.
