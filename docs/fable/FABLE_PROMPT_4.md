# Prompt for Fable: make the monitor wear the splash's identity

You are the product engineer for **loopy**'s visual design. The launch
splash landed and the owner loves it; the working monitor now looks plain
by comparison. Bring the watch window up to the splash's level — one
coherent visual identity from the first frame to the last — without
sacrificing a single column of information density.

## Start from reality

Work in:

```text
/Users/jacobbarefoot/Documents/GitHub/loopy
```

PR #10 merged; `origin/main` is `7d920fc`. Begin with:

```bash
git fetch origin --prune
git status -sb
git switch -c mjbarefo/tui/identity origin/main
```

Read before editing:

1. `CLAUDE.md` — binding invariants. `internal/loop` is stdlib-only; only
   `internal/tui` imports Bubble Tea; the engine is the single writer of
   loop state; color is never the only signal.
2. `DECISIONS.md` — especially the 2026-06-11 entries: the monitor
   redesign rationale, the bare-launch decision, and the display-width
   work. Add an entry for every call you make.
3. `internal/tui/` — `welcome.go` (the splash: `logoArt`, centered
   layout, cyan/bold/dim palette), `frame.go` (the working monitor),
   `form.go`, `model.go`, `frame_test.go`, `model_test.go`.
4. `scripts/tui-smoke.exp` — the PTY assertions you must keep green.

## The ask

The splash established the identity: the pixel-infinity logo, the spaced
wordmark, a calm cyan/dim two-tone, generous centering, one accent color
doing real work. The monitor (`loopy watch` and post-splash bare `loopy`)
should feel like the same product designed by the same hand:

- A header worth looking at. Today it is ` loopy   1 running · …` in
  plain bold. Consider a compact logo mark or glyph accent, tighter
  typography for the counts, and the same cyan discipline as the splash.
- Consistent visual rhythm: rules, spacing, section labels, tab bar, and
  footer should share one vocabulary with the splash and the form
  (`form.go` is closest to the target feel — `start a loop` header,
  dim labels, cyan action line).
- The rail and the overview carry the same hierarchy: status accents in
  exactly one place per row, dim for metadata, never two competing
  brights on one line.
- The empty/onboarding state, help overlay, and abort/flash footer states
  deserve the same pass.

You have full authority over layout details, glyphs, spacing, and color
codes. Do not ask the owner to choose colors or spacing. Surprise with
taste; keep restraint — this is an engineering instrument, and density
beats ornament everywhere except the splash and the empty state.

## Non-negotiable

- Information density does not regress: every fact currently in the
  header, rail, detail header, overview, and footer stays visible at the
  same terminal sizes.
- `NO_COLOR` and `--no-color` keep working; every signal keeps its glyph
  or word. The SGR palette is hand-rolled in `frame.go` — no styling
  dependencies.
- `watch --once` stays deterministic and ANSI-free; key hints stay out of
  it; the next command stays in it.
- All layout math goes through `loop.DisplayWidth`/`TruncateDisplay`/
  `PadDisplay` (CJK/emoji-safe). Frame geometry tests assert display
  columns — keep them passing and extend them for anything new.
- Narrow terminals (<80 cols) degrade deliberately; the 40×8 minimum and
  the too-small message stay.
- `scripts/tui-smoke.exp` keeps passing — update its expectations with
  the layout, never delete scenarios. Engine, store, and CLI behavior are
  out of scope: this is a rendering-layer pass (`internal/tui` plus, if
  truly needed, the shared renderers in `internal/loop/render.go`).

## Build a visual QA loop before changing pixels

Capture before-frames first, exactly like the last session did:

```bash
python3 -m pip install --user pyte   # tiny terminal emulator for captures
CGO_ENABLED=0 go build -o /tmp/loopy-bin ./cmd/loopy
```

Write a small PTY capture helper (spawn the binary in a pty at given
rows/cols, feed keys, render the final screen with pyte — ~80 lines of
python; pyte does not implement REP, so mid-redraw artifacts in captures
are pyte's fault, not yours — confirm anything suspicious in a real
terminal or via expect). Build a throwaway playground repo with loops in
every state: live (slow scripted agent), paused, stale running (kill -9
the engine), green, parked (budget and stuck), accepted, rejected, a long
CJK/emoji goal, and a corrupt loop.json. The previous session's recipe:
scripted shell agents (`sleep 6` + sed fixes) against a three-bug
fizzbuzz, `--max-iters` to force each terminal state.

Capture every state at 120×36, 100×26, 80×24, 60×20, 44×12, with and
without NO_COLOR, plus the splash, onboarding, form, help overlay, and
abort confirm. Repeat after each design iteration; put the best
before/after pairs in the PR.

## Verification standard

```bash
git diff --check
test -z "$(gofmt -l cmd internal)"
go vet ./...
go test ./... -count=1
go test -race ./...
CGO_ENABLED=0 go build ./cmd/loopy
make tui-smoke
scripts/demo.sh        # its embedded --once frame must still read well
```

## Context that will save you time

- The owner tests via a brew install from a local tap with a localhost
  mirror (see the previous session: `scripts/dist.sh v0.1.0`, generate the
  formula into `$(brew --repository)/Library/Taps/mjbarefo/homebrew-tap`,
  sed the URLs to `http://127.0.0.1:<port>/`, `brew reinstall`). Refresh
  their install when you finish so they can feel it.
- **Do not touch release machinery and take no public actions.** The
  v0.1.0 publication is owner-gated and pending: repo public → tap push →
  TAP_GITHUB_TOKEN → rc tag → stable tag (`docs/releasing.md`). Your work
  rides into the release when the owner cuts it.
- Branch + PR to main when done; the owner merges. Small commits, terse
  lowercase imperative messages.

## Definition of done

- Splash, onboarding, form, and working monitor read as one designed
  product; the before/after captures make the difference obvious.
- Every gate above is green; smoke scenarios extended where the layout
  changed; DECISIONS.md records the design calls.
- The owner's brew-installed binary is refreshed and a PR is open with
  the captures and a short design rationale.
