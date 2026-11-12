import {test,expect} from '@playwright/test'
import {installLayoutApi,installOrdersLayoutApi,origins,gameId,expectNoPageOverflow} from './fixture'
for(const locale of ['zh-CN','en'] as const)test(`user ledger shows transfer, historical points and ordered columns (${locale})`,async({page},info)=>{
 await installLayoutApi(page,locale);await installOrdersLayoutApi(page)
 const zh=locale==='zh-CN',agent='sending_agent_1234567890',at='2026-11-12T08:00:00Z'
 const buckets=()=>Object.fromEntries(['recharge','winning','gift','commission'].map(source=>[source,{available:'0',manual_frozen:'0',system_frozen:'0',withdrawal:'0'}]))
 const before=buckets(),delta=buckets(),after=buckets();delta.recharge!.available='12.5';after.recharge!.available='12.5';after.winning!.available='7.25';after.gift!.manual_frozen='100'
 await page.route('**/api/v1/wallet/ledger?**',route=>route.fulfill({json:{success:true,data:{total_count:'1',items:[{id:gameId,brand_id:gameId,member_id:gameId,account_id:gameId,entry_type:'agent_transfer_credit',reference_type:'agent_transfer',reference_id:gameId,operation_key:'agent-transfer:test:credit',actor_type:'agent',actor_id:gameId,request_id:'user-transfer-layout',reason:'',version:1,before_snapshot:before,delta_snapshot:delta,after_snapshot:after,source_allocation:[{source:'recharge',state:'available',points:'12.5'}],created_at:at,agent_transfer:{agent_id:gameId,agent_account:agent,amount:'12.5',created_at:at}}]}}}))
 await page.goto(origins.user+'/wallet/ledger')
 const entry=page.locator('.ledger-entry')
 await expect(entry).toContainText(zh?'代理转入':'Agent transfer received')
 await expect(entry).toContainText(agent)
 await expect(entry).toContainText('+12.5')
 await expect(entry.locator('td').nth(1)).toHaveText('+12.50')
 await expect(entry.locator('td').nth(2)).toHaveText('19.75')
 await expect(entry.locator('td').nth(3)).toHaveClass('ledger-time')
 await expect(entry.locator('td').nth(2)).toHaveCSS('display',page.viewportSize()!.width>700?'table-cell':'flex')
 await expect(page.locator('.ledger-table th')).toHaveText(zh?['类型','积分变动','变后积分','时间']:['Type','Change','Points after','Time'])
 async function checkMobileAmounts(){
  if(page.viewportSize()!.width>700)return
  const cells=entry.locator('.ledger-amount'),first=await cells.nth(0).boundingBox(),second=await cells.nth(1).boundingBox()
  const row=await entry.boundingBox()
  expect(second!.x+second!.width).toBeLessThanOrEqual(row!.x+row!.width)
  expect(Math.abs(first!.y-second!.y)).toBeLessThanOrEqual(1)
  for(const cell of await cells.all()){
   await expect(cell).toHaveCSS('white-space','nowrap')
   expect(await cell.evaluate(el=>el.scrollWidth<=el.clientWidth+1)).toBe(true)
   const labelHeight=await cell.evaluate(el=>parseFloat(getComputedStyle(el,'::before').lineHeight))
   expect((await cell.boundingBox())!.height).toBeLessThanOrEqual(labelHeight+1)
  }
 }
 await checkMobileAmounts()
 await expect(entry.getByRole('button')).toHaveCount(0)
 await expect(page.getByRole('dialog')).toHaveCount(0)
 if(info.project.name.endsWith('reflow')){
  const viewport=page.viewportSize()!
  for(const width of [320,700,701,1440]){
   await page.setViewportSize({width,height:viewport.height})
   await expectNoPageOverflow(page)
   await checkMobileAmounts()
   await expect(entry.locator('td').nth(2)).toHaveCSS('display',width>700?'table-cell':'flex')
   if(width>700)await expect(page.locator('.ledger-table thead')).toBeVisible()
   else await expect(page.locator('.ledger-table thead')).toBeHidden()
   await page.screenshot({path:info.outputPath(`ledger-${locale}-${width}.png`),fullPage:true})
  }
 }
 await expectNoPageOverflow(page)
 expect((await page.locator('.workspace-page-title').boundingBox())!.height).toBeLessThanOrEqual(52)
 await page.screenshot({path:info.outputPath(`user-agent-transfer-${locale}.png`),fullPage:true})
})
