import { test, expect } from '@playwright/test'
import { installLayoutApi, gameId, theme } from '../layout/fixture'

for (const locale of ['en', 'zh-CN'] as const) test(`Telegram login layout and consent (${locale})`, async ({ page }, info) => {
  await installLayoutApi(page, locale)
  await page.context().addCookies([{ name: 'lottery-locale', value: locale, url: 'http://localhost:5173' }])
  await page.route('**/api/v1/context', route => route.fulfill({ json: { success: true, data: {
    auth: { captcha_enabled: false, telegram_enabled: true, telegram_client_id: '9267434645' },
    available_locales: ['en', 'zh-CN'],
    brand: { id: gameId, code: 'layout', name: 'Layout trial', status: 'active', default_locale: 'zh-CN', timezone: 'Asia/Singapore', theme, config_version: 1 },
    features: { pwa: false, real_payments: false },
  } } }))
  await page.route('https://oauth.telegram.org/js/telegram-login.js', route => route.fulfill({
    contentType: 'text/javascript', body: 'window.Telegram = { Login: { auth() {} } };',
  }))
  await page.route('**/api/v1/auth/telegram/challenge', route => route.fulfill({ json: { success: true, data: {
    id: gameId, nonce: 'telegram-ui-review', expires_at: new Date(Date.now() + 300000).toISOString(),
  } } }))
  await page.goto('http://localhost:5173/login')
  const button = page.getByRole('button', { name: locale === 'zh-CN' ? '使用 Telegram 登录' : 'Continue with Telegram', exact: true })
  await expect(button).toBeVisible()
  await expect(button).toBeEnabled()
  await expect(page.locator('.combined-terms')).toHaveCount(0)
  await expect(button).toHaveCSS('border-radius', '50%')
  await expect(button).toHaveCSS('width', '48px')
  await expect(page.locator('.auth-actions > .button')).toHaveCSS('height', '48px')
  const icon = button.locator('img')
  await expect.poll(() => icon.evaluate((el: HTMLImageElement) => el.complete && el.naturalWidth > 0)).toBe(true)
  await expect(button).toBeEnabled()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true)
  const password = page.locator('.auth-form input[autocomplete="current-password"]')
  const passwordBox = (await password.boundingBox())!
  const inputContainer = (await page.locator('.login-entry__input').last().boundingBox())!
  expect(Math.abs(passwordBox.width - inputContainer.width)).toBeLessThan(1)
  await password.fill('test password')
  await page.getByRole('button', { name: locale === 'zh-CN' ? '显示密码' : 'Show password', exact: true }).click()
  await expect(password).toHaveAttribute('type', 'text')
  await page.getByRole('button', { name: locale === 'zh-CN' ? '隐藏密码' : 'Hide password', exact: true }).click()
  await expect(password).toHaveAttribute('type', 'password')
  await page.screenshot({ path: info.outputPath('telegram-login.png'), fullPage: true })
  await page.locator('.player-login-tabs a[href="/register"]').click()
  await expect(page.locator('.player-login-tabs a[href="/register"]')).toHaveAttribute('aria-current', 'page')
  await expect(page.locator('.combined-terms')).toHaveCount(0)
  await expect(button).toBeEnabled()
  for (const viewport of [{ width: 320, height: 640 }, { width: 844, height: 390 }]) {
    await page.setViewportSize(viewport)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true)
    const input = await password.boundingBox()
    expect(input!.x).toBeGreaterThanOrEqual(0)
    expect(input!.x + input!.width).toBeLessThanOrEqual(viewport.width)
  }
})
