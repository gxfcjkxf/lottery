import {test,expect} from '@playwright/test'
import {installLayoutApi,origins,gameId,expectNoPageOverflow} from './fixture'
for(const locale of ['zh-CN','en'] as const)test(`recharge detail uses a readable table (${locale})`,async({page},info)=>{
 await installLayoutApi(page,locale)
 const zh=locale==='zh-CN',memberId='bbbbbbbb-bbbb-4bbb-8bbb-000000000001',at='2026-10-24T00:00:00Z'
 await page.route('**/api/v1/admin/**',route=>{
  expect(route.request().method()).toBe('GET')
  const url=new URL(route.request().url()),path=url.pathname
  const reply=(data:unknown)=>route.fulfill({json:{success:true,data,associations:{[`member:${memberId}`]:{name:'member001'},[`admin:${gameId}`]:{name:'finance_reviewer'}}}})
  if(path.endsWith('/me'))return reply({account:{id:gameId,username:'finance_reviewer',super_admin:false,brand_ids:[gameId],permissions:[],permissions_by_brand:{[gameId]:['user.view.brand','recharge.view.brand']}}})
  if(path.endsWith('/brands'))return reply({items:[{id:gameId,code:'layout',name:'Aurora',status:'active',member_count:'1',agent_count:'0',recharge_quota_available:'0',created_at:at}]})
  if(path.endsWith('/users'))return reply({items:[],limit:Number(url.searchParams.get('limit')),offset:Number(url.searchParams.get('offset')),total_count:'0'})
  if(path.endsWith('/recharge-quota'))return reply({brand_id:gameId,available:'100'})
  if(path.endsWith('/recharges'))return reply({items:[{id:gameId,brand_id:gameId,member_id:memberId,account_id:gameId,method:'manual',points:'10',state:'confirmed',proof_reference:'proof-'+ '1234567890'.repeat(8),remark:zh?'已核对充值凭证及积分。':'Recharge proof and points checked.',version:1,created_by:gameId,confirmed_by:gameId,created_at:at}]})
  throw Error(`Unexpected recharge detail request: ${path}`)
 })
 await page.goto(origins.brand)
 await expect(page.locator('.management-console')).toBeVisible()
 const more=page.locator('.mobile-nav > button').last(),mobile=await more.isVisible()
 if(mobile)await more.click()
 await page.locator(mobile?'.mobile-more-menu':'.side-nav').getByRole('button',{name:zh?'充值记录':'Recharge records',exact:true}).click()
 await page.locator('.console-recharges-table').getByRole('button',{name:zh?'详情':'Details',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:zh?'详情':'Details',exact:true}),table=dialog.locator('.recharge-detail-table')
 await expect(table.locator('th[scope=row]')).toHaveCount(9)
 await expect(table).toContainText('member001')
 await expect(table).toContainText('finance_reviewer')
 const preset=page.viewportSize()!
 for(const width of info.project.name.includes('reflow')?[320,640,900,1440]:[preset.width]){
  if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page)
  expect(await table.evaluate(el=>el.scrollWidth<=el.clientWidth+1)).toBe(true)
  await page.screenshot({path:info.outputPath(`recharge-detail-${locale}-${width}.png`)})
 }
 await page.keyboard.press('Escape')
 await expect(dialog).toHaveCount(0)
})
