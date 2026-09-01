#!/usr/bin/env bash
# Coverage gates (WI-29, IMPLEMENTATION_PLAN §4.1).
#
#   internal/domain  >= 90%   the clinical rules; no database, no excuse
#   internal/service >= 70%   use cases and transaction boundaries
#   overall                   reported, not gated
#
# Why gate these two and not the rest: a threshold on a package is only useful
# where uncovered lines mean untested *decisions*. `internal/store` is generated,
# `internal/http` is wiring, and gating them would buy tests written to move a
# number rather than to catch anything.
#
# Each package is measured with -coverpkg pointed at ITSELF. Without that, a
# repo-wide -coverpkg spreads every package's statements across every profile
# and each number collapses to a fraction of the truth.
#
# Requires Docker: the service tests run against a real PostgreSQL 18 via
# testcontainers. Run with SHORT=1 to skip those (and the service gate with it).
set -uo pipefail
cd "$(dirname "$0")"

fail=0
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# package:threshold
GATES=(
  "internal/domain:90"
  "internal/service:70"
)

short_flag=""
if [ "${SHORT:-0}" = "1" ]; then
  short_flag="-short"
  echo "  SHORT=1 — integration tests skipped; database-backed gates will be reported, not enforced"
fi

for gate in "${GATES[@]}"; do
  pkg="${gate%%:*}"
  want="${gate##*:}"
  out="$tmp/$(echo "$pkg" | tr / _).out"

  if ! go test $short_flag "./$pkg/" -coverpkg="./$pkg/" -coverprofile="$out" >"$tmp/log" 2>&1; then
    echo "  FAIL: tests failed in $pkg"
    sed 's/^/      /' "$tmp/log"
    fail=1
    continue
  fi

  # A skipped package produces a profile with no statements; report rather than
  # divide by zero and call it 0%.
  if [ ! -s "$out" ]; then
    echo "  SKIP: $pkg produced no coverage profile (tests skipped?)"
    continue
  fi

  got=$(go tool cover -func="$out" | tail -1 | awk '{print $3}' | tr -d '%')
  if awk -v g="$got" -v w="$want" 'BEGIN{exit !(g+0 < w+0)}'; then
    if [ -n "$short_flag" ]; then
      echo "  (not enforced under SHORT) $pkg: ${got}% < ${want}%"
    else
      echo "  FAIL: $pkg coverage ${got}% is below the ${want}% gate"
      # Name the worst offenders, so the failure is actionable rather than a number.
      go tool cover -func="$out" | awk '$3+0 < 50 && $1 !~ /^total/ {print "      uncovered: " $2 " (" $3 ") " $1}' | head -10
      fail=1
    fi
  else
    echo "  OK:   $pkg ${got}% (gate ${want}%)"
  fi
done

# Overall, reported only. A single repo-wide number is a poor gate — it can be
# held up by generated code — but it is worth watching for a trend.
if [ -z "$short_flag" ]; then
  if go test ./internal/... -coverpkg=./internal/... -coverprofile="$tmp/all.out" >"$tmp/alllog" 2>&1; then
    overall=$(go tool cover -func="$tmp/all.out" | tail -1 | awk '{print $3}')
    echo "  ---   backend overall: $overall (reported, not gated)"
  fi
fi

if [ "$fail" -eq 0 ]; then
  echo "  coverage OK"
fi
exit "$fail"
