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

- **2026-06-11 — Bubble Tea v2 stable (v2.0.7) via the `charm.land` vanity
  path; no styling library.** v2 went stable since the design was written
  (crux had to pin v1). The monitor hand-rolls layout and ANSI color instead
  of taking lipgloss: frames stay byte-deterministic for `--once` and the
  frame unit tests, and the dependency surface stays one library. Only
  `internal/tui` imports it; `internal/loop` is still stdlib-only.

- **2026-06-11 — `loopy run` does not auto-attach the monitor.** The design
  suggested attaching on a TTY; instead a TTY run prints a
  `loopy watch <id>` hint. Two terminals beat one hijacked stream — the
  engine's plain log stays scrollback, the monitor stays optional, and the
  no-TTY path needs no separate code.

- **2026-06-11 — The monitor's resume spawns a detached `loopy resume`
  engine; everything else is control.json or a hint.** A paused loop has no
  engine to poll control.json, so in-monitor resume must start one — the
  spawned child is a normal engine under the normal lock, so the
  single-writer rule holds. Abort of a loop with no live engine is *not*
  taken in the monitor (it would mean writing loop state); the footer points
  at `loopy abort <id>` instead.

- **2026-06-11 — `watch --once` renders the iterations view, ANSI-free, at
  width 100 (COLUMNS overrides).** One deterministic frame for scripts:
  same renderer as the live monitor, color off, content-sized height,
  convergence timeline rather than a log tail.

- **2026-06-11 — review.json is the decision record; logbook.md is the
  narrative.** `loopy logbook --json` aggregates the per-loop review.json
  files instead of maintaining a second structured store — one source of
  truth, and the markdown stays a human document.

- **2026-06-11 — Accept keeps the worktree; only reject frees it.** The
  design states the asymmetry and it holds up: an accepted diff may still be
  compared against its worktree while being applied; a rejected one is dead
  weight. `final-diff.patch` is durable either way.

- **2026-06-11 — The judge's total order, and when it names a winner.**
  Ranking: green before red; an applicable diff before an empty one;
  manifest-free before manifest-touching; then fewer changed files, smaller
  diff, fewer iterations, less wall clock, loop ID. A winner is named only
  when the top candidate is green with a non-empty diff and no
  dependency-manifest changes — otherwise "no safe winner", which exits 1
  from a race. Overlapping files between green candidates are flagged, not
  disqualifying (the human picks one diff anyway).

- **2026-06-11 — `loopy judge <id> <id>…` is a command, not just race
  internals.** The design only routed the judge through `--race`; exposing
  it directly makes verdicts reproducible after the fact and the ranking
  testable from the CLI. Races persist their verdict under
  `.loopy/races/<race-id>/race.json`.

- **2026-06-11 — The logbook was implemented by a loopy loop.** Loop
  `implement-the-logbook-in-internal`: stubs + failing tests committed,
  claude agent, verifier `gofmt`/`go build`/targeted `go test`, green in one
  iteration; the diff was reviewed with the new `loopy review` and applied by
  hand, then the loop was accepted using the logbook code it had itself
  written. Dogfooding is the development model now: any task with a crisp
  verifier should go through a loop.

- **2026-06-11 — RC-first releases via tag push; the formula is a release
  asset.** Tagging `v0.x.y-rc.N` publishes a prerelease, `v0.x.y` a release;
  same pipeline (gate → six CGO-free archives → SHA256SUMS → generated
  homebrew formula). The formula is attached to the release rather than
  pushed to a tap — creating the tap repo and the first tag are the human's
  moves.

- **2026-06-11 — `examples/fizzbuzz-loop/.loopy` is the demo's real output,
  path-scrubbed.** A hand-written sample would drift; this one regenerates
  from `scripts/demo.sh` and only rewrites the throwaway repo path. The
  repo's own `.gitignore` entry was root-anchored (`/.loopy/`) so the
  example state can be committed — `loopy init` accepts the anchored form.

- **2026-06-11 — Dirty-repo refusal ignores untracked files.** Loop worktrees
  branch from HEAD and never see untracked files, so they can't make a diff
  unreproducible; refusing on them blocked loops for any repo with a stray
  scratch file. Only modified tracked files refuse, and the error names them.

- **2026-06-11 — One corrupt loop.json degrades, never bricks.** `ListLoops`
  skips unreadable loop state and reports it (`list` warns on stderr, the
  monitor shows the broken entry, doctor gives the exact repair command)
  instead of failing every command for every loop. Same for corrupt
  iteration.json in views; the engine stays strict — it must not resume on
  silently missing history.

- **2026-06-11 — The engine publishes phase.json while a phase runs.** The
  monitor used to guess "what is it doing" from log mtimes; now the engine
  (still the single writer) records {iteration, phase: agent|verify,
  started_at} at phase starts and clears it at boundaries. Ephemeral; only
  meaningful while the engine lock is held.

- **2026-06-11 — `list --json` emits {loops, broken}, not a bare array.**
  Damaged state must be visible to scripts too, and a wrapper object can
  grow fields without breaking consumers. Decided before the first release,
  while the JSON contract is still free.

- **2026-06-11 — Layout truncation is display-width aware.** `loopy list`
  byte-sliced goals (cutting UTF-8 mid-rune) and the monitor counted runes
  (CJK/emoji broke frame alignment). DisplayWidth/TruncateDisplay/PadDisplay
  in internal/loop are the only truncation primitives now. Implemented by a
  loopy loop (claude agent); the loop parked red because the hand-written
  test fixture had an off-by-one the agent was forbidden to fix — it
  diagnosed the bad vector in a comment instead. Accepted with the audited
  override; the fixture fix is recorded in the loop's review.json.

- **2026-06-11 — Monitor redesign: overview-first, open layout.** The M2
  monitor defaulted to a live tail that is blank for quiet agents and packed
  its facts into a box border. The redesign makes the overview the default
  tab (timeline + activity + feedback + a short live tail), adds an activity
  line driven by phase.json ("now: agent running · iter 3 · 1m32s"), sorts
  the rail by what needs eyes (live → paused → stale → green → parked →
  decided, newest first within a group) and defaults selection to the top,
  sizes the rail to its content, replaces box chrome with rules (more
  content rows, fewer padding bugs), drops whole footer key hints instead of
  cutting words, and suppresses the circular "next: loopy watch <id>" inside
  the live monitor. `--once` keeps the next command and omits key hints.
  The mascot lives only in the empty state, which is now a tailored
  three-step onboarding checklist. Verifier stage progression ("✗ test
  (2/3)") is the convergence signal in every timeline.

- **2026-06-11 — MIT license, owner-approved.** crux is private with no
  LICENSE, so there was no lineage to match; the owner picked MIT from a
  recommendation (simplest terms, matches the Bubble Tea dependency,
  friction-free for Homebrew). LICENSE, the formula, and the archives agree.

- **2026-06-11 — Headless agent matrix is now tested, and codex needs
  `--full-auto`.** Real loops on 2026-06-11: claude
  (`claude -p {prompt} --permission-mode acceptEdits`) converged over four
  feedback iterations; plain `codex exec {prompt}` runs read-only and parks
  stuck ("workspace is mounted read-only"), `codex exec --full-auto
  {prompt}` went green. `loopy init` suggestions and docs/agents.md updated;
  gemini remains a labeled suggestion.

- **2026-06-11 — CI claims match CI proof.** macOS runs the full gate
  (tests, race, PTY smoke, demo) alongside Linux; Windows is build+vet only
  because the engine shells out to `sh` — README says exactly that. Actions
  are pinned by commit SHA.

- **2026-06-11 — Release hardening: exact-SemVer tags from main, full gate,
  attestations, no SBOM file.** The workflow refuses tags that aren't
  vX.Y.Z[-rc.N], commits not on main, and versions missing from
  CHANGELOG.md; it re-runs the complete gate (incl. race, PTY smoke, demo),
  verifies the embedded version from an actual unpacked archive, signs
  GitHub build-provenance attestations for every archive, and is idempotent
  on rerun (upload --clobber + notes edit). No separate SBOM: the binaries
  embed their module graph (`go version -m`), which can't drift from the
  build.

- **2026-06-11 — Homebrew: upstream-binary formula in a real tap; RCs never
  reach it.** The formula installs the attested release archives (fast,
  provenance-checkable) rather than building from source. Canonical tap
  content lives in packaging/homebrew-tap/ (reviewable in this repo);
  scripts/tap-bootstrap.sh assembles and publishes it. Stable releases
  notify the tap via repository_dispatch with a fine-grained PAT
  (TAP_GITHUB_TOKEN, Contents:rw on the tap only); the tap's update
  workflow hard-rejects prerelease versions. Full cycle proven locally:
  brew style/audit clean, install → `loopy v0.1.0` → test → uninstall →
  reinstall via a local tap with mirrored archives.

- **2026-06-11 — No shell completions or man pages in v0.1.** The CLI is
  hand-rolled (no cobra); completions would be a second, hand-maintained
  description of the command surface that will drift. `loopy help` and
  subcommand `--help` are the contract. Revisit post-v0 if the surface
  stabilizes.

- **2026-06-11 — Bare `loopy` launches the monitor; the whole first run
  lives there.** On a terminal, `loopy` with no arguments opens the monitor
  behind a welcome splash (logo, version, repo, counts — dismissed by any
  key; pipes still get the help text). The empty state became executable
  onboarding: `i` initializes the repo, digits register detected agent CLIs
  (first one becomes the default), `n` opens the new-loop form. The form
  resolves the verifier exactly like `loopy run` — project default, else
  inference where *starting the loop is the confirmation* that stores it
  (the CLI's confirm-once contract, same storage). Loop creation uses the
  same domain call as the CLI and hands the loop to a detached engine via
  the existing resume path, so the engine remains the single writer of loop
  state and the monitor still only ever writes control.json plus the same
  setup files the CLI writes (agents.json, config.json) on explicit user
  action. Accept/reject stay in the audited CLI. `watch` and `--once` are
  unchanged (no splash).

- **2026-06-11 — Monitor identity pass: the splash's vocabulary in the
  working frames.** The splash set the identity (cyan logo, bold wordmark,
  dim metadata, one accent doing real work); the monitor now speaks it
  without losing a column of density. The calls: `∞` is the compact logo
  mark in the header (`∞ loopy` — an East Asian Ambiguous glyph, the same
  width class as the `●`/`▶` already in every frame); count numbers are
  bold with dim `·` separators; rules and the rail separator are dim so
  chrome recedes behind content. Color discipline is one status accent per
  row: the rail colors only the glyph (cyan `▶` cursor, bold selected ID,
  dim budget — whole-row painting is gone), the overview timeline colors
  only the verdict cell (`RenderIterationRowParts` re-exposes the existing
  row as label/verdict/metrics so the TUI accents without re-deriving),
  and the baseline row stays fully dim. The detail header borrows the
  form's typography — dim labels (`goal`, `agent`, `iter`, `wall`), plain
  values — replacing two all-dim lines. Inverse video left the palette:
  the active tab is cyan-bold and the brackets remain the NO_COLOR signal.
  Quoted tails keep the CLI's ASCII `| ` gutter (now dim) rather than `│`,
  so the box-drawing bar means exactly one thing: the rail separator.
  Help keys and the broken-state `run: loopy doctor` line take cyan — keys
  and next commands are actions, and actions own the accent. `--once`
  stays ANSI-free and deterministic; its bytes changed only by the `∞`
  mark and the label/value detail lines.

## For the human

- ~~**License.**~~ Resolved 2026-06-11: MIT, per owner decision above.
