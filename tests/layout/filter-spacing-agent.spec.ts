import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'
import { origins, expectNoPageOverflow } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) test(`agent member filters retain panel spacing (${locale})`, async ({ page }, info) => {
  const reads: URL[] = [], errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.route('**/api/v1/agent/members?*', route => {
    expect(route.request().method()).toBe('GET')
    const url = new URL(route.request().url()); reads.push(url)
    return route.fulfill({ json: { success: true, data: { items: [], total_count: '0', limit: 20, offset: 0, summary: { member_count: '0', available_points: '0', bet_count: '0', stake_points: '0', unsettled_points: '0', net_points: '0' } } } })
  })
  await page.route('**/__filter-spacing-agent', route => route.fulfill({ contentType: 'text/html', body: `<!doctype html><html lang="${locale}"><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><body><main id="app" class="management-console" style="padding:12px"></main><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import Members from'/src/pages/MembersPage.vue';import{i18n}from'/src/i18n.ts';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/controls.css')}';i18n.setLocale('${locale}');createApp(Members).mount('#app')</script></body></html>` }))
  await page.goto(origins.agent + '/__filter-spacing-agent')
  const search = page.getByRole('textbox', { name: locale === 'en' ? 'Account' : '账号', exact: true })
  await expect(search).toBeVisible()
  const preset = page.viewportSize()!
  for (const width of info.project.name.endsWith('reflow') ? [320, 640, 900, 1440] : [preset.width]) {
    if (info.project.name.endsWith('reflow')) await page.setViewportSize({ width, height: preset.height })
    await expectNoPageOverflow(page)
    await page.screenshot({ path: info.outputPath(`agent-filters-${locale}-${width}.png`) })
  }
  await search.fill('filter_reviewer')
  await page.locator('.ui-filter-bar button[type=submit]').click()
  await expect.poll(() => reads.at(-1)?.searchParams.get('q')).toBe('filter_reviewer')
  expect(errors).toEqual([])
})
