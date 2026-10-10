import { test, expect, type Page } from '@playwright/test'

const userOrigin = process.env.TEST_QUAL_USER_ORIGIN ?? 'http://localhost:5173'
const adminOrigin = process.env.TEST_QUAL_ADMIN_ORIGIN ?? 'http://localhost:5174'
const adminUsername = process.env.TEST_QUAL_ADMIN_USERNAME ?? 'qual_admin'
const adminPassword = process.env.TEST_QUAL_ADMIN_PASSWORD
const userPassword = process.env.TEST_QUAL_USER_PASSWORD
const brand = '0199a000-0000-7000-8000-000000000001'
type Envelope<T> = { success: boolean; data: T }

async function data<T>(response: Awaited<ReturnType<Page['request']['get']>>, status = 200): Promise<T> {
  expect(response.status()).toBe(status)
  const envelope = await response.json() as Envelope<T>
  expect(envelope.success).toBe(true)
  return envelope.data
}

async function adminAPI(page: Page) {
  const login = await page.request.post(adminOrigin + '/api/v1/admin/auth/login', {
    headers: { 'X-Brand-ID': brand, 'Idempotency-Key': crypto.randomUUID(), Origin: adminOrigin },
    data: { identifier: adminUsername, password: adminPassword },
  })
  const auth = await data<{ access_token: string }>(login)
  const headers = () => ({
    Authorization: 'Bearer ' + auth.access_token,
    'X-Brand-ID': brand,
    'Idempotency-Key': crypto.randomUUID(),
    Origin: adminOrigin,
  })
  return {
    post: (path: string, body: unknown) => page.request.post(adminOrigin + '/api/v1/admin' + path, {
      headers: headers(), data: body,
    }),
    get: (path: string) => page.request.get(adminOrigin + '/api/v1/admin' + path, { headers: headers() }),
  }
}

async function userLogin(page: Page, username: string) {
  await page.addInitScript(() => localStorage.setItem('luma-language', 'en'))
  await page.goto(userOrigin + '/login')
  await page.getByLabel('Username or phone', { exact: true }).fill(username)
  await page.getByLabel('Password', { exact: true }).fill(userPassword!)
  await page.locator('.auth-form').getByRole('button', { name: /Continue/ }).click()
  await expect(page).toHaveURL(/\/account$/)
}

test('real turnover qualification survives unknown intent and follows ledger changes', async ({ page }, info) => {
  test.skip(!adminPassword || !userPassword, 'Set credentials for the isolated synthetic qualification database')
  test.setTimeout(90_000)
  const username = info.project.name === 'mobile' ? 'qual_mobile_user' : 'qual_user'
  const runtimeErrors: string[] = []
  page.on('pageerror', error => runtimeErrors.push(error.message))
  await userLogin(page, username)
  const admin = await adminAPI(page)

  const initial = await data<Record<string, unknown>>(await page.request.get(userOrigin + '/api/v1/withdrawal-qualification'))
  expect(initial).toMatchObject({
    base_points: '119', valid_points: '1', valid_order_count: '1',
    credit_numerator: '1000000', credit_denominator: '1', meets_turnover: true,
  })
  expect(Object.keys(initial).sort()).toEqual([
    'account_id', 'base_points', 'brand_id', 'credit_denominator', 'credit_numerator', 'cutoff_at',
    'cutoff_version', 'cycle_from_at', 'cycle_from_version', 'member_id', 'meets_turnover',
    'valid_order_count', 'valid_points',
  ].sort())
  for (const field of ['order_snapshot_digest', 'rule_version_id', 'definition_snapshot', 'ledger_entry_id', 'source_allocation']) {
    expect(initial).not.toHaveProperty(field)
  }

  await page.goto(userOrigin + '/withdraw')
  const panel = page.locator('.withdrawal-page')
  await expect(panel.getByRole('heading', { name: 'Read-only turnover snapshot' })).toBeVisible()
  await expect(panel).toContainText('119 pts')
  await expect(panel).toContainText('1000000 / 1 pts')
  await expect(panel).toContainText('Turnover requirement met')
  for (const label of ['Recharge', 'Winnings', 'Gift']) await expect(panel.getByLabel(label, { exact: true })).toBeVisible()
  const initialBox = await panel.boundingBox()
  expect(initialBox).not.toBeNull()
  expect(initialBox!.x).toBeGreaterThanOrEqual(0)
  expect(initialBox!.x + initialBox!.width).toBeLessThanOrEqual(info.project.name === 'mobile' ? 360 : 1440)
  await panel.screenshot({ path: info.outputPath('withdrawal-qualification-initial.png') })

  await panel.getByLabel('Recharge', { exact: true }).fill('3')
  await panel.getByLabel('Winnings', { exact: true }).fill('2')
  await panel.getByLabel('Gift', { exact: true }).fill('4')
  const beforeOfflineWallet = await data<Record<string, unknown>>(await page.request.get(userOrigin + '/api/v1/wallet'))
  const beforeOfflineOrders = await data<{ items: Array<Record<string, unknown>> }>(await page.request.get(userOrigin + '/api/v1/withdrawals'))
  await page.context().setOffline(true)
  await panel.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(panel.getByRole('button', { name: 'Submit request', exact: true })).toHaveCount(0)
  await expect(panel.getByRole('alert')).toBeVisible()
  await page.context().setOffline(false)
  await panel.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(panel).toContainText('Turnover requirement met')
  await expect(panel.getByRole('button', { name: 'Submit request', exact: true })).toBeEnabled()
  const afterOfflineWallet = await data<Record<string, unknown>>(await page.request.get(userOrigin + '/api/v1/wallet'))
  const afterOfflineOrders = await data<{ items: Array<Record<string, unknown>> }>(await page.request.get(userOrigin + '/api/v1/withdrawals'))
  expect(afterOfflineWallet).toEqual(beforeOfflineWallet)
  expect(afterOfflineOrders).toEqual(beforeOfflineOrders)

  const intentKeys: string[] = []
  const intentBodies: unknown[] = []
  let committedReceipt: { id: string; reserve_entry_id: string; points: string } | undefined
  let firstReceipt: { id: string; reserve_entry_id: string; points: string } | undefined
  const routeURL = userOrigin + '/api/v1/withdrawals'
  await page.route(routeURL, async route => {
    if (route.request().method() !== 'POST') return route.continue()
    intentKeys.push(route.request().headers()['idempotency-key'])
    intentBodies.push(route.request().postDataJSON())
    const response = await route.fetch()
    expect(response.status()).toBe(201)
    const envelope = await response.json() as Envelope<typeof committedReceipt>
    expect(envelope.success).toBe(true)
    committedReceipt = envelope.data as NonNullable<typeof committedReceipt>
    if (intentKeys.length === 1) {
      firstReceipt = committedReceipt
      // The actual request commits; only its receipt is lost to the browser.
      await route.abort('failed')
      return
    }
    await route.fulfill({ response })
  })
  const body = {
    points: '9',
    source_allocation: [
      { source: 'recharge', state: 'available', points: '3' },
      { source: 'winning', state: 'available', points: '2' },
      { source: 'gift', state: 'available', points: '4' },
    ],
  }
  await panel.getByRole('button', { name: 'Submit request', exact: true }).click()
  await expect(panel).toContainText('The outcome is unknown')
  const replay = panel.getByRole('button', { name: 'Replay the same request', exact: true })
  await expect(replay).toBeVisible()
  await expect(panel.getByLabel('Recharge', { exact: true })).toBeDisabled()
  expect(committedReceipt?.id).toBeTruthy()
  expect(committedReceipt?.reserve_entry_id).toBeTruthy()
  expect(intentBodies[0]).toEqual(body)

  await panel.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(replay).toBeVisible()

  const memberID = initial.member_id as string
  const increase = await admin.post('/wallets/' + memberID + '/adjust', {
    source: 'recharge', delta: '2000000', reason: 'change current qualification independently of original intent',
  })
  await data(increase)
  await panel.getByRole('button', { name: 'Refresh', exact: true }).click()
  const changed = await data<Record<string, unknown>>(await page.request.get(userOrigin + '/api/v1/withdrawal-qualification'))
  expect(changed).toMatchObject({
    base_points: '2000112', valid_points: '1', valid_order_count: '1',
    credit_numerator: '1000000', meets_turnover: false,
  })
  await expect(panel).toContainText('Turnover requirement not met')
  await expect(replay).toBeVisible()

  await replay.click()
  await expect(panel).toContainText('Request submitted')
  await expect(replay).toHaveCount(0)
  expect(intentKeys).toHaveLength(2)
  expect(intentKeys[1]).toBe(intentKeys[0])
  expect(intentBodies[1]).toEqual(intentBodies[0])
  expect(committedReceipt).toMatchObject({ id: firstReceipt!.id, reserve_entry_id: firstReceipt!.reserve_entry_id })
  const firstOrders = await data<{ items: Array<Record<string, unknown>> }>(await page.request.get(userOrigin + '/api/v1/withdrawals'))
  expect(firstOrders.items).toHaveLength(1)
  expect(firstOrders.items[0]).toMatchObject({ id: firstReceipt!.id, reserve_entry_id: firstReceipt!.reserve_entry_id })
  expect(firstOrders.items[0]).not.toHaveProperty('eligibility_evidence')
  expect(firstOrders.items[0]).not.toHaveProperty('policy_snapshot')
  const firstLedger = await data<{ items: Array<{ id: string; entry_type: string; reference_type: string; reference_id: string }> }>(
    await admin.get('/wallets/' + memberID + '/ledger'),
  )
  const firstReserves = firstLedger.items.filter(entry => entry.entry_type === 'withdrawal_reserve'
    && entry.reference_type === 'withdrawal' && entry.reference_id === firstReceipt!.id)
  expect(firstReserves).toHaveLength(1)
  expect(firstReserves[0].id).toBe(firstReceipt!.reserve_entry_id)
  await page.unroute(routeURL)

  await data(await admin.post('/withdrawals/' + committedReceipt!.id + '/cancel', {
    version: 1, reason: 'cancel first synthetic request',
  }))
  await data(await admin.post('/wallets/' + memberID + '/adjust', {
    source: 'recharge', delta: '-2000000', reason: 'restore qualification fixture balance',
  }))
  await panel.getByRole('button', { name: 'Refresh', exact: true }).click()
  const qualifiedAgain = await data<Record<string, unknown>>(await page.request.get(userOrigin + '/api/v1/withdrawal-qualification'))
  expect(qualifiedAgain).toMatchObject({ base_points: '119', valid_points: '1', credit_numerator: '1000000', meets_turnover: true })
  await expect(panel).toContainText('Turnover requirement met')

  await panel.getByLabel('Recharge', { exact: true }).fill('3')
  await panel.getByLabel('Winnings', { exact: true }).fill('2')
  await panel.getByLabel('Gift', { exact: true }).fill('4')
  let releaseNewReceipt!: () => void
  let newCommitted = false
  const receiptGate = new Promise<void>(resolve => { releaseNewReceipt = resolve })
  await page.route(routeURL, async route => {
    if (route.request().method() !== 'POST') return route.continue()
    const response = await route.fetch()
    expect(response.status()).toBe(201)
    newCommitted = true
    await receiptGate
    await route.fulfill({ response })
  })
  const newSubmission = page.waitForResponse(response => response.url() === routeURL && response.request().method() === 'POST')
  await panel.getByRole('button', { name: 'Submit request', exact: true }).click()
  await expect.poll(() => newCommitted).toBe(true)
  try {
    await expect(panel.locator('.success-note')).toHaveCount(0)
  } finally {
    releaseNewReceipt()
  }
  const newResponse = await newSubmission
  const newReceipt = await data<{ id: string }>(newResponse, 201)
  expect(newReceipt.id).not.toBe(firstReceipt!.id)
  await page.unroute(routeURL)
  await expect(panel).toContainText('Request submitted')
  const orders = await data<{ items: Array<{ id: string; points: string; state: string; reserve_entry_id?: string }> }>(
    await page.request.get(userOrigin + '/api/v1/withdrawals'),
  )
  const reviewing = orders.items.filter(order => order.points === '9' && order.state === 'reviewing')
  expect(reviewing).toHaveLength(1)
  const payable = reviewing[0]
  expect(payable.reserve_entry_id).toBeTruthy()
  await data(await admin.post('/withdrawals/' + payable.id + '/approve', {
    version: 1, reason: 'approve synthetic qualification request',
  }))
  await data(await admin.post('/withdrawals/' + payable.id + '/mark-paid', {
    version: 2, reason: 'internal points completion; no external payment',
  }))

  await panel.getByRole('button', { name: 'Refresh', exact: true }).click()
  const wallet = await data<Record<string, unknown>>(await page.request.get(userOrigin + '/api/v1/wallet'))
  expect(wallet).toMatchObject({ recharge_points: '96', winning_points: '8', gift_points: '16', withdrawal_points: '0' })
  expect(wallet).not.toHaveProperty('ledger_entries')
  const finalQualification = await data<Record<string, unknown>>(await page.request.get(userOrigin + '/api/v1/withdrawal-qualification'))
  expect(finalQualification).toMatchObject({
    base_points: '112', valid_points: '0', valid_order_count: '0',
    credit_numerator: '0', credit_denominator: '1', meets_turnover: false,
  })
  const paidOrder = await data<Record<string, unknown>>(await page.request.get(userOrigin + '/api/v1/withdrawals/' + payable.id))
  expect(finalQualification.cycle_from_version).toBe(paidOrder.reserve_version)
  expect(Date.parse(finalQualification.cycle_from_at as string)).toBe(Date.parse(paidOrder.created_at as string))
  const cycleVersion = BigInt(finalQualification.cycle_from_version as string)
  const cutoffVersion = BigInt(finalQualification.cutoff_version as string)
  expect(cutoffVersion.toString()).toBe((BigInt(paidOrder.reserve_version as string) + 1n).toString())
  expect(cutoffVersion).toBeGreaterThan(cycleVersion)
  const paidLedger = await data<{ items: Array<{ id: string; entry_type: string; reference_type: string; reference_id: string }> }>(
    await admin.get('/wallets/' + memberID + '/ledger'),
  )
  const paidReserves = paidLedger.items.filter(entry => entry.entry_type === 'withdrawal_reserve'
    && entry.reference_type === 'withdrawal' && entry.reference_id === payable.id)
  expect(paidReserves).toHaveLength(1)
  expect(paidReserves[0].id).toBe(payable.reserve_entry_id)
  await expect(panel).toContainText('Turnover requirement not met')
  const finalBox = await panel.boundingBox()
  expect(finalBox).not.toBeNull()
  expect(finalBox!.x).toBeGreaterThanOrEqual(0)
  expect(finalBox!.x + finalBox!.width).toBeLessThanOrEqual(info.project.name === 'mobile' ? 360 : 1440)
  await panel.screenshot({ path: info.outputPath('withdrawal-qualification-complete.png') })
  expect(runtimeErrors).toEqual([])
})
