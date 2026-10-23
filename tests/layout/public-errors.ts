import { expect, test } from '@playwright/test'
import { installLayoutApi, origins, expectNoPageOverflow } from './fixture'

export function registerPublicErrorTests() {
  for (const surface of ['user', 'brand', 'platform'] as const) {
    test(`${surface} API failures hide technical details and change language without resubmitting`, async ({ page }) => {
      await installLayoutApi(page, 'en')
      let code = 'AUTH_INVALID_CREDENTIALS', status = 401, submissions = 0
      await page.route('**/api/**', async route => {
        if (!new URL(route.request().url()).pathname.endsWith('/auth/login')) return route.fallback()
        expect(route.request().method()).toBe('POST')
        submissions++
        return route.fulfill({ status, json: { success: false, error: { code, message: 'PRIVATE_SQL_DETAIL secret-token' } } })
      })
      await page.goto(surface === 'user' ? `${origins.user}/login` : origins[surface])
      const form = page.locator('form').first()
      await form.locator('input[autocomplete="username"]').fill('error_test_account')
      await form.locator('input[type="password"]').fill('wrong-password-2026')
      await form.locator('button.button-primary').last().click()
      await expect(form).toContainText('Incorrect account or password.')
      await expect(page.locator('body')).not.toContainText('PRIVATE_SQL_DETAIL')
      await expect(page.locator('body')).not.toContainText('AUTH_INVALID_CREDENTIALS')
      const switchLanguage = async (language: 'en' | 'zh-CN') => {
        await page.locator('.locale-picker__trigger').filter({ visible: true }).first().click()
        await page.locator(`.locale-picker__option[data-code="${surface === 'user' && language === 'zh-CN' ? 'zh' : language}"]`).filter({ visible: true }).click()
      }
      await switchLanguage('zh-CN')
      await expect(form).toContainText('账号或密码不正确。')
      await expect(form).not.toContainText('Incorrect account or password.')
      expect(submissions).toBe(1)
      code = 'NEW_UNDOCUMENTED_ERROR'; status = 503
      await form.locator('button.button-primary').last().click()
      await expect(form).toContainText('暂时无法确认操作结果')
      await switchLanguage('en')
      await expect(form).toContainText('The result could not be confirmed.')
      await expect(page.locator('body')).not.toContainText('NEW_UNDOCUMENTED_ERROR')
      await expect(page.locator('body')).not.toContainText('secret-token')
      expect(submissions).toBe(2)
      await expectNoPageOverflow(page)
    })
  }
}
