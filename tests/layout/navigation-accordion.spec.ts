import {test, expect} from '@playwright/test'
import {gameId, origins, installLayoutApi, expectNoPageOverflow} from './fixture'

for (const locale of ['en', 'zh-CN'] as const) test(`console navigation accordion (${locale})`, async ({page}, info) => {
  await installLayoutApi(page, locale)
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname
    const reply = (data: unknown) => route.fulfill({json:{success:true,data}})
    if (path === '/api/v1/platform/me') return reply({account:{id:gameId,username:'accordion_reviewer',super_admin:true,platform_permissions:[]}})
    if (path === '/api/v1/platform/platform-settings') return reply({theme:'jade',updated_at:'2026-11-12T00:00:00Z',display_name:'Layout trial',logo_text:'L',logo_url:'',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',version:1})
    if (path === '/api/v1/platform/brands' || path === '/api/v1/admin/brands') return reply({items:[{id:gameId,code:'layout',name:'Layout trial',status:'active',member_count:'0',agent_count:'0',recharge_quota_available:'0',created_at:'2026-11-12T00:00:00Z'}]})
    if (path === '/api/v1/admin/me') return reply({account:{id:gameId,username:'accordion_reviewer',super_admin:false,brand_ids:[gameId],permissions:[],permissions_by_brand:{[gameId]:[]},platform_permissions:[]}})
    if (path === '/api/v1/agent/me') return reply({account:{agent_account_id:gameId,username:'accordion_reviewer'},agent:{id:gameId,brand_id:gameId,depth:1,config:{can_create_children:false}},code:null,user_origin:origins.user,brand_code:''})
    if (path === '/api/v1/agent/members') return reply({items:[],total_count:'0',limit:20,offset:0,summary:{member_count:'0',available_points:'0',bet_count:'0',stake_points:'0',unsettled_points:'0',net_points:'0'}})
    return route.fallback()
  })
  for (const kind of ['platform','brand','agent'] as const) {
    await page.goto(origins[kind])
    await expect(page.locator('.management-console')).toBeVisible()
    const sizes = info.project.name.endsWith('reflow') ? [320,700,900,1440] : [page.viewportSize()!.width]
    for (const width of sizes) {
      if (info.project.name.endsWith('reflow')) await page.setViewportSize({width,height:900})
      let menu = page.locator('.sidebar')
      if (!(await menu.isVisible())) {
        if (kind === 'agent') { await expectNoPageOverflow(page); continue }
        await page.locator(kind === 'platform' ? '.platform-mobile-nav button' : '.mobile-nav > button').last().click()
        menu = page.locator(kind === 'platform' ? '.sidebar' : '.mobile-more-menu')
      }
      const toggles = menu.locator(kind === 'platform' ? 'button[aria-expanded]' : 'summary')
      const open = menu.locator(kind === 'platform' ? 'button[aria-expanded="true"]' : 'details[open]')
      for (const index of [0,1]) {
        if (kind === 'platform' ? await toggles.nth(index).getAttribute('aria-expanded') === 'true' : await toggles.nth(index).locator('..').getAttribute('open') !== null) await toggles.nth(index).click()
        await toggles.nth(index).click()
        await expect(open).toHaveCount(1)
      }
      await toggles.nth(1).press('Enter')
      await expect(open).toHaveCount(0)
      await toggles.first().click()
      await expectNoPageOverflow(page)
      await page.screenshot({path:info.outputPath(`${kind}-${locale}-${width}.png`)})
      if (kind === 'brand' && await page.locator('.mobile-more-menu').isVisible()) await page.keyboard.press('Escape')
      if (kind === 'platform') await page.keyboard.press('Escape')
    }
  }
})
