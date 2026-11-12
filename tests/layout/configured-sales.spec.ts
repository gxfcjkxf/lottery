import {test,expect} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {configuredDemo} from '../../shared/src/configured-rules'
import {selectControl} from '../select-control'
import {gameId,origins,installLayoutApi,expectNoPageOverflow} from './fixture'
const snapshot=JSON.parse(readFileSync('backend/internal/rules/config/examples/lucky_group_preview.json','utf8'))
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
  saved={...input,version:input.version+1}
  if(mode==='first'&&writes.length===1)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Unknown result'}}})
  return ok(saved)
 })
 const props={surface:'admin',brandId:gameId,gameId,locale,snapshot:null,canWrite:mode!=='read-only'}
 await page.route('**/configured-sales-harness',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><main id="app" class="management-console panel" style="margin:12px;padding:12px;min-width:0"></main><script id="harness-data" type="application/json">${JSON.stringify({component:'ConfiguredPublishing',props,locale})}</script><script type="module" src="/tests/layout-harness.ts"></script></body></html>`}))
 await page.goto(`${origins.brand}/configured-sales-harness`)
 await expect(page.getByRole('heading',{name:zh?'投注配置':'Betting configuration'})).toBeVisible()
 if(mode==='read-only'){
  await expect(page.getByRole('textbox')).toHaveCount(0)
  await expect(page.getByRole('button')).toHaveCount(0)
 }else{
  await expect(page.getByText(zh?'停用投注':'Disable betting',{exact:true})).toBeVisible()
  await expect(page.getByText(zh?'启用投注':'Enable betting',{exact:true})).toBeVisible()
  const save=page.getByRole('button',{name:zh?'保存投注配置':'Save betting configuration',exact:true})
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
  await save.click()
  if(mode==='first'){
   await expect(page.getByRole('button',{name:zh?'重试原请求':'Retry original request'})).toBeVisible()
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
 const zh=locale==='zh-CN',paths:string[]=[]
 const game={id:gameId,name:'Current game',code:'current_game',model:snapshot.catalog.model,timezone:'UTC',version:1,connected:true,status:'active',deletable:false,brand_enabled:true,brand_version:1,rule_published:true,sales_status:'unconfigured'}
 await page.route('**/api/v1/**',async route=>{
  expect(route.request().method()).toBe('GET')
  const path=new URL(route.request().url()).pathname;paths.push(path)
  if(path==='/api/v1/admin/platform-games')return route.fulfill({json:{success:true,data:{games:[game],limit:50,offset:0,total_count:'1'}}})
  if(path.endsWith('/configured/rules'))return route.fulfill({json:{success:true,data:version}})
  expect(path).toBe(`/api/v1/admin/platform-games/${gameId}/configured/sales`)
  return route.fulfill({status:404,json:{success:false,error:{code:'RESOURCE_NOT_FOUND',message:'No sales'}}})
 })
 const props={surface:'admin',brandId:gameId,locale,canManage:false,canConnect:false,canSell:true,oddsOnly}
 await page.route('**/sales-entry-harness',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><main id="app" class="management-console" style="padding:12px;min-width:0"></main><script id="harness-data" type="application/json">${JSON.stringify({component:'PlatformLottery',props,locale})}</script><script type="module" src="/tests/layout-harness.ts"></script></body></html>`}))
 await page.goto(`${origins.brand}/sales-entry-harness`)
 await expect(page.getByText(zh?'未配置投注':'Betting not configured',{exact:true})).toBeVisible()
 await page.locator('.member-menu-trigger').click()
 await page.getByRole('menuitem',{name:oddsOnly?(zh?'盘口赔率':'Plates & odds'):(zh?'投注配置':'Betting configuration'),exact:true}).click()
 const dialog=page.getByRole('dialog')
 await expect(dialog.getByRole('button',{name:oddsOnly?(zh?'保存赔率':'Save odds'):(zh?'保存投注配置':'Save betting configuration'),exact:true})).toBeEnabled()
 await expect(dialog.getByRole('switch')).toHaveCount(oddsOnly?0:1)
 await expect(dialog.locator('.configured-odds-editor')).toHaveCount(oddsOnly?1:0)
 expect(paths).toEqual(['/api/v1/admin/platform-games',`/api/v1/admin/platform-games/${gameId}/configured/rules`,`/api/v1/admin/platform-games/${gameId}/configured/sales`])
 if(info.project.name.endsWith('reflow'))for(const width of [320,480,768,1280]){await page.setViewportSize({width,height:800});await expectNoPageOverflow(page)}
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`sales-entry-${locale}.png`),fullPage:true})
 await dialog.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click()
 await expect(dialog).toHaveCount(0)
})

for(const locale of ['en','zh-CN'] as const)test(`mark six odds use player names and preserve plate edits ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN'
 const full=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json','utf8')))
 let submitted:any
 await page.route('**/api/v1/**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname
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
 const row=odds.locator('.configured-odds-fields label').first()
 const labelBox=await row.locator('span').boundingBox(),inputBox=await row.locator('input').boundingBox()
 expect(Math.abs((labelBox!.y+labelBox!.height/2)-(inputBox!.y+inputBox!.height/2))).toBeLessThan(2)
 expect(inputBox!.width).toBeLessThanOrEqual(89)
 if(info.project.name.endsWith('reflow'))for(const width of [320,619,620,621,1280]){
  await page.setViewportSize({width,height:800});await expectNoPageOverflow(page)
  if(width===320)await odds.screenshot({path:info.outputPath(`mark-six-numbers-320-${locale}.png`)})
 }
 await odds.screenshot({path:info.outputPath(`mark-six-numbers-${locale}.png`)})
 await selectControl(odds.getByRole('combobox',{name:zh?'玩法':'Play',exact:true}),{value:'special_sides'})
 await odds.getByLabel(zh?'大':'big',{exact:true}).fill('1.98')
 await selectControl(odds.getByRole('combobox',{name:zh?'盘口':'Plate',exact:true}),{value:'B'})
 await expect(odds.getByLabel(zh?'大':'big',{exact:true})).toHaveValue('2')
 await odds.getByLabel(zh?'大':'big',{exact:true}).fill('1.88')
 await selectControl(odds.getByRole('combobox',{name:zh?'盘口':'Plate',exact:true}),{value:'A'})
 await expect(odds.getByLabel(zh?'大':'big',{exact:true})).toHaveValue('1.98')
 if(info.project.name.endsWith('reflow'))for(const width of [320,619,620,621,1280]){await page.setViewportSize({width,height:800});await expectNoPageOverflow(page)}
 await expectNoPageOverflow(page)
 await odds.screenshot({path:info.outputPath(`mark-six-odds-${locale}.png`)})
 await page.getByRole('button',{name:zh?'保存赔率':'Save odds',exact:true}).click()
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
  if(path.endsWith('/configured/sales'))return reply({version:1,rule_id:gameId,enabled:true,prices:snapshot.prices,min_points:'1',max_points:'100',max_period_points:'10000',max_user_period_points:'5000'})
  throw Error(`Unexpected odds menu request: ${path}`)
 })
 await page.goto(origins.brand)
 await expect(page.locator('.management-console')).toBeVisible()
 const more=page.locator('.mobile-nav > button').last()
 if(await more.isVisible())await more.click()
 const nav=page.locator(await more.isVisible()?'.mobile-more-menu':'.side-nav')
 await nav.getByRole('button',{name:zh?'盘口赔率':'Plates & odds',exact:true}).click()
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
 await expect(dialog.getByRole('button',{name:zh?'保存赔率':'Save odds',exact:true})).toHaveCount(0)
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`odds-menu-readonly-${locale}.png`),fullPage:true})
 await dialog.locator('.detail-modal-close').click()
 if(await more.isVisible())await more.click()
 await nav.getByRole('button',{name:zh?'彩种管理':'Game management',exact:true}).click()
 await expect(page.locator('.management-list tbody tr')).toHaveCount(2)
})
