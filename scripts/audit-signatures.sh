#!/usr/bin/env bash
#
# Verify the registry signatures on web/'s dependencies.
#
# `npm audit signatures` answers two different questions and the CI step used
# to treat both answers as the same red check:
#
#   1. Does a package's tarball match the signature npm published for it?
#      A mismatch means the bytes are not the bytes npm signed. That is the
#      supply-chain event this gate exists to catch, and it must fail, loudly
#      and on the first attempt, because a mismatch does not heal.
#
#   2. Can we fetch the public key needed to check an attestation at all?
#      `EMISSINGSIGNATUREKEY` is npm answering "this package has attestations
#      but I cannot give you the key". That is a fact about the registry on
#      the day, not about this repository, and retrying it three times in
#      fifty seconds does not make the key appear.
#
# Conflating them cost real time: over the last twenty-five runs on main this
# job failed three times out of ten completed, always on (2), always on
# @playwright/test, and always needing a human to press re-run. Main flapped
# failure / success / failure / success inside forty minutes on one afternoon.
# A check that goes red a third of the time for a reason nobody in this repo
# can act on stops being read, which is the way a gate dies.
#
# So this script classifies before it decides:
#
#   - invalid signatures     -> fail immediately, no retries
#   - missing keys only      -> a short retry to ride out a blip, then pass
#                               with a warning naming the packages
#   - anything unrecognised  -> fail, because an unknown failure mode is not
#                               one we have decided is safe
#
# The warning is deliberate, and it is the part to argue with if you disagree.
# Passing means a persistent registry outage no longer blocks every PR; the
# cost is that if npm stopped serving keys for good, the annotation is the
# only thing that would say so. It names the packages every run so the
# silence is never total.

set -uo pipefail

cd "$(dirname "$0")/.."/web || exit 1

ATTEMPTS=${AUDIT_SIGNATURES_ATTEMPTS:-3}
SLEEP=${AUDIT_SIGNATURES_SLEEP:-20}

# warn emits a GitHub annotation when running in Actions, and a plain line
# anywhere else, so the same script reads correctly from a terminal.
warn() {
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    echo "::warning title=npm audit signatures::$1"
  else
    echo "warning: $1" >&2
  fi
}

out=""
err=""
for attempt in $(seq 1 "$ATTEMPTS"); do
  err_file=$(mktemp)
  out=$(npm audit signatures --json 2>"$err_file")
  status=$?
  err=$(cat "$err_file")
  rm -f "$err_file"

  if [ "$status" -eq 0 ]; then
    echo "npm audit signatures: every package verified."
    exit 0
  fi

  # A real mismatch, reported in the JSON rather than as an npm error code.
  # Printed whole and failed on the spot: this is the one outcome the gate
  # exists for, and making somebody wait out two sleeps to see it would be
  # the wrong way round.
  if printf '%s' "$out" | grep -q '"invalid"[[:space:]]*:[[:space:]]*\[[[:space:]]*[^][:space:]]'; then
    echo "npm audit signatures: a package does not match the signature npm published for it." >&2
    printf '%s\n' "$out" >&2
    exit 1
  fi

  # The registry could not hand over a key. Nothing here can fix that, so
  # retry briefly in case it is a blip and stop pretending a longer wait
  # would help: main has shown this condition outlast any sleep worth
  # putting in a CI job.
  if printf '%s' "$err" | grep -q 'EMISSINGSIGNATUREKEY'; then
    if [ "$attempt" -lt "$ATTEMPTS" ]; then
      echo "npm audit signatures: registry did not serve a verification key (attempt $attempt/$ATTEMPTS)" >&2
      sleep "$SLEEP"
      continue
    fi
    packages=$(printf '%s' "$err" | grep -o '[^ ]*@[^ ]* has attestations' | sed 's/ has attestations//' | sort -u | tr '\n' ' ')
    warn "npm could not serve the public key for: ${packages:-unknown}. Signatures were not verified for these. This is a registry condition, not a signature mismatch; no package failed verification."
    echo "npm audit signatures: no signature mismatched. Keys unavailable for: ${packages:-unknown}" >&2
    exit 0
  fi

  # Something else. Not swallowed: the two cases above are the ones we have
  # reasoned about, and a third one is a thing to look at.
  echo "npm audit signatures failed for a reason this script does not recognise." >&2
  printf '%s\n' "$err" >&2
  printf '%s\n' "$out" >&2
  exit 1
done

exit 1
