import { test, expect, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { configuredDemo } from '../../shared/src/configured-rules'
import { configuredTicketLabel } from '../../shared/src/configured-business'
import { installLayoutApi, installWalletLayoutApi, gameId, origins, theme, expectNoPageOverflow } from './fixture'

const at = '2026-11-03T01:00:00Z'
const snapshot = configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json', 'utf8')))
const play = snapshot.catalog.plays.find(play=>play.id==='special_numbers')!
const displayPlay = {id:play.id,label:play.label,selection:{domain:play.selection.domain}}
// Shared member, game, play, period and timestamp must never merge distinct order IDs.
const orders = ['10.01', '20.02', '30.03'].map((stake, index) => {
 const line = { play_id:'special_numbers', options:[String(index + 1)], plate:'A', stake, prices:{hit:'48.99'} }
 return { id:`bbbbbbbb-bbbb-4bbb-8bbb-${String(index + 1).padStart(12,'0')}`, member_name:'item_member', game_id:gameId, game_name:'Mark Six', period_id:gameId, period_no:'2026307', rule_id:gameId, sales_version:1, snapshot:{catalog:{funding:null,plays:[displayPlay],domains:{[displayPlay.selection.domain]:{options:[{id:line.options[0]!,label:snapshot.catalog.domains[displayPlay.selection.domain]!.options.find(option=>option.id===line.options[0])!.label}]}}}},
  quote:{definition_hash:'item-records',snapshot_hash:'item-records',requests:[{play_id:line.play_id,options:line.options,plate:'A',points:stake,multiplier:'1'}],lines:[line],bet_points:stake},
  result:index === 1 ? {bet_points:stake,prize_points:'0',refund_points:'0',effective_points:stake,rounding:[],lines:[{play_id:line.play_id,options:line.options,outcome:'lost',raw_points:'0',prize_points:'0',refund_points:'0',effective_points:stake}]} : null,
  status:index === 1 ? 'settled':'placed',placed_at:at,settled_at:index === 1 ? at:null }
})
const items = orders.map(order => ({id:order.id,kind:'configured',game_id:gameId,game_name:order.game_name,period_no:order.period_no,placed_at:at,order}))
const member = {id:gameId,username:'item_member',display_name:'',phone:'',available_points:'100',agent_username:'review_agent',is_direct:true,joined_at:at,status:'active'}

async function openRecords(page:Page, surface:'user'|'brand'|'platform'|'agent', locale:'en'|'zh-CN') {
 page.setDefaultTimeout(8_000)
 const zh = locale === 'zh-CN'
 await installLayoutApi(page,locale)
 if(surface === 'user') {
  await installWalletLayoutApi(page)
  await page.route('**/api/v1/betting-history?*',route => {
   expect(route.request().method()).toBe('GET')
   const query = new URL(route.request().url()).searchParams
   return route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,game_id:query.get('game_id') ?? '',games:[{id:gameId,name:'Mark Six'}],items,limit:Number(query.get('limit')),next_cursor:null,total_count:'3'}}})
  })
  await page.goto(origins.user+'/orders')
  return page.locator('.history-table tbody tr')
 }
 if(surface === 'agent') {
  await page.route('**/api/v1/agent/**',route => {
   expect(route.request().method()).toBe('GET')
   const path = new URL(route.request().url()).pathname
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/me'))return reply({account:{agent_account_id:gameId,username:'review_agent'},agent:{id:gameId,brand_id:gameId,depth:1,config:{can_create_children:false}},code:null,user_origin:origins.user})
   if(path.endsWith('/members'))return reply({items:[member],total_count:'1',offset:0,limit:20,summary:{member_count:'1',available_points:'100',bet_count:'3',stake_points:'60.06',unsettled_points:'40.04',net_points:'-20.02'}})
   if(path.endsWith('/summary'))return reply({member,wallet:{available_points:'100'},net_points:'-20.02',unsettled_points:'40.04'})
   if(path.endsWith('/bets'))return reply({items,total_count:'3',next_cursor:null})
   throw Error('Unexpected agent request: '+path)
  })
  await page.goto(origins.agent)
  await page.locator('.member-menu-trigger').first().click()
  await page.getByRole('menuitem',{name:zh?'查看注单':'View bets',exact:true}).click()
  return page.locator('.member-bets .history-table tbody tr')
 }
 const scope = surface === 'brand' ? 'admin':'platform'
 if(surface === 'platform')await page.addInitScript(id=>sessionStorage.setItem('lottery.platform.selectedBrand',JSON.stringify({accountId:id,brandId:id})),gameId)
 await page.route(`**/api/v1/${scope}/**`,route => {
  expect(route.request().method()).toBe('GET')
  const path = new URL(route.request().url()).pathname
  const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(path.endsWith('/me'))return reply({account:{id:gameId,username:'item_reviewer',super_admin:surface === 'platform',brand_ids:[gameId],permissions_by_brand:{[gameId]:['bet.view.brand']},platform_permissions:['bet.view.platform']}})
  if(path.endsWith('/presentation'))return reply({display_name:'Layout trial',logo_text:'L',logo_url:''})
  if(path.endsWith('/platform-settings'))return reply({theme:'jade',version:1,display_name:'Layout trial',logo_text:'L',logo_url:'',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',updated_at:at})
  if(path.endsWith('/brands'))return reply({items:[{id:gameId,code:'layout',name:'Layout trial',status:'active',member_count:'1',agent_count:'1',recharge_quota_available:'100',created_at:at}],total_count:'1'})
  if(path.endsWith('/brand-presentation'))return reply({brand_id:gameId,version:1,status:'active',base_name:'Layout trial',config:{},effective:theme})
  if(path.endsWith('/filter-options'))return reply({brand_id:gameId,items:[{id:gameId,name:'Mark Six'}],limit:100,offset:0})
  if(path.endsWith('/configured-orders'))return reply({items:orders,total_count:'3'})
  throw Error('Unexpected console request: '+path)
 })
 return navigateConsoleRecords(page,surface,locale)
}

async function navigateConsoleRecords(page:Page,surface:'brand'|'platform',locale:'en'|'zh-CN') {
 const zh=locale==='zh-CN'
 await page.goto(origins[surface])
 await expect(page.locator('.management-console')).toBeVisible()
 if(surface === 'platform') {
  const toggle=page.locator('button[aria-controls="platform-navigation"]')
  if(await toggle.isVisible() && await toggle.getAttribute('aria-expanded') === 'false')await toggle.click()
  await page.locator('#platform-navigation').getByRole('button',{name:zh?'注单':'Bets',exact:true}).click()
 } else {
  const mobile=await page.locator('.mobile-nav').isVisible()
  if(mobile)await page.locator('.mobile-nav button').last().click()
  const nav=page.locator(mobile?'.mobile-more-menu':'.side-nav')
  const button=nav.getByRole('button',{name:zh?'注单管理':'Bet order management',exact:true,includeHidden:true})
  if(!await button.isVisible())await button.locator('xpath=ancestor::details').locator('summary').click()
  await button.click()
 }
 return page.locator('.configured-orders tbody tr')
}

async function replaceRecords(page:Page,surface:'user'|'agent'|'brand'|'platform',locale:'en'|'zh-CN',records:typeof orders) {
  if(surface==='user' || surface==='agent') {
   const history=records.map(order=>({id:order.id,kind:'configured',game_id:gameId,game_name:order.game_name,period_no:order.period_no,placed_at:at,order}))
   const path=surface==='user'?'**/api/v1/betting-history?*':'**/api/v1/agent/members/*/bets?*'
   await page.route(path,route=>route.fulfill({json:{success:true,data:surface==='user'?{brand_id:gameId,member_id:gameId,game_id:new URL(route.request().url()).searchParams.get('game_id')??'',games:[{id:gameId,name:'Mark Six'}],items:history,limit:20,next_cursor:null,total_count:String(records.length)}:{items:history,total_count:String(records.length),next_cursor:null}}}))
   await page.reload()
  } else {
   await page.route(`**/api/v1/${surface==='brand'?'admin':'platform'}/configured-orders?*`,route=>route.fulfill({json:{success:true,data:{items:records,total_count:String(records.length)}}}))
   await navigateConsoleRecords(page,surface,locale)
  }
}

for(const surface of ['user','brand','platform','agent'] as const)for(const locale of ['en','zh-CN'] as const) {
 test(`independent betting records (${surface}, ${locale})`,async({page},info)=>{
  const errors:string[]=[]
  page.on('pageerror',error=>errors.push(error.message))
  const rows=await openRecords(page,surface,locale)
  await expect(rows).toHaveCount(3)
  const preset=page.viewportSize()!
  for(const width of info.project.name.endsWith('reflow')?[320,700,701,900,901,1440]:[preset.width]) {
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   await expect(rows).toHaveCount(3)
   const headers=await page.locator('.history-table thead th').allTextContents()
   expect(headers.filter(label=>!['彩种','会员','Game','Member'].includes(label))).toEqual(locale==='en'?['Period','Bet details','Result','Odds','Stake points','Prize points','Status','Placed at']:['期号','投注内容','结果','赔率','投注积分','中奖积分','状态','投注时间'])
   if(await page.evaluate(()=>matchMedia('(max-width:700px), (max-width:900px) and (pointer:coarse)').matches)) {
    for(const row of await rows.all())expect((await row.boundingBox())!.height).toBeLessThanOrEqual(260)
    expect(await page.locator('.history-table').evaluate(table=>table.scrollWidth<=table.clientWidth+1)).toBe(true)
   }
   for(const [index,order] of orders.entries()) {
    await expect(rows.nth(index)).toContainText(order.period_no)
    await expect(rows.nth(index)).toContainText(order.quote.bet_points)
    await expect(rows.nth(index)).toContainText(configuredTicketLabel(snapshot,order.quote.lines[0]!,locale))
    for(const other of orders.filter(o=>o.id!==order.id))await expect(rows.nth(index)).not.toContainText(other.quote.bet_points)
   }
   // Distinct rows/cards stay vertically separate on both table and mobile layouts.
   const boxes=await rows.evaluateAll(nodes=>nodes.map(node=>{const b=node.getBoundingClientRect();return {top:b.top,bottom:b.bottom}}))
   for(let index=1;index<boxes.length;index++)expect(boxes[index]!.top).toBeGreaterThanOrEqual(boxes[index-1]!.bottom)
   await expectNoPageOverflow(page)
   expect(errors).toEqual([])
   await page.screenshot({path:info.outputPath(`${surface}-${locale}-${width}.png`),fullPage:true})
  }
 })
}

for(const surface of ['user','agent','brand','platform'] as const)for(const locale of ['en','zh-CN'] as const) {
 test(`betting financial columns (${surface}, ${locale})`,async({page},info)=>{
  const rows=await openRecords(page,surface,locale)
  await expect(rows).toHaveCount(3)
  const zh=locale==='zh-CN'
  const content=zh?'投注内容':'Bet details', stake=zh?'投注积分':'Stake points', prize=zh?'中奖积分':'Prize points'
  const refund=zh?'退本积分':'Refund points', release=zh?'风险解冻积分':'Released risk points'
  // Zero refunds stay absent and pending prizes remain unconfirmed.
  await expect(rows.nth(0).locator(`td[data-label="${prize}"]`)).toHaveText('—')
  await expect(rows.nth(1).locator(`td[data-label="${prize}"]`)).toHaveText('0')
  await expect(page.locator('.history-table')).not.toContainText(refund)
  const preset=page.viewportSize()!
  for(const funding of ['debit','reserve']) {
   const order=orders[1]!
   const financialOrders=[{...order,snapshot:{...order.snapshot,catalog:{...order.snapshot.catalog,funding}},quote:{...order.quote,reserved_points:'100.25'},result:{...order.result!,prize_points:'48.99',refund_points:'0.01',consumed_points:'20.01'}}]
   await replaceRecords(page,surface,locale,financialOrders)
   await expect(rows).toHaveCount(1)
   const headings=await page.locator('.history-table thead th').allTextContents()
   expect(headings.filter(label=>!['彩种','会员','Game','Member'].includes(label))).toEqual(zh?['期号','投注内容','结果','赔率','投注积分','中奖积分','风险冻结积分','实际亏损积分',funding==='reserve'?release:refund,'状态','投注时间']:['Period','Bet details','Result','Odds','Stake points','Prize points','Reserved risk points','Actual loss points',funding==='reserve'?release:refund,'Status','Placed at'])
   await expect(page.locator('.configured-orders [data-refresh-button]')).toHaveCount(0)
   for(const width of info.project.name.endsWith('reflow')?[320,700,701,900,901,1440]:[preset.width]) {
    if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
    const playCell=rows.first().locator(`td[data-label="${content}"]`)
    await expect(playCell).toHaveText(configuredTicketLabel(snapshot,order.quote.lines[0]!,locale))
    await expect(rows.first().locator(`td[data-label="${stake}"]`)).toHaveText(order.quote.bet_points)
    await expect(rows.first().locator(`td[data-label="${zh?'风险冻结积分':'Reserved risk points'}"]`)).toHaveText('100.25')
    await expect(rows.first().locator(`td[data-label="${zh?'实际亏损积分':'Actual loss points'}"]`)).toHaveText('20.01')
    const prizeCell=rows.first().locator(`td[data-label="${prize}"]`)
    await expect(prizeCell).toContainText('48.99')
    await expect(prizeCell).toHaveText('48.99')
    await expect(rows.first().locator(`td[data-label="${funding==='reserve'?release:refund}"]`)).toHaveText('0.01')
    await expectNoPageOverflow(page)
    await page.screenshot({path:info.outputPath(`${surface}-${locale}-${funding}-${width}.png`),fullPage:true})
   }
  }
 })
}

for(const surface of ['user','agent','brand','platform'] as const)for(const locale of ['en','zh-CN'] as const) {
 test(`betting outcome column (${surface}, ${locale})`,async({page},info)=>{
  const rows=await openRecords(page,surface,locale)
  const settled=orders[1]!
  const outcomes=['won','lost','push'] as const
  const records=[...outcomes.map((outcome,index)=>({...settled,id:`cccccccc-cccc-4ccc-8ccc-${String(index+1).padStart(12,'0')}`,result:{...settled.result!,lines:[{...settled.result!.lines[0]!,outcome}]}})),orders[0]!]
  await replaceRecords(page,surface,locale,records)
  await expect(rows).toHaveCount(4)
  const result=locale==='en'?'Result':'结果',play=locale==='en'?'Bet details':'投注内容'
  const labels=locale==='en'?['Won','Lost','Push']:['中奖','未中','退本']
  for(const [index,outcome] of outcomes.entries()) {
   await expect(rows.nth(index).locator(`td[data-label="${result}"]`)).toHaveText(labels[index]!)
   await expect(rows.nth(index).locator(`td[data-label="${play}"]`)).toHaveText(configuredTicketLabel(snapshot,settled.quote.lines[0]!,locale))
   await expect(rows.nth(index).locator(`.configured-order-outcome--${outcome}`)).toBeVisible()
  }
  await expect(rows.nth(3).locator(`td[data-label="${result}"]`)).toHaveText('—')
  for(const mode of ['light','dark']) {
   if(mode==='dark')await page.locator('.theme-switcher').click()
   const colors=await page.locator('.configured-order-outcome--won,.configured-order-outcome--lost,.configured-order-outcome--push').evaluateAll(nodes=>nodes.map(node=>getComputedStyle(node).color))
   expect(new Set(colors).size).toBe(3)
  }
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`outcomes-${surface}-${locale}.png`),fullPage:true})
 })
}
