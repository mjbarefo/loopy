# The verifier spectrum: command · ask · hybrid

_Implemented: the engine slice (#32) and the wizard/monitor surface (this
branch). The wizard now composes a hybrid instantly and the up-front verifier
synthesis (#31) is demoted from default gate to the optional `tab` polish._

**Naming:** in code the agent-driven stage kind is **`ask`** (the loop *asks*
the agent whether it's done), not "judge". "Judge" is already the deterministic,
no-API-key ranking of competing green loops (`judge.go`) — the exact opposite of
this stage on every axis — so reusing it would mislead. This doc says "judge
stage" and "ask stage" interchangeably; the code says `KindAsk`.

## The reframe that makes this safe

loopy never merges, commits, or pushes (invariant 2). The verifier was never
the final arbiter — **the human's accept/reject is**. A green loop earns a
parked diff and its evidence; a person reads it and seals it
(`Accept`/`Reject`, `internal/loop/review.go`).

So the verifier has two jobs, neither of which requires it to be a perfect,
reproducible oracle:

1. **Drive iteration** — hand the agent a concrete reason it isn't done yet,
   which becomes the next prompt's feedback.
2. **Filter** — stop the loop from parking obvious garbage and burning the
   human's review attention.

A wrong verifier verdict therefore costs *at most* a wasted iteration or a diff
the human rejects — never a silently-merged bad change. That is the whole
license for putting a fuzzy, model-driven judge into the chain. The verifier
gets the loop *close enough to be worth a human's look*; the human seals.

## The spectrum

A verifier is still an ordered list of stages, run fast-to-slow, short-circuit
on the first failure (unchanged from today). What changes: a stage's verdict
can come from a shell command *or* from a registered agent. Each stage sits
somewhere on this spectrum by what produces its exit code:

| | **Command stage** (today) | **Judge stage** (new) |
|---|---|---|
| Verdict from | `sh -c cmd`; exit 0 = pass | a registered agent answers a yes/no question |
| Determinism | exact, reproducible | varies run to run |
| Cost | free, instant, **no API key** | one agent call + keys |
| Good at | anything mechanical: compiles, tests pass, file exists, lint clean, grep matches | quality & intent: "is the prose accurate?", "does this API read cleanly?", "is it actually idiomatic?" |
| Bad at | judging meaning | being cheap, fast, or reproducible |

**Hybrid** is not a third kind — it's the ordered mix the wizard reaches for by
default: cheap deterministic gates first, a judge last. Because stages
short-circuit, the judge (the expensive, key-requiring stage) only runs once
the mechanical gates are green. You never pay for an agent call on a build that
doesn't compile.

### Worked example — "add an AGENTS.md documenting the architecture"

```
1. exists   test -f AGENTS.md                          # command — instant, key-free
2. gate     make check                                  # command — instant, key-free
3. judge    "Does AGENTS.md accurately describe this    # judge — runs only when 1 & 2 are green
             repo's architecture and how to build it?"
```

Inference alone gives you stage 2 (`make check`) — which is baseline-green, so
the loop no-ops (the exact failure that drove #31). Stage 1 is the cheap
artifact check. Stage 3 is the part no shell command can express, and it's
where the loop's quality actually lives.

## Why this retires the synthesis pause

The 3-minute wizard pause existed for one reason: to compress a fuzzy goal into
a *static* shell command up front, which required the agent to explore the repo
before the loop could even start.

A judge stage needs no such design. **Its question is just the goal restated**
— "verify this goal is met: `<goal>`" — composed instantly, no agent call at
creation time. The agent's exploration still happens, but *at verify time,
inside the loop*, where you are already waiting on agent calls anyway. The
fuzzy judgment free-rides on iteration time instead of blocking loop creation.

Net for the wizard:

- **Loop creation is instant.** Hybrid = inferred command gates (instant) + a
  judge stage derived from the goal (instant). No synthesis call.
- The agent's repo exploration moves from a blocking pre-step to the first
  verify, overlapping the work you'd wait for regardless.
- #31's synthesis becomes an *optional polish* ("let the agent tighten these
  gates"), never the default blocking path.

Implemented in the wizard: the verifier step has two fields — `checks`
(inferred command gates, editable) and `ask` (the question, defaulting to the
goal) — switched with ↑↓; either may be cleared (command-only or ask-only);
`enter` signs. `tab` is the optional synthesis. Only the command gates are
stored as the project default; the goal-specific ask question never is. The
monitor's verifier-tab scoreboard tags ask stages with the word `ask` (not
just color) so judged greens read distinctly from mechanical ones.

## Background gate synthesis (the agent designs the gate, for free)

The synthesis pause was retired, but the *value* it produced — an
agent-designed deterministic gate, tighter than anything inference can guess —
was worth keeping. We get it back without the pause by moving it off the
critical path entirely.

When a loop starts **ask-only** (`Loop.AutoGate`, set by the wizard), the engine
kicks `SynthesizeVerifier` in a **background goroutine** at loop start. The loop
iterates immediately under its instant ask verifier; meanwhile the agent
explores a throwaway worktree and proposes a fast shell command that is red
until the goal is met. When the proposal lands, the engine **folds it in
additively at an iteration boundary**, *ahead* of the ask stage:

```
before:  [ ask: "is the goal met?" ]
after:   [ gate: <agent-designed cmd> ,  ask: "is the goal met?" ]
```

The gate runs first, so the cheap deterministic check short-circuits before the
agent is asked again — the same fast→slow ordering as a hand-written hybrid,
arrived at automatically.

Design rules:

- **The engine stays the single writer of loop state.** The goroutine only
  *computes* a proposed stage and hands it back on a channel; the engine drains
  it non-blockingly and does the `SaveLoop` itself, at a boundary — never
  mid-iteration.
- **No re-sign-off.** The loop already runs under a human-approved ask
  verifier. An additive gate only makes "green" *stricter* — it can never
  auto-accept — and the human still seals the diff at review (invariant 2). So
  folding it in owes no new consent, and invariant 1 (no verifier, no loop) is
  already satisfied. The human's `enter` at creation still bought a real
  verifier; the gate just sharpens it.
- **Ask-only only.** A loop that already carries a command stage made a
  deliberate choice loopy must not override. This also makes the fold-in
  idempotent across engine resumes: once the gate is in, the verifier is no
  longer ask-only, so nothing re-synthesizes.
- **Failure, timeout, or already-green = silent no-op.** A proposal that passes
  at HEAD is no gate (it does not test the goal) and is dropped; a failed or
  cancelled synthesis just leaves the ask-only verifier in place. The delivery
  channel is never closed on a drop, so the engine's non-blocking receive can
  never fire on a zero-value gate.
- **Cancellation.** `SynthesizeVerifier` now takes a context; the engine
  cancels it when the loop parks, tearing the throwaway worktree down if
  synthesis was still running (the common fast-green case).
- **Invariant 4 holds.** Synthesis shell-executes the registered agent exactly
  as before — loopy makes no model call of its own — so the no-API-key demo and
  inference stay command-only and never trigger it.

`tab` in the wizard remains the *interactive* synthesis (design the gate up
front, see it before signing); background synthesis is the same engine reused
on the loop's own time when the user chose the instant ask path instead.

## Data model

Extend `Stage` (`internal/loop/models.go`); existing `loop.json` files keep
parsing because every new field is omit-empty and `Kind` defaults to command.

```go
type StageKind string // "" == "command"

const (
    KindCommand StageKind = "command"
    KindAsk     StageKind = "ask"
)

type Stage struct {
    Name  string    `json:"name"`
    Kind  StageKind `json:"kind,omitempty"`  // "" → command (back-compat)
    Cmd   string    `json:"cmd,omitempty"`   // command stages
    Ask   string    `json:"ask,omitempty"`   // ask stages: the yes/no question
    Agent string    `json:"agent,omitempty"` // ask stages: which agent (default: the loop's)
}
```

Implemented: `Stage` gains these fields (omit-empty, back-compat); `StageResult`
gains a matching `Kind`; `RunVerifier` takes an `*AskContext{Root, Goal, Agent,
Diff}` and branches on `stage.kind()`; `runAskStage`/`askPrompt`/`parseVerdict`
live in `internal/loop/verifier.go`; `AskTimeout = 2*time.Minute`. The engine
threads the diff and the loop's agent through `verifyToLog`. Tests in
`internal/loop/ask_test.go` script the agent as inline shell.

## How a judge stage runs

`RunVerifier` (`internal/loop/verifier.go`) branches on `stage.Kind`. Command
stages are unchanged. A judge stage:

1. **Composes a judge prompt** from the goal, the stage's `Ask`, and the
   evidence: the iteration's **diff** plus read access to the worktree. It runs
   in the same worktree `dir` as command stages, read-only by convention (the
   prompt forbids edits; the diff is taken from the loop's snapshot regardless).
2. **Runs the registered agent** — the same external-command mechanism as the
   loop's iterations and as `SynthesizeVerifier` today, so **invariant 4 holds:
   loopy still makes zero model calls of its own.**
3. **Parses a verdict from the final output line** (reusing the `synth.go`
   tail-parse pattern). Protocol: the agent's last line is `PASS` or
   `FAIL: <reason>`. `PASS` → exit 0; `FAIL` → exit 1; **no parse → FAIL**
   (fail closed, exactly as synthesis treats "no usable command").
4. **Bounds itself** with a timeout — propose 2 min (read-and-verdict, not
   explore-and-design; tighter than synthesis's 5 min).

The judge's `FAIL: <reason>` becomes the stage output, so the existing
machinery carries it for free: it lands in `verifier.log`, becomes the
`FeedbackTail`, and drives the next prompt. **This is a strength** — natural-
language "you're not done because X" is richer iteration feedback than a stack
trace. The full reasoning is logged as evidence; only the verdict gates.

## Subtlety: stuck detection

Stuck detection keys on `FailingStage + hash(FeedbackTail)`
(`VerifierOutcome.TailHash`). A judge's reason is natural language and will
vary iteration to iteration even when the loop is genuinely stuck, so
`SameFailureRepeats` will rarely trip on a judge stage.

The backstop that still works: **`NoChangeRepeats`** (park when N consecutive
iterations leave the diff unchanged) fires regardless of reason text — if the
agent stops changing anything but the judge keeps failing, that catches it.
And the hard budget (`MaxIterations`, invariant 3) caps a flapping judge no
matter what. So judge loops are bounded; they just lean on diff-churn + budget
rather than failure-text identity. Note this in the stuck-policy docs.

## Zero-key demo & preflight

- **Inference and the demo stay command-only.** `scripts/demo.sh` (shell agent,
  no keys) and `internal/loop/infer.go` produce command stages exclusively, so
  the no-key path is unchanged (invariant 4).
- A judge stage requires the loop's agent to be runnable — the same surface as
  any iteration. A judge stage whose agent can't run is an agent-env failure,
  not a "stuck" park: surface the agent's stderr as the reason and the fix
  (ties into the agent-preflight direction).

## Monitor surface

- The verifier tab scoreboard (`IterationView.Stages`, #28) gains a per-stage
  **kind glyph** so the human sees which greens are mechanical vs judged —
  e.g. a gate glyph for command stages, a balance/scale glyph for judge stages.
  Color is never the only signal (NO_COLOR): the word carries it too —
  `judged: pass — <reason>` / `judged: fail — <reason>`.
- The judge's reasoning is **evidence feeding the human's decision, not
  replacing it.** The diff tab + verifier tab + accept/reject still lead. This
  closes the loop on the reframe: the judge gets you to a reviewable diff; the
  human seals.

## Invariant check

1. **No verifier, no loop** — a hybrid has ≥1 stage; a judge stage counts. A
   judge-only verifier is allowed (see settled decisions); the wizard prefers
   an inferred command floor but does not require one.
2. **Never merges / commits / pushes** — unchanged, and it's precisely what
   makes a fuzzy judge safe. ✓
3. **Budgets are hard caps** — judge stages spend iterations/wall-clock like
   anything; the cap holds. ✓
4. **No model calls of its own; zero-key demo** — a judge shells out to the
   registered agent, exactly as synthesis does; demo/inference stay
   command-only. ✓
5. **Layer boundaries** — judge execution lives in `internal/loop` (stdlib +
   `os/exec`, same as `synth.go`); no TUI import, no new deps. ✓
6. **Everything on disk is plain JSON/markdown/patches** — `Stage` gains
   omit-empty JSON fields; the verdict and reasoning land in `verifier.log`. ✓

## Settled decisions (owner, 2026-06-13)

- **Judge agent = the loop's own agent.** No separate judge agent; the `Stage.Agent`
  field defaults to (and for now always resolves to) the loop's agent. It grades
  its own homework by design — accepted because the human's accept/reject is the
  backstop. One agent to register, always available. (The `Agent` field stays in
  the schema for a future opt-in to a different judge, but the wizard never sets it.)
- **Verdict protocol = `PASS` / `FAIL: <reason>` final line.** Mirrors the
  `synth.go` tail-parse; no-parse fails closed. No JSON.
- **Judge-only verifiers are allowed.** A verifier may be a single judge stage
  with no deterministic floor (pure-prose goals where nothing mechanical
  applies). The wizard still *prefers* to include an inferred command gate when
  one exists — it just isn't mandatory.
- **Judge timeout = 2 min.**

## Decision to record

If adopted: the verifier is a spectrum (command · judge · hybrid); the wizard's
default verifier becomes a hybrid composed instantly from inference + a
goal-derived judge; up-front synthesis (#31) is demoted from default gate to
optional polish. Justification: the human's accept/reject is the real seal, so
the verifier may be fuzzy where shell can't reach, and the fuzzy part belongs
inline (free-riding iteration time) rather than as a blocking pre-step. → add a
dated DECISIONS.md entry.
