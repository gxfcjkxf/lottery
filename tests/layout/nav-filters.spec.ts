import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,expectNoPageOverflow} from './fixture'
for(const locale of ['en','zh-CN'] as const){
 test(`navigation titles, audit filters and member account search (${locale})`,async({page},info)=>{
  const source=await(await page.request.get(origins.brand+'/src/ReportsManagement.vue')).text(),vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
  const requests:URL[]=[];const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/**',route=>{
   const url=new URL(route.request().url());requests.push(url)
   const data=url.pathname.endsWith('filter-options')?{brand_id:'11111111-1111-4111-8111-111111111111',items:url.searchParams.has('search')?[{id:'33333333-3333-4333-8333-333333333333',name:'member_trial'}]:[],limit:100,offset:0}:{items:[]}
   return route.fulfill({json:{success:true,data}})
  })
  const widths=info.project.name.endsWith('reflow')?[320,700,1440]:[page.viewportSize()!.width]
  for(const view of ['audit','reports','platform']){
   const url=origins.brand+'/__nav-filters-'+view
   await page.route(url,route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/NavFiltersHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve('admin-web/src/style.css')}';import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Harness,${JSON.stringify({locale,view})}).mount('#app')</script></html>`}))
   for(const width of widths){
    if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:800})
    await page.goto(url);await expect(page.locator('.topbar')).toBeVisible()
    const content=page.locator('main')
    const startDate=(locale==='en'?'Start date':'开始日期')
    const endDate=(locale==='en'?'End date':'结束日期')
    await expect(content.getByRole('button',{name:startDate,exact:true})).toBeVisible()
    await expect(content.locator('.platform-date-input-display').first()).toHaveAttribute('placeholder',startDate)
    await expect(content.getByRole('button',{name:endDate,exact:true})).toBeVisible()
    await expect(content.locator('.ui-filter-bar label').filter({hasText:locale==='en'?/^Start date|^End date/:/^开始日期|^结束日期/})).toHaveCount(0)
    if(view==='audit'){
     await expect(content.locator('.audit-heading')).toHaveCount(0)
     const type=content.getByRole('combobox',{name:locale==='en'?'Type':'类型',exact:true})
     await expect(type).toBeVisible()
     expect(requests.some(u=>u.searchParams.get('kind')==='audit_resource')).toBe(false)
     const actor=page.getByRole('searchbox');await actor.fill('reviewer')
     await expect(actor).toHaveValue('reviewer')
     await type.click()
     await page.getByRole('option',{name:locale==='en'?'Edit':'编辑',exact:true}).click()
     const auditRequest=page.waitForRequest(request=>{const url=new URL(request.url());return url.pathname.endsWith('/audit')&&url.searchParams.get('actor')==='reviewer'})
     await content.getByRole('button',{name:locale==='en'?'Search':'查询',exact:true}).click()
     expect(new URL((await auditRequest).url()).searchParams.get('action_type')).toBe('edit')
     expect(requests.some(url=>url.pathname.endsWith('filter-options')&&url.searchParams.get('search')==='reviewer')).toBe(false)
    }else{
     const search=page.getByRole('searchbox').first();await search.fill('member')
     if(view==='reports')await expect(page.getByRole('button',{name:locale==='en'?'Query':'查询',exact:true})).toBeDisabled()
     await expect(search.locator('xpath=../..').getByRole('button',{name:locale==='en'?'Search':'搜索',exact:true})).toHaveCount(0)
     await expect(page.getByRole('option',{name:'member_trial',exact:true})).toBeVisible()
     await page.getByRole('option',{name:'member_trial',exact:true}).click()
     await expect(search).toHaveValue('member_trial')
     if(view==='reports'){
      await page.getByRole('button',{name:locale==='en'?'Query':'查询',exact:true}).click()
      await expect.poll(()=>requests.some(u=>u.pathname.includes('/reports/')&&u.searchParams.get('member_id')==='33333333-3333-4333-8333-333333333333')).toBe(true)
     }
     expect(requests.some(u=>u.searchParams.get('search')==='member')).toBe(true)
     await search.locator('xpath=..').getByRole('button',{name:locale==='en'?'Clear input':'清除输入',exact:true}).click();await expect(search).toHaveValue('')
    }
    await expectNoPageOverflow(page)
    await page.screenshot({path:info.outputPath(`${view}-${locale}-${width}.png`),fullPage:true})
   }
  }
  expect(errors).toEqual([])
 })
}
