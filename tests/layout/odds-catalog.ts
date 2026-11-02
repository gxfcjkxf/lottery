import { test, expect } from '@playwright/test'
import { readFileSync, readdirSync } from 'node:fs'
import { resolve } from 'node:path'
import { configuredDemo } from '../../shared/src/configured-rules'
import { origins } from './fixture'

export function registerOddsCatalogTests() {
 for (const locale of ['en', 'zh-CN'] as const) {
  test(`all built-in game and play odds fit cards (${locale})`, async ({page}, info) => {
   test.setTimeout(180_000)
   const files = readdirSync('backend/internal/rules/config').filter(file => file.endsWith('.json'))
   const snapshots = files.map(name => {
    const snapshot = configuredDemo(JSON.parse(readFileSync(`backend/internal/rules/config/${name}`, 'utf8')))
    for (const prices of Object.values(snapshot.prices.plates)) Object.keys(prices).forEach((key,index) => { prices[key] = ['48.99', '13.14', '2.5'][index % 3]! })
    return {name,snapshot}
   })
   const source = await (await page.request.get(origins.user + '/src/ConfiguredBetting.vue')).text()
   const vuePath = source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
   await page.route('**/__odds-gallery', route => route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/OddsGalleryHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve('user-web/src/style.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Harness,${JSON.stringify({initialSnapshot:snapshots[0]!.snapshot,locale})}).mount('#app')</script></html>`}))
   const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
   await page.goto(origins.user + '/__odds-gallery')
   await expect(page.locator('.configured-option-price').first()).toBeVisible()
   const widths = info.project.name.endsWith('reflow') ? [320,360,620,999,1000,1440] : [page.viewportSize()!.width]
   let single = 0, multiple = 0
   for (const catalog of snapshots) {
    await page.evaluate(value => window.dispatchEvent(new CustomEvent('odds-gallery-catalog', {detail:value})), catalog)
    await expect(page.locator('.odds-gallery')).toHaveAttribute('data-catalog', catalog.name)
    await expect(page.locator('.odds-gallery-case')).toHaveCount(catalog.snapshot.catalog.plates.length * catalog.snapshot.catalog.plays.length)
    for (const width of widths) {
     if (info.project.name.endsWith('reflow')) await page.setViewportSize({width,height:800})
     const result = await page.locator('.configured-option-price').evaluateAll(nodes => {
      const failures: unknown[] = []; let single = 0, multiple = 0
      for (const node of nodes) {
       const range = document.createRange(); range.selectNodeContents(node)
       const text = range.getBoundingClientRect(), button = node.closest('button')!.getBoundingClientRect()
       if (node.textContent!.includes(' / ')) multiple++; else single++
       if (range.getClientRects().length !== 1 || text.left < button.left || text.right > button.right || text.bottom > button.bottom) failures.push({play:node.closest('[data-case]')!.getAttribute('data-case'),text:node.textContent,lines:range.getClientRects().length,textWidth:text.width,buttonWidth:button.width,left:text.left-button.left,right:text.right-button.right,bottom:text.bottom-button.bottom,spanBottom:node.getBoundingClientRect().bottom-button.bottom,spanHeight:node.getBoundingClientRect().height,rangeHeight:text.height,font:getComputedStyle(node).fontSize})
      }
      return {failures:failures.slice(0,10),single,multiple,overflow:document.documentElement.scrollWidth > innerWidth + 1}
     })
     if (result.failures.length) {
      const first = result.failures[0] as {play:string}
      await page.locator(`[data-case="${first.play}"] .configured-options`).screenshot({path:info.outputPath('odds-overflow.png')})
     }
     expect(result.failures, `${catalog.name}, ${width}px`).toEqual([])
     expect(result.overflow, `${catalog.name}, ${width}px page overflow`).toBe(false)
     single += result.single; multiple += result.multiple
    }
   }
   expect(single).toBeGreaterThan(0); expect(multiple).toBeGreaterThan(0); expect(errors).toEqual([])
   await page.locator('.odds-gallery-case').first().screenshot({path:info.outputPath(`odds-catalog-${locale}.png`)})
  })
 }
}
