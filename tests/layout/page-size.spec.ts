import { expect, test } from '@playwright/test'
import { resolve } from 'node:path'
import { selectControl } from '../select-control'
const origin = `http://127.0.0.1:${process.env.LOTTERY_LAYOUT_PORT_BASE ?? 15273}`
const id = 'aaaaaaaa-aaaa-4aaa-8aaa-000000000001'
for (const locale of ['en', 'zh']) for (const kind of ['draw', 'recharges', 'agent', 'brand']) {
 test(`${kind} size selection reads the first page with the requested limit (${locale})`, async ({ page }, info) => {
  let empty = false
  const errors: string[] = [], requests: URL[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.route('**/api/**', async route => {
   expect(route.request().method()).toBe('GET')
   const url = new URL(route.request().url()), limit = Number(url.searchParams.get('limit')), offset = Number(url.searchParams.get('offset'))
   requests.push(url)
   const total = empty ? '0' : '231'
   const items = Array.from({ length: empty ? 0 : Math.min(limit, 231-offset) }, (_, i) => {
    const index = offset + i + 1, rowId = `bbbbbbbb-bbbb-4bbb-8bbb-${String(index).padStart(12,'0')}`
    if (kind === 'draw') return { period_no: String(index), result: {regular:[],special:[],digits:[1,2,3]}, drawn_at:'2026-10-27T00:00:00Z', draw_date:null,regular_order:null,attributes:{} }
    if (kind === 'agent') return { id:rowId,username:`agent${index}`,depth:1,config:{status:'active'} }
    if (kind === 'brand') return { id:rowId,brand_id:id,payment_id:id,agent_id:rowId,member_id:rowId,target_id:null,original_points:'10',adjusted_points:null,state:'pending',paid_at:null }
    return {id:rowId,brand_id:id,member_id:id,member_username:`member${index}`,account_id:id,points:'10',state:'pending',method:'manual',proof_reference:'',remark:'',created_by:id,created_by_name:'Tester',confirmed_by_name:'',version:1,created_at:'2026-10-27T00:00:00Z'}
   })
   const data = {brand_id:id,payment_id:id,limit,offset,total_count:total,items,has_more:!empty && offset+limit<231,...(kind==='draw'?{game_id:url.searchParams.get('game_id'),game_name:'Demo',mark_six:false,timezone:'Asia/Singapore',daily:true,choices:['2026-10-27'],selected:'2026-10-27'}:{})}
   await route.fulfill({json:{success:true,data:kind==='recharges'?{items,total_count:total}:kind==='brand'?{brand_id:route.request().headers()['x-brand-id'],payment_id:url.pathname.split('/').at(-2),limit,offset,total_count:total,items}:data}})
  })
  await page.route('**/__page_size?*', route => route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import C from'/@fs${resolve('tests/layout/PaginationHarness.vue')}';createApp(C).mount('#app')</script></html>`}))
  await page.goto(`${origin}/__page_size?kind=${kind}&locale=${locale}`)
  const nav=page.locator('.table-pagination'), select=nav.getByRole('combobox'), current=nav.locator('[aria-current="page"]')
  const mobile = page.viewportSize()!.width <= 640 || await page.evaluate(() => matchMedia('(hover:none) and (pointer:coarse)').matches)
  if (mobile) {
   await expect(select).toBeHidden()
   await expect(nav.locator('.table-pagination-summary')).toHaveText(locale==='en'?'Total: 231':'共 231', {useInnerText:true})
   await expect(nav.getByRole('button', {name:locale==='en'?'First':'首页',exact:true})).toBeDisabled()
   const before = requests.length
   await nav.getByRole('button',{name:locale==='en'?'Last':'尾页',exact:true}).click()
   await expect.poll(()=>requests.length).toBeGreaterThan(before)
   const initialSize = Number(requests[0].searchParams.get('limit'))
   expect(requests.at(-1)!.searchParams.get('offset')).toBe(String(Math.floor(230/initialSize)*initialSize))
   await nav.getByRole('button',{name:locale==='en'?'First':'首页',exact:true}).click()
   await expect(current).toHaveAttribute('aria-label',locale==='en'?'Page 1':'第 1 页')
  } else {
  await expect(select).toBeVisible()
  await expect(nav.getByRole('button',{name:locale==='en'?'First':'首页',exact:true})).toBeHidden()
  for (const size of [10,20,50,100]) {
   await nav.locator('.table-pagination-arrow').last().click()
   await expect(current).toHaveText('2', {useInnerText:true})
   const before=requests.length
   await selectControl(select,String(size))
   await expect.poll(()=>requests.length).toBe(before+1)
   expect(requests.at(-1)!.searchParams.get('limit')).toBe(String(size))
   expect(requests.at(-1)!.searchParams.get('offset')).toBe('0')
   await expect(current).toHaveText('1', {useInnerText:true})
   await expect(select).toHaveAttribute('data-value',String(size))
   const rows=page.locator(kind==='draw'?'.archive-row':'tbody tr')
   await expect(rows).toHaveCount(size)
   await expect(nav.locator('.table-pagination-summary')).toContainText(locale==='en'?`${size} records on this page`:`本页 ${size} 条`)
  }
  }
  if (info.project.name.endsWith('reflow')) {
   await page.setViewportSize({width:320,height:720})
   await expect(select).toBeHidden()
   await expect(nav.locator('.table-pagination-summary')).toHaveText(locale==='en'?'Total: 231':'共 231', {useInnerText:true})
  }
  if (mobile || info.project.name.endsWith('reflow')) {
   const centers = await nav.evaluate(node => [...node.querySelectorAll('.table-pagination-summary, button')].filter(element => (element as HTMLElement).offsetWidth > 0).map(element => {const box=element.getBoundingClientRect(); return box.y + box.height/2}))
   expect(Math.max(...centers)-Math.min(...centers)).toBeLessThanOrEqual(1)
   await expect(current).toHaveText('1')
   await expect(nav.locator('button:visible .site-icon')).toHaveCount(4)
   const boxes = await nav.locator('button:visible').evaluateAll(buttons => buttons.map(button => { const box=button.getBoundingClientRect(); return {left:box.left,right:box.right,height:box.height} }))
   for (let index=1;index<boxes.length;index++) expect(boxes[index]!.left-boxes[index-1]!.right).toBeGreaterThanOrEqual(4)
   expect(boxes.every(box=>box.height>=44)).toBe(true)
  }
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
  expect(errors).toEqual([])
  await nav.screenshot({path:info.outputPath(`${kind}-size.png`)})
  empty = true
  const before = requests.length
  if(kind==='brand'||kind==='recharges')await page.locator(kind==='brand'?'.commission-toolbar button[type=submit]':'.wallet-query button[type=submit]').click()
  else await page.reload()
  await expect.poll(()=>requests.length).toBeGreaterThan(before)
  await expect(nav).toHaveCount(0)
  await expect(page.locator('.empty-state:visible')).toHaveCount(1)
 })
}
