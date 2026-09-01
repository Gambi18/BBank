#!/usr/bin/env bash
# Enforces the dependency rule from TRD §4.2. Run in CI and locally.
#
#   domain  imports nothing from this project
#   service may import domain and store
#   http    may import service, domain, store, dto, response, legacy, middleware, platform
#   nothing imports http except cmd/
#
# A layered structure that is not enforced stops being layered within a month.
set -uo pipefail
cd "$(dirname "$0")"
MOD=$(go list -m)
fail=0

violation() { echo "  ARCH VIOLATION: $*"; fail=1; }

# domain must be pure.
deps=$(go list -deps ./internal/domain 2>/dev/null | grep "^${MOD}/" | grep -v "^${MOD}/internal/domain$" || true)
if [ -n "$deps" ]; then
  violation "internal/domain imports project packages:"; echo "$deps" | sed 's/^/      /'
fi

# service must not import http.
if go list -deps ./internal/service 2>/dev/null | grep -q "^${MOD}/internal/http"; then
  violation "internal/service imports internal/http"
fi

# store must not import service or http.
for bad in service http; do
  if go list -deps ./internal/store 2>/dev/null | grep -q "^${MOD}/internal/${bad}"; then
    violation "internal/store imports internal/${bad}"
  fi
done

# Only cmd/ may import http.
for pkg in $(go list ./internal/... 2>/dev/null); do
  case "$pkg" in
    "${MOD}/internal/http"|"${MOD}/internal/http/"*) continue ;;
  esac
  if go list -f '{{join .Imports "\n"}}' "$pkg" 2>/dev/null | grep -q "^${MOD}/internal/http$"; then
    violation "$pkg imports internal/http (only cmd/ may)"
  fi
done

if [ "$fail" -eq 0 ]; then
  echo "  architecture OK — dependency rule holds"
fi
exit "$fail"
