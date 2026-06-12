# Prompt for Fable: the calm pass — herdr's restraint, the splash's taste

You are the product engineer for **loopy**'s visual design. The identity
pass (PR #11, branch `mjbarefo/tui/identity`) gave the monitor the splash's
vocabulary; the owner's verdict from real use: "okay, but nowhere as
intuitive or as beautifully minimal as herdr — loses some of the taste of
the black-and-blue launch screen." This pass restores the taste.

## The reference points

1. **The splash** (`internal/tui/welcome.go`): mostly emptiness, one cyan
   accent doing real work, dim metadata. Its beauty is what it leaves out.
2. **herdr** (`/usr/local/bin/herdr`, capture it in a PTY): generous blank
   rows around content, two-space padding inside every region, plain prose
   sentences ("A workspace is one project context."), and exactly one
   affordance line at a time ("↵ continue"). Capture its screens before
   designing — the owner runs both tools side by side.

## Diagnosis from the owner's screenshots (80×24 and 134×27, real loops)

- **The color budget is blown.** Whole lines carry status color (the green
  activity line, the status phrase in the title, the cyan next-command, the
  cyan-bold tab) and they all compete. The splash never shows more than one
  accent at a time.
- **No margins.** Content butts against the rules and the left edge; every
  row is full. herdr floats content in whitespace.
- **Word noise.** Nine footer key hints in a `·` chain; a bracketed tab
  bar; label-heavy rows. herdr ships one hint and a `?`.
- **The rail is an undifferentiated table** — live work, things needing
  the human, and dead history all read identically.

## The plan

1. **Margins as structure.** A blank row under the header rule and above
   the footer rule; a two-column left gutter for rail and detail content.
   Collapse the margins below ~80×20 — the 40×8 floor and the density
   degradation ladder stay exactly as they are.
2. **Color diet: status hues only ever glyph-sized.** The activity line
   becomes a colored glyph + plain text (not a painted line); the title's
   status phrase goes plain (the glyph already says it); the timeline
   verdict cell keeps its green/red — it is the evidence and the one
   permitted block of status color. Cyan stays reserved for the single
   action: the next command, the active nav item, the cursor.
3. **Tab bar → quiet nav.** `▸ overview   live   diff   verifier` —
   active gets ▸ + cyan, inactive dim, brackets retired. ▸ is the
   non-color signal (NO_COLOR keeps it), so color is never the only
   signal.
4. **Rail hierarchy.** A blank row between urgency groups (live → needs
   you → history). Consider a tiny dim group label; the gap may be enough.
5. **Footer diet.** `n new · enter open · ? keys` plus the next command,
   right-aligned. Every other binding lives behind `?`. This is the one
   deliberate density trade in the pass: *hints* compress because `?`
   retains them; *facts* (counts, ids, budgets, timeline) do not move.
   Drop `· q quit` from the header for the same reason.
6. **The wizard gets splash-grade staging.** It is the launch-screen
   moment of the working monitor: pad it vertically, one accent per
   screen, herdr-style prose for the hints. Same five steps, same
   validation.
7. **Baseline-green deserves honesty.** The owner's first real loop went
   "green after 0 iterations" — the verifier passed at baseline, the agent
   never ran, the diff is empty, and the monitor presented it like a win.
   Render baseline-green distinctly: "already green at baseline — nothing
   to do, or the verifier may not test the goal" (dim/yellow, not
   celebration green). Check `loopy review`'s existing wording for the
   same case and align.

## Non-negotiable (unchanged from the identity pass)

- Facts keep their visibility at the same sizes; the only sanctioned
  compression is key hints behind `?` (item 5).
- `NO_COLOR`/`--no-color` clean; every signal keeps a glyph or word.
- `watch --once` deterministic, ANSI-free, next command in, hints out.
- All layout math through `loop.DisplayWidth`/`TruncateDisplay`/
  `PadDisplay`; geometry tests extended, never weakened.
- `scripts/tui-smoke.exp`: update expectations, never delete scenarios.
- Engine, store, CLI behavior out of scope except the baseline-green
  wording check (item 7).

## Process (the previous sessions' loop, proven twice)

- Build the playground (`/tmp/loopy-playground.sh`) and capture
  before-frames with `/tmp/ptycap.py` at 120×36, 100×26, 80×24, 60×20,
  44×12, ± NO_COLOR, plus splash/picker/wizard/help/abort. Capture herdr
  alongside as the reference.
- `make check`, `go test -race ./...`, `make tui-smoke`, `scripts/demo.sh`
  before the PR; before/after pairs in the PR body.
- DECISIONS.md entry per design call. Branch from `mjbarefo/tui/identity`
  if PR #11 is still open, else from main. Refresh the owner's brew
  install via the local tap mirror (dist.sh → formula sed to
  `http://127.0.0.1:8931/` → `brew reinstall`; the mirror serves `dist/`,
  pid in `/tmp/loopy-httpd.pid`).
- Take no public actions; the owner merges and releases.
