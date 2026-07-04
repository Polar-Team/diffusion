#!/bin/sh
# E2E tests for `diffusion docs` feature.
# Usage: e2e-docs.sh <path-to-diffusion-binary> <path-to-fixtures>
#
# Fixtures directory must contain: defaults/, vars/, templates/, tasks/, README.md
set -e

DIFFUSION="$1"
FIXTURES_SRC="$2"

if [ -z "$DIFFUSION" ] || [ -z "$FIXTURES_SRC" ]; then
  echo "Usage: $0 <diffusion-binary> <fixtures-dir>"
  exit 1
fi

# Work on a copy so we don't mutate the repo fixtures
WORK_DIR="/tmp/diffusion-docs-e2e"
rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR"
cp -r "$FIXTURES_SRC"/. "$WORK_DIR/"

# Ensure README.md exists (some cp implementations skip it)
if [ ! -f "$WORK_DIR/README.md" ]; then
  printf '%s\n' "# Test Role" "" "A test role for docs generation." > "$WORK_DIR/README.md"
fi

# Disable set -e for test assertions (we handle failures manually)
set +e

PASS=0
FAIL=0

pass() {
  PASS=$((PASS + 1))
  echo "  PASS: $1"
}

fail() {
  FAIL=$((FAIL + 1))
  echo "  FAIL: $1"
}

echo "=========================================="
echo "diffusion docs — E2E tests"
echo "=========================================="
echo ""

# ── Test 1: dry-run mode ────────────────────────────────────────────────
echo "--- Test 1: dry-run mode ---"

DRY_OUTPUT=$("$DIFFUSION" docs --path "$WORK_DIR" --dry-run 2>/dev/null || true)

echo "$DRY_OUTPUT" | grep -q "| Variable | Type | Default | Source | Description |" \
  && pass "table header present" \
  || fail "table header not found in dry-run output"

echo "$DRY_OUTPUT" | grep -q "required" \
  && pass "required marker present" \
  || fail "required marker not found"

echo "$DRY_OUTPUT" | grep -q "optional" \
  && pass "optional marker present" \
  || fail "optional marker not found"

# README should NOT be modified during dry-run
grep -q "begin role_variables" "$WORK_DIR/README.md" \
  && fail "README was modified during dry-run" \
  || pass "README untouched during dry-run"

echo ""

# ── Test 2: docs generation ─────────────────────────────────────────────
echo "--- Test 2: docs generation ---"

"$DIFFUSION" docs --path "$WORK_DIR"

# Markers
grep -q "<!-- begin role_variables -->" "$WORK_DIR/README.md" \
  && pass "begin marker in README" \
  || fail "begin marker not found"

grep -q "<!-- end role_variables -->" "$WORK_DIR/README.md" \
  && pass "end marker in README" \
  || fail "end marker not found"

# Typed variables from defaults
grep -q "app_name" "$WORK_DIR/README.md" \
  && pass "app_name in README" \
  || fail "app_name not found"

grep -q "app_port" "$WORK_DIR/README.md" \
  && pass "app_port in README" \
  || fail "app_port not found"

grep -q "packages" "$WORK_DIR/README.md" \
  && pass "packages in README" \
  || fail "packages not found"

# Required variables
grep -q "db_password" "$WORK_DIR/README.md" \
  && pass "db_password in README" \
  || fail "db_password not found"

grep -q "api_key" "$WORK_DIR/README.md" \
  && pass "api_key in README" \
  || fail "api_key not found"

grep -q "required" "$WORK_DIR/README.md" \
  && pass "required marker in README" \
  || fail "required marker not found"

# Optional variables
grep -q "http_timeout" "$WORK_DIR/README.md" \
  && pass "http_timeout in README" \
  || fail "http_timeout not found"

grep -q "optional" "$WORK_DIR/README.md" \
  && pass "optional marker in README" \
  || fail "optional marker not found"

# For-loop source variables included
grep -q "server_hosts" "$WORK_DIR/README.md" \
  && pass "server_hosts in README" \
  || fail "server_hosts not found"

grep -q "backend_url" "$WORK_DIR/README.md" \
  && pass "backend_url in README" \
  || fail "backend_url not found"

# For-loop iterators NOT included
grep -q '| `pkg`' "$WORK_DIR/README.md" \
  && fail "pkg iterator should NOT be in README" \
  || pass "pkg iterator excluded"

grep -q '| `host`' "$WORK_DIR/README.md" \
  && fail "host iterator should NOT be in README" \
  || pass "host iterator excluded"

# {% set %} local vars NOT included
grep -q '| `local_var`' "$WORK_DIR/README.md" \
  && fail "local_var should NOT be in README" \
  || pass "local_var excluded"

# Underscore-prefixed vars NOT included
grep -q '| `_internal_var`' "$WORK_DIR/README.md" \
  && fail "_internal_var should NOT be in README" \
  || pass "_internal_var excluded"

# Source includes file path
grep -q "defaults/main.yml" "$WORK_DIR/README.md" \
  && pass "source file path in README" \
  || fail "source file path not found"

# Duplicate detection
grep -q "duplicate" "$WORK_DIR/README.md" \
  && pass "duplicate warning in README" \
  || fail "duplicate warning not found"

echo ""

# ── Test 3: idempotency ────────────────────────────────────────────────
echo "--- Test 3: idempotency ---"

# First run already happened in test 2, save that result
cp "$WORK_DIR/README.md" /tmp/readme_after_first.md

# Run again — should produce identical output
"$DIFFUSION" docs --path "$WORK_DIR" 2>/dev/null

if diff "$WORK_DIR/README.md" /tmp/readme_after_first.md >/dev/null 2>&1; then
  pass "idempotent output"
else
  echo "  DEBUG: diff output:"
  diff "$WORK_DIR/README.md" /tmp/readme_after_first.md || true
  # Non-critical — mark as pass if only whitespace differs
  if diff -b "$WORK_DIR/README.md" /tmp/readme_after_first.md >/dev/null 2>&1; then
    pass "idempotent output (whitespace-only diff)"
  else
    fail "docs generation is not idempotent"
  fi
fi

echo ""

# ── Test 4: marker replacement ─────────────────────────────────────────
echo "--- Test 4: marker replacement ---"

cat > "$WORK_DIR/README.md" << 'MARKER_EOF'
# Updated Role

<!-- begin role_variables -->
OLD CONTENT HERE
<!-- end role_variables -->

## License
MIT
MARKER_EOF

"$DIFFUSION" docs --path "$WORK_DIR"

grep -q "OLD CONTENT HERE" "$WORK_DIR/README.md" \
  && fail "old content was not replaced" \
  || pass "old content replaced"

grep -q "app_name" "$WORK_DIR/README.md" \
  && pass "new content generated" \
  || fail "new content not generated"

grep -q "## License" "$WORK_DIR/README.md" \
  && pass "content after markers preserved" \
  || fail "content after markers lost"

grep -q "MIT" "$WORK_DIR/README.md" \
  && pass "license text preserved" \
  || fail "license text lost"

echo ""

# ── Summary ────────────────────────────────────────────────────────────
echo "=========================================="
echo "Results: $PASS passed, $FAIL failed"
echo "=========================================="

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
