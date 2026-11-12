import { expect,test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import {configuredDemo} from '../../shared/src/configured-rules'
import { gameId,origins,installLayoutApi,expectNoPageOverflow } from './fixture'
const snapshot=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json','utf8')))
const play=snapshot.catalog.plays[0]!, option=snapshot.catalog.domains[play.selection.domain]!.options[0]!
const order={id:gameId,game_id:gameId,game_name:'Mark Six / 配置化六合彩',member_name:'visual_review_member',period_id:gameId,period_no:'20261028001',rule_id:gameId,sales_version:1,snapshot,
 quote:{bet_points:'10',lines:[{play_id:play.id,options:[option.id],stake:'10',prices:{hit:'2.5'},plate:'A',multiplier:1,combination_count:1}]},
 result:{prize_points:'25',refund_points:'0',effective_points:'10',lines:[{outcome:'won',prize_points:'25',refund_points:'0',effective_points:'10'}]},status:'settled',placed_at:'2026-10-28T02:00:00Z',settled_at:'2026-10-28T03:00:00Z'}
for(const surface of ['admin','platform'] as const)for(const locale of ['en','zh-CN'] as const)test(`configured order reader ${surface} (${locale})`,async({page},info)=>{
 const calls:string[]=[];const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 await page.route('**/api/v1/**',async route=>{
  const request=route.request();calls.push(`${request.method()} ${new URL(request.url()).pathname}`)
  expect(request.method()).toBe('GET');expect(request.headers()['idempotency-key']).toBeUndefined()
  expect(new URL(request.url()).pathname).toBe(`/api/v1/${surface}/configured-orders`)
  expect(request.headers()['x-brand-id']).toBe(gameId)
  await route.fulfill({json:{success:true,data:{items:[order,{...order,id:'bbbbbbbb-bbbb-4bbb-8bbb-000000000002',status:'placed',result:null,settled_at:null}], total_count: '2'}}})
 })
 const props={surface,brandId:gameId,locale}
 await page.route('**/configured-orders-harness',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><main id="app" class="management-console" style="padding:12px;min-width:0"></main><script id="harness-data" type="application/json">${JSON.stringify({component:'ConfiguredOrdersPanel',props,locale})}</script><script type="module" src="/tests/layout-harness.ts"></script></body></html>`}))
 await page.goto(`${origins.brand}/configured-orders-harness`)
 await expect(page.locator('tbody tr')).toHaveCount(2)
 const pagination=page.locator('.configured-orders .table-pagination')
 await expect(pagination.locator('.table-pagination-actions')).toHaveCSS('gap','12px')
 await expect(pagination.locator('.table-pagination-arrow')).toHaveCount(2)
 for(const button of await pagination.locator('.table-pagination-arrow').all())await expect(button).toBeDisabled()
 await expect(pagination.locator('[aria-current=page]')).toHaveAttribute('aria-label', /^(?:Page 1|第 1 页)$/)
 await expect(page.getByText('25',{exact:true})).toBeVisible()
 await expect(page.getByText('—',{exact:true})).toBeVisible()
 const refresh=page.getByRole('button',{name:locale==='en'?'Refresh':'刷新',exact:true})
 await expect(refresh).toHaveAttribute('data-refresh-button','true')
 await expect(refresh.locator('svg')).toBeVisible()
 await expect(page.locator('.configured-orders > .management-list-toolbar')).toBeVisible()
 const checkRefresh=async()=>{
  const button=await refresh.boundingBox(),actions=await page.locator('.configured-orders .console-page-actions').boundingBox()
  expect(button).not.toBeNull();expect(actions).not.toBeNull()
  expect(Math.abs(button!.x+button!.width-actions!.x-actions!.width)).toBeLessThanOrEqual(1)
  expect(button!.width).toBe(button!.height)
  if(info.project.name.startsWith('android')||info.project.name.startsWith('ios')||page.viewportSize()!.width<=900)expect(button!.height).toBeGreaterThanOrEqual(44)
 }
 await checkRefresh()
 await refresh.focus();await refresh.press('Enter')
 await expect.poll(()=>calls.length).toBe(2)
 await expect(page.getByRole('checkbox')).toHaveCount(0)
 if(info.project.name.endsWith('reflow'))for(const width of [320,480,768,1280]){await page.setViewportSize({width,height:800});await expectNoPageOverflow(page);await checkRefresh();if(width===320)await page.screenshot({path:info.outputPath(`configured-orders-320-${surface}-${locale}.png`),fullPage:true})}
 await expectNoPageOverflow(page);expect(errors).toEqual([])
 await page.screenshot({path:info.outputPath(`configured-orders-${surface}-${locale}.png`),fullPage:true})
})

for(const locale of ['en','zh-CN'] as const)test(`platform wallet keeps queries without maintenance tools (${locale})`,async({page})=>{
 await installLayoutApi(page,locale)
 await page.goto(origins.platform)
 await expect(page.getByRole('button',{name:locale==='en'?'Maintenance tools':'维护工具',exact:true})).toHaveCount(0)
 await expectNoPageOverflow(page)
})

for(const surface of ['admin','platform'] as const)for(const locale of ['en','zh-CN'] as const)test(`configured orders retain pages on failed reads ${surface} (${locale})`,async({page},info)=>{
 const reads:number[]=[];let fail=false,empty=false
 await page.route('**/api/v1/**',route=>{
  const request=route.request(),url=new URL(request.url());expect(request.method()).toBe('GET');expect(url.pathname).toBe(`/api/v1/${surface}/configured-orders`)
  expect(url.searchParams.get('limit')).toBe('50');const offset=Number(url.searchParams.get('offset'));reads.push(offset)
  if(fail){fail=false;return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic failure'}}})}
  const items=empty?[]:Array.from({length:offset===0?50:1},(_,i)=>({...order,id:`bbbbbbbb-bbbb-4bbb-8bbb-${String(offset+i).padStart(12,'0')}`,period_no:`period_${offset+i}`}))
  return route.fulfill({json:{success:true,data:{items,total_count:empty?'0':'51'}}})
 })
 const props={surface,brandId:gameId,locale}
 await page.route('**/configured-orders-pages',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><main id="app" class="management-console" style="padding:12px;min-width:0"></main><script id="harness-data" type="application/json">${JSON.stringify({component:'ConfiguredOrdersPanel',props,locale})}</script><script type="module" src="/tests/layout-harness.ts"></script></body></html>`}))
 await page.goto(origins.brand+'/configured-orders-pages')
 const list=page.locator('.configured-orders'),pager=list.locator('.table-pagination'),rows=list.locator('tbody tr'),next=pager.locator('.table-pagination-arrow').last()
 await expect(rows).toHaveCount(50);fail=true;await next.click();await expect(list.getByRole('alert')).toBeVisible()
 await expect(rows).toHaveCount(50);await expect(pager.locator('[aria-current=page]')).toHaveAttribute('aria-label', /^(?:Page 1|第 1 页)$/)
 await next.click();await expect(rows).toHaveCount(1);await expect(rows).toContainText('period_50');await expect(pager.locator('[aria-current=page]')).toHaveAttribute('aria-label', /^(?:Page 2|第 2 页)$/);await expect(next).toBeDisabled()
 await expect(list.locator('[data-refresh-button]')).toHaveCount(0);fail=true;await pager.locator('.table-pagination-arrow').first().click();await expect(list.getByRole('alert')).toBeVisible();await expect(rows).toContainText('period_50');await expect(pager.locator('[aria-current=page]')).toHaveAttribute('aria-label', /^(?:Page 2|第 2 页)$/)
 empty=true;await pager.locator('.table-pagination-arrow').first().click();await expect(list.locator('td.empty-state')).toBeVisible();await expect(list.locator('thead')).toHaveCount(1);await expect(pager).toHaveCount(0)
 expect(reads).toEqual([0,50,50,0,0]);await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`configured-empty-${surface}-${locale}.png`),fullPage:true})
})
