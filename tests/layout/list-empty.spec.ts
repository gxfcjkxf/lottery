import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,expectNoPageOverflow} from './fixture'
const brand='11111111-1111-4111-8111-111111111111'
const fields={commission:['entry_count','paid_entry_count','paid_points','adjustment_entry_count','adjustment_credit_points','adjustment_debit_points','correction_entry_count','correction_credit_points','correction_debit_points','net_points'],withdrawal:['order_count','requested_points','reviewing_count','reviewing_points','processing_count','processing_points','paid_count','paid_points','rejected_count','rejected_points','failed_count','failed_points','cancelled_count','cancelled_points'],rewards:['entry_count','grant_entry_count','grant_points','reversal_entry_count','reversal_points','net_points']}
for(const locale of ['en','zh-CN'] as const)test(`management empty lists retain headers, pagination and clear pending searches (${locale})`,async({page},info)=>{
 test.setTimeout(120_000)
 const errors:string[]=[],reads:URL[]=[];page.on('pageerror',e=>errors.push(e.message))
 await page.route('**/api/**',route=>{
  const req=route.request(),url=new URL(req.url()),q=url.searchParams;expect(req.method()).toBe('GET');reads.push(url)
  const reply=(data:unknown)=>route.fulfill({json:{success:true,request_id:'list-empty-review',data}})
  if(url.pathname.endsWith('/recharge-quota'))return reply({brand_id:brand,available:'0'})
  if(url.pathname.endsWith('/filter-options'))return reply({brand_id:brand,items:[],limit:100,offset:0})
  if(url.pathname==='/api/v1/platform/recharges')return reply({items:[],total_count:'0'})
  if(url.pathname.includes('/reports/')){
   const kind=url.pathname.split('/').at(-1) as keyof typeof fields
   const query={from:q.get('from'),to:q.get('to'),group_by:q.get('group_by'),limit:Number(q.get('limit')),offset:Number(q.get('offset')),member_id:q.get('member_id'),...(kind==='commission'?{agent_id:q.get('agent_id'),cycle_id:q.get('cycle_id')}:kind==='withdrawal'?{game_id:null}:{order_id:q.get('order_id')})}
   return reply({brand_id:brand,snapshot_at:'2026-11-02T00:00:00Z',timezone:'Asia/Singapore',query,summary:Object.fromEntries(fields[kind].map(key=>[key,'0'])),items:[],total_groups:'0'})
  }
  if(['/api/v1/admin/audit','/api/v1/admin/reward-orders','/api/v1/platform/reward-orders','/api/v1/admin/withdrawals','/api/v1/platform/withdrawals','/api/v1/admin/recharges'].includes(url.pathname))return reply({brand_id:brand,items:[],total_count:'0',...(url.pathname.endsWith('/withdrawals')?{has_more:false}:{}),limit:Number(q.get('limit')??50),offset:0})
  throw Error('Unconfigured empty-list component request: '+url.pathname)
 })
 const source=await(await page.request.get(origins.brand+'/src/RewardsManagement.vue')).text(),vue=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
 for(const view of ['audit','reward-report','commission-report','withdrawal-report','brand-rewards','brand-withdrawals','brand-recharges','platform-rewards','platform-withdrawals','platform-recharges']){
  await page.route('**/__list-empty',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta name="viewport" content="width=device-width,initial-scale=1"><main id="app"></main><script type="module">import{createApp}from'${vue}';import C from'/@fs${resolve('tests/layout/ListEmptyHarness.vue')}';import'/@fs${resolve('shared/src/styles.css')}';import'/src/style.css';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/empty-state.css')}';createApp(C,${JSON.stringify({locale,view})}).mount('#app')</script></html>`}))
  await page.goto(origins.brand+'/__list-empty')
  const list=page.locator('.management-list').first(),pager=list.locator('.table-pagination').first()
  if(view.endsWith('-report'))await list.locator('button[type=submit]').click()
  await expect(list.locator('td.empty-state').first(),view).toBeVisible();await expect(list.locator('thead').first(),view).toHaveCount(1)
  await expect(pager,view).toBeVisible();await expect(pager).toContainText(locale==='en'?'Total: 0':'共 0 条')
  const bar=list.locator('.ui-filter-bar'),search=bar.getByRole('searchbox').first()
  if(await search.count()){
   await search.fill('unfinished_lookup');const before=reads.filter(url=>!url.pathname.endsWith('/filter-options')).length
   await bar.locator(':scope > button, :scope > .audit-actions > button').filter({hasText:locale==='en'?/^Clear$/:/^清空$/}).click();await expect(search).toHaveValue('')
   await expect.poll(()=>reads.filter(url=>!url.pathname.endsWith('/filter-options')).length,{timeout:10_000}).toBeGreaterThan(before)
   await expect(pager).toBeVisible()
  }
  const preset=page.viewportSize()!
  for(const width of info.project.name.endsWith('reflow')?[320,900,1440]:[preset.width]){if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height});await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`${view}-${locale}-${width}.png`),fullPage:true})}
 }
 expect(errors).toEqual([])
})
