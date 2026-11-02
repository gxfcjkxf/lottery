import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

test('platform uses one accessibly named topbar brand selector without duplicate brand badges', () => {
  const source = readFileSync(new URL('../../platform-web/src/App.vue', import.meta.url), 'utf8')
  assert.doesNotMatch(source, /access-brand-control|access-scope-row|selection-tag/)
  const selectors = [...source.matchAll(/<SelectControl[^>]*:model-value="selectedBrand"[^>]*>/g)]
  const labelled = [...source.matchAll(/<label v-if="showBrandPicker" class="topbar-brand-picker">\s*<SelectControl[^>]*:model-value="selectedBrand"[^>]*:label="lang.brandLabel"[^>]*>/g)]
  assert.equal(selectors.length, 1)
  assert.equal(labelled.length, selectors.length)
  assert.match(source, /brands\).*?map\(\(brand\) => \([^\n]*value: brand.id, label: \(String\(brand.name\)\)/)
  assert.match(source, /<SelectControl :disabled="accountWriteLocked" :model-value="selectedBrand"/)
})
