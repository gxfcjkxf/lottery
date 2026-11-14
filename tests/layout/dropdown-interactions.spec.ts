import { test, expect, type Locator } from '@playwright/test'
import { resolve } from 'node:path'
import { origins, installLayoutApi, expectNoPageOverflow } from './fixture'

for(const locale of ['en','zh-CN'] as const){
 test(`four actual login language menus stay local and close correctly (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale)
  const activate=async(locator:Locator)=>info.project.use.hasTouch?locator.tap():locator.click()
  const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
  const widths=info.project.name.endsWith('reflow')?[320,1440]:[page.viewportSize()!.width]
  for(const surface of ['user','brand','platform','agent'] as const){
   for(const width of widths){
    if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:800})
   await page.goto(origins[surface]+(surface==='user'?'/login':''))
   const trigger=page.locator('.login-entry .locale-picker__trigger'), list=page.locator('.login-entry .locale-picker__list')
   await expect(trigger).toBeVisible();await expect(page.locator('.global-loading-overlay')).toBeHidden()
   await page.evaluate(()=>{
    const state={starts:0};(window as any).__dropdownLoading=state
    new MutationObserver(records=>{for(const record of records)for(const node of record.addedNodes)if(node instanceof Element&&(node.matches('.global-loading-overlay')||node.querySelector('.global-loading-overlay')))state.starts++}).observe(document.body,{childList:true,subtree:true})
   })
   await trigger.focus();await expect(trigger).toBeFocused()
   await activate(trigger);await expect(list).toBeVisible();await expect(trigger).toHaveAttribute('aria-expanded','true')
   await activate(list.locator('[lang="zh-CN"]'));await expect(list).toBeHidden();await expect(trigger).toBeFocused()
   await trigger.press('Enter');await expect(list).toBeVisible()
   await trigger.press('Tab');await expect(list.locator('button').first()).toBeFocused()
   await page.keyboard.press('Escape');await expect(list).toBeHidden();await expect(trigger).toBeFocused()
   await trigger.press('Space');await expect(list).toBeVisible()
   await trigger.press('Tab');await page.keyboard.press('Enter');await expect(list).toBeHidden();await expect(trigger).toBeFocused()
   await activate(trigger);await expect(list).toBeVisible()
   const outside=page.locator('.login-entry__topbar');if(info.project.use.hasTouch)await outside.tap({position:{x:2,y:2}});else await outside.click({position:{x:2,y:2}});await expect(list).toBeHidden()
   await activate(trigger);await trigger.press('Tab');await page.keyboard.press('Tab');await page.keyboard.press('Tab');await expect(list).toBeHidden()
   await activate(trigger);await activate(trigger);await expect(list).toBeHidden()
   await expect(page.locator('.global-loading-overlay')).toBeHidden()
   expect(await page.evaluate(()=>(window as any).__dropdownLoading.starts),surface).toBe(0)
   await expectNoPageOverflow(page)
   await page.screenshot({path:info.outputPath(`language-menu-${surface}-${locale}-${width}.png`)})
   }
  }
  expect(errors).toEqual([])
 })
 test(`shared dropdown interaction under four app styles (${locale})`,async({page},info)=>{
  await page.clock.install()
  const source=await(await page.request.get(origins.user+'/src/ConfiguredBetting.vue')).text(),vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
  const activate=async(locator:Locator)=>info.project.use.hasTouch?locator.tap():locator.click()
  const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/**',route=>{throw new Error(`Unexpected request ${route.request().url()}`)})
  const widths=info.project.name.endsWith('reflow')?[320,1440]:[page.viewportSize()!.width]
  for(const surface of ['user','brand','platform','agent'] as const){
   const directory=surface==='brand'?'admin-web':surface+'-web',url=origins.user+'/__dropdown-'+surface
   await page.route(url,route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/DropdownInteractionHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';${surface==='agent'?'':`import'/@fs/${resolve(directory+'/src/style.css')}';`}import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Harness,${JSON.stringify({surface,locale})}).mount('#app')</script></html>`}))
   for(const width of widths){
    if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:800})
   await page.goto(url)
   const custom=page.getByRole('combobox',{name:locale==='en'?'Custom selection':'共享选择',exact:true}), popup=page.getByRole('listbox')
   await activate(custom);await expect(popup).toBeVisible()
   await page.clock.fastForward(1000);await expect(popup).toBeVisible()
   await activate(popup.getByRole('option',{name:'Beta',exact:true}));await expect(popup).toBeVisible();await expect(page.getByTestId('custom-value')).toHaveText('a')
   await activate(popup.getByRole('option',{name:'Charlie',exact:true}));await expect(popup).toBeHidden();await expect(custom).toBeFocused();await expect(page.getByTestId('custom-value')).toHaveText('c')
   await custom.press('Enter');await custom.press('Home');await custom.press('ArrowDown');await custom.press('Enter');await expect(page.getByTestId('custom-value')).toHaveText('c')
   await custom.press('Space');await custom.press('End');await custom.press('Escape');await expect(popup).toBeHidden();await expect(page.getByTestId('custom-value')).toHaveText('c')
   await custom.press('a');await custom.press('Enter');await expect(page.getByTestId('custom-value')).toHaveText('a')
   await custom.press('End');await custom.press('Tab');await expect(popup).toBeHidden();await expect(page.getByTestId('custom-value')).toHaveText('d')
   await activate(custom);await activate(page.getByTestId('outside'));await expect(popup).toBeHidden();await expect(custom).toHaveAttribute('aria-expanded','false')
   await activate(custom);await expect(popup).toBeVisible()
   const box=(await popup.boundingBox())!;expect(box.x).toBeGreaterThanOrEqual(0);expect(box.x+box.width).toBeLessThanOrEqual(page.viewportSize()!.width+1)
   await page.screenshot({path:info.outputPath(`select-open-${surface}-${locale}-${width}.png`)})
   await custom.press('Escape')
   await activate(page.getByRole('button',{name:locale==='en'?'Disable':'禁用',exact:true}));await expect(custom).toBeDisabled();await expect(popup).toBeHidden()
   await expect(page.getByTestId('loading-starts')).toHaveText('0');await expect(page.locator('.global-loading-overlay')).toBeHidden();await expectNoPageOverflow(page)
   }
  }
  expect(errors).toEqual([])
 })
}
