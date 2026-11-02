import {resolve} from 'node:path'
import {test,expect} from '@playwright/test'
import {installLayoutApi,origins,gameId,expectNoPageOverflow} from './fixture'
const note='用户要求现有品牌的认证设置和合规风控统一继承平台默认配置，保留完整创建原因以便后续核对。'.repeat(3)
for(const locale of ['en','zh-CN'] as const)test(`platform audit reasons (${locale})`,async({page},info)=>{
 await installLayoutApi(page,locale)
 await page.addInitScript(id=>sessionStorage.setItem('lottery.platform.selectedBrand',JSON.stringify({accountId:id,brandId:id})),gameId)
 await page.route('**/api/v1/platform/**',route=>{
  expect(route.request().method()).toBe('GET')
  const path=new URL(route.request().url()).pathname
  const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(path.endsWith('/presentation'))return reply({display_name:'Layout',logo_text:'L',logo_url:''})
  if(path.endsWith('/me'))return reply({account:{id:gameId,username:'reviewer',super_admin:true,platform_permissions:[]}})
  if(path.endsWith('/platform-settings'))return reply({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',display_name:'Layout',logo_text:'L',logo_url:'',version:1,updated_at:'2026-10-18T00:00:00Z'})
  if(path.endsWith('/brands'))return reply({items:[{id:gameId,code:'aurora',name:'Aurora',status:'active',member_count:'12',agent_count:'3',recharge_quota_available:'1000',created_at:'2026-10-24T00:00:00Z'}], total_count: '1'})
  if(path.endsWith('/audit'))return reply({items:['frozen configured rule settlement',note,''].map((reason,i)=>({id:String(i),brand_id:gameId,actor_type:'system',actor_id:'',actor_username:'',request_id:'',ip_address:'',before_json:null,after_json:null,action:'configured.settle',resource_type:'configured_order',resource_id:gameId,reason,created_at:'2026-10-27T04:00:00Z'})), total_count: '3'})
  throw Error(`Unexpected audit request ${path}`)
 })
 await page.goto(origins.platform)
 await expect(page.locator('.management-console')).toBeVisible()
 const menu=page.locator('button[aria-controls="platform-navigation"]')
 if(await menu.isVisible())await menu.click()
 await page.locator('.sidebar button[aria-expanded]').filter({hasText:locale==='zh-CN'?'运营':'Operations'}).click()
 await page.getByTestId('platform-operations-tabs').getByRole('button',{name:locale==='zh-CN'?'审计日志':'Audit log',exact:true}).click()
 const pagination=page.getByTestId('platform-audit-pages')
 await expect(pagination).toHaveClass(/table-pagination/)
 await expect(pagination.locator('.table-pagination-summary')).toHaveText(locale==='zh-CN'?'第 1 页 · 本页 3 条 · 共 3 条':'Page 1 · 3 records on this page · Total: 3')
 await expect(pagination.locator('..')).toHaveClass('panel')
 const reasons=page.locator('.reason-cell')
 await expect(reasons).toHaveCount(3)
 await expect(reasons.first()).toHaveText(locale==='zh-CN'?'按投注时锁定的规则结算':'Settlement using the rules locked at bet placement')
 await expect(reasons.nth(1)).toHaveAttribute('title',note)
 await expect(reasons.nth(1)).toHaveText(note)
 for(const width of info.project.name.endsWith('reflow')?[320,640,900,1440]:[page.viewportSize()!.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:900})
  await expect(reasons.nth(1)).toHaveCSS('white-space','normal')
  expect(await reasons.nth(1).evaluate(el=>el.scrollWidth<=el.clientWidth+1&&el.scrollHeight<=el.clientHeight+1)).toBe(true)
  await expect(pagination.locator('.table-pagination-actions')).toHaveCSS('gap','8px')
  await expectNoPageOverflow(page)
  await pagination.screenshot({path:info.outputPath(`audit-pagination-${locale}-${width}.png`)})
 }
 await page.screenshot({path:info.outputPath(`audit-reasons-${locale}.png`),fullPage:true})
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
 h(Controls,{page:number.value,count:count.value,total:total.value,locale:locale.value,previousLabel:'Previous',nextLabel:'Next',busy:false,hasPrevious:true,hasNext:true,previous:async()=>{number.value=1;count.value=0;total.value='0'},next:async()=>{number.value=2;count.value=3}}),
 h('button',{onClick:()=>locale.value='zh-CN'},'Language')])}}).mount('#app');
 </script></html>`}))
 await page.goto(origins.platform+'/__pagination-copy')
 const summaries=page.locator('.table-pagination-summary')
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
 await page.getByRole('button',{name:'Previous',exact:true}).click()
 await expect(summaries).toHaveText(['第 1 页 · 本页 0 条 · 共 0 条','第 1 页 · 本页 0 条 · 共 0 条'])
 expect(errors).toEqual([])
})
