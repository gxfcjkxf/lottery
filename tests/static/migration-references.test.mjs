import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
import { extname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('../../', import.meta.url))
const historical = new Set(['docs/implementation-progress.md', 'docs/requirements-draft.md'])
// These names are deliberately unsupported metadata in rejection tests, never SQL inputs.
const rejectionFixtures = new Map([
  ['backend/internal/database/integration_test.go', new Set(['9998_unsupported.up.sql'])],
  ['backend/internal/database/migration_status_test.go', new Set(['9999_future.up.sql'])],
])
rejectionFixtures.set('tests/static/migration-references.test.mjs', new Set([...rejectionFixtures.values()].flatMap(names => [...names])))
const extensions = new Set(['.md', '.go', '.mjs', '.js', '.ts', '.sh', '.py', '.yaml', '.yml'])
function files(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap(entry => {
    const path = join(directory, entry.name)
    if (entry.isDirectory() && ['.local', '.git', 'node_modules'].includes(entry.name)) return []
    return entry.isDirectory() ? files(path) : extensions.has(extname(path)) ? [path] : []
  })
}

test('current documentation, scripts and tests reference only installed migration files', () => {
  const migrations = new Set(readdirSync(join(root, 'backend/migrations')).filter(name => name.endsWith('.up.sql')))
  const paths = ['docs', 'scripts', 'tests', 'backend', '.github'].flatMap(name => files(join(root, name)))
  paths.push(join(root, 'README.md'), join(root, 'AGENTS.md'), join(root, 'Makefile'))
  const stale = []
  for (const path of paths) {
    const name = relative(root, path)
    if (historical.has(name)) continue
    const content = readFileSync(path, 'utf8')
    for (const match of content.matchAll(/\b\d{4}_[a-z][a-z0-9_]*\.up\.sql\b/g)) {
      if (!migrations.has(match[0]) && !rejectionFixtures.get(name)?.has(match[0])) stale.push(`${name}: ${match[0]}`)
    }
    if (name.startsWith('docs/')) assert.doesNotMatch(content, /仅支持当前迁移序列的新迁移|TestAppliedBaselineChecksumIsImmutable/, name)
  }
  assert.deepEqual(stale, [], 'Update obsolete migration references instead of restoring removed SQL files')
})

test('historical implementation evidence explicitly forbids following old migration instructions', () => {
  assert.match(readFileSync(join(root, 'docs/implementation-progress.md'), 'utf8'), /不作为迁移操作指引/)
})
