import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'
import { origins, expectNoPageOverflow } from './fixture'
export function registerControlSizingTests(reflow = false) {
 for (const surface of ['user','brand','platform'] as const) test(`shared ${surface} controls fit desktop content and mobile viewports`, async ({page},info) => {
  const directory = surface === 'brand' ? 'admin-web' : surface + '-web'
  await page.route('**/__control-sizing',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><style>.control-demo{box-sizing:border-box;width:min(900px,100%);margin:0 auto;padding:16px;display:grid;gap:16px}.control-demo label{display:grid;gap:6px;min-width:0}.control-demo .actions{display:grid;grid-template-columns:1fr;gap:8px}</style><body><div id="app" class="${surface==='user'?'':'management-console'}"><form class="control-demo"><label>Account<input aria-label="Account" value="sample_operator"></label><label>Email<input type="email" aria-label="Email" value="operator@example.com"></label><label>Points<input type="number" aria-label="Points" value="10000"></label><label>Status<select aria-label="Status"><option>All active and paused members</option><option>Online</option></select></label><div class="filters"><input aria-label="Search" placeholder="Search"></div><div class="actions"><button type="button" class="button button-primary">Save changes</button><button type="button" class="button button-secondary">Cancel</button></div></form></div><script type="module">import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve(directory+'/src/style.css')}';import'/@fs${resolve('shared/src/controls.css')}';${surface==='user'?'':`import'/@fs${resolve('shared/src/management.css')}';`}</script></body></html>`}));
  await page.goto(origins[surface]+'/__control-sizing')
  const preset=page.viewportSize()!
  for(const width of reflow?[320,640,900,1440]:[preset.width]){
   if(reflow)await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
   const desktop=await page.evaluate(()=>matchMedia('(min-width:901px) and (pointer:fine)').matches)
   if(desktop){
    const parent=(await page.locator('label').first().boundingBox())!.width
    for(const name of ['Account','Email','Points'])expect((await page.getByLabel(name,{exact:true}).boundingBox())!.width).toBeLessThan(parent-80)
    const normal=(await page.getByLabel('Account',{exact:true}).boundingBox())!.width,email=(await page.getByLabel('Email',{exact:true}).boundingBox())!.width,number=(await page.getByLabel('Points',{exact:true}).boundingBox())!.width
    expect(email).toBeGreaterThan(normal);expect(number).toBeLessThan(normal)
    for(const button of await page.locator('.actions button').all())expect((await button.boundingBox())!.width).toBeLessThan(200)
    const select=page.getByLabel('Status',{exact:true})
    const optionFits=await select.evaluate((element:HTMLSelectElement)=>{const style=getComputedStyle(element),canvas=document.createElement('canvas'),context=canvas.getContext('2d')!;context.font=style.font;const text=Math.max(...[...element.options].map(option=>context.measureText(option.text).width));return element.getBoundingClientRect().width>=text+parseFloat(style.paddingLeft)+parseFloat(style.paddingRight)-2})
    expect(optionFits).toBe(true)
   }
  }
  const save=page.getByRole('button',{name:'Save changes',exact:true});await save.scrollIntoViewIfNeeded();const before=await save.boundingBox()
  await save.hover();await save.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))))
  const after=await save.boundingBox();expect(after!.x).toBeCloseTo(before!.x,1);expect(after!.y).toBeCloseTo(before!.y,1)
  await expect(save).toHaveCSS('background-image','none')
  await page.screenshot({path:info.outputPath(`controls-${surface}.png`)})
 })
}
