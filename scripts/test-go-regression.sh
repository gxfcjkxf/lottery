#!/usr/bin/env bash
set -euo pipefail

# A missing database would make integration tests SKIP instead of validating
# history, settlement and ledger behavior.
: "${TEST_DATABASE_URL:?TEST_DATABASE_URL must identify an owned nonproduction test database}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Schema migrations share a database advisory lock, and real Argon2 hashing is
# expensive under -race. Bound resource contention without changing the tests
# or password parameters. The package deadline matches the existing CI suite;
# it covers every test in a package, including real period-boundary waits.
exec go -C "$repo_root/backend" test -race -p 1 -parallel 4 -count=1 -timeout=20m ./...
