import { expect, test } from '@playwright/test'
import { resolve } from 'node:path'
const origin='http://127.0.0.1:15473', id='aaaaaaaa-aaaa-4aaa-8aaa-000000000001'
for(const locale of ['zh','en']) for(const kind of ['draw','recharges']) {
 test(`${kind} pagination retains content until the response is applied (${locale})`,async({page},info)=>{
  const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
  let blocked=false,release=()=>{},started=0,fail=false
  let gate=Promise.resolve()
  await page.route('**/api/**',async route=>{
   const url=new URL(route.request().url());expect(route.request().method()).toBe('GET')
   const offset=Number(url.searchParams.get('offset')??0)
   const brand=route.request().headers()['x-brand-id']??id
   started++
   if(blocked)await gate
   if(fail)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic read failure'}}})
   const data=kind==='draw'?{
    total_count:'100',game_id:url.searchParams.get('game_id'),game_name:'Mark Six',mark_six:true,timezone:'Asia/Singapore',daily:false,choices:['2026'],selected:'2026',has_more:offset===0,
    items:Array.from({length:50},(_,i)=>({period_no:`2026-${offset+i+1}`,result:{regular:[1,2,3,4,5,6],special:[7],digits:[]},drawn_at:'2026-10-27T00:00:00Z',draw_date:'2026-10-27',regular_order:[1,2,3,4,5,6],attributes:Object.fromEntries([1,2,3,4,5,6,7].map(n=>[String(n).padStart(2,'0'),{wave:'red',zodiac:'rat',element:'wood'}]))}))
   }:{total_count:'100',items:Array.from({length:offset?50:51},(_,i)=>({id:`aaaaaaaa-aaaa-4aaa-8aaa-${String(offset+i+1).padStart(12,'0')}`,brand_id:brand,member_id:id,member_username:`member-${offset+i+1}`,account_id:id,points:'100',state:'pending',method:'manual',proof_reference:'',remark:'',created_by:id,created_by_name:'Tester',confirmed_by_name:'',version:1,created_at:'2026-10-27T00:00:00Z'}))}
   await route.fulfill({json:{success:true,data}})
  })
  await page.route('**/__pagination?*',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import C from'/@fs${resolve('tests/layout/PaginationHarness.vue')}';createApp(C).mount('#app')</script></html>`}))
  await page.goto(`${origin}/__pagination?kind=${kind}&locale=${locale}`)
  const list=page.locator(kind==='draw'?'.archive-list':'[data-testid="platform-recharges"] .table-wrap'),rows=list.locator(kind==='draw'?'.archive-row':'tbody tr')
  await expect(rows).toHaveCount(50)
  const overlay=page.locator('.global-loading-overlay')
  await expect(overlay).toBeHidden()
  const next=page.getByRole('button',{name:locale==='en'?(kind==='draw'?'Next':'Next page'):'下一页',exact:true})
  const previous=page.getByRole('button',{name:locale==='en'?(kind==='draw'?'Previous':'Previous page'):'上一页',exact:true})
  for(const [button,wanted] of [[next,51],[previous,1]] as const){
   await button.scrollIntoViewIfNeeded()
   const before=await list.boundingBox(),scroll=await page.evaluate(()=>scrollY),first=await rows.first().textContent(),reads=started
   blocked=true;gate=new Promise(resolve=>{release=resolve})
   try{
    await button.click()
    await expect.poll(()=>started).toBeGreaterThan(reads)
    await expect(overlay).toBeVisible();await expect(overlay).toContainText(locale==='en'?'Loading':'加载中')
    await expect(rows).toHaveCount(50);await expect(rows.first()).toHaveText(first!)
    await expect(list).toBeVisible();await expect(next).toBeDisabled();await expect(previous).toBeDisabled()
    expect((await list.boundingBox())!.height).toBe(before!.height)
    expect(await page.evaluate(()=>scrollY)).toBe(scroll)
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
    if(wanted===51)await page.screenshot({path:info.outputPath('pagination-loading.png')})
   }finally{blocked=false;release()}
   await expect(overlay).toBeHidden()
   await expect(rows.first()).toContainText(kind==='draw'?`2026-${wanted}`:`member-${wanted}`)
  }
  // A failed read ends loading and never pretends that a new page succeeded.
  fail=true;const failedRead=started;await next.click();await expect.poll(()=>started).toBeGreaterThan(failedRead);await expect(overlay).toBeHidden();if(kind==='draw')await expect(rows).toHaveCount(0);else await expect(next).toBeEnabled()
  // Changing identity must remove every old row before the next response.
  fail=false;blocked=true;gate=new Promise(resolve=>{release=resolve})
  try{
   await page.locator('#change-scope').evaluate((button:HTMLButtonElement)=>button.click())
   await expect(overlay).toBeVisible();await expect(rows).toHaveCount(0)
  }finally{blocked=false;release()}
  await expect(overlay).toBeHidden();await expect(rows).toHaveCount(50)
  expect(errors).toEqual([])
 })
}

for (const locale of ['zh', 'en']) test(`shared numbered pagination and empty state (${locale})`, async ({page}, info) => {
  await page.route('**/__pagination?*', route => route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import C from'/@fs${resolve('tests/layout/PaginationHarness.vue')}';createApp(C).mount('#app')</script></html>`}))
  await page.goto(`${origin}/__pagination?kind=controls&locale=${locale}`)
  const nav = page.getByRole('navigation')
  const pageButton = (number: number) => nav.getByRole('button', {name: locale === 'zh' ? `第 ${number} 页` : `Page ${number}`, exact:true})
  const current = nav.locator('[aria-current="page"]')
  await expect(current).toHaveText('1')
  await expect(nav.getByRole('button', {name:locale === 'zh' ? '上一页' : 'Previous page', exact:true})).toBeDisabled()
  await pageButton(2).click(); await expect(current).toHaveText('2')
  await pageButton(15).click(); await expect(current).toHaveText('15')
  await expect(nav).toContainText(locale === 'zh' ? '本页 31 条' : '31 records on this page')
  await expect(nav.getByRole('button', {name:locale === 'zh' ? '下一页' : 'Next page', exact:true})).toBeDisabled()
  const input = nav.getByRole('textbox')
  await input.fill('16'); await expect(nav.getByRole('button', {name:'GO',exact:true})).toBeDisabled()
  await input.fill('7'); await input.press('Enter'); await expect(current).toHaveText('7')
  await expect(input).toHaveValue('')
  await expect(nav).toContainText(locale === 'zh' ? '50 条/页' : '50 / page')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true)
  await page.screenshot({path:info.outputPath('numbered-pagination.png')})
  await page.locator('#empty-list').click(); await expect(nav).toHaveCount(0)
})
