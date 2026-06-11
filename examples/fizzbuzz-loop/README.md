# Example: a complete `.loopy/` state tree

This is the untouched state left behind by `scripts/demo.sh` — one loop that
converged in three iterations and was accepted — with only the throwaway
repo's path rewritten to `/home/demo/project`. Everything loopy knows lives
in files like these; `cat` is a fully supported interface.

```
.loopy/
  agents.json                 registered agent command templates
  config.json                 project defaults
  logbook.md                  durable memory: every accept/reject, and why
  loops/make-fizzbuzz-pass-its-tests/
    loop.json                 the loop: goal, verifier, budget, status
    review.json               the audited human decision
    final-diff.patch          the durable result — apply with `git apply`
    iterations/
      0000/                   baseline: verify only, no agent — seeds feedback
        iteration.json        verdict, stage results, feedback tail hash
        verifier.log          per-stage output and exit codes
      0001..0003/             one turn of the crank each
        prompt.md             exactly what the agent was told
        agent.log             everything the agent printed
        verifier.log          what the verifier said about the result
        diff.patch            cumulative diff vs. the loop's base commit
        iteration.json        the iteration's record
```

Worth reading in order:

1. `loops/.../iterations/0001/prompt.md` — how the goal and the baseline's
   failing-stage output are composed into the agent's instructions.
2. `0001/verifier.log` vs `0002/prompt.md` — the failure tail of one
   iteration becoming the feedback of the next. This is the loop closing.
3. `loop.json` — budgets are hard caps; `iterations_used` and
   `wall_clock_used` are the meter.
4. `review.json` and `logbook.md` — the decision record. Had the loop been
   accepted while red, the override reason would be here, verbatim.

A live project also has `.loopy/worktrees/<loop-id>/` (the isolated git
worktree each loop iterates in — removed here, it's a full checkout) and,
while an engine runs, `control.json` (monitor→engine requests) and
`engine.lock`.
