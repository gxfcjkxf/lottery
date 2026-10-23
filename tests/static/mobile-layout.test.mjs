import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { devices } from '@playwright/test'
const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8')

test('mobile suites retain device viewports instead of a universal 360×800', () => {
  for (const file of ['playwright.config.ts', 'playwright.qualification.config.ts', 'playwright.review.config.ts', 'playwright.pwa.config.ts']) {
    const source = read(file)
    assert.match(source, /devices\[['"]Pixel 7['"]\]/)
    assert.match(source, /contextOptions:\s*\{\s*screen:\s*devices\[['"]Pixel 7['"]\]\.screen/)
    assert.doesNotMatch(source, /width:\s*360,\s*height:\s*800/)
  }
  const webkit = read('playwright.webkit.config.ts')
  assert.match(webkit, /contextOptions: \{ screen: devices\['iPhone 13'\]\.screen \}/)
  assert.doesNotMatch(webkit, /width:\s*360,\s*height:\s*800/)
  assert.notDeepEqual(devices['Pixel 7'].viewport, devices['iPhone 13'].viewport)
  assert.notDeepEqual(devices['iPhone 13'].viewport, devices['iPhone 13'].screen)
})

test('layout acceptance covers both engines, device orientations and explicit reflow separately', () => {
  const config = read('playwright.layout.config.ts')
  for (const device of ['Pixel 7', 'Pixel 7 landscape', 'iPhone SE', 'iPhone 13', 'iPhone 13 Pro Max', 'iPhone 13 landscape']) assert.ok(config.includes(`devices['${device}']`))
  for (const engine of ['chromium', 'webkit']) assert.ok(config.includes(`browserName: '${engine}'`))
  assert.match(config, /retries: 0/)
  assert.equal((config.match(/reuseExistingServer: false/g) ?? []).length, 4)
  assert.doesNotMatch(config, /viewport:|test\.skip|DATABASE_URL/)
  const workflow = read('.github/workflows/ci.yaml').split('  mobile-layout:\n')[1].split('  webkit-functional:\n')[0]
  assert.match(workflow, /prepare-ci-browser.mjs/)
  assert.match(workflow, /install --with-deps webkit/)
  assert.match(workflow, /pnpm test:layout/)
  assert.match(workflow, /if: always\(\)/)
  assert.doesNotMatch(workflow, /workflow_dispatch|continue-on-error/)
  assert.ok(JSON.parse(read('package.json')).scripts['test:layout'])
})

test('all four authentication entries share the layout and credential fields', () => {
  for (const [surface, component] of [['user-web', 'Page.vue'], ['admin-web', 'App.vue'], ['platform-web', 'App.vue'], ['agent-web', 'App.vue']]) {
    const source = read(`${surface}/src/${component}`)
    assert.match(source, /import LoginEntry from '@lottery\/shared\/LoginEntry.vue'/)
    assert.match(source, /import LoginCredentials from '@lottery\/shared\/LoginCredentials.vue'/)
    assert.match(source, /<LoginEntry\b/)
    assert.match(source, /<LoginCredentials\b/)
    assert.match(read(`${surface}/src/main.ts`), /@lottery\/shared\/login-entry.css/)
    if (surface === 'agent-web') assert.doesNotMatch(read(`${surface}/src/main.ts`), /import\s+['"]\.\/[^'"]+\.css['"]/)
    else assert.doesNotMatch(read(`${surface}/src/style.css`), /\.(?:login-entry(?:__[\w-]+)?|login-shell|login-card|auth-layout|auth-form-wrap)\s*\{/)
  }
})
