import { expect, test } from '@playwright/test'
import { resolve } from 'node:path'
import { origins, expectNoPageOverflow } from './fixture'
const brand = 'aaaaaaaa-aaaa-4aaa-8aaa-000000000001', member = 'bbbbbbbb-bbbb-4bbb-8bbb-000000000001', anchor = 'cccccccc-cccc-4ccc-8ccc-000000000001'
const cycleRecord = {id:member,brand_id:brand,window_from:'2026-10-12T16:00:00Z',window_to:'2026-10-19T16:00:00Z',anchor_order_id:anchor,calendar:{timezone:'Asia/Singapore',cycle:'weekly',boundary_time:'00:00:00',weekday:1,month_day:null,short_month:''},state:'enumerating',version:1,target_count:'0',scan_complete:false,current_run_id:null,current_generation:null,evidence_epoch:null,evidence_current:false,calculated_count:'0',earning_count:'0',total_points:'0',created_by:member,creation_actor_type:'admin',reason:'',created_at:'2026-10-20T00:00:00Z',updated_at:'2026-10-20T00:00:00Z',last_error_code:null,creation_audit_log_id:anchor}
const candidate = { anchor_order_id: anchor, window_from: '2026-10-05T16:00:00Z', window_to: '2026-10-12T16:00:00Z', cycle: 'weekly' }
for (const locale of ['zh-CN', 'en'] as const) test(`missing cycle selection and ledger translations (${locale})`, async ({ page }, info) => {
 const zh = locale === 'zh-CN', errors: string[] = []
 page.on('pageerror', error => errors.push(error.message))
 let empty = false
 await page.route('**/api/**', route => {
  expect(route.request().method()).toBe('GET')
  const url = new URL(route.request().url()), path = url.pathname
  const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
  expect(route.request().headers()['x-brand-id']).toBe(brand)
  if (path.endsWith('/commission-cycles/candidates')) return reply({brand_id:brand,items:empty?[]:[candidate],limit:100,offset:0})
  if (path.endsWith('/commission-cycles')) return reply({brand_id:brand,items:[cycleRecord],total_count:'1',limit:20,offset:0})
  if (path.endsWith('/ledger')) {
   const zero = Object.fromEntries(['recharge','winning','gift','commission'].map(source => [source,{available:'0',manual_frozen:'0',system_frozen:'0',withdrawal:'0'}]))
   return reply({total_count:'3',items:['bet','refund','prize'].map((type,index) => ({id:`cccccccc-cccc-4ccc-8ccc-${String(index+1).padStart(12,'0')}`,brand_id:brand,account_id:member,member_id:member,entry_type:type,reference_type:'configured_order',reference_id:anchor,operation_key:'isolated-ledger-review',before_snapshot:zero,delta_snapshot:zero,after_snapshot:zero,source_allocation:[],reason:'',actor_type:'system',actor_id:'',request_id:'isolated-review',version:index+1,created_at:'2026-11-01T00:00:00Z'}))})
  }
  throw Error('Unexpected request '+url)
 })
 const source = await (await page.request.get(origins.brand+'/src/CommissionCyclesManagement.vue')).text()
 const vuePath = source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
 for (const surface of ['cycles','ledger']) await page.route(origins.brand+'/__candidate-review-'+surface, route => route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">
 import{createApp,h}from'${vuePath}';
 import Cycles from'/src/CommissionCyclesManagement.vue';
 import Wallet from'/@fs${resolve('platform-web/src/WalletPanel.vue')}';
 import{createAdminI18n,adminI18nKey}from'/src/i18n.ts';
 import{createPlatformI18n,platformI18nKey}from'/@fs${resolve('platform-web/src/i18n.ts')}';
 import{setPlatformDisplaySettings}from'/@fs${resolve('shared/src/index.ts')}';
 import'/@fs${resolve('shared/src/styles.css')}';import'/src/style.css';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/controls.css')}';
 const settings={theme:'jade',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'};setPlatformDisplaySettings(settings);
 const admin=createAdminI18n();admin.setLocale('${locale}');const platform=createPlatformI18n();platform.setLocale('${locale}');
 const account={id:'${member}',super_admin:false,brand_ids:['${brand}'],permissions_by_brand:{'${brand}':['commission.view.brand','commission.run.brand']}};
 createApp({render:()=>h('div',{class:'management-console'},[h(${surface==='cycles'?'Cycles':'Wallet'},${surface==='cycles'?`{account,brandId:'${brand}'}`:`{brandId:'${brand}',memberId:'${member}',memberName:'isolated_member',locale:'${locale}',displaySettings:settings,view:'ledger'}`})])}).provide(adminI18nKey,admin).provide(platformI18nKey,platform).mount('#app');
 </script></html>`}))
 await page.goto(origins.brand+'/__candidate-review-cycles')
 const open = page.getByRole('button',{name:zh?'补建核算':'Add missing calculation',exact:true})
 const widths = info.project.name.includes('reflow')?[320,900,1280]:[page.viewportSize()!.width]
 for (const width of widths) {
  if (info.project.name.includes('reflow')) await page.setViewportSize({width,height:800})
  const cycleCell=page.locator('.commission-cycle-cell')
  await expect(cycleCell).toBeVisible()
  if (width <= 1080) expect(await cycleCell.evaluate(el=>getComputedStyle(el,'::before').content)).toBe('none')
  await page.screenshot({path:info.outputPath(`cycle-list-${locale}-${width}.png`)})
  await open.click()
  const dialog=page.getByRole('dialog',{name:zh?'补建核算':'Add missing calculation',exact:true})
  const picker=dialog.getByRole('combobox')
  await picker.click()
  const option=page.getByRole('option').filter({hasText:zh?'周结':'Weekly'})
  await expect(option).toHaveCount(1)
  await expect(option).toContainText('2026-10-06')
  await expect(option).toContainText('2026-10-13')
  await expect(page.getByRole('option',{name:zh?'全部':'All',exact:true})).toHaveCount(0)
  await option.click()
  if (width <= 900 || info.project.name.startsWith('ios') || info.project.name.startsWith('android')) {
   const lines = await picker.locator('.select-control-value > span').last().evaluate(element => {
    const text = element.firstChild!, value = text.textContent!;
    return ['2026-10-06 00:00:00','2026-10-13 00:00:00'].map(date => {
     const range = document.createRange(), start = value.indexOf(date);
     range.setStart(text,start); range.setEnd(text,start+date.length);
     return [...range.getClientRects()].map(rect=>({top:rect.top,bottom:rect.bottom}));
    });
   });
   expect(lines[0]).toHaveLength(1); expect(lines[1]).toHaveLength(1);
   expect(lines[1][0].top).toBeGreaterThanOrEqual(lines[0][0].bottom);
  }
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`missing-cycle-${locale}-${width}.png`)})
  await dialog.getByRole('button',{name:zh?'核对周期':'Review cycle',exact:true}).click()
  const review=page.getByRole('dialog')
  await expect(review).toContainText(zh?'周结':'Weekly')
  await expect(review).not.toContainText(anchor)
  await review.locator('.detail-modal-close').click()
 }
 empty=true;await open.click()
 await expect(page.getByRole('dialog')).toContainText(zh?'暂无待补建的核算周期':'No cycles need to be added.')
 await expect(page.getByRole('dialog').getByRole('combobox')).toHaveCount(0)
 await page.goto(origins.brand+'/__candidate-review-ledger')
 await page.waitForLoadState('networkidle')
 expect(errors).toEqual([])
 const ledger=page.getByRole('dialog')
 for (const text of zh?['投注','原路退款','派奖入账']:['Bet','Refund','Prize credited']) await expect(ledger.getByText(text,{exact:true})).toBeVisible()
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`ledger-types-${locale}.png`)})
 expect(errors).toEqual([])
})
