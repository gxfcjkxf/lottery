import { test, expect, type BrowserContext, type Page, type TestInfo } from '@playwright/test'
import { rememberAdminSession, restoreAdminSession } from './support/admin-session'

const origin = process.env.TEST_ADMIN_ORIGIN ?? 'http://localhost:5174'
const brand = '0199a000-0000-7000-8000-000000000001'
const username = process.env.TEST_ADMIN_USERNAME
const password = process.env.TEST_ADMIN_PASSWORD
const admin = `${origin}/api/v1/admin`
const headers = { 'X-Brand-ID': brand }

test.beforeEach(() => test.skip(!username || !password, 'Provide isolated test administrator credentials'))
async function language(page: Page, value: 'en' | 'zh-CN') {
  await page.getByTestId('admin-language').selectOption(value)
  await expect(page.locator('html')).toHaveAttribute('lang', value)
}
async function navigate(page: Page, info: TestInfo, destination: 'funds' | 'reconciliation') {
  if (info.project.name === 'mobile') {
    if (destination === 'funds') await page.locator('.mobile-nav button').nth(3).click()
    else {
      await page.locator('.mobile-nav button').last().click()
      await page.locator('.mobile-more-menu').getByRole('button', { name: /Bulk reconciliation|批量对账/ }).click()
    }
  } else await page.locator('.side-nav').getByRole('button', { name: destination === 'funds' ? /Funds.*ledger|资金与账本/ : /Bulk reconciliation|批量对账/ }).click()
}
async function login(page: Page, context: BrowserContext, info: TestInfo) {
  const restored = await restoreAdminSession(context, username!, brand, origin)
  await page.goto(origin)
  await language(page, 'en')
  if (!restored) {
    if (info.project.name === 'mobile') await page.locator('.mobile-nav button').nth(1).click()
    else await page.locator('.side-nav').getByRole('button', { name: /Users and members/ }).click()
    await page.getByLabel('Account', { exact: true }).fill(username!)
    await page.getByLabel('Password', { exact: true }).fill(password!)
    await page.getByRole('button', { name: 'Sign in and load members', exact: true }).click()
  }
  await page.locator('.directory-brand-bar select').selectOption(brand)
  rememberAdminSession(username!, await context.cookies(), origin)
}
async function data(response: Awaited<ReturnType<Page['request']['get']>>, status = 200) {
  expect(response.status()).toBe(status)
  const envelope = await response.json()
  expect(envelope.success).toBe(true)
  return envelope.data
}
async function member(page: Page) {
  const result = await data(await page.request.post(`${admin}/users`, {
    headers: { ...headers, Origin: origin, 'Idempotency-Key': crypto.randomUUID() },
    data: { username: `langcash_${crypto.randomUUID().replaceAll('-', '').slice(0, 12)}`, password: 'synthetic-member-only-password-2026', reason: 'isolated multilingual finance fixture' },
  }), 201)
  return result.member_id as string
}
async function wallet(page: Page, id: string) { return data(await page.request.get(`${admin}/wallets/${id}`, { headers })) }
async function fingerprint(page: Page, id: string) {
  return JSON.stringify({ wallet: await wallet(page, id), ledger: await data(await page.request.get(`${admin}/wallets/${id}/ledger?limit=50&offset=0`, { headers })) })
}
async function fits(page: Page) {
  const requested = page.viewportSize()!
  const viewport = await page.evaluate(() => ({ visibleWidth: visualViewport?.width ?? document.documentElement.clientWidth, width: document.documentElement.clientWidth, scrollWidth: document.documentElement.scrollWidth }))
  expect(viewport.visibleWidth).toBe(requested.width)
  expect(viewport.scrollWidth).toBeLessThanOrEqual(viewport.width + 1)
}

test('English recharge and original-source freeze reversal keep amounts and drafts intact across language changes', async ({ page, context }, info) => {
  test.setTimeout(60_000)
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await login(page, context, info)
  const id = await member(page)
  await navigate(page, info, 'funds')
  const finance = page.locator('.finance-management')
  await finance.getByLabel('Member ID', { exact: true }).fill(id)
  await finance.getByRole('button', { name: 'Load wallet', exact: true }).click()
  await expect(finance.locator('.wallet-panel')).toBeVisible()
  const recharge = finance.locator('.form-panel').filter({ has: page.getByRole('heading', { name: /^(?:Create recharge|创建充值单)$/ }) })
  await recharge.getByLabel(/Recharge points/).fill('100')
  await recharge.getByLabel('Remark (optional)', { exact: true }).fill('充值 中文备注 unchanged')
  await recharge.getByLabel('Reason', { exact: true }).fill('中文充值原因')
  const sent = page.waitForRequest(request => request.method() === 'POST' && request.url().endsWith('/api/v1/admin/recharges'))
  await language(page, 'zh-CN')
  await expect(recharge.getByLabel(/充值积分/)).toHaveValue('100')
  await expect(recharge.getByLabel('备注（可选）', { exact: true })).toHaveValue('充值 中文备注 unchanged')
  await language(page, 'en')
  await recharge.getByRole('button', { name: 'Create pending recharge', exact: true }).click()
  expect((await sent).postDataJSON()).toMatchObject({ member_id: id, points: '100', remark: '充值 中文备注 unchanged', reason: '中文充值原因' })
  const order = finance.locator('.recharge-row').first()
  await expect(order).toContainText('Pending')
  await expect(order).toContainText('充值 中文备注 unchanged')
  expect((await wallet(page, id)).available_points).toBe('0')
  await order.getByLabel('Confirmation reason', { exact: true }).fill('中文确认原凭证')
  await order.getByRole('button', { name: 'Confirm and credit', exact: true }).click()
  await expect(order).toContainText('Confirmed')
  expect((await wallet(page, id)).by_source.recharge.available).toBe('100')
  const freeze = finance.locator('form').filter({ has: page.getByRole('heading', { name: /^(?:Manual freeze|人工冻结)$/ }) })
  await freeze.getByLabel(/^Points to freeze/).fill('60')
  await freeze.getByLabel('Reason', { exact: true }).fill('中文冻结原因')
  await language(page, 'zh-CN')
  await expect(freeze.getByLabel(/冻结积分/)).toHaveValue('60')
  await language(page, 'en')
  await freeze.getByRole('button', { name: 'Freeze points', exact: true }).click()
  await expect(finance.getByRole('status')).toContainText('freeze request succeeded')
  const frozen = await wallet(page, id)
  expect(frozen.by_source.recharge.available).toBe('40')
  expect(frozen.by_source.recharge.manual_frozen).toBe('60')
  expect(frozen.display_points).toBe('100')
  const unfreeze = finance.locator('.unfreeze-row').first()
  await unfreeze.getByLabel('Reason for full reversal unfreeze', { exact: true }).fill('中文原路解冻原因')
  await unfreeze.getByRole('button', { name: 'Unfreeze entire original amount', exact: true }).click()
  await expect(finance.getByRole('status')).toContainText('unfreeze request succeeded')
  const after = await wallet(page, id)
  expect(after.by_source.recharge.available).toBe('100')
  expect(after.by_source.recharge.manual_frozen).toBe('0')
  expect(after.gift_points).toBe('0')
  expect(after.winning_points).toBe('0')
  const immutable = await fingerprint(page, id)
  await language(page, 'zh-CN')
  await language(page, 'en')
  expect(await fingerprint(page, id)).toBe(immutable)
  const repair = page.locator('.balance-repair')
  await repair.getByLabel('Member UUID for inspection', { exact: true }).fill(id)
  await repair.getByRole('button', { name: 'Inspect balance and repair history', exact: true }).click()
  await expect(repair).toContainText('Ledger and balance match; no repair is needed')
  await expect(repair.getByRole('button', { name: 'Confirm repair from original ledger' })).toHaveCount(0)
  await fits(page)
  await finance.locator('.wallet-panel').screenshot({ path: info.outputPath('english-wallet.png') })
  expect(errors).toEqual([])
})

test('large integer policy values and withdrawal-config drafts remain language-independent', async ({ page, context }, info) => {
  test.setTimeout(60_000)
  await login(page, context, info)
  await navigate(page, info, 'funds')
  const policy = page.locator('.point-policy-settings')
  await expect(policy.getByRole('heading', { name: 'Current policy', exact: true })).toBeVisible()
  await policy.getByLabel(/^Balance limit/).fill('9007199254740993')
  await policy.getByLabel(/^Per-recharge limit/).fill('10000')
  await policy.getByLabel(/^Per-adjustment limit/).fill('1000')
  await policy.getByLabel(/Reason for change/).fill('中文限额原因')
  await language(page, 'zh-CN')
  await expect(policy.getByLabel(/^余额上限/)).toHaveValue('9007199254740993')
  await language(page, 'en')
  const sent = page.waitForRequest(request => request.method() === 'PUT' && request.url().endsWith('/api/v1/admin/point-policy'))
  await policy.getByRole('button', { name: 'Save policy', exact: true }).click()
  expect((await sent).postDataJSON()).toMatchObject({ max_balance_points: '9007199254740993', max_recharge_points: '10000', max_adjustment_points: '1000', reason: '中文限额原因' })
  await expect(policy.getByRole('status')).toContainText('saved and applied immediately')
  await language(page, 'zh-CN')
  await expect(policy.getByRole('status')).toContainText('积分策略已保存')
  await language(page, 'en')
  await policy.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(policy.getByLabel(/^Balance limit/)).toHaveValue('9007199254740993')
  const withdrawal = page.getByTestId('withdrawal-policy-settings')
  await expect(withdrawal.locator('#withdraw-brand-min')).toBeVisible()
  await withdrawal.locator('#withdraw-brand-min').fill('17')
  await withdrawal.locator('#withdraw-brand-multiple').fill('3')
  await withdrawal.locator('#withdraw-brand-review').selectOption('automatic')
  const before = await data(await page.request.get(`${admin}/withdrawal-policy`, { headers }))
  const writes: string[] = []
  page.on('request', request => { if (request.method() !== 'GET' && request.url().includes('/api/')) writes.push(request.url()) })
  await language(page, 'zh-CN')
  await expect(withdrawal.locator('#withdraw-brand-min')).toHaveValue('17')
  await expect(withdrawal.locator('#withdraw-brand-multiple')).toHaveValue('3')
  await expect(withdrawal.locator('#withdraw-brand-review')).toHaveValue('automatic')
  await language(page, 'en')
  expect(await data(await page.request.get(`${admin}/withdrawal-policy`, { headers }))).toEqual(before)
  expect(writes).toEqual([])
  await fits(page)
  await policy.screenshot({ path: info.outputPath('english-points-policy.png') })
})

test('unknown reconciliation creation retains the original body and key across language changes', async ({ page, context }, info) => {
  test.setTimeout(60_000)
  await login(page, context, info)
  const id = await member(page)
  const before = await fingerprint(page, id)
  await navigate(page, info, 'reconciliation')
  const panel = page.locator('.recon')
  await panel.getByLabel('Reason for operation', { exact: true }).fill('中文对账原因 原文不变')
  let count = 0
  const attempts: { body: string | null; key: string | undefined }[] = []
  await page.route('**/api/v1/admin/reconciliations', async route => {
    if (route.request().method() !== 'POST') return route.continue()
    attempts.push({ body: route.request().postData(), key: route.request().headers()['idempotency-key'] })
    const response = await route.fetch()
    expect(response.status()).toBe(201)
    if (count++ === 0) await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ success: true, data: null }) })
    else await route.fulfill({ response })
  })
  const submitButton = panel.getByRole('button', { name: 'Review and confirm submission', exact: true })
  await submitButton.scrollIntoViewIfNeeded()
  const pointerSafe = await submitButton.evaluate(element => {
    const rect = element.getBoundingClientRect()
    const x = rect.x + rect.width / 2, y = rect.y + rect.height / 2
    const hit = document.elementFromPoint(x, y)
    return hit !== null && element.contains(hit) && x >= 0 && x <= document.documentElement.clientWidth && y >= 0 && y <= (visualViewport?.height ?? innerHeight)
  })
  expect(pointerSafe).toBe(true)
  await submitButton.click()
  const confirm = page.getByRole('dialog')
  await confirm.getByRole('checkbox').check()
  await confirm.getByRole('button', { name: 'Confirm and submit', exact: true }).click()
  await expect(panel.locator('.pending-panel')).toContainText('Unconfirmed writes')
  await expect(panel.locator('.write-error')).toContainText('The write result is unknown')
  await expect(panel.locator('.pending-panel .tag')).toHaveText('Unknown outcome / replay required')
  await fits(page)
  await panel.locator('.pending-panel').screenshot({ path: info.outputPath('english-unknown-intent.png') })
  await language(page, 'zh-CN')
  await expect(panel.locator('.pending-panel')).toContainText('尚未确定的写入')
  await language(page, 'en')
  await panel.getByRole('button', { name: 'Refresh jobs', exact: true }).click()
  await expect(panel.locator('.pending-panel')).toBeVisible()
  expect(attempts).toHaveLength(1)
  await panel.getByRole('button', { name: 'Confirm and replay original creation', exact: true }).click()
  await confirm.getByRole('checkbox').check()
  await confirm.getByRole('button', { name: 'Confirm and submit', exact: true }).click()
  await expect(panel.locator('.receipt-panel')).toContainText('Server-confirmed request receipt')
  expect(attempts).toHaveLength(2)
  expect(attempts[1]).toEqual(attempts[0])
  expect(JSON.parse(attempts[0].body!)).toEqual({ reason: '中文对账原因 原文不变' })
  await expect(panel.locator('.pending-panel')).toHaveCount(0)
  expect(await fingerprint(page, id)).toBe(before)
  await fits(page)
  await panel.locator('.target-card').first().screenshot({ path: info.outputPath('english-target-result.png') })
  await panel.screenshot({ path: info.outputPath('english-reconciliation.png') })
})
