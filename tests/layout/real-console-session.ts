import { type Page } from '@playwright/test'
import { origins } from './fixture'
import { selectControl } from '../select-control'

// These routes restore an isolated synthetic session. HTML, navigation, app
// providers and CSS are always loaded from the application's real entry point.
export async function installConsoleSession(page: Page, surface: 'brand' | 'platform', locale: 'en' | 'zh-CN', brandId: string, grants: string[], accountId = '22222222-2222-4222-8222-222222222222', initialSettings: Record<string,unknown> = {}) {
  const scope = surface === 'brand' ? 'admin' : 'platform'
  const settings = { theme: 'jade', version: 1, display_name: 'Review platform', logo_text: 'R', logo_url: '', timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss', updated_at: '2026-11-02T00:00:00Z', ...initialSettings }
  const replies: Record<string, unknown> = {
    '/api/v1/display-settings': {theme:settings.theme,timezone:settings.timezone,date_format:settings.date_format,time_format:settings.time_format},
    [`/api/v1/${scope}/me`]: { account: { id: accountId, username: 'real_page_reviewer', super_admin: surface === 'platform', brand_ids: [brandId], permissions: [], permissions_by_brand: { [brandId]: grants }, platform_permissions: grants } },
    [`/api/v1/${scope}/brands`]: { items: [{ id: brandId, name: 'Aurora', code: 'aurora', status: 'active', member_count: '0', agent_count: '0', recharge_quota_available: '0', created_at: settings.updated_at }], total_count: '1' },
    '/api/v1/platform/presentation': { display_name: settings.display_name, logo_text: settings.logo_text, logo_url: '' },
    '/api/v1/platform/platform-settings': settings,
  }
  await page.context().addCookies(Object.values(origins).map(url => ({ name: 'lottery-locale', value: locale, url })))
  for (const [path, data] of Object.entries(replies)) await page.route(new RegExp(`${path}$`), route => {
    if(route.request().method()!=='GET')return route.fallback()
    return route.fulfill({ json: { success: true, request_id: 'real-session-review', data } })
  })
}

export async function openBrandPage(page: Page, label: string) {
  const mobile = await page.locator('.mobile-nav').isVisible()
  if (mobile) await page.locator('.mobile-nav button').last().click()
  const navigation = page.locator(mobile ? '.mobile-more-menu' : '.side-nav')
  const target = navigation.getByRole('button', { name: label, exact: true, includeHidden: true })
  const group = navigation.locator('details').filter({ has: page.getByRole('button', { name: label, exact: true, includeHidden: true }) })
  if (await group.count() && await group.getAttribute('open') === null) await group.locator('summary').click()
  await target.click()
}

export async function openPlatformPage(page: Page, label: string, brandId?: string) {
  const toggle = page.locator('button[aria-controls="platform-navigation"]')
  if (await toggle.isVisible() && await toggle.getAttribute('aria-expanded') === 'false') await toggle.click()
  await page.locator('#platform-navigation').getByRole('button', { name: label, exact: true }).click()
  if (brandId) await selectControl(page.locator('.topbar-brand-picker .select-control-trigger'), brandId)
}
