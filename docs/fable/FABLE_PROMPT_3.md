# Prompt for Fable: take loopy from project to production

You are the founding product engineer taking **loopy** from a strong,
feature-complete project to a production-quality v0.1 release.

This is not a checklist-only cleanup pass. Own the end-user experience, make
the hard calls the repository leaves open, and leave behind a product that
feels intentional from first install through first successful loop. I want
you to exercise taste, especially in the TUI. Surprise me with a better
experience than the one I would have specified.

## Start from reality

Work in:

```text
/Users/jacobbarefoot/Documents/GitHub/loopy
```

At handoff time, PR #9 had merged and `origin/main` was `676157a`. The local
checkout was still on the now-deleted `codex/deslop-repo-cleanup` branch at
its merged parent commit. Do not build on that stale branch.

Begin with:

```bash
git fetch origin --prune
git status -sb
git log --oneline --decorate -12 origin/main
git switch -c mjbarefo/production/v0.1.0 origin/main
```

If the worktree is no longer clean or `origin/main` has moved, investigate
and adapt. Never discard changes you did not make.

Read these before editing:

1. `CLAUDE.md` - binding invariants, architecture, and conventions.
2. `DECISIONS.md` - prior product and engineering calls.
3. `README.md` and `QUICKSTART.md` - the current user promise.
4. `DESIGN.md` - intended product shape and remaining open questions.
5. `internal/tui/`, `.github/workflows/`, `scripts/dist.sh`, and
   `scripts/homebrew-formula.sh` - the current implementation.

Add a concise `DECISIONS.md` entry whenever you make a meaningful call that
the design did not already settle.

## The current baseline

The core product exists: loop engine, worktree isolation, evidence capture,
budgets, stuck detection, monitor, review/accept/reject, logbook, race mode,
judge, demo, CI, cross-platform archives, and a release workflow.

The following gates were green on June 11, 2026:

```bash
test -z "$(gofmt -l cmd internal)"
go vet ./...
go test ./... -count=1
go test -race ./...
CGO_ENABLED=0 go build ./cmd/loopy
make tui-smoke
```

`scripts/dist.sh v0.1.0-rc.1` also produced all six archives and a valid
`SHA256SUMS`. This does **not** mean the project is production-ready:

- No release has ever been tagged.
- There is no `LICENSE`; do not choose legal terms on the owner's behalf.
- The generated formula is only attached to a loopy release. There is no
  public tap and therefore no real `brew install mjbarefo/tap/loopy` path.
- The formula currently uses `license :cannot_represent`.
- `brew style dist/loopy.rb` currently reports missing Sorbet and frozen
  string literal headers.
- A formula must be audited by name from a real tap; auditing an arbitrary
  path is not the production test.
- The headless agent command matrix is still suggestion-level, not proven.
- The existing TUI is capable and well-tested, but it still reads as a
  milestone implementation. It needs a product designer's pass.

Treat every statement above as a lead to verify, not permanent truth.

## Your mandate

Deliver three things as one coherent production push:

1. A TUI that feels designed for people running real agent loops.
2. A hardened CLI/repository with an honest v0.1 support contract.
3. A complete, repeatable GitHub Release and Homebrew tap path.

You have broad authority over interaction design, layout, visual hierarchy,
wording, release mechanics, workflow structure, and implementation details.
Preserve the six invariants in `CLAUDE.md`. Challenge other prior choices
when the production evidence says they are weak; document why.

Do not ask me to choose colors, spacing, pane layouts, key bindings, naming,
or ordinary implementation details. Use judgment. Bring me in only for:

- the repository license, with a concrete recommendation and tradeoffs;
- credentials or repository permissions you cannot obtain;
- the final irreversible act of creating public repositories, pushing a
  release tag, or publishing a release.

Everything leading up to those acts is yours to implement and verify.

## Workstream 1: make the TUI the product's face

Start by using the product, not by reading renderer code forever. Create real
throwaway loops in representative states and drive `loopy watch` in a PTY:

- no loops yet;
- one live loop in agent and verifier phases;
- a loop visibly converging over several iterations;
- a stuck or budget-exhausted loop;
- green, accepted, and rejected loops;
- a stale "running" loop with no engine;
- several concurrent/race loops;
- long goals, long IDs, Unicode, noisy logs, and narrow terminals;
- `NO_COLOR`, redirected output, and `watch --once`.

The monitor should let a new user answer these questions in two seconds:

- What is running?
- Is it getting better?
- What is it doing right now?
- Why did it stop?
- What can I safely do next?

You may substantially redesign the monitor. Do not preserve the current
two-pane box merely because it exists. Explore stronger information
hierarchy, clearer progress and convergence signals, better empty/error
states, more legible artifacts, discoverable controls, and a more satisfying
relationship between live activity and iteration history.

Use restraint. This is an engineering instrument, not a dashboard full of
ornament. Personality is welcome when it improves orientation or delight.
The pixel infinity identity may appear in onboarding or an idle/empty state,
but never at the expense of information density.

Non-negotiable behavior:

- Color is never the only signal and `NO_COLOR` works.
- Narrow terminals degrade deliberately, not accidentally.
- `watch --once` remains deterministic and ANSI-free.
- The engine remains the single writer of loop state.
- Accept/reject remain audited CLI actions unless you can prove a safer,
  equally auditable interaction and log the decision.
- Tail loading remains bounded; truncation is explicit.
- Keyboard help and the next safe action are discoverable.
- Accessibility and terminal compatibility beat visual cleverness.

Build a visual QA loop, not just string assertions. Keep deterministic frame
tests, add representative golden/snapshot fixtures if they improve review,
and exercise the real binary in PTYs at multiple dimensions. Before/after
captures in the PR are useful. Test the experience manually as well.

## Workstream 2: productionize the whole product

Audit the entire first-run-to-review journey. Fix rough edges you find rather
than limiting yourself to files named in this prompt.

At minimum, evaluate and harden:

- root help, subcommand help, errors, exit codes, and actionable recovery;
- `init`, verifier inference, agent registration, and the first-run path;
- corrupt/partial state handling and `doctor` guidance;
- interruption, crash, pause/resume/abort, and stale-engine behavior;
- version reporting in source builds, release builds, and Homebrew installs;
- shell completion and man-page support if they materially improve v0.1;
- macOS, Linux, and Windows claims versus what CI actually proves;
- security boundaries around shell commands, worktrees, paths, logs, and
  untrusted verifier output;
- dependency and supply-chain posture;
- documentation consistency and a genuinely short path to first value.

Keep scope disciplined: production quality for the product that exists, not a
pile of post-v0 features. Scheduled loops, notifications, reviewer agents,
and cost accounting remain out unless a small piece is essential to the
release contract.

Create or improve the production documents the repository actually needs.
Likely candidates include a release runbook, changelog/release-note process,
support/security policy, and a clear compatibility statement. Avoid
ceremonial files that nobody will maintain.

The license is a blocking owner decision. Inspect the lineage and intended
distribution, recommend a specific SPDX license, and ask once. After approval,
add the license and make every package manifest/formula agree.

## Workstream 3: make release and Homebrew real

The release is not complete until a new user can install from the public tap
in one command on a clean machine:

```bash
brew install mjbarefo/tap/loopy
loopy version
loopy help
```

Research current official guidance rather than relying on old examples:

- https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap
- https://docs.brew.sh/Formula-Cookbook
- https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository
- https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations

Homebrew now recommends direct installation as
`brew install user/repository/formula`; use that form in the docs. It avoids
depending on short-name tap trust behavior that is changing across Homebrew
5.2/6.0.

Own the packaging design. Decide whether loopy should be a source-built
formula, an upstream-binary formula, a cask, or another well-supported tap
pattern. Do not assume the current multi-URL formula is correct simply because
it works syntactically. Optimize for reliable installs, transparent supply
chain, fast upgrades, and maintainability.

Implement the full path:

1. Harden tag validation and release gates. Release only exact SemVer tags
   from the intended commit on `main`; make reruns and partial failures safe.
2. Produce reproducible CGO-free artifacts with embedded version metadata,
   checksums, sensible archive contents, and verified executable permissions.
3. Evaluate GitHub artifact attestations and an SBOM. Add them when they
   provide real user-verifiable provenance without fragile ceremony.
4. Make release notes useful. An RC must explain how to install, verify, test,
   and report problems.
5. Create the `mjbarefo/homebrew-tap` repository structure locally using
   Homebrew's supported tooling, with `Formula/loopy.rb`, README, and CI.
6. Automate stable formula updates from a successful loopy release. Use a
   secure cross-repository mechanism and document the required token/App,
   permissions, rotation, and failure recovery.
7. Decide and document the RC policy. A stable tap must not accidentally
   upgrade users to a prerelease. If RC installation is supported, make it
   explicit and isolated.
8. Test the formula as Homebrew tests it: style, audit, install, test,
   uninstall, reinstall/upgrade, and version output. Test Intel and Apple
   Silicon macOS plus Linux where available.
9. Update README/QUICKSTART so Homebrew is the primary macOS/Linux install
   path after publication, with source install as the fallback.
10. Write an exact release runbook from clean `main` through RC soak, stable
    tag, GitHub release verification, tap update, clean install, and rollback.

Prefer automation that is easy to understand over opaque release machinery.
Pin third-party GitHub Actions appropriately and grant the minimum token
permissions each job needs.

## Verification standard

Expand this list when your changes create new risk:

```bash
git diff --check
test -z "$(gofmt -l cmd internal)"
go vet ./...
go test ./... -count=1
go test -race ./...
CGO_ENABLED=0 go build ./cmd/loopy
make tui-smoke
scripts/demo.sh

scripts/dist.sh v0.1.0-rc.1
(cd dist && shasum -a 256 -c SHA256SUMS)
```

For Homebrew, run the checks against an actual local tap/formula name, not
only `dist/loopy.rb`. The final evidence should include the equivalent of:

```bash
brew style mjbarefo/tap/loopy
brew audit --new --formula mjbarefo/tap/loopy
brew install mjbarefo/tap/loopy
brew test mjbarefo/tap/loopy
loopy version
brew uninstall loopy
```

Use clean temp repositories and clean install environments where practical.
Do not declare success based only on mocks or generated YAML.

## How to work

- Make a short evidence-based plan after the initial audit, then execute it.
- Keep the work in reviewable commits or small PRs when that reduces risk,
  but own the dependency order and carry the effort through.
- Dogfood loopy for implementation tasks with crisp verifiers, then review
  the parked diff manually before applying it.
- Do not rewrite unrelated code or erase existing user changes.
- Keep docs and behavior synchronized as changes land.
- Never weaken tests to make the release green.

You are allowed to change your plan when using the product teaches you
something. Good taste here means noticing what the checklist missed.

## Definition of done

Before asking to publish:

- The TUI has a coherent design rationale and has been exercised in real PTYs,
  not merely unit-tested.
- All repository gates pass from a clean checkout.
- The license decision is resolved and represented consistently.
- Release artifacts are reproducible, versioned, checksummed, and verified.
- The tap repository and automation are ready and tested locally.
- A release candidate can be installed, exercised, upgraded, and removed via
  Homebrew.
- README, quickstart, help, and release notes describe the same product.
- The release runbook contains exact commands and rollback steps.
- Remaining limitations are explicit rather than hidden.

Then present me with:

1. The product/design decisions you made and what surprised you.
2. Before/after TUI evidence and the scenarios tested.
3. The exact verification results.
4. The branch, commits, and PRs.
5. The proposed license and any still-required owner action.
6. The exact commands you intend to run for the RC tag, public GitHub release,
   tap publication, and clean-install verification.

Stop for approval immediately before the first irreversible public action.
After approval, execute the release end to end, verify the public artifacts
and tap from scratch, and report the final tag, release URL, tap URL, install
command, and observed `loopy version` output.
