# loopy — iteration 2 of 6 in loop "make-fizzbuzz-pass-its-tests"

## Goal

make fizzbuzz pass its tests

## Verifier

The loop is green when all of these commands exit 0, run in order from the worktree root. They run automatically after you exit; you may also run them yourself:

1. verify: `sh ./test.sh`

## Feedback from the last verification

In iteration 1, verifier stage `verify` failed. Output tail:

```text
fizzbuzz(5): want buzz, got buz
```

## Changes so far (cumulative, vs base commit 48542304f0a6)

- fizzbuzz.sh

## Rules

- You are one iteration of an autonomous loop. Make concrete progress toward the goal, then exit.
- Work only inside this directory (the loop's isolated git worktree).
- Do not commit, push, switch branches, or create branches. loopy snapshots your changes automatically.
- Do not edit anything under .loopy/.
- When the verifier passes, the loop ends and a human reviews the full diff.
