#!/usr/bin/env bash
# Copies Liquid Textile into web/src/vendor/liquid-textile/ and records which
# commit it came from. The vendored folder is never edited in place: change
# Liquid Textile, then run this, and the diff is the whole of the update.
#
#   scripts/sync-lt.sh ../liquid-textile
#
# Vendored rather than installed because Liquid Textile is private, and a
# dependency every fork inherits cannot require a key (docs/adr/077). When
# it is public, this folder becomes a package and the imports in
# web/src/main.js are the only lines that move.
#
# FILES is what Patchwork imports, nothing more. behaviors.js joined with
# step 4b (viewport, through web/src/lib/lt.js); base.css joins when a
# migration step starts using it.
set -euo pipefail

src="${1:?usage: scripts/sync-lt.sh <path-to-liquid-textile>}"
here="$(cd "$(dirname "$0")/.." && pwd)"
dest="$here/web/src/vendor/liquid-textile"
FILES=(foundation.css liquid-textile.css patterns.css textures.css behaviors.js LICENSE)

for f in "${FILES[@]}"; do
  [ -f "$src/$f" ] || { echo "sync-lt: $src/$f not found" >&2; exit 1; }
done

version="$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' "$src/package.json" | head -1)"
commit="$(git -C "$src" rev-parse --short HEAD)"
if [ -n "$(git -C "$src" status --porcelain -- "${FILES[@]}")" ]; then
  echo "sync-lt: $src has uncommitted changes to the vendored files; commit them first" >&2
  exit 1
fi

mkdir -p "$dest"
for f in "${FILES[@]}"; do
  # Normalize line endings so a Windows checkout of Liquid Textile does not
  # turn every line of the diff.
  tr -d '\r' < "$src/$f" > "$dest/$f"
done

cat > "$dest/README.md" <<EOF
# Liquid Textile (vendored)

Patchwork's design system: tokens, components and the textile layer, from
[Liquid Textile](https://github.com/nathanswavely/liquid-textile) (MIT; see
LICENSE). Copied in rather than installed, so a fork needs no key to build.

- Version: $version, commit $commit
- Files: ${FILES[*]}

Do not edit these copies. Change Liquid Textile, then run
\`scripts/sync-lt.sh <path-to-liquid-textile>\` from the repo root.
EOF

echo "sync-lt: Liquid Textile $version ($commit) -> web/src/vendor/liquid-textile/"
