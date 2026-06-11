#!/bin/sh
# tap-bootstrap.sh — assemble the mjbarefo/homebrew-tap repository from
# packaging/homebrew-tap/ and (optionally) create + push it on GitHub.
#
# Usage:
#   scripts/tap-bootstrap.sh <dir>          # assemble into <dir>, git init
#   scripts/tap-bootstrap.sh <dir> --push   # …and create the public repo
#
# Pushing creates a PUBLIC repository — run it only as part of the release
# runbook (docs/releasing.md).
set -eu

cd "$(dirname "$0")/.."
dir=${1:?usage: tap-bootstrap.sh <dir> [--push]}
push=${2:-}

mkdir -p "$dir"
cp -R packaging/homebrew-tap/. "$dir/"
mkdir -p "$dir/Formula"

cd "$dir"
if [ ! -d .git ]; then
  git init -q -b main
fi
git add -A
git diff --cached --quiet || git commit -qm "tap scaffolding (from loopy packaging/homebrew-tap)"
echo "tap assembled at $dir"

if [ "$push" = "--push" ]; then
  gh repo create mjbarefo/homebrew-tap --public --description "Homebrew tap for loopy" \
    --source . --push
  echo "pushed: https://github.com/mjbarefo/homebrew-tap"
fi
