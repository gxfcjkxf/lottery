import {test,expect} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {configuredDemo} from '../../shared/src/configured-rules'
import {origins,gameId,installLayoutApi,installOrdersLayoutApi,installWalletLayoutApi,expectNoPageOverflow} from './fixture'

for(const locale of ['en','zh-CN'] as const)test(`opening schedule disables betting until opening ${locale}`,async({page},info)=>{
 await page.clock.install({time:Date.now()})
 await page.clock.pauseAt(new Date(Date.now()+100))
 const start=await page.evaluate(()=>Date.now())
 await installLayoutApi(page,locale)
 await installOrdersLayoutApi(page)
 const snapshot=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json','utf8')))
 const period={id:gameId,game_id:gameId,period_no:'2026115',bet_start_at:new Date(start+60000).toISOString(),bet_end_at:new Date(start+120000).toISOString(),draw_at:new Date(start+180000).toISOString(),result:null,published_at:null}
 let closed=false
 await page.route(`**/api/v1/configured/games/${gameId}`,route=>route.fulfill({json:{success:true,data:{game:{id:gameId,name:'Mark Six',status:'active',brand_enabled:true},snapshot,rule_id:gameId,snapshot_hash:'opening-test',sales:{version:1,enabled:true},period:closed?null:period}}}))
 await page.route(`**/api/v1/platform-games/${gameId}?*`,route=>route.fulfill({json:{success:true,data:{game:{id:gameId,name:'Mark Six'},plays:[],rule_protocol:'configured',periods:[period],total_count:'1'}}}))
 await page.goto(`${origins.user}/configured/${gameId}`)
 const strip=page.locator('.configured-live .user-period-strip'),option=page.getByTestId('configured-selection').locator('.configured-options button').first()
 await expect(strip.locator('.period-times time')).toHaveCount(3)
 for(const [index,value] of [period.bet_start_at,period.bet_end_at,period.draw_at].entries())await expect(strip.locator('.period-times time').nth(index)).toHaveAttribute('datetime',value)
 await expect(strip.locator('.period-countdown')).toHaveAttribute('aria-label',locale==='en'?'Opens in':'开盘倒计时')
 await expect(page.getByText(locale==='en'?'Betting has not opened yet.':'尚未开盘，暂不可投注。',{exact:true})).toBeVisible()
 const toast=page.locator('.operation-toast-center .toast-error')
 await expect(toast).toHaveAttribute('role','alert')
 await expect(toast.locator('.toast-heading')).toHaveCount(0)
 await expect(toast.locator('.toast-message')).toHaveCSS('text-align','center')
 expect(await toast.locator('.toast-message').evaluate(el=>{
  const probe=document.createElement('span');probe.style.color='var(--danger-text)';document.body.append(probe)
  const matches=getComputedStyle(el).color===getComputedStyle(probe).color;probe.remove();return matches
 })).toBe(true)
 const preset=page.viewportSize()!
 for(const width of info.project.name.endsWith('reflow')?[320,480,900,1280]:[preset.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
  const box=await toast.boundingBox()
  expect(Math.abs(box!.x+box!.width/2-page.viewportSize()!.width/2)).toBeLessThanOrEqual(1)
  expect(Math.abs(box!.y+box!.height/2-page.viewportSize()!.height/2)).toBeLessThanOrEqual(1)
  await expectNoPageOverflow(page)
 }
 await expect(option).toBeDisabled()
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`opening-${locale}.png`),fullPage:true})
 await page.screenshot({path:info.outputPath(`opening-viewport-${locale}.png`)})
 await page.clock.fastForward(4000)
 await expect(toast).toBeVisible()
 await page.clock.fastForward(55000)
 await expect(toast).toBeVisible()
 await expect(option).toBeDisabled()
 await page.clock.fastForward(1200)
 await expect(option).toBeEnabled()
 await expect(toast).toHaveCount(0)
 await expect(strip.locator('.period-countdown')).toHaveAttribute('aria-label',locale==='en'?'Closes in':'封盘倒计时')
 closed=true
 await page.clock.fastForward(60000)
 await expect(option).toBeDisabled()
 await expect(strip.locator('.period-countdown')).toHaveAttribute('aria-label',locale==='en'?'Draws in':'开奖倒计时')
 await expectNoPageOverflow(page)
 await expect(toast).toBeVisible()
 await page.locator('a[href="/orders"]:visible').first().click()
 await expect(toast).toHaveCount(0)
})

for(const locale of ['en','zh-CN'] as const)test(`successful configured bet clears selection and retains remembered stake ${locale}`,async({page},info)=>{
 await page.clock.install({time:new Date('2026-10-27T03:00:00Z')})
 await page.clock.pauseAt(new Date('2026-10-27T03:00:01Z'))
 await installLayoutApi(page,locale);await installWalletLayoutApi(page)
 const snapshot=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json','utf8')))
 const period={id:gameId,game_id:gameId,period_no:'2026115',bet_start_at:'2026-10-24T13:00:00Z',bet_end_at:'2026-10-27T13:29:45Z',draw_at:'2026-10-27T13:30:00Z',result:null,published_at:null}
 await page.route(`**/api/v1/configured/games/${gameId}`,route=>route.fulfill({json:{success:true,data:{game:{id:gameId,name:'Mark Six',status:'active',brand_enabled:true},snapshot,rule_id:gameId,snapshot_hash:'bet-toast',sales:{version:1,enabled:true},period}}}))
 await page.route(`**/api/v1/platform-games/${gameId}?*`,route=>route.fulfill({json:{success:true,data:{game:{id:gameId,name:'Mark Six'},plays:[],rule_protocol:'configured',periods:[period],total_count:'1'}}}))
 await page.route('**/api/v1/configured/preview',route=>{
  const request=route.request().postDataJSON(),line=request.requests[0]
  return route.fulfill({json:{success:true,data:{game_id:gameId,period_id:gameId,rule_id:gameId,snapshot_hash:'bet-toast',sales_version:1,actor_context:'synthetic-bet',bet_points:'10',lines:[{play_id:line.play_id,options:line.options,plate:line.plate,stake:'10',prices:{}}]}}})
 })
 let writes=0
 await page.route('**/api/v1/configured/orders',route=>{expect(route.request().method()).toBe('POST');writes++;expect(route.request().headers()['idempotency-key']).toBeTruthy();return route.fulfill({json:{success:true,data:{}}})})
 await page.addInitScript(({id})=>localStorage.setItem(`lottery:betting-preferences:${id}:${id}`,JSON.stringify({lastPoints:'75'})),{id:gameId})
 await page.goto(`${origins.user}/configured/${gameId}`)
 await page.clock.resume()
 const selection=page.getByTestId('configured-selection')
 const points=selection.locator('.configured-points-control input')
 await expect(points).toHaveValue('')
 await points.fill('25')
 await page.reload()
 await expect(points).toHaveValue('25')
 const mobile=page.viewportSize()!.width<1000
 await expect(page.locator('.period-times time').first().locator('span:visible')).toHaveText(mobile?'10-24 21:00':'2026-10-24 21:00:00')
 await expect(page.locator('.period-times time').nth(1).locator('span:visible')).toHaveText(mobile?'10-27 21:29':'2026-10-27 21:29:45')
 const summary=selection.locator('.configured-selected'),dock=selection.locator('.configured-stakes--dock')
 if(mobile)await expect(summary).toBeHidden()
 const dockHeight=(await dock.boundingBox())!.height
 await selection.locator('.configured-options button').first().click()
 await expect(points).toBeFocused()
 await expect(summary).toBeVisible()
 if(mobile){
  const shownDock=(await dock.boundingBox())!,shownSummary=(await summary.boundingBox())!
  expect(shownDock.height).toBe(dockHeight)
  expect(Math.abs(shownSummary.y+shownSummary.height-shownDock.y)).toBeLessThanOrEqual(1)
 }
 if(info.project.name.endsWith('reflow')){
  for(const width of [320,620,999,1000,1280]){
   await page.setViewportSize({width,height:800})
   await expect(summary).toBeVisible()
   await expectNoPageOverflow(page)
   await expect(page.locator('.period-times time').first().locator('span:visible')).toHaveText(width<1000?'10-24 21:00':'2026-10-24 21:00:00')
   if(width===320)await page.screenshot({path:info.outputPath(`bet-selected-320-${locale}.png`)})
  }
 }
 await page.screenshot({path:info.outputPath(`bet-selected-${locale}.png`)})
 await selection.locator('.configured-clear').click()
 if(mobile)await expect(summary).toBeHidden()
 await selection.locator('.configured-options button').first().click()
 await selection.locator('.configured-stakes input[type=number]').fill('10')
 await selection.getByRole('button',{name:locale==='en'?'Bet':'投注',exact:true}).click()
 const dialog=page.getByRole('dialog')
 await dialog.getByRole('button',{name:locale==='en'?'Confirm bet':'确认投注',exact:true}).click()
 await expect(dialog).toHaveCount(0);expect(writes).toBe(1)
 await expect(selection.locator('.configured-options [aria-pressed=true]')).toHaveCount(0)
 await expect(points).toHaveValue('10')
 await selection.locator('.configured-options button').first().click()
 await expect(points).toBeFocused()
 await expect(points).toHaveValue('10')
 await selection.locator('.configured-clear').click()
 const toast=page.locator('.operation-toast-stack .toast-success')
 await expect(toast).toContainText(locale==='en'?'Bet placed':'投注成功')
 await expect(page.locator('.configured-bet-notice')).toHaveCount(0)
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`bet-success-${locale}.png`)})
})

for(const locale of ['en','zh-CN'] as const)test(`betting unavailability identifies the actual reason ${locale}`,async({page},info)=>{
 await installLayoutApi(page,locale)
 await installWalletLayoutApi(page)
 const snapshot=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json','utf8')))
 const now=Date.now()
 const period={id:gameId,period_no:'2026115',bet_start_at:new Date(now-60000).toISOString(),bet_end_at:new Date(now+3600000).toISOString(),draw_at:new Date(now+3660000).toISOString()}
 const catalog={game:{id:gameId,name:'Mark Six',status:'active',brand_enabled:true},snapshot,rule_id:gameId,snapshot_hash:'reason-test',sales:{version:1,enabled:true},period:period as typeof period|null,previous_draw:null}
 let periods:typeof period[]=[]
 await page.route(`**/api/v1/configured/games/${gameId}`,route=>route.fulfill({json:{success:true,data:catalog}}))
 await page.route(`**/api/v1/platform-games/${gameId}?*`,route=>route.fulfill({json:{success:true,data:{game:catalog.game,plays:[],rule_protocol:'configured',periods,total_count:String(periods.length)}}}))
 const cases=[
  {zh:'彩种已停用。',en:'This game is disabled.',setup:()=>{catalog.game.status='disabled'}},
  {zh:'当前品牌未启用此彩种。',en:'This game is not enabled for this brand.',setup:()=>{catalog.game.status='active';catalog.game.brand_enabled=false}},
  {zh:'投注已暂停。',en:'Betting is paused.',setup:()=>{catalog.game.brand_enabled=true;catalog.sales.enabled=false}},
  {zh:'暂无可投注期次。',en:'No period is available for betting.',setup:()=>{catalog.sales.enabled=true;catalog.period=null}},
  {zh:'本期已封盘。',en:'This period is closed.',setup:()=>{periods=[{...period,bet_end_at:new Date(now-1000).toISOString()}]}},
 ]
 const toast=page.locator('.operation-toast-center .toast-error')
 for(const item of cases){
  item.setup()
  await page.goto(`${origins.user}/configured/${gameId}`)
  await expect(toast).toHaveText(locale==='en'?item.en:item.zh)
  await expect(page.getByTestId('configured-selection').locator('.configured-options button').first()).toBeDisabled()
  await expectNoPageOverflow(page)
 }
 catalog.period={...period,bet_end_at:new Date(now-1000).toISOString()}
 await page.reload()
 await expect(toast).toHaveText(locale==='en'?'This period is closed.':'本期已封盘。')
 catalog.period=period;periods=[]
 await page.reload()
 await expect(toast).toHaveCount(0)
 await expect(page.getByTestId('configured-selection').locator('.configured-options button').first()).toBeEnabled()
 catalog.game.brand_enabled=false
 await page.reload()
 await expect(toast).toHaveText(locale==='en'?'This game is not enabled for this brand.':'当前品牌未启用此彩种。')
 for(const width of info.project.name.endsWith('reflow')?[320,480,900,1280]:[page.viewportSize()!.width]){
  if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:800})
  await expectNoPageOverflow(page)
 }
 await page.screenshot({path:info.outputPath(`betting-reason-${locale}.png`)})
})
