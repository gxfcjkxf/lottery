import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {selectControl} from '../select-control'

const brand='aaaaaaaa-aaaa-4aaa-8aaa-000000000001',actor='bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
for(const locale of ['en','zh-CN'] as const)test(`monthly retention records, policy and original retry ${locale}`,async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 const settings={timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}
 let policy={version:1,enabled:false,execution_day:10,execution_time:'02:00',timezone:settings.timezone,next_run:null as null|{execute_at:string;cutoff_at:string},updated_at:'2026-11-12T00:00:00Z'}
 const tasks=Array.from({length:51},(_,i)=>({id:`cccccccc-cccc-4ccc-8ccc-${String(i+1).padStart(12,'0')}`,brand_id:brand,brand_name:locale==='en'?'Retention trial':'归档试用品牌',policy_version:1,timezone:settings.timezone,execute_at:'2026-11-09T18:00:00Z',cutoff_at:'2026-09-30T16:00:00Z',state:i===1?'failed':'deferred',counts:{configured_orders:{deleted:i===0?'9007199254740993123':'12',receipts:'12',deferred:'2'},audit_logs:{deleted:'240',receipts:'3',deferred:'0'}},error_code:i===1?'DATA_RETENTION_BATCH_FAILED':'',created_at:'2026-11-09T18:00:00Z',finished_at:'2026-11-09T18:01:00Z'}))
 const writes:Array<{key:string|null;actor:string|null;body:string}> = []
 let retries=0,readFail=false
 await page.route('**/api/**',async route=>{
  const req=route.request(),url=new URL(req.url()),reply=(data:unknown)=>route.fulfill({json:{success:true,data,request_id:'owned-layout'}})
  expect(req.headers()['x-brand-id']).toBeUndefined()
  if(readFail&&req.method()==='GET')return route.fulfill({status:503,json:{success:false,error:{code:'UNAVAILABLE',message:'Owned read failure'},request_id:'owned-layout'}})
  if(url.pathname==='/api/v1/platform/data-retention-policy'){
   if(req.method()==='GET')return reply(policy)
   expect(req.method()).toBe('PUT');const body=req.postDataJSON();writes.push({key:req.headers()['idempotency-key']??null,actor:req.headers()['x-data-retention-actor-id']??null,body:req.postData()!})
   policy={...policy,...body,version:body.version+1,next_run:{execute_at:'2026-12-27T18:00:00Z',cutoff_at:'2026-10-31T16:00:00Z'}};return reply(policy)
  }
  if(url.pathname==='/api/v1/platform/data-retention-tasks'){
   expect(req.method()).toBe('GET');expect(url.searchParams.get('brand_id')).toBe(brand);const limit=Number(url.searchParams.get('limit')),offset=Number(url.searchParams.get('offset'));return reply({items:tasks.slice(offset,offset+limit),total_count:String(tasks.length),limit,offset})
  }
  if(url.pathname.endsWith('/retry')){
   retries++;writes.push({key:req.headers()['idempotency-key']??null,actor:req.headers()['x-data-retention-actor-id']??null,body:req.postData()!})
   if(retries===1)return route.fulfill({status:503,json:{success:false,error:{code:'UNAVAILABLE',message:'Owned layout failure'},request_id:'owned-layout'}})
   tasks[1].state='pending';return reply({id:tasks[1].id})
  }
  throw Error('Unexpected retention request '+req.method()+' '+url.pathname)
 })
 const source=await(await page.request.get('/src/DataRetentionPanel.vue')).text(),vue=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
 const props={brandId:brand,accountId:actor,canManage:true,canView:true,locale,displaySettings:settings}
 await page.route('**/__retention',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html lang="${locale}"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp,h,provide}from'${vue}';import C from'/src/DataRetentionPanel.vue';import{createPlatformI18n,platformI18nKey}from'/src/i18n.ts';import{setPlatformDisplaySettings}from'/@fs${resolve('shared/src/display-settings.ts')}';import'/@fs${resolve('shared/src/styles.css')}';import'/src/style.css';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/refresh-button.css')}';setPlatformDisplaySettings(${JSON.stringify(settings)});createApp({setup(){const i=createPlatformI18n();i.setLocale('${locale}');provide(platformI18nKey,i);return()=>h('main',{class:'management-console',style:'padding:12px;min-width:0'},[h(C,${JSON.stringify(props)})])}}).mount('#app')</script></html>`}))
 await page.goto('/__retention')
 const panel=page.getByTestId('data-retention');await expect(panel).toBeVisible();await expect(panel.locator('tbody tr')).toHaveCount(50)
 expect(await panel.textContent()).toContain('9007199254740993363')
 const noOverflow=async()=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
 const preset=page.viewportSize()!
 for(const width of info.project.name.endsWith('reflow')?[320,700,701,900,1280]:[preset.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
  await noOverflow()
 }
 await panel.getByRole('button',{name:locale==='en'?'Execution details':'执行详情',exact:true}).first().click()
 await expect(page.getByRole('dialog')).toBeVisible();await expect(page.getByRole('dialog')).not.toContainText(/Asia\/Singapore|UTC|时区|Timezone/);await noOverflow();await page.getByRole('button',{name:locale==='en'?'Close':'关闭',exact:true}).click()
 await panel.getByRole('button',{name:locale==='en'?'Retry':'重试',exact:true}).click()
 await expect(panel).toContainText(locale==='en'?'The result is unconfirmed':'结果尚未确认')
 await panel.getByRole('button',{name:locale==='en'?'Retry original request':'重试原请求',exact:true}).click()
 await expect.poll(()=>retries).toBe(2);expect(writes[0]).toEqual(writes[1]);expect(writes[0].actor).toBe(actor)
 await panel.getByRole('tab',{name:locale==='en'?'Archive settings':'归档设置',exact:true}).click()
 await expect(panel).not.toContainText(/Asia\/Singapore|UTC|时区|Timezone/)
 await noOverflow()
 await page.screenshot({path:info.outputPath(`retention-policy-${locale}.png`),fullPage:true})
 await panel.locator('input[type=checkbox]').check()
 await selectControl(panel.locator('.retention-form .select-control-trigger').first(),'28')
 await panel.getByRole('button',{name:locale==='en'?'Save':'保存',exact:true}).click()
 await expect.poll(()=>policy.version).toBe(2);expect(policy.execution_day).toBe(28);expect(policy.enabled).toBe(true)
 await expect(panel).toContainText('2026-11-01')
 for(const width of info.project.name.endsWith('reflow')?[320,700,701,900,1280]:[preset.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
  await noOverflow()
  if(width===320)await page.screenshot({path:info.outputPath(`retention-policy-320-${locale}.png`),fullPage:true})
 }
 await page.screenshot({path:info.outputPath(`retention-policy-${locale}.png`),fullPage:true})
 await panel.getByRole('tab',{name:locale==='en'?'Execution records':'执行记录',exact:true}).click();await noOverflow()
 await page.screenshot({path:info.outputPath(`retention-records-${locale}.png`),fullPage:false})
 readFail=true;await page.reload()
 const reload=page.locator('button[data-refresh-button]');await expect(reload).toHaveCount(1);await expect(reload.locator('svg')).toBeVisible()
 readFail=false;await reload.click();await expect(reload).toHaveCount(0);await expect(page.getByTestId('data-retention').locator('tbody tr')).toHaveCount(50)
 expect(errors).toEqual([])
})

test('platform navigation separates global policy from brand-filtered tasks',async({page})=>{
 const at='2026-11-12T00:00:00Z',reads:URL[]=[],errors:string[]=[]
 page.on('pageerror',e=>errors.push(e.message))
 await page.route('**/api/**',route=>{
  const req=route.request(),url=new URL(req.url());expect(req.method()).toBe('GET')
  const reply=(data:unknown)=>route.fulfill({json:{success:true,data,request_id:'owned-navigation'}})
  const path=url.pathname
  if(path==='/api/v1/display-settings')return reply({theme:'jade',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
  if(path.endsWith('/presentation'))return reply({display_name:'Retention review',logo_text:'R',logo_url:''})
  if(path.endsWith('/me'))return reply({account:{id:actor,username:'retention_navigation',super_admin:true,platform_permissions:['platform_settings.view.platform','platform_settings.write.platform']}})
  if(path.endsWith('/platform-settings'))return reply({version:1,display_name:'Retention review',logo_text:'R',logo_url:'',theme:'jade',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',updated_at:at})
  if(path.endsWith('/brands'))return reply({items:[{id:brand,name:'Retention trial',code:'retention',status:'active',member_count:'0',agent_count:'0',recharge_quota_available:null,created_at:at}]})
  if(path.endsWith('/data-retention-policy'))return reply({version:1,enabled:false,execution_day:10,execution_time:'02:00',timezone:'Asia/Singapore',next_run:null,updated_at:at})
  if(path.endsWith('/data-retention-tasks')){expect(req.headers()['x-brand-id']).toBeUndefined();reads.push(url);return reply({items:[],total_count:'0',limit:Number(url.searchParams.get('limit')),offset:Number(url.searchParams.get('offset'))})}
  throw Error('Unexpected platform navigation request '+path)
 })
 await page.addInitScript(()=>localStorage.setItem('lottery.platform.locale','en'))
 await page.goto('/')
 await expect(page.locator('.management-console')).toBeVisible()
 const toggle=page.locator('button[aria-controls="platform-navigation"]')
 if(await toggle.isVisible()&&await toggle.getAttribute('aria-expanded')==='false')await toggle.click()
 const operations=page.locator('.sidebar').getByRole('button',{name:/^(Operations|运营)$/,exact:true})
 if(await operations.getAttribute('aria-expanded')==='false')await operations.click()
 await page.getByTestId('platform-operations-tabs').getByRole('button',{name:/^(Data archiving|数据归档)$/,exact:true}).click()
 const panel=page.getByTestId('data-retention'),picker=page.locator('.topbar-brand-picker')
 await expect(panel).toBeVisible();await expect(picker).toBeVisible();await expect(page.getByTestId('platform-brand-required')).toHaveCount(0)
 expect(reads.at(-1)!.searchParams.has('brand_id')).toBe(false)
 await panel.getByRole('tab',{name:/^(Archive settings|归档设置)$/,exact:true}).click();await expect(picker).toHaveCount(0)
 await panel.getByRole('tab',{name:/^(Execution records|执行记录)$/,exact:true}).click();await expect(picker).toBeVisible()
 await selectControl(picker.locator('.select-control-trigger'),brand)
 await expect.poll(()=>reads.at(-1)!.searchParams.get('brand_id')).toBe(brand)
 expect(errors).toEqual([])
})
