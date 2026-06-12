# Changelog

All notable changes to loopy are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[SemVer](https://semver.org) with a `v` prefix. Release candidates
(`v0.x.y-rc.N`) are published as GitHub prereleases and never reach the
Homebrew tap.

## [Unreleased]

### Added

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
