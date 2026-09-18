#!/usr/bin/env bash
# Runs errcheck and fails only on discards that are not already recorded in
# the committed baseline (errcheck.baseline) — see docs/adr and issue #312.
#
# internal/ has a large pre-existing backlog of discarded errors (253+
# db.Exec( calls alone, per the third-party review that opened #312), mostly
# harmless bookkeeping. Fixing it all at once was out of scope for wiring up
# the check; the baseline lets CI gate new discards without blocking on the
# old ones. The ones on the governance write path that are NOT harmless are
# tracked separately by issue #309.
#
# Each baseline entry is "path\ttext" with the line:col numbers stripped, so
# an unrelated edit that shifts line numbers within a file does not produce a
# false "new violation" — but it also means a genuinely new discard whose
# expression text exactly matches an already-baselined one *in the same
# file* can slip through unflagged. That is an accepted trade-off for a
# backlog this size; the fix is shrinking the baseline (see below), not
# making the matching exact.
#
# Usage: scripts/errcheck-check.sh
set -euo pipefail
cd "$(dirname "$0")/.."

# Pin the version here and in .github/workflows/ci.yaml together — same
# reasoning as the govulncheck pin: a step that silently starts using a
# newer tool version is not something a PR diff shows you.
ERRCHECK_VERSION=v1.20.0

baseline="errcheck.baseline"

# errcheck exits non-zero whenever it finds anything, which today is always
# true (the backlog), so the exit code alone tells us nothing — only the
# line-by-line diff against the baseline does.
raw="$(go run "github.com/kisielk/errcheck@${ERRCHECK_VERSION}" ./... 2>&1 || true)"

normalize() {
	grep -E '^[^:]+:[0-9]+:[0-9]+:' | sed -E 's/^([^:]+):[0-9]+:[0-9]+:/\1:/' | tr '\\' '/' | sort
}

current="$(printf '%s\n' "$raw" | normalize)"
known="$(sort "$baseline")"

# Both sides are sorted with duplicates kept, so this is a multiset diff: a
# `<` line is one the current run has one more of than the baseline allows.
new_only="$(diff <(printf '%s\n' "$current") <(printf '%s\n' "$known") | grep '^< ' | sed 's/^< //' || true)"

if [ -n "$new_only" ]; then
	echo "errcheck: unchecked error(s) not covered by errcheck.baseline:" >&2
	echo "$new_only" >&2
	echo >&2
	echo "Check the error return, or if it is genuinely fine to discard, regenerate the baseline:" >&2
	echo "  go run github.com/kisielk/errcheck@${ERRCHECK_VERSION} ./... 2>&1 | grep -E '^[^:]+:[0-9]+:[0-9]+:' | sed -E 's/^([^:]+):[0-9]+:[0-9]+:/\\1:/' | tr '\\\\' '/' | sort > errcheck.baseline" >&2
	echo "then check the diff really only adds the lines you intended (or removes lines you fixed)." >&2
	exit 1
fi

echo "errcheck: no discards outside the baseline ($(printf '%s\n' "$known" | sed '/^$/d' | wc -l | tr -d ' ') entries)."
