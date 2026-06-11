#!/bin/sh
# dist.sh — build the six CGO-free release archives into dist/.
#
# Usage: scripts/dist.sh [version]
# Version defaults to `git describe`; release builds pass the tag.
#
# Archives are reproducible when GNU tar is available (CI builds on Linux):
# fixed mtimes from the commit date, sorted entries, no owner metadata.
# Local macOS builds fall back to plain bsdtar — fine for testing, not for
# publishing.
set -eu

cd "$(dirname "$0")/.."
version=${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
ldflags="-s -w -X github.com/mjbarefo/loopy/internal/loop.Version=$version"

SOURCE_DATE_EPOCH=$(git log -1 --format=%ct 2>/dev/null || echo 0)
export SOURCE_DATE_EPOCH

# GNU tar (gtar on macOS via brew) enables reproducible archives.
TAR=tar
if command -v gtar >/dev/null 2>&1; then TAR=gtar; fi
tar_repro=""
if $TAR --version 2>/dev/null | grep -q "GNU tar"; then
  tar_repro="--sort=name --owner=0 --group=0 --numeric-owner --mtime=@$SOURCE_DATE_EPOCH"
fi

rm -rf dist
mkdir -p dist

echo "==> building loopy $version"
for target in "darwin amd64" "darwin arm64" "linux amd64" "linux arm64" "windows amd64" "windows arm64"; do
  set -- $target
  GOOS=$1 GOARCH=$2
  bin=loopy
  [ "$GOOS" = windows ] && bin=loopy.exe
  out="dist/$GOOS-$GOARCH"
  mkdir -p "$out"
  echo "    $GOOS/$GOARCH"
  CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags "$ldflags" -o "$out/$bin" ./cmd/loopy
  [ -x "$out/$bin" ] || { echo "error: $out/$bin is not executable" >&2; exit 1; }
  cp README.md LICENSE "$out/"
  name="loopy_${version}_${GOOS}_${GOARCH}"
  if [ "$GOOS" = windows ]; then
    # -X drops platform extra fields; touch pins mtimes for reproducibility.
    (cd "$out" && find . -exec touch -t "$(date -r "$SOURCE_DATE_EPOCH" +%Y%m%d%H%M.%S 2>/dev/null || date -d "@$SOURCE_DATE_EPOCH" +%Y%m%d%H%M.%S)" {} + 2>/dev/null || true
     zip -q -X "../$name.zip" "$bin" README.md LICENSE)
  else
    # shellcheck disable=SC2086
    $TAR $tar_repro -czf "dist/$name.tar.gz" -C "$out" "$bin" README.md LICENSE
  fi
  rm -rf "$out"
done

(cd dist && shasum -a 256 ./* | sed 's|\./||' > SHA256SUMS)
echo "==> dist/ ready:"
ls -1 dist
