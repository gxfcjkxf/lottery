import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { terminologyIssues } from '../ui-terminology-contract.mjs'
function files(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => entry.isDirectory() ? files(join(dir, entry.name)) : /\.(ts|vue)$/.test(entry.name) && !/\.(?:test|spec)\./.test(entry.name) ? [join(dir, entry.name)] : [])
}
test('four apps and shared UI follow settings terminology and platform-only timezone display', () => {
  const issues = []
  for (const root of ['platform-web/src', 'admin-web/src', 'agent-web/src', 'user-web/src', 'shared/src']) {
    for (const file of files(root)) issues.push(...terminologyIssues(file, readFileSync(file, 'utf8')))
  }
  assert.deepEqual(issues, [])
})
test('rejects escaped, multiline and template copy, plus independent timezone controls', () => {
  for (const source of [
    String.raw`const copy = { title: '\u7b56\u7565' }`,
    'const copy = {title: `Platform\ntime zone`}',
    'const copy = {title: "UTC+08:00"}',
    'const copy = {title: "GMT"}',
    'const copy = {title: "Asia/Singapore"}',
    'const copy = {title: "+08:00"}',
    'const copy = {title: "Retention policy"}',
    'const copy = {title: "Data retention strategy"}',
  ]) assert.ok(terminologyIssues('shared/src/catalog.ts', source).length, source)
  for (const source of [
    '<template><p>策略</p></template>',
    '<script setup>const copy = {title: "UTC"}</script>',
    '<template><input placeholder="时区" /></template>',
    '<template><p>{{ "UTC" }}</p></template>',
    '<template><input :title="\'GMT\'" /></template>',
    '<template><p>{{ policy.timezone }}</p></template>',
    '<template><TimeZoneSelect /></template>',
    '<script setup>import Zone from "@lottery/shared/TimeZoneSelect.vue"</script>',
  ]) assert.ok(terminologyIssues('agent-web/src/Example.vue', source).length, source)
})
test('allows precise platform settings keys, legal privacy copy and internal time calculations', () => {
  assert.deepEqual(terminologyIssues('platform-web/src/i18n.ts', `const catalog = {'common.timeZone':'时区','common.timeZoneUtc':'UTC'}`), [])
  assert.ok(terminologyIssues('platform-web/src/i18n.ts', `const catalog = {other:'UTC'}`).length)
  assert.ok(terminologyIssues('platform-web/src/i18n.ts', `const catalog = {'common.timeZone':'策略'}`).length)
  assert.deepEqual(terminologyIssues('user-web/src/i18n.ts', `const catalog = {'common.privacyPolicy':'Privacy Policy'}`), [])
  assert.deepEqual(terminologyIssues('shared/src/date-time.ts', `const zone = 'UTC'; const date = Date.UTC(2026, 1, 1)`), [])
  assert.deepEqual(terminologyIssues('shared/src/catalog.ts', `const copy = {'filter.fromDateUTC':'Start date', 'retention.policy':'Archive settings'}`), [])
  assert.deepEqual(terminologyIssues('platform-web/src/PlatformSettingsPanel.vue', '<template><TimeZoneSelect />{{ current.timezone }}</template>'), [])
})

test('push and pull-request CI run the terminology check before other frontend checks', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/ci.yaml', import.meta.url), 'utf8')
  const manifest = JSON.parse(readFileSync(new URL('../../package.json', import.meta.url), 'utf8'))
  assert.equal(manifest.scripts['test:ui-terminology'], 'node --test tests/static/ui-terminology-contract.test.mjs')
  const triggers = workflow.slice(workflow.indexOf('on:'), workflow.indexOf('concurrency:'))
  assert.match(triggers, /^  push:\s*$/m)
  assert.match(triggers, /^  pull_request:\s*$/m)
  assert.doesNotMatch(triggers, /paths(?:-ignore)?:|branches(?:-ignore)?:/)
  const frontend = workflow.match(/^  frontend:\n([\s\S]*?)(?=^  [\w-]+:|(?![\s\S]))/m)?.[1]
  assert.ok(frontend, 'frontend job must exist')
  assert.doesNotMatch(frontend, /^    (?:if|continue-on-error|needs):/m)
  assert.match(frontend, /- name: Check UI terminology\n        run: pnpm test:ui-terminology\n      - run: pnpm typecheck/)
  assert.doesNotMatch(frontend, /continue-on-error:|pnpm test:ui-terminology[^\n]*(?:\|\||;)/)
})
