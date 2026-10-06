import { test, expect, type BrowserContext, type Page, type TestInfo } from '@playwright/test'
import { rememberAdminSession, restoreAdminSession } from './support/admin-session'

const origin = process.env.TEST_ADMIN_ORIGIN ?? 'http://localhost:5174'
const username = process.env.TEST_ADMIN_USERNAME
const password = process.env.TEST_ADMIN_PASSWORD
const brandId = '0199a000-0000-7000-8000-000000000001'

async function signedIn(page: Page, context: BrowserContext, info: TestInfo) {
  const restored = await restoreAdminSession(context, username!, brandId, origin)
  await page.goto(origin)
  await switchLanguage(page, 'en')
  await navigate(page, info, 'Users and members')
  if (!restored) {
    await page.locator('input[autocomplete="username"]').first().fill(username!)
    await page.locator('input[autocomplete="current-password"]').first().fill(password!)
    await page.getByRole('button', { name: 'Sign in and load members', exact: true }).click()
  }
  await page.locator('.directory-brand-bar select').selectOption(brandId)
  await expect(page.locator('.provision')).toBeVisible()
  rememberAdminSession(username!, await context.cookies(), origin)
}

async function switchLanguage(page: Page, language: 'en' | 'zh-CN') {
  await page.locator('[data-testid="admin-language"]').selectOption(language)
  await expect(page.locator('html')).toHaveAttribute('lang', language)
}
async function navigate(page: Page, info: TestInfo, name: string) {
  if (info.project.name === 'mobile') {
    if (name === 'Users and members') {
      await page.locator('.mobile-nav button').nth(1).click()
      return
    }
    await page.locator('.mobile-nav button').last().click()
    await page.locator('.mobile-more-menu').getByRole('button', { name: new RegExp(name) }).click()
  } else {
    await page.locator('.side-nav').getByRole('button', { name: new RegExp(name) }).click()
  }
}
async function fits(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true)
}

test('admin language selection changes navigation and login without changing a login draft', async ({ page }, info) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  const mutations: string[] = []
  page.on('request', request => { if (request.method() !== 'GET' && request.url().includes('/api/')) mutations.push(request.url()) })
  await page.goto(origin)
  await switchLanguage(page, 'en')
  await navigate(page, info, 'Users and members')
  const identifier = page.locator('input[autocomplete="username"]').first()
  const credential = page.locator('input[autocomplete="current-password"]').first()
  await identifier.fill('unsubmitted_language_draft')
  await credential.fill('synthetic-unsent-value')
  await expect(page.getByRole('button', { name: 'Sign in and load members', exact: true })).toBeVisible()
  await switchLanguage(page, 'zh-CN')
  await expect(page.getByRole('button', { name: '登录并加载真实成员', exact: true })).toBeVisible()
  await expect(identifier).toHaveValue('unsubmitted_language_draft')
  await expect(credential).toHaveValue('synthetic-unsent-value')
  await switchLanguage(page, 'en')
  await fits(page)
  await page.screenshot({ path: info.outputPath('admin-english-login.png'), fullPage: true })
  await page.reload()
  await expect(page.locator('[data-testid="admin-language"]')).toHaveValue('en')
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')
  expect(mutations).toEqual([])
  expect(errors).toEqual([])
})

test('real authenticated member and permission drafts survive language changes with no business writes', async ({ page, context }, info) => {
  test.skip(!username || !password, 'Provide isolated test administrator credentials')
  test.setTimeout(60_000)
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await signedIn(page, context, info)
  const writes: string[] = []
  page.on('request', request => { if (request.method() !== 'GET' && request.url().includes('/api/')) writes.push(request.url()) })
  const provision = page.locator('.provision')
  const memberName = provision.getByLabel('Username', { exact: false })
  await memberName.fill('unsent_member_draft')
  await provision.getByLabel(/Initial password/).fill('synthetic-member-draft-2026')
  await provision.getByLabel(/Creation reason/).fill('中文原因 must remain unchanged')
  await switchLanguage(page, 'zh-CN')
  await expect(provision.getByRole('heading', { name: '新建品牌成员' })).toBeVisible()
  await expect(provision.getByLabel(/用户名/)).toHaveValue('unsent_member_draft')
  await expect(provision.getByLabel(/创建原因/)).toHaveValue('中文原因 must remain unchanged')
  await switchLanguage(page, 'en')
  await expect(provision.getByRole('heading', { name: 'Create brand member' })).toBeVisible()
  await fits(page)
  await page.screenshot({ path: info.outputPath('admin-english-members.png'), fullPage: true })
  await navigate(page, info, 'Accounts and permissions')
  const access = page.locator('.access-page')
  await access.getByLabel(/Role code/).fill('unsent_role')
  await access.getByLabel(/Display name/).fill('中文 role draft')
  await access.getByLabel('user.view.brand', { exact: true }).check()
  await switchLanguage(page, 'zh-CN')
  await expect(access.getByLabel(/角色代码/)).toHaveValue('unsent_role')
  await expect(access.getByLabel(/显示名称/)).toHaveValue('中文 role draft')
  await expect(access.getByLabel('user.view.brand', { exact: true })).toBeChecked()
  await switchLanguage(page, 'en')
  await fits(page)
  await page.screenshot({ path: info.outputPath('admin-english-access.png'), fullPage: true })
  await navigate(page, info, 'Brands.*domains')
  const authSettings = page.locator('.auth-settings')
  await expect(authSettings.getByRole('heading', { name: 'Authentication settings' })).toBeVisible()
  await authSettings.getByRole('checkbox', { name: /Enable CAPTCHA/ }).check()
  const presentation = page.locator('.brand-presentation')
  await presentation.getByLabel('Display name inherit default', { exact: true }).uncheck()
  await presentation.getByLabel('Display name', { exact: true }).fill('中文品牌未提交')
  await presentation.getByLabel('Copy inherit default', { exact: true }).uncheck()
  await expect(presentation.getByLabel('English tagline', { exact: true })).toHaveCount(1)
  await expect(presentation.getByLabel('Chinese tagline', { exact: true })).toHaveCount(1)
  await switchLanguage(page, 'zh-CN')
  await expect(authSettings.getByRole('checkbox', { name: /启用验证码/ })).toBeChecked()
  await expect(presentation.getByLabel('展示名称', { exact: true })).toHaveValue('中文品牌未提交')
  await expect(presentation.getByLabel('英文标语', { exact: true })).toHaveCount(1)
  await expect(presentation.getByLabel('中文标语', { exact: true })).toHaveCount(1)
  await switchLanguage(page, 'en')
  await expect(presentation.getByLabel('Display name', { exact: true })).toHaveValue('中文品牌未提交')
  await fits(page)
  await page.screenshot({ path: info.outputPath('admin-english-brand-settings.png'), fullPage: true })
  await presentation.locator('.copy-group').screenshot({ path: info.outputPath('admin-english-content-fields.png') })
  expect(writes).toEqual([])
  expect(errors).toEqual([])
})

test('English member submission preserves Chinese business text and its receipt changes language', async ({ page, context }, info) => {
  test.skip(!username || !password, 'Provide isolated test administrator credentials')
  test.setTimeout(60_000)
  await signedIn(page, context, info)
  const provision = page.locator('.provision')
  const identifier = `lang_${crypto.randomUUID().replaceAll('-', '').slice(0, 16)}`
  await provision.getByLabel('Username', { exact: false }).fill(identifier)
  await provision.getByLabel(/Initial password/).fill('synthetic-language-member-2026')
  await provision.getByLabel(/Display name/).fill('角色权限')
  await provision.getByLabel(/Creation reason/).fill('中文原因 must not be translated')
  await switchLanguage(page, 'zh-CN')
  await switchLanguage(page, 'en')
  const sent = page.waitForRequest(request => request.method() === 'POST' && request.url().endsWith('/api/v1/admin/users'))
  const returned = page.waitForResponse(response => response.request().method() === 'POST' && response.url().endsWith('/api/v1/admin/users'))
  await provision.getByRole('button', { name: 'Create member', exact: true }).click()
  const request = await sent
  expect(request.postDataJSON()).toMatchObject({ username: identifier, display_name: '角色权限', reason: '中文原因 must not be translated' })
  expect(request.headers()['idempotency-key']).toMatch(/^[0-9a-f-]{36}$/)
  const response = await returned
  expect(response.status()).toBe(201)
  const receipt = (await response.json()).data
  await expect(provision.getByRole('status')).toContainText('Member created')
  await switchLanguage(page, 'zh-CN')
  await expect(provision.getByRole('status')).toContainText('成员已创建')
  await expect(provision.getByRole('status')).toContainText(receipt.member_id)
  await switchLanguage(page, 'en')
  await expect(page.locator('.directory-page')).toContainText('角色权限')
  await fits(page)
  await page.screenshot({ path: info.outputPath('admin-english-live-receipt.png'), fullPage: true })
})

test('published selected-brand languages control fallback without borrowing the public entry brand', async ({ page, context }, info) => {
  test.skip(!username || !password, 'Provide isolated test administrator credentials')
  test.setTimeout(60_000)
  await signedIn(page, context, info)
  const read = async (id: string) => {
    const response = await page.request.get(`${origin}/api/v1/admin/brand-presentation`, { headers: { 'X-Brand-ID': id } })
    expect(response.status()).toBe(200)
    return (await response.json()).data.effective as { default_locale: string; available_locales: string[] }
  }
  const initial = await read(brandId)
  await page.evaluate(() => localStorage.removeItem('lottery.admin.locale'))
  await page.reload()
  await page.locator('.directory-brand-bar select').selectOption(brandId)
  await expect(page.locator('[data-testid="admin-language"]')).toHaveValue(initial.default_locale)
  const preference = initial.available_locales.includes('zh-CN') ? 'zh-CN' : initial.default_locale
  await switchLanguage(page, preference as 'en' | 'zh-CN')
  const options = await page.locator('.directory-brand-bar select option').evaluateAll(elements => elements.map(element => (element as HTMLOptionElement).value).filter(Boolean))
  const writes: string[] = []
  page.on('request', request => { if (request.method() !== 'GET' && request.url().includes('/api/')) writes.push(request.url()) })
  for (const id of options) {
    const published = await read(id)
    await page.locator('.directory-brand-bar select').selectOption(id)
    await expect(page.locator('[data-testid="admin-language"] option')).toHaveCount(published.available_locales.length)
    await expect(page.locator('[data-testid="admin-language"]')).toHaveValue(published.available_locales.includes(preference) ? preference : published.default_locale)
    expect((await page.locator('[data-testid="admin-language"] option').evaluateAll(elements => elements.map(element => (element as HTMLOptionElement).value))).sort()).toEqual([...published.available_locales].sort())
  }
  await page.locator('.directory-brand-bar select').selectOption(brandId)
  await expect(page.locator('[data-testid="admin-language"]')).toHaveValue(preference)
  expect(writes).toEqual([])
})
