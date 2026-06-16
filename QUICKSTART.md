# loopy quickstart

A walk from zero to your first reviewed diff. Plain vocabulary throughout:
a **loop** is a goal + a verifier + a budget; an agent **iterates** in an
isolated worktree until the verifier goes **green** or the budget **parks**
it; you **review** and the diff is yours to ship.

## 0. Install

Homebrew (macOS and Linux):

```bash
brew install mjbarefo/tap/loopy
loopy version
```

Or grab an archive from the
[latest release](https://github.com/mjbarefo/loopy/releases/latest)
(checksums in `SHA256SUMS`, provenance via `gh attestation verify`), or
build from source (Go 1.26+):

```bash
go install github.com/mjbarefo/loopy/cmd/loopy@latest
```

## 1. See one converge first (zero API keys, ~30 seconds)

```bash
scripts/demo.sh
```

This builds loopy, creates a throwaway repo with a three-bug fizzbuzz,
registers a *shell script* as the agent, and runs the whole arc: baseline
red → three feedback-driven fixes → green → review → accept → logbook.
Read its output top to bottom once — it is the entire product in miniature.
(`examples/fizzbuzz-loop/` is the state tree such a run leaves behind, with
a guided README.)

## 2. Prepare your own repo

```bash
cd ~/code/your-project
loopy
```

Bare `loopy` launches the monitor, and its empty state walks you through
setup in place: press `i` to initialize the repo (creates `.loopy/`,
git-ignores it), press a digit to register an agent CLI it found on your
PATH, then press `n` to start your first loop. The same setup works from
the CLI if you prefer it scripted:

```bash
loopy init
loopy agent add claude --cmd "claude -p {prompt} --permission-mode acceptEdits" --default
```

The template variables (`{prompt}`, `{prompt_file}`, `{worktree}`,
`{loop_id}`, `{goal}`, `{iteration}`) are always shell-quoted on expansion.
[docs/agents.md](docs/agents.md) is the tested invocation matrix (Claude
Code and Codex are exercised through real loops) — test any new agent once
with a tiny goal before trusting it with a budget. Commit the `.gitignore`
change: loops refuse to start while tracked files have uncommitted changes —
or pass `--stash` (the monitor offers this when you start a loop) to set them
aside first, recoverable any time with `git stash pop`.

## 3. Start your first loop

In the monitor, press `n`, describe the goal, and hit enter — the form
shows exactly which verifier, agent, and budget the loop will use before
you commit. From the CLI, the same happy path infers a verifier from your
repo (`make check`, `go test ./...`, `npm test`, …) and confirms it with
you once:

```bash
loopy "fix the flaky importer test"
```

When you're engineering the loop deliberately, say exactly what "done"
means — the verifier is both the definition of done and the feedback the
agent sees after every iteration, so order stages fast → slow:

```bash
loopy run "fix the flaky importer test" \
  --verify "go vet ./..." \
  --verify "go test -run TestImporter -count=20 ./importer" \
  --max-iters 6 --max-time 20m \
  --forbidden-path vendor/
```

### The verifier spectrum

A verifier is an ordered list of stages; they short-circuit on the first
failure, cheapest first. A stage can be a shell **command** or a
plain-English **ask** question:

| | Command stage | Ask stage |
|---|---|---|
| Verdict from | `sh -c cmd`, exit 0 = pass | the loop's agent answers a yes/no question |
| Cost | free, no API key | one agent call |
| Good at | anything mechanical: builds, tests, file checks, lint | quality and intent: "is the prose accurate?", "does this API read cleanly?" |

A **hybrid** is the default the wizard builds: inferred command gates first,
then an ask question derived from your goal (no pause, composed instantly).
The ask stage only runs after the command gates are green, so you never
spend an agent call on a build that doesn't compile.

In the monitor wizard, `checks` is the optional shell gate and the **ask
question is the hero field** (defaults to your goal). Press `↑`/`↓` to
switch between them; either can be cleared for an ask-only or command-only
verifier; `tab` asks the agent to propose tighter command gates as optional
polish.

For an **ask-only loop** (goal-as-question, no shell gate), the engine
designs a deterministic command gate in the background while the loop
iterates immediately under your ask verifier. When the proposal lands it
folds in ahead of the ask stage — cheap check first, ask only when green —
with no creation-time pause and no re-sign-off from you. A failed or
already-passing proposal is a silent no-op.

An ask stage's `FAIL: <reason>` becomes the next iteration's feedback,
exactly like a failing command's output.

Rules that protect you, always on:

- **No verifier, no loop.** Creation refuses an empty verifier.
- **Budgets are hard caps.** Exhaustion parks the loop with its history.
- **Stuck detection parks early** — the same failure three times in a row,
  or an iteration that changes nothing, stops the burn.
- The agent works in `.loopy/worktrees/<loop-id>/` on branch
  `loopy/<loop-id>` — your checkout is never touched.

## 4. Watch it

```bash
loopy watch          # in a second terminal
```

The monitor: every loop on the left, most urgent first; the selected loop
on the right. The default **overview** shows the iteration timeline (with
how far through the verifier each iteration got), what the engine is doing
right now, and the last feedback the agent saw. `tab`/`1-4` switch to
**live** (the full output tail), **diff**, and **verifier**. `enter` drills
in to scroll. The footer always shows the exact next command.

Keys are contextual — what a key does depends on the loop's state:

| Key | Moving loop | Paused loop | Green loop | Parked loop |
|---|---|---|---|---|
| `p` | pause at next boundary | — | — | — |
| `r` | — | resume | reject (confirms) | reject (confirms) |
| `a` | abort (confirms) | — | accept (confirms) | — |
| `d` | — | delete (confirms) | delete (confirms) | delete (confirms) |
| `q` | quit | quit | quit | quit |

A focused green loop's footer advertises `a accept · r reject` so the
actions are visible at the moment you reach for them. A parked loop shows
`r reject`.

Additional keys: **`A`** applies the most recently *accepted* loop's diff to
your working tree and removes that loop (confirms) — the follow-up to `a`
(see §5), so accept first, then `A`; `c` copies the next command to the
clipboard via OSC 52 (hold Option/Shift to select text normally while mouse
capture is on); `o` quits and prints the next command; `?` shows the full
key reference.

**Mouse:** the wheel scrolls whichever pane is under the pointer (the detail
body by lines, the loop rail by selection); clicking a rail row selects that
loop; clicking a view name in the nav switches to it. Pending confirmations
ignore clicks.

For scripts and CI: `loopy watch --once` prints one plain frame;
`loopy status --json` is the machine-readable view.

## 5. Judge the result

A green loop is parked, not shipped. Look at everything in one place:

```bash
loopy review <loop-id>     # final diff + verifier transcript + history
```

Then decide, on the record:

```bash
loopy accept <loop-id>     # writes final-diff.patch + review.json
loopy reject <loop-id> --reason "right tests, wrong approach"
```

Accepting a loop that is *not* green requires
`--override --reason "<why>"`, recorded verbatim forever. Accept prints
the apply command — shipping stays your move:

```bash
git apply .loopy/loops/<loop-id>/final-diff.patch
```

You can also do this without leaving the monitor: press `a` on a green loop
to accept it, then `A` to apply its diff to your working tree and clear it
from the rail. `A` runs `git apply` only — never a commit, push, or merge
— so the patch lands as an uncommitted change you review and ship. A patch
that doesn't fit leaves your tree and the loop untouched.

Every decision lands in the project's memory:

```bash
loopy logbook
```

## 6. When one agent isn't enough: race

```bash
loopy run "make the importer handle quoted newlines" --race claude,codex
```

One loop per agent, parallel worktrees. First green does **not** win: when
all park, the deterministic judge ranks the evidence — smallest clean green
diff first — and flags dependency-manifest changes and overlapping files.
"No safe winner" is a legitimate verdict (exit 1). Re-rank any finished
loops later with `loopy judge <id> <id>`.

## 7. When something looks wrong

```bash
loopy doctor               # environment + state diagnosis, repair guidance
loopy log <loop-id>        # the full recorded history, iteration by iteration
```

Everything loopy knows is plain files under `.loopy/` — every prompt, log,
diff, and verdict. `cat` is a fully supported interface.

## The habit

Designing the loop is the engineering: a crisp verifier, a budget you can
afford, constraints for what must not change. Cheap-to-verify tasks make
great loops; taste-heavy tasks don't. Start small — one failing test and
`--max-iters 3` — and review every parked diff like it came from a
stranger, because it did.
