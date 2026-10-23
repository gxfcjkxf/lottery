import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'
const origin = process.env.TEST_ADMIN_ORIGIN ?? 'http://localhost:5174'
const brand = '0199a000-0000-7000-8000-000000000001'
// All API operations are mocked; this editor check never logs in or writes a shared brand.
for (const locale of ['en', 'zh-CN'] as const) test(`domain editor has one enabled state and no priority (${locale})`, async ({ page }) => {
 const zh = locale === 'zh-CN', at = '2026-10-21T00:00:00Z'
 let record = { brand_id: brand, version: 1, status: 'active', domains: [] as { id: string; domain: string; enabled: boolean }[], updated_at: at }
 const writes: unknown[] = []
 await page.route('**/api/**', async route => {
  const req = route.request(), path = new URL(req.url()).pathname
  expect(path).toBe('/api/v1/admin/brand-domains')
  if (req.method() === 'GET') return route.fulfill({ json: { success: true, data: record } })
  expect(req.method()).toBe('POST'); const body = req.postDataJSON(); writes.push(body)
  expect(body).toEqual({ version: 1, domain: 'play.example.com', enabled: true, reason: '' })
  expect(req.headers()['idempotency-key']).toBeTruthy()
  record = { ...record, version: 2, domains: [{ id: brand, domain: body.domain, enabled: body.enabled }] }
  return route.fulfill({ status: 201, json: { success: true, data: { ...record, audit_log_id: brand } } })
 })
 const props = { brandId: brand, account: { id: brand, super_admin: false, brand_ids: [brand], permissions_by_brand: { [brand]: ['brand_domains.view.brand', 'brand_domains.write.brand'] } } }
 await page.route('**/__domain-editor', route => route.fulfill({ contentType: 'text/html', body: `<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><body><div id="app"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import Component from'/src/BrandDomains.vue';import{createAdminI18n,adminI18nKey}from'/src/i18n.ts';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve('admin-web/src/style.css')}';import'/@fs${resolve('shared/src/controls.css')}';const i18n=createAdminI18n();i18n.setLocale('${locale}');createApp(Component,${JSON.stringify(props)}).provide(adminI18nKey,i18n).mount('#app')</script></body></html>` }))
 await page.goto(origin + '/__domain-editor')
 await page.getByRole('button', { name: zh ? '添加域名' : 'Add domain', exact: true }).click()
 const dialog = page.getByRole('dialog'); await expect(dialog.getByRole('checkbox')).toHaveCount(1)
 await expect(dialog.locator('small')).toHaveCount(0)
 await expect(dialog).not.toContainText(zh ? '主域名' : 'Primary domain')
 await dialog.locator('input[autocomplete=off]').fill('play.example.com'); await dialog.getByRole('checkbox').check()
 await dialog.locator('button[type=submit]').click()
 await page.getByRole('dialog').getByRole('button', { name: zh ? '提交' : 'Submit', exact: true }).click()
 await expect(page.getByRole('dialog')).toHaveCount(0); await expect(page.locator('.domain-row')).toContainText('play.example.com'); expect(writes).toHaveLength(1)
})
