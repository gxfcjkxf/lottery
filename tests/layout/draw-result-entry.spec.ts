import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,gameId,expectNoPageOverflow} from './fixture'
for(const locale of ['zh-CN','en'] as const) for(const modelType of ['X_PLUS_Y','M_SELECT_N','DIGITS_0_9']) test(`manual results ${locale} ${modelType}`,async({page},info)=>{
 const zh=locale==='zh-CN', digits=modelType==='DIGITS_0_9'
 const model={model:modelType,regular_count:digits?0:6,special_count:digits?0:1,length:digits?3:0,allow_repeat:false,regular_pool:{min:1,max:49,allow_repeat:false},special_pool:{min:1,max:49,allow_repeat:false}}
 const settings={theme:'jade',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}
 const game={id:gameId,name:'Mark Six',model,status:'active',rule_published:true}
 const period={id:gameId,game_id:gameId,period_no:'2026117',draw_at:'2026-10-01T13:30:00Z',bet_start_at:'2026-10-01T00:00:00Z',bet_end_at:'2026-10-01T13:29:00Z',result:null,published_at:null,state:'waiting'}
 const writes:unknown[]=[],errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));let published=false
 await page.route('**/api/**',route=>{
  const req=route.request(),path=new URL(req.url()).pathname,ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(req.method()==='POST'){
   expect(path).toBe(`/api/v1/platform/platform-games/${gameId}/results`)
   writes.push({body:req.postDataJSON(),key:req.headers()['idempotency-key']})
   if(writes.length===1)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE'}}})
   expect(writes[1]).toEqual(writes[0]);published=true;return ok({})
  }
  if(path.endsWith('/draw-overview'))return ok({latest_period_no:digits?'00099999999999999999':'2026116',daily:digits,today:'2026-11-14',yesterday:'2026-11-13',date:new URL(req.url()).searchParams.get('date')||'2026-11-14',total_count:published?'1':'0',years:['2026','1999'],year:new URL(req.url()).searchParams.get('year')||'2026',has_more:false,pending_has_more:false,pending:[],items:published?[{period_no:period.period_no,draw_date:'1999-01-02',source:'published',result:digits?{digits:[1,2,3],regular:[],special:[]}:{digits:[],regular:[1,2,3,4,5,6],special:[7]}}]:[]})
  if(path.endsWith('/'+gameId))return ok({total_count:'1',game,periods:[period]})
  throw new Error('Unexpected request '+path)
 })
 await page.route('**/__draw-entry',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console"></div><script type="module">import {createApp,h} from '/node_modules/.vite/deps/vue.js';import C from '/@fs${resolve('shared/src/PlatformLottery.vue')}';import Host from '/@fs${resolve('shared/src/OperationToastHost.vue')}';import {setPlatformDisplaySettings} from '/@fs${resolve('shared/src/display-settings.ts')}';import '/@fs${resolve('shared/src/styles.css')}';import '/@fs${resolve('shared/src/management.css')}';import '/@fs${resolve('shared/src/controls.css')}';import '/@fs${resolve('shared/src/form-validation.css')}';setPlatformDisplaySettings(${JSON.stringify(settings)});createApp({render:()=>[h(C,${JSON.stringify({surface:'platform',brandId:'',initialGameId:gameId,drawsOnly:true,locale,canManage:true,canConnect:false,displaySettings:settings})}),h(Host,{locale:'${locale}'})]}).mount('#app')</script></html>`}))
 await page.goto(origins.platform+'/__draw-entry')
 await expect(page.getByText(zh?'实时采集':'Live collection',{exact:true})).toHaveCount(0)
 await expect(page.getByText(zh?'待开奖期次':'Pending draws',{exact:true})).toHaveCount(0)
 await page.getByRole('button',{name:zh?'手动添加开奖结果':'Manually add draw result',exact:true}).click()
 const dialog=page.getByRole('dialog'),periodInput=dialog.getByRole('textbox',{name:zh?'期号':'Period number',exact:true}),save=dialog.locator('[type=submit]')
 const attemptSave=async()=>{await expect(dialog.locator('form')).not.toHaveAttribute('data-write-locked','true');await save.click()}
 await expect(periodInput).toHaveValue(digits?'00100000000000000000':'2026117')
 const today=await page.evaluate(()=>{const p=Object.fromEntries(new Intl.DateTimeFormat('en',{timeZone:'Asia/Singapore',year:'numeric',month:'2-digit',day:'2-digit'}).formatToParts(new Date()).map(v=>[v.type,v.value]));return `${p.year}-${p.month}-${p.day}`})
 await expect(dialog.locator('.platform-date-input-display')).toHaveValue(today)
 await periodInput.fill(period.period_no);await dialog.locator('.platform-date-input-display').fill('1999-01-02')
 const cells=dialog.locator('.draw-result-cells input');await expect(cells).toHaveCount(digits?3:7)
 await cells.first().focus();await cells.first().press('Tab');await expect(dialog.getByRole('alert')).toHaveCount(0)
 await attemptSave();await expect(dialog.getByRole('alert')).toHaveCount(1)
 await expect(dialog.getByRole('alert')).toHaveText(zh?`请填写${digits?'位数':'普通号码'} 1 开奖号码。`:`Enter the draw number for ${digits?'Digits':'Regular numbers'} 1.`)
 expect(writes).toHaveLength(0)
 if(!digits){
  await cells.nth(0).fill('50');await expect(cells.nth(0)).toBeFocused();await expect(dialog.getByRole('alert')).toHaveCount(0)
  await attemptSave();await expect(dialog.getByRole('alert')).toHaveCount(1);await expect(dialog.getByRole('alert')).toContainText('1–49'.replace('–',zh?'–':' to '))
  await cells.nth(0).fill('01');await expect(cells.nth(1)).toBeFocused()
  await cells.nth(1).fill('01');await expect(dialog.getByRole('alert')).toHaveCount(0)
  await attemptSave();await expect(dialog.getByRole('alert')).toHaveCount(1);await expect(dialog.getByRole('alert')).toContainText(zh?'不允许重复':'cannot be repeated')
  await cells.nth(1).fill('2');await cells.nth(1).press('Tab');await expect(cells.nth(2)).toBeFocused()
  for(let i=2;i<6;i++)await cells.nth(i).fill(String(i+1).padStart(2,'0'))
  await cells.nth(6).fill('01');await expect(cells.nth(6)).toBeFocused();await expect(dialog.getByRole('alert')).toHaveCount(0)
  await attemptSave();await expect(dialog.getByRole('alert')).toHaveCount(1);await expect(dialog.getByRole('alert')).toContainText(zh?'不允许重复':'cannot be repeated')
  await cells.nth(6).fill('07')
 }else{
  await cells.nth(0).fill('1');await expect(cells.nth(1)).toBeFocused()
  await attemptSave();await expect(dialog.getByRole('alert')).toHaveCount(1);await expect(dialog.getByRole('alert')).toHaveText(zh?'请填写位数 2 开奖号码。':'Enter the draw number for Digits 2.')
  await cells.nth(1).fill('1');await expect(dialog.getByRole('alert')).toHaveCount(0)
  await attemptSave();await expect(dialog.getByRole('alert')).toHaveCount(1);await expect(dialog.getByRole('alert')).toContainText(zh?'不允许重复':'cannot be repeated')
  await cells.nth(1).fill('2');await cells.nth(2).fill('3')
 }
 const narrowInputs=async()=>{for(const input of await cells.all()){const box=await input.boundingBox();expect(box!.width).toBeLessThanOrEqual(44)}}
 await narrowInputs()
 if(info.project.name.endsWith('reflow'))for(const width of [1440,900,620,320]){await page.setViewportSize({width,height:800});await expectNoPageOverflow(page);await narrowInputs()}
 await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`manual-${locale}-${modelType}.png`)})
 await attemptSave();await expect(page.locator('.toast-error')).toBeVisible()
 await expect(cells.first()).toBeDisabled();await expect(dialog.locator('.detail-modal-close')).toBeDisabled()
 await page.locator('.toast-error button').click();await dialog.locator('[type=submit]').click()
 await expect(dialog).toHaveCount(0);await expect(page.locator('.draw-overview table')).toContainText(period.period_no);if(digits) await expect(page.locator('.draw-overview .platform-date-input-display')).toHaveValue('1999-01-02');else await expect(page.getByRole('combobox',{name:zh?'年份':'Year',exact:true})).toHaveAttribute('data-value','1999')
 expect((writes[0] as any).body.draw_date).toBe('1999-01-02');expect((writes[0] as any).body.period_no).toBe(period.period_no)
 expect((writes[0] as any).body.result).toEqual(digits?{regular:[],special:[],digits:[1,2,3]}:{regular:[1,2,3,4,5,6],special:[7],digits:[]})
 await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`history-${locale}-${modelType}.png`)});expect(errors).toEqual([])
})

for(const format of ['YYYY-MM-DD','DD/MM/YYYY','MM/DD/YYYY'] as const)test(`manual date keyboard and calendar ${format}`,async({page})=>{
 const settings={theme:'jade',timezone:'Asia/Singapore',date_format:format,time_format:'HH:mm:ss'}
 await page.route('**/__manual-date',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import {createApp,h,ref} from '/node_modules/.vite/deps/vue.js';import C from '/@fs${resolve('shared/src/PlatformDateInput.vue')}';import '/@fs${resolve('shared/src/styles.css')}';import '/@fs${resolve('shared/src/controls.css')}';const date=ref('');createApp({render:()=>h('form',[h(C,{modelValue:date.value,'onUpdate:modelValue':v=>date.value=v,displaySettings:${JSON.stringify(settings)},locale:'en',label:'Draw date',editable:true,required:true}),h('output',{'data-testid':'value'},date.value)])}).mount('#app')</script></html>`}))
 await page.goto(origins.platform+'/__manual-date')
 const input=page.getByRole('textbox',{name:'Draw date'})
 const formatted=(day:string)=>format==='YYYY-MM-DD'?`1999-01-${day}`:format==='DD/MM/YYYY'?`${day}/01/1999`:`01/${day}/1999`
 await input.pressSequentially(formatted('02'));await expect(page.getByTestId('value')).toHaveText('1999-01-02')
 await page.locator('.platform-date-input-button').click();await page.getByRole('button',{name:'1999-01-03',exact:true}).click()
 await expect(input).toHaveValue(formatted('03'));await expect(page.getByTestId('value')).toHaveText('1999-01-03')
 await input.fill(format==='YYYY-MM-DD'?'1999-02-30':format==='DD/MM/YYYY'?'30/02/1999':'02/30/1999')
 await expect(page.getByTestId('value')).toHaveText('');expect(await input.evaluate((el:HTMLInputElement)=>el.checkValidity())).toBe(false)
 await input.fill('');await input.pressSequentially(formatted('04'));await expect(page.getByTestId('value')).toHaveText('1999-01-04')
 expect(await input.evaluate((el:HTMLInputElement)=>el.checkValidity())).toBe(true)
})
