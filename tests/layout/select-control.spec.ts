import { test, expect } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { configuredDemo } from '../../shared/src/configured-rules'

for (const locale of ['en','zh-CN'] as const) {
 test(`plate selection and popup layout (${locale})`, async ({page}, info) => {
  const errors:string[] = []
  page.on('pageerror', error => errors.push(error.message))
  const origin = 'http://127.0.0.1:15373'
  const snapshot = configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json','utf8')))
  snapshot.catalog.plates = ['A','B','C']
  for (const [index, plate] of snapshot.catalog.plates.entries()) {
   snapshot.prices.plates[plate] = Object.fromEntries(Object.keys(snapshot.prices.plates.A!).map(key => [key, String(48 + index)]))
  }
  const source = await (await page.request.get(origin + '/src/ConfiguredBetting.vue')).text()
  const vuePath = source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
  await page.route('**/__select-review', route => route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/SelectControlHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve('user-web/src/style.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Harness,${JSON.stringify({snapshot,locale})}).mount('#app')</script></html>`}))
  await page.goto(origin + '/__select-review')
  const plate = page.getByRole('combobox',{name:locale === 'en' ? 'Plate' : '盘口',exact:true})
  const popup = page.getByRole('listbox',{name:locale === 'en' ? 'Plate' : '盘口',exact:true})
  await expect(plate.locator('.select-control-value > span').last()).toHaveText('A')
  const touch = info.project.use.isMobile || info.project.name === 'reflow-320'
  expect((await plate.boundingBox())!.height).toBe(touch ? 44 : 32)
  if (info.project.use.isMobile) await plate.tap()
  else await plate.click()
  await expect(popup).toBeVisible()
  await expect(plate).toHaveAttribute('aria-expanded','true')
  await expect(popup.getByRole('option',{name:'A',exact:true})).toHaveAttribute('aria-selected','true')
  const anchor = (await plate.boundingBox())!, box = (await popup.boundingBox())!
  expect(box.y + box.height).toBeLessThanOrEqual(anchor.y)
  await page.screenshot({path:info.outputPath(`plate-open-${locale}.png`)})
  const optionB = page.getByRole('option',{name:'B',exact:true})
  if (info.project.use.isMobile) await optionB.tap()
  else await optionB.click()
  await expect(popup).toBeHidden()
  await expect(page.getByTestId('plate-value')).toHaveText('B')
  await expect(page.locator('.configured-option-price').first()).toHaveText('49')
  await expect(plate).toBeFocused()
  await plate.press('ArrowDown'); await plate.press('End'); await plate.press('Escape')
  await expect(popup).toBeHidden(); await expect(page.getByTestId('plate-value')).toHaveText('B')
  await plate.press('Enter'); await plate.press('Home'); await plate.press('ArrowDown'); await plate.press('ArrowDown'); await plate.press('Enter')
  await expect(page.getByTestId('plate-value')).toHaveText('C')
  await plate.press('b'); await plate.press('Enter')
  await expect(page.getByTestId('plate-value')).toHaveText('B')
  await plate.click()
  await page.getByRole('tab').first().click()
  await expect(popup).toBeHidden()
  await page.getByRole('button',{name:locale === 'en' ? 'Disable plate' : '禁用盘口'}).click()
  await expect(plate).toBeDisabled()
  const longList = page.getByRole('combobox',{name:locale === 'en' ? 'Long list' : '长列表',exact:true})
  await longList.press('Home'); await longList.press('ArrowDown'); await longList.press('Enter')
  await expect(longList.locator('.select-control-value > span').last()).toContainText('3 ·')
  await longList.press('End')
  const last = page.getByRole('option').last()
  await expect(last).toBeInViewport()
  await longList.press('Tab')
  await expect(page.getByRole('listbox')).toBeHidden()
  await longList.click()
  const bounds = (await page.getByRole('listbox').boundingBox())!
  expect(bounds.x).toBeGreaterThanOrEqual(0)
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(page.viewportSize()!.width)
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(page.viewportSize()!.height)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true)
  await expect(page.getByTestId('loading-starts')).toHaveText('0')
  await expect(page.locator('.global-loading-overlay')).toBeHidden()
  expect(errors).toEqual([])
 })
}
