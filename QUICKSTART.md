# loopy quickstart

A walk from zero to your first reviewed diff. Plain vocabulary throughout:
a **loop** is a goal + a verifier + a budget; an agent **iterates** in an
isolated worktree until the verifier goes **green** or the budget **parks**
it; you **review** and the diff is yours to ship.

## 0. Install

No binaries are published yet — build from source (Go 1.26+):

```bash
git clone https://github.com/mjbarefo/loopy && cd loopy
make build          # produces ./loopy
```

Put `loopy` somewhere on your PATH, or use `go install ./cmd/loopy`.

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
loopy init
```

`init` creates `.loopy/`, git-ignores it, and lists agent CLIs it found on
your PATH with suggested registrations. Register at least one agent — loopy
makes no model calls of its own:

```bash
loopy agent add claude --cmd "claude -p {prompt} --permission-mode acceptEdits" --default
```

The template variables (`{prompt}`, `{prompt_file}`, `{worktree}`,
`{loop_id}`, `{goal}`, `{iteration}`) are always shell-quoted on expansion.
The suggested flags are suggestions — test your agent's headless mode once
before trusting it with a budget. Commit the `.gitignore` change: loops
refuse to start from a dirty tree.

## 3. Start your first loop

The happy path infers a verifier from your repo (`make check`,
`go test ./...`, `npm test`, …) and confirms it with you once:

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

The monitor: loop list on the left, detail on the right. `tab`/`1-4`
switch between **live** (agent/verifier output tailing), **iterations**
(the convergence timeline), **diff**, and **verifier**. `enter` drills in
to scroll, `p` pauses at the next iteration boundary, `r` resumes, `a`
aborts (with confirmation), `q` quits. The footer always shows the exact
next command. For scripts and CI: `loopy watch --once` prints one plain
frame; `loopy status --json` is the machine-readable view.

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
