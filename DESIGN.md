---
title: loopy — Design
status: draft
date: 2026-06-10
authors: Jacob Barefoot (with Claude)
---

# loopy — Design

**Engineer loops, not prompts.** loopy is a local tool for designing, running, and
watching coding-agent loops: you define a goal, a verifier, and a budget; an agent
iterates inside an isolated worktree until the verifier goes green or the budget runs
out; you review the result with the full iteration history in front of you.

## Thesis

The unit of engineering is shifting from the prompt to the loop. Boris Cherny (creator
of Claude Code) describes his own workflow as no longer prompting agents directly:
*"I don't prompt Claude anymore. I have loops running. They're the ones prompting
Claude and figuring out what to do."* The practice — loop engineering — replaces the
human as turn-by-turn prompt operator with a designed feedback system: feed the agent
the current state, run it, verify the output mechanically, feed failures back, repeat.

Today that practice is held together with shell scripts, cron jobs, and markdown files.
loopy makes the loop a first-class, inspectable, resumable object with a real UI.

What stays human, permanently:

- **Designing the loop** — choosing the goal, the verifier, and the budget *is* the
  engineering. A loop with a weak verifier converges on garbage quickly.
- **Reviewing the result** — loopy never merges. A green verifier earns a parked diff
  and a review, not a commit to your branch.
- **Judgment between competing results** — when loops race, ranking is deterministic
  and evidence-based, but acceptance is yours.

## What loopy is not

- **Not a terminal multiplexer.** It orchestrates headless agent runs; it does not host
  interactive agent sessions.
- **Not an agent.** It has no model calls of its own. Agents are registered external
  commands (Claude Code, Codex, Gemini CLI, a shell script). loopy needs no API key.
- **Not a merge tool.** The output of a loop is a reviewed diff plus its evidence
  trail. Humans ship.
- **Not crux.** crux (its predecessor) is a human-gated judgment pipeline: every step
  is a manual command and the safety model is "a person approves each joint." loopy
  inverts the default — autonomous between two human moments (loop design, final
  review) — and borrows crux's proven machinery underneath.

## Lineage: what we port from crux

| From crux | In loopy |
| --- | --- |
| Worktree engine (isolated worktree + branch per run, dirty-repo refusal) | Ported nearly as-is; one worktree per loop, branch `loopy/<loop-id>` |
| Evidence collection (diff, logs, trial output, changed files) | Becomes the per-**iteration** record — richer, automatic, never a manual step |
| Trial command as seal gate | Becomes the **verifier** — the loop's fitness function, run every iteration |
| Council deterministic ranking | Becomes the **judge** in race mode (compare green loops, detect overlap, rank evidence) |
| Audited seal/banish with `--override --reason` | Becomes **accept/reject** at review, same audit discipline |
| `QuestView` view-model boundary; stdlib-only domain layer | Same pattern: `LoopView` shared by plain and TUI renderers |
| Exit-code contract (0 success / 1 runtime / 2 usage), `--json`, `NO_COLOR` | Identical |
| CI, RC-first release pipeline, six-target CGO-free archives, homebrew formula, PTY smoke tests | Ported with names changed |

Left behind: the six-command manual choreography, the medieval vocabulary, and the
read-only-dashboard constraint.

## Core concepts

### Loop

The central object. A loop is defined once and then runs itself.

```jsonc
// .loopy/loops/fix-csv-quoting/loop.json (illustrative)
{
  "id": "fix-csv-quoting",
  "goal": "Make the CSV importer handle quoted fields containing newlines",
  "constraints": ["do not change the public Importer API"],
  "forbidden_paths": ["vendor/", "go.sum"],
  "agent": "claude",
  "verifier": [
    { "name": "fmt",  "cmd": "test -z \"$(gofmt -l .)\"" },
    { "name": "vet",  "cmd": "go vet ./..." },
    { "name": "test", "cmd": "go test ./importer/..." }
  ],
  "budget": { "max_iterations": 8, "max_wall_clock": "30m" },
  "status": "running",          // draft | running | paused | green | parked | accepted | rejected
  "iterations_used": 3
}
```

- **Goal** — natural-language objective, plus optional constraints. Written into every
  iteration's prompt.
- **Verifier** — an ordered list of shell commands; the loop is **green** when all exit
  0. Ordered fast-to-slow so cheap failures (fmt, vet) short-circuit before expensive
  ones (tests, e2e). The verifier is the loop's definition of done *and* its feedback
  signal: stage name + output tail of the first failing stage is what the agent sees
  next iteration. **A loop cannot be created without a verifier.** This is the
  load-bearing design constraint — no verifier, no loop.
- **Budget** — hard caps: max iterations and max wall-clock (later: max cost). Budgets
  are not advisory; exhaustion parks the loop.
- **Escalation** — what happens when the loop can't get to green (see Stuck detection).
  v0 policy is always "park for human review with full history." Later: notify
  hooks, widen-budget-with-approval.

### Iteration

One turn of the crank, fully recorded:

1. **Compose** the prompt: goal + constraints + first failing verifier stage's output
   tail + summary of the diff so far + iteration number. Bounded sizes everywhere.
2. **Run** the agent headlessly in the loop's worktree; capture `agent.log`.
3. **Verify**: run stages in order; capture each stage's output and exit status.
4. **Record**: `diff.patch` (cumulative, against the loop's base commit), `prompt.md`,
   `agent.log`, `verifier.log`, `iteration.json`. State is flushed to disk *before*
   the next iteration starts — the model forgets everything between runs, so memory
   lives on disk, never only in context.
5. **Decide**: green → park for review. Budget left and not stuck → go to 1.
   Otherwise → park as `parked` (not green) with the reason.

### Stuck detection

Burning budget on a divergent loop is the failure mode of naive looping. The engine
escalates early when:

- the first failing verifier stage and a hash of its output tail are identical for
  **3 consecutive iterations** (the agent is going in circles), or
- an iteration produces **no change to the diff** (the agent gave up or did nothing).

Both thresholds are configurable per loop; the parked record says exactly which rule
fired.

### Race

`--race claude,codex` runs N loops on the same goal in parallel worktrees. First
green doesn't auto-win: when racing, green loops park and the **judge** (ported crux
council ranking — deterministic, no API key) compares diffs, flags overlapping files
and dependency-manifest changes, and ranks evidence. You review the ranked result.
"No safe winner" is a legitimate verdict.

### Review

The terminal human moment. `loopy review <loop-id>` shows the final diff, the verifier
transcript, and the iteration history; `loopy accept` records an audited decision in
`review.json` and preserves `final-diff.patch` durably; `loopy reject` preserves all
evidence and frees the worktree. Accepting a non-green loop requires
`--override --reason <text>`, recorded verbatim — same discipline as crux's seal.

### Reviewer agent (post-v0)

The creator shouldn't grade its own work. An optional reviewer stage runs a *different*
registered agent with review-only instructions against the green diff before parking;
its critique is attached as evidence, not a gate. This is Boris's sub-agent
verification pattern and slots in cleanly after the verifier.

## Default workflow

The whole point is that the happy path is one command:

```bash
loopy "make the CSV importer handle quoted newlines"
```

which:

1. Infers the verifier on first run (`make check`, `go test ./...`, `npm test`,
   `pytest`, `cargo test` — detected from the repo, confirmed interactively once,
   stored in `.loopy/config.json` as the default verifier).
2. Uses the default registered agent.
3. Creates the loop, the worktree, and starts iterating.
4. If stdout is a terminal, attaches the **monitor** so you watch it converge; if not,
   streams plain progress lines (CI-friendly).

Explicit form when you're engineering the loop deliberately:

```bash
loopy run "fix flaky importer test" \
  --verify "go vet ./..." --verify "go test -run TestImporter -count=20 ./importer" \
  --agent codex --max-iters 6 --max-time 20m \
  --forbidden-path vendor/
```

## CLI surface

```bash
loopy init
loopy agent add <name> --cmd "claude -p {prompt} --permission-mode acceptEdits" [--default]
loopy agent list | remove <name>
loopy "<goal>"                          # sugar for: loopy run "<goal>" with defaults
loopy run "<goal>" [--verify cmd]... [--agent name] [--race a,b] \
         [--max-iters N] [--max-time dur] [--forbidden-path p]... [--constraint text]...
loopy list [--json]
loopy status [loop-id] [--json] [--no-color]
loopy watch [loop-id] [--once]          # monitor TUI; --once = one plain ANSI-free frame
loopy pause | resume | abort <loop-id>
loopy log <loop-id> [--iter N]
loopy review <loop-id>
loopy accept <loop-id> [--override --reason text]
loopy reject <loop-id> [--reason text]
loopy logbook [--json]                  # durable project memory of accepted/rejected loops
loopy doctor [--json]
loopy version
```

Agent command templates substitute `{prompt}`, `{worktree}`, `{loop_id}`, `{goal}`,
`{iteration}`. Exit codes: 0 success, 1 runtime failure, 2 usage error.

## Execution model: daemonless, resumable

No daemon. `loopy run` owns the loop in the foreground (or under `nohup`/tmux/CI if
you want it backgrounded — loopy doesn't care). Crash-safety comes from disk, not from
a supervisor:

- State is flushed after every phase of every iteration; `loopy resume <loop-id>`
  continues exactly where a crashed or aborted loop stopped.
- The monitor and the engine are separate processes that meet at the filesystem. The
  monitor renders from state files and tails logs; control actions (pause/abort) are
  written to `control.json`, which the engine polls between agent/verifier phases.
  Pause is honored at phase boundaries; abort additionally sends the agent process
  group a signal.
- Stale-lock detection and repair guidance live in `loopy doctor`, ported from crux.

Considered and rejected for v0: a background daemon with an RPC socket. It buys
instant control latency at the cost of lifecycle management, a second failure domain,
and platform divergence. Phase-boundary polling is enough; revisit only if real usage
demands mid-phase control.

## The monitor

`loopy watch` is the product's face: a loop monitor, not a status board. Unlike crux's
strictly read-only dashboard, the monitor takes actions — but only the safe,
reversible ones (pause / resume / abort / open review). Accept and reject stay in the
CLI where they are validated and audited; the monitor deep-links to them by always
showing the exact next command in the footer.

```
┌ loops ──────────────┬ fix-csv-quoting · running · iter 3/8 · 12m left ─────────┐
│ ▶ fix-csv-quoting   │ [live] [iterations] [diff] [verifier] [review]           │
│   green-deploy-docs │                                                          │
│   flaky-importer ✗  │  iter  agent      verifier        wall                   │
│                     │  1     claude     ✗ test (3 fail) 2m41s                  │
│                     │  2     claude     ✗ test (1 fail) 3m02s                  │
│                     │  3     claude     ● running…                             │
│                     │                                                          │
│                     │  ~ tail: importer_test.go:88 TestQuotedNewlines …        │
├─────────────────────┴──────────────────────────────────────────────────────────┤
│ p pause · a abort · enter drill in · ? help        next: loopy review …        │
└────────────────────────────────────────────────────────────────────────────────┘
```

- **Live** — agent/verifier output tailing for the current iteration.
- **Iterations** — the timeline above: per-iteration verdicts and the convergence
  trend (failing-count per iteration). Divergence is visible at a glance.
- **Diff / Verifier / Review** — drill-down viewers; tail-first loading with a 256 KiB
  cap and truncation banners (crux's viewer rules).
- Carried wholesale from crux: semantic color that is never the only signal, `NO_COLOR`
  and `--no-color`, narrow-terminal collapse below 80 columns, `--once` as the
  deterministic ANSI-free frame for scripts, PTY smoke tests driving the real binary.

## State layout

Stable, human-readable, inspectable without loopy:

```text
.loopy/
  config.json               # default verifier, default agent, thresholds
  agents.json
  logbook.md                # durable memory: accepted/rejected loops, why
  loops/
    <loop-id>/
      loop.json             # definition + status + budget accounting
      control.json          # pause/abort requests (monitor → engine)
      iterations/
        0001/
          prompt.md         # exactly what the agent was told
          agent.log
          verifier.log      # per-stage output + exit statuses
          diff.patch        # cumulative diff vs. loop base commit
          iteration.json
      final-diff.patch      # written at accept; durable after worktree removal
      review.json           # audited accept/reject, overrides verbatim
  worktrees/
    <loop-id>/              # git worktree on branch loopy/<loop-id>
```

## Architecture

Three layers with the same enforced boundaries as crux:

- `cmd/loopy` — CLI entry; `usageError`/`helpRequest` mapped to exit codes.
- `internal/loop` — engine, verifier runner, feedback composer, stuck detection, judge,
  state store, git/worktrees. **stdlib only**; no TUI imports.
- `internal/tui` — the monitor; the only package that may import Bubble Tea.

`LoopView` in `internal/loop` is the shared view-model rendered by both the plain
renderer and the monitor — rendering logic never lives in the domain.

**Go**, `CGO_ENABLED=0`, six release targets. **Bubble Tea v2** from the start — crux
pinned v1 for stability mid-project; a greenfield repo is the right moment to take v2
(alt-screen lifecycle, key handling, and cursor APIs changed; budget a spike in M2).

## Safety model

- A loop cannot exist without a verifier; high-stakes defaults fail closed.
- Worktree isolation for every loop; refuse to start from a dirty repository.
- Budgets are hard caps; every override (`--override --reason`) is recorded verbatim.
- Forbidden-path and dependency-manifest changes are checked **every iteration**, not
  just at the end — a violation fails the iteration and is fed back to the agent.
- loopy never merges, commits to your branches, or pushes. Humans ship
  `final-diff.patch`.
- Registered agent commands run with your shell and your permissions. Register agents
  in their non-interactive, permission-scoped modes (e.g. Claude Code's
  `--permission-mode`), and review `.loopy/agents.json` in shared repos.

## Branding and language

Plain verbs, fun face. The vocabulary is loop / iteration / verifier / green / parked /
review / logbook — no ceremony, nothing you have to learn. The mascot is the pixelated
infinity guy (the ∞-as-8-bit pun), who belongs in the README, the help screen, and
maybe an idle monitor easter egg — and nowhere in the command surface.

## Milestones

- **M0 — skeleton**: repo scaffold, three-layer architecture, CI (fmt/vet/test/build +
  cross-compile), `loopy init`, `agent add`, and a *single-iteration* loop: worktree,
  one agent run, verifier, recorded iteration. (A loop that runs once is just crux's
  run+collect — this milestone is mostly porting.)
- **M1 — the loop**: feedback composition, multi-iteration engine, budgets, stuck
  detection, pause/resume/abort, crash resumability, `status`/`log`/`list` plain
  output. **This is the product**; everything after is leverage.
- **M2 — the monitor**: Bubble Tea v2 spike, then the monitor with live tailing,
  iteration timeline, drill-downs, control actions, `--once`, PTY smoke tests.
- **M3 — judgment**: race mode, ported judge, `review`/`accept`/`reject`, logbook.
- **M4 — ship**: release pipeline (RC-first), six-target archives, homebrew formula,
  demo script (shell-agent loop in a throwaway repo, no API keys), sample `.loopy/`
  snapshot in `examples/`.
- **Post-v0**: reviewer agent, scheduled loops (Boris's "automations" — recurring
  discovery/triage loops), cost budgets, notification hooks.

## Open questions

- **Prompt composition limits** — how much iteration history to carry forward beyond
  the last failure tail; whether a rolling digest beats a fixed window.
- **Verifier inference UX** — confirm-once-and-store is the plan; how loud to be when
  inference guesses wrong.
- **Headless agent matrix** — exact non-interactive invocations and permission flags
  per tool (Claude Code, Codex, Gemini CLI) belong in a tested, documented table.
- **Cost tracking** — agents don't report spend uniformly; may start as wall-clock
  proxy only.
- **Windows** — crux's known gaps (interactive resize polling) carry over; keep parity.

## References

- [Stop Prompting AI and Start Building Loops](https://www.productmarketfit.tech/p/stop-prompting-ai-and-start-building) — interview with Boris Cherny
- [Loop Engineering — Addy Osmani](https://addyosmani.com/blog/loop-engineering/) — the five components (automations, worktrees, skills, connectors, sub-agents) plus state-on-disk
- [Loop Engineering — The New Stack](https://thenewstack.io/loop-engineering/)
- crux — predecessor and parts donor: https://github.com/mjbarefo/crux
