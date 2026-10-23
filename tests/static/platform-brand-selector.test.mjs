import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

test('platform uses one accessibly named topbar brand selector without duplicate brand badges', () => {
  const source = readFileSync(new URL('../../platform-web/src/App.vue', import.meta.url), 'utf8')
  assert.doesNotMatch(source, /access-brand-control|access-scope-row|selection-tag/)
  const selectors = [...source.matchAll(/<select[^>]*:value="selectedBrand"[^>]*>/g)]
  const labelled = [...source.matchAll(/<label v-if="showBrandPicker" class="topbar-brand-picker">\s*<select[^>]*:value="selectedBrand"[^>]*:aria-label="lang.brandLabel"[^>]*>/g)]
  assert.equal(selectors.length, 1)
  assert.equal(labelled.length, selectors.length)
  const options = [...source.matchAll(/<option v-for="brand in brands"[^>]*>(.*?)<\/option>/g)]
  assert.equal(options.length, selectors.length)
  for (const [option, label] of options) {
    assert.equal(label, '{{ brand.name }}')
    assert.match(option, /:key="brand.id" :value="brand.id"/)
  }
  assert.match(source, /<select :disabled="accountWriteLocked" :value="selectedBrand"/)
})
