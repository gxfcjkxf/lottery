import {readFileSync} from 'node:fs'
import {configuredDemo} from '../../shared/src/configured-rules'
import {expect,test} from '@playwright/test'
import {installLayoutApi,installOrdersLayoutApi,origins,gameId,expectNoPageOverflow,expectInsideViewport,expectCompactPageLayout} from './fixture'
export function registerWorkspaceTests(){
 for(const locale of ['en','zh-CN'] as const)test(`user login opens home and navigation follows the viewport (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installOrdersLayoutApi(page)
  let authenticated=false
  const profile={user:{id:gameId,username:'layout-member',status:'active'},member:{id:gameId,brand_id:gameId,status:'normal',display_name:'Layout member',joined_at:'2026-10-17T00:00:00Z'}}
  await page.route('**/api/v1/me',route=>authenticated
   ? route.fulfill({json:{success:true,data:profile}})
   : route.fulfill({status:401,json:{success:false,error:{code:'UNAUTHORIZED',message:'Sign in required'}}}))
  let logins=0
  await page.route('**/api/v1/auth/login',route=>{
   expect(route.request().method()).toBe('POST')
   expect(route.request().postDataJSON()).toMatchObject({identifier:'layout-member',password:'layout-only-password'})
   authenticated=true;logins++
   return route.fulfill({json:{success:true,data:{...profile,access_token:'synthetic-layout-token',token_type:'Bearer',expires_at:'2099-01-01T00:00:00Z'}}})
  })
  await page.goto(`${origins.user}/login`)
  await page.locator('input[autocomplete=username]').fill('layout-member')
  await page.locator('input[autocomplete=current-password]').fill('layout-only-password')
  await page.locator('form button[type=submit]').click()
  await expect(page).toHaveURL(`${origins.user}/`)
  expect(logins).toBe(1)
  await expect(page.locator('.configured-options button')).toHaveCount(49)
  const initialViewport=page.viewportSize()!
  const widths=info.project.name.endsWith('reflow')?[320,620,820,999,1000,1001,1024,1280,1440]:[initialViewport.width]
  for(const width of widths){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:720})
   const desktop=width>=1000
   const sidebar=page.locator('.workspace-sidebar'),bottom=page.locator('.mobile-primary-nav')
   await expect(sidebar).toBeVisible({visible:desktop})
   await expect(bottom).toBeVisible({visible:!desktop})
   const navigation=desktop?sidebar.locator('nav'):bottom
   await expect(navigation.locator('a[aria-current=page]')).toHaveAttribute('href','/')
   for(const link of await navigation.locator('a').all())await expectInsideViewport(link)
   if(desktop){
    const side=(await sidebar.boundingBox())!,content=(await page.locator('main.content').boundingBox())!
    expect(content.x).toBeGreaterThanOrEqual(side.x+side.width)
   }
   await expectNoPageOverflow(page)
   if(width===initialViewport.width||[320,999,1000,1440].includes(width))await page.screenshot({path:info.outputPath(`user-home-${locale}-${width}.png`)})
   await navigation.locator('a[href="/account"]').click()
   await expect(page).toHaveURL(`${origins.user}/account`)
   await expect(navigation.locator('a[aria-current=page]')).toHaveAttribute('href','/account')
   await expect(page.locator('.account-page')).toBeVisible()
   await expectNoPageOverflow(page)
   await navigation.locator('a[href="/"]').last().click()
   await expect(page).toHaveURL(`${origins.user}/`)
  }
  await page.reload()
  await expect(page).toHaveURL(`${origins.user}/`)
  await expect(page.locator('.configured-options button')).toHaveCount(49)
 })
 for(const locale of ['en','zh-CN'] as const)test(`guest protected routes open the sign-in page without an account expander (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale)
  for(const path of ['/','/help']){
   await page.goto(origins.user+path)
   await expect(page).toHaveURL(/\/login$/)
   const form=page.locator('form')
   await expect(form).toBeVisible()
   await expect(form.locator('input[autocomplete=username]')).toBeVisible()
   await expect(form.locator('input[type=password]')).toBeVisible()
   await expect(page.locator('.workspace-overview,.workspace-overview-control,.workspace-login')).toHaveCount(0)
   const submit=form.locator('button[type=submit]')
   await submit.scrollIntoViewIfNeeded()
   await expectInsideViewport(submit);await expectNoPageOverflow(page)
  }
  await page.screenshot({path:info.outputPath(`guest-sign-in-${locale}.png`)})

 })
}

// Exercise the shared sale UI with explicit synthetic API receipts. Backend
// tests separately verify real ledger and settlement behavior.
export function registerConfiguredSaleTests(){
 for(const locale of ['en','zh-CN'] as const)test(`configured sales restore an uncertain purchase and retry the same request (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installOrdersLayoutApi(page)
  const writes:Array<{key:string|null;body:unknown}>=[]
  await page.route('**/api/v1/configured/preview',async route=>{
   const input=route.request().postDataJSON(),request=input.requests[0]
   await route.fulfill({json:{success:true,data:{...input,game_id:gameId,definition_hash:'synthetic-definition',actor_context:'synthetic-context',bet_points:request.points,lines:[{play_id:request.play_id,options:request.options,plate:request.plate,stake:request.points,prices:{selected:'2'}}]}}})
  })
  await page.route('**/api/v1/configured/orders',async route=>{
   if(route.request().method()==='GET')return route.fallback()
   writes.push({key:route.request().headers()['idempotency-key']??null,body:route.request().postDataJSON()})
   if(writes.length===1)return route.abort('failed')
   return route.fulfill({status:201,json:{success:true,data:{id:'synthetic-order'}}})
  })
  await page.goto(`${origins.user}/configured/${gameId}`)
  await expect(page.locator('.configured-options button')).toHaveCount(49)
  await page.locator('.configured-options button').first().click()
  await page.getByRole('button',{name:locale==='en'?'Review bet':'核对投注',exact:true}).click()
  const confirm=page.getByRole('button',{name:locale==='en'?'Confirm and place bet':'确认并投注',exact:true})
  await expect(confirm).toBeVisible();await confirm.click()
  const retry=page.getByRole('button',{name:locale==='en'?'Retry original request':'重试原请求',exact:true})
  await expect(retry).toBeVisible();await page.reload();await expect(retry).toBeVisible()
  await expect(page.locator('.configured-selection select').first()).toBeDisabled()
  await expectCompactPageLayout(page)
  await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`configured-retry-${locale}.png`),fullPage:true})
  await retry.click();await expect(retry).toHaveCount(0)
  expect(writes).toHaveLength(2);expect(writes[0]!.key).toBeTruthy();expect(writes[1]).toEqual(writes[0])
  await expect(page.locator('.configured-selection select').first()).toBeEnabled()
 })
}

export function registerConfiguredHistoryLayoutTests(){
 for(const locale of ['en','zh-CN'] as const)test(`configured betting and historical game filters retain the viewport (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installOrdersLayoutApi(page)
  await page.goto(`${origins.user}/configured/${gameId}`)
  await expect(page.locator('.configured-options button')).toHaveCount(49)
  await expect(page.locator('.workspace-menu a[href^="/results"]')).toHaveAttribute('href',`/results?platform_game_id=${gameId}`)
  await expectCompactPageLayout(page)
  await expectNoPageOverflow(page)
  await page.locator('.play-rules-entry button').click()
  const rules = page.getByRole('dialog')
  await expect(rules).toContainText(locale === 'en' ? 'How to choose' : '怎么选')
  await expect(rules).toContainText(locale === 'en' ? 'special numbers' : '特别号码')
  await expect(rules).not.toContainText('schema_version')
  await expect(rules).not.toContainText('price_key')
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`play-rules-${locale}.png`),fullPage:true})
  if (info.project.name.endsWith('reflow')) {
   const original=page.viewportSize()!
   for (const viewport of [{width:320,height:568},{width:640,height:320}]) {
    await page.setViewportSize(viewport)
    await expectNoPageOverflow(page)
    await rules.locator('.detail-modal-content').evaluate(el=>{el.scrollTop=el.scrollHeight})
    await expectInsideViewport(rules.locator('.detail-modal-close'))
   }
   await page.setViewportSize(original)
  }
  await rules.locator('.detail-modal-close').click()
  await page.locator('.workspace-topbar > button.icon-button').click()
  await page.locator('.workspace-menu a[href^="/results"]').click()
  await expect(page.locator('[data-testid="draw-archive"]')).toBeVisible()
  await expect(page.locator('[data-testid="platform-shared-games"]')).toHaveCount(0)
  await expect(page.locator('.draw-archive .empty-state')).toBeVisible()
  await page.route('**/api/v1/draw-archive?*', route => {
   expect(route.request().method()).toBe('GET')
   const selected = new URL(route.request().url()).searchParams.get('selected') || '2026'
   return route.fulfill({json:{success:true,data:{game_id:gameId,game_name:'Mark Six',mark_six:true,timezone:'Asia/Singapore',daily:false,choices:['2026','2025'],selected,has_more:false,items:[{period_no:selected+'10',drawn_at:'2026-10-23T01:01:00Z',result:{regular:[3,12,28,14,25,48],special:[7],digits:[]},attributes:Object.fromEntries([3,12,28,14,25,48,7].map(n=>[String(n).padStart(2,'0'),{wave:'red',zodiac:'dragon',element:'wood'}]))}]}}})
  })
  await page.reload()
  await expect(page.locator('.draw-archive .archive-period')).toContainText('202610')
  await expect(page.locator('.archive-numbers .archive-ball')).toHaveText(['03','12','28','14','25','48','07'])
  await expect(page.locator('.archive-zodiac')).toHaveCount(7)
  await expect(page.locator('.archive-plus')).toHaveText('+')
  await page.getByRole('button',{name:locale==='en'?'Special size':'特大小',exact:true}).click()
  await expect(page.locator('.archive-facts')).toContainText(locale==='en'?'Odd':'单')
  await page.getByRole('button',{name:locale==='en'?'Total':'总和',exact:true}).click()
  await expect(page.locator('.archive-facts')).toContainText('137')
  await page.getByRole('button',{name:locale==='en'?'Numbers':'号码',exact:true}).click()
  await expect(page.locator('.archive-filter select')).toHaveValue('2026')
  await page.locator('.archive-filter select').selectOption('2025')
  await expect(page.locator('.draw-archive .archive-period')).toContainText('202510')
  await expect(page.locator('.draw-archive input,.draw-archive [role="tablist"]')).toHaveCount(0)
  await expect(page.locator('.draw-archive select')).toHaveCount(1)
  if (info.project.name.endsWith('reflow')) {
   const original = page.viewportSize()!
   for (const width of [320, 640, 999, 1000, 1001]) {
    await page.setViewportSize({width,height:720})
    await expectNoPageOverflow(page)
    await expect(page.locator('.draw-archive .archive-period')).toBeVisible()
   }
   await page.setViewportSize(original)
  }
  await page.screenshot({path:info.outputPath(`configured-results-${locale}.png`),fullPage:true})
  await expect(page.locator('.draw-archive')).not.toContainText(locale === 'en' ? 'Number model' : '号码模型')
  await expectNoPageOverflow(page)
  await page.goto(`${origins.user}/results`)
  await expect(page.locator('.draw-results')).toBeVisible()
  await expect(page.locator('[data-testid="platform-shared-games"]')).toHaveCount(0)
  await page.route('**/api/v1/draw-archive?*', route => {
   expect(route.request().method()).toBe('GET')
   const selected=new URL(route.request().url()).searchParams.get('selected') || '2026-10-23'
   return route.fulfill({json:{success:true,data:{game_id:gameId,game_name:'Quick draw',mark_six:false,timezone:'Asia/Singapore',daily:true,choices:['2026-10-23','2026-10-22'],selected,has_more:false,items:[{period_no:selected.replaceAll('-','')+'001',drawn_at:selected+'T01:00:00Z',result:{regular:[],special:[],digits:[0,1,0]},attributes:{}}]}}})
  })
  await page.reload()
  await expect(page.locator('.archive-filter select')).toHaveValue('2026-10-23')
  await page.locator('.archive-filter select').selectOption('2026-10-22')
  await expect(page.locator('.draw-archive .archive-period')).toContainText('20261022001')
  await expect(page.locator('.archive-numbers .archive-ball')).toHaveText(['0','1','0'])
  await expectNoPageOverflow(page)
  await page.goto(`${origins.user}/orders`)
  const games=page.locator('.history-filters select');await expect(games.locator('option')).toHaveCount(4)
  for(let index=0;index<4;index++){
   await games.selectOption({index})
   await expect(games).toBeEnabled()
   await expect(page.locator('.order-history-empty')).toBeVisible();await expectNoPageOverflow(page)
  }
  const snapshot=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/canada_niuniu.json','utf8')))
  await page.route('**/api/v1/betting-history?*',route=>{
   expect(route.request().method()).toBe('GET')
   const selected=new URL(route.request().url()).searchParams.get('game_id')??''
   const order={id:'historical-risk',game_id:gameId,game_name:'Niu risk history',period_no:'history-1',status:'settled',snapshot,quote:{bet_points:'10',reserved_points:'200',lines:[{play_id:'aa',options:['aa'],plate:'A',stake:'10',prices:{aa:'2'}}]},result:{prize_points:'0',refund_points:'10',consumed_points:'190',lines:[{outcome:'lost',prize_points:'0'}]}}
   return route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,game_id:selected,games:[{id:gameId,name:'Niu risk history'}],items:[{id:order.id,kind:'configured',game_id:gameId,game_name:order.game_name,period_no:order.period_no,placed_at:'2026-10-21T00:00:00Z',order}],limit:20,next_cursor:null}}})
  })
  await page.reload()
  const ticket=page.locator('.configured-order-card');await expect(ticket).toHaveCount(1)
  await expect(ticket).toContainText(locale==='en'?'Reserved risk points: 200':'风险冻结积分: 200')
  await expect(ticket).toContainText(locale==='en'?'Actual loss points: 190':'实际亏损积分: 190')
  await expect(ticket).toContainText(locale==='en'?'Released risk points: 10':'风险解冻积分: 10')
  await expectCompactPageLayout(page)
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`configured-history-${locale}.png`),fullPage:true})
 })
}
