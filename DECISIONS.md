# DECISIONS

A running log of deviations from `DESIGN.md` and calls the design left open.
One entry each: what was decided, and why. Newest at the bottom of each section.

## Decisions

- **2026-06-10 — Module path is `github.com/mjbarefo/loopy`.** Matches the
  origin remote and crux's lineage.

- **2026-06-10 — Zero dependencies through M1.** The design already requires a
  stdlib-only domain layer; M0/M1 don't need the TUI, so `go.mod` stays
  dependency-free until Bubble Tea v2 lands in `internal/tui` at M2. Easier
  review, trivially auditable supply chain for the early milestones.

- **2026-06-10 — M0's "single-iteration loop" folded into the M1 PR.** A
  one-shot engine would be scaffolding we'd delete a day later. M0 ships the
  repo skeleton, CI, `init`, agent registration, and the domain primitives
  (store, git/worktrees, verifier runner); the engine arrives whole in M1.

- **2026-06-10 — Baseline verify before iteration 1 (recorded as iteration 0).**
  The first agent prompt needs verifier feedback to act on, and a loop whose
  verifier is already green should park green without spending an agent run.
  So the engine runs the verifier once, agent-free, before iterating; the
  record lives at `iterations/0000` and renders as "baseline".

- **2026-06-10 — All agent template variables are shell-quoted on expansion.**
  `{prompt}` substitutes the full prompt text; unquoted it would be a shell
  injection from the verifier's own output. Every variable (`{prompt}`,
  `{prompt_file}`, `{worktree}`, `{loop_id}`, `{goal}`, `{iteration}`) expands
  single-quoted. `{prompt_file}` (path to `prompt.md`) is an addition for
  agents that prefer reading a file.

- **2026-06-10 — No built-in agent templates.** crux shipped built-ins; loopy
  doesn't, because the headless-flag matrix is untested (design open question)
  and a silently wrong default is worse than none. `loopy init` detects
  installed agent CLIs and offers a registration command instead; `agent add`
  help shows the suggested invocations, labeled as suggestions.

- **2026-06-10 — `loopy run` exit codes: 0 only when the loop parks green;
  1 when it parks red (budget/stuck/abort) or fails; 2 usage.** Scripts and CI
  can gate on the loop's verdict directly.

- **2026-06-10 — M1 streams plain progress lines even on a TTY.** The design
  says attach the monitor when stdout is a terminal; the monitor doesn't exist
  until M2, and the plain stream must be good regardless (CI path). Auto-attach
  comes with M2.

- **2026-06-10 — No `draft` loop status.** Loops are created and immediately
  run; there is no separate authoring step in v0. Statuses: running, paused,
  green, parked, accepted, rejected.

- **2026-06-10 — Single-writer ownership instead of file locks.** The engine
  owns `loop.json` and `iterations/`; other processes own `control.json` only.
  `engine.lock` (pid + start time) prevents two engines on one loop; a lock
  whose pid is dead is stale and resumable. All JSON writes are
  temp-file+rename atomic.

- **2026-06-10 — Abort is honored mid-phase, pause only at phase boundaries.**
  The design promises a signal to the agent's process group on abort; the
  engine watches `control.json` every 2s while the agent or a verifier stage
  runs and kills the process group on abort. Pause stays a phase-boundary
  affair as designed.

- **2026-06-10 — `loopy init` ensures `.loopy/` is in `.gitignore`.** Worktrees
  and live state under `.loopy/` would otherwise dirty the repo and trip the
  dirty-repo refusal on the next loop. Durable records the user may want to
  commit (logbook) can be revisited at M3.

- **2026-06-10 — Wall-clock budget counts engine work, not idle time.**
  `wall_clock_used` accumulates per-iteration durations and survives
  pause/crash/resume; a loop paused overnight doesn't burn its budget while
  parked. Checked at phase boundaries — a single slow verifier stage can
  overshoot, and that overshoot is recorded, not hidden.

- **2026-06-10 — `loopy "<goal>"` sugar requires a multi-word argument.** A
  single-word unknown command reads as a typo, not a goal; turning `loopy lst`
  into a loop named "lst" would be hostile. Multi-word arguments (anything
  with whitespace) start a loop; single words get the unknown-command error
  with a pointer to `loopy run`.

- **2026-06-10 — `loopy status` with no ID shows the newest loop.** That's the
  loop you're most likely watching; `loopy list` is the overview.

- **2026-06-10 — Pause exits 0 from `loopy run`/`resume`.** An intentional
  pause is not a failure; only red parks (budget, stuck, abort) and runtime
  errors exit 1.

- **2026-06-10 — Iterations record precise `wall_ms` alongside RFC3339
  timestamps.** Second-resolution timestamps are for humans; budget
  accounting uses milliseconds so fast iterations don't vanish from
  `wall_clock_used`.

- **2026-06-10 — Verifier inference fails closed without a TTY.** A guessed
  verifier is never used unconfirmed: interactive runs confirm once and store
  it; non-interactive runs must pass `--verify` or have a stored default.

## For the human

- **License.** The repo has no LICENSE file. crux's license should probably be
  matched, but publishing terms are yours. Reversible default taken: none yet.
