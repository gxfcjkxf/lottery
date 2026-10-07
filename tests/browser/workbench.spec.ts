import { test, expect } from '@playwright/test'

const origin = process.env.TEST_ADMIN_ORIGIN ?? 'http://localhost:5174'
const brand = '0199a000-0000-7000-8000-000000000001'
const username = process.env.TEST_ADMIN_USERNAME
const password = process.env.TEST_ADMIN_PASSWORD

test('workbench displays real scoped financial data, explicit gaps, and clears stale data on failed refresh', async ({ page }, info) => {
  test.skip(!username || !password, 'Provide isolated test administrator credentials')
  test.setTimeout(60_000)
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.goto(origin)
  await page.getByTestId('admin-language').selectOption('en')
  if (info.project.name === 'mobile') await page.locator('.mobile-nav button').nth(1).click()
  else await page.locator('.side-nav').getByRole('button', { name: /Users and members/ }).click()
  await page.getByLabel('Account', { exact: true }).fill(username!)
  await page.getByLabel('Password', { exact: true }).fill(password!)
  await page.getByRole('button', { name: 'Sign in and load members', exact: true }).click()
  await page.locator('.directory-brand-bar select').selectOption(brand)
  const live = await page.request.get(`${origin}/api/v1/admin/workbench`, { headers: { 'X-Brand-ID': brand } })
  expect(live.status()).toBe(200)
  const { data } = await live.json()
  if (info.project.name === 'mobile') await page.locator('.mobile-nav button').first().click()
  else await page.locator('.side-nav').getByRole('button', { name: /Dashboard/ }).click()
  const dashboard = page.locator('.workbench')
  await expect(dashboard.getByRole('heading', { name: 'Operations workbench', exact: true })).toBeVisible()
  const card = (heading: string) => dashboard.locator('.card').filter({ has: page.getByRole('heading', { name: heading, exact: true }) })
  const metric = (heading: string, label: string) => card(heading).locator('.metrics > div').filter({ has: page.locator('dt').filter({ hasText: new RegExp(`^${label}$`) }) }).locator('dd')
  await expect(metric('Balances', 'Total points')).toHaveText(data.balances.data.total_points)
  await expect(metric('Balances', 'Available points')).toHaveText(data.balances.data.available_points)
  await expect(metric('Ledger', 'Net points')).toHaveText(data.ledger.data.net_points)
  const withdrawals = data.withdrawals
  await expect(card('Withdrawals').locator('.badge')).toHaveText(withdrawals.status === 'ready' ? 'Ready' : 'Unavailable')
  if (withdrawals.status === 'ready') {
    await expect(metric('Withdrawals', 'Awaiting review')).toHaveText(withdrawals.data.reviewing_count)
    await expect(metric('Withdrawals', 'Awaiting review (points)')).toHaveText(withdrawals.data.reviewing_points)
    await expect(metric('Withdrawals', 'Processing count')).toHaveText(withdrawals.data.processing_count)
    await expect(metric('Withdrawals', 'Processing points')).toHaveText(withdrawals.data.processing_points)
    await expect(card('Withdrawals')).toContainText('does not indicate eligibility, actual payment, or profit')
  } else {
    expect(withdrawals.status).toBe('forbidden')
    expect(withdrawals.data).toBeNull()
    await expect(card('Withdrawals')).toContainText('Your account cannot view')
    await expect(card('Withdrawals').locator('dd')).toHaveCount(0)
  }
  for (const name of ['Commissions', 'Rewards']) {
    expect(data[name.toLowerCase()]).toEqual({ status: 'not_implemented', data: null })
    await expect(card(name).locator('.badge')).toHaveText('Not implemented')
    await expect(card(name).locator('dd')).toHaveCount(0)
  }
  await expect(card('Draw sources')).toContainText(data.sources.status === 'forbidden' ? 'Your account cannot view' : 'does not indicate upstream health')
  await expect(dashboard).not.toContainText('Lin Lan')
  await expect(dashboard).not.toContainText('128,450')
  await page.getByTestId('admin-language').selectOption('zh-CN')
  await expect(dashboard.getByRole('heading', { name: '运营工作台', exact: true })).toBeVisible()
  await page.getByTestId('admin-language').selectOption('en')
  await expect(metric('Balances', 'Total points')).toHaveText(data.balances.data.total_points)
  const dimensions = await page.evaluate(() => ({ width: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth, visible: visualViewport?.width }))
  expect(dimensions.scroll).toBeLessThanOrEqual(dimensions.width + 1)
  expect(dimensions.visible).toBe(page.viewportSize()!.width)
  await dashboard.screenshot({ path: info.outputPath('workbench-live.png') })
  await page.route('**/api/v1/admin/workbench', route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ success: false, error: { code: 'WORKBENCH_UNAVAILABLE', message: 'Injected unavailable' } }) }))
  await dashboard.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(dashboard.getByRole('alert')).toContainText('temporarily unavailable')
  await expect(dashboard.locator('.card')).toHaveCount(0)
  await page.unroute('**/api/v1/admin/workbench')
  await dashboard.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(metric('Balances', 'Total points')).toHaveText(data.balances.data.total_points)
  await card('Recharges').getByRole('button', { name: 'Open management page' }).click()
  await expect(page.locator('.finance-management')).toBeVisible()
  expect(errors).toEqual([])
})
