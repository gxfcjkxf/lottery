import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const workflow = readFileSync(new URL('../../.github/workflows/ci.yaml', import.meta.url), 'utf8')
const backend = workflow.match(/^  backend:\n([\s\S]*?)(?=^  frontend:)/m)?.[1]
const verifier = readFileSync(new URL('../../scripts/verify-betting-test-split.sh', import.meta.url), 'utf8')

test('backend CI partitions real betting tests without changing the per-binary race budget', () => {
  assert.ok(backend)
  assert.match(backend, /fail-fast: false/)
  assert.match(backend, /group: \[core, betting, commission\]/)
  assert.match(backend, /TEST_DATABASE_URL: postgres:/)
  assert.match(backend, /matrix\.group == 'commission'[\s\S]*?go test -race -count=1 -timeout=20m \.\/internal\/betting -run '\^TestCommission'/)
  assert.match(backend, /matrix\.group == 'betting'[\s\S]*?go test -race -count=1 -timeout=20m \.\/internal\/betting -skip '\^TestCommission'/)
  assert.match(backend, /bash \.\.\/scripts\/verify-betting-test-split\.sh/)
  assert.doesNotMatch(backend, /go test.*-run ['"]?\^\$/)
  assert.doesNotMatch(backend, /go test.*\|\| true/)
})

test('core runs every other package and the split validator derives coverage from the current inventory', () => {
  assert.match(backend, /betting_package="\$\(go list \.\/internal\/betting\)"/)
  assert.match(backend, /all_package_output="\$\(go list \.\/\.\.\.\)"/)
  assert.match(backend, /"\$package" == "\$betting_package"/)
  assert.match(backend, /go test -race -count=1 -timeout=20m "\$\{non_betting_packages\[@\]\}"/)
  assert.match(verifier, /set -euo pipefail/)
  assert.match(verifier, /list_tests -list '\^Test' \.\/internal\/betting/)
  assert.match(verifier, /list_tests -list '\^TestCommission' \.\/internal\/betting/)
  assert.match(verifier, /comm -12.*commission.*non-commission/)
  assert.match(verifier, /diff -u.*all.*union/)
  assert.match(verifier, /requires non-commission tests/)
  assert.doesNotMatch(verifier, /TEST_DATABASE_URL=/)
})
