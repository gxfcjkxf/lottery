import {test,expect,type Page} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {origins,gameId,expectNoPageOverflow} from './fixture'
import {expectWriteProgress,mountWriteProgress as mount} from './write-progress'
import {registerSharedAccountMenuTests} from './agent-portal'
import {registerConfiguredSaleTests} from './workspace'

registerSharedAccountMenuTests()
registerConfiguredSaleTests()
const snapshot=JSON.parse(readFileSync('backend/internal/rules/config/examples/lucky_group_preview.json','utf8'))
const settings={timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}
const schedule={version:1,enabled:true,mode:'daily',draw_time:'21:30:00',open_before_hours:24,close_before_seconds:15,collect_after_seconds:30}
const plan={version:1,mapping:{schedule:'source'},open_before_seconds:3600,close_before_seconds:15,anchor_draw_at:null,anchor_period_no:null,interval_seconds:null}
for(const locale of ['en','zh-CN'] as const)for(const kind of ['brand-sales','platform-rules','draw-schedule','automation-time','manual-recharge','quota'] as const)test(`write progress ${kind} ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN',errors:string[]=[],writes:Array<{body:string|null;key:string;brand:string|undefined}>=[]
 page.on('pageerror',cause=>errors.push(cause.message))
 const admin=kind==='brand-sales'||kind==='manual-recharge',surface=admin?'admin':'platform'
 const rule={id:gameId,game_id:gameId,revision:1,snapshot,snapshot_hash:'test',status:'published',cases:[],validation:null}
 const sales={version:1,rule_id:gameId,enabled:true,prices:snapshot.prices,min_points:'1',max_points:'100',max_period_points:'10000',max_user_period_points:'5000'}
 await page.route('**/api/**',async route=>{
  const req=route.request(),path=new URL(req.url()).pathname
  const ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(req.method()==='GET'){
   if(path.endsWith('/automation'))return ok({schedule,plan,source_check:null,attempts:[],allowed_origins:[]})
   if(path.endsWith('/configured/rules'))return ok(rule)
   if(path.endsWith('/configured/sales'))return ok(sales)
   if(path.endsWith('/recharge-quota'))return ok({brand_id:gameId,available:'100'})
   if(path.endsWith('/entries'))return ok({items:[],total_count:'0'})
   throw Error('Unexpected test read '+path)
  }
  expect(req.method()).toBe('POST')
  writes.push({body:req.postData(),key:req.headers()['idempotency-key'],brand:req.headers()['x-brand-id']})
  expect(writes.at(-1)!.key).toBeTruthy()
  await expectWriteProgress(page,'body',`${kind}-${locale}-${writes.length}`)
  if(writes.length===1)return route.abort('failed')
  if(writes.length===3)return route.fulfill({status:400,json:{success:false,error:{code:'PLATFORM_GAME_INVALID',message:'Rejected'}}})
  expect(writes[1]).toEqual(writes[0])
  if(kind==='brand-sales')return ok({...sales,version:2})
  if(kind==='platform-rules')return ok({...rule,revision:2})
  if(kind==='draw-schedule')return ok({...schedule,version:2})
  if(kind==='automation-time')return ok({...plan,version:2})
  if(kind==='quota')return ok({brand_id:gameId,available:'110'})
  return ok({id:gameId,brand_id:gameId,member_id:gameId,points:'10',method:'manual',state:'confirmed',ledger_entry_id:gameId,confirmed_at:'2026-11-16T00:00:00Z'})
 })
 if(kind==='brand-sales'||kind==='platform-rules')await mount(page,'shared/src/ConfiguredPublishing.vue',{surface,brandId:admin?gameId:'',gameId,locale,snapshot,canWrite:true},surface,locale)
 if(kind==='draw-schedule')await mount(page,'shared/src/DrawScheduleSettings.vue',{gameId,timezone:'Asia/Singapore',locale,canWrite:true},surface,locale)
 if(kind==='automation-time')await mount(page,'shared/src/AutomationTimeSettings.vue',{gameId,locale,canWrite:true},surface,locale)
 if(kind==='manual-recharge'){
  await mount(page,'admin-web/src/ManualRechargeForm.vue',{accountId:gameId,brandId:gameId,memberId:gameId,username:'owned_progress_test'},surface,locale)
  await page.locator('input[inputmode=decimal]').fill('10')
 }
 if(kind==='quota'){
  await mount(page,'shared/src/RechargeQuotaPanel.vue',{brandId:gameId,actorId:gameId,locale,basePath:'/api/v1/platform',canGrant:true},surface,locale)
  await page.getByRole('button',{name:zh?'额度操作':'Operation',exact:true}).click()
  await page.locator('input[inputmode=decimal]').fill('10')
 }
 const save=page.locator('.button-primary').last()
 await save.click()
 await expect(page.getByRole('button',{name:zh?'重试':'Retry',exact:true})).toBeVisible()
 expect(writes).toHaveLength(1)
 if(await page.locator('.toast-close').count())await page.locator('.toast-close').first().click()
 await page.getByRole('button',{name:zh?'重试':'Retry',exact:true}).click()
 await expect(page.getByRole('button',{name:zh?'重试':'Retry',exact:true})).toHaveCount(0)
 await expect(page.locator('.global-loading-overlay')).toHaveCount(0)
 expect(writes).toHaveLength(2)
 expect(errors).toEqual([])
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`${kind}-${locale}-saved.png`)})
 if(kind==='draw-schedule'){
  await save.click()
  await expect.poll(()=>writes.length).toBe(3)
  await expect(page.locator('.global-loading-overlay')).toHaveCount(0)
  await expect(page.getByRole('button',{name:zh?'重试':'Retry',exact:true})).toHaveCount(0)
  await expect(page.locator('input[type=checkbox]')).toBeEnabled()
  expect(writes[2].key).not.toBe(writes[0].key)
 }
})
