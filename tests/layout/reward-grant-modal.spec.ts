import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,expectNoPageOverflow,expectInsideViewport} from './fixture'
const origin=origins.brand,brand='11111111-1111-4111-8111-111111111111',member='33333333-3333-4333-8333-333333333333'
for(const locale of ['en','zh-CN'] as const)test(`reward grant form opens from a button (${locale})`,async({page},info)=>{
 const searches:string[]=[];const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 await page.route('**/api/**',route=>{
  expect(route.request().method()).toBe('GET')
  const requestURL=new URL(route.request().url()),path=requestURL.pathname
  if(path.endsWith('/filter-options')){expect(requestURL.searchParams.get('action')).toBe('grant');expect(requestURL.searchParams.get('resource')).toBe('reward');searches.push(requestURL.searchParams.get('search')!)}
  const data=path.endsWith('/filter-options')?{brand_id:brand,items:[{id:member,name:'reward_user'}],limit:100,offset:0}:path.endsWith('/reward-orders')?{brand_id:brand,items:[],total_count:'0',limit:20,offset:0}:path.endsWith('/association-selection')?{id:member,member_id:member,brand_id:brand,username:'reward_user'}:null
  if(!data)throw Error(`Unexpected grant request ${path}`)
  return route.fulfill({json:{success:true,data}})
 })
 const source=await(await page.request.get(origin+'/src/RewardsManagement.vue')).text(),vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
 for(const readonly of [false,true]){
  const url=origin+'/__reward-grant-modal-'+readonly
  await page.route(url,route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/RewardGrantHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/src/style.css';import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';import'/@fs/${resolve('shared/src/refresh-button.css')}';createApp(Harness,${JSON.stringify({locale,readonly})}).mount('#app')</script></html>`}))
  await page.goto(url)
  const opener=page.getByTestId('rewards-grant-open')
  await expect(opener).toBeVisible()
  const initialViewport=page.viewportSize()!
  for(const width of info.project.name.includes('reflow')?[320,640,900,1440]:[initialViewport.width]){
   if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:initialViewport.height})
   const toolbar=page.locator('.rw-toolbar')
   await expect(toolbar.getByTestId('rewards-grant-open')).toBeVisible()
   await expect(toolbar.getByTestId('rewards-list-refresh')).toBeVisible()
   const right=await toolbar.evaluate(el=>el.getBoundingClientRect().right)
   const refreshRight=await page.getByTestId('rewards-list-refresh').evaluate(el=>el.getBoundingClientRect().right)
   expect(Math.abs(refreshRight-right)).toBeLessThanOrEqual(1)
   await expectNoPageOverflow(page)
   if(!readonly)await page.screenshot({path:info.outputPath(`grant-entry-${locale}-${width}.png`)})
  }
  if(info.project.name.includes('reflow'))await page.setViewportSize(initialViewport)
  await expect(page.getByTestId('rewards-grant-member')).toHaveCount(0)
  if(readonly){await expect(opener).toBeDisabled();continue}
  await opener.click()
  const dialog=page.getByRole('dialog',{name:locale==='en'?'Manual grant':'人工发放',exact:true})
  await expect(dialog).toBeVisible()
  await expect(page.getByTestId('rewards-grant-review')).toBeDisabled()
  await page.getByTestId('rewards-grant-member').fill('r')
  await page.waitForTimeout(400);expect(searches).not.toContain('r')
  await page.getByTestId('rewards-grant-member').fill('re')
  await expect(dialog.getByRole('button',{name:'reward_user',exact:true})).toBeVisible()
  await page.getByTestId('rewards-grant-points').fill('25')
  await expect(page.getByTestId('rewards-grant-review')).toBeDisabled()
  await expectInsideViewport(dialog);await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`grant-results-${locale}.png`)})
  await dialog.getByRole('button',{name:'reward_user',exact:true}).click()
  await page.getByTestId('rewards-grant-points').fill('25')
  const preset=page.viewportSize()!
  for(const width of info.project.name.includes('reflow')?[320,640,900,1440]:[preset.width]){
   if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectInsideViewport(dialog);await expectNoPageOverflow(page)
   await page.screenshot({path:info.outputPath(`grant-modal-${locale}-${width}.png`)})
  }
  await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0);await expect(opener).toBeFocused()
  await opener.click();await expect(page.getByTestId('rewards-grant-points')).toHaveValue('25')
  await page.getByTestId('rewards-grant-review').click()
  const review=page.getByTestId('rewards-review')
  await expect(review).toBeVisible();await expect(dialog).toHaveCount(0)
  await expect(page.getByTestId('rewards-confirm-submit')).toBeDisabled()
  await page.keyboard.press('Escape');await expect(review).toHaveCount(0);await expect(dialog).toBeVisible()
  await expect(page.getByTestId('rewards-grant-member')).toHaveValue('reward_user')
 }
 expect(errors).toEqual([])
})
