import { expect, test } from '@playwright/test'
import { gameId, installLayoutApi, installOrdersLayoutApi, installWalletLayoutApi, origins } from './fixture'

test('all three surfaces keep background requests quiet while an explicit submission remains visible', async ({ page }) => {
  await installLayoutApi(page, 'en')
  for (const url of [`${origins.user}/login`, origins.brand, origins.platform]) {
    let backgroundStarted = false, submitted = false
    let releaseBackground!: () => void, releaseSubmit!: () => void
    const background = new Promise<void>(resolve => { releaseBackground = resolve })
    const submission = new Promise<void>(resolve => { releaseSubmit = resolve })
    await page.route('**/api/v1/loading-probe', async route => {
      backgroundStarted = true
      await background
      await route.fulfill({ json: { success: true, data: {} } })
    })
    await page.route(/\/api\/v1\/(?:admin\/|platform\/)?auth\/login$/, async route => {
      expect(route.request().method()).toBe('POST')
      submitted = true
      await submission
      await route.fulfill({ status: 401, json: { success: false, error: { code: 'INVALID_CREDENTIALS', message: 'Synthetic rejected login' } } })
    })
    try {
      await page.goto(url)
      const form = page.locator('form').first()
      await expect(form).toBeVisible()
      await expect(page.locator('.global-loading-overlay')).toBeHidden()
      await page.evaluate(() => { void fetch('/api/v1/loading-probe').then(response => response.json()) })
      await expect.poll(() => backgroundStarted).toBe(true)
      await expect(page.locator('.global-loading-overlay')).toBeHidden()
      await form.locator('input[autocomplete="username"]').fill('layout-user')
      await form.locator('input[type="password"]').fill('layout-only-password')
      await form.locator('button[type="submit"], button:not([type])').last().click()
      await expect.poll(() => submitted).toBe(true)
      await expect(page.locator('.global-loading-overlay')).toBeVisible()
      releaseSubmit()
      // A still-pending background request must not keep a completed foreground action open.
      await expect(page.locator('.global-loading-overlay')).toBeHidden()
    } finally { releaseSubmit(); releaseBackground() }
  }
})

test('wallet timer updates retain content without a loading prompt, while manual refresh shows it', async ({ page }) => {
  await page.clock.install()
  await installLayoutApi(page, 'en')
  await installWalletLayoutApi(page)
  await page.goto(`${origins.user}/wallet`)
  await expect(page.locator('.total-primary strong')).toHaveText('170')
  await expect(page.locator('.global-loading-overlay')).toBeHidden()
  let requests = 0, release!: () => void
  let gate = new Promise<void>(resolve => { release = resolve })
  const releaseBackground = release
  await page.route('**/api/v1/wallet', async route => { requests++; await gate; await route.fallback() })
  try {
    await page.clock.fastForward(30_000)
    await expect.poll(() => requests).toBeGreaterThan(0)
    await expect(page.locator('.global-loading-overlay')).toBeHidden()
    await expect(page.locator('.total-primary strong')).toHaveText('170')
    const refresh = page.locator('#user-page-refresh button')
    await expect(refresh).toBeEnabled()
    gate = new Promise<void>(resolve => { release = resolve })
    const previous = requests
    await page.locator('.workspace-topbar > button.icon-button').click()
    await refresh.click()
    await expect.poll(() => requests).toBeGreaterThan(previous)
    await expect(page.locator('.global-loading-overlay')).toBeVisible()
    release()
    await expect(page.locator('.global-loading-overlay')).toBeHidden()
    await expect(page.locator('.total-primary strong')).toHaveText('170')
  } finally { release(); releaseBackground() }
})


test('quick stake autosave stays quiet while explicit delete and retry own their loading lifecycle', async ({ page }) => {
  await page.clock.install()
  await installLayoutApi(page, 'en')
  await installOrdersLayoutApi(page)
  let values = ['10'], writes = 0, rejectWrite = false, release!: () => void
  let gate = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/me/betting-preferences', async route => {
    if (route.request().method() === 'PUT') {
      writes++
      const requested = route.request().postDataJSON().quick_amounts
      await gate
      if (rejectWrite) return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic save failure'}}})
      values = requested
    }
    await route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,quick_amounts:values}}})
  })
  try {
    await page.goto(`${origins.user}/account`)
    const settings = page.getByTestId('betting-preferences'), overlay = page.locator('.global-loading-overlay')
    const input = settings.locator('input').first()
    await expect(input).toHaveValue('10')
    await input.fill('20')
    await page.clock.fastForward(400)
    await expect.poll(() => writes).toBe(1)
    await expect(overlay).toBeHidden()
    release()
    await expect.poll(() => values).toEqual(['20'])
    await page.clock.fastForward(1000)
    await expect(settings.locator('.chip-delete')).toBeEnabled()
    gate = new Promise<void>(resolve => { release = resolve })
    rejectWrite = true
    await settings.locator('.chip-delete').click()
    await expect.poll(() => writes).toBe(2)
    await expect(overlay).toBeVisible()
    release()
    await expect(settings.getByRole('alert')).toBeVisible()
    await expect(overlay).toBeHidden()
    gate = new Promise<void>(resolve => { release = resolve })
    rejectWrite = false
    await settings.getByRole('alert').getByRole('button').click()
    await expect.poll(() => writes).toBe(3)
    await expect(overlay).toBeVisible()
    release()
    await expect(overlay).toBeHidden()
    await expect.poll(() => values).toEqual([])
    await expect(settings.getByRole('alert')).toHaveCount(0)
  } finally { release() }
})

test('configured wallet retry waits for the response and preserves unavailable-wallet protection', async ({ page }) => {
  await installLayoutApi(page, 'en')
  await installOrdersLayoutApi(page)
  let requests = 0, delayResponse = false, release!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/wallet', async route => {
    requests++
    if (delayResponse) await gate
    await route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic wallet failure'}}})
  })
  try {
    await page.goto(`${origins.user}/configured/${gameId}`)
    const warning = page.locator('.configured-funds-warning'), overlay = page.locator('.global-loading-overlay')
    await expect(warning).toBeVisible()
    await expect(overlay).toBeHidden()
    const initialRequests = requests
    delayResponse = true
    await warning.getByRole('button').click()
    await expect.poll(() => requests).toBe(initialRequests + 1)
    await expect(overlay).toBeVisible()
    release()
    await expect(overlay).toBeHidden()
    await expect(warning).toBeVisible()
    await expect(page.locator('.configured-available strong')).toHaveText('—')
    await expect(page.getByRole('button', {name:'Confirm bet',exact:true})).toHaveCount(0)
  } finally { release() }
})
