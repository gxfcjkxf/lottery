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

test('authentication has no consent gate or consent persistence', () => {
  const page = read('user-web/src/Page.vue')
  assert.doesNotMatch(page, /termsVersions|authTerms|policiesAgreed|joinTermsPrompt|telegramRegistrationPrompt/)
  assert.doesNotMatch(read('backend/migrations/0001_baseline.up.sql'), /privacy_policy_version|service_terms_version|terms_accepted|accepted_at|member_consent_consistent/)
})

test('the removed user preview does not leave obsolete sandbox translations', () => {
  assert.doesNotMatch(read('shared/src/configured-rule-catalog.ts'), /playerHint:|Configurable rule sandbox|可配置规则试验区/)
})
