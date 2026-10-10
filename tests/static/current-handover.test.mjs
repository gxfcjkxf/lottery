import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8')

test('current handover uses the single baseline and independent platform creation route', () => {
  const readme = read('README.md')
  assert.match(readme, /0001_baseline\.up\.sql/)
  assert.match(readme, /POST \/api\/v1\/platform\/brands/)
  assert.doesNotMatch(readme, /POST \/api\/v1\/admin\/brands|migrate至00\d\d|0018–0020|原型演示订单|用户投注\/提现与业务订单仍待/)
  assert.match(readme, /platform-web\s+总后台/)
})

test('default infrastructure starts only the implemented PostgreSQL dependency', () => {
  assert.match(read('Makefile'), /infra:\n\tdocker compose up -d postgres\n/)
  assert.match(read('README.md'), /docker compose up -d postgres/)
  assert.doesNotMatch(read('.env.example'), /^(?:REDIS_URL|NATS_URL|OBJECT_STORAGE_\w+)=/m)
})
