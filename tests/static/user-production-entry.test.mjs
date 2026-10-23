import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, existsSync } from 'node:fs'
const root = new URL('../../', import.meta.url)
const read = path => readFileSync(new URL(path, root), 'utf8')

test('ordinary user routes do not expose the retired draft-file tool', () => {
  assert.doesNotMatch(read('user-web/src/main.ts'), /ConfiguredRulePreview|path:\s*['"]\/rule-preview['"]/)
  assert.doesNotMatch(read('user-web/src/Page.vue'), /rules\.preview|to=["']\/rule-preview["']/)
  assert.equal(existsSync(new URL('user-web/src/ConfiguredRulePreview.vue', root)), false)
})

test('user policy consent obtains versions from context instead of a trial default', () => {
  const page = read('user-web/src/Page.vue')
  assert.doesNotMatch(page, /dev-1/)
  assert.match(page, /termsVersions\.value = context\.terms/)
  assert.match(page, /if \(!termsVersions\.value\) throw/)
  assert.doesNotMatch(read('user-web/src/i18n.ts'), /common\.mockVersion/)
})

test('the removed user preview does not leave obsolete sandbox translations', () => {
  assert.doesNotMatch(read('shared/src/configured-rule-catalog.ts'), /playerHint:|Configurable rule sandbox|可配置规则试验区/)
})
