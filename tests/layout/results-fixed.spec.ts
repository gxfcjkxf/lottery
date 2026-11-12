import { expectRealEntry, expectListLayout } from './real-page-review'
import { test, expect } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { configuredDemo } from '../../shared/src/configured-rules'
import { installLayoutApi, installOrdersLayoutApi, installWalletLayoutApi, expectNoPageOverflow, origins, gameId } from './fixture'

for (const locale of ['zh-CN', 'en'] as const) {
  test(`results controls stay visible while records scroll (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    await installOrdersLayoutApi(page)
    let archiveReads=0
    await page.route('**/api/v1/draw-archive?*', route => {
      archiveReads++
      const url = new URL(route.request().url()), offset = Number(url.searchParams.get('offset'))
      return route.fulfill({ json: { success: true, data: {
        game_id: url.searchParams.get('game_id'), game_name: 'Mark Six', mark_six: true,
        timezone: 'Asia/Singapore', daily: false, choices: ['2026', '2025'], selected: '2026', has_more: offset === 0,
        items: Array.from({ length: 50 }, (_, index) => ({
          period_no: String(2026050 - offset - index), draw_date: '2026-10-27', drawn_at: null,
          result: { regular: [1, 2, 3, 4, 5, 6], special: [7], digits: [] }, regular_order: [1, 2, 3, 4, 5, 6],
          attributes: Object.fromEntries([1, 2, 3, 4, 5, 6, 7].map(number => [String(number).padStart(2, '0'), { wave: ['red','blue','green'][(number-1)%3], zodiac: number<=2?'rat':number===7?'pig':'dragon', element: number<=2?'water':number===7?'metal':'wood' }])),
        })), total_count: '100',
      } } })
    })
    await page.goto(`${origins.user}/results?game_id=${gameId}`)
    const list = page.locator('.archive-list'), toolbar = page.locator('.archive-toolbar'), pagination = page.locator('.draw-archive .page-actions')
    const first = list.locator('.archive-row').first(), headers = list.locator('thead')
    const attributes = page.getByRole('radiogroup', {name: locale==='en'?'Attribute':'属性', exact:true})
    await expect(list.locator('.archive-row')).toHaveCount(50)
    const yearSelect=toolbar.locator('.archive-filter [role=combobox]')
    await yearSelect.click()
    await expect(yearSelect).toHaveAttribute('aria-expanded','true')
    await yearSelect.click()
    await expect(yearSelect).toHaveAttribute('aria-expanded','false')
    if(info.project.use.hasTouch){
      await yearSelect.tap()
      await expect(yearSelect).toHaveAttribute('aria-expanded','true')
      await yearSelect.tap()
      await expect(yearSelect).toHaveAttribute('aria-expanded','false')
    }
    await toolbar.locator('.archive-filter>span').first().click()
    await expect(yearSelect).toHaveAttribute('aria-expanded','false')
    await toolbar.click({position:{x:(await toolbar.boundingBox())!.width-2,y:2}})
    await expect(yearSelect).toHaveAttribute('aria-expanded','false')

    await expect(attributes.getByRole('radio')).toHaveCount(3)
    await expect(attributes.locator('input[value=zodiac]')).toBeChecked()
    await expect(toolbar.getByRole('combobox')).toHaveCount(1)
    await expect(first.locator('.archive-number-attribute')).toHaveText(locale==='en'?['Rat','Rat','Dragon','Dragon','Dragon','Dragon','Pig']:['鼠','鼠','龙','龙','龙','龙','猪'])
    await expect(headers.locator('th[scope=colgroup]')).toHaveCount(0)
    await expect(page.locator('.archive-summary-options input:checked')).toHaveCount(0)
    await expect(first.locator('[data-summary]')).toHaveCount(0)
    const conditions=page.getByRole('group',{name:locale==='en'?'Conditions':'条件',exact:true})
    await expect(conditions.getByRole('checkbox')).toHaveCount(12)
    await page.locator('input[name=archive-condition-rat]').check()
    await expect(first.locator('.archive-number.is-dimmed')).toHaveCount(5)
    await expect(first.locator('.archive-number.is-dimmed').first()).toHaveCSS('opacity','0.2')
    await expect(first.locator('.archive-ball')).toHaveCount(7)
    await page.locator('input[name=archive-condition-pig]').check()
    await expect(first.locator('.archive-number.is-dimmed')).toHaveCount(4)
    await expect(first.locator('.archive-special-cell .archive-number')).not.toHaveClass(/is-dimmed/)
    await page.locator('input[name=archive-condition-rat]').uncheck()
    await expect(first.locator('.archive-number.is-dimmed')).toHaveCount(6)
    await page.locator('input[name=archive-condition-pig]').uncheck()
    await expect(first.locator('.archive-number.is-dimmed')).toHaveCount(0)
    await attributes.locator('label').filter({has:page.locator('input[value=element]')}).click()
    await expect(conditions.getByRole('checkbox')).toHaveCount(5)
    await page.locator('input[name=archive-condition-water]').check()
    await page.locator('input[name=archive-condition-metal]').check()
    await expect(first.locator('.archive-number.is-dimmed')).toHaveCount(4)
    await page.screenshot({path:info.outputPath(`draw-conditions-${locale}.png`)})
    await attributes.locator('label').filter({has:page.locator('input[value=animal]')}).click()
    await expect(conditions.getByRole('checkbox')).toHaveCount(2)
    await expect(conditions.locator('input:checked')).toHaveCount(0)
    await expect(first.locator('.archive-number.is-dimmed')).toHaveCount(0)
    await page.locator('input[name=archive-condition-wild]').check()
    await expect(first.locator('.archive-number.is-dimmed')).toHaveCount(1)
    await expect(first.locator('.archive-special-cell .archive-number')).toHaveClass(/is-dimmed/)
    await attributes.locator('label').filter({has:page.locator('input[value=zodiac]')}).click()
    await expect(conditions.locator('input:checked')).toHaveCount(0)
    await expect(first.locator('.archive-number.is-dimmed')).toHaveCount(0)
    expect(archiveReads).toBe(1)
    await toolbar.evaluate(node=>{node.scrollTop=0})
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320,390,699,700,999,1000,1001,1440,1920] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({width,height:800})
      await list.evaluate(node=>{node.scrollTop=0})
      await toolbar.evaluate(node=>{node.scrollTop=0})
      const top = (await toolbar.boundingBox())!, bottom = (await pagination.boundingBox())!, content = (await list.boundingBox())!
      expect(content.height).toBeGreaterThanOrEqual(80)
      expect(content.y).toBeGreaterThanOrEqual(top.y+top.height)
      expect(content.y+content.height).toBeLessThanOrEqual(bottom.y+1)
      const headTop=(await headers.boundingBox())!.y
      const headingTops=await headers.locator('tr:visible th').evaluateAll(cells=>cells.map(cell=>cell.getBoundingClientRect().top))
      expect(await headers.locator('th').evaluateAll(cells=>cells.every(cell=>cell.scrollHeight<=cell.clientHeight+1))).toBe(true)
      const geometry=await first.evaluate(row=>{
        const bounds=row.getBoundingClientRect()
        const balls=[...row.querySelectorAll('.archive-ball')].map(ball=>ball.getBoundingClientRect())
        return {fits:balls.every(ball=>ball.left>=bounds.left&&ball.right<=bounds.right),labels:[...row.querySelectorAll('.archive-stat-cell')].map(cell=>cell.textContent!.trim()),fontSize:getComputedStyle(row.querySelector('.archive-ball')!).fontSize}
      })
      expect(geometry.fits).toBe(true)
      expect(geometry.fontSize).toBe('14px')
      const numberSpacing=await first.evaluate(row=>{
        const numbers=[...row.querySelectorAll('.archive-number')].map(node=>node.getBoundingClientRect())
        const centers=numbers.map(bounds=>bounds.left+bounds.width/2)
        const gaps=centers.slice(1).map((center,index)=>center-centers[index]!)
        const plus=row.querySelector('.archive-plus')!.getBoundingClientRect()
        return {gaps,plusCenter:plus.left+plus.width/2,regularEnd:numbers[5]!.right,specialStart:numbers[6]!.left}
      })
      expect(Math.max(...numberSpacing.gaps)-Math.min(...numberSpacing.gaps)).toBeLessThan(18)
      expect(numberSpacing.plusCenter).toBeGreaterThan(numberSpacing.regularEnd)
      expect(numberSpacing.plusCenter).toBeLessThan(numberSpacing.specialStart)
      const spacing=await toolbar.evaluate(node=>{const style=getComputedStyle(node);return {top:style.paddingTop,bottom:style.paddingBottom,marginTop:style.marginTop,marginBottom:style.marginBottom,rowGap:style.rowGap}})
      expect(spacing).toEqual({top:'8px',bottom:'4px',marginTop:'0px',marginBottom:'0px',rowGap:'4px'})
      if(width<1000){
        const controls=await toolbar.evaluate(node=>{
          const year=node.querySelector('.archive-filter')!.getBoundingClientRect(),attribute=node.querySelector('.archive-attributes')!.getBoundingClientRect(),sort=node.querySelector('.archive-sorting')!.getBoundingClientRect(),conditions=node.querySelector('.archive-condition-options')!.getBoundingClientRect()
          return {yearBottom:year.bottom,yearInputLeft:node.querySelector('.archive-filter .select-control-trigger')!.getBoundingClientRect().left,attributeInputLeft:node.querySelector('.archive-attribute-option')!.getBoundingClientRect().left,attributeTop:attribute.top,attributeBottom:attribute.bottom,sortTop:sort.top,sortBottom:sort.bottom,conditionsTop:conditions.top,buttons:[...node.querySelectorAll('.archive-attribute-option>span')].map(button=>button.getBoundingClientRect().height),choices:[...node.querySelectorAll('.archive-condition-options label')].map(label=>label.getBoundingClientRect().height),height:node.scrollHeight}
        })
        expect(controls.attributeInputLeft).toBeGreaterThan(controls.yearInputLeft)
        expect(controls.attributeTop).toBeLessThan(controls.yearBottom)
        expect(controls.sortTop-Math.max(controls.attributeBottom,controls.yearBottom)).toBeCloseTo(4,0)
        expect(controls.conditionsTop-controls.sortBottom).toBeCloseTo(4,0)
        for(const height of controls.buttons)expect(height).toBe(28)
        for(const height of controls.choices)expect(height).toBe(24)
        if(locale==='zh-CN')expect(controls.height).toBeLessThanOrEqual(240)

        const stacked=await first.locator('.archive-number').evaluateAll(numbers=>numbers.every(number=>{const ball=number.querySelector('.archive-ball')!.getBoundingClientRect(),attribute=number.querySelector('.archive-number-attribute')!.getBoundingClientRect();return attribute.top>=ball.bottom&&Math.abs(attribute.left+attribute.width/2-ball.left-ball.width/2)<1}))
        expect(stacked).toBe(true)
        const plusAlignment=await first.evaluate(row=>{const plus=row.querySelector('.archive-plus')!.getBoundingClientRect(),ball=row.querySelector('.archive-special-cell .archive-ball')!.getBoundingClientRect();return Math.abs(plus.top+plus.height/2-ball.top-ball.height/2)})
        expect(plusAlignment).toBeLessThan(1)
        expect(await first.locator('.archive-ball').evaluateAll(balls=>balls.every(ball=>Math.abs(ball.getBoundingClientRect().top-balls[0]!.getBoundingClientRect().top)<1))).toBe(true)
      }else{
        const specialLayout=await first.evaluate(row=>{
          const cell=row.querySelector('.archive-special-cell')!,group=cell.querySelector('.archive-numbers')!,number=cell.querySelector('.archive-number')!,separator=row.querySelector('.archive-separator-cell')!
          const box=cell.getBoundingClientRect(),ball=number.getBoundingClientRect()
          return {center:box.left+box.width/2,numberCenter:ball.left+ball.width/2,alignment:getComputedStyle(group).justifyContent,separatorWidth:separator.getBoundingClientRect().width,header:document.querySelector('thead .archive-special-cell')!.getBoundingClientRect().right,cellRight:box.right}
        })
        expect(specialLayout.numberCenter).toBeCloseTo(specialLayout.center,0)
        expect(specialLayout.alignment).toBe('center')
        expect(specialLayout.separatorWidth).toBeLessThanOrEqual(10)
        expect(specialLayout.header).toBeCloseTo(specialLayout.cellRight,0)
        await expect(headers.locator('.archive-separator-cell')).toHaveText('')
      }

      await list.evaluate(node=>{node.scrollTop=node.scrollHeight})
      expect((await headers.boundingBox())!.y).toBeLessThan(headTop)
      const scrolledTops=await headers.locator('tr:visible th').evaluateAll(cells=>cells.map(cell=>cell.getBoundingClientRect().top))
      for(let index=0;index<headingTops.length;index++)expect(scrolledTops[index]!-headingTops[index]!).toBeCloseTo(scrolledTops[0]!-headingTops[0]!,0)
      await expect(list.locator('.archive-row').last()).toBeInViewport()
      expect((await toolbar.boundingBox())!.y).toBe(top.y)
      expect((await pagination.boundingBox())!.y).toBe(bottom.y)
      await expectRealEntry(page,'user');await expectListLayout(page);await expectNoPageOverflow(page)
      await list.evaluate(node=>{node.scrollTop=0})
      await page.screenshot({path:info.outputPath(`draw-table-${locale}-${width}.png`)})
      await toolbar.screenshot({path:info.outputPath(`draw-options-${locale}-${width}.png`)})
    }
    await expect(first.locator('.archive-time')).toHaveText('2026-10-27')
    await expect(first.locator('.archive-period')).toHaveText(locale==='en'?'050':'050期')
    await attributes.locator('input[value=zodiac]').focus()
    await page.keyboard.press('ArrowRight')
    await expect(attributes.locator('input[value=element]')).toBeChecked()
    await expect(first.locator('.archive-number-attribute')).toHaveText(locale==='en'?['Water','Water','Wood','Wood','Wood','Wood','Metal']:['水','水','木','木','木','木','金'])
    await expect(first.locator('.archive-ball').first()).toHaveCSS('background-image','none')
    await expect(first.locator('.archive-ball').first()).toHaveCSS('color','rgb(255, 255, 255)')
    const allKeys=['total','totalParity','totalSize','totalWave','specialParity','specialSize']
    for(const key of allKeys)await page.locator(`input[name=archive-summary-${key}]`).uncheck()
    await expect(first.locator('[data-summary]')).toHaveCount(0)
    await expect(headers.locator('th[scope=colgroup]')).toHaveCount(0)
    await page.locator('input[name=archive-summary-specialSize]').check()
    await expect(first.locator('[data-summary]')).toHaveText([locale==='en'?'Small':'小'])
    await expect(headers.locator('th[scope=colgroup]')).toHaveText([locale==='en'?'Special':'特码'])
    await expect(headers.locator('[data-summary-header]')).toHaveText([locale==='en'?'Big/\u200bsmall':'大小'])
    await page.locator('input[name=archive-summary-total]').check()
    await expect(first.locator('[data-summary]')).toHaveText(['28',locale==='en'?'Small':'小'])
    for(const key of allKeys)await page.locator(`input[name=archive-summary-${key}]`).check()
    await expect(first.locator('[data-summary]')).toHaveText(locale==='en'?['28','Even','Small','Red','Odd','Small']:['28','双','小','红','单','小'])
    const presetWithColumns=page.viewportSize()!
    for(const width of info.project.name.includes('reflow')?[320,390,999,1000,1440,1920]:[presetWithColumns.width]){
      if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:800})
      await list.evaluate(node=>{node.scrollTop=0;node.scrollLeft=0})
      await toolbar.evaluate(node=>{node.scrollTop=0})
      const toolbarBefore=(await toolbar.boundingBox())!,paginationBefore=(await pagination.boundingBox())!
      const record=await first.evaluate(row=>{
        const cells=[...row.querySelectorAll('td')].map(cell=>cell.getBoundingClientRect())
        const balls=[...row.querySelectorAll('.archive-ball')].map(ball=>ball.getBoundingClientRect())
        return {oneRow:cells.every(cell=>Math.abs(cell.top-cells[0]!.top)<1),ballsAligned:balls.every(ball=>Math.abs(ball.top-balls[0]!.top)<1)}
      })
      expect(record.oneRow).toBe(true)
      if(width<1000)expect(record.ballsAligned).toBe(true)
      if(width<1000){
        const activeHeaders=headers.locator('tr:visible th')
        await expect(activeHeaders).toHaveCount(12)
        await expect(headers.locator('th[scope=colgroup]')).toHaveText(locale==='en'?['Total','Special']:['总和','特码'])
        await expect(headers.locator('[data-summary-header]')).toHaveText(locale==='en'?['Total','Odd/\u200beven','Big/\u200bsmall','Wave','Odd/\u200beven','Big/\u200bsmall']:['总数','单双','大小','七色波','单双','大小'])
        expect(await activeHeaders.evaluateAll(cells=>cells.every(cell=>getComputedStyle(cell).whiteSpace==='nowrap'))).toBe(true)
        expect(await headers.locator('tr:visible').count()).toBe(2)
        const size=await list.evaluate(node=>({width:node.clientWidth,scrollWidth:node.scrollWidth}))
        if(width<=430)expect(size.scrollWidth).toBeGreaterThan(size.width)
        await page.screenshot({path:info.outputPath(`draw-horizontal-start-${locale}-${width}.png`)})
        if(size.scrollWidth>size.width){
          await list.evaluate(node=>{node.scrollLeft=node.scrollWidth})
          expect(await list.evaluate(node=>node.scrollLeft)).toBeGreaterThan(0)
          await expect(headers.locator('[data-summary-header=specialSize]')).toBeInViewport()
          await expect(first.locator('[data-summary=specialSize]')).toBeInViewport()
          const alignment=await page.evaluate(()=>{const header=document.querySelector('[data-summary-header=specialSize]')!.getBoundingClientRect(),cell=document.querySelector('.archive-row [data-summary=specialSize]')!.getBoundingClientRect();return Math.abs(header.left-cell.left)})
          expect(alignment).toBeLessThan(1)
          expect((await toolbar.boundingBox())!.x).toBe(toolbarBefore.x)
          expect((await pagination.boundingBox())!.x).toBe(paginationBefore.x)
          await page.screenshot({path:info.outputPath(`draw-horizontal-end-${locale}-${width}.png`)})
        }
      }else await expect(headers.locator('th[scope=colgroup]')).toHaveText(locale==='en'?['Total','Special']:['总和','特码'])
      await expectNoPageOverflow(page)
    }
    if(info.project.name.includes('reflow'))await page.setViewportSize(presetWithColumns)
    await list.evaluate(node=>{node.scrollTop=0;node.scrollLeft=0})

    await attributes.locator('label').filter({has:page.locator('input[value=animal]')}).click()
    await expect(first.locator('.archive-number-attribute')).toHaveText(locale==='en'?['Wild','Wild','Wild','Wild','Wild','Wild','Farm']:['野兽','野兽','野兽','野兽','野兽','野兽','家禽'])
    await toolbar.evaluate(node=>{node.scrollTop=0})
    await expectNoPageOverflow(page)
    await page.screenshot({path:info.outputPath(`draw-table-attributes-${locale}.png`)})
    await pagination.getByRole('button',{name:locale==='en'?'Next':'下一页',exact:true}).click()
    await expect(list.locator('.archive-period').first()).toHaveText(locale==='en'?'000':'000期')
  })
}

for (const locale of ['zh-CN', 'en'] as const) {
  test(`mobile period links to the current game draw history (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    await installOrdersLayoutApi(page)
    await installWalletLayoutApi(page)
    const snapshot = configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json', 'utf8')))
    await page.route(`**/api/v1/configured/games/${gameId}`, route => {
      const now = Date.now()
      return route.fulfill({ json: { success: true, data: {
        game: { id: gameId, name: 'Mark Six', status: 'active', brand_enabled: true }, snapshot,
        rule_id: gameId, snapshot_hash: 'draw-history-link', sales: { version: 1, enabled: true },
        previous_draw: { period_no: '2026305', regular_order: [31,49,29,44,18,40],
          result: { regular: [31,49,29,44,18,40], special: [26], digits: [] },
          attributes: Object.fromEntries([31,49,29,44,18,40,26].map((number, index) => [String(number).padStart(2,'0'), {wave:['blue','green','red'][index % 3],zodiac:'rat',element:'water'}])) },
        period: { id: gameId, period_no: '2026306', bet_start_at: new Date(now-3600000).toISOString(), bet_end_at: new Date(now+3600000).toISOString(), draw_at: new Date(now+3660000).toISOString() },
      } } })
    })
    await page.goto(`${origins.user}/configured/${gameId}`)
    const strip = page.locator('.user-period-strip'), link = strip.getByRole('link', {name: locale === 'en' ? 'Draw history' : '开奖记录', exact: true})
    await expect(strip.locator('.result-heading strong')).toContainText('2026305')
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320,620,999,1000,1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({width, height:800})
      await expect(link).toBeVisible({visible: width < 1000})
      if (width < 1000) {
        await expect(link).toHaveAttribute('href', `/results?platform_game_id=${gameId}`)
        const geometry = await link.evaluate(node => {
          const bounds = node.getBoundingClientRect(), heading = node.previousElementSibling!.getBoundingClientRect()
          const probe = document.createElement('span'); probe.style.color = 'var(--danger)'; node.append(probe)
          const red = getComputedStyle(probe).color; probe.remove()
          return {rightOfPeriod:bounds.left >= heading.right, aligned:heading.top >= bounds.top && heading.bottom <= bounds.bottom, height:bounds.height, color:getComputedStyle(node).color, red}
        })
        expect(geometry.rightOfPeriod).toBe(true); expect(geometry.aligned).toBe(true)
        expect(geometry.height).toBeGreaterThanOrEqual(44); expect(geometry.color).toBe(geometry.red)
      }
      await expectNoPageOverflow(page)
      await strip.screenshot({path:info.outputPath(`period-history-${locale}-${width}.png`)})
    }
    if (info.project.name.includes('reflow')) await page.setViewportSize({width:320,height:568})
    await link.click()
    await expect(page).toHaveURL(`${origins.user}/results?platform_game_id=${gameId}`)
    await expect(page.locator('.draw-archive')).toBeVisible()
    await expect(page.locator('h1')).toHaveText(locale === 'en' ? 'Draw history' : '开奖记录')
  })
}

for(const locale of ['en','zh-CN'] as const)test(`mobile user pages keep compact vertical spacing (${locale})`,async({page},info)=>{
  await installLayoutApi(page,locale);await installWalletLayoutApi(page)
  const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
  const pages=[['/account','.account-page'],['/help','.faq-list'],['/wallet','.wallet-summary'],['/wallet/buckets','.bucket-grid'],['/wallet/ledger','.ledger-entry'],['/notifications','.notifications-panel']] as const
  const preset=page.viewportSize()!
  for(const [path,selector] of pages){
    await page.goto(origins.user+path);await expect(page.locator(selector)).toBeVisible()
    for(const width of info.project.name.includes('reflow')?[320,700,1440]:[preset.width]){
      if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:800})
      await expectNoPageOverflow(page)
      const compact=await page.evaluate(()=>matchMedia('(max-width:900px), (pointer:coarse)').matches)
      if(compact){
        const metrics=await page.locator('main.content').evaluate(main=>[...main.querySelectorAll('.detail-panel,.wallet-summary .panel:not(.user-record-list),.state-card,.faq-list details,.account-tools a,.notification-summary')].filter(node=>node.getBoundingClientRect().height>0).map(node=>({top:parseFloat(getComputedStyle(node).paddingTop),bottom:parseFloat(getComputedStyle(node).paddingBottom)})))
        for(const metric of metrics){expect(metric.top).toBeLessThanOrEqual(8);expect(metric.bottom).toBeLessThanOrEqual(8)}
        for(const summary of await page.locator('.faq-list summary').all())expect((await summary.boundingBox())!.height).toBeGreaterThanOrEqual(44)
      }
      if(width===320||!info.project.name.includes('reflow'))await page.screenshot({path:info.outputPath(`mobile-spacing-${path.replaceAll('/','-')}-${locale}-${width}.png`)})
    }
  }
  await page.route('**/api/v1/me',route=>route.fulfill({status:401,json:{success:false,error:{code:'UNAUTHORIZED',message:'Sign in required'}}}))
  for(const path of ['/login','/register']){
    await page.goto(origins.user+path);await expect(page.locator('.auth-form')).toBeVisible()
    if(info.project.name.includes('reflow'))await page.setViewportSize({width:320,height:568})
    await expectNoPageOverflow(page)
    const compact=await page.evaluate(()=>matchMedia('(max-width:900px), (pointer:coarse)').matches)
    if(compact){
      await expect(page.locator('.login-entry__card form')).toHaveCSS('row-gap','8px')
      await expect(page.locator('.login-entry__card form')).toHaveCSS('margin-top','8px')
      const buttons=await page.locator('.auth-actions>.button').evaluateAll(buttons=>buttons.map(button=>({top:button.getBoundingClientRect().top,height:button.getBoundingClientRect().height})))
      expect(buttons[0]!.top).toBeCloseTo(buttons[1]!.top,0);expect(buttons[0]!.height).toBeCloseTo(buttons[1]!.height,0)
      for(const input of await page.locator('.auth-form input:not([type=checkbox],[type=hidden],[type=radio])').all())if(await input.isVisible())expect((await input.boundingBox())!.height).toBeGreaterThanOrEqual(44)
    }
    await page.screenshot({path:info.outputPath(`mobile-spacing-${path.slice(1)}-${locale}.png`)})
  }
  expect(errors).toEqual([])
})
