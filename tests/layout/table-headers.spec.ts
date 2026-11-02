import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'
import { origins, expectNoPageOverflow } from './fixture'

for (const locale of ['en','zh-CN'] as const) {
 test(`four apps share subtle table headers without changing mobile reflow (${locale})`, async ({page}, info) => {
  const source = await (await page.request.get(origins.user + '/src/ConfiguredBetting.vue')).text()
  const vuePath = source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
  const errors:string[]=[]; page.on('pageerror', error => errors.push(error.message))
  await page.route('**/api/**', route => {throw new Error(`Unexpected API request: ${route.request().url()}`)})
  for (const surface of ['user','brand','platform','agent'] as const) {
   const directory = surface === 'brand' ? 'admin-web' : `${surface}-web`
   await page.route(`${origins.user}/__table-headers-${surface}`, route => route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/TableHeaderHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';${surface==='agent'?'':`import'/@fs/${resolve(directory+'/src/style.css')}';`}import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Harness,${JSON.stringify({surface,locale})}).mount('#app')</script></html>`}))
   await page.goto(`${origins.user}/__table-headers-${surface}`)
   await expect(page.getByTestId('native-table')).toBeVisible()
   const widths = info.project.name.endsWith('reflow') ? [320,700,1080,1081,1440] : [page.viewportSize()!.width]
   for(const width of widths){
    if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:800})
    const headers = page.locator('thead th,[role="columnheader"],.table-head>span')
    for(const header of await headers.all()){
     if(!await header.isVisible())continue
     const target = await header.getAttribute('role')==='columnheader' ? header.locator('..') : header
     await expect(target).toHaveCSS('background-color','rgb(241, 245, 249)')
     await expect(header).toHaveCSS('color','rgb(51, 65, 85)')
     await expect(header).toHaveCSS('font-weight','650')
     await expect(target).toHaveCSS('border-bottom-color','rgb(226, 232, 240)')
    }
    await expect(page.locator('tbody th[scope="row"]')).not.toHaveCSS('background-color','rgb(241, 245, 249)')
    const shared = page.getByTestId('shared-table')
    if(width<=1080){
     await expect(shared.locator('thead')).not.toBeVisible()
     const backgrounds = await shared.locator('td').evaluateAll(nodes => nodes.map(node => getComputedStyle(node,'::before').backgroundColor))
     expect(backgrounds.every(value=>value==='rgba(0, 0, 0, 0)')).toBe(true)
    }else await expect(shared.locator('thead')).toBeVisible()
    await expectNoPageOverflow(page)
   }
   await page.screenshot({path:info.outputPath(`table-headers-${surface}-${locale}.png`),fullPage:true})
   await page.getByRole('button',{name:locale==='en'?'Details':'详情',exact:true}).click()
   const dialog = page.getByRole('dialog')
   await expect(dialog.locator('thead th').first()).toHaveCSS('background-color','rgb(241, 245, 249)')
   await expectNoPageOverflow(page)
   await page.screenshot({path:info.outputPath(`dialog-table-header-${surface}-${locale}.png`)})
   await dialog.getByRole('button',{name:locale==='en'?'Close':'关闭',exact:true}).click()
  }
  expect(errors).toEqual([])
 })
}
