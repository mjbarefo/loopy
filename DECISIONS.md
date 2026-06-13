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

- **2026-06-11 — Bare `loopy` outside a git repo gets the front door, not
  the help wall.** On a terminal, the not-a-repo case used to print the
  full help text with the real problem buried in a trailing stderr line —
  the owner hit it on first launch from `~`. Now it prints the identity
  (logo, wordmark, tagline), the problem in one line (`~ is not a git
  repository — loops live inside one`), and the exact next move (`cd` into
  the repo, or `git init` first), then returns to the prompt. It stays CLI
  output rather than a full-screen frame: a dead-end TUI you can only quit
  is ceremony, and the user's next action is at their shell anyway. No
  interactive `git init` offer — initializing a repo in `~` by accidental
  keypress would be worse than the wall of text. Pipes keep the full help
  (unchanged contract); exit code stays 0, same as before. Rendering lives
  in `internal/tui` (`FrontDoor`) beside the splash that owns the
  branding; the home directory displays as `~`.

- **2026-06-11 — The logo is now a pixel lemniscate.** The original mark
  didn't resolve into anything at a glance; the owner asked for something
  circular/loop-inspired. The new art is two textured loops whose inner
  walls slope into an X crossing at the center — a literal infinity, drawn
  with the same two weights (`██` stroke, `░░` weave) and the same 5×18
  footprint, so every layout that centers or measures it is untouched.
  One source of truth in `welcome.go` (`logoArt`); README updated to
  match. The header's compact `∞` mark now abbreviates the actual logo.

- **2026-06-11 — Outside a repo, the front door becomes a repo picker.**
  When bare `loopy` finds git repositories nearby, it offers them instead
  of instructions: a chooser with the identity block, `↑↓`/enter into the
  chosen repo's monitor (onboarding takes over from there), `g` to
  git-init in place — `g` is a deliberate labeled keypress, never the
  default, so an accidental enter can't initialize `~`. Discovery
  (`loop.FindRepos`, stdlib-only) walks breadth-first, bounded in depth
  (4), directories (8000), results (100), and time (1.5s) — a front door
  must open instantly, so an incomplete list beats a complete hang; it
  skips hidden trees, `node_modules`/`vendor`, and macOS home furniture,
  and never descends past a repo root. Repos already holding loops sort
  first (then by git activity). The picker skips the splash afterwards —
  it was the branded moment — and exit hints from a picked repo are
  prefixed with `cd <repo> &&` so the printed command works from where
  the user actually is. With no repos found, the static front-door text
  remains. The monitor itself still never selects repos: the picker only
  runs when there is no repo at all.

- **2026-06-11 — The repo scan streams into the picker; no pre-scan, no
  static fallback.** The first cut scanned synchronously with a 1.5s
  cliff before deciding whether to show the picker — and on a real macOS
  home it showed the owner the dead-end text instead: a cold-cache walk
  can blow any fixed budget, and a TCC block on Documents/Desktop makes
  ReadDir fail silently. Now the picker opens instantly and the scan
  (`loop.ScanRepos`, emit-callback form; `FindRepos` remains the sync
  wrapper) streams candidates in behind it with an 8s budget. The cursor
  rests on the top-ranked repo until the user navigates, then sticks to
  their choice through re-sorts. When the scan ends empty the picker
  itself carries the old front-door guidance — `FrontDoor` is gone — and
  permission-denied near-top directories are reported by name with the
  System Settings → Privacy & Security path, because a silent TCC block
  is otherwise indistinguishable from having no repos.

- **2026-06-11 — Picker copy: "pick a project to run loops in", not "pick
  where loops should live".** The first phrasing read as a storage
  decision and prompted exactly that user question ("where should the
  loops live?"). The picker chooses a project, not a home for data; a dim
  annotation under the header ("loop state lives inside the repo it works
  on, under .loopy/") answers the storage question in place.

- **2026-06-11 — The new-loop form became a five-step wizard.** Owner
  feedback: composing a loop on the CLI (goal + repeated `--verify` flags
  + budget flags) assumes someone who already knows agentic development;
  the monitor should walk the user through it. `n` now steps through
  goal → agent → verifier → budget → confirm, one question per screen in
  plain words, defaults prefilled, enter advances, esc walks back. The
  agent step lists registered agents (space marks several — that races
  them, one loop per agent via the same detached-resume path, with
  `loopy judge <ids>` suggested when they all park; the monitor still
  never drives engines, so the CLI's blocking `RunRace`/auto-judge stays
  CLI-only and no race.json is written from the TUI). When no agents are
  registered the step offers the detected CLIs and registers the chosen
  one in place, so `n` now only requires an initialized repo. The
  verifier step is an editable command with provenance shown — an
  untouched stored/inferred prefill keeps its multi-stage form and the
  confirm-once storage contract; an edited command runs as a single
  stage for this loop only and is not stored (matching `--verify`
  semantics). Budget fields are validated text, not flags.

- **2026-06-11 — Default iteration budget: 8 → 5.** Owner asked to lean
  smaller (citing reported research that ~3 rounds is usually enough).
  Self-refinement returns do fall off steeply after the first feedback
  rounds, and stuck detection (no-change, same-failure-3x) parks
  degenerate loops before any cap — but this repo's own history argues
  against 3: the display-width loop did 4 productive iterations of real
  diagnostic work. 5 covers the observed maximum with margin while
  halving the worst-case tail; the cap is a ceiling on slow progress,
  not a target, since green ends the loop immediately. Per-loop override
  stays one field in the wizard and `--max-iters` in the CLI.

- **2026-06-11 — The calm pass: margins, a color diet, and a quiet nav.**
  Owner's verdict on the identity pass from real use: "nowhere as
  intuitive or as beautifully minimal as herdr." The fixes, in the
  splash's spirit (mostly emptiness, one accent doing real work): at
  ≥80×20 the frame gets a blank row inside each rule and a two-column
  gutter ahead of the rail (below that, the dense layout and the 40×8
  floor are byte-identical); status hues shrink to glyph size everywhere
  but the timeline's verdict cell (the evidence keeps its green/red) —
  activity lines, the title's status phrase, the truncation banner, the
  flash, and the abort confirm all carry one colored glyph and plain
  words; the bracketed tab bar becomes `▸ overview   live   diff
  verifier` with ▸ as the non-color signal; the rail separates live →
  needs-you → history with a blank row. The gutter stops at two columns
  and the separator gap stays a single space because widening it shaved
  the last byte-count column off the timeline at 100 wide — facts
  outrank air.

- **2026-06-11 — Footer diet: three hints, all-or-nothing.** The footer
  is `n new · enter open · ? keys` (detail focus: `esc back · ? keys`)
  plus the right-aligned next command; the header keeps `? help` and
  drops `q quit`; everything else lives behind `?` (help gained the page
  keys). When the next command needs the room, hints now yield whole —
  full chain, then bare `? keys`, then nothing — replacing the old
  word-by-word dropping; the next command is a fact and always stays.
  This is the pass's one sanctioned compression: hints, never facts.

- **2026-06-11 — The wizard is staged like the splash.** Headroom above
  the title, one accent per screen (the input cursor, the list cursor,
  or the confirm action line — the race marks went plain so the cursor
  keeps the accent), and one dim affordance line per screen ("enter
  continues · esc goes back") instead of a cyan action line plus a
  footer key chain; the footer goes blank while the wizard is open
  (validation flashes still land there). The confirm screen's cyan
  action line is its own affordance — esc said the same thing on all
  four screens before it, so it is not repeated there.

- **2026-06-11 — Rail groups get a gap, not a label.** Considered a tiny
  dim group label ("needs you") between the urgency groups; the blank
  row alone already reads as structure at every tested size, and the
  diagnosis named word noise as the disease — so the gap is the label.
  Revisit if a rail ever holds enough loops that the groups blur.

- **2026-06-11 — Baseline-green tells the truth.** A loop that goes
  green with zero iterations means the verifier passed before any agent
  ran; the monitor used to present it as a win. The activity line now
  reads "already green at baseline — nothing to do, or the verifier may
  not test the goal" behind a yellow `!` (caution, not celebration);
  `loopy review`'s diff-none line aligned to the same wording. The rail
  glyph stays the green ✓ — the status is factually green; only the
  framing changed.

- **2026-06-12 — "Reviewing stays human" is policy, not invariant.** The
  design fused two claims: loopy never ships (a tool invariant, permanent)
  and a human reviews every diff (a placement policy). Current
  loop-engineering practice — remove yourself as the bottleneck, stack
  loops — pressures the second claim, never the first; and the first is
  exactly what makes the second movable, because an outer loop can gate on
  `loopy run`'s exit code and read the evidence files, putting the human
  one rung higher. DESIGN.md and the README now separate the two and name
  the composition surface ("loopy as a rung"). No invariant changed.

- **2026-06-12 — `run`/`resume --json` stream NDJSON; the flag means "machine
  face", not "one document".** Everywhere else `--json` prints a single
  document, but a foreground engine's output is inherently a stream, so run's
  `--json` emits one event object per line and ends with a `result` event
  carrying the same LoopView that `status --json` serves — one shape for
  consumers. Race mode shares the stream (`loop_id` disambiguates) and ends
  with a `verdict` event. `--json` also implies non-interactive: an
  unconfirmed inferred verifier is refused, never prompted into the stream.
  Exit codes are untouched — the stream is observability, the exit code is
  the verdict. The schema lives in docs/orchestration.md.

- **2026-06-12 — The fleet view: browsing shows every loop breathing.** The
  herdr comparison named what the monitor lacked: with several loops only
  the selected one was alive on screen. Now browsing (rail focus) renders
  the fleet — one strip per loop in rail order: status words, a verdict run
  (`✗ ✗ ●`, one glyph per agent iteration), a verifier stage meter
  (`verify ▮▮▯ test`), and a 2-line live tail; enter opens the full detail,
  esc returns, tab/1–4 from the fleet land in the detail. herdr shows you
  terminals; loopy shows you convergence — that is the loop-shaped
  translation, and it was picked over tmux-style split panes and a card
  grid after prototyping all three (LOOPY_UI_PROTO, throwaway branch).
  The calls: fleet only at ≥2 loops (a count rule — a status rule would
  flip the view mid-watch; one loop keeps its richer detail), `--once`
  keeps its single-loop byte contract, decided loops compress to one quiet
  line, the strip gap doubles at urgency-group boundaries (the gap is
  still the label), the strips carry the ▶ cursor only when the rail is
  collapsed (one cursor at a time), and the per-loop tails are 8 KiB
  bounded reads (the fleet re-reads every live loop twice a second).

- **2026-06-12 — The reviewer agent ships pre-v0.1, exactly as designed:
  evidence, not a gate.** The design slotted it post-v0; the rung plan
  pulled it forward as the first review-moves-up-the-stack feature. The
  calls: `--reviewer <name>` must resolve to a registered agent *different*
  from the author (refused at creation — the creator shouldn't grade its
  own work); it runs once, when the loop first goes green with a non-empty
  diff (baseline green is skipped — nothing to review), under a `review`
  phase record; its stdout lands in `loops/<id>/critique.md` (256 KiB cap)
  beside `review-prompt.md`, and `loopy review` quotes it. No reviewer
  outcome — failure, timeout, missing agent — can stop the loop from
  parking green; the exit code is recorded on the loop. The reviewer
  reads, it must not ship: if the worktree changes during review it is
  force-restored to base + the verified diff (`RestoreWorktree`), so the
  parked diff is exactly the one the verifier approved. A resumed engine
  that lands on green again skips a reviewer that already wrote its
  critique.

- **2026-06-12 — The rail is the fleet; the strips are gone.** The owner's
  first real drive of the fleet view delivered the verdict: with a normal
  repo — mostly green/parked/decided history, rarely two live loops — the
  strips duplicated the rail item-for-item and the right pane stopped
  answering "what about the loop I selected." The strips earned their keep
  only in the synthetic many-live-loops demo. So: browsing shows the
  selected loop's detail again (the calm-pass view), and the multi-loop
  awareness lives where it always did — the rail's glyphs, urgency groups,
  and counts. Decided loops leave the rail too (selection skips them): the
  header still counts them, `loopy logbook`/`list` hold the history, and
  `loopy watch <id>` pins one back into view when asked. The reviewer
  phase's activity wording stays. Net of the fleet experiment: the bounded
  per-loop tail loader and the prototype lesson — herdr's "everything
  breathing" translates to loopy as a *denser rail*, not a second list.

- **2026-06-12 — `loopy delete <id>`, and `d` in the monitor.** The owner
  asked where loop deletion was; it didn't exist — reject freed the
  worktree but the evidence record lived forever, and test loops piled up.
  The calls: deletion removes the worktree, the branch, and the whole
  `loops/<id>/` directory, but **the logbook keeps one narrative line**
  ("deleted — goal, status") so the project still remembers that evidence
  was discarded (`logbook --json` loses the entry — it aggregates the
  deleted review.json — and that's the honest trade). A live engine is
  refused (abort first); unreadable loop state IS deletable — `delete` is
  the cleanup path when repair isn't worth it. In the monitor, `d` asks
  for y/n confirmation and then shells out to `loopy delete` — the resume
  precedent: the CLI is the actor, the monitor still writes no loop state
  itself. No `--force`, no flags: the confirmation lives in the monitor,
  and the CLI command is named like what it does.

- **2026-06-12 — Accept and reject from the monitor, on contextual keys.**
  The monitor was built with "judging stays in the CLI" (a code-comment-level
  stance, never argued here). Giving the monitor `d` broke that position:
  deleting evidence is strictly more consequential than recording a
  decision, and the monitor already renders the full review surface (diff,
  verifier, history). The owner asked why judging needed a shell round-trip;
  no good answer existed. The calls: **no new keys** — the verbs follow the
  loop's state. `a` is abort while a loop moves, accept once it parks green;
  `r` is resume for a paused loop, reject for a parked one (green or red).
  Every decision goes through the same y/n confirm footer as delete, which
  names the verb — a mispress is caught by reading. Both shell out to the
  audited CLI (`loopy accept` / `loopy reject`), the delete precedent.
  Accepting a *non-green* loop stays CLI-only: `--override --reason` is a
  deliberate, recorded act and deserves the deliberate path.

- **2026-06-12 — The quiet rail, and the apply command outlives the flash.**
  First in-monitor accept in anger: the accepted loop *stayed on screen*,
  because when every loop is decided the selection fell back to index 0 and
  the "selected loops always render" pin kept it there. It looked like the
  accept hadn't worked; the owner deleted the loop seconds later — taking
  `final-diff.patch` with it. Two calls: (1) when nothing needs eyes and no
  loop is pinned by ID, the selection becomes *none* and the rail shows a
  quiet state instead of re-pinning the last decision; (2) the quiet state
  and the accept flash both carry the accepted loop's `git apply` command —
  the human's road from accepted diff to branch and PR must outlive a
  three-second flash. loopy still never commits, pushes, or opens PRs
  (invariant 2); it prints the next move and stays out of the way.

- **2026-06-12 — "Agent blocked" is a park reason, not "stuck".** A gemini
  loop parked "stuck: no change to the diff" when the truth was the CLI
  refusing to run headless in an untrusted directory (exit 55, fix:
  `--skip-trust`). The distinction the engine now draws: a *nonzero agent
  exit with an untouched worktree* means the CLI never got to work (trust
  prompt, dead auth, missing binary) — park immediately as
  `agent blocked (exit N): <the agent's own last words>`, ANSI-stripped and
  bounded, verbatim discipline as ever. Exit 0 with no change stays "stuck"
  (the model tried and gave up); nonzero exit *with* changes is judged by
  the verifier alone (some CLIs exit nonzero after doing real work). First
  rung of "running a loop is natural": when the environment is the problem,
  the park reason is the fix.

- **2026-06-12 — Agent preflight: `loopy agent check`, not a side effect of
  registration.** The owner wanted trust/auth failures surfaced before they
  block a loop. The calls: a real smoke run (trivial prompt, throwaway
  directory, 2-minute cap) spends a model call, so it is an explicit command
  — registration stays instant and free, and `agent add` prints the
  `loopy agent check <name>` hint instead of silently spending money. What
  *is* free runs everywhere: `agent add` warns inline when the template's
  binary isn't on PATH, and `loopy doctor` warns per registered agent. The
  engine's "agent blocked" park stays the backstop for what only a real
  iteration can reveal. Also: the gemini suggestion in `KnownAgentCLIs`,
  help text, and docs/agents.md now carries `--skip-trust` — proven by a
  green loop today; without it every loop worktree is an untrusted
  directory and gemini refuses to work.

- **2026-06-12 — `--verify auto`: the agent proposes the verifier, the human
  still signs it.** The owner's direction: running a loop should lean on the
  frontier models, not on hand-written shell — the `make check` default is
  green-at-baseline for any goal the project gate doesn't test (file moves,
  docs), and the loop no-ops. The calls: synthesis is *the registered agent*
  run once in a **throwaway detached worktree** (invariant 4 holds — loopy
  makes no model calls of its own; and a --yolo agent can scribble there
  without touching the checkout). The proposal is the agent's final output
  line (fences/`$ `/backticks stripped), is **trial-run in the same
  worktree** — red is the desired verdict; an already-green proposal is
  flagged before any budget is spent — and is **confirmed on a TTY before
  use, never stored as the project default** (it is goal-specific; inference
  keeps owning the stored default). Fails closed without a TTY, same rule as
  inference. `--verify auto` stands alone rather than mixing with hand
  stages: the prompt tells the agent to chain the project gate itself.
  Wizard integration deferred — the form needs a spinner for a 1–3 minute
  agent call; the CLI path proves the shape first.

- **2026-06-12 — Synthesis in the wizard: tab proposes, enter signs.** The
  owner's bar, restated after the CLI rung shipped: *only run `loopy`* —
  the whole flow lives in the monitor, so a flag-only feature isn't done.
  The calls: on the wizard's verifier step, **tab** asks the selected agent
  to propose the verifier (an async tea.Cmd; the monitor keeps breathing
  and shows "asking <agent>… Ns"). The proposal lands **in the same
  editable field**, attributed ("proposed by claude — edit freely; enter is
  your sign-off"), already-green proposals flagged in the flash. Typing and
  enter are parked while the agent thinks; esc cancels in place, and a late
  result is dropped by sequence number. A synthesized verifier arrives
  marked edited, so it is never stored as the project default — same
  contract as `--verify auto`. Tab was free on that step (budget's
  field-switcher is a different step), and a proposal you must read sits in
  an input you can edit — no separate accept screen.

- **2026-06-12 — Diff and verifier tabs answer first, evidence below.** The
  owner's ask: the artifact tabs must read for a lay person, not just for
  someone who parses unified diffs. The calls: each tab opens with a header
  in plain words — the diff's "N files changed · +A -D" plus a per-file
  list, the verifier's per-stage ✓/✗ scoreboard plus a verdict sentence
  ("green: the goal is met" / "red: stage X failed — the log below shows
  why") — then the raw artifact with per-line styling (diff +/- in
  green/red, `=== stage` markers as dim dividers; on a red run the passing
  stages' output dims so the failure reads bright). Because the answer
  leads, **these two tabs open scrolled to the top**, not tail-following —
  live and overview keep the tail. A **truncated diff gets no stat header**:
  counting only the visible tail would lie, and the `!` banner already says
  the load is partial. The diff parser lives in `internal/loop/diffstat.go`
  (pure, stdlib), and `IterationView` now carries its per-stage results —
  the scoreboard renders from the view-model, never by parsing the log.

- **2026-06-13 — Mouse in the monitor: wheel scrolls, clicks navigate,
  decisions stay on the keyboard.** The owner wants herdr's feel: scroll
  with the wheel, point at things. The calls: cell-motion tracking (not
  all-motion — no hover effects, no event flood), with hit-testing as a
  **pure function over frameState** (`internal/tui/hittest.go`) whose
  arithmetic mirrors renderFrame's — the layout is deterministic, so "what
  is under (x, y)" is unit-testable like the frames themselves. Wheel over
  the rail moves the selection, over the detail body scrolls it; clicking a
  rail row selects, a nav name switches views, the body takes scroll focus.
  **Clicks never decide**: pending y/n confirms ignore the mouse entirely,
  and the wizard stays keyboard-driven. Mouse capture costs native text
  selection (Option-drag bypasses it), so the `c` key covers the real copy
  need — the next command — via OSC 52 (works in iTerm2/kitty/WezTerm;
  Terminal.app ignores it; the help line says so). On a quiet rail `c`
  copies the newest accepted loop's `git apply`, the exact command shown.
  Selection and tab switches now refresh only the artifact
  (`reloadArtifact`) instead of re-reading every loop — the 500ms tick owns
  disk truth; this also keeps the handlers unit-testable without a disk.
  The PTY smoke now injects real SGR mouse sequences and asserts the OSC 52
  write reaches the terminal.

- **2026-06-13 — Baseline green is named everywhere, and long text wraps.**
  The owner's first real drive hit the baseline-green trap head-on: a "build
  AGENTS.md" loop kept the inferred `make check` verifier, parked green at
  0 iterations, and read like a success ("1 green to review", green ✓ in
  the rail) while codex never ran. The calls: a baseline-green loop now
  carries the yellow `!` in the rail and title, leaves the "green to
  review" header bucket for its own "already green — check the verifier",
  the title says "green at baseline (the agent never ran)", and the
  verifier tab's verdict refuses to celebrate ("this verifier may not test
  the goal") — the verifier-truth status on disk is untouched; only the
  presentation stops conflating "verifier green" with "goal met". Second
  call: the detail header's goal (≤3 lines) and activity (≤2 lines) now
  word-wrap under hanging indents instead of truncating — `WrapDisplay` in
  internal/loop is display-width aware; the header's height is counted, not
  assumed, so the scroll math, the mouse hit-test, and `--once` sizing all
  derive it from one builder (`detailHeaderLines`). The welcome splash
  gains one dim line naming the mouse and `c`, because features nobody
  discovers don't exist.

- **2026-06-13 — The wizard designs the verifier up front, by default.**
  The owner's verdict after the baseline-green no-op: "the agent should
  design the verifier up front when the goal is designed," and he chose
  always-auto-synthesize over a baseline-guard or opt-in. So the wizard now
  kicks synthesis automatically at the goal→agent→verifier boundary (the
  earliest point an agent is known to do the proposing) instead of waiting
  for `tab`. The verifier step lands already-synthesizing; the proposal
  fills the editable field attributed to its agent; enter is still the
  human's signature (invariant 1 holds — no verifier, no loop, and the
  human signs). Inference stays prefilled as the **fallback** when the
  agent can't propose one. Re-entering the verifier step for the same goal
  does not re-ask (tracked by `synthGoal`); `tab` re-asks on demand. This
  accepts the 1–3 min synthesis pause on every wizard-created loop — the
  owner's explicit call, the cost of a verifier that actually tests the
  goal. Scope: the **wizard** only. `loopy run` keeps inference as its
  no-flag default with `--verify auto` opt-in — a scripted/non-interactive
  CLI run can't take a TTY-gated multi-minute pause (synthesis fails closed
  without a TTY), and the baseline-green CLI park already points at
  `--verify auto`. Builds on the `--verify auto` and wizard tab-synthesis
  decisions above; the machinery was already there but opt-in — this flips
  it to the default.

- **2026-06-13 — Verifiers become a spectrum: command · ask · hybrid.** A
  verifier stage's verdict can now come from a shell command (today's kind,
  the zero value) or from a registered agent answering a yes/no question
  about the worktree (`Kind: "ask"`, `Ask:` the question). A hybrid is just
  the ordered mix the wizard will reach for: cheap deterministic gates
  first, an ask stage last, so the expensive key-requiring stage only runs
  once the mechanical gates are green. **Why this is safe:** loopy never
  merges (invariant 2) — the human's accept/reject is the real seal, so a
  verifier may be fuzzy where shell can't reach; a wrong ask verdict costs
  at most a wasted iteration or a rejected diff, never a bad merge. **Why
  it retires the synthesis pause:** an ask stage needs no design (its
  question is the goal restated), so loop creation is instant and the
  agent's exploration moves inline to verify-time, free-riding the
  iteration the human was already waiting on — demoting #31's up-front
  synthesis from default gate to optional polish. Settled calls: the ask
  agent is the **loop's own agent** (it grades its own homework, accepted
  because the human seals); verdict protocol is a final `PASS` /
  `FAIL: <reason>` line, **fail-closed** on a timeout, an unrunnable agent,
  or a missing verdict; **judge-only verifiers are allowed** (no
  deterministic floor required — pure-prose goals); `AskTimeout` is 2 min.
  Naming: the kind is **`ask`**, not "judge" — "judge" is the deterministic
  no-API-key race-mode ranking (`judge.go`), the opposite of this stage on
  every axis. Invariant 4 holds: an ask stage shell-executes the registered
  agent exactly as synthesis does; loopy still makes no model call of its
  own, and inference/the demo stay command-only so the no-key path is
  untouched. Stuck detection: a varying ask reason won't trip
  `SameFailureRepeats`, so ask loops lean on `NoChangeRepeats` (diff
  unchanged) plus the hard budget. Full design in `docs/verifier-spectrum.md`.
  This slice is the engine (`Stage`/`StageResult` fields, `RunVerifier` ask
  branch, `runAskStage`); the wizard composing hybrids by default and the
  monitor's per-stage kind glyph are the follow-up.

- **2026-06-13 — The wizard composes a hybrid instantly; #31's synthesis is
  demoted to optional polish.** This is the payoff of the spectrum and the
  fix for the live-drive verdict (the default-on synthesis pause hit 3m+ and
  felt too slow). The verifier step now lands instantly with two fields:
  `checks` (command gates, prefilled from inference) and `ask` (the question,
  defaulting to the goal), switched with ↑↓. No agent call on arrival — the
  agent's judgment moves inline to verify-time, where the user already waits
  on the agent, instead of blocking loop creation. Either field may be
  cleared (command-only or ask-only, both valid); enter signs. `tab` keeps
  the #31 synthesis as an opt-in that designs tighter command gates and lands
  attributed in `checks`. **Storage:** only the command gates become the
  project default (confirm-once); the ask question is goal-specific and is
  never stored. **Monitor:** the verifier-tab scoreboard tags ask stages with
  the word `ask` and shows the question — a judged green must not look
  identical to a mechanically-proved one (color is never the only signal).
  The blocking auto-synthesis (`enterVerifierStep` → `startSynth`) is removed
  from the default path; `synthGoal`/re-ask-guard logic goes with it. Scope:
  wizard + monitor; `loopy run` CLI still inference-default + `--verify auto`.

## For the human

- ~~**License.**~~ Resolved 2026-06-11: MIT, per owner decision above.
