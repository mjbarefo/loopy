# loopy

```
   ████      ████
 ██░░░░██  ██░░░░██        l o o p y
 ██░░░░░░██░░░░░░██
 ██░░░░██  ██░░░░██   engineer loops, not prompts
   ████      ████
```

**You define a goal, a verifier, and a budget. An agent iterates in an
isolated git worktree until the verifier goes green or the budget runs out.
You review the result with the full iteration history in front of you.**

```bash
brew install mjbarefo/tap/loopy

loopy        # the monitor: set up the repo, start loops, watch them converge
loopy "make the CSV importer handle quoted newlines"
```

New here? **[QUICKSTART.md](QUICKSTART.md)** walks from install to your
first reviewed diff.

## Why loops

The unit of engineering is shifting from the prompt to the loop. Boris Cherny
(creator of Claude Code) describes his own workflow: *"I don't prompt Claude
anymore. I have loops running. They're the ones prompting Claude and figuring
out what to do."* The practice — loop engineering — replaces the human as
turn-by-turn prompt operator with a designed feedback system: feed the agent
the current state, run it, verify the output mechanically, feed failures back,
repeat.

Today that practice is held together with shell scripts, cron jobs, and
markdown files. loopy makes the loop a first-class, inspectable, resumable
object.

What stays human:

- **Designing the loop.** Choosing the goal, the verifier, and the budget *is*
  the engineering. A loop with a weak verifier converges on garbage, quickly.
- **Accountability for what ships.** loopy never merges, never commits to your
  branches, never pushes — that is the tool's permanent invariant. A green
  verifier earns a parked diff and a review. Who reviews it is yours to place:
  you, a reviewer agent, or an outer loop gating on `loopy run`'s exit code —
  loopy is built to be a rung in a taller stack
  ([driving loopy from an outer loop](docs/orchestration.md)), and the
  evidence trail is the way back down when the stack misbehaves.

## See it converge (no API keys)

```bash
git clone https://github.com/mjbarefo/loopy && cd loopy
scripts/demo.sh
```

The demo creates a throwaway repo with a three-bug fizzbuzz, registers a
scripted shell agent that can only see what loopy tells it, and runs one loop:
baseline red → three feedback-driven fixes → green, with every prompt, log,
and diff recorded on disk. loopy has no model calls of its own — agents are
registered external commands, so it works end-to-end with a shell script and
zero API keys.

## How it works

```
        ┌────────────────────────────────────────────────┐
        │                  the loop                      │
        │   compose prompt → run agent → snapshot diff   │
        │        ↑                          ↓            │
        │   feedback tail  ←  verify (ordered stages)    │
        └────────────────────────────────────────────────┘
   green → parked for review        red → next iteration
   budget gone / stuck → parked with the reason, full history
```

- **No verifier, no loop.** A loop cannot be created without at least one
  verifier stage. A stage can be a shell command (exit 0 = pass, exact and
  free) or an **ask** stage (a registered agent answers a yes/no question about
  the worktree — `PASS` / `FAIL: <reason>` — for goals no shell command can
  check). A hybrid is the ordered mix: cheap deterministic command gates first,
  an ask stage last, so the agent call only runs once the mechanical gates are
  green. The failing stage's output or reason becomes the agent's next feedback.
- **Budgets are hard caps** — max iterations and max wall clock. Exhaustion
  parks the loop; nothing is advisory.
- **Stuck detection** parks early instead of burning budget: the same failure
  N times in a row, or iterations that change nothing.
- **Worktree isolation.** Each loop runs on its own branch in its own
  worktree. Your checkout is never touched; dirty repos are refused by default
  (`--stash`, which the monitor offers, sets your changes aside first —
  restore them whenever with `git stash pop`).
- **Everything on disk is inspectable without loopy** — plain JSON, markdown
  prompts, patches. `cat` is a fully supported interface.

## Quick start in your own repo

```bash
loopy init                      # one-time; offers to register agents it finds
loopy agent add claude --cmd "claude -p {prompt} --permission-mode acceptEdits" --default

loopy "fix the flaky importer test"
```

On first run loopy infers the verifier from your repo (`make check`,
`go test ./...`, `npm test`, …), confirms it with you once, and stores it as
the project default. When you're engineering the loop deliberately:

```bash
loopy run "fix flaky importer test" \
  --verify "go vet ./..." \
  --verify "go test -run TestImporter -count=20 ./importer" \
  --agent codex --max-iters 6 --max-time 20m \
  --forbidden-path vendor/
```

Watch and steer:

```bash
loopy watch                     # the monitor: live tail, timeline, drill-downs
loopy watch --once              # one plain frame for scripts (honors COLUMNS)
loopy list                      # all loops, one line each
loopy status                    # the newest loop in depth (--json for scripts)
loopy log <loop-id> --iter 2    # exactly what happened in iteration 2
loopy pause | resume | abort <loop-id>
```

The monitor is the product's face: start a loop in one terminal, open
`loopy watch` in another, and *see* it converge. The rail lists every loop,
most urgent first; the overview answers the live questions at a glance —
the iteration timeline with verifier stage progression, what the engine is
doing right now (agent or verify, and for how long), the last feedback the
agent saw, and why a stopped loop stopped. Tabs switch to the full live
tail, the cumulative diff, and the verifier log; the diff and verifier tabs
open with a plain-words summary (stat header, per-stage scoreboard, verdict
sentence) before the raw artifact, so the answer leads the evidence. Long
log, diff, and verifier lines wrap instead of being cut off. Mouse support:
the wheel scrolls the pane under the pointer, clicking a rail row selects
it, clicking a tab name switches to it; decisions stay on the keyboard.
`c` copies the next command (or the accepted loop's `git apply` line on a
quiet rail) to the clipboard via OSC 52. Contextual keyboard actions follow
the loop's state: `a` aborts a running loop and accepts a parked green one
(y/n confirm); `r` resumes a paused loop and rejects a parked one (y/n
confirm) — each shells out to the audited CLI. `A` applies an accepted
loop's durable diff to your working tree and then removes the loop — `git
apply` to the working tree only, never a commit, push, or merge (invariant
2); a patch conflict leaves both your tree and the loop untouched. Deciding
a non-green loop stays CLI-only (`loopy accept --override --reason`).

Judge:

```bash
loopy review <loop-id>          # final diff + verifier transcript + history
loopy accept <loop-id>          # audited; non-green needs --override --reason
loopy reject <loop-id> --reason "too broad"   # evidence kept, worktree freed
loopy logbook                   # what was decided, and why, forever
```

Every iteration records `prompt.md` (exactly what the agent was told),
`agent.log`, `verifier.log`, a cumulative `diff.patch`, and `iteration.json`
under `.loopy/loops/<id>/iterations/`. Accepting writes a durable
`final-diff.patch` that applies with `git apply` — shipping it is your call,
on your terms.

## STATUS

What works today:

- The full loop engine: baseline verification, prompt composition with
  verifier feedback, multi-iteration runs, hard budgets, stuck detection,
  forbidden-path enforcement every iteration.
- `init`, `agent add/list/remove`, `run` (+ `loopy "<goal>"` sugar), `list`,
  `status`, `log` (all with `--json`), `pause` / `resume` / `abort`, `doctor`,
  crash resumability, verifier inference.
- **The monitor**: `loopy watch` (Bubble Tea v2) — loop list, live tailing,
  iteration timeline, diff/verifier viewers, pause/resume/abort/accept/reject
  from the keyboard, mouse support (wheel + click), `c` copies next command
  via OSC 52, `A` applies an accepted loop's diff to your working tree and
  removes the loop, `--once` for scripts, PTY smoke tests in CI.
- **The judgment**: `loopy review` (final diff + verifier transcript +
  history), `accept`/`reject` with `--override --reason` recorded verbatim,
  durable `final-diff.patch` and `review.json`, and the `logbook` — the
  project's memory of every decision. The logbook implementation was itself
  built by a loopy loop (see `DECISIONS.md`).
- **The reviewer agent**: `loopy run --reviewer <name>` runs a *different*
  registered agent against the green diff before parking; its critique is
  recorded as `critique.md` and shown by `loopy review` — evidence, never a
  gate.
- **The verifier spectrum**: verifier stages can be a shell command (`exit 0`
  = pass) or an **ask** stage (the loop asks a registered agent a yes/no
  question about the worktree; `PASS`/`FAIL: <reason>`). The wizard composes a
  hybrid instantly — inferred command gates first, the goal as the ask question
  — so loop creation is instant. For an ask-only loop the engine designs a
  deterministic gate in the background and folds it in additively once it
  arrives, without re-sign-off.
- **`loopy run --json` / `loopy resume --json`**: NDJSON event stream for
  scripts and outer orchestrators; `--race` interleaves all loops and ends with
  a `verdict` event. Schema in `docs/orchestration.md`.
- **Race mode and the judge**: `loopy run "<goal>" --race claude,codex` runs
  one loop per agent in parallel worktrees; the deterministic judge ranks
  the parked evidence (smallest clean green diff wins), flags
  dependency-manifest changes and overlapping files, and "no safe winner"
  is a legitimate verdict. `loopy judge <id> <id>` re-ranks any finished
  loops; race verdicts persist under `.loopy/races/`.
- **The release pipeline**: RC-first tags (`v0.x.y-rc.N` → prerelease),
  six CGO-free archives + checksums + a generated homebrew formula per
  release (`make dist` builds them locally), and
  `examples/fizzbuzz-loop/` — a complete, readable `.loopy/` state tree
  from a real run.
- The demo: `scripts/demo.sh`, no API keys, now running the full cycle
  through accept and the logbook.

What doesn't exist yet:
- Scheduled loops, cost budgets, notification hooks.
- Windows is build-verified only: archives are produced and CI
  cross-compiles them, but the engine shells out to `sh` (Git Bash works)
  and nobody runs the test suite there yet.

Claude Code, Codex, and Gemini CLI are all exercised through real loops —
the [tested agent matrix](docs/agents.md) has the exact invocations (Gemini
needs `--skip-trust` to work headless).

## Lineage

loopy is the successor to [crux](https://github.com/mjbarefo/crux) and ports
its battle-tested machinery — the worktree engine, evidence collection,
audited decisions, view-model split — while inverting its default: crux is
human-gated at every joint; loopy is autonomous between two human moments,
loop design and final review.

`DESIGN.md` holds the full design; `DECISIONS.md` logs every deviation from it
and every call the design left open; `DEV.md` is the contributor guide (build,
test, extend).
