import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const config = readFileSync(new URL('../../playwright.webkit.config.ts', import.meta.url), 'utf8')
const scripts = JSON.parse(readFileSync(new URL('../../package.json', import.meta.url), 'utf8')).scripts

test('WebKit functional suite requires its own executable and explicit test credentials', () => {
  assert.match(config, /PLAYWRIGHT_WEBKIT_EXECUTABLE_PATH/)
  assert.match(config, /isAbsolute\(executablePath\)/)
  assert.match(config, /existsSync\(executablePath\)/)
  assert.match(config, /browserName: 'webkit'/)
  assert.doesNotMatch(config, /PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH|channel: 'chrome'/)
  for (const role of ['ADMIN', 'HARBOR_ADMIN', 'COMPLIANCE_ADMIN']) {
    for (const field of ['USERNAME', 'PASSWORD']) assert.ok(config.includes(`TEST_${role}_${field}`))
  }
  assert.match(config, /if \(!process\.env\[name\]\) throw/)
  assert.equal(scripts['test:webkit'], 'playwright test --config playwright.webkit.config.ts')
})

test('WebKit functional scope is explicit, serial, and separate from service-worker validation', () => {
  assert.match(config, /testMatch: \['auth\.spec\.ts', 'management\.spec\.ts', 'compliance\.spec\.ts'\]/)
  assert.match(config, /serviceWorkers: 'block'/)
  assert.match(config, /fullyParallel: false/)
  assert.match(config, /workers: 1/)
  assert.match(config, /retries: 0/)
  assert.match(config, /width: 1440, height: 900/)
  assert.match(config, /width: 360, height: 800/)
  assert.equal((config.match(/reuseExistingServer: false/g) ?? []).length, 3)
})
