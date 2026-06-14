# Changelog

All notable changes to loopy are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[SemVer](https://semver.org) with a `v` prefix. Release candidates
(`v0.x.y-rc.N`) are published as GitHub prereleases and never reach the
Homebrew tap.

## [Unreleased]

### Added

- Monitor: **`A` applies an accepted loop's diff to your working tree.**
  After you accept a green loop, its diff was durable but you had to copy or
  retype `git apply <…/final-diff.patch>` yourself; now `A` runs it for you,
  behind a y/n confirm. This is the only place the monitor touches your
  checkout, and it is the weakest possible touch — `git apply` to the working
  tree, **never a commit, push, or merge** — so the patch lands as an
  uncommitted change you review, commit, and ship. A patch that doesn't fit
  leaves your tree untouched and says so.
- The agent now **designs a deterministic gate in the background** for an
  ask-only loop. Start a loop with just a plain-English question (the instant
  hybrid) and it runs immediately; meanwhile loopy asks your agent to propose
  a fast shell command that is red until the goal is met (`test -f x && make
  check`, `git diff --check`, …). When the proposal lands it folds into the
  live verifier *ahead* of the question, so the cheap deterministic check
  short-circuits before the agent is asked again. It is purely additive — it
  only makes "green" stricter, never auto-accepts (you still seal the diff at
  review) — and a failed, timed-out, or already-passing proposal is a silent
  no-op that leaves your question-only verifier exactly as it was. No model
  call of loopy's own: it runs your registered agent, so the no-API-key demo
  is untouched. (Design: `docs/verifier-spectrum.md`.)
- Verifier stages can now be an **ask** stage: instead of a shell command, a
  stage poses a yes/no question and a registered agent answers it about the
  current worktree (`PASS` / `FAIL: <reason>`). Use it for goals no shell
  command can check — "is the prose accurate?", "does this read cleanly?" —
  alongside the deterministic gates (`test -f x && make check`) that stay
  fast and key-free. Stages still run fast-to-slow and short-circuit, so an
  ask stage only spends an agent call once the command gates ahead of it are
  green. An ask stage's `FAIL` reason becomes the next iteration's feedback,
  the same as a failing command's output. It fails closed (a timeout, an
  unrunnable agent, or a missing verdict all read as `FAIL`) and never makes
  a model call of loopy's own — it runs your registered agent, so the
  no-API-key demo and inference stay command-only. (Engine support;
  wizard/monitor surfacing to follow. Design: `docs/verifier-spectrum.md`.)

### Changed

- Monitor: long log, diff, and verifier lines now **wrap** instead of being
  cut off at the right edge. A wrapped line preserves its exact columns — the
  leading indent and the `+`/`-` gutter survive, and each row keeps the line's
  color — so a long verifier command or a wide diff stays readable. A
  pathological line (a minified bundle, a base64 blob) is capped after a few
  rows with a trailing …; the raw artifact on disk still has every byte.
- Monitor: a focused **green** loop now advertises its review actions in the
  footer — `a accept · r reject` — instead of hiding them behind `?`. The keys
  always worked; they were just invisible at the one moment you reach for them.
  A parked loop surfaces `r reject` (accepting a red loop stays a deliberate
  CLI override). The next-command handoff keeps its place on the right.
- Monitor: the loop detail header is restructured for calm and hierarchy. The
  loop id leads (with its status glyph), a single label-free meta line carries
  `status · agent · iter · wall`, and the goal stands on its own as the hero
  with room to breathe — no more `goal`/`agent` labels crowding the top. The
  activity/park-reason sits below the goal. (Color stays disciplined: the
  glyph carries the state, the words stay plain.)
- Monitor wizard: the verifier step is now a **hybrid, composed instantly** —
  no more multi-minute pause on loop creation. The **ask question is the hero**
  (plain English the agent judges, defaulting to your goal); `checks` is an
  optional, clearly-labelled shell gate below it, so a description never lands
  in a shell by mistake. ↑↓ switches; either can be cleared for an ask-only or
  command-only verifier; enter is your sign-off. The agent's judgment now
  happens inline at verify-time instead of blocking the wizard up front. `tab`
  is the optional polish: it asks the agent to design tighter command gates.
  (Replaces the always-on up-front synthesis; the `loopy run` CLI is unchanged.)
- Monitor verifier tab: a cleaner per-stage scoreboard — bold stage names,
  ask stages rendered as `asks <agent>: "<question>"`, and the verdict word
  colored (green/red/yellow) with the explanation plain. A command stage that
  exits 127 (the classic "I typed a description, not a command" mistake) now
  gets a verdict that points at the fix — clear the check to let the agent
  judge, or write real shell — instead of a generic failure.
- Monitor: a loop that parks green at baseline (the verifier passed before
  the agent ever ran) no longer looks like a win. It gets the yellow `!` in
  the rail and title, its own header count ("already green — check the
  verifier") instead of joining "green to review", and the verifier tab's
  verdict says plainly that the verifier may not test the goal.
- Monitor: the goal (up to three lines) and the activity line (up to two)
  wrap under hanging indents instead of truncating to one line, so long
  goals and park reasons stay readable. The welcome splash names the mouse
  bindings so they can be discovered.

### Added

- Monitor: mouse support. The wheel scrolls the pane under the pointer —
  the detail body by lines, the loop rail by selection; clicking a rail row
  selects that loop, clicking a view name in the nav switches to it.
  Decisions stay explicit: pending y/n confirmations ignore clicks, and the
  wizard remains keyboard-driven. Since mouse capture takes over native
  text selection (hold Option/Shift to bypass), the new `c` key copies the
  next command to the system clipboard via OSC 52 — on a quiet rail it
  copies the newest accepted loop's `git apply` command, the one on screen.
- Monitor: the diff and verifier tabs answer first, evidence below. The
  diff tab opens with "N files changed · +A -D" and a per-file list before
  the patch (adds green, removals red, headers bold); the verifier tab
  opens with a per-stage ✓/✗ scoreboard and a plain-words verdict before
  the log, where stage markers recede and — on a red run — passing stages'
  output dims so the failure reads. Both tabs open at the top; `G` still
  jumps to the tail. Readable without color: glyphs and words carry every
  verdict.

- The reviewer agent: `loopy run --reviewer <name>` runs a *different*
  registered agent against the green diff before parking; its critique is
  recorded as `critique.md` and shown by `loopy review` — evidence, never a
  gate. Any change the reviewer makes to the worktree is reverted.
- `loopy delete <loop-id>` removes a loop entirely — worktree, branch, and
  evidence — while the logbook keeps one line recording the deletion. The
  monitor's `d` key confirms and calls it; loops with a live engine are
  refused.
- Monitor: decided (accepted/rejected) loops leave the rail — the header
  count and the logbook keep the history; `loopy watch <id>` still pins one.
- Monitor: accept and reject without leaving the TUI. The keys are
  contextual — `a` aborts a moving loop and accepts a parked green one,
  `r` resumes a paused loop and rejects a parked one — each behind the same
  y/n confirmation as delete, shelling out to the audited CLI. Accepting a
  non-green loop remains CLI-only (`loopy accept --override --reason`).
- `loopy run --json` and `loopy resume --json`: progress as an NDJSON event
  stream for scripts and outer orchestrators, ending in a `result` event
  that carries the full loop view (`--race` interleaves all loops on one
  stream and ends with a `verdict` event). Schema and a worked example in
  `docs/orchestration.md`.
- Monitor wizard: on the verifier step, `tab` asks the selected agent to
  propose a goal-testing verifier — the monitor keeps running while the
  agent explores a throwaway worktree, the proposal lands in the editable
  field attributed to its agent, and enter remains your sign-off. The whole
  loop-creation flow now lives inside bare `loopy`.
- `loopy run "<goal>" --verify auto`: the agent proposes the goal-testing
  verifier. The registered agent runs once in a throwaway worktree, its
  proposed command is trial-run there (an already-passing proposal is
  flagged), and you confirm before the loop starts — goal-specific, never
  stored as the project default, refused without a TTY. Baseline-green
  parks now hint at it.
- `loopy agent check [name]`: smoke-run a registered agent (or all of them)
  against a trivial prompt in a throwaway directory — trust prompts, dead
  auth, and missing CLIs surface in seconds, with the agent's own words and
  the re-registration hint on failure; exits 1 so setup scripts can gate.
  `agent add` warns when the command's binary is missing from PATH, and
  `loopy doctor` checks every registered agent the same way. The suggested
  gemini command now includes `--skip-trust` (required for headless work in
  loop worktrees; proven by a real loop).
- Engine: a nonzero agent exit that leaves the worktree untouched parks as
  `agent blocked (exit N): <the agent's own last words>` instead of generic
  "stuck" — the park reason now names the environment problem (trust prompt,
  dead auth, missing CLI) and its fix. Exit 0 with no change remains "stuck";
  a nonzero exit with real changes is still judged by the verifier alone.

### Fixed

- Monitor: deciding the last undecided loop no longer leaves it pinned in
  the rail looking un-decided. The rail goes quiet ("all quiet — every loop
  is decided") and keeps the newest accepted loop's `git apply` command on
  screen — the bridge from an accepted diff to a branch and a PR.
  (`loopy watch <id>` still pins a decided loop explicitly.)

## [v0.1.0] - unreleased

The first release: the complete loop engine and review workflow.

### Added

- **The loop engine**: goal + verifier + budget → an agent iterates in an
  isolated git worktree until green or the budget runs out. Baseline verify,
  feedback-driven prompt composition, hard iteration/wall-clock budgets,
  stuck detection (identical failures, no-change iterations), forbidden-path
  enforcement every iteration, crash resumability, pause/resume/abort.
- **The monitor** (`loopy watch`): urgency-sorted loop rail, overview with
  the iteration timeline and verifier stage progression, a live activity
  line (agent/verify phase with elapsed time), full live tail, diff and
  verifier viewers with capped tail-first loading, safe controls
  (pause/resume/abort), `--once` deterministic frames for scripts.
- **The judgment**: `loopy review` (final diff + verifier transcript +
  history), audited `accept`/`reject` (`--override --reason` recorded
  verbatim), durable `final-diff.patch`, the logbook.
- **Race mode**: `--race a,b` runs one loop per agent in parallel worktrees;
  the deterministic judge ranks parked evidence; "no safe winner" is a
  legitimate verdict. `loopy judge` re-ranks any finished loops.
- **Operability**: `init` with verifier inference and agent detection,
  `doctor` with repair guidance, `--json` everywhere it makes sense,
  `NO_COLOR` support, exit-code contract (0 success / 1 runtime / 2 usage;
  `loopy run` exits 0 only when the loop parks green).
- **Zero-key demo**: `scripts/demo.sh` runs the whole arc with a scripted
  shell agent — no API keys anywhere.

[Unreleased]: https://github.com/mjbarefo/loopy/compare/v0.1.0...HEAD
[v0.1.0]: https://github.com/mjbarefo/loopy/releases/tag/v0.1.0
