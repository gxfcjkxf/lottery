import {test,expect} from '@playwright/test'
import {installLayoutApi,installOrdersLayoutApi,origins,gameId,expectNoPageOverflow} from './fixture'
for(const locale of ['zh-CN','en'] as const)test(`user sees incoming agent transfer and details (${locale})`,async({page},info)=>{
 await installLayoutApi(page,locale);await installOrdersLayoutApi(page)
 const zh=locale==='zh-CN',agent='sending_agent_1234567890',at='2026-11-12T08:00:00Z'
 const buckets=()=>Object.fromEntries(['recharge','winning','gift','commission'].map(source=>[source,{available:'0',manual_frozen:'0',system_frozen:'0',withdrawal:'0'}]))
 const before=buckets(),delta=buckets(),after=buckets();delta.recharge!.available='12.5';after.recharge!.available='12.5'
 await page.route('**/api/v1/wallet/ledger?**',route=>route.fulfill({json:{success:true,data:{total_count:'1',items:[{id:gameId,brand_id:gameId,member_id:gameId,account_id:gameId,entry_type:'agent_transfer_credit',reference_type:'agent_transfer',reference_id:gameId,operation_key:'agent-transfer:test:credit',actor_type:'agent',actor_id:gameId,request_id:'user-transfer-layout',reason:'',version:1,before_snapshot:before,delta_snapshot:delta,after_snapshot:after,source_allocation:[{source:'recharge',state:'available',points:'12.5'}],created_at:at,agent_transfer:{agent_id:gameId,agent_account:agent,amount:'12.5',created_at:at}}]}}}))
 await page.goto(origins.user+'/wallet/ledger')
 const entry=page.locator('.ledger-entry')
 await expect(entry).toContainText(zh?'代理转入':'Agent transfer received')
 await expect(entry).toContainText(agent)
 await expect(entry).toContainText('+12.5')
 const time=await entry.locator('.ledger-time').innerText()
 if(info.project.name.endsWith('reflow')){
  const viewport=page.viewportSize()!
  for(const width of [320,700,701,1440]){
   await page.setViewportSize({width,height:viewport.height})
   await expectNoPageOverflow(page)
   if(width>700)await expect(page.locator('.ledger-table thead')).toBeVisible()
   else await expect(page.locator('.ledger-table thead')).toBeHidden()
   await page.screenshot({path:info.outputPath(`ledger-${locale}-${width}.png`),fullPage:true})
  }
 }
 await expectNoPageOverflow(page)
 expect((await page.locator('.workspace-page-title').boundingBox())!.height).toBeLessThanOrEqual(52)
 await entry.getByRole('button',{name:zh?'明细':'Details',exact:true}).click()
 const details=page.getByRole('dialog')
 await expect(details.locator('.agent-transfer-receipt dt')).toHaveText(zh?['转入代理','积分','转入时间']:['Sending agent','Points','Received at'])
 await expect(details.locator('.agent-transfer-receipt dd')).toHaveText([agent,'12.50',time])
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`user-agent-transfer-${locale}.png`),fullPage:true})
 await details.getByRole('button',{name:zh?'关闭消息':'Close message',exact:true}).click()
 await expect(details).toHaveCount(0)
})
