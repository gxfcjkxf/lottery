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
   expect((await cell.boundingBox())!.height).toBeLessThanOrEqual(2*labelHeight+1)
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

for(const locale of ['zh-CN','en'] as const)test(`large ledger amounts remain complete without overlap (${locale})`,async({page},info)=>{
 await installLayoutApi(page,locale);await installOrdersLayoutApi(page)
 const buckets=(available:string)=>Object.fromEntries(['recharge','winning','gift','commission'].map(source=>[source,{available:source==='recharge'?available:'0',manual_frozen:'0',system_frozen:'0',withdrawal:'0'}]))
 const cases=[
  {type:'prize',before:'0',delta:'999999.99',after:'999999.99',change:'+999,999.99',balance:'999,999.99'},
  {type:'bet',before:'999999.99',delta:'-999999.99',after:'0',change:'−999,999.99',balance:'0'},
  {type:'prize',before:'999999.98',delta:'0.01',after:'999999.99',change:'+0.01',balance:'999,999.99'},
 ]
 await page.route('**/api/v1/wallet/ledger?**',route=>route.fulfill({json:{success:true,data:{total_count:'3',items:cases.map((c,i)=>({id:`bbbbbbbb-bbbb-4bbb-8bbb-${String(i+1).padStart(12,'0')}`,brand_id:gameId,member_id:gameId,account_id:gameId,entry_type:c.type,reference_type:'manual',reference_id:gameId,before_snapshot:buckets(c.before),delta_snapshot:buckets(c.delta),after_snapshot:buckets(c.after),reason:'',version:i+1,created_at:'2026-11-12T08:00:00Z'}))}}}))
 await page.goto(origins.user+'/wallet/ledger')
 const rows=page.locator('.ledger-entry');await expect(rows).toHaveCount(3)
 const preset=page.viewportSize()!
 for(const width of info.project.name.endsWith('reflow')?[320,360,375,390,412,600,700,701,915,1440]:[preset.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page)
  for(const [i,c] of cases.entries()){
   const row=rows.nth(i),cells=row.locator('.ledger-amount')
   await expect(cells.nth(0)).toHaveText(c.change);await expect(cells.nth(1)).toHaveText(c.balance)
   const geometry=await cells.evaluateAll(nodes=>nodes.map(node=>{
    const cell=node.getBoundingClientRect(),row=node.parentElement!.getBoundingClientRect(),range=document.createRange();range.selectNodeContents(node)
    const text=range.getBoundingClientRect(),style=getComputedStyle(node)
    return {left:cell.left,right:cell.right,top:cell.top,bottom:cell.bottom,rowLeft:row.left,rowRight:row.right,textLeft:text.left,textRight:text.right,textHeight:text.height,textTop:text.top,textBottom:text.bottom,lineHeight:parseFloat(style.lineHeight),fontSize:parseFloat(style.fontSize),overflow:node.scrollWidth>node.clientWidth+1,textOverflow:style.textOverflow}
   }))
   for(const cell of geometry){
    expect(cell.overflow).toBe(false);expect(cell.textOverflow).not.toBe('ellipsis');expect(cell.fontSize).toBeGreaterThanOrEqual(14)
    expect(cell.left).toBeGreaterThanOrEqual(cell.rowLeft-1);expect(cell.right).toBeLessThanOrEqual(cell.rowRight+1)
    expect(cell.textLeft).toBeGreaterThanOrEqual(cell.left-1);expect(cell.textRight).toBeLessThanOrEqual(cell.right+1)
    expect(cell.textHeight).toBeLessThanOrEqual(cell.lineHeight+1)
   }
   const [a,b]=geometry
   if(width<=700){
    expect(Math.abs(a!.top-b!.top)).toBeLessThanOrEqual(1)
    expect(Math.abs(a!.textTop-b!.textTop)).toBeLessThanOrEqual(1)
    expect(Math.abs(a!.textBottom-b!.textBottom)).toBeLessThanOrEqual(1)
    expect(a!.right).toBeLessThan(b!.left)
    expect(Math.abs(a!.textLeft-a!.left)).toBeLessThanOrEqual(1)
    expect(Math.abs(b!.textRight-b!.right)).toBeLessThanOrEqual(1)
    expect(a!.textTop-a!.top).toBeGreaterThanOrEqual(a!.lineHeight-1)
    expect(b!.textTop-b!.top).toBeGreaterThanOrEqual(b!.lineHeight-1)
   }
   expect(a!.right<=b!.left+1||b!.right<=a!.left+1||a!.bottom<=b!.top+1||b!.bottom<=a!.top+1).toBe(true)
  }
  await page.screenshot({path:info.outputPath(`large-ledger-${locale}-${width}.png`),fullPage:true})
 }
})
