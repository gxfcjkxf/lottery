import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'
import { origins } from './fixture'
const origin = origins.brand
const brand = '11111111-1111-4111-8111-111111111111'
const event = '33333333-3333-4333-8333-333333333333'
for (const locale of ['en', 'zh-CN'] as const) {
 test(`brand audit table and modal changes (${locale})`, async ({page}, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message))
  await page.route('**/api/**', route => {
   expect(route.request().method()).toBe('GET')
   const path = new URL(route.request().url()).pathname
   const respond = (data: unknown) => route.fulfill({json: {success: true, data}})
   if(path === '/api/v1/admin/audit') return respond({items:[{id:event,brand_id:brand,action:'user.update',actor_type:'admin',actor_id:event,actor_username:'log_reviewer',resource_type:'user',resource_id:event,reason:'Reviewed account settings / 已核对账号设置',request_id:'review',created_at:'2026-10-27T04:00:00Z',ip_address:"127.0.0.1",before_json:{display_name:'Before',password:'hidden-secret'},after_json:{display_name:'After'}}], total_count: '1'})
   if(path.includes('filter-options')) return respond({brand_id:brand,items:[],limit:100,offset:0, total_count: '0'})
   throw Error(`Unexpected request ${path}`)
  })
  const source = await (await page.request.get(origin + '/src/AuditManagement.vue')).text()
  const vuePath = source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
  const surface = 'audit'
   await page.route(`${origin}/__brand-logs-${surface}`, route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/BrandLogsHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/src/style.css';import'/@fs/${resolve('shared/src/controls.css')}';import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/key-value.css')}';createApp(Harness,${JSON.stringify({locale})}).mount('#app')</script></html>`}))
   await page.goto(`${origin}/__brand-logs-${surface}`)
   const table = page.locator('.audit-table')
   await expect(table).toBeVisible()
   await expect(page.locator('.audit-pagination .table-pagination-summary')).toHaveText(locale==='en'?'Page 1 · 1 record on this page · Total: 1':'第 1 页 · 本页 1 条 · 共 1 条')
   await expect(page.locator('.audit-pagination')).not.toContainText(/Offset|偏移/)
    await expect(table.locator('tbody')).toContainText('log_reviewer')
    await table.getByRole('button').click()
    await expect(page.locator('.audit-snapshots')).toContainText('Before')
    await expect(page.locator('.audit-snapshots')).not.toContainText('hidden-secret')
   const widths = info.project.name.endsWith('reflow') ? [320,700,760,900,1440] : [page.viewportSize()!.width]
   for(const width of widths){
    if(info.project.name.endsWith('reflow')) await page.setViewportSize({width,height:900})
    const overflow = await page.evaluate(()=>({width:innerWidth,scroll:document.documentElement.scrollWidth,nodes:[...document.querySelectorAll('*')].filter(el=>el.getBoundingClientRect().right>innerWidth+1).map(el=>({tag:el.tagName,cls:el.className,width:el.getBoundingClientRect().width}))}))
    expect(overflow.scroll, JSON.stringify(overflow)).toBeLessThanOrEqual(overflow.width+1)
    for(const header of await table.locator('th').all()) if(await header.isVisible()) await expect(header).toHaveCSS('background-color','rgb(241, 245, 249)')
   }
   await page.screenshot({path:info.outputPath(`${surface}-${locale}.png`),fullPage:true})
   await page.keyboard.press('Escape')
   await expect(page.locator('.audit-snapshots')).toHaveCount(0)
  expect(errors).toEqual([])
 })
}
