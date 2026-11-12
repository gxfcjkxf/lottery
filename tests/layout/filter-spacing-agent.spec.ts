import { test, expect } from '@playwright/test'
import { origins, gameId, installLayoutApi, expectNoPageOverflow } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) test(`agent member filters retain panel spacing (${locale})`, async ({ page }, info) => {
  const reads: URL[] = [], errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await installLayoutApi(page, locale)
  await page.route('**/api/v1/agent/me', route => route.fulfill({json:{success:true,data:{account:{agent_account_id:gameId,username:'filter_reviewer'},agent:{id:gameId,brand_id:gameId,depth:1,config:{can_create_children:false}},code:null,user_origin:origins.user,brand_code:''}}}))
  await page.route('**/api/v1/agent/members?*', route => {
    expect(route.request().method()).toBe('GET')
    const url = new URL(route.request().url()); reads.push(url)
    return route.fulfill({ json: { success: true, data: { items: [], total_count: '0', limit: 20, offset: 0, summary: { member_count: '0', available_points: '0', bet_count: '0', stake_points: '0', unsettled_points: '0', net_points: '0' } } } })
  })
  await page.goto(origins.agent + '/#/members')
  const search = page.getByRole('searchbox', { name: locale === 'en' ? 'Account' : '账号', exact: true })
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
