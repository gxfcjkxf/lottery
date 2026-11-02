import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'
import { origins, expectNoPageOverflow, installLayoutApi } from './fixture'
export function registerControlSizingTests(reflow = false) {
 test('four application login fields share responsive control heights',async({page})=>{
  await installLayoutApi(page,'en')
  const preset=page.viewportSize()!
  for(const surface of ['user','brand','platform','agent'] as const){
   await page.goto(origins[surface]+(surface==='user'?'/login':''))
   const fields=page.locator('.login-entry input:not([type="hidden"]),.login-entry .select-control-trigger,.login-entry form .button')
   await expect(fields.first()).toBeVisible()
   for(const width of reflow?[320,900,901,1440]:[preset.width]){
    if(reflow)await page.setViewportSize({width,height:preset.height})
    const desktop=await page.evaluate(()=>matchMedia('(min-width:901px) and (pointer:fine)').matches)
    for(const field of await fields.all())if(await field.isVisible())expect((await field.boundingBox())!.height).toBe(desktop?32:44)
    await expectNoPageOverflow(page)
   }
  }
 })
 for (const surface of ['user','brand','platform','agent'] as const) test(`shared ${surface} controls fit desktop content and mobile viewports`, async ({page},info) => {
  const directory = surface === 'brand' ? 'admin-web' : surface + '-web'
  await page.route('**/__control-sizing',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><style>.control-demo{box-sizing:border-box;width:min(900px,100%);margin:0 auto;padding:16px;display:grid;gap:16px}.control-demo label{display:grid;gap:6px;min-width:0}.control-demo .actions{display:grid;grid-template-columns:1fr;gap:8px}</style><body><div id="app" class="${surface==='user'?'':'management-console'}"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import Harness from'/@fs${resolve('tests/layout/ControlSizingHarness.vue')}';createApp(Harness).mount('#app');import'/@fs${resolve('shared/src/styles.css')}';${surface==='agent'?'':`import'/@fs${resolve(directory+'/src/style.css')}';`}import'/@fs${resolve('shared/src/controls.css')}';${surface==='user'?'':`import'/@fs${resolve('shared/src/management.css')}';`}</script></body></html>`}));
  await page.goto(origins[surface]+'/__control-sizing')
  const preset=page.viewportSize()!
  for(const width of reflow?[320,640,899,900,901,999,1000,1001,1440]:[preset.width]){
   if(reflow)await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
   const desktop=await page.evaluate(()=>matchMedia('(min-width:901px) and (pointer:fine)').matches)
   for(const control of await page.locator('input:not([data-select-validation]),.select-control-trigger,.actions button').all()) {
    expect((await control.boundingBox())!.height).toBe(desktop?32:44)
   }
   if(desktop){
    const parent=(await page.locator('label').first().boundingBox())!.width
    for(const name of ['Account','Email','Points'])expect((await page.getByLabel(name,{exact:true}).boundingBox())!.width).toBeLessThan(parent-80)
    const normal=(await page.getByLabel('Account',{exact:true}).boundingBox())!.width,email=(await page.getByLabel('Email',{exact:true}).boundingBox())!.width,number=(await page.getByLabel('Points',{exact:true}).boundingBox())!.width
    expect(email).toBeGreaterThan(normal);expect(number).toBeLessThan(normal)
    for(const button of await page.locator('.actions button').all())expect((await button.boundingBox())!.width).toBeLessThan(200)
    const select=page.getByRole('combobox',{name:'Status',exact:true})
    const optionFits=await select.evaluate(element=>{
     const value=element.querySelector('.select-control-value')!,icon=element.querySelector('.site-icon')!
     return value.scrollWidth<=value.clientWidth+1&&value.getBoundingClientRect().right<=icon.getBoundingClientRect().left
    })
    expect(optionFits).toBe(true)
   }
  }
  const save=page.getByRole('button',{name:'Save changes',exact:true});await save.scrollIntoViewIfNeeded();const before=await save.boundingBox()
  await save.hover();await save.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))))
  const after=await save.boundingBox();expect(after!.x).toBeCloseTo(before!.x,1);expect(after!.y).toBeCloseTo(before!.y,1)
  await expect(save).toHaveCSS('background-image','none')
  await page.emulateMedia({ reducedMotion: 'reduce' })
  expect(await save.evaluate(el => Math.max(...getComputedStyle(el).transitionDuration.split(',').map(value => parseFloat(value))))).toBeLessThan(0.001)
  await page.screenshot({path:info.outputPath(`controls-${surface}.png`)})
 })
 for (const surface of ['user','brand','platform','agent'] as const) test(`shared ${surface} action menu rows use desktop and touch heights`, async ({page},info) => {
  const directory = surface === 'brand' ? 'admin-web' : surface + '-web'
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.route('**/__menu-sizing', route => route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><body><div id="app" class="${surface==='user'?'':'management-console'}"></div><script type="module">
   import{createApp,h}from'/node_modules/.vite/deps/vue.js';
   import MemberActionsMenu from'/@fs${resolve('shared/src/MemberActionsMenu.vue')}';
   import AccountMenu from'/@fs${resolve('shared/src/AccountMenu.vue')}';
   import LanguageSwitcher from'/@fs${resolve('shared/src/LanguageSwitcher.vue')}';
   import'/@fs${resolve('shared/src/styles.css')}';
   ${surface==='agent'?'':`import'/@fs${resolve(directory+'/src/style.css')}';`}
   import'/@fs${resolve('shared/src/controls.css')}';
   ${surface==='user'?'':`import'/@fs${resolve('shared/src/management.css')}';`}
   createApp({render:()=>h('main',{style:'padding:16px;display:grid;gap:12px;max-width:480px;box-sizing:border-box'},[
    h(MemberActionsMenu,{id:'row-actions',label:'Member actions'},{default:()=>[h('button',{type:'button',role:'menuitem'},'编辑'),h('button',{type:'button',role:'menuitem'},'踢出')]}),
    h(AccountMenu,{username:'operator'},{default:()=>[h('button',{type:'button',role:'menuitem'},'Profile'),h('button',{type:'button',role:'menuitem'},'Sign out')]}),
    h(LanguageSwitcher,{modelValue:'en',label:'Language',options:[{code:'en',label:'English'},{code:'zh-CN',label:'简体中文'}]}),
    h('details',{open:true},[h('summary','Details'),h('p','Menu layout verification')])
   ])}).mount('#app');</script></body></html>`}))
  await page.goto(origins[surface]+'/__menu-sizing')
  const preset=page.viewportSize()!
  for(const width of reflow?[320,900,901,1440]:[preset.width]){
   if(reflow)await page.setViewportSize({width,height:preset.height})
   const desktop=await page.evaluate(()=>matchMedia('(min-width:901px) and (pointer:fine)').matches)
   const height=desktop?32:44
   for(const [trigger,items] of [['.member-menu-trigger','.member-action-menu button'],['.account-trigger','.account-popover button'],['.locale-picker__trigger','.locale-picker__option']]){
    await page.locator(trigger).click()
    for(const item of await page.locator(items).all()){
     await expect(item).toBeVisible()
     expect((await item.boundingBox())!.height).toBe(height)
     expect(await item.evaluate(el=>parseFloat(getComputedStyle(el).fontSize))).toBeGreaterThanOrEqual(11)
    }
    if(trigger==='.member-menu-trigger'){
     await expect(page.getByRole('menuitem',{name:'编辑',exact:true})).toBeFocused()
     await page.keyboard.press('ArrowDown')
     await expect(page.getByRole('menuitem',{name:'踢出',exact:true})).toBeFocused()
     if(width===preset.width||width===1440)await page.screenshot({path:info.outputPath(`menu-${surface}-${width}.png`)})
    }
    await page.locator(items).first().focus()
    await page.keyboard.press('Escape')
    await expect(page.locator(trigger)).toHaveAttribute('aria-expanded','false')
   }
   expect((await page.locator('summary').boundingBox())!.height).toBe(height)
   expect((await page.locator('.locale-picker__trigger').boundingBox())!.height).toBe(height)
   await expectNoPageOverflow(page)
  }
  expect(errors).toEqual([])
 })

}
