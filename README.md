# loopy

```
 ██░░██░░░░██░░██
 ██░░░░██████░░░░██        l o o p y
 ██░░░░██░░██░░░░██
 ██░░░░██████░░░░██   engineer loops, not prompts
 ██░░██░░░░██░░██
```

**You define a goal, a verifier, and a budget. An agent iterates in an
isolated git worktree until the verifier goes green or the budget runs out.
You review the result with the full iteration history in front of you.**

```bash
loopy "make the CSV importer handle quoted newlines"
```

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

What stays human, permanently:

- **Designing the loop.** Choosing the goal, the verifier, and the budget *is*
  the engineering. A loop with a weak verifier converges on garbage, quickly.
- **Reviewing the result.** loopy never merges, never commits to your
  branches, never pushes. A green verifier earns a parked diff and a review.
  Humans ship.

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
  verifier command. The verifier is the loop's definition of done *and* its
  feedback signal: the failing stage's output tail is what the agent sees next.
- **Budgets are hard caps** — max iterations and max wall clock. Exhaustion
  parks the loop; nothing is advisory.
- **Stuck detection** parks early instead of burning budget: the same failure
  N times in a row, or iterations that change nothing.
- **Worktree isolation.** Each loop runs on its own branch in its own
  worktree. Your checkout is never touched; dirty repos are refused.
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
`loopy watch` in another, and *see* it converge — live agent/verifier
tailing, the iteration timeline, and drill-down viewers for the diff and
verifier log (tail-first, 256 KiB cap, truncation banners). It takes only
the safe, reversible actions — pause, resume, abort (with confirmation) —
and always shows the exact next command in the footer. Accept and reject
stay in the CLI.

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
  iteration timeline, diff/verifier viewers, pause/resume/abort from the
  keyboard, `--once` for scripts, PTY smoke tests in CI.
- **The judgment**: `loopy review` (final diff + verifier transcript +
  history), `accept`/`reject` with `--override --reason` recorded verbatim,
  durable `final-diff.patch` and `review.json`, and the `logbook` — the
  project's memory of every decision. The logbook implementation was itself
  built by a loopy loop (see `DECISIONS.md`).
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

What doesn't exist yet (in design order — see `DESIGN.md`):
- A first tagged release — the pipeline is ready; cutting `v0.1.0-rc.1` is
  the human's call. Until then, build from source.
- The headless agent matrix (exact flags per agent CLI) is documented as
  suggestions, not yet systematically tested.

## Lineage

loopy is the successor to [crux](https://github.com/mjbarefo/crux) and ports
its battle-tested machinery — the worktree engine, evidence collection,
audited decisions, view-model split — while inverting its default: crux is
human-gated at every joint; loopy is autonomous between two human moments,
loop design and final review.

`DESIGN.md` holds the full design; `DECISIONS.md` logs every deviation from it
and every call the design left open.
