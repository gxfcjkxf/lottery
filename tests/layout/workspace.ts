import { selectControl } from '../select-control'
import {readFileSync} from 'node:fs'
import {configuredDemo} from '../../shared/src/configured-rules'
import {expect,test} from '@playwright/test'
import {installLayoutApi,installOrdersLayoutApi,installWalletLayoutApi,origins,gameId,expectNoPageOverflow,expectInsideViewport,expectCompactPageLayout} from './fixture'
export function registerWorkspaceTests(){
 for(const locale of ['en','zh-CN'] as const)test(`home selection fills content and play tabs have complete borders (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installWalletLayoutApi(page)
  await page.goto(`${origins.user}/`)
  await expect(page.locator('.configured-options button')).toHaveCount(49)
  const preset=page.viewportSize()!
  for(const width of info.project.name.endsWith('reflow')?[320,620,999,1000,1001,1440,1920]:[preset.width]){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:800})
   const layout=await page.locator('.configured-selection').evaluate(selection=>{
    const content=document.querySelector('main.content')!,style=getComputedStyle(content),bounds=content.getBoundingClientRect()
    const tabs=getComputedStyle(selection.querySelector('.configured-play-tabs')!)
    return {left:selection.getBoundingClientRect().left,right:selection.getBoundingClientRect().right,leftEdge:bounds.left+parseFloat(style.paddingLeft),rightEdge:bounds.right-parseFloat(style.paddingRight),borders:[tabs.borderLeftWidth,tabs.borderTopWidth,tabs.borderRightWidth,tabs.borderBottomWidth]}
   })
   expect(layout.left).toBeCloseTo(layout.leftEdge,0)
   expect(layout.right).toBeCloseTo(layout.rightEdge,0)
   expect(layout.borders).toEqual(['1px','1px','1px','1px'])

   await expectNoPageOverflow(page)
   const tabs=page.locator('.configured-play-tabs button')
   await tabs.nth(1).click();await expect(tabs.nth(1)).toHaveAttribute('aria-selected','true')
   await tabs.first().click();await expect(tabs.first()).toHaveAttribute('aria-selected','true')
   await page.screenshot({path:info.outputPath(`home-width-${locale}-${width}.png`)})
  }
 })

 for(const locale of ['en','zh-CN'] as const)test(`user login opens home and navigation follows the viewport (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installWalletLayoutApi(page)
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
  await expect(page.locator('.workspace-overview-control')).toBeVisible()
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
   const topbar=await page.locator('.workspace-game-heading').boundingBox()
   const game=await page.locator('.workspace-game-picker .select-control-trigger').boundingBox()
   expect(Math.abs(game!.x+game!.width/2-topbar!.x-topbar!.width/2)).toBeLessThan(1)
   await expectNoPageOverflow(page)
   if(width===initialViewport.width||[320,999,1000,1440].includes(width))await page.screenshot({path:info.outputPath(`user-home-${locale}-${width}.png`)})
   await navigation.locator('a[href="/account"]').click()
   await expect(page).toHaveURL(`${origins.user}/account`)
   await expect(navigation.locator('a[aria-current=page]')).toHaveAttribute('href','/account')
   await expect(page.locator('.account-page')).toBeVisible()
   await expect(page.locator('.account-points strong')).toHaveText('120')
   await page.reload()
   await expect(page.locator('.account-points strong')).toHaveText('120')
   if(width===initialViewport.width||[320,1440].includes(width))await page.screenshot({path:info.outputPath(`account-points-${locale}-${width}.png`)})
   await expectNoPageOverflow(page)
   await navigation.locator('a[href="/"]').last().click()
   await expect(page).toHaveURL(`${origins.user}/`)
  }
  await page.reload()
  await expect(page).toHaveURL(`${origins.user}/`)
  await expect(page.locator('.configured-options button')).toHaveCount(49)
  await expect(page.locator('.workspace-overview-control')).toBeVisible()
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
 for(const locale of ['en','zh-CN'] as const)test(`help and personal pages share content width and visible titles (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installOrdersLayoutApi(page)
  const preset=page.viewportSize()!
  const widths=info.project.name.includes('reflow')?[320,360,619,620,621,719,720,721,999,1000,1001,1440]:[preset.width]
  for(const path of ['/account','/help']){
   await page.goto(origins.user+path)
   const section=page.locator(path==='/help'?'.help-page':'.account-page')
   const heading=page.locator('.workspace-page-title')
   await expect(heading).toHaveText(path==='/help'?(locale==='en'?'Help centre':'帮助中心'):(locale==='en'?'Me':'个人'))
   for(const width of widths){
    if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:800})
    await expect(heading).toBeVisible()
    await expectNoPageOverflow(page)
    const bounds=await section.evaluate(element=>{
     const content=element.closest('main')!,style=getComputedStyle(content),outer=content.getBoundingClientRect(),inner=element.getBoundingClientRect()
     return {left:inner.left,right:inner.right,contentLeft:outer.left+parseFloat(style.paddingLeft),contentRight:outer.right-parseFloat(style.paddingRight)}
    })
    expect(Math.abs(bounds.left-bounds.contentLeft)).toBeLessThanOrEqual(1)
    expect(Math.abs(bounds.right-bounds.contentRight)).toBeLessThanOrEqual(1)
    if(path==='/account')for(const card of await section.locator(':scope > .detail-panel').all()){
     const box=(await card.boundingBox())!
     expect(Math.abs(box.x-bounds.left)).toBeLessThanOrEqual(1)
     expect(Math.abs(box.x+box.width-bounds.right)).toBeLessThanOrEqual(1)
    }
    if(width===preset.width||[320,1440].includes(width))await page.screenshot({path:info.outputPath(`${path.slice(1)}-unified-${locale}-${width}.png`),fullPage:true})
   }
   if(path==='/help'){
    const item=section.locator('details').nth(1)
    await item.locator('summary').click();await expect(item.locator('p')).toBeVisible()
    await item.locator('summary').press('Enter');await expect(item.locator('p')).toBeHidden()
   }
  }
 })
}

// Exercise the shared sale UI with explicit synthetic API receipts. Backend
// tests separately verify real ledger and settlement behavior.
export function registerConfiguredSaleTests(){
 for(const locale of ['en','zh-CN'] as const)test(`configured sales restore an uncertain purchase and retry the same request (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installWalletLayoutApi(page)
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
  await expect(page.locator('.workspace-overview-control')).toBeVisible()
  await page.locator('.configured-options button').first().click()
  await page.getByRole('spinbutton',{name:locale==='en'?'Points per entry':'每注积分',exact:true}).fill('10')
  await page.getByRole('button',{name:locale==='en'?'Bet':'投注',exact:true}).click()
  const confirm=page.getByRole('button',{name:locale==='en'?'Confirm bet':'确认投注',exact:true})
  await expect(confirm).toBeVisible();await confirm.click()
  const retry=page.locator('.betting-sheet').getByRole('button',{name:locale==='en'?'Retry original request':'重试原请求',exact:true})
  await expect(retry).toBeVisible();await page.reload();await expect(retry).toBeVisible()
  await expect(page.locator(".configured-selection .select-control-trigger").first()).toBeDisabled()
  await expectCompactPageLayout(page)
  await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`configured-retry-${locale}.png`),fullPage:true})
  await retry.click();await expect(retry).toHaveCount(0)
  expect(writes).toHaveLength(2);expect(writes[0]!.key).toBeTruthy();expect(writes[1]).toEqual(writes[0])
  await expect(page.locator(".configured-selection .select-control-trigger").first()).toBeEnabled()
 })
}

export function registerConfiguredHistoryLayoutTests(){
 for(const locale of ['en','zh-CN'] as const)test(`configured betting and historical game filters retain the viewport (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installOrdersLayoutApi(page)
  await page.goto(`${origins.user}/configured/${gameId}`)
  await expect(page.locator('.configured-options button')).toHaveCount(49)
  await expect(page.locator('.workspace-overview-control')).toBeVisible()
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
  await page.getByRole('button',{name:locale==='en'?'Menu':'菜单',exact:true}).click()
  await page.locator('.workspace-menu a[href^="/results"]').click()
  await expect(page.locator('[data-testid="draw-archive"]')).toBeVisible()
  await expect(page.locator('.workspace-overview,.workspace-overview-control')).toHaveCount(0)
  await expect(page.locator('[data-testid="platform-shared-games"]')).toHaveCount(0)
  await expect(page.locator('.draw-archive .empty-state')).toBeVisible()
  await page.route('**/api/v1/draw-archive?*', route => {
   expect(route.request().method()).toBe('GET')
   const selected = new URL(route.request().url()).searchParams.get('selected') || '2026'
   return route.fulfill({json:{success:true,data:{game_id:gameId,game_name:'Mark Six',mark_six:true,timezone:'Asia/Singapore',daily:false,choices:['2026','2025'],selected,has_more:false,total_count:'1',items:[{period_no:selected+'10',drawn_at:selected+'-10-23T01:01:00Z',draw_date:null,regular_order:[3,12,28,14,25,48],result:{regular:[3,12,28,14,25,48],special:[7],digits:[]},attributes:Object.fromEntries([3,12,28,14,25,48,7].map(n=>[String(n).padStart(2,'0'),{wave:n%3===0?'red':n%3===1?'blue':'green',zodiac:'dragon',element:'wood'}]))}]}}})
  })
  await page.reload()
  await expect(page.locator('.draw-archive .archive-period')).toHaveText(locale==='en'?'10':'10期')
  await expect(page.locator('.archive-numbers .archive-ball')).toHaveText(['03','12','28','14','25','48','07'])
  await expect(page.locator('.archive-number-attribute')).toHaveCount(7)
  await expect(page.locator('.archive-plus')).toHaveText('+')
  const assertCompactResultRow = async () => {
   const filterHeight=await page.locator(".archive-filter .select-control-trigger").evaluate(select=>select.getBoundingClientRect().height)
   expect(filterHeight).toBe(await page.evaluate(()=>matchMedia('(min-width:901px) and (pointer:fine)').matches?32:44))
   await expect(page.locator('.archive-summary-options input[type=checkbox]')).toHaveCount(6)
   await expect(page.locator('.archive-sorting input[type=radio]')).toHaveCount(2)
   const geometry=await page.locator('.archive-row').first().evaluate(row=>({right:Math.max(...[...row.querySelectorAll('.archive-ball')].map(ball=>ball.getBoundingClientRect().right)),bounds:row.getBoundingClientRect().right}))
   expect(geometry.right).toBeLessThanOrEqual(geometry.bounds+1)
   await expect(page.locator('.draw-archive h2')).toHaveCount(0)
  }
  await assertCompactResultRow()
  const attribute=page.getByRole('radiogroup',{name:locale==='en'?'Attribute':'属性',exact:true})
  await expect(attribute.locator('input[value=zodiac]')).toBeChecked()
  await attribute.locator('label').filter({has:page.locator('input[value=element]')}).click()
  await expect(page.locator('.archive-number-attribute')).toHaveText(Array(7).fill(locale==='en'?'Wood':'木'))
  await attribute.locator('label').filter({has:page.locator('input[value=zodiac]')}).click()
  await page.locator('input[name=archive-sort][value=size]').check()
  await expect(page.locator('.archive-numbers .archive-ball')).toHaveText(['03','12','14','25','28','48','07'])
  await page.locator('input[name=archive-sort][value=drawn]').check()
  await expect(page.locator('.archive-numbers .archive-ball')).toHaveText(['03','12','28','14','25','48','07'])
  await expect(page.locator(".archive-filter .select-control-trigger")).toHaveAttribute('data-value', '2026')
  await selectControl(page.locator(".archive-filter .select-control-trigger"), '2025')
  await expect(page.locator('.draw-archive .archive-period')).toHaveText(locale==='en'?'10':'10期')
  await expect(page.locator('.draw-archive [role="tablist"]')).toHaveCount(0)
  await expect(page.locator(".draw-archive .archive-toolbar .select-control-trigger")).toHaveCount(1)
  if (info.project.name.endsWith('reflow')) {
   const original = page.viewportSize()!
   for (const width of [320, 640, 999, 1000, 1001]) {
    await page.setViewportSize({width,height:720})
    await expectNoPageOverflow(page)
    await expect(page.locator('.draw-archive .archive-period')).toBeVisible()
    await assertCompactResultRow()
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
   return route.fulfill({json:{success:true,data:{game_id:gameId,game_name:'Quick draw',mark_six:false,timezone:'Asia/Singapore',daily:true,choices:['2026-10-23','2026-10-22'],selected,has_more:false,total_count:'1',items:[{period_no:selected.replaceAll('-','')+'001',drawn_at:selected+'T01:00:00Z',draw_date:null,result:{regular:[],special:[],digits:[0,1,0]},attributes:{}}]}}})
  })
  await page.reload()
  await expect(page.locator(".archive-filter .select-control-trigger")).toHaveAttribute('data-value', '2026-10-23')
  expect(await page.locator(".archive-filter .select-control-trigger").evaluate(select=>select.getBoundingClientRect().height)).toBe(await page.evaluate(()=>matchMedia('(min-width:901px) and (pointer:fine)').matches?32:44))
  await selectControl(page.locator(".archive-filter .select-control-trigger"), '2026-10-22')
  await expect(page.locator('.draw-archive .archive-period')).toContainText('20261022001')
  await expect(page.locator('.archive-numbers .archive-ball')).toHaveText(['0','1','0'])
  await expectNoPageOverflow(page)
  await page.goto(`${origins.user}/orders`)
  const trigger=page.locator('.workspace-game-picker .select-control-trigger')
  await trigger.click();await expect(page.locator('.workspace-game-picker [role="option"]')).toHaveCount(4)
  for(let index=0;index<4;index++){
   if(!await page.locator('.workspace-game-picker [role="listbox"]').isVisible())await trigger.click()
   await page.locator('.workspace-game-picker [role="option"]').nth(index).click()
   await expect(page.locator('.order-history-empty')).toBeVisible();await expectNoPageOverflow(page)
  }
  const snapshot=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/canada_niuniu.json','utf8')))
  await page.route('**/api/v1/betting-history?*',route=>{
   expect(route.request().method()).toBe('GET')
   const selected=new URL(route.request().url()).searchParams.get('game_id')??''
   const order={id:'historical-risk',game_id:gameId,game_name:'Niu risk history',period_no:'history-1',status:'settled',snapshot,quote:{bet_points:'10',reserved_points:'200',lines:[{play_id:'aa',options:['aa'],plate:'A',stake:'10',prices:{aa:'2'}}]},result:{prize_points:'0',refund_points:'10',consumed_points:'190',lines:[{outcome:'lost',prize_points:'0'}]}}
   return route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,game_id:selected,games:[{id:gameId,name:'Niu risk history'}],items:[{id:order.id,kind:'configured',game_id:gameId,game_name:order.game_name,period_no:order.period_no,placed_at:'2026-10-21T00:00:00Z',order}],limit:20,total_count:'1',next_cursor:null}}})
  })
  await page.reload()
  const ticket=page.locator('.history-table');await expect(ticket).toHaveCount(1)
  await expect(ticket.locator('td[data-label="'+(locale==='en'?'Reserved risk points':'风险冻结积分')+'"]')).toHaveText('200')
  await expect(ticket.locator('td[data-label="'+(locale==='en'?'Actual loss points':'实际亏损积分')+'"]')).toHaveText('190')
  await expect(ticket.locator('td[data-label="'+(locale==='en'?'Released risk points':'风险解冻积分')+'"]')).toHaveText('10')
  await expectCompactPageLayout(page)
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`configured-history-${locale}.png`),fullPage:true})
 })
}
