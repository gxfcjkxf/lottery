import { expect, test } from '@playwright/test'
import {installConsoleSession,openPlatformPage} from './real-console-session'
import {expectRealEntry,expectListLayout} from './real-page-review'
import { origins, gameId, expectInsideViewport } from './fixture'

export function registerCompactListTests(reflow = false) {
  for (const locale of ['en', 'zh-CN'] as const) {
    for (const component of ['NotificationTemplatesPanel', 'CommissionPanel'] as const) {
      test(`compact ${component} keeps operational information and opens details (${locale})`, async ({ page }, info) => {
        page.setDefaultTimeout(8000)
        const errors: string[] = [], at = '2026-10-21T00:00:00Z'
        page.on('pageerror', error => errors.push(error.message))
        const agent = { id: gameId, brand_id: gameId, agent_account_id: gameId, parent_id: null, depth: 1, path: [gameId], version: 1, config: { ratio: '0.1', mode: 'loss', status: 'active', can_create_children: true }, effective_mode: 'loss', mode_source_agent_id: null, policy_version: 1, parent_version: null, created_by: gameId, created_at: at, updated_at: at }
        await page.route('**/api/**', async route => {
          const request = route.request(), url = new URL(request.url()), path = url.pathname
          expect(request.method()).toBe('GET')
          if(component === 'CommissionPanel') expect(request.headers()['x-brand-id']).toBe(gameId)
          const reply = (data: unknown) => route.fulfill({ json: { success: true, data, associations: { [`agent:${gameId}`]: { name: 'compact_operator' }, [`member:${gameId}`]: { name: 'compact_operator' } } } })
          if (path.endsWith('/notification-deliveries') || path.endsWith('/history')) return reply({ items: [] })
          if (path.endsWith('/notification-templates')) return reply({ items: [{ key: 'member.joined', version: 1, content: { en: { title: 'Welcome', body: 'Member {resource_id}' }, 'zh-CN': { title: '欢迎', body: '会员 {resource_id}' } }, updated_at: at, audit_log_id: gameId }] })
          if (path.endsWith('/agents')) return reply({ brand_id: gameId, items: [{ ...agent, username: 'compact_operator', online: false, display_points: '125' }], limit: 50, offset: 0, total_count: '1' })
          if (path.endsWith(`/agents/${gameId}`)) return reply(agent)
          throw Error(`Unexpected compact list read: ${path}`)
        })
        await installConsoleSession(page,'platform',locale,gameId,['agent.view.platform','notification_template.view.platform','notification_template.write.platform'],gameId)
        await page.goto(origins.platform);await expect(page.locator('.management-console')).toBeVisible()
        if(component==='NotificationTemplatesPanel'){
          await openPlatformPage(page,locale==='en'?'Operations':'运营');await openPlatformPage(page,locale==='en'?'Notification templates':'通知模板')
        }else{
          await openPlatformPage(page,locale==='en'?'Commission':'佣金');await openPlatformPage(page,locale==='en'?'Agents':'代理',gameId)
        }
        await expectRealEntry(page,'platform')
        const notification = component === 'NotificationTemplatesPanel'
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
          await expectListLayout(page)
        }
        await page.screenshot({ path: info.outputPath(`compact-table-${component}-${locale}.png`) })
        await list.getByRole('button', { name: notification ? locale === 'en' ? 'Member · Welcome' : '会员 · 欢迎加入' : 'compact_operator', exact: true }).click()
        const detail = page.getByRole('dialog')
        await expect(detail).toBeVisible()
        await expect(detail).not.toContainText(gameId)
        if(notification){
          await expect(detail.locator('.template-languages input').nth(1)).toHaveValue('Welcome')
          await expect(detail.locator('.template-languages input').nth(0)).toHaveValue('欢迎')
          await expect(detail.locator('h2')).not.toContainText('member.joined')
          await expect(detail.locator('.template-languages textarea').nth(1)).toHaveValue('Member {resource_id}')
        }else await expect(detail).toContainText('compact_operator')
        await expectInsideViewport(detail)
        await expectListLayout(page)
        await page.screenshot({ path: info.outputPath(`compact-list-${component}-${locale}.png`) })
        expect(errors).toEqual([])
      })
    }
  }
}
