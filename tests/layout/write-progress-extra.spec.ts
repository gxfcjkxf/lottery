import {test,expect} from '@playwright/test'
import {gameId,expectNoPageOverflow} from './fixture'
import {mountWriteProgress as mount,expectWriteProgress} from './write-progress'

const settings={timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}
for(const locale of ['en','zh-CN'] as const)for(const kind of ['data-retention','commission-settings','platform-game','draw-source','withdrawal-settings'] as const)test(`write progress ${kind} ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN',errors:string[]=[],writes:Array<{body:string|null;key:string;actor:string|undefined}>=[]
 page.on('pageerror',cause=>errors.push(cause.message))
 let policy={version:1,enabled:false,execution_day:10,execution_time:'02:00',timezone:settings.timezone,next_run:null as null|{execute_at:string;cutoff_at:string},updated_at:'2026-11-16T00:00:00Z'}
 let commission={version:1,payouts_enabled:false,config:{enabled:false,payout_mode:'manual',calendar:{timezone:settings.timezone,cycle:'weekly',boundary_time:'00:00:00',weekday:1,month_day:null,short_month:''}}}
 const game={id:gameId,name:'Owned game',code:'owned_game',model:{kind:'digits',digits:3},timezone:settings.timezone,version:1,status:'active',deletable:true}
 const source={id:gameId,name:'Owned source',revision:1,plan:{version:1,source_type:'mapped',mapping:{schedule:'source'},enabled:false,anchor_draw_at:null,anchor_period_no:null,interval_seconds:null,open_before_seconds:3600,close_before_seconds:15,source_url:'https://example.test/draw',credential_ref:''}}
 let withdrawal={brand_id:gameId,version:1,points_step:'1',config:{enabled:false,min_points:'1',max_points:null,allowed_sources:['recharge','winning','gift'],review_mode:'manual',turnover_multiple:'1'},updated_at:'2026-11-16T00:00:00Z',audit_log_id:gameId}
 let active=false
 await page.route('**/api/**',async route=>{
  const req=route.request(),url=new URL(req.url()),path=url.pathname
  const ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(req.method()==='GET'){
   if(path.endsWith('/filter-options'))return ok({brand_id:gameId,items:[],limit:100,offset:0})
   if(path.endsWith('/withdrawal-policy'))return ok(withdrawal)
   if(path.endsWith('/data-retention-policy'))return ok(policy)
   if(path.endsWith('/data-retention-tasks'))return ok({items:[],total_count:'0',limit:Number(url.searchParams.get('limit')),offset:Number(url.searchParams.get('offset'))})
   if(path.endsWith('/commission-settings'))return ok(commission)
   if(path.endsWith('/platform-games'))return ok({games:[game],total_count:'1',limit:Number(url.searchParams.get('limit')),offset:0})
   if(path.endsWith('/automation'))return ok({schedule:null,plan:null,source_check:null,attempts:[],allowed_origins:[]})
   if(path.endsWith('/draw-sources'))return ok({items:[source],active_source_id:active?gameId:'',active_source_revision:active?1:0,version:1})
   throw Error('Unexpected isolated read '+path)
  }
  expect(req.method()).toBe(kind==='data-retention'||kind==='commission-settings'||kind==='withdrawal-settings'?'PUT':'POST')
  writes.push({body:req.postData(),key:req.headers()['idempotency-key'],actor:req.headers()['x-data-retention-actor-id']})
  expect(writes.at(-1)!.key).toBeTruthy()
  await expectWriteProgress(page,kind==='platform-game'?'.detail-modal':'#app',`${kind}-${locale}-${writes.length}`)
  if(writes.length===1)return route.abort('failed')
  expect(writes[1]).toEqual(writes[0])
  if(kind==='data-retention'){expect(writes[0].actor).toBe(gameId);policy={...policy,...req.postDataJSON(),version:2,next_run:{execute_at:'2026-12-09T18:00:00Z',cutoff_at:'2026-10-31T16:00:00Z'}};return ok(policy)}
  if(kind==='commission-settings'){const {reason,...input}=req.postDataJSON();commission={...input,version:2};return ok(commission)}
  if(kind==='withdrawal-settings'){const {reason,...body}=req.postDataJSON();withdrawal={...withdrawal,...body,version:2};return ok(withdrawal)}
  if(kind==='platform-game'){game.name=req.postDataJSON().name;game.version++;return ok(game)}
  active=true;return ok({id:gameId})
 })
 if(kind==='withdrawal-settings'){
  await mount(page,'admin-web/src/WithdrawalPolicySettings.vue',{account:{id:gameId,super_admin:false,brand_ids:[gameId],permissions:[],permissions_by_brand:{[gameId]:['withdrawal_policy.view.brand','withdrawal_policy.write.brand','game.view.brand']}},brandId:gameId},'admin',locale)
  await page.locator('#withdraw-brand-min').fill('2')
 }
 if(kind==='data-retention'){
  await mount(page,'platform-web/src/DataRetentionPanel.vue',{brandId:gameId,accountId:gameId,canManage:true,canView:true,locale,displaySettings:settings},'platform',locale)
  await page.getByRole('tab',{name:zh?'归档设置':'Archive settings',exact:true}).click()
  await page.locator('input[type=checkbox]').check()
 }
 if(kind==='commission-settings')await mount(page,'platform-web/src/CommissionSettings.vue',{timezone:settings.timezone,canManage:true,canView:true},'platform',locale)
 if(kind==='platform-game'){
  await mount(page,'shared/src/PlatformLottery.vue',{surface:'platform',brandId:'',locale,canManage:true,displaySettings:settings},'platform',locale)
  await page.locator('.member-menu-trigger').click()
  await page.getByRole('menuitem',{name:zh?'编辑':'Edit',exact:true}).click()
  await page.getByRole('dialog').getByLabel(zh?'名称':'Name',{exact:true}).fill('Updated game')
 }
 if(kind==='draw-source'){
  await mount(page,'shared/src/AutomationSettings.vue',{surface:'platform',brandId:'',gameId,locale,canWrite:true},'platform',locale)
  await page.locator('.member-menu-trigger').click()
  await page.getByRole('menuitem',{name:zh?'使用此版本':'Use this revision',exact:true}).click()
 }else {
  await page.locator('button[type=submit]').last().click()
  if(kind==='withdrawal-settings')await page.getByRole('dialog').locator('.button-primary').click()
 }
 const retry=page.getByRole('button',{name:zh?'重试':'Retry',exact:true})
 await expect(retry).toBeVisible()
 expect(writes).toHaveLength(1)
 await page.screenshot({path:info.outputPath(`${kind}-${locale}-unknown.png`)})
 await retry.click()
 await expect(page.locator('.global-loading-overlay')).toHaveCount(0)
 await expect(retry).toHaveCount(0)
 expect(writes).toHaveLength(2)
 expect(errors).toEqual([])
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`${kind}-${locale}-saved.png`)})
})
