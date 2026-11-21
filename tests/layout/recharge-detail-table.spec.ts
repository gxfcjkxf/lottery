import { openBrandPage, openPlatformPage, installConsoleSession } from './real-console-session'
import { selectControl } from '../select-control'
import { expectListLayout } from './real-page-review'
import {test,expect} from '@playwright/test'
import {installLayoutApi,origins,gameId,expectNoPageOverflow} from './fixture'
for(const surface of ['brand','platform'] as const)for(const locale of ['zh-CN','en'] as const)test(`recharge types share one list and readable details (${surface}, ${locale})`,async({page},info)=>{
 await installLayoutApi(page,locale)
 const zh=locale==='zh-CN',memberId='bbbbbbbb-bbbb-4bbb-8bbb-000000000001',at='2026-10-24T00:00:00Z'
 const scope=surface==='brand'?'admin':'platform', requests:string[]=[], states:string[]=[]
 await page.route(`**/api/v1/${scope}/**`,route=>{
  expect(route.request().method()).toBe('GET')
  const url=new URL(route.request().url()),path=url.pathname
  const reply=(data:unknown)=>route.fulfill({json:{success:true,data,associations:{[`member:${memberId}`]:{name:'member001'},[`admin:${gameId}`]:{name:'finance_reviewer'}}}})
  if(path.endsWith('/me'))return reply({account:{id:gameId,username:'finance_reviewer',super_admin:false,brand_ids:[gameId],permissions:[],permissions_by_brand:{[gameId]:['user.view.brand','recharge.view.brand']}}})
  if(path.endsWith('/brands'))return reply({items:[{id:gameId,code:'layout',name:'Aurora',status:'active',member_count:'1',agent_count:'0',recharge_quota_available:'0',created_at:at}]})
  if(path.endsWith('/users'))return reply({items:[],limit:Number(url.searchParams.get('limit')),offset:Number(url.searchParams.get('offset')),total_count:'0'})
  if(path.endsWith('/recharge-quota'))return reply({brand_id:gameId,available:'100'})
  if(path.endsWith('/cash/orders')){
   expect(route.request().headers()['x-brand-id']).toBe(gameId)
   const kind=url.searchParams.get('kind')!;requests.push(kind);states.push(url.searchParams.get('state')??'')
   return reply({items:[{id:gameId,brand_id:gameId,member_id:memberId,member_account:'member001',kind,currency:'USDT',amount:'100',requested_amount:'100',points:'5000',fee:'0',rate:'50',state:url.searchParams.get('state')||(kind==='deposit'?'confirmed':'completed'),version:1,destination:kind==='deposit'?{id:gameId,version:1,enabled:true,currency:'USDT',channel:'TRON/TRC20',holder:'',institution:'',number:'T'+'a'.repeat(33)}:{id:''},proof:'',reference:'',automatic:false,credited_at:at,sender:'',created_at:at,expires_at:at}],total_count:'1'})
  }
  if(path.endsWith('/recharges')){requests.push('manual');states.push(url.searchParams.get('state')??'');return reply({total_count:'1',items:[{id:gameId,brand_id:gameId,member_id:memberId,account_id:gameId,...(surface==='platform'?{member_username:'member001'}:{}),method:'manual',points:'10',state:url.searchParams.get('state')||'confirmed',proof_reference:'proof-'+ '1234567890'.repeat(8),remark:zh?'已核对充值凭证及积分。':'Recharge proof and points checked.',version:1,created_by:gameId,created_by_name:'finance_reviewer',confirmed_by_name:'finance_reviewer',...(!url.searchParams.get('state')||url.searchParams.get('state')==='confirmed'?{confirmed_by:gameId,ledger_entry_id:gameId,confirmed_at:at}:{}),created_at:at}]})}
  throw Error(`Unexpected recharge detail request: ${path}`)
 })
 await installConsoleSession(page,surface,locale,gameId,[`user.view.${surface}`,`recharge.view.${surface}`])
 await page.goto(origins[surface])
 await expect(page.locator('.management-console')).toBeVisible()
 if(surface==='brand')await openBrandPage(page,zh?'充值记录':'Recharge records')
 else { await openPlatformPage(page,zh?'资金':'Finance'); await openPlatformPage(page,zh?'充值记录':'Recharge records',gameId) }
 const picker=()=>page.getByTestId('recharge-record-type')
 const list=page.locator('.management-list:visible')
 await expect(picker()).toContainText(zh?'资金充值':'Funds deposit')
 await expect(list).toHaveCount(1)
 await expect(list).toContainText('100 USDT')
 expect(requests).toEqual(['deposit'])
 const preset=page.viewportSize()!
 for(const kind of ['to_points','from_points','manual'] as const){
  await selectControl(picker(),kind)
  await expect.poll(()=>requests.at(-1)).toBe(kind)
  await expect(list).toHaveCount(1)
  await expect(list.locator('tbody')).toContainText(kind==='manual'?'10':'100 USDT')
  for(const width of info.project.name.includes('reflow')?[320,640,900,1440]:[preset.width]){
   if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectListLayout(page)
   await page.screenshot({path:info.outputPath(`recharge-${surface}-${kind}-${locale}-${width}.png`)})
  }
 }
 expect(requests).toEqual(['deposit','to_points','from_points','manual'])
 await expect(page.getByText('充值记录快照',{exact:true})).toHaveCount(0)
 await list.getByRole('button',{name:zh?'详情':'Details',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:zh?'详情':'Details',exact:true}),table=dialog.locator(surface==='brand'?'.recharge-detail-table':'.confirm-list')
 if(surface==='brand')await expect(table.locator('th[scope=row]')).toHaveCount(9)
 await expect(table).toContainText('member001')
 await expect(table).toContainText('finance_reviewer')
 for(const width of info.project.name.includes('reflow')?[320,640,900,1440]:[preset.width]){
  if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page)
  expect(await table.evaluate(el=>el.scrollWidth<=el.clientWidth+1)).toBe(true)
  await page.screenshot({path:info.outputPath(`recharge-detail-${locale}-${width}.png`)})
 }
 await page.keyboard.press('Escape')
 await expect(dialog).toHaveCount(0)
 await selectControl(picker(),'deposit')
 await expect(list).toHaveCount(1)
 await expect(list).toContainText('100 USDT')
 await selectControl(page.getByTestId('cash-state-filter'),'cancelled')
 await expect.poll(()=>states.at(-1)).toBe('cancelled')
 await expect(list.locator('tbody')).toContainText(zh?'已取消':'Cancelled')
 await page.screenshot({path:info.outputPath(`recharge-state-${surface}-${locale}.png`)})
 await selectControl(page.getByTestId('cash-state-filter'),'')
 await expect.poll(()=>states.at(-1)).toBe('')
 await expect(list.locator('tbody')).toContainText(zh?'已到账':'Credited')
 await selectControl(picker(),'manual')
 await expect(picker()).toContainText(zh?'积分充值':'Points recharge')
 await selectControl(page.getByTestId('cash-state-filter'),'cancelled')
 await expect.poll(()=>states.at(-1)).toBe('cancelled')
 await expect(list.locator('tbody')).toContainText(zh?'已取消':'Cancelled')
 await page.screenshot({path:info.outputPath(`points-recharge-state-${surface}-${locale}.png`)})
 await selectControl(picker(),'to_points')
 await expect.poll(()=>states.at(-1)).toBe('')
 await selectControl(page.getByTestId('cash-state-filter'),'completed')
 await expect.poll(()=>states.at(-1)).toBe('completed')
 await expect(list.locator('tbody')).toContainText(zh?'已兑换':'Converted')
})
