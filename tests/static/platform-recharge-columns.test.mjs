import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

test('recharge orders and member usernames occupy separate columns', () => {
  const source = readFileSync(new URL('../../platform-web/src/RechargesPanel.vue', import.meta.url), 'utf8')
  const row = source.match(/<tr v-for="order in rows"[\s\S]*?<\/tr>/)?.[0]
  assert.ok(row)
  assert.equal([...row.matchAll(/<td>/g)].length, 5)
  assert.match(row, /@click="detail = order">\{\{ order\.id \}\}<\/button><\/td><td>\{\{ order\.member_username \}\}<\/td>/)
  assert.doesNotMatch(row, /<small>/)
  assert.match(source, /<th>\{\{ ui\('platform.copy.098b8e97'\) \}\}<\/th>/)
  assert.match(source, /colspan="5" class="empty-state"/)
})
