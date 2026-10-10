import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const config = readFileSync(new URL('../../playwright.responsive.config.ts', import.meta.url), 'utf8')
const betting = readFileSync(new URL('../browser/betting.spec.ts', import.meta.url), 'utf8')
const shell = readFileSync(new URL('../../admin-web/src/App.vue', import.meta.url), 'utf8')
const scripts = JSON.parse(readFileSync(new URL('../../package.json', import.meta.url), 'utf8')).scripts

test('intermediate viewport acceptance reuses real authentication and full betting flows', () => {
  assert.match(config, /testMatch: \['auth\.spec\.ts', 'betting\.spec\.ts'\]/)
  assert.match(config, /width: 768, height: 1024/)
  assert.match(config, /width: 1024, height: 768/)
  assert.match(config, /fullyParallel: false/)
  assert.match(config, /workers: 1/)
  assert.match(config, /retries: 0/)
  assert.match(config, /reuseExistingServer: false/)
  assert.match(config, /isAbsolute\(executablePath\)/)
  assert.match(config, /existsSync\(executablePath\)/)
  assert.doesNotMatch(betting, /info.project.name === "mobile"/)
  assert.equal((betting.match(/adminPage.viewportSize\(\)!\.width <= 700/g) ?? []).length, 2)
  assert.equal(scripts['test:responsive'], 'playwright test --config playwright.responsive.config.ts')
  assert.match(shell, /class="nav-item"\s+:aria-label="ui\(item.name\)"\s+:title="ui\(item.name\)"/)
})
