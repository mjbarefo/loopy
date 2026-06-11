# Releasing loopy

The exact path from clean `main` to an installable release. RC-first: every
stable release is preceded by at least one release candidate that soaked as
a GitHub prerelease. Stable releases publish to the Homebrew tap; RCs never
do.

## One-time setup (before the first release)

1. **Make the repository public.** Release assets and the tap depend on it.
2. **Create the tap repository:**

   ```sh
   scripts/tap-bootstrap.sh /tmp/homebrew-tap --push
   ```

   This assembles `packaging/homebrew-tap/` into a repo and creates
   `github.com/mjbarefo/homebrew-tap` (public).
3. **Create the tap token.** The release workflow notifies the tap via
   `repository_dispatch`, which needs a token that can write to the tap:
   - GitHub → Settings → Developer settings → Fine-grained personal access
     tokens → generate; Resource owner `mjbarefo`; Repository access: only
     `mjbarefo/homebrew-tap`; Permissions: **Contents: read and write**.
   - Save it as the `TAP_GITHUB_TOKEN` actions secret on `mjbarefo/loopy`.
   - Rotation: the token expires (pick ≤1 year); when it does, releases
     still succeed but print a warning and the tap must be updated manually
     (see Recovery below). Regenerate and replace the secret.

## Cutting a release candidate

```sh
git checkout main && git pull --ff-only
make check && go test -race ./... && make tui-smoke && scripts/demo.sh

# CHANGELOG.md must already have the ## [vX.Y.Z] section (the workflow
# enforces this). Then:
git tag v0.1.0-rc.1
git push origin v0.1.0-rc.1
```

The Release workflow validates the tag (exact SemVer, commit on main,
changelog section present), re-runs the full gate, builds six reproducible
CGO-free archives with the version embedded, checks the binary reports the
tag, attests build provenance, and publishes a **prerelease** with
SHA256SUMS, the generated formula, and install/verify/report notes.

### Verify the RC

```sh
gh release view v0.1.0-rc.1 --repo mjbarefo/loopy
gh release download v0.1.0-rc.1 --repo mjbarefo/loopy --pattern '*darwin_arm64*' --pattern 'SHA256SUMS'
shasum -a 256 -c SHA256SUMS --ignore-missing
gh attestation verify loopy_v0.1.0-rc.1_darwin_arm64.tar.gz --repo mjbarefo/loopy
tar -xzf loopy_v0.1.0-rc.1_darwin_arm64.tar.gz loopy && ./loopy version
# → loopy v0.1.0-rc.1
```

Soak: run real loops against the RC for at least a day. Anything found →
fix on main → `v0.1.0-rc.2`.

## Cutting the stable release

```sh
git checkout main && git pull --ff-only
# main must equal the soaked RC plus only release-note changes; diff it:
git diff v0.1.0-rc.N..main --stat

git tag v0.1.0
git push origin v0.1.0
```

Same pipeline, published as a full release. The workflow then fires
`repository_dispatch` (`loopy-release`) at the tap; the tap's
`update-formula` workflow downloads `loopy.rb` from the release, syntax
checks it, and commits it to `Formula/loopy.rb`.

### Verify the stable release end to end

```sh
# 1. The release and its assets
gh release view v0.1.0 --repo mjbarefo/loopy

# 2. The tap picked it up
gh run list --repo mjbarefo/homebrew-tap --limit 3
git -C "$(brew --repository mjbarefo/tap 2>/dev/null || echo /tmp)" pull 2>/dev/null || true

# 3. Clean install on a machine (or after `brew untap mjbarefo/tap`)
brew install mjbarefo/tap/loopy
loopy version          # → loopy v0.1.0
loopy help
brew test mjbarefo/tap/loopy
brew audit --online --formula mjbarefo/tap/loopy

# 4. Provenance of what brew installed
gh attestation verify "$(brew --cache --formula mjbarefo/tap/loopy)" --repo mjbarefo/loopy
```

Finally: move the `[Unreleased]` heading in CHANGELOG.md, commit, and PR.

## Recovery and rollback

- **Release workflow failed mid-way** — fix the cause and re-run the
  workflow run (`gh run rerun <id>`), or delete the release (not the tag)
  and re-run: publishing is idempotent (`--clobber` upload + notes edit).
- **Tag points at the wrong commit and nothing was published:**
  `git tag -d vX.Y.Z && git push origin :refs/tags/vX.Y.Z`, then re-tag.
  Never move a tag that produced a published release — cut the next patch
  version instead.
- **Bad stable release already published:**
  1. `gh release edit vX.Y.Z --draft` to pull it from /releases/latest.
  2. Revert the tap: in homebrew-tap,
     `git revert <commit> && git push` (or run the update-formula workflow
     manually with the previous good version via `workflow_dispatch`).
  3. Cut the fixed `vX.Y.(Z+1)` through the normal RC path.
- **Tap dispatch failed (expired token, network):** run the tap's
  "Update formula" workflow by hand: Actions → Update formula → Run
  workflow → enter the stable tag. Or locally:

  ```sh
  gh release download vX.Y.Z --repo mjbarefo/loopy --pattern loopy.rb
  cd <homebrew-tap checkout> && cp ../loopy.rb Formula/loopy.rb
  git commit -am "loopy vX.Y.Z" && git push
  ```

## Decisions baked into this pipeline

- **Upstream-binary formula** (installs release artifacts, no compile):
  fast installs, and what you run is exactly what was attested. The cost is
  trusting GitHub release assets, which the attestation check covers.
- **RCs are isolated**: prereleases on GitHub only; the tap's update
  workflow hard-rejects non-stable versions. Installing an RC is a
  deliberate act (direct download or `go install …@vX.Y.Z-rc.N`).
- **Provenance**: GitHub artifact attestations on every archive
  (`gh attestation verify`). No separate SBOM file — the binaries embed
  their full module graph (`go version -m loopy`), which is the SBOM that
  cannot drift.
