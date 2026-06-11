# loopy — iteration 1 of 6 in loop "make-fizzbuzz-pass-its-tests"

## Goal

make fizzbuzz pass its tests

## Verifier

The loop is green when all of these commands exit 0, run in order from the worktree root. They run automatically after you exit; you may also run them yourself:

1. verify: `sh ./test.sh`

## Feedback from the last verification

In the baseline check (before any agent ran), verifier stage `verify` failed. Output tail:

```text
fizzbuzz(3): want fizz, got fiz
```

## Rules

- You are one iteration of an autonomous loop. Make concrete progress toward the goal, then exit.
- Work only inside this directory (the loop's isolated git worktree).
- Do not commit, push, switch branches, or create branches. loopy snapshots your changes automatically.
- Do not edit anything under .loopy/.
- When the verifier passes, the loop ends and a human reviews the full diff.
