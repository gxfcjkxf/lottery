import { test, expect } from '@playwright/test'
import {resolve} from 'node:path'
import {origins} from './fixture'
for (const client of ['user','brand','platform','agent'] as const) test(`shared main workspace ${client}`,async({page},info)=>{
 const app=client==='brand'?'admin':client
 const source=await(await page.request.get(origins[client]+'/src/App.vue')).text()
 const vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
 const url=origins[client]+'/__theme-workspace'
 await page.route(url,route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs${resolve('tests/layout/ThemeWorkspaceHarness.vue')}';import{applyTheme}from'/@fs${resolve('shared/src/theme.ts')}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve(app==='agent'?'shared/src/styles.css':app+'-web/src/style.css')}';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/themes.css')}';applyTheme('obsidian');window.applyTheme=applyTheme;createApp(Harness,{client:'${client}'}).mount('#app')</script></html>`}))
 await page.goto(url)
 await page.addStyleTag({content:'*,*::before,*::after{transition:none!important;animation:none!important}'})
 await page.getByRole('textbox',{name:'账号',exact:true}).fill('review_member')
 await expect(page.getByRole('button',{name:'日间模式',exact:true})).toBeVisible()
 for(const theme of ['obsidian','jade','vermilion','ocean']) {
  await page.evaluate(id=>(window as any).applyTheme(id),theme)
  for(const mode of ['light','dark']) {
   if(mode==='dark') await page.getByRole('button',{name:'日间模式',exact:true}).click()
   await expect(page.locator('html')).toHaveAttribute('data-user-theme',mode)
   const preset=page.viewportSize()!
   for(const width of info.project.name.includes('reflow')?[320,768,1280]:[preset.width]) {
    if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
    const sizes = await page.evaluate(() => {
      const size = (selector: string) => { const el = document.querySelector(selector)!; const box = el.getBoundingClientRect(); const icon = el.querySelector('svg')!.getBoundingClientRect(); return { width: box.width, height: box.height, iconWidth: icon.width, iconHeight: icon.height, padding: getComputedStyle(el).padding }; };
      const group = document.querySelector('.top-actions')!;
      const buttons = ['.theme-switcher', '.locale-picker', '.header-menu', '.account-menu'].flatMap(selector => { const el = group.querySelector(selector); return el ? [el.getBoundingClientRect()] : []; });
      return { theme: size('.theme-switcher'), language: size('.locale-picker__trigger'), menu: size('.header-menu'), account: group.querySelector('.account-trigger') ? size('.account-trigger') : null, gap: getComputedStyle(group).gap, gaps: buttons.slice(1).map((box, index) => box.left - buttons[index].right), control: getComputedStyle(group).getPropertyValue('--control-height').trim() };

    });
    await expect(page.locator('.theme-switcher')).toHaveCSS('appearance','none');
    await expect(page.locator('.header-menu')).toHaveCSS('appearance','none');
    expect(sizes.theme).toEqual(sizes.language);
    expect(sizes.theme).toEqual(sizes.menu);
    expect(sizes.theme.iconWidth).toBe(20);
    expect(sizes.theme.iconHeight).toBe(20);
    expect(sizes.theme.width).toBe(parseFloat(sizes.control));
    if (sizes.account) {
      expect(sizes.account.height).toBe(sizes.theme.height);
      expect(sizes.account.iconWidth).toBe(20);
    }
    expect(sizes.gap).toBe(sizes.control === '44px' ? '4px' : '8px');
    for (const gap of sizes.gaps) expect(gap).toBeCloseTo(parseFloat(sizes.gap), 1);
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
    const panelColors=await page.locator('.management-list').evaluate(el=>{
      const probe=document.createElement('span');el.append(probe);probe.style.color='var(--line)';probe.style.backgroundColor='var(--surface)'
      const expected=getComputedStyle(probe),panel=getComputedStyle(el)
      const result={border:panel.borderTopColor,line:expected.color,surface:expected.backgroundColor,background:panel.backgroundColor,overflow:panel.overflow,radius:panel.borderTopLeftRadius,toolbar:getComputedStyle(el.querySelector('.management-list-toolbar')!).backgroundColor,footer:getComputedStyle(el.querySelector('.table-pagination')!).backgroundColor}
      probe.remove();return result
    })
    expect(panelColors.border).toBe(panelColors.line)
    expect(panelColors.background).toBe(panelColors.surface)
    expect(panelColors.toolbar).toBe(panelColors.surface)
    expect(panelColors.footer).toBe(panelColors.surface)
    if(client!=='user'){expect(panelColors.overflow).toBe('hidden');expect(panelColors.radius).toBe('8px')}

    const colors=await page.evaluate(()=>{const style=getComputedStyle(document.querySelector('.main-shell')!);return {bg:style.backgroundColor,ink:style.color}})
    expect(colors.bg).toBe(mode==='dark'?'rgb(9, 13, 22)':({obsidian:'rgb(240, 243, 241)',jade:'rgb(246, 247, 245)',vermilion:'rgb(250, 248, 245)',ocean:'rgb(244, 247, 250)'} as any)[theme])
    await page.screenshot({path:info.outputPath(`${client}-${theme}-${mode}-${width}.png`),fullPage:true})
   }
  }
  await page.getByRole('button',{name:'夜间模式',exact:true}).click()
  await expect(page.getByRole('textbox',{name:'账号',exact:true})).toHaveValue('review_member')
  await expect(page.getByRole('button',{name:'自动模式',exact:true})).toBeVisible()
  await page.emulateMedia({colorScheme:'dark'})
  await expect(page.locator('html')).toHaveAttribute('data-user-theme','dark')
  await page.emulateMedia({colorScheme:'light'})
  await expect(page.locator('html')).toHaveAttribute('data-user-theme','light')
  await page.getByRole('button',{name:'自动模式',exact:true}).click()
 }
})

import {installConsoleSession, openPlatformPage, openBrandPage} from './real-console-session'
import {gameId} from './fixture'
for(const client of ['brand','platform'] as const)test(`real main theme ${client}`,async({page},info)=>{
 await page.route('**/api/**',route=>{
  expect(route.request().method()).toBe('GET')
  const path=new URL(route.request().url()).pathname
  if(path.endsWith('/audit'))return route.fulfill({json:{success:true,data:{items:[],total_count:'0'}}})
  throw Error(`Unexpected read ${path}`)
 })
 await installConsoleSession(page,client,'zh-CN',gameId,client==='brand'?['audit.view.brand']:['platform_settings.view.platform','platform_settings.write.platform'],undefined,{theme:'obsidian'})
 await page.goto(origins[client])
 await expect(page.locator('.management-console')).toBeVisible()
 if(client==='platform')await openPlatformPage(page,'设置')
 else await openBrandPage(page,'审计日志')
 await expect(page.locator('html')).toHaveAttribute('data-theme','obsidian')
 await expect(page.locator('.theme-switcher')).toBeVisible()
 if(client==='platform') {
 await page.getByRole('button',{name:'编辑设置',exact:true}).click()
 const dialog=page.getByRole('dialog')
 await dialog.getByRole('combobox',{name:'主题',exact:true}).click()
 for(const name of ['青瓷白玉','珊瑚朱砂','海盐冰蓝','曜石翡翠']) await expect(page.getByRole('option',{name,exact:true})).toBeVisible()
 await page.keyboard.press('Escape')
 await dialog.getByRole('button',{name:'关闭',exact:true}).click()
 }
 await expect(page.locator('.topbar')).toHaveCSS('background-color','rgb(255, 255, 255)')
 const widths=info.project.name.includes('reflow')?[320,768,1280]:[page.viewportSize()!.width]
 for(const theme of ['jade','vermilion','ocean','obsidian'])for(const mode of ['light','dark']) {
 await page.evaluate(async({theme,mode,themePath})=>{localStorage.setItem('lottery.theme.mode',mode);const{applyTheme}=await import(themePath);applyTheme(theme)},{theme,mode,themePath:'/@fs'+resolve('shared/src/theme.ts')})
 if(client==='platform') {
 await expect(page.locator('.platform-settings-page > .panel').first()).toHaveCSS('background-color',mode==='dark'?'rgb(17, 25, 35)':'rgb(255, 255, 255)')
 await expect(page.locator('.platform-settings-page dd').first()).toHaveCSS('color',mode==='dark'?'rgb(229, 236, 233)':'rgb(17, 24, 39)')
 }
 for(const width of widths){
  if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:800})
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
  await page.screenshot({path:info.outputPath(`real-${client}-${theme}-${mode}-${width}.png`),fullPage:true})
 }
 }
})

import { readFileSync } from 'node:fs'
import { currencies } from '../../shared/src/cash'
import { configuredDemo } from '../../shared/src/configured-rules'
import { installLayoutApi, installWalletLayoutApi, expectNoPageOverflow } from './fixture'
for (const locale of ['en', 'zh-CN'] as const) test(`real player betting theme coverage ${locale}`, async ({page}, info) => {
 await installLayoutApi(page, locale)
 await installWalletLayoutApi(page)
 const errors:string[]=[]
 page.on('pageerror', error=>errors.push(error.message))
 const snapshot=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json','utf8')))
 const numbers=[6,9,25,27,33,22,48]
 await page.route(`**/api/v1/configured/games/${gameId}`, route=>{
  expect(route.request().method()).toBe('GET')
  const now=Date.now()
  return route.fulfill({json:{success:true,data:{game:{id:gameId,name:'Mark Six',status:'active',brand_enabled:true},snapshot,rule_id:gameId,snapshot_hash:'theme-review',sales:{version:1,enabled:true},previous_draw:{period_no:'2026315',regular_order:numbers.slice(0,6),result:{regular:numbers.slice(0,6),special:numbers.slice(6),digits:[]},attributes:Object.fromEntries(numbers.map((n,i)=>[String(n).padStart(2,'0'),{wave:i%2?'blue':'green',zodiac:'tiger',element:'wood'}]))},period:{id:gameId,period_no:'2026316',bet_start_at:new Date(now-3600000).toISOString(),bet_end_at:new Date(now+3600000).toISOString(),draw_at:new Date(now+3660000).toISOString()}}}})
 })
 await page.goto(origins.user)
 await page.addStyleTag({content:'*,*::before,*::after{transition:none!important;animation:none!important}'})
 await expect(page.locator('.configured-options button')).toHaveCount(49)
 await page.locator('.configured-play-tabs button').nth(1).click()
 const option=page.locator('.configured-options button').first()
 await option.click()
 await expect(option).toHaveAttribute('aria-pressed','true')
 const preset=page.viewportSize()!
 for(const theme of ['jade','vermilion','ocean','obsidian'])for(const mode of ['light','dark']) {
  await page.evaluate(async ({theme,mode,themePath})=>{
   localStorage.setItem('lottery.theme.mode',mode)
   const {applyTheme}=await import(themePath)
   applyTheme(theme)
  },{theme,mode,themePath: "/@fs"+resolve("shared/src/theme.ts")})
  const widths=info.project.name.endsWith('reflow')?[320,700,999,1000,1001,1440]:[preset.width]
  for(const width of widths){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   const colors=await page.evaluate(()=>{
    const color=(selector:string,property:string)=>getComputedStyle(document.querySelector(selector)!).getPropertyValue(property)
    const probe=document.createElement('span');document.querySelector('.workspace-frame')!.append(probe)
    const token=(name:string)=>{probe.style.color=`var(${name})`;return getComputedStyle(probe).color}
    const result={accent:token('--accent'),surface:token('--surface'),primary:token('--primary'),ink:token('--primary-dark'),light:token('--primary-light'),onPrimary:token('--on-primary'),strip:color('.user-period-strip','background-color'),art:color('.user-period-strip','background-image'),heading:color('.result-heading>strong','color'),tab:color('.configured-play-tabs button.active','background-color'),tabInk:color('.configured-play-tabs button.active','color'),selected:color('.configured-options button[aria-pressed="true"]','background-color'),selectedInk:color('.configured-options button[aria-pressed="true"]','color'),link:color('.configured-live a','color')}
    probe.remove();return result
   })
   expect(colors.strip).toBe(colors.surface);expect(colors.art).toBe('none')
   expect(colors.heading).toBe(colors.ink);expect(colors.link).toBe(colors.accent)
   expect(colors.tab).toBe(colors.light);expect(colors.tabInk).toBe(colors.ink)
   expect(colors.selected).toBe(colors.primary);expect(colors.selectedInk).toBe(colors.onPrimary)
   await expect(option).toHaveAttribute('aria-pressed','true')
   await expect(page.locator('.result-numbers .mark-six-ball')).toHaveCount(7)
   await expectNoPageOverflow(page)
   if(width===preset.width||width===320||width===1440)await page.screenshot({path:info.outputPath(`player-${locale}-${theme}-${mode}-${width}.png`)})
  }
 }
 expect(errors).toEqual([])
})

for (const locale of ['en','zh-CN'] as const) for(const path of ['/wallet','/login']) test(`real player account theme coverage ${locale} ${path}`, async({page},info)=>{
 await installLayoutApi(page,locale);await installWalletLayoutApi(page)
 await page.route('**/api/v1/brand-contacts',route=>route.fulfill({json:{success:true,data:{items:[{id:'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2',kind:'telegram',value:'@brand_support',image_url:null}],default_id:'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2'}}}))
 const cashConfig={version:1,platform_rules:currencies.map(currency=>({currency,rate:'7',deposit_min:'1',deposit_max:'10000',withdrawal_min:'1',withdrawal_max:'10000',fee:'0',mode:'count',threshold:'2',inherit:false,enabled:true})),rules:currencies.map(currency=>({currency,rate:'7',deposit_min:'1',deposit_max:'10000',withdrawal_min:'1',withdrawal_max:'10000',fee:'0',mode:'count',threshold:'2',inherit:false,enabled:true}))}
 await page.route('**/api/v1/cash**',route=>{expect(route.request().method()).toBe('GET');const path=new URL(route.request().url()).pathname;const data=path==='/api/v1/cash'?{auto_usdt:false,actor_context:'a'.repeat(64),brand_id:gameId,member_id:gameId,balances:currencies.map(currency=>({currency,deposited:'10',withdrawable:'0',reserved:'0'})),destinations:[],platform:cashConfig,brand:cashConfig}:{items:[],total_count:'0',limit:20,offset:0};return route.fulfill({json:{success:true,data}})})
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
  if(path==='/login')await page.route('**/api/v1/me',route=>route.fulfill({status:401,json:{success:false,error:{code:'UNAUTHORIZED',message:'Sign in required'}}}))
  await page.goto(origins.user+path)
  await page.addStyleTag({content:'*,*::before,*::after{transition:none!important;animation:none!important}'})
  await expect(page.locator(path==='/wallet'?'.total-primary':'.player-login-tabs a[aria-current="page"]')).toBeVisible()
  for(const theme of ['jade','vermilion','ocean','obsidian'])for(const mode of ['light','dark']){
   await page.evaluate(async({theme,mode,themePath})=>{localStorage.setItem('lottery.theme.mode',mode);const{applyTheme}=await import(themePath);applyTheme(theme)},{theme,mode,themePath:'/@fs'+resolve('shared/src/theme.ts')})
   const colors=await page.evaluate(path=>{
    const container=document.querySelector(path==='/wallet'?'.total-primary':'.login-entry')!
    const probe=document.createElement('span');container.append(probe)
    const token=(name:string)=>{probe.style.color=`var(${name})`;return getComputedStyle(probe).color}
    const primary=token('--primary'),light=token('--primary-light'),ink=token('--primary-dark'),onPrimary=token('--on-primary')
    const target=getComputedStyle(document.querySelector(path==='/wallet'?'.total-primary':'.player-login-tabs a[aria-current="page"]')!)
    const button=document.querySelector('.button-primary')
    const result={primary,light,ink,onPrimary,bg:target.backgroundColor,color:target.color,button:button?getComputedStyle(button).backgroundColor:null,buttonInk:button?getComputedStyle(button).color:null}
    probe.remove();return result
   },path)
   if(path==='/wallet'){expect(colors.bg).toBe(colors.light);expect(await page.locator('.cash-record-table .empty-state').evaluate(el=>getComputedStyle(el,'::before').backgroundImage)).toContain(mode==='dark'?'%23a0b0b9':'%2364748b');await expect(page.locator('.customer-service-button')).toHaveCSS('color',colors.onPrimary);await expect(page.locator('.customer-service-button')).toHaveCSS('background-color',colors.primary)}
   else {expect(colors.color).toBe(colors.primary);expect(colors.button).toBe(colors.primary);expect(colors.buttonInk).toBe(colors.onPrimary)}
   expect(await page.locator('html').evaluate(el=>getComputedStyle(el).getPropertyValue('--slider-chevron'))).toContain(mode==='dark'?"stroke='%23071510'":"stroke='white'")
   await expect(page.locator('.operation-toast-stack .toast-error')).toHaveCount(0)
   await expectNoPageOverflow(page)
   await page.screenshot({path:info.outputPath(`player-${path.slice(1)}-${locale}-${theme}-${mode}.png`)})
  }
 expect(errors).toEqual([])
})

for(const locale of ['en','zh-CN'] as const)test(`real player archive native theme ${locale}`,async({page},info)=>{
 await installLayoutApi(page,locale);await installWalletLayoutApi(page)
 await page.route('**/api/v1/draw-archive?*',route=>{
  expect(route.request().method()).toBe('GET')
  const url=new URL(route.request().url())
  return route.fulfill({json:{success:true,data:{game_id:url.searchParams.get('game_id'),game_name:'Mark Six',mark_six:true,timezone:'Asia/Singapore',daily:false,choices:['2026','2025'],selected:'2026',has_more:false,total_count:'30',items:Array.from({length:30},(_,i)=>({period_no:String(2026315-i),draw_date:'2026-11-12',drawn_at:null,result:{regular:[1,2,3,4,5,6],special:[7],digits:[]},regular_order:[1,2,3,4,5,6],attributes:Object.fromEntries([1,2,3,4,5,6,7].map(n=>[String(n).padStart(2,'0'),{wave:'red',zodiac:'rat',element:'wood'}]))}))}}})
 })
 await page.goto(`${origins.user}/results?game_id=${gameId}`)
 await page.addStyleTag({content:'*,*::before,*::after{transition:none!important;animation:none!important}'})
 const list=page.locator('.archive-list'),checkbox=page.locator('input[name=archive-condition-rat]'),radio=page.locator('.archive-sorting input[value=size]')
 await expect(list.locator('.archive-row')).toHaveCount(30)
 const preset=page.viewportSize()!
 for(const theme of ['jade','vermilion','ocean','obsidian'])for(const mode of ['light','dark']){
  await page.evaluate(async({theme,mode,themePath})=>{localStorage.setItem('lottery.theme.mode',mode);const{applyTheme}=await import(themePath);applyTheme(theme)},{theme,mode,themePath:'/@fs'+resolve('shared/src/theme.ts')})
  const widths=info.project.name.endsWith('reflow')?[320,700,1000,1440]:[preset.width]
  for(const width of widths){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   const colors=await list.evaluate(el=>{
    const checkbox=document.querySelector('.archive-condition-options input')!,radio=document.querySelector('.archive-sorting input')!,year=document.querySelector('.archive-filter [role=combobox]')!
    const probe=document.createElement('span');el.append(probe);probe.style.color='var(--primary)';probe.style.background='var(--surface)';probe.style.scrollbarColor='var(--scrollbar-thumb) var(--theme-canvas)'
    const resolved=getComputedStyle(probe)
    const data={primary:resolved.color,surface:resolved.backgroundColor,scrollbar:resolved.scrollbarColor,list:getComputedStyle(el).scrollbarColor,checkbox:getComputedStyle(checkbox).accentColor,radio:getComputedStyle(radio).accentColor,checkboxScheme:getComputedStyle(checkbox).colorScheme,radioScheme:getComputedStyle(radio).colorScheme,yearScheme:getComputedStyle(year).colorScheme,yearBackground:getComputedStyle(year).backgroundColor}
    probe.remove();return data
   })
   expect(colors.checkboxScheme).toBe(mode);expect(colors.radioScheme).toBe(mode);expect(colors.yearScheme).toBe(mode)
   expect(colors.checkbox).toBe(colors.primary);expect(colors.radio).toBe(colors.primary)
   expect(colors.yearBackground).toBe(colors.surface);expect(colors.list).toBe(colors.scrollbar);expect(colors.list).not.toBe('auto')
   await checkbox.check();await expect(checkbox).toBeChecked();await checkbox.uncheck()
   await radio.check();await expect(radio).toBeChecked()
   await list.evaluate(el=>{el.scrollTop=200})
   expect(await list.evaluate(el=>el.scrollTop)).toBeGreaterThan(0)
   await expectNoPageOverflow(page)
   if(width===preset.width||width===320||width===1440)await page.screenshot({path:info.outputPath(`archive-native-${locale}-${theme}-${mode}-${width}.png`)})
  }
 }
})

// Use real overview, member forms and role editors with read-only synthetic data.
for(const client of ['brand','platform'] as const)test(`console page palette coverage ${client}`,async({page},info)=>{
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
 const scope=client==='brand'?'brand':'platform'
 const permissions=['admin.view','admin.write','role.view','role.write'].map(p=>`${p}.${scope}`)
 await page.route('**/api/**',route=>{
  expect(route.request().method()).toBe('GET')
  const path=new URL(route.request().url()).pathname
  const ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(path.endsWith('permissions'))return ok({items:permissions})
  if(path.endsWith('roles'))return ok({items:[{id:gameId,brand_id:client==='brand'?gameId:'',code:'theme_review',name:'Theme review',status:'active',version:1,is_bootstrap:false,permissions}],total_count:'1'})
  if(path.endsWith('accounts'))return ok({items:[],total_count:'0'})
  throw Error(`Unexpected theme review request ${path}`)
 })
 const app=client==='brand'?'admin':'platform'
 const source=await(await page.request.get(origins[client]+'/src/App.vue')).text()
 const vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
 const url=origins[client]+'/__theme-surfaces'
 await page.route(url,route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs${resolve('tests/layout/ThemeSurfacesHarness.vue')}';import{applyTheme}from'/@fs${resolve('shared/src/theme.ts')}';${['shared/src/styles.css',app+'-web/src/style.css','shared/src/management.css','shared/src/controls.css','shared/src/themes.css'].map(path=>`import'/@fs${resolve(path)}';`).join('')}applyTheme('jade');window.applyTheme=applyTheme;createApp(Harness,{client:'${client}'}).mount('#app')</script></html>`}))
 await page.goto(url)
 await page.addStyleTag({content:'*,*::before,*::after{transition:none!important;animation:none!important}'})
 await expect(page.locator('.overview-stats')).toBeVisible()
 await expect(page.locator('.management-list')).toContainText('Theme review')
 const assertColor=async(selector:string,property:string,token:string)=>{
  const values=await page.locator(selector).first().evaluate((el,{property,token})=>{
   const probe=document.createElement('span');document.body.append(probe);probe.style.color=token.startsWith('--')?`var(${token})`:token
   const result={actual:getComputedStyle(el).getPropertyValue(property),expected:getComputedStyle(probe).color};probe.remove();return result
  },{property,token})
  expect(values.actual,`${client} ${selector} ${property}`).toBe(values.expected)
 }
 for(const theme of ['jade','vermilion','ocean','obsidian'])for(const mode of ['light','dark']){
  await page.evaluate(({theme,mode})=>{localStorage.setItem('lottery.theme.mode',mode);(window as any).applyTheme(theme)},{theme,mode})
  await assertColor('.overview-stats','background-color','--canvas')
  await expect(page.locator('.overview-stats')).toHaveCSS('background-image','none')
  await assertColor('.overview-stat','background-color','--surface')
  await assertColor('.overview-stat','border-top-color','--line')
  await assertColor('.overview-bar','background-color','color-mix(in srgb, var(--warning) 60%, var(--line))')
  if(client==='brand'){
   await assertColor('.profile-avatar','background-color','--primary-light')
   await assertColor('.profile-avatar','color','--primary-dark')
   await assertColor('.game-avatar.mint','background-color','--primary-light')
   await assertColor('.simulation-result','background-color','--surface')
   await assertColor('.sim-stats b','color','--ink')
   await assertColor('.success-check','background-color','--success-bg')
   await assertColor('.success-check','color','--success-text')
   await assertColor('.save-status','color','--success-text')
   await assertColor('.agent-alert','background-color','--warning-bg')
   await assertColor('.withdraw-note','color','--warning-text')
   await assertColor('.modal-hint','background-color','--warning-bg')
   await assertColor('.provision','color','--ink')
   await assertColor('.provision textarea','background-color','--surface')
   await assertColor('.provision label:has(textarea)','color','--ink-secondary')
   await page.locator('.provision textarea').focus()
   await assertColor('.provision textarea','background-color','--surface')
  }
  await page.getByRole('button',{name:'新建角色',exact:true}).click()
  const dialog=page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  await assertColor('dialog','background-color','--surface')
  await assertColor('dialog','border-top-color','--line')
  await assertColor('dialog .detail-modal-heading h2','color','--ink')
  await assertColor('dialog .permission-option','color','--ink')
  await assertColor('dialog input:not([type=checkbox])','color','--ink')
  await assertColor('dialog input:not([type=checkbox])','background-color','--surface')
  await assertColor('dialog .permission-group','background-color','--surface')
  await assertColor('dialog .permission-group','border-top-color','--line')
  const input=dialog.locator('input:not([type=checkbox])').first()
  await input.fill('theme_review');await input.focus()
  await assertColor('dialog input:not([type=checkbox])','background-color','--surface')
  await dialog.locator('.permission-group input').first().check()
  await expect(dialog.locator('.permission-group input').first()).toBeChecked()
  await assertColor('dialog .button-primary','background-color','--primary')
  await assertColor('dialog .button-primary','color','--on-primary')
  await assertColor('dialog .button-secondary','border-top-color','--line')
  if(info.project.name.includes('reflow'))await dialog.locator('.button-secondary').first().hover()
  const preset=page.viewportSize()!
  for(const width of info.project.name.includes('reflow')?[320,768,1280]:[preset.width]){
   if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
   await page.screenshot({path:info.outputPath(`roles-${client}-${theme}-${mode}-${width}.png`)})
  }
  await dialog.getByRole('button',{name:'关闭',exact:true}).click()
  await page.screenshot({path:info.outputPath(`overview-form-${client}-${theme}-${mode}.png`),fullPage:true})
 }
 expect(errors).toEqual([])
})
