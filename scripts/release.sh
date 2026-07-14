#!/usr/bin/env bash
#
# release.sh — mint a plugin release version in one step.
#
# The version is NOT bumped per PR (that caused number collisions when several
# PRs landed on develop in the same week). Instead PRs only append a bullet under
# the `## [Unreleased]` section of CHANGELOG.md, and this script mints the version
# once, when promoting develop → main:
#
#   1. rewrites `## [Unreleased]` into a dated `## [x.y.z]` section and starts a
#      fresh empty `## [Unreleased]` above it;
#   2. bumps the version in lockstep across the six files that carry it.
#
# It does NOT commit, tag, or push — review the result, commit it, merge to main,
# then tag `vx.y.z` (the Release workflow reads the `## [x.y.z]` CHANGELOG section
# for the GitHub release notes).
#
# Usage:  make release VERSION=2.5.0     (or)     scripts/release.sh 2.5.0

set -euo pipefail

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  echo "ERROR: no version given. Usage: scripts/release.sh <x.y.z>  (or: make release VERSION=x.y.z)" >&2
  exit 1
fi
if ! printf '%s' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$'; then
  echo "ERROR: version '$VERSION' is not semver (expected x.y.z, optionally x.y.z-suffix)." >&2
  exit 1
fi

# Repo root = the parent of this script's dir, so the script works from anywhere.
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CHANGELOG="CHANGELOG.md"

# The six files that carry the plugin version, in lockstep.
VERSION_FILES="
plugins/simulator/.claude-plugin/plugin.json
plugins/simulator/.codex-plugin/plugin.json
plugins/simulator/.kiro-plugin/plugin.json
.claude-plugin/marketplace.json
.agents/plugins/marketplace.json
POWER.md
"

# Current version is the one declared in the canonical plugin manifest.
CURRENT="$(grep -m1 '"version"' plugins/simulator/.claude-plugin/plugin.json | sed -E 's/.*"version": *"([^"]+)".*/\1/')"
if [ -z "$CURRENT" ]; then
  echo "ERROR: could not read the current version from plugins/simulator/.claude-plugin/plugin.json" >&2
  exit 1
fi
if [ "$CURRENT" = "$VERSION" ]; then
  echo "ERROR: current version is already $VERSION — nothing to bump." >&2
  exit 1
fi

# Fail fast if the six files are not already in lockstep — a drifted file would
# be silently missed by the exact-string bump below.
drift=0
for f in $VERSION_FILES; do
  case "$f" in
    POWER.md) v="$(grep -m1 '^version:' "$f" | sed -E 's/^version: *//')" ;;
    *)        v="$(grep -m1 '"version"' "$f" | sed -E 's/.*"version": *"([^"]+)".*/\1/')" ;;
  esac
  if [ "$v" != "$CURRENT" ]; then
    echo "ERROR: $f is at '$v' but the canonical version is '$CURRENT' — the version files are out of lockstep. Fix them before releasing." >&2
    drift=1
  fi
done
[ "$drift" -eq 0 ] || exit 1

# CHANGELOG must have an Unreleased section to promote.
if ! grep -q '^## \[Unreleased\]' "$CHANGELOG"; then
  echo "ERROR: no '## [Unreleased]' section in $CHANGELOG — add one (PRs append their entries there)." >&2
  exit 1
fi

DATE="$(date +%F)"
CURRENT_RE="$(printf '%s' "$CURRENT" | sed 's/\./\\./g')"

echo "Releasing $CURRENT → $VERSION ($DATE)"

# 1) CHANGELOG: leave `## [Unreleased]` in place (now empty) and insert a dated
#    `## [x.y.z]` header right below it, so the accumulated Unreleased entries
#    become this version's notes.
tmp="$(mktemp)"
awk -v ver="$VERSION" -v date="$DATE" '
  !done && /^## \[Unreleased\]/ {
    print
    print ""
    print "## [" ver "] - " date
    done = 1
    next
  }
  { print }
' "$CHANGELOG" > "$tmp" && mv "$tmp" "$CHANGELOG"

# Warn (do not fail) if the new section has no entries — a release with an empty
# changelog is legal but usually a mistake.
if awk -v ver="$VERSION" '
  $0 ~ "^## \\[" ver "\\]" {inseg=1; next}
  inseg && /^## /{exit}
  inseg && /[^[:space:]]/{found=1}
  END{exit found?0:1}
' "$CHANGELOG"; then :; else
  echo "WARNING: the new ## [$VERSION] section has no entries — was ## [Unreleased] empty?" >&2
fi

# 2) Bump the version in the six files (exact old→new string, so unrelated
#    version fields in other files — swagger specs, SKILL.md — are untouched).
for f in $VERSION_FILES; do
  tmp="$(mktemp)"
  case "$f" in
    POWER.md) sed "s/^version: ${CURRENT_RE}\$/version: ${VERSION}/" "$f" > "$tmp" ;;
    *)        sed "s/\"version\": \"${CURRENT_RE}\"/\"version\": \"${VERSION}\"/" "$f" > "$tmp" ;;
  esac
  mv "$tmp" "$f"
  echo "  bumped $f"
done

cat <<EOF

Done. Version files and CHANGELOG are at $VERSION.
Next:
  1. review the diff:            git diff
  2. commit:                     git commit -am "chore(release): $VERSION"
  3. merge develop → main, then tag on main:
                                 git tag v$VERSION && git push origin v$VERSION
     (the Release workflow builds binaries and reads the ## [$VERSION] notes)
EOF
