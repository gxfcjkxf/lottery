import { selectControl } from '../select-control'
import {expect,test} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,gameId,expectNoPageOverflow,expectInsideViewport} from './fixture'
const orderId='bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
for(const locale of ['en','zh-CN'] as const)test(`manual calculation has compact dividers and separate actions (${locale})`,async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',cause=>errors.push(cause.message))
 await page.route('**/api/**',route=>{
  expect(route.request().method()).toBe('GET')
  const url=new URL(route.request().url()),path=url.pathname
  if(path.endsWith('/commission-cycles/candidates'))return route.fulfill({json:{success:true,data:{brand_id:gameId,items:[{anchor_order_id:orderId,window_from:'2026-10-05T16:00:00Z',window_to:'2026-10-12T16:00:00Z',cycle:'weekly'}],limit:100,offset:0}}})
  if(path.endsWith('/commission-cycles'))return route.fulfill({json:{success:true,data:{brand_id:gameId,items:[],total_count:'0',limit:20,offset:0}}})
  if(path.endsWith('/filter-options'))return route.fulfill({json:{success:true,data:{brand_id:gameId,items:url.searchParams.get('kind')==='anchor_order'?[{id:orderId,name:'20261101 · review_member'}]:[],limit:100,offset:0}}})
  throw Error('Unexpected modal read '+path)
 })
 const source=await(await page.request.get(origins.brand+'/src/CommissionCyclesManagement.vue')).text()
 const vue=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
 await page.route('**/__manual-spacing',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vue}';import Harness from'/@fs/${resolve('tests/layout/ManualCalculationHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/src/style.css';import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';import'/@fs/${resolve('shared/src/refresh-button.css')}';createApp(Harness,{locale:'${locale}'}).mount('#app')</script></html>`}))
 await page.goto(origins.brand+'/__manual-spacing')
 await page.getByRole('button',{name:locale==='en'?'Add missing calculation':'补建核算',exact:true}).click()
 const dialog=page.getByRole('dialog'),select=dialog.getByRole('combobox'),review=dialog.getByRole('button',{name:locale==='en'?'Review cycle':'核对周期',exact:true})
 await expect(select).toBeEnabled();await expect(review).toBeDisabled()
 const preset=page.viewportSize()!
 for(const width of info.project.name.endsWith('reflow')?[320,620,900,901,1440]:[preset.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
  const mobile=await page.evaluate(()=>matchMedia('(max-width:900px), (pointer:coarse)').matches)
  const layout=await dialog.evaluate(el=>{
   const heading=el.querySelector('.detail-modal-heading')!,content=el.querySelector('.detail-modal-content')!,field=content.querySelector('label')!,action=content.querySelector('.ui-form-actions button')!
   const h=heading.getBoundingClientRect(),c=content.getBoundingClientRect(),f=field.getBoundingClientRect(),a=action.getBoundingClientRect()
   return {headingMargin:getComputedStyle(heading).margin,dividerGap:c.top-h.bottom,contentTop:f.top-c.top,actionGap:a.top-f.bottom,actionRight:c.right-parseFloat(getComputedStyle(content).paddingRight)-a.right,headingPadding:getComputedStyle(heading).paddingTop,fieldGap:getComputedStyle(field).rowGap,actionHeight:a.height}
  })
  expect(layout).toMatchObject({headingMargin:'0px',dividerGap:0,contentTop:mobile?8:12,actionGap:mobile?8:12,headingPadding:mobile?'4px':'8px',fieldGap:mobile?'4px':'6px'})
  expect(Math.abs(layout.actionRight)).toBeLessThanOrEqual(1)
  expect(layout.actionHeight).toBeGreaterThanOrEqual(info.project.name.endsWith('reflow')&&width>900?32:44)
  expect((await dialog.boundingBox())!.width).toBeLessThanOrEqual(520)
  await expectInsideViewport(dialog);await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`manual-spacing-${locale}-${width}.png`)})
 }
 await selectControl(select,orderId)
 await expect(review).toBeEnabled();await review.click()
 await expect(page.getByRole('dialog')).toHaveCount(1)
 await expect(page.getByRole('dialog')).not.toContainText(orderId)
 await expect(page.getByRole('dialog')).toContainText('2026-10-06')
 await page.keyboard.press('Escape');await expect(page.getByRole('dialog')).toHaveCount(0)
 expect(errors).toEqual([])
})
