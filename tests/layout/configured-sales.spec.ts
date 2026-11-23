import {expectDialogFieldColumns} from './dialog-form-columns'
import { openBrandPage } from './real-console-session'
import {test,expect} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {configuredDemo,configuredOptionPriceKeys} from '../../shared/src/configured-rules'
import {selectControl} from '../select-control'
import {gameId,origins,installLayoutApi,expectNoPageOverflow} from './fixture'
const snapshot=JSON.parse(readFileSync('backend/internal/rules/config/examples/lucky_group_preview.json','utf8'))
const emptyAvailability=(surface:'admin'|'platform')=>({rule_id:gameId,platform:{version:0,items:{}},brand:surface==='admin'?{version:0,items:{}}:null})
const version={id:gameId,game_id:gameId,revision:1,snapshot,snapshot_hash:'test',cases:[],status:'published',validation:null}
for(const locale of ['en','zh-CN'] as const)for(const mode of ['first','read-only','outdated'] as const)test(`configured sales ${mode} ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN',errors:string[]=[],writes:{body:string|null;key:string|undefined}[]=[]
 page.on('pageerror',error=>errors.push(error.message))
 let saved:Record<string,unknown>|null=mode==='outdated'?{version:3,rule_id:'bbbbbbbb-bbbb-4bbb-8bbb-000000000002',enabled:true,prices:{...snapshot.prices,plates:{A:{lucky_price:'1.5'}}},min_points:'1',max_points:'100',max_period_points:'10000',max_user_period_points:'5000'}:null
 await page.route('**/api/v1/**',async route=>{
  const r=route.request(),path=new URL(r.url()).pathname
  expect(r.headers()['x-brand-id']).toBe(gameId)
  const ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(r.method()==='GET'&&path.endsWith('/configured/rules'))return ok(version)
  expect(path).toBe(`/api/v1/admin/platform-games/${gameId}/configured/sales`)
  if(r.method()==='GET')return saved?ok(saved):route.fulfill({status:404,json:{success:false,error:{code:'RESOURCE_NOT_FOUND',message:'No sales'}}})
  expect(mode).not.toBe('read-only');expect(r.method()).toBe('POST')
  writes.push({body:r.postData(),key:r.headers()['idempotency-key']})
  const input=r.postDataJSON();expect(input.rule_id).toBe(gameId)
  expect(input.version).toBe(mode==='outdated'?3:0)
  if(mode==='first')expect(input.min_points).toBe('0.25')
  if(mode==='outdated'&&writes.length===1)return route.fulfill({status:400,json:{success:false,error:{code:'CONFIGURED_SALES_ODDS_INVALID',message:'Invalid betting odds'}}})
  saved={...input,version:input.version+1}
  if(mode==='first'&&writes.length===1)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Unknown result'}}})
  return ok(saved)
 })
 const props={surface:'admin',brandId:gameId,gameId,locale,snapshot:null,canWrite:mode!=='read-only'}
 await page.route('**/configured-sales-harness',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><main id="app" class="management-console panel" style="margin:12px;padding:12px;min-width:0"></main><script id="harness-data" type="application/json">${JSON.stringify({component:'ConfiguredPublishing',props,locale,operationToasts:mode==='outdated'})}</script><script type="module" src="/tests/layout-harness.ts"></script></body></html>`}))
 await page.goto(`${origins.brand}/configured-sales-harness`)
 await expect(page.getByRole('heading',{name:zh?'投注设置':'Betting settings'})).toBeVisible()
 if(mode==='read-only'){
  await expect(page.getByRole('textbox')).toHaveCount(0)
  await expect(page.getByRole('button')).toHaveCount(0)
 }else{
  await expect(page.getByText(zh?'停用投注':'Disable betting',{exact:true})).toBeVisible()
  await expect(page.getByText(zh?'启用投注':'Enable betting',{exact:true})).toBeVisible()
  const save=page.getByRole('button',{name:zh?'保存投注设置':'Save betting settings',exact:true})
  const minimum=page.getByLabel(zh?'每注最低积分':'Minimum entry stake',{exact:true})
  if(mode==='first'){
   await expect(page.getByRole('switch',{name:zh?'启用投注':'Enable betting'})).not.toBeChecked()
   await expect(save).toBeEnabled();expect(writes).toHaveLength(0)
   await minimum.fill('0');await expect(save).toBeDisabled();await expect(page.getByRole('alert')).toBeVisible()
   await minimum.fill('0.25');await page.getByRole('switch').check()
   await expect(page.getByRole('switch')).toBeChecked()
   await page.getByRole('switch').press('Space')
   await expect(page.getByRole('switch')).not.toBeChecked()
   await page.getByRole('switch').press('Space')
  }
  await expect(page.locator('.configured-odds-editor')).toHaveCount(0)
  if(mode==='outdated')await expect(page.getByRole('status')).toHaveText(zh?'平台规则已更新，需重新保存。':'Platform rules have changed. These settings need to be saved again.')
  await save.click()
  if(mode==='outdated'){
   await expect(page.getByText(zh?'部分赔率缺失或不符合当前规则，请检查盘口赔率。':'Odds are missing or invalid for the current rules. Review Plates & odds.',{exact:true})).toBeVisible()
   await expect(page.getByRole('button',{name:zh?'重试':'Retry',exact:true})).toHaveCount(0)
   await expect(page.getByRole('button',{name:zh?'重新读取':'Reload',exact:true})).toHaveCount(0)
   await expect(minimum).toHaveValue('1')
   await page.screenshot({path:info.outputPath(`sales-validation-${locale}.png`)})
   await save.click()
  }
  if(mode==='first'){
   await expect(page.getByRole('button',{name:zh?'重试':'Retry'})).toBeVisible()
   await expect(minimum).toBeDisabled()
   await expect(page.getByRole('switch')).toBeDisabled()
   await page.locator('.configured-actions .button-primary').click()
   await expect.poll(()=>writes.length).toBe(2);expect(writes[0]).toEqual(writes[1])
  }
  await expect(save).toBeEnabled();await expect(minimum).toBeEnabled()
  const actionRight=await save.evaluate(button=>button.parentElement!.getBoundingClientRect().right-button.getBoundingClientRect().right)
  expect(Math.abs(actionRight)).toBeLessThanOrEqual(1)
  expect(saved?.prices).toEqual(mode==='outdated'?{...snapshot.prices,plates:{A:{lucky_price:'1.5'}}}:snapshot.prices)
 }
 if(info.project.name.endsWith('reflow'))for(const width of [320,480,768,1280]){
  await page.setViewportSize({width,height:800});await expectNoPageOverflow(page)
  if(width===320)await page.screenshot({path:info.outputPath(`sales-320-${mode}-${locale}.png`),fullPage:true})
 }
 await expectNoPageOverflow(page);expect(errors).toEqual([])
 await page.screenshot({path:info.outputPath(`sales-${mode}-${locale}.png`),fullPage:true})
})
for(const locale of ['en','zh-CN'] as const)for(const oddsOnly of [false,true])test(`brand catalog opens ${oddsOnly?'odds':'configured sales'} ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN',paths:string[]=[],writes:Array<{body:unknown;key:string|undefined}>=[]
 let saved:Record<string,unknown>|null=null
 const game={id:gameId,name:'Current game',code:'current_game',model:snapshot.catalog.model,timezone:'UTC',version:1,connected:true,status:'active',deletable:false,brand_enabled:true,brand_version:1,rule_published:true,sales_status:'unconfigured'}
 await page.route('**/api/v1/**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname;paths.push(path)
  if(request.method()==='POST'){
   expect(path).toBe(`/api/v1/admin/platform-games/${gameId}/configured/sales`)
   writes.push({body:request.postDataJSON(),key:request.headers()['idempotency-key']})
   if(writes.length===1)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Unavailable'}}})
   expect(writes[1]).toEqual(writes[0]);saved={...request.postDataJSON(),version:1}
   return route.fulfill({json:{success:true,data:saved}})
  }
  expect(request.method()).toBe('GET')
  if(path==='/api/v1/admin/platform-games')return route.fulfill({json:{success:true,data:{games:[game],limit:50,offset:0,total_count:'1'}}})
  if(path.endsWith('/configured/rules'))return route.fulfill({json:{success:true,data:version}})
  if(path.endsWith('/configured/availability'))return route.fulfill({json:{success:true,data:emptyAvailability('admin')}})
  expect(path).toBe(`/api/v1/admin/platform-games/${gameId}/configured/sales`)
  if(saved)return route.fulfill({json:{success:true,data:saved}})
  return route.fulfill({status:404,json:{success:false,error:{code:'RESOURCE_NOT_FOUND',message:'No sales'}}})
 })
 const props={surface:'admin',brandId:gameId,locale,canManage:false,canConnect:false,canSell:true,oddsOnly}
 await page.route('**/sales-entry-harness',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><main id="app" class="management-console" style="padding:12px;min-width:0"></main><script id="harness-data" type="application/json">${JSON.stringify({component:'PlatformLottery',props,locale})}</script><script type="module" src="/tests/layout-harness.ts"></script></body></html>`}))
 await page.goto(`${origins.brand}/sales-entry-harness`)
 await expect(page.getByText(zh?'未配置投注':'Betting not configured',{exact:true})).toBeVisible()
 await page.locator('.member-menu-trigger').click()
 await page.getByRole('menuitem',{name:oddsOnly?(zh?'盘口赔率':'Plates & odds'):(zh?'投注设置':'Betting settings'),exact:true}).click()
 const dialog=page.getByRole('dialog')
 await expect(dialog).toHaveAccessibleName(`${game.name} · ${oddsOnly?(zh?'盘口赔率':'Plates & odds'):(zh?'投注设置':'Betting settings')}`)
 await expect(dialog.getByRole('button',{name:oddsOnly?(zh?'保存':'Save'):(zh?'保存投注设置':'Save betting settings'),exact:true})).toBeEnabled()
 if(oddsOnly)await expect(dialog.getByRole('button',{name:zh?'保存':'Save',exact:true})).toBeInViewport()
 await expect(dialog.getByRole('switch')).toHaveCount(oddsOnly?0:1)
 await expect(dialog.locator('.configured-odds-editor')).toHaveCount(oddsOnly?1:0)
 expect(paths).toEqual(['/api/v1/admin/platform-games',`/api/v1/admin/platform-games/${gameId}/configured/rules`,...(oddsOnly?[`/api/v1/admin/platform-games/${gameId}/configured/availability`]:[]),`/api/v1/admin/platform-games/${gameId}/configured/sales`])
 if(!oddsOnly)await expectDialogFieldColumns(dialog)
 if(info.project.name.endsWith('reflow'))for(const width of [320,480,768,1280]){await page.setViewportSize({width,height:800});await expectNoPageOverflow(page);if(!oddsOnly)await expectDialogFieldColumns(dialog)}
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`sales-entry-${locale}.png`),fullPage:true})
 await dialog.getByRole('button',{name:oddsOnly?(zh?'保存':'Save'):(zh?'保存投注设置':'Save betting settings'),exact:true}).click()
 await expect(dialog).toBeVisible()
 await expect(dialog.getByRole('button',{name:zh?'重试':'Retry',exact:true})).toBeEnabled()
 await dialog.getByRole('button',{name:zh?'重试':'Retry',exact:true}).click()
 await expect(dialog).toHaveCount(0)
 await expect(page.getByRole('row').filter({hasText:game.name})).toBeVisible()
 await expectNoPageOverflow(page)
})

for(const locale of ['en','zh-CN'] as const)test(`mark six odds use player names and preserve plate edits ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN'
 const full=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json','utf8')))
 let submitted:any
 await page.route('**/api/v1/**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/configured/availability'))return route.fulfill({json:{success:true,data:emptyAvailability('admin')}})
  const data=request.method()==='POST'?(submitted=request.postDataJSON()):path.endsWith('/rules')?{...version,snapshot:full}:submitted??{version:1,rule_id:gameId,enabled:true,prices:full.prices,min_points:'1',max_points:'100',max_period_points:'10000',max_user_period_points:'5000'}
  await route.fulfill({json:{success:true,data}})
 })
 const props={surface:'admin',brandId:gameId,gameId,locale,snapshot:null,canWrite:true,oddsOnly:true}
 await page.route('**/mark-six-sales-harness',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><main id="app" class="management-console panel" style="margin:12px;padding:12px;min-width:0"></main><script id="harness-data" type="application/json">${JSON.stringify({component:'ConfiguredPublishing',props,locale})}</script><script type="module" src="/tests/layout-harness.ts"></script></html>`}))
 await page.goto(`${origins.brand}/mark-six-sales-harness`)
 const odds=page.locator('.configured-odds-editor')
 await expect(odds).toBeVisible()
 await expect(page.getByRole('switch')).toHaveCount(0)
 await expect(odds.getByRole('textbox')).toHaveCount(50)
 await odds.getByLabel(zh?'批量赔率':'Batch odds',{exact:true}).fill('47.5')
 await odds.getByRole('button',{name:zh?'批量修改':'Apply to play',exact:true}).click()
 for(const input of await odds.locator('.configured-odds-fields input').all())await expect(input).toHaveValue('47.5')
 const row=odds.locator('.configured-odds-item').first()
 const labelBox=await row.locator('span').boundingBox(),inputBox=await row.locator('input').boundingBox()
 expect(Math.abs((labelBox!.y+labelBox!.height/2)-(inputBox!.y+inputBox!.height/2))).toBeLessThan(2)
 expect(inputBox!.width).toBeLessThanOrEqual(89)
 if(info.project.name.endsWith('reflow'))for(const width of [320,619,620,621,1280]){
  await page.setViewportSize({width,height:800});await expectNoPageOverflow(page)
  if(width===320)await odds.screenshot({path:info.outputPath(`mark-six-numbers-320-${locale}.png`)})
 }
 await odds.screenshot({path:info.outputPath(`mark-six-numbers-${locale}.png`)})
 await selectControl(odds.getByRole('combobox',{name:zh?'玩法':'Play',exact:true}),{value:'play:special_sides'})
 await odds.getByLabel(zh?'大':'big',{exact:true}).fill('1.98')
 await selectControl(odds.getByRole('combobox',{name:zh?'盘口':'Plate',exact:true}),{value:'B'})
 await expect(odds.getByLabel(zh?'大':'big',{exact:true})).toHaveValue('2')
 await odds.getByLabel(zh?'大':'big',{exact:true}).fill('1.88')
 await selectControl(odds.getByRole('combobox',{name:zh?'盘口':'Plate',exact:true}),{value:'A'})
 await expect(odds.getByLabel(zh?'大':'big',{exact:true})).toHaveValue('1.98')
 if(info.project.name.endsWith('reflow'))for(const width of [320,619,620,621,1280]){await page.setViewportSize({width,height:800});await expectNoPageOverflow(page)}
 await expectNoPageOverflow(page)
 await odds.screenshot({path:info.outputPath(`mark-six-odds-${locale}.png`)})
 await page.getByRole('button',{name:zh?'保存':'Save',exact:true}).click()
 await expect.poll(()=>submitted?.prices.plates.A.big).toBe('1.98')
 expect(Object.values(submitted.prices.plates.A)).toContain('47.5')
 expect(Object.values(submitted.prices.plates.B)).not.toContain('47.5')
 expect(submitted.prices.plates.B.big).toBe('1.88')
 expect(submitted.prices.plates.A.small).toBe('2')
 expect(submitted.enabled).toBe(true)
 expect(submitted.min_points).toBe('1')
 expect(submitted.max_points).toBe('100')
 expect(submitted.max_period_points).toBe('10000')
 expect(submitted.max_user_period_points).toBe('5000')
})

for(const locale of ['en','zh-CN'] as const)test(`brand odds menu read-only ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN'
 await installLayoutApi(page,locale)
 await page.route('**/api/v1/admin/**',async route=>{
  expect(route.request().method()).toBe('GET')
  const path=new URL(route.request().url()).pathname
  const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(path.endsWith('/me'))return reply({account:{id:gameId,username:'odds_viewer',super_admin:false,brand_ids:[gameId],permissions:[],permissions_by_brand:{[gameId]:['platform_game.view.brand']},platform_permissions:[]}})
  if(path.endsWith('/brands'))return reply({items:[{id:gameId,code:'layout',name:'Layout trial',status:'active',member_count:'0',agent_count:'0',recharge_quota_available:'0',created_at:'2026-10-24T00:00:00Z'}]})
  expect(route.request().headers()['x-brand-id']).toBe(gameId)
  if(path.endsWith('/platform-games'))return reply({games:[{id:gameId,name:'Mark Six',code:'mark_six',model:snapshot.catalog.model,timezone:'UTC',version:1,connected:true,status:'active',deletable:false,brand_enabled:true,brand_version:1,rule_published:true,sales_status:'ready'},{id:'bbbbbbbb-bbbb-4bbb-8bbb-000000000002',name:'Unconnected game',connected:false,status:'active'}],limit:50,offset:0,total_count:'2'})
  if(path.endsWith('/configured/rules'))return reply(version)
  if(path.endsWith('/configured/availability'))return reply(emptyAvailability('admin'))
  if(path.endsWith('/configured/sales'))return reply({version:1,rule_id:gameId,enabled:true,prices:snapshot.prices,min_points:'1',max_points:'100',max_period_points:'10000',max_user_period_points:'5000'})
  throw Error(`Unexpected odds menu request: ${path}`)
 })
 await page.goto(origins.brand)
 await expect(page.locator('.management-console')).toBeVisible()
 const more=page.locator('.mobile-nav > button').last()
 if(await more.isVisible())await more.click()
 const nav=page.locator(await more.isVisible()?'.mobile-more-menu':'.side-nav')
 await openBrandPage(page,zh?'盘口赔率':'Plates & odds')
 await expect(page.locator('.breadcrumbs b')).toHaveText(zh?'盘口赔率':'Plates & odds')
 await expect(page.getByText('Unconnected game',{exact:true})).toHaveCount(0)
 await expect(page.locator('.management-list tbody tr')).toHaveCount(1)
 await page.locator('.member-menu-trigger').click()
 await page.getByRole('menuitem',{name:zh?'盘口赔率':'Plates & odds',exact:true}).click()
 const dialog=page.getByRole('dialog')
 await expect(dialog.locator('.configured-odds-fields input').first()).toBeDisabled()
 await expect(dialog.getByRole('combobox').first()).toBeEnabled()
 await dialog.getByRole('combobox').first().click()
 await expect(page.getByRole('option',{name:'A',exact:true})).toBeVisible()
 await page.keyboard.press('Escape')
 await expect(dialog.getByRole('switch')).toHaveCount(0)
 await expect(dialog.getByRole('button',{name:zh?'保存':'Save',exact:true})).toHaveCount(0)
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`odds-menu-readonly-${locale}.png`),fullPage:true})
 await dialog.locator('.detail-modal-close').click()
 if(await more.isVisible())await more.click()
 await nav.getByRole('button',{name:zh?'彩种管理':'Game management',exact:true}).click()
 await expect(page.locator('.management-list tbody tr')).toHaveCount(2)
})

for(const locale of ['en','zh-CN'] as const)test(`Canada28 odds share player categories ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN'
 const full=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/canada_28.json','utf8')))
 let submitted:any
 await page.route('**/api/v1/**',async route=>{
  const request=route.request()
  if(request.url().endsWith('/configured/availability'))return route.fulfill({json:{success:true,data:{...emptyAvailability('admin'),brand:{version:1,items:{small:{small:false}}}}}})
  const data=request.method()==='POST'?(submitted=request.postDataJSON()):request.url().endsWith('/rules')?{...version,snapshot:full}:submitted??{version:1,rule_id:gameId,enabled:true,prices:full.prices,min_points:'1',max_points:'100',max_period_points:'10000',max_user_period_points:'5000'}
  await route.fulfill({json:{success:true,data}})
 })
 const props={surface:'admin',brandId:gameId,gameId,locale,snapshot:null,canWrite:true,oddsOnly:true}
 await page.route('**/canada-odds-harness',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><main id="app" class="management-console panel" style="margin:12px;padding:12px;min-width:0"></main><script id="harness-data" type="application/json">${JSON.stringify({component:'ConfiguredPublishing',props,locale})}</script><script type="module" src="/tests/layout-harness.ts"></script></html>`}))
 await page.goto(`${origins.brand}/canada-odds-harness`)
 const odds=page.locator('.configured-odds-editor'),play=odds.getByRole('combobox',{name:zh?'玩法':'Play',exact:true})
 await expect(play).toHaveAttribute('data-value','group:sides')
 await play.click()
 await expect(page.getByRole('option')).toHaveText(zh?['两面','组合','总和','尾数']:['Sides','Combinations','Sum','Tails'])
 await page.keyboard.press('Escape')
 const fields=odds.locator('.configured-odds-fields input')
 await expect(fields).toHaveCount(10)
 await odds.getByLabel(zh?'批量赔率':'Batch odds',{exact:true}).fill('1.95')
 await odds.getByRole('button',{name:zh?'批量修改':'Apply to play',exact:true}).click()
 await expect(fields.nth(1)).toBeDisabled()
 await expect(fields.nth(1)).toHaveValue(zh?'已停用':'Disabled')
 for(const input of await odds.locator('.configured-odds-fields input:enabled').all())await expect(input).toHaveValue('1.95')
 for(const [group,count] of [['shapes',3],['sums',28],['tailNumbers',16]] as const){
  await selectControl(play,{value:`group:${group}`})
  await expect(fields).toHaveCount(count)
  await expect(odds.getByLabel(zh?'批量赔率':'Batch odds',{exact:true})).toHaveValue('')
  const labels=await odds.locator('.configured-odds-fields .configured-odds-label').allTextContents()
  if(group==='sums')expect(labels).toEqual(Array.from({length:28},(_,i)=>String(i)))
  if(group==='tailNumbers')expect(labels.slice(8)).toEqual(zh?['大','小','单','双','大单','大双','小单','小双']:['Big','Small','Odd','Even','Big odd','Big even','Small odd','Small even'])
  await expectNoPageOverflow(page)
 }
 if(info.project.name.endsWith('reflow'))for(const width of [320,380,620,1280]){
  await page.setViewportSize({width,height:800});await expectNoPageOverflow(page)
  if(width===320)await odds.screenshot({path:info.outputPath(`canada-tail-odds-320-${locale}.png`)})
 }
 await odds.screenshot({path:info.outputPath(`canada-tail-odds-${locale}.png`)})
 await selectControl(play,{value:'group:sides'})
 await expect(fields.nth(1)).toBeDisabled()
 await expect(fields.nth(1)).toHaveValue(zh?'已停用':'Disabled')
 for(const input of await odds.locator('.configured-odds-fields input:enabled').all())await expect(input).toHaveValue('1.95')
 await odds.screenshot({path:info.outputPath(`canada-side-odds-${locale}.png`)})
 await page.getByRole('button',{name:zh?'保存':'Save',exact:true}).click()
 await expect.poll(()=>submitted?.version).toBe(1)
 const expected=structuredClone(full.prices)
 for(const p of full.catalog.plays.filter(p=>p.id!=='small'&&(p.group==='sides'||p.group==='extremes')))for(const option of full.catalog.domains[p.selection.domain]!.options)for(const key of configuredOptionPriceKeys(full,p,option))expected.plates.A![key]='1.95'
 expect(submitted.prices).toEqual(expected)
 await expect(page.getByRole('button',{name:zh?'保存':'Save',exact:true})).toBeDisabled()
})

for(const locale of ['en','zh-CN'] as const)test(`platform odds read only and missing rules ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN';let missing=false
 await page.route('**/api/v1/**',route=>{
  expect(route.request().method()).toBe('GET')
  const path=new URL(route.request().url()).pathname
  if(path.endsWith('/configured/availability'))return route.fulfill({json:{success:true,data:emptyAvailability('platform')}})
  expect(path).toBe(`/api/v1/platform/platform-games/${gameId}/configured/rules`)
  return route.fulfill({json:{success:true,data:missing?null:version}})
 })
 const props={surface:'platform',brandId:'',gameId,locale,snapshot:null,canWrite:false,oddsOnly:true}
 await page.route('**/platform-odds-harness',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><main id="app" class="management-console panel" style="margin:12px;padding:12px;min-width:0"></main><script id="harness-data" type="application/json">${JSON.stringify({component:'ConfiguredPublishing',props,locale})}</script><script type="module" src="/tests/layout-harness.ts"></script></html>`}))
 await page.goto(`${origins.brand}/platform-odds-harness`)
 await expect(page.locator('.configured-odds-editor')).toBeVisible()
 await expect(page.locator('.configured-odds-fields input').first()).toBeDisabled()
 await expect(page.getByRole('button',{name:zh?'保存':'Save',exact:true})).toHaveCount(0)
 await expect(page.locator('.configured-odds-batch')).toHaveCount(0)
 await expectNoPageOverflow(page)
 missing=true;await page.reload()
 await expect(page.locator('.empty-state')).toBeVisible()
 await expect(page.locator('.configured-odds-editor')).toHaveCount(0)
 await expect(page.getByRole('button',{name:zh?'保存':'Save',exact:true})).toHaveCount(0)
})

for(const surface of ['platform','admin'] as const)test(`odds items stay distinct across themes ${surface}`,async({page},info)=>{
 const full=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/canada_28.json','utf8')))
 const game={id:gameId,name:'Canada 28',code:'canada_28',model:full.catalog.model,timezone:'Asia/Singapore',version:1,connected:true,status:'active',deletable:false,brand_enabled:true,brand_version:1,rule_published:true,sales_status:'ready'}
 const settings={theme:'ocean',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}
 await page.route('**/api/**',route=>{
  const path=new URL(route.request().url()).pathname
  expect(route.request().method()).toBe('GET')
  const ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(path===`/api/v1/${surface}/platform-games`)return ok({games:[game],limit:50,offset:0,total_count:'1'})
  if(path.endsWith('/configured/rules'))return ok({...version,snapshot:full})
  if(path.endsWith('/configured/availability'))return ok(emptyAvailability(surface))
  expect(surface).toBe('admin');expect(path).toBe(`/api/v1/admin/platform-games/${gameId}/configured/sales`)
  return ok({version:1,rule_id:gameId,enabled:true,prices:full.prices,min_points:'1',max_points:'100',max_period_points:'10000',max_user_period_points:'5000'})
 })
 const props={surface,brandId:surface==='admin'?gameId:'',locale:'en',canManage:true,canConnect:false,canSell:true,oddsOnly:true,displaySettings:settings}
 await page.route('**/odds-theme-harness',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><main id="app" class="management-console" style="padding:12px;min-width:0"></main><script id="harness-data" type="application/json">${JSON.stringify({component:'PlatformLottery',props,locale:'en'})}</script><script type="module" src="/tests/layout-harness.ts"></script></html>`}))
 await page.goto(`${origins.brand}/odds-theme-harness`)
 await page.addStyleTag({url:`${origins.brand}/@fs${resolve('shared/src/themes.css')}`})
 await page.addStyleTag({content:'*,*::before,*::after{transition:none!important;animation:none!important}'})
 if(surface==='admin'){
  await page.locator('.member-menu-trigger').click()
  await page.getByRole('menuitem',{name:'Plates & odds',exact:true}).click()
 }else await page.getByRole('button',{name:'Plates & odds',exact:true}).click()
 const dialog=page.getByRole('dialog'),items=dialog.locator('.configured-odds-item')
 await expect(items).toHaveCount(10)
 const preset=page.viewportSize()!
 for(const theme of ['jade','vermilion','ocean','obsidian'])for(const mode of ['light','dark']){
  await page.evaluate(async({theme,mode,path})=>{
   localStorage.setItem('lottery.theme.mode',mode)
   const {applyTheme}=await import(path);applyTheme(theme)
  },{theme,mode,path:`/@fs${resolve('shared/src/theme.ts')}`})
  for(const width of info.project.name.endsWith('reflow')?[1280,768,320]:[preset.width]){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
   const appearance=await items.evaluateAll(labels=>{
    const canvas=document.createElement('canvas'),ctx=canvas.getContext('2d')!
    const rgb=(color:string)=>{ctx.clearRect(0,0,1,1);ctx.fillStyle=color;ctx.fillRect(0,0,1,1);return [...ctx.getImageData(0,0,1,1).data].slice(0,3)}
    const luminance=(color:string)=>rgb(color).map(v=>{v/=255;return v<=.04045?v/12.92:((v+.055)/1.055)**2.4}).reduce((sum,v,i)=>sum+v*[.2126,.7152,.0722][i]!,0)
    const contrast=(a:string,b:string)=>{const x=luminance(a),y=luminance(b);return(Math.max(x,y)+.05)/(Math.min(x,y)+.05)}
    return labels.map(label=>{
     const style=getComputedStyle(label),input=label.querySelector('input')!,inputStyle=getComputedStyle(input),span=label.querySelector('span')!,textStyle=getComputedStyle(span),panel=getComputedStyle(label.closest('dialog')!)
     const box=label.getBoundingClientRect(),children=[span,input].map(el=>el.getBoundingClientRect())
     return {fill:contrast(style.backgroundColor,panel.backgroundColor),title:contrast(textStyle.color,style.backgroundColor),value:contrast(inputStyle.color,inputStyle.backgroundColor),accent:contrast(style.borderLeftColor,style.backgroundColor),border:style.borderTopWidth,contained:children.every(r=>r.left>=box.left&&r.right<=box.right+.5&&r.top>=box.top&&r.bottom<=box.bottom+.5)}
    })
   })
   for(const item of appearance){
    expect(item.fill,`${theme}/${mode}`).toBeGreaterThan(1.2)
    expect(item.title,`${theme}/${mode}`).toBeGreaterThanOrEqual(4.5)
    expect(item.value,`${theme}/${mode}`).toBeGreaterThanOrEqual(4.5)
    expect(item.accent,`${theme}/${mode}`).toBeGreaterThanOrEqual(2.5)
    expect(item.border).toBe('1px');expect(item.contained).toBe(true)
   }
   await expect(dialog.getByRole('button',{name:'Save',exact:true})).toBeInViewport()
   await dialog.screenshot({path:info.outputPath(`odds-items-${surface}-${theme}-${mode}-${width}.png`)})
  }
 }
 await items.first().locator('input').focus()
 await expect(items.first()).not.toHaveCSS('box-shadow','none')
})

for(const surface of ['platform','admin'] as const)test(`configured item availability ${surface}`,async({page},info)=>{
 const zh=surface==='admin'
 const locale=zh?'zh-CN':'en'
 const full=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/canada_28.json','utf8')))
 const group=full.catalog.plays.filter(play=>play.group==='sides'||play.group==='extremes')
 const big=group.find(play=>full.catalog.domains[play.selection.domain]!.options[0]!.label.en==='Big')!
 const option=full.catalog.domains[big.selection.domain]!.options[0]!.id
 const game={id:gameId,name:zh?'加拿大28':'Canada 28',code:'canada_28',model:full.catalog.model,version:1,connected:true,status:'active',deletable:false,brand_enabled:true,brand_version:1,rule_published:true,sales_status:'ready'}
 const hiddenShapeItems=Object.fromEntries(full.catalog.plays.filter(play=>play.group==='shapes').map(play=>[
  play.id,Object.fromEntries(full.catalog.domains[play.selection.domain]!.options.map(option=>[option.id,false])),
 ]))
 const platform={version:2,items:{...(zh?hiddenShapeItems:{}),[big.id]:{[option]:false}}}
 const brand=zh?{version:1,items:{[big.id]:{[option]:true}}}:null
 const initialSales={version:1,rule_id:gameId,enabled:true,prices:full.prices,min_points:'1',max_points:'100',max_period_points:'10000',max_user_period_points:'5000'}
 let rules={...version,snapshot:full}, sales=initialSales
 let submitted:{rule_id:string;version:number;items:Record<string,Record<string,boolean>>}|undefined
 const writes:Array<{path:string;key:string|undefined;body:any}>=[]
 await page.route('**/api/v1/**',route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  const ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
  const state=()=>{
   const saved=submitted&&{version:submitted.version+1,items:submitted.items}
   return {rule_id:gameId,platform:!zh&&saved?saved:platform,brand:zh&&saved?saved:brand}
  }
  if(path===`/api/v1/${surface}/platform-games`)return ok({games:[game],limit:50,offset:0,total_count:'1'})
  if(request.method()==='GET'){
   if(path.endsWith('/rules'))return ok(rules)
   if(path.endsWith('/sales'))return ok(sales)
   expect(path.endsWith('/availability')).toBe(true)
   return ok(state())
  }
  const key=request.headers()['idempotency-key'], body=request.postDataJSON()
  expect(key).toBeTruthy()
  writes.push({path,key,body})
  if(writes.length===1)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Unavailable'}}})
  if(path.endsWith('/availability')){
   submitted=body
   return ok(state())
  }
  submitted={rule_id:zh?body.rule_id:body.id,...body.availability}
  if(zh){
   expect(path.endsWith('/sales')).toBe(true)
   sales={...initialSales,prices:body.prices,version:2}
   return ok(sales)
  }
  expect(path.endsWith('/rules')).toBe(true)
  rules={...rules,snapshot:body.snapshot}
  return ok(rules)
 })
 const props={surface,brandId:zh?gameId:'',locale,canManage:true,canConnect:false,canSell:true,oddsOnly:true,displaySettings:{theme:'ocean',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}}
 await page.route('**/availability-harness',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><main id="app" class="management-console panel" style="margin:12px;padding:12px;min-width:0"></main><script id="harness-data" type="application/json">${JSON.stringify({component:'PlatformLottery',props,locale})}</script><script type="module" src="/tests/layout-harness.ts"></script></html>`}))
 await page.goto(`${origins.brand}/availability-harness`)
 if(zh){
  await page.locator('.member-menu-trigger').click()
  await page.getByRole('menuitem',{name:'盘口赔率',exact:true}).click()
 }else await page.getByRole('button',{name:'Plates & odds',exact:true}).click()
 const dialog=page.getByRole('dialog')
 const unavailableOdds=dialog.locator('.configured-odds-item-disabled input')
 await expect(unavailableOdds).toHaveCount(zh?0:1)
 await expect(dialog.locator('.configured-odds-fields input')).toHaveCount(zh?9:10)
 if(zh){
  await expect(dialog.locator('.configured-odds-fields').getByText('大',{exact:true})).toHaveCount(0)
  await dialog.getByRole('combobox',{name:'玩法',exact:true}).click()
  await expect(page.getByRole('option')).toHaveText(['两面','总和','尾数'])
  await page.keyboard.press('Escape')
 }else{
  await expect(unavailableOdds).toBeDisabled()
  await expect(unavailableOdds).toHaveValue('Disabled')
 }
 await expect(dialog.locator('.console-tabs')).toHaveCount(0)
 const save=dialog.getByRole('button',{name:zh?'保存':'Save',exact:true})
 await expect(dialog.locator('.configured-actions button')).toHaveCount(1)
 await expect(save).toBeDisabled()
 const selectPlay=dialog.getByRole('combobox',{name:zh?'玩法':'Play',exact:true})
 const oddsItems=dialog.locator('.configured-odds-item')
 const small=oddsItems.filter({has:page.getByText(zh?'小':'Small',{exact:true})})
 await small.locator('input').fill('1.85')
 await small.getByRole('button',{name:zh?'停用小':'Disable Small',exact:true}).click()
 await expect(small.locator('input')).toHaveValue(zh?'已停用':'Disabled')
 await expect(small.locator('input')).toBeDisabled()
 await expect(small.getByRole('button',{name:zh?'启用小':'Enable Small',exact:true})).toHaveText(zh?'启':'On')
 await expect(save).toBeEnabled()
 await dialog.getByRole('button',{name:zh?'全部停用':'Disable all',exact:true}).click()
 await expect(unavailableOdds).toHaveCount(zh?9:10)
 await expect(dialog.getByRole('textbox',{name:zh?'批量赔率':'Batch odds',exact:true})).toBeDisabled()
 await expect(save).toBeEnabled()
 expect(submitted).toBeUndefined()

 if(!zh){
  await dialog.getByRole('button',{name:'Enable Big',exact:true}).click()
  await expect(unavailableOdds).toHaveCount(9)
  await expect(save).toBeEnabled()
 }
 await selectControl(selectPlay,{value:'group:sums'})
 await expect(oddsItems).toHaveCount(28)
 await oddsItems.first().locator('input').fill('1.75')
 if(info.project.name.endsWith('reflow')){
  for(const width of [320,360,459,460,461,620,900,1280]){
   await page.setViewportSize({width,height:568})
   await expectNoPageOverflow(page)
   expect(await dialog.evaluate(el=>el.scrollWidth<=el.clientWidth+1)).toBe(true)
  }
  await page.setViewportSize({width:320,height:568})
  await oddsItems.last().scrollIntoViewIfNeeded()
  await expect(oddsItems.last()).toBeInViewport()
  await expect(dialog.locator('.detail-modal-close')).toBeInViewport()
  await selectPlay.scrollIntoViewIfNeeded()
 }
 await selectControl(selectPlay,{value:'group:sides'})
 if(zh)await expect(oddsItems.getByText('大',{exact:true})).toHaveCount(0)
 else await expect(oddsItems.getByRole('button',{name:'Disable Big',exact:true})).toHaveText('Off')
 await expectNoPageOverflow(page)
 if(info.project.name.endsWith('reflow')){
  await dialog.screenshot({path:info.outputPath(`item-availability-${surface}-320.png`)})
  await page.setViewportSize({width:1280,height:800})
 }
 await dialog.screenshot({path:info.outputPath(`item-availability-${surface}.png`)})
 await save.click()
 const retry=dialog.getByRole('button',{name:zh?'重试':'Retry',exact:true})
 await expect(retry).toBeEnabled()
 await expect(small.locator('input')).toBeDisabled()
 await retry.click()
 await expect(dialog).toHaveCount(0)
 expect(writes).toHaveLength(2)
 expect(writes[0]).toEqual(writes[1])
 const prices=zh?sales.prices:rules.snapshot.prices
 const smallPlay=group.find(play=>full.catalog.domains[play.selection.domain]!.options[0]!.label.en==='Small')!
 const sumPlay=full.catalog.plays.find(play=>play.group==='sums')!
 const [smallKey]=configuredOptionPriceKeys(full,smallPlay,full.catalog.domains[smallPlay.selection.domain]!.options[0]!)
 const [sumKey]=configuredOptionPriceKeys(full,sumPlay,full.catalog.domains[sumPlay.selection.domain]!.options[0]!)
 expect(prices.plates.A![smallKey!]).toBe(full.prices.plates.A![smallKey!])
 expect(prices.plates.A![sumKey!]).toBe('1.75')
 await expect.poll(()=>submitted?.version).toBe(zh?1:2)
 expect(submitted!.rule_id).toBe(gameId)
 if(zh){
  expect(submitted!.items[big.id]).toEqual({[option]:true})
  expect(Object.values(submitted!.items).flatMap(Object.values).filter(value=>value===false)).toHaveLength(9)
 }
 else{
  expect(submitted!.items[big.id]?.[option]).toBeUndefined()
  expect(Object.values(submitted!.items).flatMap(Object.values)).toEqual(Array(9).fill(false))
 }
 if(zh){
  await page.locator('.member-menu-trigger').click()
  await page.getByRole('menuitem',{name:'盘口赔率',exact:true}).click()
 }else await page.getByRole('button',{name:'Plates & odds',exact:true}).click()
 await expect(dialog.locator('.configured-odds-fields input')).toHaveCount(zh?9:10)
 await expect(unavailableOdds).toHaveCount(9)
 await expect(save).toBeDisabled()
 await small.getByRole('button',{name:zh?'启用小':'Enable Small',exact:true}).click()
 await save.click()
 await expect(dialog).toHaveCount(0)
 expect(writes).toHaveLength(3)
 expect(writes[2]!.path).toMatch(/\/configured\/availability$/)
})
