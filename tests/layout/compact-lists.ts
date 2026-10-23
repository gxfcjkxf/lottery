import { expect, test } from '@playwright/test'
import { resolve } from 'node:path'
import { origins, gameId, expectInsideViewport, expectNoPageOverflow } from './fixture'

export function registerCompactListTests(reflow = false) {
  for (const locale of ['en', 'zh-CN'] as const) {
    for (const component of ['NotificationsPanel', 'CommissionPanel'] as const) {
      test(`compact ${component} keeps operational information and opens details (${locale})`, async ({ page }, info) => {
        const errors: string[] = [], at = '2026-10-21T00:00:00Z'
        page.on('pageerror', error => errors.push(error.message))
        const agent = { id: gameId, brand_id: gameId, agent_account_id: gameId, parent_id: null, depth: 1, path: [gameId], version: 1, config: { ratio: '0.1', mode: 'loss', status: 'active', can_create_children: true }, effective_mode: 'loss', mode_source_agent_id: null, policy_version: 1, parent_version: null, created_by: gameId, created_at: at, updated_at: at }
        await page.route('**/api/**', async route => {
          const request = route.request(), url = new URL(request.url()), path = url.pathname
          expect(request.method()).toBe('GET')
          expect(request.headers()['x-brand-id']).toBe(gameId)
          const reply = (data: unknown) => route.fulfill({ json: { success: true, data, associations: { [`agent:${gameId}`]: { name: 'compact_operator' }, [`member:${gameId}`]: { name: 'compact_operator' } } } })
          if (path.endsWith('/notification-deliveries') || path.endsWith('/history')) return reply({ items: [] })
          if (path.endsWith('/notification-templates')) return reply({ items: [{ brand_id: gameId, key: 'member.joined', version: 1, content: { en: { title: 'Welcome', body: 'Member {resource_id}' }, 'zh-CN': { title: '欢迎', body: '会员 {resource_id}' } }, updated_at: at, audit_log_id: gameId }] })
          if (path.endsWith('/agents')) return reply({ brand_id: gameId, items: [{ ...agent, username: 'compact_operator', online: false, display_points: '125' }], limit: 50, offset: 0, total_count: '1' })
          if (path.endsWith(`/agents/${gameId}`)) return reply(agent)
          throw Error(`Unexpected compact list read: ${path}`)
        })
        const props = { brandId: gameId, locale, displaySettings: { timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' } }
        await page.route('**/__compact-list', route => route.fulfill({ contentType: 'text/html', body: `<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><body><div id="app" class="management-console"></div><script id="harness-data" type="application/json">${JSON.stringify({component,props,locale})}</script><script type="module" src="/@fs${resolve('platform-web/tests/layout-harness.ts')}"></script></body></html>` }))
        await page.goto(origins.platform + '/__compact-list')
        const notification = component === 'NotificationsPanel'
        if (notification) await page.getByTestId('platform-notification-tabs').getByRole('button', { name: locale === 'en' ? 'Templates' : '模板', exact: true }).click()
        const list = page.getByTestId(notification ? 'platform-template-list' : 'platform-commission-list')
        await expect(list.locator('thead th')).toHaveCount(notification ? 2 : 9)
        await expect(list).not.toContainText(gameId)
        if (!notification) {
          await expect(list.getByText('compact_operator', { exact: true })).toHaveCount(1)
          await expect(list).toContainText('0.1')
        }
        const preset = page.viewportSize()!
        for (const width of reflow ? [320, 640, 900, 1440] : [preset.width]) {
          if (reflow) await page.setViewportSize({ width, height: preset.height })
          await expectNoPageOverflow(page)
        }
        await page.screenshot({ path: info.outputPath(`compact-table-${component}-${locale}.png`) })
        await list.getByRole('button', { name: notification ? locale === 'en' ? 'Welcome' : '欢迎' : 'compact_operator', exact: true }).click()
        const detail = page.getByRole('dialog')
        await expect(detail).toBeVisible()
        await expect(detail).not.toContainText(gameId)
        if(notification){
          await expect(detail).toContainText('Welcome')
          await expect(detail).toContainText('欢迎')
          await expect(detail.locator('h2')).not.toContainText('member.joined')
          await expect(detail).toContainText('Member {resource_id}')
        }else await expect(detail).toContainText('compact_operator')
        await expectInsideViewport(detail)
        await expectNoPageOverflow(page)
        await page.screenshot({ path: info.outputPath(`compact-list-${component}-${locale}.png`) })
        expect(errors).toEqual([])
      })
    }
  }
}
