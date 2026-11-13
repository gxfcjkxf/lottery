import { test, expect } from '@playwright/test'
import { installLayoutApi, installWalletLayoutApi, installOrdersLayoutApi, origins, gameId, expectNoPageOverflow } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) test(`fractional member wallet (${locale})`, async ({ page }, info) => {
  await installLayoutApi(page, locale)
  await installWalletLayoutApi(page)
  const zero = { available: '0', manual_frozen: '0', system_frozen: '0', withdrawal: '0' }
  await page.route('**/api/v1/wallet', route => route.fulfill({ json: { success: true, data: {
    account_id: gameId, brand_id: gameId, member_id: gameId, version: 2,
    display_points: '1234.56', available_points: '1234.56', frozen_points: '0', withdrawal_points: '0',
    recharge_points: '1200', winning_points: '34.56', gift_points: '0', commission_points: '0', manual_frozen_points: '0', system_frozen_points: '0',
    by_source: { recharge: { ...zero, available: '1200' }, winning: { ...zero, available: '34.56' }, gift: zero, commission: zero },
  } } }))
  await page.goto(origins.user + '/wallet')
  await expect(page.locator('.balance-amount strong')).toHaveText('1,234.56')
  const preset = page.viewportSize()!
  for (const width of info.project.name.endsWith('reflow') ? [320, 620, 900, 1440] : [preset.width]) {
    if (info.project.name.endsWith('reflow')) await page.setViewportSize({ width, height: preset.height })
    await expectNoPageOverflow(page)
    await page.screenshot({ path: info.outputPath(`wallet-${width}.png`), fullPage: true })
  }
  await page.goto(origins.user + '/wallet/buckets')
  await expect(page.locator('.bucket-source').nth(1)).toContainText('34.56')
  await expectNoPageOverflow(page)
})
