import { test, expect } from '@playwright/test'
import { installLayoutApi, installWalletLayoutApi, origins, expectNoPageOverflow } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) test(`withdrawal route uses the currency wallet without legacy point requests (${locale})`, async ({ page }) => {
  await installLayoutApi(page, locale)
  await installWalletLayoutApi(page)
  const legacy: string[] = []
  page.on('request', request => {
    if (/\/api\/v1\/withdrawal(?:s|-[\w-]+)?(?:\?|$)/.test(request.url())) legacy.push(request.url())
  })
  await page.goto(origins.user + '/withdraw')
  await expect(page.locator('.currency-wallet')).toBeVisible()
  await expect(page.locator('.withdrawal-toolbar')).toHaveCount(0)
  await expectNoPageOverflow(page)
  expect(legacy).toEqual([])
})
