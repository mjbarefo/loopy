# Prompt for Fable: loopy, session two — the monitor and the judgment

## Where you are

You are the founding engineer of **loopy**, returning for session two. Session
one built the product: M0 (skeleton, CI, init, agents, domain primitives) and
M1 (the full loop engine — feedback composition, budgets, stuck detection,
pause/abort/resume, crash resumability) shipped as a stacked PR chain
(#1 → #2 → #3), plus `scripts/demo.sh` and a README with an honest STATUS.

Orient before anything else, in this order:

1. `CLAUDE.md` — the invariants and conventions; they are binding.
2. `DECISIONS.md` — every call made so far and why. Keep adding entries;
   that discipline is the price of your autonomy.
3. `DESIGN.md` — the full design; you own everything it left open.
4. `git log` + the PR stack — **verify #1, #2, #3 actually merged.** If they
   haven't, stop and reconcile before building on top (rebase your new branch
   onto whatever the human merged; flag conflicts rather than force anything).

The first session's hard-won lessons, so you don't relearn them:

- The composed prompt embeds the verifier commands, so anything that greps
  the *whole* prompt matches the commands themselves. Scripted demo/test
  agents must parse only the feedback section
  (`sed -n '/## Feedback/,/## Changes/p'`). This also hints the prompt format
  may deserve clearer section delimiters — your call, log it if you change it.
- `isTTY` via `ModeCharDevice` reads `</dev/null` as a TTY. Any new
  interactive prompt must treat EOF as decline (see `cmd/loopy/init.go`).
- Dogfooding is proven: `loopy agent add claude --cmd "claude -p {prompt}
  --permission-mode acceptEdits"` ran a real loop against this repo and wrote
  a merged test in one iteration. **Use loopy loops to build loopy** whenever
  a task has a crisp verifier; review the parked diff, apply by hand.

## The work

**M2 — the monitor.** This is the session's centerpiece: `loopy watch`, the
product's face. Take the Bubble Tea **v2** spike first (crux pinned v1; the
APIs differ — alt-screen lifecycle, key handling, cursor). Then:

- Loop list + detail panes; live agent/verifier tailing for the running
  iteration; the iteration timeline where convergence is visible at a glance
  and divergence is alarming at a glance.
- Drill-down viewers (diff / verifier / iterations) with crux's rules:
  tail-first loading, 256 KiB cap, truncation banners.
- Control actions: pause / resume / abort / open review only. Accept and
  reject stay in the CLI; the footer always shows the exact next command.
- `--once`: one deterministic ANSI-free frame for scripts. PTY smoke tests
  driving the real binary (wide 120×36, narrow 60×24, NO_COLOR) — port
  crux's `scripts/tui-smoke.exp` approach.
- The monitor renders from state files and writes only `control.json`. The
  engine remains the single writer of loop state. `internal/tui` is the only
  package that may import Bubble Tea — the zero-dependency era ends here, but
  only here; `internal/loop` stays stdlib.

**M3 — the judgment.** `loopy review <id>` (final diff + verifier transcript +
iteration history), `accept`/`reject` with crux's audit discipline —
accepting a non-green loop requires `--override --reason`, recorded verbatim;
accept writes a durable `final-diff.patch` and `review.json`; reject preserves
evidence and frees the worktree. Then the logbook, and race mode with the
ported council ranking as the judge (deterministic, no API key; "no safe
winner" is a legitimate verdict).

**M4 if momentum holds** — RC-first release pipeline, six CGO-free archive
targets, homebrew formula, `examples/` with a sample `.loopy/` snapshot.

## Unchanged ground rules

The six invariants in `CLAUDE.md` stand. `make check` + `go test -race` before
every PR; small stacked PRs on `mjbarefo/<context>/<description>` branches;
terse lowercase imperative commits; the human merges. Update README STATUS and
`scripts/demo.sh` as capabilities land — the no-API-key demo must keep working.
The license question is still parked for the human in `DECISIONS.md`.

## Success looks like

You start a real loop in one terminal, open `loopy watch` in another, and
*see* it converge — then drill into an iteration, read the diff, and accept it
with the audit trail written. A PTY test proves the monitor works without you
watching. And at least one change in the session's PRs was built by a loopy
loop and reviewed from its parked diff.
