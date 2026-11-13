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
    const permissions = ['role.view', 'role.write', 'admin.view', 'admin.write', 'agent.view', 'agent.write', 'agent_policy.view', 'agent_policy.write', 'join_code.view', 'join_code.write'].map(p => `${p}.brand`)
    const props = { ...(component === 'AccessManagement' ? { section: 'roles' } : {}), brandId: gameId, memberId: gameId, agentId: gameId, kind: 'agent', ownerRecord: {}, account: { id: gameId, username: 'modal_reviewer', super_admin: false, brand_ids: [gameId], permissions_by_brand: { [gameId]: permissions } } }
    const code = { id: gameId, brand_id: gameId, kind: 'agent', code: '0123456789ABCDEF01234567', owner_member_id: gameId, agent_id: gameId, status: 'active', starts_at: null, expires_at: null, version: 1, usable: true, created_at: at, updated_at: at, audit_log_id: gameId }
    const targetId = 'bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
    const role = {id:targetId,brand_id:gameId,code:'modal_role',name:'Modal role',status:'active',version:1,is_bootstrap:false,permissions:['role.view.brand']}
    const account = {id:targetId,username:'modal_operator',status:'active',version:1,super_admin:false,brand_ids:[gameId],role_ids:[targetId],role_codes:['modal_role']}
    const node = {id:targetId,brand_id:gameId,agent_account_id:targetId,parent_id:null,depth:1,path:[targetId],version:1,config:{ratio:'0.08',mode:'loss',status:'active',can_create_children:true},effective_mode:'loss',mode_source_agent_id:targetId,policy_version:1,parent_version:null,created_by:gameId,created_at:at,updated_at:at,username:'modal_agent',online:false,display_points:null}
    let writes = 0
    await page.route('**/api/**', async route => {
     const req = route.request(), url = new URL(req.url()), path = url.pathname
     if (req.method() !== 'GET') writes++
     expect(req.method()).toBe('GET')
     const reply = (data: unknown) => route.fulfill({ json: { success: true, data, associations: { [`member:${gameId}`]: { name: 'modal_member' }, [`agent:${gameId}`]: { name: 'modal_agent' }, [`agent:${targetId}`]: { name: 'modal_agent' } } } })
     if (path.endsWith('/roles')) return reply({items:[role],total_count:'1'})
     if (path.endsWith('/accounts')) return reply({items:[account],total_count:'1'})
     if (path.endsWith('/permissions')) return reply({items: permissions})
     if (path.endsWith('/agent-policy')) return reply({brand_id: gameId, version: 1, config: {enabled: true, max_depth: 5, loss_ratio_cap: '0.25', turnover_ratio_cap: '0.25', mode: 'loss', cycle: 'monthly'}, updated_at: at})
     if (path.endsWith('/agents')) return reply({brand_id:gameId,items:[node],limit:20,offset:0,total_count:'1'})
     if (path.endsWith('/agents/tree')) return reply({brand_id:gameId,parent_id:null,items:[node],limit:20,offset:0,total_count:'1'})
     if (path.endsWith('/agents/'+targetId)) return reply(node)
     if (path.endsWith('/join-codes')) return reply({brand_id: gameId, kind: 'agent', owner_member_id: url.searchParams.get('owner_member_id'), items: [{...code, owner_member_id: url.searchParams.get('owner_member_id'),agent_id:targetId}], limit: 20, offset: 0, total_count: '1'})
     if (path.endsWith(`/join-codes/${gameId}`)) return reply(code)
     throw Error(`Unexpected editor request: ${path}`)
    })
    await page.route('**/__action-editor', route => route.fulfill({contentType: 'text/html', body: `<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><body><div id="app"></div><script id="harness-data" type="application/json">${JSON.stringify({component,props,locale})}</script><script type="module" src="/@fs${resolve('admin-web/tests/layout-harness.ts')}"></script></body></html>`} ))
    await page.goto(origins.brand + '/__action-editor')
    if (component === 'JoinCodeManagement') {
     await expect(page.locator('.owner-join-code')).toContainText(code.code)
     await expect(page.locator('.owner-join-code button')).toHaveCount(0)
     await expect(page.getByRole('dialog')).toHaveCount(0)
     await expectNoPageOverflow(page)
     await page.screenshot({path: info.outputPath(`bound-agent-code-${locale}.png`)})
     expect(writes).toBe(0);expect(errors).toEqual([])
     return
    }
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
    async function review(name: string) {
     await expect(dialog).toBeVisible()
     for (const width of reflow ? [320, 640, 900, 1440] : [preset.width]) {
      if (reflow) await page.setViewportSize({width, height:preset.height})
      const submit=dialog.locator('.ui-form > .button, .ui-form-actions > button[type=submit], .detail-modal-footer > button').last()
      await submit.scrollIntoViewIfNeeded()
      await expect(submit).toBeInViewport()
      await expectInsideViewport(dialog)
      await expectNoPageOverflow(page)
      await page.screenshot({path:info.outputPath(`action-editor-${name}-${locale}-${width}.png`)})
      if (name === 'agent-edit') await dialog.screenshot({path:info.outputPath(`agent-detail-${locale}-${width}.png`)})
     }
     if(reflow) await page.setViewportSize(preset)
    }
    await review(component+'-create')
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    await open.click()
    await expect(dialog).toBeVisible()
    await page.keyboard.press('Escape')
    if (component === 'AccessManagement') {
     await page.locator('.access-page tbody tr').first().locator('.member-menu-trigger').click()
     await page.getByRole('menuitem',{name:zh?'编辑':'Edit',exact:true}).click()
     await review('role-edit')
     await page.keyboard.press('Escape')
     Object.assign(props, {section:'accounts'})
     await page.reload()
     await page.getByRole('button',{name:zh?'新建管理员':'Create administrator',exact:true}).click()
     await expect(dialog.locator('input[autocomplete="username"]')).toBeVisible()
     await review('account-create')
     await page.keyboard.press('Escape')
     const row=page.locator('.account-directory-table tbody tr')
     await row.locator('.member-menu-trigger').click()
     await page.getByRole('menuitem',{name:zh?'编辑':'Edit',exact:true}).click()
     await review('account-edit')
     await page.keyboard.press('Escape')
     await row.locator('.member-menu-trigger').click()
     await page.getByRole('menuitem',{name:zh?'重置密码':'Reset password',exact:true}).click()
     await review('account-password')
    } else if(component === 'AgentManagement') {
     await page.locator('.agent-directory-table .member-account').click()
     await review('agent-edit')
     await page.keyboard.press('Escape')
     Object.assign(props,{mode:'settings'})
     await page.reload()
     await page.getByRole('button',{name:zh?'编辑':'Edit',exact:true}).click()
     await review('agent-policy')
    }
    expect(writes).toBe(0)
    expect(errors).toEqual([])
   })
  }
 }
}
