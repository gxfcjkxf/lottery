import { test, expect } from '@playwright/test'
import { gameId, origins, installLayoutApi, openPlatformBrandList } from './fixture'
import { expectRealEntry, expectListLayout } from './real-page-review'

test('real-page checks reject missing headers, legacy pagers and nested surfaces', async ({ page }) => {
  await installLayoutApi(page, 'en')
  await page.route('**/api/v1/platform/**', route => {
    const path = new URL(route.request().url()).pathname
    expect(route.request().method()).toBe('GET')
    const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
    if (path.endsWith('/presentation')) return reply({ display_name: 'Review', logo_text: 'R', logo_url: '' })
    if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'guard_reviewer', super_admin: true, platform_permissions: ['brand.view.platform'] } })
    if (path.endsWith('/platform-settings')) return reply({ version: 1, display_name: 'Review', logo_text: 'R', logo_url: '', timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss', updated_at: '2026-11-02T00:00:00Z' })
    if (path.endsWith('/brands')) return reply({ items: [], total_count: '0' })
    throw Error('Unconfigured guard request: ' + path)
  })
  await page.goto(origins.platform)
  await openPlatformBrandList(page, 'en')
  await expectRealEntry(page, 'platform')
  const list = page.getByTestId('platform-brand-list')
  await expect(list.locator('td.empty-state')).toBeVisible()
  await expectListLayout(page)

  await test.step('removing the table header fails acceptance', async () => {
    const header = await list.locator('thead').evaluateHandle(node => { node.remove(); return node })
    try { await expect(expectListLayout(page)).rejects.toThrow(/missing table headers/) }
    finally { await list.locator('table').evaluate((table, header) => table.prepend(header), header) }
  })
  await test.step('reintroducing a text-only next button fails acceptance', async () => {
    const arrow = list.locator('.table-pagination-arrow').last()
    const original = await arrow.innerHTML()
    await arrow.evaluate(button => { button.textContent = 'Next' })
    try { await expect(expectListLayout(page)).rejects.toThrow(/nonstandard pagination arrows/) }
    finally { await arrow.evaluate((button, html) => { button.innerHTML = html }, original) }
  })
  await test.step('putting the pager inside table scrolling fails acceptance', async () => {
    await list.evaluate(list => list.querySelector('.table-wrap')!.append(list.querySelector('.table-pagination')!))
    try { await expect(expectListLayout(page)).rejects.toThrow(/pagination inside table scroll area/) }
    finally { await list.evaluate(list => list.append(list.querySelector('.table-pagination')!)) }
  })
  await test.step('adding a second list surface fails acceptance', async () => {
    await list.evaluate(list => { const nested = document.createElement('section'); nested.className = 'management-list'; nested.id = 'regression-probe'; nested.textContent = 'Nested list'; list.append(nested) })
    try { await expect(expectListLayout(page)).rejects.toThrow(/nested list surface/) }
    finally { await list.locator('#regression-probe').evaluate(node => node.remove()) }
  })
  await expectListLayout(page)
})
