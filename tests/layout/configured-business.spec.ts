import {expect,test,type Page} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {resolve} from 'node:path'
const origin=`http://127.0.0.1:${process.env.LOTTERY_LAYOUT_PORT_BASE??15273}`,game='aaaaaaaa-aaaa-4aaa-8aaa-000000000001',versionID='bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
const sample=()=>JSON.parse(readFileSync('backend/internal/rules/config/examples/lucky_group_preview.json','utf8'))
const caseInput=()=>({requests:[{play_id:'operator_new_play',options:['lucky'],plate:'A',points:'10',multiplier:'1'}],draw:{regular:[1,3,7,4,5,6],special:[2],digits:[]},bet_points:'10',prize_points:'25',refund_points:'0'})
async function mount(page:Page,component:string,props:unknown){
 const userComponent=component.startsWith('user-web/')
 let vuePath='/node_modules/.vite/deps/vue.js',routerSetup='',installRouter=''
 if(userComponent){
  const source=await(await page.request.get(origin+'/src/ConfiguredBetting.vue')).text()
  const routerPath=source.match(/from "([^"]*vue-router\.js[^"]*)"/)![1]
  vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
  routerSetup=`import{createUserI18n,userI18nKey}from'/src/i18n.ts';const userI18n=createUserI18n();userI18n.setLocale(${JSON.stringify((props as {locale:string}).locale==='en'?'en':'zh-CN')});import{createRouter,createMemoryHistory}from'${routerPath}';const router=createRouter({history:createMemoryHistory(),routes:[{path:'/',component:{render:()=>null}},{path:'/login',component:{render:()=>null}}]});`
  installRouter='.provide(userI18nKey,userI18n).use(router)'
 }
 const surfaceStyle=component.startsWith('user-web/')?'user-web/src/style.css':(props as {surface:string}).surface==='platform'?'platform-web/src/style.css':'admin-web/src/style.css'
 await page.route('**/__configured-business',r=>r.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Component from'/@fs/${resolve(component)}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve(surfaceStyle)}';import'/@fs/${resolve('shared/src/controls.css')}';${routerSetup}createApp(Component,${JSON.stringify(props)})${installRouter}.mount('#app')</script></html>`}));await page.goto(origin+'/__configured-business')
}
async function noOverflow(page:Page){expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)}
for(const locale of ['en','zh-CN'] as const){
 test(`configured saved review and brand sales (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN',errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));let record:any=null;const calls:any[]=[]
  await page.route('**/api/**',route=>{
   const r=route.request(),path=new URL(r.url()).pathname;const ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(r.method()==='GET')return ok({items:record?[record]:[]})
   const body=r.postDataJSON();expect(r.headers()['idempotency-key']).toBeTruthy();calls.push(body)
   if(path.endsWith('/versions')){expect(body.cases[0].prize_points).toBe('25');record={id:versionID,game_id:game,revision:1,snapshot:body.snapshot,snapshot_hash:'frozen',cases:body.cases,status:'draft',validation:null}}
   else{expect(body.revision).toBe(record.revision);record={...record,revision:record.revision+1,status:({validate:'validated',submit:'submitted',approve:'approved',publish:'published'} as Record<string,string>)[body.action]}}
   return ok(record)
  })
  await mount(page,'shared/src/ConfiguredPublishing.vue',{surface:'platform',brandId:'',gameId:game,locale,snapshot:sample()})
  await page.getByLabel(zh?'独立预期验证用例（JSON）':'Independent validation cases (JSON)',{exact:true}).fill(JSON.stringify([caseInput()]))
  await page.getByRole('button',{name:zh?'保存草稿':'Save draft',exact:true}).click()
  await page.getByRole('combobox',{name:zh?'已保存版本':'Saved version',exact:true}).selectOption(versionID)
  for(const name of zh?['验证已保存草稿','提交审核','审核通过','发布已通过规则']:['Validate saved draft','Submit for review','Approve','Publish approved rules'])await page.getByRole('button',{name,exact:true}).click()
  await expect(page.getByRole('button',{name:zh?'保存草稿':'Save draft',exact:true})).toBeDisabled()
  expect(calls).toHaveLength(5);await noOverflow(page);await page.screenshot({path:info.outputPath(`publication-${locale}.png`),fullPage:true})
  await page.unroute('**/api/**');let savedSales:any=null
  await page.route('**/api/**',route=>{
   const r=route.request(),path=new URL(r.url()).pathname;const ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
   expect(r.headers()['x-brand-id']).toBe(game)
   if(path.endsWith('/orders'))return ok({items:[]})
   if(path.endsWith('/versions'))return ok({items:[record]})
   if(r.method()==='GET'&&savedSales)return ok({...savedSales,version:1})
   if(r.method()==='GET')return route.fulfill({status:404,json:{success:false,error:{code:'RESOURCE_NOT_FOUND',message:'No sales configured'}}})
   const body=r.postDataJSON();if(Number(body.prices.plates.A.lucky_price)>2.5)return route.fulfill({status:400,json:{success:false,error:{code:'PLATFORM_GAME_INVALID',message:'Invalid sales prices'}}})
   savedSales=body;return ok({...body,version:1})
  })
  await mount(page,'shared/src/ConfiguredPublishing.vue',{surface:'admin',brandId:game,gameId:game,locale,snapshot:null})
  await page.getByRole('combobox',{name:zh?'已保存版本':'Saved version',exact:true}).selectOption(versionID)
  await page.getByRole('button',{name:zh?'用选中发布版本建立销售草稿':'Use selected publication as sales draft',exact:true}).click()
  await page.getByRole('combobox',{name:zh?'启用':'Enabled',exact:true}).selectOption('true')
  for(const text of zh?['价格表','盘口','A']:['Price table','Plates','A'])await page.locator('summary').filter({hasText:new RegExp(`^${text}$`)}).first().click()
  const price=page.getByLabel('lucky_price',{exact:true}).first();await price.fill('3')
  await page.getByRole('button',{name:zh?'保存品牌销售':'Save brand sales',exact:true}).click();await expect(page.getByRole('alert')).toBeVisible()
  await price.fill('2.4');await page.getByRole('button',{name:zh?'保存品牌销售':'Save brand sales',exact:true}).click()
  await expect.poll(()=>savedSales?.prices.plates.A.lucky_price).toBe('2.4');expect(savedSales.rule_id).toBe(versionID);expect(savedSales.enabled).toBe(true)
  await noOverflow(page);expect(errors).toEqual([])
 })
 test(`configured real confirmation preserves uncertain intent across reload (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN',errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));const snapshot=sample(),input=caseInput().requests[0],quote={definition_hash:'definition',snapshot_hash:'frozen',requests:[input],lines:[{play_id:input.play_id,options:input.options,plate:'A',stake:'10',prices:{hit:'2.5'}}],bet_points:'10',game_id:game,period_id:game,rule_id:versionID,sales_version:1,actor_context:'signed-actor'}
  let changed=false,attempts=0,available='5';const writes:Array<{key:string|undefined;body:unknown}>=[];let orders:any[]=[]
  const receipt={member_name:'Trial member',id:versionID,game_id:game,period_id:game,period_no:'2026001',rule_id:versionID,sales_version:1,snapshot,quote,result:{bet_points:'10',prize_points:'25',refund_points:'0',effective_points:'10',lines:[{play_id:input.play_id,options:input.options,outcome:'won',prize_points:'25',refund_points:'0'}]},status:'settled',placed_at:'2026-10-20T00:00:00Z',settled_at:'2026-10-20T00:00:01Z'}
  await page.route('**/api/**',route=>{
   const r=route.request(),path=new URL(r.url()).pathname,ok=(data:unknown,status=200)=>route.fulfill({status,json:{success:true,data}})
   if(path.endsWith('/me/betting-preferences'))return ok({brand_id:game,member_id:game,quick_amounts:[]})
   if(path.endsWith('/platform-games/'+game))return ok({game:{id:game,name:'Lucky game'},plays:[],rule_protocol:'configured',periods:[]})
   if(path.endsWith('/wallet')){const balance=orders.length?'115':available;const by_source=Object.fromEntries(['recharge','winning','gift','commission'].map(source=>[source,{available:source==='recharge'?balance:'0',manual_frozen:'0',system_frozen:'0',withdrawal:'0'}]));return ok({account_id:game,brand_id:game,member_id:game,version:1,display_points:balance,available_points:balance,frozen_points:'0',withdrawal_points:'0',recharge_points:balance,winning_points:'0',gift_points:'0',commission_points:'0',manual_frozen_points:'0',system_frozen_points:'0',by_source})}
   if(path.endsWith('/orders')&&r.method()==='GET')return ok({items:orders})
   if(path.endsWith('/games/'+game)){const next=JSON.parse(JSON.stringify(snapshot));if(changed)next.prices.plates.A.lucky_price='2';return ok({game:{id:game,name:'Lucky game',status:'active',brand_enabled:true},snapshot:next,rule_id:versionID,snapshot_hash:changed?'new-frozen':'frozen',sales:{version:changed?2:1,enabled:true,rule_id:versionID,prices:next.prices},period:{id:game,period_no:'2026001',bet_start_at:new Date(Date.now()-3600000).toISOString(),bet_end_at:new Date(Date.now()+3600000).toISOString(),draw_at:new Date(Date.now()+3660000).toISOString()}})}
   if(path.endsWith('/preview')){expect(r.postDataJSON().requests).toEqual([input]);return ok(quote)}
   expect(path).toBe('/api/v1/configured/orders');writes.push({key:r.headers()['idempotency-key'],body:r.postDataJSON()});attempts++
   if(attempts===1){changed=true;return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Service unavailable'}}})}
   expect(writes[1]).toEqual(writes[0]);orders=[receipt];return ok(receipt,201)
  })
  const props={gameId:game,locale:zh?'zh':'en',auth:{user:{id:versionID},member:{id:game,brand_id:game}}}
  await mount(page,'user-web/src/ConfiguredBetting.vue',props)
  await page.locator('.configured-stakes--dock input[type="number"]').fill('10')
  await page.locator('.configured-options button').click()
  const bet=page.locator('.configured-stakes--dock').getByRole('button',{name:zh?'投注':'Bet',exact:true})
  await expect(bet).toBeDisabled()
  await expect(page.locator('.configured-funds-toast')).toContainText(zh?'可用积分不足':'Insufficient available points')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(writes).toHaveLength(0)
  available='100'
  await page.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')))
  await expect(bet).toBeEnabled()
  await expect(page.locator('.configured-funds-toast')).toHaveCount(0)
  await bet.click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await expect(page.getByRole('dialog')).toContainText('2.5')
  const confirm=page.getByRole('dialog').getByRole('button',{name:zh?'确认投注':'Confirm bet',exact:true})
  await expect(confirm).toBeEnabled()
  await confirm.click()
  await expect(page.getByRole('dialog').getByRole('button',{name:zh?'重试原请求':'Retry original request',exact:true})).toBeVisible()
  await page.reload()
  await page.getByRole('dialog').getByRole('button',{name:zh?'重试原请求':'Retry original request',exact:true}).click()
  await expect(page.getByText(zh?'投注成功':'Bet placed',{exact:true})).toBeVisible()
  await expect(page.locator('article')).toHaveCount(0);expect(orders[0].quote.lines[0].prices.hit).toBe('2.5');expect(orders[0].result.prize_points).toBe('25')
  expect(writes).toHaveLength(2);await noOverflow(page);await page.screenshot({path:info.outputPath(`configured-order-${locale}.png`),fullPage:true});expect(errors).toEqual([])
 })
}
