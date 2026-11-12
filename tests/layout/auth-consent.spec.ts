import { test, expect } from '@playwright/test'
import { installLayoutApi, installOrdersLayoutApi, origins, gameId, theme, expectNoPageOverflow } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) {
  test(`password and Telegram authenticate without consent (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    await installOrdersLayoutApi(page)
    let signedIn = false
    let authorizations = 0
    const requests: { path: string; input: Record<string, unknown> }[] = []
    const profile = {
      user: { id: gameId, username: 'auth-fixture', status: 'active' },
      member: { id: gameId, brand_id: gameId, status: 'normal', display_name: 'Auth fixture', joined_at: '2026-11-11T00:00:00Z' },
    }
    await page.route('**/api/v1/me', route => signedIn
      ? route.fulfill({ json: { success: true, data: profile } })
      : route.fulfill({ status: 401, json: { success: false, error: { code: 'UNAUTHORIZED', message: 'Sign in required' } } }))
    await page.route('**/api/v1/context', route => route.fulfill({ json: { success: true, data: {
      auth: { captcha_enabled: false, telegram_enabled: true, telegram_client_id: '9267434645' },
      available_locales: ['en', 'zh-CN'],
      brand: { id: gameId, code: 'layout', name: 'Layout trial', status: 'active', default_locale: locale, timezone: 'Asia/Singapore', theme, config_version: 1 },
      features: { pwa: false, real_payments: false },
    } } }))
    await page.exposeFunction('countTelegramAuthorization', () => authorizations++)
    await page.route('https://oauth.telegram.org/js/telegram-login.js', route => route.fulfill({
      contentType: 'text/javascript', body: 'window.Telegram = { Login: { auth(options, callback) { window.countTelegramAuthorization(); callback({ id_token: "fixture-token-" + options.nonce }) } } };',
    }))
    await page.route('**/api/v1/auth/telegram/challenge', route => route.fulfill({ json: { success: true, data: {
      id: gameId, nonce: 'auth-fixture-nonce', expires_at: new Date(Date.now() + 300000).toISOString(),
    } } }))
    for (const action of ['telegram', 'register', 'login']) await page.route(`**/api/v1/auth/${action}`, async route => {
      requests.push({ path: action, input: route.request().postDataJSON() })
      signedIn = true
      return route.fulfill({ status: action === 'register' ? 201 : 200, json: { success: true, data: { ...profile, access_token: 'fixture-session', token_type: 'Bearer', expires_at: new Date(Date.now() + 300000).toISOString() } } })
    })
    for (const path of ['/login', '/register']) {
      for (const method of ['telegram', 'password']) {
        signedIn = false
        await page.goto(`${origins.user}${path}`)
        await expect(page.locator('.combined-terms, .join-terms')).toHaveCount(0)
        await expectNoPageOverflow(page)
        await page.screenshot({ path: info.outputPath(`${path.slice(1)}-${method}.png`), fullPage: true })
        const count = requests.length
        const authCount = authorizations
        if (method === 'telegram') {
          await page.getByRole('button', { name: locale === 'zh-CN' ? '使用 Telegram 登录' : 'Continue with Telegram', exact: true }).click()
        } else {
          await page.locator('input[autocomplete="username"]').fill('auth_fixture')
          await page.locator('input[type="password"]').fill('test-password-2026')
          await page.locator('.auth-actions button[type="submit"]').click()
        }
        await expect(page).toHaveURL(`${origins.user}/`)
        await page.waitForLoadState('networkidle')
        expect(requests).toHaveLength(count + 1)
        expect(requests[count]!.input).not.toHaveProperty('privacy_policy_version')
        expect(requests[count]!.input).not.toHaveProperty('service_terms_version')
        if (method === 'telegram') expect(authorizations).toBe(authCount + 1)
      }
    }
    if (info.project.name.endsWith('reflow')) {
      signedIn = false
      await page.setViewportSize({ width: 320, height: 640 })
      await page.goto(`${origins.user}/register`)
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath('register-320.png'), fullPage: true })
    }
  })
}
