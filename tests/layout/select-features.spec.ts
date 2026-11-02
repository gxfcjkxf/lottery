import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'
for (const locale of ['en','zh-CN'] as const) for (const surface of ['user','admin','platform','agent']) {
 test(`unified selection features ${surface} ${locale}`,async({page},info)=>{
  const zh=locale==='zh-CN',errors:string[]=[]
  page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/**',route=>{
   expect(route.request().method()).toBe('GET')
   expect(new URL(route.request().url()).pathname).toBe('/api/v1/admin/filter-options')
   return route.fulfill({json:{success:true,data:{brand_id:'select-test-brand',limit:100,offset:0,total:1,items:[{id:'member-1',name:zh?'独立测试会员':'Independent test member'}]}}})
  })
  const origin='http://127.0.0.1:15373'
  const source=await (await page.request.get(origin+'/src/ConfiguredBetting.vue')).text()
  const vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
  const style=surface==='agent'?'shared/src/management.css':`${surface}-web/src/${surface==='user'?'style.css':'style.css'}`
  await page.route('**/__select-features',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/SelectFeaturesHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve(style)}';import'/@fs/${resolve('shared/src/controls.css')}';import'/@fs/${resolve('shared/src/form-validation.css')}';createApp(Harness,${JSON.stringify({locale})}).mount('#app')</script></html>`}))
  await page.goto(origin+'/__select-features')
  const select=page.getByRole('combobox',{name:zh?'必选项':'Required choice',exact:true})
  const result=()=>page.getByTestId('result').textContent().then(s=>JSON.parse(s!))
  await page.getByRole('button',{name:zh?'保存':'Save',exact:true}).click()
  await expect(page.getByRole('alert')).toContainText(zh?'必选项':'Required choice')
  await expect(select).toHaveAttribute('aria-invalid','true')
  expect((await result()).saves).toBe(0)
  await select.click()
  await expect(page.getByRole('group',{name:zh?'平台':'Platform',exact:true})).toBeVisible()
  await expect(page.getByRole('option',{name:zh?'否':'No',exact:true})).toHaveAttribute('aria-disabled','true')
  await page.getByRole('option',{name:zh?'第一项':'First',exact:true}).click()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(select).not.toHaveAttribute('aria-invalid')
  expect((await result()).changes).toBe(1)
  await select.press('Enter');await select.press('End');await select.press('Escape')
  expect((await result()).required).toBe('first')
  for(const [name,option] of [[zh?'数字':'Number','1'],[zh?'布尔值':'Boolean',zh?'是':'Yes']] as const){await page.getByRole('combobox',{name,exact:true}).click();await page.getByRole('option',{name:option,exact:true}).click()}
  expect((await result()).numeric).toBe(1);expect((await result()).boolean).toBe(true)
  await page.getByRole('combobox',{name:zh?'会员':'Member',exact:true}).click()
  await page.getByRole('option',{name:zh?'独立测试会员':'Independent test member',exact:true}).click()
  expect((await result()).member).toBe('member-1')
  const hour=page.getByRole('combobox',{name:zh?'时间 · 时':'Time · Hour',exact:true})
  await hour.click();await page.getByRole('option',{name:'02',exact:true}).click()
  expect((await result()).time).toBe('14:05:00')
  const zone=page.getByRole('combobox').filter({hasText:'Asia/Singapore'})
  await zone.click()
  const popup=page.getByRole('listbox').filter({visible:true})
  const bounds=(await popup.boundingBox())!
  expect(bounds.x).toBeGreaterThanOrEqual(0);expect(bounds.x+bounds.width).toBeLessThanOrEqual(page.viewportSize()!.width+1)
  expect(bounds.y+bounds.height).toBeLessThanOrEqual(page.viewportSize()!.height+1)
  await page.screenshot({path:info.outputPath(`${surface}-${locale}.png`)})
  await page.keyboard.press('Escape')
  await page.getByRole('button',{name:zh?'保存':'Save',exact:true}).click()
  expect((await result()).saves).toBe(1)
  await select.click();await page.mouse.click(2,2);await expect(select).toHaveAttribute('aria-expanded','false')
  await page.getByRole('button',{name:zh?'禁用':'Disable',exact:true}).click();await expect(select).toBeDisabled()
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
  expect(await select.evaluate(el=>parseFloat(getComputedStyle(el).fontSize))).toBeGreaterThanOrEqual(info.project.use.isMobile||info.project.name==='reflow-320'?16:11)
  await expect(page.locator('select')).toHaveCount(0)
  expect(errors).toEqual([])
 })
}
