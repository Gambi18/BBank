#!/usr/bin/env bash
#
# WI-25's acceptance criterion, as a gate: "No clinical constant remains in Go
# source. `grep` finds no numeric clinical literal in `internal/`."
#
# Why a script and not a code review: a clinical threshold compiled into Go is a
# threshold nobody can correct without a deploy, and `FR-20` / `FR-68` require
# them to be editable, versioned, effective-dated policy rows. The failure is
# quiet — the code works, it is simply not correctable — so it needs a check
# that runs whether or not anyone is looking.
#
# What it checks: the distinctive values from the seeded policy set (schema
# §12.1) must not appear as numeric literals in non-test Go source under
# internal/. Comments are stripped first, because a comment that says "35 days,
# CPDA-1" is documentation, not a constant.
#
# What it deliberately does NOT check: small round numbers — 7, 18, 24, 50, 60,
# 65, 72, 120. They are far too common to flag. The first version of this script
# included 7 (the platelet interval) and 24 (the apheresis cap), and immediately
# failed on the 7-day refresh TTL, the 7-day invite TTL, a severity index and
# three separate 24-hour windows — six true negatives and no true positive,
# which is a gate nobody would keep.
#
# The values below could only be clinical. That is a narrower net than "every
# policy number", and it is honest about being one: the second half of this
# check is `TestSeedCoversEveryKeyTheDomainNeeds`, which comes at the same
# problem from the other end by asserting every threshold the domain reads has a
# seeded row. A number missing from the seed fails there; a number hardcoded in
# Go fails here. Between them, a threshold has to exist in policy AND not exist
# in source.
set -euo pipefail

cd "$(dirname "$0")"

# key                                  literal that would betray it
CLINICAL=(
    "donation_interval_days.whole_blood:56"
    "donor_min_hemoglobin_g_dl:12.5"
    "donor_min_hemoglobin_g_dl:13.0"
    "shelf_life_hours.whole_blood:840"
    "shelf_life_hours.packed_red_cells:1008"
    "shelf_life_hours.fresh_frozen_plasma:8760"
)

# Files to scan: non-test Go under internal/, excluding the generated store.
mapfile -t FILES < <(find internal -name '*.go' ! -name '*_test.go' ! -path 'internal/store/*' | sort)

# strip_comments removes // and /* */ comments and the contents of string and
# rune literals, so only real numeric literals survive to be matched.
strip_comments() {
    gofmt -r 'a -> a' "$1" 2>/dev/null |
        perl -0777 -pe '
            s{/\*.*?\*/}{}gs;          # block comments
            s{//[^\n]*}{}g;            # line comments
            s{`[^`]*`}{""}gs;          # raw strings
            s{"(\\.|[^"\\])*"}{""}g;   # interpreted strings
            s{'"'"'(\\.|[^'"'"'\\])'"'"'}{0}g;  # runes
        '
}

fail=0
for entry in "${CLINICAL[@]}"; do
    key="${entry%%:*}"
    literal="${entry##*:}"

    for f in "${FILES[@]}"; do
        # \b on both sides so 56 does not match 156 or 560; the decimal values
        # need the dot escaped, which printf %s of the literal already gives us
        # inside the bracket-free pattern below.
        pattern="(^|[^0-9A-Za-z_.])${literal//./\\.}([^0-9]|$)"
        # Materialised first, deliberately. Piping into `grep -q` under
        # `set -o pipefail` is a trap: grep exits on the first match, the
        # upstream `gofmt | perl` dies on SIGPIPE once its output exceeds the
        # pipe buffer, the pipeline status becomes 141, and a REAL hardcoded
        # constant in a large file is reported as clean. A gate that fails open
        # under exactly the conditions it is meant to catch is worse than none.
        stripped=$(strip_comments "$f")
        if hit=$(printf '%s\n' "$stripped" | grep -En "$pattern" | head -1); then
            line=${hit%%:*}
            echo "  FAIL: $f:$line has the literal $literal"
            echo "        That is the '$key' policy. Read it from active_policies"
            echo "        via domain.Policies, not from Go source (FR-20, FR-68)."
            fail=1
        fi
    done
done

if [ "$fail" -ne 0 ]; then
    echo
    echo "  clinical constants found in Go source — see backend/noclinical.sh"
    exit 1
fi

echo "  no clinical constant in Go source — all ${#CLINICAL[@]} checked values live in policy"
