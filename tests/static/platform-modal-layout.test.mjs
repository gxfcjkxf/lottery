import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
const root = new URL('../../platform-web/src/', import.meta.url)
test('platform detail dialogs do not nest page cards or repeat their modal title', () => {
  let dialogs = 0
  for (const file of readdirSync(root).filter(name => name.endsWith('.vue'))) {
    const source = readFileSync(new URL(file, root), 'utf8')
    for (const match of source.matchAll(/<DetailModal\b[^>]*>([\s\S]*?)<\/DetailModal>/g)) {
      dialogs++
      assert.doesNotMatch(match[1], /class="panel reward-detail"/, file)
      assert.doesNotMatch(match[1], /<h[23]>\{\{ copy\.detail \}\}/, file)
    }
  }
  assert.ok(dialogs >= 11)
  const recharge = readFileSync(new URL('RechargesPanel.vue', root), 'utf8')
  assert.match(recharge, /<details class="detail-technical">/)
  assert.doesNotMatch(recharge.slice(recharge.indexOf('<DetailModal')), /\{\{ copy\.member \}\}/)
})
