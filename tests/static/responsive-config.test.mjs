import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const config = readFileSync(new URL('../../playwright.responsive.config.ts', import.meta.url), 'utf8')

const shell = readFileSync(new URL('../../admin-web/src/App.vue', import.meta.url), 'utf8')
const scripts = JSON.parse(readFileSync(new URL('../../package.json', import.meta.url), 'utf8')).scripts

test('intermediate viewport authentication keeps actual viewports', () => {
  assert.match(config, /testMatch: \['auth\.spec\.ts'\]/)
  assert.match(config, /width: 768, height: 1024/)
  assert.match(config, /width: 1024, height: 768/)
  assert.match(config, /fullyParallel: false/)
  assert.match(config, /workers: 1/)
  assert.match(config, /retries: 0/)
  assert.match(config, /reuseExistingServer: false/)
  assert.match(config, /isAbsolute\(executablePath\)/)
  assert.match(config, /existsSync\(executablePath\)/)
  assert.equal(scripts['test:responsive'], 'playwright test --config playwright.responsive.config.ts')
  assert.match(shell, /class="nav-item"\s+:aria-label="navLabel\(item.name\)"\s+:title="navLabel\(item.name\)"/)
})
