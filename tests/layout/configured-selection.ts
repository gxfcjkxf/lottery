import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { configuredDemo } from '../../shared/src/configured-rules'
import { origins, gameId, installLayoutApi, installOrdersLayoutApi, expectNoPageOverflow } from './fixture'

export function registerConfiguredSelectionTests() {
for (const locale of ['en', 'zh-CN'] as const) {
 test(`empty quick stake chip shows blinking insertion caret (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installOrdersLayoutApi(page)
  let values:string[]=[]
  await page.route('**/api/v1/me/betting-preferences',route=>{if(route.request().method()==='PUT')values=route.request().postDataJSON().quick_amounts;return route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,quick_amounts:values}}})})
  await page.goto(`${origins.user}/account`)
  const input=page.getByTestId('betting-preferences').locator('.quick-amount-chip input').first()
  await input.click();await expect(input).toBeFocused();await expect(input).toHaveValue('')
  const chip=input.locator('..')
  const marker=()=>chip.evaluate(element=>{const s=getComputedStyle(element,'::after');return {content:s.content,width:s.width,height:s.height,animation:s.animationName,opacity:s.opacity,events:s.pointerEvents}})
  expect(await marker()).toMatchObject({content:'""',width:'2px',height:'20px',animation:'quick-stake-empty-caret',events:'none'})
  await expect.poll(async()=>(await marker()).opacity).toBe('1')
  await expect.poll(async()=>(await marker()).opacity).toBe('0')
  await expect.poll(async()=>(await marker()).opacity).toBe('1')
  await input.screenshot({path:info.outputPath(`empty-chip-caret-${locale}.png`),animations:'disabled',caret:'initial'})
  await input.fill('50')
  await expect.poll(()=>values).toEqual(['50'])
  await expect(input).toBeFocused()
  expect((await marker()).content).not.toBe('""')
  expect(await input.evaluate(element=>getComputedStyle(element).caretColor)).not.toBe('rgba(0, 0, 0, 0)')
 })
 test(`quick stake chip focus and six digit sizes (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installOrdersLayoutApi(page)
  let values=['10','100','1000','10000','999999']
  await page.route('**/api/v1/me/betting-preferences',route=>{if(route.request().method()==='PUT')values=route.request().postDataJSON().quick_amounts;return route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,quick_amounts:values}}})})
  await page.goto(`${origins.user}/account`)
  const settings=page.getByTestId('betting-preferences'),inputs=settings.locator('.quick-amount-chip input')
  await expect(inputs.last()).toHaveValue('999999')
  const sizes=await settings.locator('.quick-amount-chip').evaluateAll(chips=>chips.map(chip=>chip.getBoundingClientRect().width))
  for(let i=1;i<sizes.length;i++)expect(sizes[i]).toBeGreaterThan(sizes[i-1]!)
  const before=await inputs.last().evaluate(input=>getComputedStyle(input).backgroundColor)
  await inputs.last().click();await expect(inputs.last()).toBeFocused()
  expect(await inputs.last().evaluate(input=>getComputedStyle(input).backgroundColor)).not.toBe(before)
  await expect(inputs.last()).toHaveCSS('border-top-width','2px')
  await expect(inputs.last()).toHaveCSS('cursor','text')
  expect(await inputs.last().evaluate(input=>getComputedStyle(input).caretColor)).not.toBe('rgba(0, 0, 0, 0)')
  await inputs.last().fill('999998')
  await expect.poll(()=>values.at(-1)).toBe('999998')
  expect(await inputs.last().evaluate(input=>(input as HTMLInputElement).selectionStart)).toBe(6)
  await inputs.last().fill('1000000')
  await expect(inputs.last()).toHaveValue('999998')
  await expect(settings.getByRole('status')).toContainText('999999')
  await expect(settings.getByRole('alert')).toHaveCount(0)
  expect(values.at(-1)).toBe('999998')
  async function fits(locator:ReturnType<typeof page.locator>){
   for(const core of await locator.all()){
    expect(await core.evaluate(element=>{const style=getComputedStyle(element),canvas=document.createElement('canvas'),ctx=canvas.getContext('2d')!;ctx.font=`${style.fontWeight} ${style.fontSize} ${style.fontFamily}`;const value=element instanceof HTMLInputElement?element.value:element.textContent!;return ctx.measureText(value).width<=element.clientWidth-parseFloat(style.paddingLeft)-parseFloat(style.paddingRight)})).toBe(true)
   }
  }
  const widths=info.project.name.endsWith('reflow')?[320,375,768,1000,1440]:[page.viewportSize()!.width]
  for(const width of widths){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:720})
   await fits(inputs);await expectNoPageOverflow(page)
   const boxes=await settings.locator('.quick-amount-chip').evaluateAll(chips=>chips.map(chip=>{const b=chip.getBoundingClientRect();return {top:b.top,bottom:b.bottom,center:b.top+b.height/2}}))
   for(const a of boxes)for(const b of boxes)if(Math.max(a.top,b.top)<Math.min(a.bottom,b.bottom))expect(Math.abs(a.center-b.center)).toBeLessThan(1)
  }
  await page.screenshot({path:info.outputPath(`focused-six-digit-chips-${locale}.png`),caret:'initial'})
  await page.goto(`${origins.user}/configured/${gameId}`)
  await page.locator('.configured-stakes--dock input').click()
  const popup=page.locator('.configured-quick-amounts')
  await expect(popup.getByRole('button')).toHaveCount(5)
  await expect(popup.getByRole('button').first()).toHaveCSS('cursor','pointer')
  await fits(popup.locator('.stake-chip-core'));await expect(popup).toBeInViewport({ratio:1});await expectNoPageOverflow(page)
  const first=popup.getByRole('button').first(),beforeSelected=await first.boundingBox()
  await first.click();await page.locator('.configured-stakes--dock input').click()
  await expect(first).toHaveAttribute('aria-pressed','true')
  const afterSelected=await first.boundingBox()
  expect(afterSelected!.width).toBe(beforeSelected!.width);expect(afterSelected!.height).toBe(beforeSelected!.height)
  await expect(popup).toHaveCSS('gap','12px')
  const chipBoxes=await popup.getByRole('button').evaluateAll(chips=>chips.map(chip=>{const b=chip.getBoundingClientRect();return {x:b.x,right:b.right,y:b.y,bottom:b.bottom}}))
  for(let i=0;i<chipBoxes.length;i++)for(let j=i+1;j<chipBoxes.length;j++){
   const a=chipBoxes[i]!,b=chipBoxes[j]!
   if(Math.max(a.y,b.y)<Math.min(a.bottom,b.bottom))expect(Math.max(b.x-a.right,a.x-b.right)).toBeGreaterThanOrEqual(11.9)
  }

  await page.screenshot({path:info.outputPath(`six-digit-quick-chips-${locale}.png`)})
 })
 test(`quick stake popup follows amount input (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installOrdersLayoutApi(page)
  let values=['50']
  await page.route('**/api/v1/me/betting-preferences',route=>route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,quick_amounts:values}}}))
  const widths=info.project.name.endsWith('reflow')?[320,375,620,768,999,1000,1001,1440]:[page.viewportSize()!.width]
  for(const count of [1,5]){
   values=count===1?['50']:['5','10','20','50','100']
   await page.goto(`${origins.user}/configured/${gameId}`)
   const points=page.locator('.configured-stakes--dock input[type="number"]'),popup=page.locator('.configured-quick-amounts')
   for(const width of widths){
    if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:720})
    await points.click();await expect(popup.getByRole('button')).toHaveCount(count)
    await expect(popup).toBeInViewport({ratio:1})
    await expect.poll(async()=>{
     const p=await popup.boundingBox(),i=await points.boundingBox(),d=await page.locator('.configured-stakes--dock').boundingBox()
     const center=Math.max(8+p!.width/2,Math.min(i!.x+i!.width/2,page.viewportSize()!.width-8-p!.width/2))
     return Math.max(Math.abs(p!.x+p!.width/2-center),Math.abs(p!.y+p!.height-d!.y+8))
    }).toBeLessThan(2)
    await expectNoPageOverflow(page)
   }
   await page.screenshot({path:info.outputPath(`aligned-${count}-chips-${locale}.png`)})
  }
 })
 test(`personal quick stakes and amount memory (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installOrdersLayoutApi(page)
  let savedAmounts:string[]=[]
  await page.route('**/api/v1/me/betting-preferences',route=>{if(route.request().method()==='PUT')savedAmounts=route.request().postDataJSON().quick_amounts;return route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,quick_amounts:savedAmounts}}})})
  await page.goto(`${origins.user}/account`)
  const settings=page.getByTestId('betting-preferences')
  await expect(settings).toBeVisible()
  const chipInputs=settings.locator('.quick-amount-chip input')
  await expect(chipInputs).toHaveCount(5)
  await expect.poll(()=>chipInputs.evaluateAll(inputs=>inputs.map(input=>(input as HTMLInputElement).value))).toEqual(['','','','',''])
  for(const [index,amount] of ['5','10','20','50','100'].entries()){
   await chipInputs.nth(index).fill(amount)
   await expect.poll(()=>savedAmounts[index]).toBe(amount)
  }
  await expect(settings.getByRole('button',{name:locale==='en'?'Add':'添加',exact:true})).toHaveCount(0)
  await settings.locator('.quick-amount-chip').nth(1).getByRole('button').click()
  await expect.poll(()=>savedAmounts.length).toBe(4)
  await chipInputs.first().fill('7')
  await expect.poll(()=>savedAmounts[0]).toBe('7')
  await chipInputs.last().fill('200')
  await expect.poll(()=>savedAmounts.at(-1)).toBe('200')
  await expect(settings.getByRole('button',{name:locale==='en'?'Save':'保存',exact:true})).toHaveCount(0)
  if(info.project.name.endsWith('reflow')){
   for(const width of [320,768,1000,1440]){await page.setViewportSize({width,height:720});await expectNoPageOverflow(page)}
  }
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`stake-settings-${locale}.png`),fullPage:true})
  expect(savedAmounts).toEqual(['7','20','50','100','200'])
  const otherContext=await page.context().browser()!.newContext({viewport:page.viewportSize()!})
  try{
   const otherPage=await otherContext.newPage()
   await installLayoutApi(otherPage,locale);await installOrdersLayoutApi(otherPage)
   await otherPage.route('**/api/v1/me/betting-preferences',route=>route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,quick_amounts:savedAmounts}}}))
   await otherPage.goto(`${origins.user}/account`)
   await expect(otherPage.getByTestId('betting-preferences').locator('.quick-amount-chip input').first()).toHaveValue('7')
   expect(await otherPage.getByTestId('betting-preferences').locator('.quick-amount-chip input').evaluateAll(inputs=>inputs.map(input=>(input as HTMLInputElement).value))).toEqual(['7','20','50','100','200'])
  }finally{await otherContext.close()}
  await page.goto(`${origins.user}/configured/${gameId}`)
  const selection=page.getByTestId('configured-selection'),dock=selection.locator('.configured-stakes--dock'),points=dock.locator('input[type="number"]')
  await expect(points).toHaveValue('')
  await points.click()
  const shortcuts=selection.getByRole('group',{name:locale==='en'?'Quick stake amounts':'常用投注积分',exact:true})
  await expect(shortcuts.getByRole('button')).toHaveText(['7','20','50','100','200'])
  await expect(shortcuts.getByRole('button').first()).toHaveCSS('border-radius','50%')
  const chipColors=await shortcuts.getByRole('button').evaluateAll(buttons=>buttons.map(button=>getComputedStyle(button).getPropertyValue('--chip-color').trim()))
  expect(new Set(chipColors).size).toBe(5)
  for(const chip of await shortcuts.getByRole('button').all()){const circle=await chip.locator('span').boundingBox();expect(Math.abs(circle!.width-circle!.height)).toBeLessThan(1)}
  await expect(shortcuts).toBeInViewport({ratio:1})
  const shortcutBox=await shortcuts.boundingBox(),inputBox=await points.boundingBox()
  expect(shortcutBox!.y+shortcutBox!.height).toBeLessThanOrEqual(inputBox!.y)
  await shortcuts.getByRole('button',{name:'50',exact:true}).click()
  await expect(points).toHaveValue('50');await expect(shortcuts).toHaveCount(0)
  await points.fill('75')
  await page.reload();await expect(points).toHaveValue('75')
  await selection.getByRole('tab').nth(1).click();await expect(points).toHaveValue('75')
  const clear=dock.getByRole('button',{name:locale==='en'?'Clear':'清空',exact:true}),bet=dock.getByRole('button',{name:locale==='en'?'Bet':'投注',exact:true})
  const sizes=info.project.name.endsWith('reflow')?[320,375,768,999,1000,1001,1440]:[page.viewportSize()!.width]
  for(const width of sizes){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:720})
   await points.click();await expect(shortcuts).toBeInViewport({ratio:1});await expectNoPageOverflow(page)
   const popup=await shortcuts.boundingBox(),input=await points.boundingBox()
   const expectedCenter=Math.max(8+popup!.width/2,Math.min(input!.x+input!.width/2,page.viewportSize()!.width-8-popup!.width/2))
   expect(Math.abs(popup!.x+popup!.width/2-expectedCenter)).toBeLessThan(2)
   expect(Math.abs(popup!.y+popup!.height-((await dock.boundingBox())!.y-8))).toBeLessThan(2)
   const a=await clear.boundingBox(),b=await bet.boundingBox();expect(Math.abs(a!.y-b!.y)).toBeLessThan(2)
  }
  await page.screenshot({path:info.outputPath(`quick-stakes-${locale}.png`)})
  await page.goto(`${origins.user}/account`)
  await expect(settings.locator('.quick-amount-chip input').first()).toHaveValue('7')
  expect(await settings.locator('.quick-amount-chip input').evaluateAll(inputs=>inputs.map(input=>(input as HTMLInputElement).value))).toEqual(['7','20','50','100','200'])
  await settings.locator('.quick-amount-chip input').first().fill('8')
  await settings.locator('.quick-amount-chip input').first().blur()
  await expect.poll(()=>savedAmounts[0]).toBe('8')
  await page.goto(`${origins.user}/configured/${gameId}`);await expect(points).toHaveValue('75')
  await points.click();await expect(shortcuts.getByRole('button').first()).toHaveText('8')
  const fiveChipWidth=(await shortcuts.boundingBox())!.width
  savedAmounts=['8']
  await page.reload();await points.click();await expect(shortcuts.getByRole('button')).toHaveCount(1)
  await expect(shortcuts).toBeInViewport({ratio:1})
  const oneChipWidth=(await shortcuts.boundingBox())!.width
  const singlePopup=await shortcuts.boundingBox(),singleInput=await points.boundingBox()
  expect(Math.abs(singlePopup!.x+singlePopup!.width/2-singleInput!.x-singleInput!.width/2)).toBeLessThan(2)
  expect(oneChipWidth).toBeLessThan(100);expect(oneChipWidth).toBeLessThan(fiveChipWidth/2)
  await page.screenshot({path:info.outputPath(`one-quick-stake-${locale}.png`)})
 })
 test(`compact configured selection and period synchronization (${locale})`, async ({ page }, info) => {
  await page.clock.install({time: Date.now()})
  const start = await page.evaluate(() => Date.now())
  await installLayoutApi(page, locale)
  await installOrdersLayoutApi(page)
  const walletSnapshot = (available: string, version: number) => ({account_id:gameId,brand_id:gameId,member_id:gameId,version,available_points:available,display_points:available,recharge_points:available,winning_points:'0',gift_points:'0',commission_points:'0',frozen_points:'0',withdrawal_points:'0',manual_frozen_points:'0',system_frozen_points:'0',by_source:Object.fromEntries(['recharge','winning','gift','commission'].map(source=>[source,{available:source==='recharge'?available:'0',manual_frozen:'0',system_frozen:'0',withdrawal:'0'}]))})
  await page.route('**/api/v1/wallet',route=>route.fulfill({json:{success:true,data:walletSnapshot('40',1)}}))
  const snapshot = configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json', 'utf8')))
  let reads = 0, closed = false, orderReads = 0, periodReads = 0
  page.on('request', request => {if (new URL(request.url()).pathname === `/api/v1/configured/games/${gameId}/orders`) orderReads++})
  await page.route(`**/api/v1/configured/games/${gameId}`, route => {
   reads++
   return route.fulfill({json: {success: true, data: {
    game: {id: gameId, name: 'Mark Six', status: 'active', brand_enabled: true},
    snapshot, rule_id: gameId, snapshot_hash: 'compact-selection', sales: {version: 1, enabled: true},
    period: !closed ? {id: gameId, period_no: '2026111', bet_start_at: new Date(start - 3600000).toISOString(), bet_end_at: new Date(start + 3600000).toISOString(), draw_at: new Date(start + 3660000).toISOString()} : null,
   }}})
  })
  const currentPeriod = {id: gameId, game_id: gameId, period_no: '2026111', bet_start_at: new Date(start - 3600000).toISOString(), bet_end_at: new Date(start + 3600000).toISOString(), draw_at: new Date(start + 3660000).toISOString(), result: null, published_at: null}
  await page.route(`**/api/v1/platform-games/${gameId}?*`, route => {
   periodReads++
   return route.fulfill({json: {success: true, data: {game: {id: gameId, name: 'Mark Six'}, plays: [], rule_protocol: 'configured', periods: [currentPeriod, {...currentPeriod, id: 'previous-draw', period_no: '2026110', draw_at: new Date(start - 3600000).toISOString(), result: {regular: [1, 2, 3, 4, 5, 6], special: [7], digits: []}}]}}})
  })
  await page.goto(`${origins.user}/configured/${gameId}`)
  const selection = page.getByTestId('configured-selection')
  await expect(selection.locator('.configured-options button')).toHaveCount(49)
  await expect(page.locator('.configured-live > h2')).toHaveCount(0)
  const strip = page.locator('.configured-live .user-period-strip')
  await expect(strip.locator('.result-row')).toContainText('2026110')
  await expect(strip.locator('.configured-result-ball')).toHaveCount(7)
  await expect(strip.locator('.configured-result-plus')).toHaveText('+')
  await expect(strip.locator('.configured-result-ball').nth(0)).toHaveClass(/lottery-wave-red/)
  await expect(strip.locator('.configured-result-ball').nth(1)).toHaveClass(/lottery-wave-blue/)
  await expect(strip.locator('.configured-result-ball').nth(2)).toHaveClass(/lottery-wave-green/)
  await expect(strip.locator('.configured-result-ball.special')).toHaveText('07')
  await expect(strip.locator('.period-number')).toHaveText(locale === 'en' ? '2026111' : '2026111期')
  await expect(strip.locator('.period-times time').first()).toHaveAttribute('datetime', currentPeriod.bet_end_at)
  await expect(strip.locator('.period-times time').last()).toHaveAttribute('datetime', currentPeriod.draw_at)
  const history = page.locator('.configured-live .game-results-link')
  await expect(history).toHaveCount(0)
  await expect(page.locator('.configured-live article')).toHaveCount(0)
  await expect(page.getByRole('button', {name: /刷新彩种与订单|Refresh game and orders/})).toHaveCount(0)
  await expect(selection).not.toContainText(/每个选项一注|可选.*项|Each option|options per entry|请在投注核对页/)
  const dock = selection.locator('.configured-stakes--dock')
  await expect(dock).toHaveCSS('position', 'fixed')
  await expect(dock.locator('.configured-available strong')).toHaveText('40')
  for (const label of await dock.locator('label').all()) {
   const title = await label.locator(':scope > span').boundingBox()
   const field = await label.locator('input,select').boundingBox()
   expect(title!.x + title!.width).toBeLessThanOrEqual(field!.x)
   expect(title!.y).toBeLessThan(field!.y + field!.height)
   expect(field!.y).toBeLessThan(title!.y + title!.height)
  }
  await expect(dock.locator('input[type="number"]')).toHaveValue('')
  const betButton = dock.getByRole('button', {name: locale === 'en' ? 'Bet' : '投注', exact: true})
  await betButton.click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await expect(page.getByRole('dialog').getByRole('alert')).toContainText(locale === 'en' ? 'Choose at least' : '请至少选择')
  await page.getByRole('dialog').getByRole('button', {name: locale === 'en' ? 'Continue choosing' : '继续选号', exact: true}).last().click()
  await selection.locator('.configured-options button').first().click()
  await betButton.click()
  await expect(page.getByRole('dialog').getByRole('alert')).toContainText(locale==='en'?'positive whole-point':'正整数积分')
  await page.getByRole('dialog').getByRole('button',{name:locale==='en'?'Continue choosing':'继续选号',exact:true}).last().click()
  await dock.getByRole('button',{name:locale==='en'?'Clear':'清空',exact:true}).click()
  await dock.locator('input[type="number"]').fill('10')
  const first = selection.locator('.configured-options button').first()
  await expect(first.locator('span')).toHaveCount(1)
  await expect(first.locator('.configured-option-price')).toHaveCSS('color', 'rgb(185, 28, 28)')
  await first.evaluate(element => element.scrollIntoView({block:'start'}))
  await first.click()
  const total = dock.locator('.configured-total-points')
  const points = dock.locator('input[type="number"]')
  await expect(points).toHaveCSS('appearance', 'textfield')
  await expect(total).toHaveText('10')
  const second = selection.locator('.configured-options button').nth(1)
  await second.click()
  await expect(total).toHaveText('20')
  await points.fill('25')
  await expect(total).toHaveText('50')
  await expect(betButton).toBeDisabled()
  await expect(betButton).toHaveCSS('background-color','rgb(226, 232, 240)')
  await expect(selection.locator('.configured-funds-toast')).toContainText(locale==='en'?'Insufficient available points':'可用积分不足')
  await expect(selection.locator('.configured-funds-toast')).toBeInViewport({ratio:1})
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`insufficient-${locale}.png`)})
  const clear = dock.getByRole('button', {name: locale === 'en' ? 'Clear' : '清空', exact: true})
  await clear.click()
  await expect(selection.locator('.configured-options button[aria-pressed="true"]')).toHaveCount(0)
  await expect(total).toHaveText('0')
  await expect(betButton).toBeEnabled()
  await expect(selection.locator('.configured-funds-toast')).toHaveCount(0)
  await expect(points).toHaveValue('25')
  await expect(clear).toBeDisabled()
  await first.evaluate(element => element.scrollIntoView({block:'start'}))
  await first.click()
  await second.click()
  await expect(total).toHaveText('50')
  await second.click()
  await expect(total).toHaveText('25')
  await expect(betButton).toBeEnabled()
  await expect(selection.locator('.configured-funds-toast')).toHaveCount(0)
  for(const identity of [{brand_id:'bbbbbbbb-bbbb-4bbb-8bbb-000000000001'}, {member_id:'bbbbbbbb-bbbb-4bbb-8bbb-000000000001'}]) {
   await page.evaluate(value=>window.dispatchEvent(new CustomEvent('lottery:wallet-snapshot',{detail:value})),{...walletSnapshot('1000',5),...identity})
   await expect(dock.locator('.configured-available strong')).toHaveText('40')
  }
  await page.evaluate(value=>window.dispatchEvent(new CustomEvent('lottery:wallet-snapshot',{detail:value})),walletSnapshot('20',2))
  await expect(selection.locator('.configured-funds-toast')).toBeVisible()
  await expect(betButton).toBeDisabled()
  await page.evaluate(value=>window.dispatchEvent(new CustomEvent('lottery:wallet-snapshot',{detail:value})),walletSnapshot('60',3))
  await expect(selection.locator('.configured-funds-toast')).toHaveCount(0)
  await page.evaluate(value=>window.dispatchEvent(new CustomEvent('lottery:wallet-snapshot',{detail:value})),walletSnapshot('20',2))
  await expect(dock.locator('.configured-available strong')).toHaveText('60')
  await expect(betButton).toBeEnabled()
  await points.fill('')
  await expect(total).toHaveText('—')
  await points.fill('10')
  await expect(total).toHaveText('10')
  await expect(first).toHaveAttribute('aria-pressed', 'true')
  await expect(first.locator('.configured-option-price')).toHaveCSS('color', 'rgb(255, 255, 255)')
  const tabs = selection.getByRole('tab')
  await expect(selection.getByRole('combobox', {name: locale === 'en' ? 'Play' : '玩法', exact: true})).toHaveCount(0)
  await expect(tabs.first()).toHaveAttribute('aria-selected', 'true')
  await tabs.nth(1).click()
  await expect(tabs.nth(1)).toHaveAttribute('aria-selected', 'true')
  await expect(selection.locator('.configured-options button[aria-pressed="true"]')).toHaveCount(0)
  await tabs.nth(2).click()
  for (const [index, wave, background] of [[0,'red','rgb(254, 226, 226)'],[1,'blue','rgb(219, 234, 254)'],[2,'green','rgb(209, 250, 229)']] as const) {
   const option = selection.locator('.configured-options button').nth(index)
   await expect(option).toHaveClass(new RegExp(`lottery-wave-${wave}`))
   await expect(option).toHaveCSS('background-color', background)
  }
  await selection.locator('.configured-options button').first().click()
  await expect(selection.locator('.configured-options button').first()).toHaveCSS('background-color', 'rgb(220, 38, 38)')
  await expect(selection.locator('.configured-option-price').first()).toHaveCSS('color', 'rgb(255, 255, 255)')
  await tabs.nth(1).click()
  await tabs.nth(1).press('ArrowUp')
  await expect(tabs.first()).toBeFocused()
  await expect(tabs.first()).toHaveAttribute('aria-selected', 'true')
  await first.evaluate(element => element.scrollIntoView({block:'start'}))
  await first.click()
  // All supported brand presets use the shared primary button foreground.
  for (const primary of ['#20594c', '#274c75', '#694477']) {
   await page.evaluate(color => document.documentElement.style.setProperty('--primary', color), primary)
   await expect(first.locator('.configured-option-price')).toHaveCSS('color', 'rgb(255, 255, 255)')
   await expect(selection.locator('.configured-options button').nth(1).locator('.configured-option-price')).toHaveCSS('color', 'rgb(185, 28, 28)')
  }
  await page.evaluate(() => document.documentElement.style.removeProperty('--primary'))
  await expectNoPageOverflow(page)
  if (info.project.name.endsWith('reflow')) {
   for (const width of [320, 375, 768, 999, 1000, 1001, 1440]) {
    await page.setViewportSize({width, height: 720})
    await expectNoPageOverflow(page)
    await expect(betButton).toBeInViewport()
    const left = await selection.getByRole('tablist').boundingBox()
    const right = await selection.getByRole('tabpanel').boundingBox()
    expect(left!.x + left!.width).toBeLessThanOrEqual(right!.x)
   }
  }
  await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight))
  await expect(betButton).toBeInViewport()
  const lastBox = await selection.locator('.configured-options button').last().boundingBox()
  const dockBox = await dock.boundingBox()
  expect(lastBox!.y + lastBox!.height).toBeLessThanOrEqual(dockBox!.y)
  await page.evaluate(() => window.scrollTo(0, 0))
  expect((await first.boundingBox())!.width).toBeGreaterThanOrEqual(64)
  await page.screenshot({path: info.outputPath(`selection-${locale}.png`), fullPage: true})
  const initialReads = reads, initialPeriodReads = periodReads
  const before = await strip.locator('.period-countdown').textContent()
  await page.clock.fastForward(1000)
  await expect(strip.locator('.period-countdown')).not.toHaveText(before!)
  await page.clock.fastForward(59000)
  expect(reads).toBe(initialReads)
  expect(orderReads).toBe(0)
  expect(periodReads).toBe(initialPeriodReads)
  closed = true
  await page.clock.fastForward(3541000)
  await expect.poll(() => reads).toBe(initialReads + 1)
  await expect(first).toBeDisabled()
  await expect(strip.locator('.period-countdown')).toHaveAttribute('aria-label', locale === 'en' ? 'Draws in' : '开奖倒计时')
  await page.clock.fastForward(60000)
  await expect.poll(() => reads).toBe(initialReads + 2)
 })
}
}
