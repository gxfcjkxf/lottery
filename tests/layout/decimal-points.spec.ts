import { test, expect } from '@playwright/test'
import { installLayoutApi, installWalletLayoutApi, installOrdersLayoutApi, origins, gameId, expectNoPageOverflow } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) test(`fractional member wallet (${locale})`, async ({ page }, info) => {
  await installLayoutApi(page, locale)
  await installWalletLayoutApi(page)
  const zero = { available: '0', manual_frozen: '0', system_frozen: '0', withdrawal: '0' }
  await page.route('**/api/v1/wallet', route => route.fulfill({ json: { success: true, data: {
    account_id: gameId, brand_id: gameId, member_id: gameId, version: 2,
    display_points: '1234.56', available_points: '1234.56', frozen_points: '0', withdrawal_points: '0',
    recharge_points: '1200', winning_points: '34.56', gift_points: '0', commission_points: '0', manual_frozen_points: '0', system_frozen_points: '0',
    by_source: { recharge: { ...zero, available: '1200' }, winning: { ...zero, available: '34.56' }, gift: zero, commission: zero },
  } } }))
  await page.goto(origins.user + '/wallet')
  await expect(page.locator('.balance-amount strong')).toHaveText('1,234.56')
  const preset = page.viewportSize()!
  for (const width of info.project.name.endsWith('reflow') ? [320, 620, 900, 1440] : [preset.width]) {
    if (info.project.name.endsWith('reflow')) await page.setViewportSize({ width, height: preset.height })
    await expectNoPageOverflow(page)
    await page.screenshot({ path: info.outputPath(`wallet-${width}.png`), fullPage: true })
  }
  await page.goto(origins.user + '/wallet/buckets')
  await expect(page.locator('.bucket-source').nth(1)).toContainText('34.56')
  await expectNoPageOverflow(page)
})

for (const locale of ['en', 'zh-CN'] as const) test(`fractional member recharge records (${locale})`, async ({page},info) => {
 await installLayoutApi(page,locale);await installWalletLayoutApi(page)
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
 await page.route('**/api/v1/recharges?**',route=>{
  expect(route.request().method()).toBe('GET')
  const q=new URL(route.request().url()).searchParams
  return route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,snapshot_at:'2026-11-09T00:00:00Z',state:q.get('state'),limit:Number(q.get('limit')),offset:Number(q.get('offset')),total_count:'1',items:[{id:gameId,brand_id:gameId,member_id:gameId,method:'manual',points:'1234.56',state:'confirmed',version:'2',created_at:'2026-11-08T00:00:00Z',confirmed_at:'2026-11-08T00:00:00Z',ledger_entry_id:gameId}]}}})
 })
 await page.goto(origins.user+'/recharge')
 await expect(page.locator('.recharge-page')).toContainText('1,234.56')
 const preset=page.viewportSize()!
 for(const width of info.project.name.endsWith('reflow')?[320,620,900,1440]:[preset.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page)
 }
 await page.screenshot({path:info.outputPath('decimal-recharge-records.png'),fullPage:true})
 expect(errors).toEqual([])
})

for(const locale of ['en','zh-CN'] as const)test(`fractional member withdrawal submits exact decimals (${locale})`,async({page},info)=>{
 await installLayoutApi(page,locale);await installOrdersLayoutApi(page)
 const at='2026-11-09T00:00:00Z',writes:{body:unknown;key:string|undefined}[]=[],errors:string[]=[]
 let created=false
 page.on('pageerror',error=>errors.push(error.message))
 const row={id:gameId,brand_id:gameId,member_id:gameId,account_id:gameId,points:'0.5',state:'reviewing',version:1,source_allocation:[{source:'recharge',state:'available',points:'0.5'}],reserve_entry_id:gameId,release_entry_id:null,paid_entry_id:null,cycle_from_at:null,cycle_from_version:'0',reserve_version:'1',created_at:at,updated_at:at,reviewed_at:null,completed_at:null,decision_reason:'',audit_log_id:gameId}
 await page.route('**/api/v1/withdrawal*',route=>{
  const req=route.request(),path=new URL(req.url()).pathname,reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(path.endsWith('/withdrawal-availability'))return reply({points_step:'0.01',available_points:'1.50',source_balances:[{source:'recharge',state:'available',points:'1.50'}],actor_context:'a'.repeat(64),brand_id:gameId,member_id:gameId,policy_enabled:true,eligibility_configured:true,can_apply:true,reason_code:'AVAILABLE',min_points:'0.01',max_points:'1.50',allowed_sources:['recharge'],real_payments:false})
  if(path.endsWith('/withdrawal-qualification'))return reply({brand_id:gameId,member_id:gameId,account_id:gameId,base_points:'0.01',valid_points:'1',valid_order_count:'1',credit_numerator:'1',credit_denominator:'1',meets_turnover:true,cycle_from_at:null,cycle_from_version:'0',cutoff_at:at,cutoff_version:'8'})
  if(path.endsWith('/withdrawals')&&req.method()==='GET')return reply({brand_id:gameId,items:created?[row]:[],limit:20,offset:0,has_more:false,total_count:created?'1':'0'})
  expect(req.method()).toBe('POST');writes.push({body:req.postDataJSON(),key:req.headers()['idempotency-key']})
  if(writes.length===1)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic unknown response'}}})
  created=true;return route.fulfill({status:201,json:{success:true,data:row}})
 })
 await page.goto(origins.user+'/withdraw')
 await page.locator('.withdrawal-toolbar button').click()
 const dialog=page.getByRole('dialog'),input=dialog.getByRole('textbox'),submit=dialog.getByRole('button',{name:locale==='en'?'Submit':'提交',exact:true})
 await input.fill('0.001');await expect(submit).toBeDisabled()
 await input.fill('0.50');expect(await input.evaluate((element:HTMLInputElement)=>element.checkValidity())).toBe(true)
 await expect(submit).toBeEnabled();await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath('fractional-withdrawal.png'),fullPage:true})
 await submit.click();await dialog.getByRole('button',{name:locale==='en'?'Retry original request':'重试原申请',exact:true}).click()
 await expect(dialog).toHaveCount(0);expect(writes).toHaveLength(2);expect(writes[0]).toEqual(writes[1]);expect(writes[0]!.key).toBeTruthy()
 expect(writes[0]!.body).toEqual({points:'0.5',source_allocation:[{source:'recharge',state:'available',points:'0.5'}]})
 await expect(page.locator('.withdrawal-records')).toContainText('0.50');await expectNoPageOverflow(page);expect(errors).toEqual([])
})
