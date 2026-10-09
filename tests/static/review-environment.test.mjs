import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const rootPackage = JSON.parse(readFileSync(new URL('../../package.json', import.meta.url), 'utf8'))
const userReview = readFileSync(new URL('../../user-web/vite.review.config.ts', import.meta.url), 'utf8')
const adminReview = readFileSync(new URL('../../admin-web/vite.review.config.ts', import.meta.url), 'utf8')
const userDev = readFileSync(new URL('../../user-web/vite.config.ts', import.meta.url), 'utf8')
const adminDev = readFileSync(new URL('../../admin-web/vite.config.ts', import.meta.url), 'utf8')
const workflow = readFileSync(new URL('../../.github/workflows/ci.yaml', import.meta.url), 'utf8')
const reviewConfig = readFileSync(new URL('../../playwright.review.config.ts', import.meta.url), 'utf8')

test('review Vite configs extend normal configs and pin loopback ports and API proxy', () => {
  for (const [config, port] of [[userReview, 5183], [adminReview, 5184]]) {
    assert.match(config, /mergeConfig\(config,/)
    assert.match(config, /host: '127\.0\.0\.1'/)
    assert.match(config, new RegExp(`port: ${port}`))
    assert.match(config, /strictPort: true/)
    assert.match(config, /target: 'http:\/\/127\.0\.0\.1:8085'/)
    assert.match(config, /changeOrigin: false/)
    assert.doesNotMatch(config, /process\.env|import\.meta\.env|localhost|0\.0\.0\.0/)
  }
})

test('root review scripts select the matching review config while normal dev configs stay unchanged', () => {
  assert.equal(rootPackage.scripts['review:user'], 'pnpm --filter @lottery/user-web exec vite --config vite.review.config.ts')
  assert.equal(rootPackage.scripts['review:admin'], 'pnpm --filter @lottery/admin-web exec vite --config vite.review.config.ts')
  assert.match(userDev, /port: 5173/)
  assert.match(adminDev, /port: 5174/)
  assert.match(userDev, /target: 'http:\/\/localhost:8080'/)
  assert.match(adminDev, /target: 'http:\/\/localhost:8080'/)
  assert.doesNotMatch(userDev + adminDev, /5183|5184|127\.0\.0\.1:8085/)
})

test('isolated entry CI pins the installed Chromium before loading the strict review config', () => {
  const job = workflow.split('  administration-entries:')[1].split('  audit-exports-browser:')[0]
  assert.match(job, /chromium\.executablePath\(\)/)
  assert.match(job, /appendFileSync\(process\.env\.GITHUB_ENV/)
  assert.match(job, /PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=\$\{chromium\.executablePath\(\)\}/)
  assert.ok(job.indexOf('Pin the installed Chromium executable') < job.indexOf('Verify real isolated administrative entries'))
  assert.match(reviewConfig, /if \(!executablePath\)/)
  assert.match(reviewConfig, /launchOptions: \{ executablePath \}/)
})

test('complete review runs require brand creation credentials instead of silently skipping it', () => {
  for (const key of ['TEST_PLATFORM_ADMIN_USERNAME', 'TEST_PLATFORM_ADMIN_PASSWORD', 'TEST_PLATFORM_ADMIN_ORIGIN']) assert.ok(reviewConfig.includes(key))
  assert.match(reviewConfig, /brand creation must not be skipped/)
  assert.match(reviewConfig, /workers: 2/)
  assert.match(workflow, /TEST_PLATFORM_ADMIN_USERNAME: review_platform_\{project\}/)
  assert.match(workflow, /create-admin --username review_platform_desktop1440 --super/)
  assert.match(workflow, /create-admin --username review_platform_mobile360 --super/)
  assert.match(workflow, /create-admin --username review_operator_desktop1440 --brand aurora/)
  assert.match(workflow, /create-admin --username review_operator_mobile360 --brand aurora/)
})

test('read-only review panels share real worker sessions without disabling auth limits', () => {
  const fixture = readFileSync(new URL('../review/platform-fixture.ts', import.meta.url), 'utf8')
  assert.match(fixture, /scope: 'worker'/)
  assert.match(fixture, /platform\/auth\/login/)
  assert.match(fixture, /api\.storageState\(\)/)
  assert.match(fixture, /login\.status\(\).*\.toBe\(200\)/)
  assert.match(fixture, /review_operator_\$\{project\}/)
  assert.match(fixture, /expect\(account\.super_admin\)\.toBe\(false\)/)
  assert.doesNotMatch(fixture, /route\.fulfill|auth_rate_limits|DELETE FROM|X-Forwarded-For|retry/)
  for (const file of ['platform-bets', 'platform-read-pages', 'platform-rewards', 'platform-wallet', 'platform-withdrawals']) {
    const source = readFileSync(new URL(`../review/${file}.spec.ts`, import.meta.url), 'utf8')
    assert.match(source, /from '\.\/platform-fixture'/)
  }
  const authTests = readFileSync(new URL('../review/admin-entry.spec.ts', import.meta.url), 'utf8')
  assert.match(authTests, /from '@playwright\/test'/)
  assert.match(authTests, /Sign in/)
  for (const file of ['platform-lottery', 'platform-rules', 'platform-recharges']) {
    const source = readFileSync(new URL(`../review/${file}.spec.ts`, import.meta.url), 'utf8')
    assert.match(source, /newContext\(\{ storageState: operatorSession \}\)/)
    assert.doesNotMatch(source, /identifier: 'review_operator'|\/auth\/login/)
  }
})
