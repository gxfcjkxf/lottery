import {test,expect,type Page} from '@playwright/test'
import {resolve} from 'node:path'
const origin=`http://127.0.0.1:${process.env.LOTTERY_LAYOUT_PORT_BASE??15273}`
const brand='aaaaaaaa-aaaa-4aaa-8aaa-000000000001', actor='aaaaaaaa-aaaa-4aaa-8aaa-000000000002'
const settings={theme:'jade',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}
async function mount(page:Page,file:string,props:unknown,locale:string,setup='') {
 const app=file.startsWith('admin')?'admin-web':'platform-web',factory=app==='admin-web'?'createAdminI18n':'createPlatformI18n',key=app==='admin-web'?'adminI18nKey':'platformI18nKey'
 const path='/@fs'+resolve(file),source=await(await page.request.get(origin+path)).text()
 const i18nPath=source.match(/from ["']([^"'\n]*\/i18n\.ts[^"'\n]*)["']/)?.[1]??'/@fs'+resolve(app+'/src/i18n.ts')
 await page.route('**/__permission-lifecycle',r=>r.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console"></div><script type="module">import{createApp,reactive,h}from'/node_modules/.vite/deps/vue.js';import Component from'${path}';import{${factory},${key}}from'${i18nPath}';import Toast from'/@fs${resolve('shared/src/OperationToastHost.vue')}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve(app+'/src/style.css')}';import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/management.css')}';const props=reactive(${JSON.stringify(props)});window.lifecycleProps=props;window.failures=[];props.onFailure=props.onSessionInvalid=(e)=>window.failures.push(e);${setup};const i18n=${factory}();i18n.setLocale('${locale}');createApp({render:()=>[h(Component,props),h(Toast,{locale:'${locale}'})]}).provide(${key},i18n).mount('#app')</script></html>`}))
 await page.goto(origin+'/__permission-lifecycle')
}
const account=(permissions:string[])=>({id:actor,super_admin:false,brand_ids:[brand],permissions:[],permissions_by_brand:{[brand]:permissions}})
for(const locale of ['en','zh-CN']) {
 test(`overview invalidates reads when permission changes ${locale}`,async({page},info)=>{
  await mount(page,'shared/src/ManagementOverview.vue',{scope:'platform',canView:true,locale,displaySettings:settings},locale,`window.reads=[];props.read=signal=>new Promise((resolve,reject)=>window.reads.push({resolve,reject,signal}));props.t=key=>key;props.errorMessage=e=>String(e);props.displayError=e=>String(e)`)
  await expect.poll(()=>page.evaluate(()=>(window as any).reads.length)).toBe(1)
  await page.evaluate(()=>{(window as any).lifecycleProps.canView=false})
  await expect(page.getByRole('status')).toContainText('overview.denied')
  expect(await page.evaluate(()=>(window as any).reads[0].signal.aborted)).toBe(true)
  await page.evaluate(()=>{(window as any).lifecycleProps.canView=true})
  await expect.poll(()=>page.evaluate(()=>(window as any).reads.length)).toBe(2)
  const summary=(count:string)=>({snapshot_at:'2026-11-13T00:00:00Z',timezone:'Asia/Singapore',today:'2026-11-13',total_members:count,online_members:'0',unsettled_points:'0',available_points:'0',frozen_points:'0',withdrawal_points:'0',withdrawal_count:'0',yesterday_same_time_stake:'0',days:Array.from({length:7},(_,i)=>({day:`2026-11-${String(7+i).padStart(2,'0')}`,stake_points:'0',net_points:'0',order_count:'0',bettor_count:'0',refund_points:'0',new_members:'0'})),brands:[]})
  await page.evaluate(value=>(window as any).reads[1].resolve(value),summary('888'))
  await expect(page.locator('.overview-stats')).toContainText('888')
  await page.evaluate(value=>(window as any).reads[0].resolve(value),summary('111'))
  await expect(page.locator('.overview-stats')).toContainText('888')
  await expect(page.locator('.overview-stats')).not.toContainText('111')
  expect(await page.evaluate(()=>(window as any).failures.length)).toBe(0)
  const viewport=page.viewportSize()!
  for(const width of info.project.name.endsWith('reflow')?[320,640,1440]:[viewport.width]){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:viewport.height})
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
   await page.screenshot({path:info.outputPath(`overview-${locale}-${width}.png`)})
  }
 })
 test(`brand operation reloads rights and rejects stale 401 ${locale}`,async({page})=>{
  const replies:Array<(value?:unknown,status?:number)=>Promise<void>>=[]
  await page.route('**/api/**',async route=>{
   expect(route.request().method()).toBe('GET')
   await new Promise<void>(resolve=>replies.push(async(value,status=200)=>{await route.fulfill({status,json:status===200?{success:true,data:value}:{success:false,error:{code:'AUTH_SESSION_REVOKED',message:'old session'}}});resolve()}))
  })
  await mount(page,'admin-web/src/BrandOperation.vue',{account:account([]),brandId:brand},locale)
  await expect(page.locator('article,section').first()).toBeVisible()
  expect(replies).toHaveLength(0)
  const setRights=async(rights:string[])=>page.evaluate(({brand,rights})=>{(window as any).lifecycleProps.account.permissions_by_brand={[brand]:rights}},{brand,rights})
  await setRights(['brand_operation.view.brand'])
  await expect.poll(()=>replies.length).toBe(1)
  await setRights([])
  await replies[0](null,401)
  expect(await page.evaluate(()=>(window as any).failures.length)).toBe(0)
  await setRights(['brand_operation.view.brand','brand_operation.write.brand'])
  await expect.poll(()=>replies.length).toBe(2)
  await replies[1]({brand_id:brand,name:'Lifecycle brand',status:'active',version:2,updated_at:'2026-11-13T00:00:00Z'})
  await expect(page.getByRole('button',{name:locale==='en'?'Pause':'暂停',exact:true})).toBeEnabled()
 })
 test(`notification template list ignores older permission load ${locale}`,async({page})=>{
  const replies:Array<(version:number)=>Promise<void>>=[]
  await page.route('**/api/**',async route=>{
   expect(route.request().method()).toBe('GET')
   await new Promise<void>(resolve=>replies.push(async version=>{await route.fulfill({json:{success:true,data:{items:[{key:'recharge.confirmed',version,content:{en:{title:'Version '+version,body:'body'},'zh-CN':{title:'版本 '+version,body:'正文'}},updated_at:'2026-11-13T00:00:00Z',audit_log_id:null}]}}});resolve()}))
  })
  await mount(page,'platform-web/src/NotificationTemplatesPanel.vue',{accountId:actor,canManage:true,canView:true,locale,displaySettings:settings},locale)
  await expect.poll(()=>replies.length).toBe(1)
  await page.evaluate(()=>{(window as any).lifecycleProps.canManage=false})
  await expect.poll(()=>replies.length).toBe(2)
  await replies[1](2)
  await expect(page.locator('.template-table .row-action')).toHaveCount(1)
  await replies[0](1)
  await page.locator('.template-table .row-action').click()
  await expect(page.getByRole('dialog').locator('input').first()).toHaveValue('版本 2')
  await expect(page.getByRole('dialog').locator('input').first()).toBeDisabled()
  await page.evaluate(()=>{(window as any).lifecycleProps.canView=false})
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.locator('.template-table .row-action')).toHaveCount(0)
  expect(replies).toHaveLength(2)
  await page.evaluate(()=>{(window as any).lifecycleProps.canView=true})
  await expect.poll(()=>replies.length).toBe(3)
  await replies[2](3)
  await page.locator('.template-table .row-action').click()
  await expect(page.getByRole('dialog').locator('input').first()).toHaveValue('版本 3')
 })
}

for(const locale of ['en','zh-CN']) test(`cash permission changes preserve the original mutation ${locale}`,async({page})=>{
 const permissions=['platform_settings.view.platform','platform_settings.write.platform']
 let version=1
 const config=()=>({version,rules:['CNY','USD','THB','USDT'].map(currency=>({currency,rate:'7',deposit_min:'1',deposit_max:'10000',withdrawal_min:'1',withdrawal_max:'10000',fee:'0',mode:'count',threshold:'1',inherit:false,enabled:true}))})
 const writes:Array<{body:unknown;key:string|undefined}>=[]
 await page.route('**/api/**',route=>{
  const req=route.request()
  if(req.method()==='GET')return route.fulfill({json:{success:true,data:config()}})
  expect(req.method()).toBe('PUT')
  writes.push({body:req.postDataJSON(),key:req.headers()['idempotency-key']})
  if(writes.length===1)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'unconfirmed'}}})
  version=2
  return route.fulfill({json:{success:true,data:config()}})
 })
 await mount(page,'shared/src/CashConsole.vue',{actorId:actor,basePath:'/api/v1/platform',platform:true,mode:'settings',locale,permissions},locale)
 await page.getByRole('button',{name:locale==='en'?'Save':'保存',exact:true}).click()
 const retry=page.getByRole('button',{name:locale==='en'?'Retry original operation':'重试原操作',exact:true})
 await expect(retry).toBeEnabled()
 await page.evaluate(()=>{(window as any).lifecycleProps.permissions=['platform_settings.view.platform']})
 await expect(retry).toBeDisabled()
 expect(writes).toHaveLength(1)
 await page.evaluate(permissions=>{(window as any).lifecycleProps.permissions=permissions},permissions)
 await expect(retry).toBeEnabled()
 await retry.click()
 await expect(retry).toHaveCount(0)
 expect(writes).toHaveLength(2)
 expect(writes[1]).toEqual(writes[0])
 expect(writes[0].key).toBeTruthy()
})

for(const locale of ['en','zh-CN']) test(`contact permission changes preserve uncertain saves ${locale}`,async({page})=>{
 const rights=['brand_presentation.view.brand','brand_presentation.write.brand']
 let version=1
 const config={items:[{id:actor,kind:'wechat',value:'before',image_url:null}],default_id:actor}
 const writes:Array<{body:any;key:string|undefined}>=[]
 await page.route('**/api/**',route=>{
  const req=route.request(),reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(req.method()==='GET')return reply({brand_id:brand,status:'active',version,config})
  expect(req.method()).toBe('PUT')
  writes.push({body:req.postDataJSON(),key:req.headers()['idempotency-key']})
  if(writes.length===1)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'unconfirmed'}}})
  version=2
  return reply({brand_id:brand,status:'active',version,config:writes[1].body.config})
 })
 await mount(page,'admin-web/src/BrandContacts.vue',{brandId:brand,account:account(rights)},locale)
 await page.getByRole('textbox',{name:locale==='en'?'Number':'号码',exact:true}).fill('after')
 await page.getByRole('button',{name:locale==='en'?'Save':'保存',exact:true}).click()
 const retry=page.getByRole('button',{name:locale==='en'?'Retry save':'重试保存',exact:true})
 await expect(retry).toBeEnabled()
 await page.evaluate(brand=>{(window as any).lifecycleProps.account.permissions_by_brand={[brand]:['brand_presentation.view.brand']}},brand)
 await expect(retry).toHaveCount(0)
 await page.evaluate(({brand,rights})=>{(window as any).lifecycleProps.account.permissions_by_brand={[brand]:rights}},{brand,rights})
 await expect(retry).toBeEnabled()
 await expect(page.getByRole('textbox',{name:locale==='en'?'Number':'号码',exact:true})).toHaveValue('after')
 await retry.click()
 await expect(retry).toHaveCount(0)
 expect(writes).toHaveLength(2)
 expect(writes[1]).toEqual(writes[0])
})

for(const locale of ['en','zh-CN']) test(`wallet retry keeps its member and key through view revocation ${locale}`,async({page})=>{
 const member='aaaaaaaa-aaaa-4aaa-8aaa-000000000003'
 const rights=['wallet.view.brand','wallet.adjust.brand']
 const buckets=Object.fromEntries(['recharge','winning','gift','commission'].map(source=>[source,{available:'0',manual_frozen:'0',system_frozen:'0',withdrawal:'0'}]))
 const writes:Array<{path:string;body:unknown;key:string|undefined}>=[]
 await page.route('**/api/**',route=>{
  const req=route.request(),path=new URL(req.url()).pathname,reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(req.method()==='POST'){
   writes.push({path,body:req.postDataJSON(),key:req.headers()['idempotency-key']})
   if(writes.length===1)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'unconfirmed'}}})
   return reply({})
  }
  if(path.endsWith(`/wallets/${member}`))return reply({account_id:brand,brand_id:brand,member_id:member,version:1,display_points:'0',available_points:'0',frozen_points:'0',withdrawal_points:'0',recharge_points:'0',winning_points:'0',gift_points:'0',commission_points:'0',manual_frozen_points:'0',system_frozen_points:'0',by_source:buckets})
  if(path.endsWith('/ledger'))return reply({items:[],total_count:'0'})
  if(path.endsWith('/reconciliation'))return reply({account_id:brand,member_id:member,version:1,ledger_version:1,consistent:true,actual:buckets,expected:buckets})
  if(path.includes('/cash/balances/'))return reply({items:[]})
  throw Error('Unexpected wallet request '+path)
 })
 await mount(page,'admin-web/src/FinanceManagement.vue',{brandId:brand,account:account(rights),initialMemberId:member,initialMemberUsername:'frozen_member',view:'wallet'},locale,`import{setPlatformDisplaySettings}from'/@fs${resolve('shared/src/index.ts')}';setPlatformDisplaySettings(${JSON.stringify(settings)})`)
 const dialog=page.getByRole('dialog'),form=dialog.locator('.operations-grid form')
 await form.getByLabel(locale==='en'?'Amount':'调整额',{exact:true}).fill('25')
 await form.getByRole('button',{name:locale==='en'?'Submit adjustment':'提交调整',exact:true}).click()
 const retry=dialog.getByRole('button',{name:locale==='en'?'Retry original request':'重试原请求',exact:true})
 await expect(retry).toBeVisible()
 await page.evaluate(brand=>{(window as any).lifecycleProps.account.permissions_by_brand={[brand]:[]}},brand)
 await expect(dialog).toBeVisible()
 await retry.click()
 expect(writes).toHaveLength(1)
 await page.evaluate(({brand,rights})=>{(window as any).lifecycleProps.account.permissions_by_brand={[brand]:rights}},{brand,rights})
 await expect(form.getByLabel(locale==='en'?'Amount':'调整额',{exact:true})).toBeVisible()
 await retry.click()
 await expect(retry).toHaveCount(0)
 expect(writes).toHaveLength(2)
 expect(writes[1]).toEqual(writes[0])
 expect(writes[0].path).toBe(`/api/v1/admin/wallets/${member}/adjust`)
})
