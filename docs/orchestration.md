# Driving loopy from an outer loop

loopy is built to be a rung in a taller stack (see `DESIGN.md`, "loopy as a
rung"): a script — or another agent — can design loops, run them, and decide
what to do with the results, with a human moved one level up. This page is
the contract that makes that safe.

## The contract in one paragraph

`loopy run` **exits 0 only when the loop parks green**; everything else
(budget exhausted, stuck, aborted, runtime failure) exits 1, and usage errors
exit 2. With `--json`, progress streams as NDJSON events on stdout. Every
artifact a decision needs is a plain file under `.loopy/` — readable without
loopy. `accept` and `reject` are non-interactive and audited. loopy itself
never merges, commits to your branches, or pushes: what to do with a green
diff is the *caller's* policy, and the caller is accountable for it.

## The event stream: `loopy run --json`

```bash
loopy run "make the importer handle quoted newlines" \
  --verify "go test ./importer/..." --max-iters 5 --json
```

One JSON object per line. Every event has `event`, `ts` (RFC3339, UTC), and
`loop_id`; the rest of the fields depend on the event:

| event               | extra fields                                                  |
| ------------------- | ------------------------------------------------------------- |
| `loop_started`      | `agent`, `branch`, `worktree`, `max_iterations`, `max_wall_clock` |
| `baseline_started`  | — (the agent-free verify recorded as iteration 0)             |
| `iteration_started` | `iteration`, `max_iterations`                                 |
| `agent_done`        | `iteration`, `exit_code`, `duration_ms`                       |
| `stage_done`        | `iteration`, `stage` {`name`, `cmd`, `exit_code`, `duration_ms`} |
| `iteration_done`    | `iteration`, `green`, `failing_stage`, `violation`, `diff_bytes` |
| `note`              | `note` (resume notices, reviewer notices, stuck warnings)     |
| `reviewer_done`     | `exit_code`, `duration_ms` (only with `--reviewer`)           |
| `loop_ended`        | `status`, `parked_reason`                                     |
| `result`            | `status`, `result` — the full loop view, the same object `loopy status --json` returns |

The `result` event is always last, for green and red parks alike (a hard
runtime failure may end the stream early; stderr carries the error).
`loopy resume <id> --json` streams the same events; `--race a,b --json` interleaves every loop's events on one
stream (`loop_id` tells them apart) and ends with a `verdict` event
(`race_id`, `verdict` — the judge's ranking, winner empty when nothing was
safe to take).

`--json` implies non-interactive: an inferred-but-unconfirmed verifier is
refused instead of prompted for. Pass `--verify` or store a project default
first.

## A minimal outer loop

```bash
goal="$1"
loopy run "$goal" --max-iters 5 --json > events.ndjson
verdict=$?
id=$(tail -1 events.ndjson | sed -n 's/.*"loop_id":"\([^"]*\)".*/\1/p')
if [ "$verdict" -eq 0 ]; then
  # Green: review mechanically or escalate to a person. The diff and the
  # whole evidence trail are plain files:
  ls .loopy/loops/$id/iterations/        # prompt.md, agent.log, verifier.log, diff.patch per iteration
  loopy review "$id"                     # the critique from --reviewer <agent> shows here too
  loopy accept "$id"                     # audited; writes review.json + durable final-diff.patch
  git apply ".loopy/loops/$id/final-diff.patch"
else
  # Red park: the evidence says why (budget, stuck, which stage failed).
  loopy status "$id" --json              # or escalate to the human
fi
```

(In production parse the NDJSON with `jq -c`; the sed line keeps the example
dependency-free.)

## Going down a loop

When the stack misbehaves, descend with the evidence: each
`.loopy/loops/<id>/iterations/NNNN/` holds the exact `prompt.md` the agent
saw, its full `agent.log`, the per-stage `verifier.log`, and the cumulative
`diff.patch`; `loop.json` records the budget accounting and, for red parks,
which stuck rule fired. `review.json` records every accept/reject with any
override reason verbatim. Nothing requires loopy to read.

## Rules that protect the stack

- Budgets are hard caps — an outer loop cannot talk a loop past them.
- Accepting a non-green loop requires `--override --reason <text>`, recorded
  verbatim. Give your orchestrator that flag only if you mean it.
- Registered agents run with your shell and your permissions; an outer loop
  that registers agents is part of your trusted computing base. Review
  `.loopy/agents.json` in shared repos.
