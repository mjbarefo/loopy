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

What stays human:

- **Designing the loop** — choosing the goal, the verifier, and the budget *is* the
  engineering. A loop with a weak verifier converges on garbage quickly; when the
  human steps out of the loop, the verifier is their judgment, mechanized.
- **Accountability for what ships** — loopy never merges, commits to your branches,
  or pushes. A green verifier earns a parked diff and its evidence, not a commit.
  That invariant belongs to the tool and is permanent. *Who reviews* each parked
  diff is a policy that belongs to whatever sits above loopy: a person today; a
  reviewer agent or an outer loop tomorrow. Moving review up the stack is
  composition, not a loopy setting — and the audited accept/reject record stays,
  whoever drives it.
- **Judgment between competing results** — when loops race, ranking is deterministic
  and evidence-based, but acceptance is yours.

### loopy as a rung

The unit above a loop is another loop. loopy is built to be stacked on, not just
watched: `loopy run` exits 0 only when the loop parks green, `--json` covers the
read surface, accept/reject are non-interactive, and everything on disk is plain
JSON / markdown / patches an outer orchestrator can read without loopy. Going *up*
a loop (a goal generator that spawns loops and escalates only failures to a person)
is the leverage story. Going *down* one is the reliability story: every iteration's
exact prompt, verifier transcript, diff, and stuck-reason is the flight recorder
for the rung below you — and recorders matter more, not less, as the distance
between the human and the failure grows. loopy's job is to be a load-rated rung in
both directions.

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
- **Verifier** — an ordered list of stages; the loop is **green** when all pass.
  Ordered fast-to-slow so cheap failures short-circuit before expensive ones. The
  verifier is the loop's definition of done *and* its feedback signal: stage name +
  output tail of the first failing stage is what the agent sees next iteration.
  **A loop cannot be created without a verifier.** This is the load-bearing design
  constraint — no verifier, no loop.

  Stages sit on a spectrum by what produces their verdict: a **command stage** runs a
  shell command (`sh -c`; exit 0 = pass) — deterministic, instant, no API key; an
  **ask stage** poses a yes/no question to a registered agent (`PASS` / `FAIL: <reason>`
  on the final output line) — for goals no shell command can express ("is the prose
  accurate?", "does this read cleanly?"). A **hybrid** is the ordered mix the wizard
  reaches for by default: cheap deterministic gates first, an ask stage last, so the
  agent call only runs once the mechanical gates are green. The wizard composes a hybrid
  instantly (inferred command gates + a goal-derived ask question); `tab` triggers the
  optional background synthesis of tighter command gates. For an ask-only loop, the
  engine also designs a deterministic gate in the background and folds it in additively
  ahead of the ask — making green stricter without re-sign-off — once the proposal
  lands. Ask stages fail closed (timeout, unrunnable agent, or missing verdict all read
  as `FAIL`). The no-API-key demo and verifier inference produce command stages only.
  See `docs/verifier-spectrum.md` for full design.
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

### Reviewer agent

The creator shouldn't grade its own work. `loopy run --reviewer <name>` runs a
*different* registered agent against the green diff before parking; its critique is
recorded as `critique.md` and shown by `loopy review` — evidence, never a gate. The
reviewer agent must differ from the loop's author agent (refused at creation). Any
worktree changes the reviewer makes are reverted; a reviewer failure, timeout, or
missing agent does not prevent the loop from parking green. This is Boris's sub-agent
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
loopy agent check [name]                # smoke-run agents; catches trust/auth failures
loopy agent list | remove <name>
loopy "<goal>"                          # sugar for: loopy run "<goal>" with defaults
loopy run "<goal>" [--verify cmd|auto]... [--agent name] [--race a,b] \
         [--reviewer name] \
         [--max-iters N] [--max-time dur] [--forbidden-path p]... [--constraint text]... \
         [--json]                       # NDJSON event stream; --race ends with verdict event
loopy resume <loop-id> [--json]
loopy list [--json]
loopy status [loop-id] [--json] [--no-color]
loopy watch [loop-id] [--once]          # monitor TUI; --once = one plain ANSI-free frame
loopy pause | resume | abort <loop-id>
loopy log <loop-id> [--iter N] [--json]
loopy review <loop-id>
loopy accept <loop-id> [--override --reason text]
loopy reject <loop-id> [--reason text]
loopy delete <loop-id>                  # removes loop + evidence; logbook keeps one line
loopy judge <id> <id> [...]             # rank finished loops by evidence (used by --race)
loopy logbook [--json]                  # durable project memory of accepted/rejected loops
loopy doctor [--json]
loopy version
```

Agent command templates substitute `{prompt}`, `{prompt_file}`, `{worktree}`,
`{loop_id}`, `{goal}`, `{iteration}` — all values shell-quoted. `--verify auto` asks
the registered agent to propose a goal-testing command in a throwaway worktree; the
proposal is trial-run and confirmed interactively before use, never stored as the
project default. Exit codes: 0 success, 1 runtime failure, 2 usage error; `loopy run`
exits 0 only when the loop parks green.

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
strictly read-only dashboard, the monitor takes actions. Control actions are contextual
on the loop's state: `p` pauses a running loop; `a` aborts a moving loop and accepts a
parked green one; `r` resumes a paused loop and rejects a parked one; `d` deletes a
loop after confirmation. Each decision (`a` accept, `r` reject) shells out to the
audited CLI behind a y/n confirmation, so the audit discipline is identical to using
the CLI directly. Accepting a non-green loop remains CLI-only (`loopy accept --override
--reason`). `A` applies an accepted loop's `final-diff.patch` to your working tree via
`git apply` (behind a y/n confirm) then removes the loop — this is the monitor's only
write to the user's checkout, and it is deliberately the weakest: `git apply` to the
working tree, never a commit, push, or merge. The monitor also supports mouse: the
scroll wheel scrolls the pane under the pointer, clicking a rail row selects it,
clicking a nav name switches views; `c` copies the next command to the system clipboard
via OSC 52. The footer always shows the exact next command, so the monitor is never a
dead end.

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
- Worktree isolation for every loop; refuse to start from a dirty repository
  by default (opt in with `--stash`, offered by the monitor, which sets the
  changes aside and never pops them back — the user restores with `git stash
  pop`).
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

All M0–M4 milestones shipped and merged to `main` as of 2026-06-11.

- **M0 — skeleton** ✓: repo scaffold, three-layer architecture, CI (fmt/vet/test/build
  + cross-compile), `loopy init`, `agent add`.
- **M1 — the loop** ✓: feedback composition, multi-iteration engine, budgets, stuck
  detection, pause/resume/abort, crash resumability, `status`/`log`/`list` plain
  output. **This is the product**; everything after is leverage.
- **M2 — the monitor** ✓: Bubble Tea v2, the monitor with live tailing, iteration
  timeline, drill-downs, control actions, `--once`, PTY smoke tests. The monitor
  evolved past M2 with: contextual accept/reject keys, the `A` apply-and-remove flow,
  mouse support, fleet view (iterated and resolved back to the dense rail), the wizard
  (five-step new-loop form), the splash/picker/front-door, and the verifier spectrum UI.
- **M3 — judgment** ✓: race mode, ported judge, `review`/`accept`/`reject`, logbook,
  `loopy judge` as a standalone command.
- **M4 — ship** ✓: RC-first release pipeline, six-target CGO-free archives, homebrew
  formula, demo script (shell-agent loop, no API keys), sample `.loopy/` in
  `examples/`.
- **Post-v0 shipped**: reviewer agent (`--reviewer`), the verifier spectrum
  (command · ask · hybrid), `--verify auto`, background gate synthesis, `loopy delete`,
  `loopy agent check`, `--json` on run/resume (NDJSON streams), agent-blocked park
  reasons with fix hints.
- **Post-v0 open**: scheduled/recurring loops (Boris's "automations"), cost budgets,
  notification hooks, shell completions.

## Open questions

- **Prompt composition limits** — how much iteration history to carry forward beyond
  the last failure tail; whether a rolling digest beats a fixed window.
- **Verifier inference UX** — confirm-once-and-store ships; the open question is how
  loud to be when inference guesses wrong (baseline-green is the symptom; `--verify
  auto` and the wizard's auto-synthesis are the current mitigations).
- **Headless agent matrix** *(partially resolved)* — claude and codex invocations are
  tested and documented (`docs/agents.md`); gemini's `--skip-trust` flag is confirmed
  required for loop worktrees. The table is live but will grow as more CLIs are tested.
- **Cost tracking** — agents don't report spend uniformly; currently wall-clock proxy
  only. Cost budgets remain post-v0.
- **Windows** — crux's known gaps (interactive resize polling) carry over; CI is
  build+vet only on Windows (`sh` dependency); macOS/Linux are the supported targets.
- **Stuck detection for ask stages** — ask stages produce varying natural-language
  failure reasons, so `SameFailureRepeats` rarely trips; ask loops lean on
  `NoChangeRepeats` (diff unchanged N times) and the hard budget. Whether a tighter
  heuristic is warranted is open.
- **Scheduled / recurring loops** — Boris's "automations" (recurring discovery/triage
  loops that run on a schedule) are not yet designed. The `loopy run` exit-code
  contract and `--json` stream make it composable from outside loopy for now.

## References

- [Stop Prompting AI and Start Building Loops](https://www.productmarketfit.tech/p/stop-prompting-ai-and-start-building) — interview with Boris Cherny
- [Loop Engineering — Addy Osmani](https://addyosmani.com/blog/loop-engineering/) — the five components (automations, worktrees, skills, connectors, sub-agents) plus state-on-disk
- [Loop Engineering — The New Stack](https://thenewstack.io/loop-engineering/)
- crux — predecessor and parts donor: https://github.com/mjbarefo/crux
