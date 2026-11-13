import { expect, test } from '@playwright/test'
import { resolve } from 'node:path'
import { origins, gameId, expectNoPageOverflow, expectInsideViewport } from './fixture'

export function registerStateReviewTests(){
 for(const surface of ['user','brand','platform','agent'] as const){
  async function harness(page:import('@playwright/test').Page,mode:'read'|'shared'){
   const directory=surface==='brand'?'admin-web':surface+'-web'
   await page.route('**/__states',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><body><div id="app" class="${surface==='user'?'':'management-console'}"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import Harness from'/@fs${resolve('tests/layout/StateReviewHarness.vue')}';import'/@fs${resolve('shared/src/styles.css')}';${surface==='agent'?'':`import'/@fs${resolve(directory+'/src/style.css')}';`}import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/empty-state.css')}';createApp(Harness,{surface:'${surface}',mode:'${mode}'}).mount('#app')</script></body></html>`}))
   await page.goto(origins[surface]+'/__states')
  }
  test(`empty states distinguish pending, failed and empty reads (${surface})`,async({page},info)=>{
   let finish!:()=>void,requested=false,readPhase:'waiting'|'failed'|'empty'='waiting'
   const wait=new Promise<void>(resolve=>finish=resolve)
   const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
   await page.route('**/api/**',async route=>{
    expect(route.request().method()).toBe('GET');requested=true
    if(readPhase==='waiting')await wait
    if(readPhase==='failed')return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic read failure'}}})
    const path=new URL(route.request().url()).pathname
    const data=surface==='agent'?{items:[],total_count:'0',limit:20,offset:0,summary:{member_count:'0',available_points:'0',bet_count:'0',stake_points:'0',unsettled_points:'0',net_points:'0'}}:surface==='user'?{brand_id:gameId,member_id:gameId,snapshot_at:'2026-10-24T00:00:00Z',state:null,items:[],total_count:'0',limit:20,offset:0}:path.endsWith('/permissions')?{items:[]}:{items:[],total_count:'0',limit:50,offset:0}
    return route.fulfill({json:{success:true,data}})
   })
   try{
    await harness(page,'read');await expect.poll(()=>requested).toBe(true)
    await expect(page.locator('.empty-state')).toHaveCount(0);await expect(page.locator('.global-loading-overlay')).toBeVisible()
    readPhase='failed';finish();await expect(page.getByRole('alert').first()).toBeVisible();await expect(page.locator('.global-loading-overlay')).toBeHidden();await expect(page.locator('.empty-state')).toHaveCount(0)
    readPhase='empty';await page.reload();await expect(page.locator('.empty-state')).toBeVisible();await expect(page.getByRole('alert')).toHaveCount(0)
    await page.locator('#state-language').click();await expect(page.locator('.empty-state')).toContainText(/暂无|没有/)
    await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`empty-${surface}.png`),fullPage:true});expect(errors).toEqual([])
   }finally{finish()}
  })
  test(`confirmation, progress and success remain usable (${surface})`,async({page},info)=>{
   await page.clock.install()
   let release!:()=>void,writing=false
   const wait=new Promise<void>(resolve=>release=resolve)
   await page.route('**/api/state-save',async route=>{expect(route.request().method()).toBe('POST');writing=true;await wait;await route.fulfill({json:{success:true}})})
   try{
    await harness(page,'shared');await page.locator('#state-open').focus();await page.locator('#state-open').press('Enter');const dialog=page.getByRole('dialog');await expect(dialog).toBeVisible()
    const preset=page.viewportSize()!
    if(info.project.name.endsWith('reflow')){for(const size of [{width:320,height:480},{width:640,height:320}]){await page.setViewportSize(size);await expectInsideViewport(dialog);await expectInsideViewport(dialog.locator('.detail-modal-close'));await expectInsideViewport(dialog.locator('#state-save'));await expectNoPageOverflow(page)}await page.setViewportSize(preset)}
    await page.keyboard.press('Tab');await expect.poll(()=>dialog.evaluate(el=>el.contains(document.activeElement))).toBe(true)
    await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0);await expect(page.locator('#state-open')).toBeFocused()
    await page.locator('#state-open').click();await page.locator('#state-save').click();await expect.poll(()=>writing).toBe(true)
    const overlay=page.locator('.global-loading-overlay');await expect(overlay).toBeVisible();await expect(overlay).toHaveJSProperty('popover','manual');await expect(dialog.locator('.detail-modal-close')).toBeDisabled()
    await expect.poll(()=>overlay.evaluate(el=>{const box=el.querySelector('.global-loading-card')!.getBoundingClientRect();return el.contains(document.elementFromPoint(box.x+box.width/2,box.y+box.height/2))})).toBe(true)
    await page.keyboard.press('Escape');await expect(dialog).toBeVisible();release();await expect(dialog).toHaveCount(0);await expect(overlay).toBeHidden()
    const toast=page.locator('.toast');await expect(toast).toContainText('review_account');await toast.locator('button').focus();await page.clock.runFor(2000);await expect(toast).toBeVisible()
    await page.locator('#state-language').click();await expect(toast).toContainText('账号「review_account」的设置已保存。');await expect(toast.locator('button')).toHaveCSS('width','44px');await expectNoPageOverflow(page)
    await page.screenshot({path:info.outputPath(`success-${surface}.png`)});await toast.locator('button').click();await expect(toast).toHaveCount(0)
   }finally{release()}
  })
 }
}
