import {resolve} from 'node:path'
import {test,expect} from '@playwright/test'
import {installLayoutApi,origins,gameId,expectNoPageOverflow} from './fixture'
test.use({actionTimeout:10000})
const note='用户要求现有品牌的认证设置和合规风控统一继承平台默认配置，保留完整创建原因以便后续核对。'.repeat(3)
for(const locale of ['en','zh-CN'] as const)test(`platform audit reasons (${locale})`,async({page},info)=>{
 await page.clock.setFixedTime(new Date('2026-11-12T08:00:00Z'))
 await installLayoutApi(page,locale)
 await page.addInitScript(id=>sessionStorage.setItem('lottery.platform.selectedBrand',JSON.stringify({accountId:id,brandId:id})),gameId)
 await page.route('**/api/v1/platform/**',route=>{
  expect(route.request().method()).toBe('GET')
  const path=new URL(route.request().url()).pathname
  const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(path.endsWith('/presentation'))return reply({display_name:'Layout',logo_text:'L',logo_url:''})
  if(path.endsWith('/me'))return reply({account:{id:gameId,username:'reviewer',super_admin:true,platform_permissions:['audit.view.platform']}})
  if(path.endsWith('/platform-settings'))return reply({theme:'jade',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',display_name:'Layout',logo_text:'L',logo_url:'',version:1,updated_at:'2026-10-18T00:00:00Z'})
  if(path.endsWith('/brands'))return reply({items:[{id:gameId,code:'aurora',name:'Aurora',status:'active',member_count:'12',agent_count:'3',recharge_quota_available:'1000',created_at:'2026-10-24T00:00:00Z'}], total_count: '1'})
  if(path.endsWith('/audit'))return reply({items:['frozen configured rule settlement',note,''].map((reason,i)=>({id:String(i),brand_id:gameId,actor_type:'system',actor_id:'',actor_username:'',request_id:'',ip_address:'',action:'user.delete.maintenance',resource_type:'configured_order',resource_id:gameId,reason,created_at:'2026-10-27T04:00:00Z'})), total_count: '3'})
  throw Error(`Unexpected audit request ${path}`)
 })
 await page.goto(origins.platform)
 await expect(page.locator('.management-console')).toBeVisible()
 const menu=page.locator('button[aria-controls="platform-navigation"]')
 if(await menu.isVisible())await menu.click()
 await page.locator('.sidebar button[aria-expanded]').filter({hasText:locale==='zh-CN'?'运营':'Operations'}).click()
 await page.locator('.sidebar').getByRole('button',{name:locale==='zh-CN'?'审计日志':'Audit log',exact:true}).click()
 if(await menu.isVisible() && await menu.getAttribute('aria-expanded')==='true')await menu.click()
 const pagination=page.getByTestId('platform-audit-pages')
 await expect(pagination).toHaveClass(/table-pagination/)
 await expect(pagination.locator('.table-pagination-summary .table-pagination-desktop')).toHaveText(locale==='zh-CN'?'第 1 页 · 本页 3 条 · 共 3 条':'Page 1 · 3 records on this page · Total: 3')
 await expect(pagination.locator('..')).toHaveClass(/management-list/)
 const reasons=page.locator('.reason-cell')
 await expect(reasons).toHaveCount(3)
 await expect(reasons.first()).toHaveText(locale==='zh-CN'?'按投注时锁定的规则结算':'Settlement using the rules locked at bet placement')
 await expect(reasons.nth(1)).toHaveAttribute('title',note)
 await expect(reasons.nth(1)).toHaveText(note)
 for(const width of info.project.name.endsWith('reflow')?[320,640,900,1440]:[page.viewportSize()!.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:900})
  if(width<=1080)await expect(page.locator('.management-list table thead')).toBeHidden()
  await expect(reasons.nth(1)).toHaveCSS('white-space','normal')
  expect(await reasons.nth(1).evaluate(el=>el.scrollWidth<=el.clientWidth+1&&el.scrollHeight<=el.clientHeight+1)).toBe(true)
  await expectNoPageOverflow(page)
  await pagination.screenshot({path:info.outputPath(`audit-pagination-${locale}-${width}.png`)})
 }
 await page.evaluate(()=>window.scrollTo(0,0))
 await page.screenshot({path:info.outputPath(`audit-reasons-${locale}.png`),fullPage:true})
 const form=page.locator('.management-list-filters form')
 await expect(page.locator('.action-label').first()).toHaveText(locale==='zh-CN'?'会员 · 删除':'Members · Delete')
 await form.getByRole('searchbox',{name:locale==='zh-CN'?'操作人':'Operator',exact:true}).fill('reviewer')
 await form.getByRole('combobox',{name:locale==='zh-CN'?'类型':'Type',exact:true}).click()
 await page.getByRole('option',{name:locale==='zh-CN'?'删除':'Delete',exact:true}).click()
 for(const endpoint of ['from','to']) {
  await form.getByRole('button',{name:locale==='zh-CN'?(endpoint==='from'?'开始日期':'结束日期'):(endpoint==='from'?'Start date':'End date'),exact:true}).click()
  await page.getByRole('button',{name:'2026-11-12',exact:true}).click()
 }
 const request=page.waitForRequest(r=>r.url().includes('/audit?')&&new URL(r.url()).searchParams.get('actor')==='reviewer')
 await form.getByRole('button',{name:locale==='zh-CN'?'查询':'Search',exact:true}).click()
 const query=new URL((await request).url()).searchParams
 expect(query.get('action_type')).toBe('delete')
 expect(query.get('from')).toBe('2026-11-12T00:00:00.000Z')
 expect(query.get('to')).toBe('2026-11-13T00:00:00.000Z')
 await expect(form.getByRole('button',{name:locale==='zh-CN'?'清空':'Clear',exact:true})).toBeEnabled()
 const cleared=page.waitForRequest(r=>r.url().includes('/audit?')&&!new URL(r.url()).searchParams.has('actor'))
 await form.getByRole('button',{name:locale==='zh-CN'?'清空':'Clear',exact:true}).click()
 await cleared

})

// Exercise both pagination entry points with the same page/count state and a live locale change.
test('pagination copy stays consistent for full, partial and empty pages', async ({page},info)=>{
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
 await page.route('**/__pagination-copy',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">
 import{createApp,h,ref}from'/node_modules/.vite/deps/vue.js';
 import Pagination from'/@fs/${resolve('shared/src')}/TablePagination.vue';
 import Controls from'/@fs/${resolve('shared/src')}/PaginationControls.vue';
 import'/@fs/${resolve('shared/src')}/styles.css';
 import'/@fs/${resolve('shared/src')}/controls.css';
 createApp({setup(){const locale=ref('en'),number=ref(1),count=ref(50),total=ref('53');return()=>h('main',[
 h(Pagination,{page:number.value,count:count.value,total:total.value,locale:locale.value}),
 h(Controls,{page:number.value,count:count.value,total:total.value,locale:locale.value,previousLabel:'Previous',nextLabel:'Next',busy:false,hasPrevious:true,hasNext:true,pageSize:50,goTo:async(target)=>{number.value=target;count.value=target===2?3:50},changePageSize:async()=>{}}),
 h('button',{onClick:()=>locale.value='zh-CN'},'Language'),h('button',{onClick:()=>{number.value=1;count.value=0;total.value='0'}},'Empty')])}}).mount('#app');
 </script></html>`}))
 await page.goto(origins.platform+'/__pagination-copy')
 const summaries=page.locator('.table-pagination-summary .table-pagination-desktop')
 await expect(summaries).toHaveText(['Page 1 · 50 records on this page · Total: 53','Page 1 · 50 records on this page · Total: 53'])
 await page.getByRole('button',{name:'Next',exact:true}).click()
 await expect(summaries).toHaveText(['Page 2 · 3 records on this page · Total: 53','Page 2 · 3 records on this page · Total: 53'])
 await page.getByRole('button',{name:'Language',exact:true}).click()
 await expect(summaries).toHaveText(['第 2 页 · 本页 3 条 · 共 53 条','第 2 页 · 本页 3 条 · 共 53 条'])
 const preset=page.viewportSize()!
 for(const width of info.project.name.endsWith('reflow')?[320,640,1440]:[preset.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`pagination-copy-${width}.png`),fullPage:true})
 }
 await page.getByRole('button',{name:'Empty',exact:true}).click()
 await expect(page.locator('.table-pagination')).toHaveCount(0)
 expect(errors).toEqual([])
})

import { installConsoleSession } from './real-console-session'
for (const locale of ['en', 'zh-CN'] as const) test(`brand audit filters (${locale})`, async ({page}, info) => {
 const brand='11111111-1111-4111-8111-111111111111'
 const event='33333333-3333-4333-8333-333333333333'
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
 await page.route('**/api/**', route => {
  expect(route.request().method()).toBe('GET')
  const url=new URL(route.request().url())
  if(url.pathname==='/api/v1/admin/audit')return route.fulfill({json:{success:true,data:{items:[{id:event,brand_id:brand,action:'user.delete.maintenance',actor_type:'admin',actor_id:event,actor_username:'reviewer',resource_type:'global_user',resource_id:event,reason:'User requested removal of historical unnamed Telegram account',request_id:'review',created_at:'2026-11-12T04:00:00Z',ip_address:'127.0.0.1'}],total_count:'1'}}})
  throw Error(`Unexpected audit request ${url.pathname}`)
 })
 await installConsoleSession(page,'brand',locale,brand,['audit.view.brand'])
 await page.goto(origins.brand)
 await expect(page.locator('.management-console')).toBeVisible()
 const mobile=await page.locator('.mobile-nav').isVisible()
 if(mobile)await page.locator('.mobile-nav button').last().click()
 const nav=page.locator(mobile?'.mobile-more-menu':'.side-nav')
 await nav.locator('summary').filter({hasText:locale==='en'?'Administration':'系统管理'}).click()
 await nav.getByRole('button',{name:locale==='en'?'Audit log':'审计日志',exact:true}).click()
 await expect(page.locator('.audit-table')).toContainText(locale==='zh-CN'?'会员 · 删除':'Members · Delete')
 await expect(page.locator('.audit-table')).toContainText(locale==='zh-CN'?'用户请求删除历史未命名 Telegram 账号':'User requested removal of historical unnamed Telegram account')
 const form=page.locator('.management-list-filters form')
 await form.getByRole('searchbox',{name:locale==='zh-CN'?'操作人':'Operator',exact:true}).fill('reviewer')
 await form.getByRole('combobox',{name:locale==='zh-CN'?'类型':'Type',exact:true}).click()
 await page.getByRole('option',{name:locale==='zh-CN'?'查询':'Query',exact:true}).click()
 const request=page.waitForRequest(r=>r.url().includes('/audit?')&&new URL(r.url()).searchParams.get('actor')==='reviewer')
 await form.getByRole('button',{name:locale==='zh-CN'?'查询':'Search',exact:true}).click()
 expect(new URL((await request).url()).searchParams.get('action_type')).toBe('query')
 await expect(form.getByRole('button',{name:locale==='zh-CN'?'清空':'Clear',exact:true})).toBeEnabled()
 for(const width of info.project.name.endsWith('reflow')?[320,640,900,1440]:[page.viewportSize()!.width]) {
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:900})
  await expectNoPageOverflow(page)
 }
 await page.screenshot({path:info.outputPath(`audit-filters-brand-${locale}.png`),fullPage:true})
 expect(errors).toEqual([])
})
