import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'
import { origins, gameId, expectInsideViewport, expectNoPageOverflow } from './fixture'

export function registerActionEditorTests(reflow = false) {
 for (const locale of ['en', 'zh-CN'] as const) {
  for (const component of ['AccessManagement', 'AgentManagement', 'JoinCodeManagement'] as const) {
   test(`brand action editor opens in a modal ${component} (${locale})`, async ({ page }, info) => {
    page.setDefaultTimeout(8000)
    const zh = locale === 'zh-CN', errors: string[] = [], at = '2026-10-21T00:00:00Z'
    page.on('pageerror', error => errors.push(error.message))
    const permissions = ['role.view', 'role.write', 'admin.view', 'admin.write', 'agent.view', 'agent.write', 'agent_policy.view', 'join_code.view', 'join_code.write'].map(p => `${p}.brand`)
    const props = { brandId: gameId, memberId: gameId, agentId: gameId, kind: 'agent', ownerRecord: {}, account: { id: gameId, username: 'modal_reviewer', super_admin: false, brand_ids: [gameId], permissions_by_brand: { [gameId]: permissions } } }
    const code = { id: gameId, brand_id: gameId, kind: 'agent', code: '0123456789ABCDEF01234567', owner_member_id: gameId, agent_id: gameId, status: 'active', starts_at: null, expires_at: null, version: 1, usable: true, created_at: at, updated_at: at, audit_log_id: gameId }
    let writes = 0
    await page.route('**/api/**', async route => {
     const req = route.request(), url = new URL(req.url()), path = url.pathname
     if (req.method() !== 'GET') writes++
     expect(req.method()).toBe('GET')
     const reply = (data: unknown) => route.fulfill({ json: { success: true, data, associations: { [`member:${gameId}`]: { name: 'modal_member' }, [`agent:${gameId}`]: { name: 'modal_agent' } } } })
     if (path.endsWith('/roles') || path.endsWith('/accounts')) return reply({items: []})
     if (path.endsWith('/permissions')) return reply({items: permissions})
     if (path.endsWith('/agent-policy')) return reply({brand_id: gameId, version: 1, config: {enabled: true, max_depth: 5, loss_ratio_cap: '0.25', turnover_ratio_cap: '0.25', mode: 'loss', cycle: 'monthly'}, updated_at: at})
     if (path.endsWith('/agents') || path.endsWith('/agents/tree')) return reply({brand_id: gameId, parent_id: null, items: [], limit: 20, offset: 0, total_count: '0'})
     if (path.endsWith('/join-codes')) return reply({brand_id: gameId, kind: 'agent', owner_member_id: gameId, items: [code], limit: 20, offset: 0, total_count: '1'})
     if (path.endsWith(`/join-codes/${gameId}`)) return reply(code)
     throw Error(`Unexpected editor request: ${path}`)
    })
    await page.route('**/__action-editor', route => route.fulfill({contentType: 'text/html', body: `<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><body><div id="app" class="management-console"></div><script id="harness-data" type="application/json">${JSON.stringify({component,props,locale})}</script><script type="module" src="/@fs${resolve('admin-web/tests/layout-harness.ts')}"></script></body></html>`} ))
    await page.goto(origins.brand + '/__action-editor')
    const open = component === 'AccessManagement' ? page.getByRole('button', {name: zh ? '新建角色' : 'Create role', exact: true}) : component === 'AgentManagement' ? page.getByRole('button', {name: zh ? '新建代理' : 'Create agent', exact: true}) : page.locator('.jc-row button')
    const dialog = page.getByRole('dialog')
    await expect(dialog).toHaveCount(0)
    await open.click()
    await expect(dialog).toBeVisible()
    if (component === 'AgentManagement') {
     expect((await dialog.boundingBox())!.width).toBeLessThanOrEqual(520)
     const labels = dialog.locator('.am-form > label:not(.am-check)')
     const boxes = await labels.evaluateAll(items => items.map(item => { const box=item.getBoundingClientRect();return {x:box.x,y:box.y,bottom:box.bottom} }))
     expect(boxes.length).toBeGreaterThanOrEqual(4)
     for(let index=1;index<boxes.length;index++){expect(boxes[index].x).toBeCloseTo(boxes[0].x,1);expect(boxes[index].y).toBeGreaterThanOrEqual(boxes[index-1].bottom)}
    }
    const preset = page.viewportSize()!
    for (const width of reflow ? [320, 640, 900, 1440] : [preset.width]) {
     if (reflow) await page.setViewportSize({width, height: preset.height})
     await expectInsideViewport(dialog)
     await expectNoPageOverflow(page)
    }
    await page.screenshot({path: info.outputPath(`action-editor-${component}-${locale}.png`)})
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    await open.click()
    await expect(dialog).toBeVisible()
    if (component === 'AccessManagement') {
     await page.keyboard.press('Escape')
     await page.getByRole('tab', {name: zh ? '管理员账号' : 'Administrator accounts', exact: true}).click()
     await page.getByRole('button', {name: zh ? '新建管理员' : 'Create administrator', exact: true}).click()
     await expect(dialog).toBeVisible()
     await expect(dialog.locator('input[autocomplete="username"]')).toBeVisible()
    }
    if (component === 'JoinCodeManagement') {
     await dialog.locator('button[type="submit"]').click()
     await expect(page.locator('.jc-review')).toBeVisible()
     await expect(page.locator('.jc-form')).not.toBeVisible()
     await page.keyboard.press('Escape')
     await expect(page.locator('.jc-form')).toBeVisible()
     await page.keyboard.press('Escape')
     await page.getByRole('button', {name: zh ? '核对待处理变更' : 'Review pending change', exact: true}).click()
     await expect(page.locator('.jc-review')).toBeVisible()
    }
    expect(writes).toBe(0)
    expect(errors).toEqual([])
   })
  }
 }
}
