#!/bin/sh
# release-notes.sh — emit the release notes for one tag to stdout.
#
# Usage: scripts/release-notes.sh <version>      (e.g. v0.1.0 or v0.1.0-rc.1)
#
# The changelog section comes from CHANGELOG.md (RCs reuse their stable
# section); install/verify instructions are appended so every release —
# especially an RC — explains how to install, verify, test, and report.
set -eu

cd "$(dirname "$0")/.."
version=${1:?usage: release-notes.sh <version>}
stable=${version%%-rc.*}

case "$version" in
*-rc.*)
  cat <<EOF
**This is a release candidate for $stable.** It soaks here before the
stable tag; install it, run your own loops, and report anything surprising
at https://github.com/mjbarefo/loopy/issues — include \`loopy version\` and
\`loopy doctor\` output. RCs are not published to the Homebrew tap.

EOF
  ;;
esac

# The version's section from CHANGELOG.md: from its heading to the next one.
awk -v v="$stable" '
  $0 ~ "^## \\[" v "\\]" { found = 1; next }
  found && /^## / { exit }
  found { print }
' CHANGELOG.md

cat <<EOF

## Install

Homebrew (macOS/Linux), stable releases only:

\`\`\`sh
brew install mjbarefo/tap/loopy
\`\`\`

Direct download — grab the archive for your platform below, then:

\`\`\`sh
tar -xzf loopy_${version}_<os>_<arch>.tar.gz   # .zip on Windows
./loopy version                                # should print: loopy $version
\`\`\`

From source: \`go install github.com/mjbarefo/loopy/cmd/loopy@$version\`

## Verify

Checksums:

\`\`\`sh
shasum -a 256 -c SHA256SUMS --ignore-missing
\`\`\`

Build provenance (GitHub attestations — proves the binary was built by this
repository's release workflow from this tag):

\`\`\`sh
gh attestation verify loopy_${version}_<os>_<arch>.tar.gz --repo mjbarefo/loopy
\`\`\`

## First five minutes

\`\`\`sh
loopy help                # the command surface
git clone https://github.com/mjbarefo/loopy && cd loopy && scripts/demo.sh
                          # a full loop converging, no API keys
\`\`\`

Problems? https://github.com/mjbarefo/loopy/issues
EOF
