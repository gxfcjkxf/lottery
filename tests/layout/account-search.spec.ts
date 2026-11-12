import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'
import { origins, expectNoPageOverflow } from './fixture'
const brand='11111111-1111-4111-8111-111111111111',id='33333333-3333-4333-8333-333333333333'
for(const locale of ['en','zh-CN'] as const)for(const view of ['cycles'] as const){
 test(`account search on ${view} (${locale})`,async({page},info)=>{
  const source=await(await page.request.get(origins.brand+'/src/ReportsManagement.vue')).text(),vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
  const requests:URL[]=[],errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
  let fail=false
  await page.route('**/api/**',async route=>{
   const url=new URL(route.request().url());requests.push(url)
   expect(route.request().method()).toBe('GET')
   if(url.pathname.endsWith('/filter-options')){
    expect(route.request().headers()['x-brand-id']).toBe(brand)
    const kind=url.searchParams.get('kind'),search=url.searchParams.get('search')
    if(['agent','member','actor'].includes(kind!))expect(search).toBeTruthy()
    if(search&&fail)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Search failed'}}})
    const offset=Number(url.searchParams.get('offset'))
    const items=!search||search==='no_match'?[]:offset?[{id,name:'target_agent'}]:Array.from({length:100},(_,i)=>({id:`33333333-3333-4333-8333-${String(i+1).padStart(12,'0')}`,name:`agent_${String(i).padStart(3,'0')}`}))
    return route.fulfill({json:{success:true,data:{brand_id:brand,items,limit:100,offset}}})
   }
   return route.fulfill({json:{success:true,data:{items:[],total_count:'0',limit:20,offset:0}}})
  })
  const url=origins.brand+'/__account-search-'+view
  await page.route(url,route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/NavFiltersHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve('admin-web/src/style.css')}';import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Harness,${JSON.stringify({locale,view})}).mount('#app')</script></html>`}))
  const widths=info.project.name.endsWith('reflow')?[320,700,1440]:[page.viewportSize()!.width]
  for(const width of widths){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:800})
   await page.goto(url);requests.length=0
   const input=page.getByRole('searchbox',{name:locale==='en'?/^Agent(?: account)?$/:/^代理(?:账号)?$/}).first()
   await expect(input).toBeVisible()
   const form=input.locator('xpath=ancestor::form[1]')
   const before=await form.boundingBox()
   await input.fill('t')
   await page.waitForTimeout(400)
   expect(requests.some(u=>u.pathname.endsWith('/filter-options')&&u.searchParams.get('search')==='t')).toBe(false)
   await input.fill('ta')
   await expect(input.locator('xpath=ancestor::span[@class="association-search"]').getByRole('button',{name:locale==='en'?'More results':'更多结果',exact:true})).toBeVisible()
   expect(await form.boundingBox()).toEqual(before)
   const popup=input.locator('xpath=ancestor::span[@class="association-search"]').getByRole('listbox')
   await expect(popup).toHaveJSProperty('popover','auto')
   await expect(popup).toHaveCSS('position','fixed')
   const bounds=await popup.boundingBox()
   expect(bounds!.x).toBeGreaterThanOrEqual(0)
   expect(bounds!.y).toBeGreaterThanOrEqual(0)
   expect(bounds!.height).toBeLessThanOrEqual(240)
   expect(bounds!.x+bounds!.width).toBeLessThanOrEqual(page.viewportSize()!.width)
   expect(bounds!.y+bounds!.height).toBeLessThanOrEqual(page.viewportSize()!.height)
   await page.screenshot({path:info.outputPath(`account-results-${locale}-${width}.png`),fullPage:true})
   await input.fill('target')
   const control=input.locator('xpath=ancestor::span[@class="association-search"]')
   const query=input.locator('xpath=ancestor::form[1]').locator('button[type="submit"]')
   await expect(query).toBeDisabled()
   await expect(control.getByRole('button',{name:locale==='en'?'Search':'搜索',exact:true})).toHaveCount(0)
   await control.getByRole('button',{name:locale==='en'?'More results':'更多结果',exact:true}).click()
   await control.getByRole('option',{name:'target_agent',exact:true}).click();await expect(input).toHaveValue('target_agent')
   await expect(control.getByRole('listbox')).toBeHidden()
   await expect(query).toBeEnabled()
   expect(requests.some(u=>!u.pathname.endsWith('/filter-options')&&u.searchParams.get('agent_id')===id)).toBe(false)
   await query.click()
   await expect.poll(()=>requests.some(u=>!u.pathname.endsWith('/filter-options')&&u.searchParams.get('agent_id')===id)).toBe(true)
   expect(requests.some(u=>u.searchParams.get('kind')==='agent'&&u.searchParams.get('search')==='target'&&u.searchParams.get('offset')==='100')).toBe(true)
   await input.fill('keyboard')
   await expect(control.getByRole('option').first()).toBeVisible()
   await input.press('ArrowDown');await input.press('Escape')
   await expect(control.getByRole('listbox')).toBeHidden()
   await input.press('ArrowDown');await input.press('Enter')
   await expect(input).toHaveValue('agent_000')
   await expect(input).toBeFocused()
   await expect(control.getByRole('listbox')).toBeHidden()
   await control.getByRole('button',{name:locale==='en'?'Clear input':'清除输入',exact:true}).click();await expect(input).toHaveValue('')
   await input.fill('no_match');await expect.poll(()=>requests.some(u=>u.searchParams.get('search')==='no_match')).toBe(true);await expect(control.getByRole('status')).toHaveCount(0);await expect(query).toBeDisabled()
   fail=true;await input.fill('failure');await expect(control.getByRole('alert')).toBeVisible()
   fail=false;await control.getByRole('button',{name:locale==='en'?'Retry':'重试',exact:true}).click();await expect(control.getByRole('alert')).toHaveCount(0)
   await control.getByRole('button',{name:locale==='en'?'Clear input':'清除输入',exact:true}).click();await expect(input).toHaveValue('');await expect(query).toBeEnabled();await expect(page.getByRole('button',{name:locale==='en'?/^(Clear|Reset)$/:/^(清除|清空|重置)$/,exact:true})).toHaveCount(0)
   await expectNoPageOverflow(page)
   await page.screenshot({path:info.outputPath(`${view}-${locale}-${width}.png`),fullPage:true})
  }
  expect(errors).toEqual([])
 })
}

for(const locale of ['en','zh-CN'] as const)test(`finance member selectors submit selected accounts (${locale})`,async({page},info)=>{
 const requests:URL[]=[],errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 const source=await(await page.request.get(origins.brand+'/src/RewardsManagement.vue')).text(),vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
 await page.route('**/api/**',route=>{
  expect(route.request().method()).toBe('GET')
  const url=new URL(route.request().url());requests.push(url)
  const data=url.pathname.endsWith('/filter-options')?{brand_id:brand,items:[{id,name:'test_member'}],limit:100,offset:0}:url.pathname.endsWith('/recharge-quota')?{brand_id:brand,available:'0'}:url.pathname.endsWith('/withdrawals')?{brand_id:brand,items:[],has_more:false,limit:Number(url.searchParams.get('limit')),offset:0}:{items:[]}
  return route.fulfill({json:{success:true,data}})
 })
 for(const view of ['brand-recharges','brand-withdrawals','platform-recharges','platform-withdrawals']){
  const url=origins.brand+'/__finance-account-search-'+view
  await page.route(url,route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/FinanceConsistencyHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';import'/@fs/${resolve(view.startsWith('brand')?'admin-web/src/style.css':'platform-web/src/style.css')}';createApp(Harness,${JSON.stringify({view,locale})}).mount('#app')</script></html>`}))
  await page.goto(url)
  const input=page.getByRole('searchbox'),query=page.locator('form button[type=submit],form button:not([type])').first()
  await expect(input).toBeVisible();requests.length=0
  const before=await input.locator('xpath=ancestor::form[1]').boundingBox()
  await input.fill('te');await expect(query).toBeDisabled()
  await expect(page.getByRole('option',{name:'test_member',exact:true})).toBeVisible()
  expect(await input.locator('xpath=ancestor::form[1]').boundingBox()).toEqual(before)
  await page.getByRole('option',{name:'test_member',exact:true}).click()
  await expect(page.getByRole('listbox')).toBeHidden()
  await expect(query).toBeEnabled();await query.click()
  await expect.poll(()=>requests.some(u=>u.searchParams.get('member_id')===id)).toBe(true)
  expect(requests.some(u=>u.pathname.endsWith('/users'))).toBe(false)
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`${view}-${locale}.png`),fullPage:true})
 }
 expect(errors).toEqual([])
})
