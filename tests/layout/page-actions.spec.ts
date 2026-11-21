import {test,expect,type Locator} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,gameId,theme,expectNoPageOverflow} from './fixture'
import {installConsoleSession,openBrandPage,openPlatformPage} from './real-console-session'
import {currencies} from '../../shared/src/cash'
import {selectControl} from '../select-control'
const actor='bbbbbbbb-bbbb-4bbb-8bbb-000000000001',at='2026-10-21T00:00:00Z'
for(const locale of ['en','zh-CN'] as const)for(const view of ['presentation','withdrawal','archive','preferences','automation'] as const){
 test(`ordinary page actions align ${view} (${locale})`,async({page},info)=>{
  page.setDefaultTimeout(8000)
  const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/**',route=>{
   const path=new URL(route.request().url()).pathname
   expect(route.request().method()).toBe('GET')
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   const plan={source_type:'hk_marksix_6hch',version:1,enabled:true,anchor_draw_at:null,anchor_period_no:null,interval_seconds:null,close_before_seconds:30,source_url:'https://historical.example/feed',credential_ref:''}
   if(path.endsWith('/draw-sources'))return reply({items:[{id:actor,name:'Historical source',revision:1,plan}],active_source_id:actor,active_source_revision:1,version:1})
   if(path.endsWith('/automation'))return reply({plan,brand:null,allowed_origins:['https://historical.example'],attempts:[]})
   if(path.endsWith('/brand-presentation/defaults'))return reply(theme)
   if(path.endsWith('/brand-presentation'))return reply({brand_id:gameId,version:1,status:'active',base_name:theme.display_name,config:{...Object.fromEntries(Object.keys(theme).map(key=>[key,null])),},effective:{...theme,},updated_at:at,audit_log_id:gameId})
   if(path.endsWith('/filter-options'))return reply({brand_id:gameId,items:[{id:actor,name:'Trial game'}],limit:100,offset:0})
   if(path.endsWith('/games/'+actor+'/withdrawal-policy'))return reply({brand_id:gameId,game_id:actor,version:1,config:{turnover_multiple:null},effective:{turnover_multiple:'1',source:'brand',brand_version:1,game_version:1},updated_at:at})
   if(path.endsWith('/withdrawal-policy'))return reply({brand_id:gameId,version:1,points_step:'1',config:{enabled:false,min_points:'1',max_points:null,allowed_sources:['recharge','winning','gift'],review_mode:'manual',turnover_multiple:'1'},updated_at:at,audit_log_id:gameId})
   if(path.endsWith('/data-retention-policy'))return reply({version:1,enabled:false,execution_day:10,execution_time:'02:00',timezone:'Asia/Singapore',next_run:null,updated_at:at})
   if(path.endsWith('/data-retention-tasks'))return reply({items:[],total_count:'0',limit:50,offset:0})
   if(path.endsWith('/betting-preferences'))return reply({brand_id:gameId,member_id:actor,quick_amounts:['10','100']})
   throw Error('Unexpected page action read '+path)
  })
  const origin=view==='preferences'?origins.user:(view==='archive'||view==='automation')?origins.platform:origins.brand
  const app=view==='preferences'?'user-web':(view==='archive'||view==='automation')?'platform-web':'admin-web'
  await page.route('**/__page-actions',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import Harness from'/@fs${resolve('tests/layout/PageActionsHarness.vue')}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve(app+'/src/style.css')}';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/controls.css')}';createApp(Harness,${JSON.stringify({view,locale})}).mount('#app')</script></html>`}))
  await page.goto(origin+'/__page-actions')
  if(view==='preferences')await page.getByRole('button',{name:locale==='en'?'Personal settings':'个人设置',exact:true}).click()
  if(view==='archive')await page.getByRole('tab',{name:locale==='en'?'Archive settings':'归档设置'}).click()
  if(view==='automation')await page.getByRole('button',{name:locale==='en'?'Add data source':'新增数据源',exact:true}).click()
  const actions=page.locator(view==='presentation'?'.reset-settings':view==='withdrawal'?'.action-row':view==='archive'?'.retention-form > .ui-form-actions':view==='automation'?'.automation-settings .ui-form-actions':'.betting-preferences .ui-form-actions')
  await expect(actions.first()).toBeVisible()
  if(view==='withdrawal'){
   await selectControl(page.locator('#withdraw-game-select'),actor)
   await expect(actions).toHaveCount(2)
  }
  async function checkRight(row:Locator){
   const buttons=row.locator('button').filter({visible:true})
   await buttons.last().scrollIntoViewIfNeeded()
   await expect(buttons.last()).toBeInViewport()
   const geometry=await row.evaluate(el=>{
    const style=getComputedStyle(el),box=el.getBoundingClientRect()
    const buttons=[...el.querySelectorAll('button')].filter(b=>b.getBoundingClientRect().width>0).map(b=>b.getBoundingClientRect())
    return {right:box.right-parseFloat(style.paddingRight)-parseFloat(style.borderRightWidth),lastRight:Math.max(...buttons.map(b=>b.right)),rows:[...new Set(buttons.map(b=>Math.round(b.top)))].map(top=>Math.max(...buttons.filter(b=>Math.round(b.top)===top).map(b=>b.right)))}
   })
   expect(Math.abs(geometry.right-geometry.lastRight)).toBeLessThanOrEqual(2)
   for(const right of geometry.rows)expect(Math.abs(geometry.right-right)).toBeLessThanOrEqual(2)
  }
  if(view==='presentation'){
   await expect(page.locator('.theme-options')).toHaveCount(0)
   await page.locator('.advanced-settings summary').click()
   await expect(page.locator('input[type=color]')).toHaveCount(0)
   await expect(page.locator('.advanced-settings [role=combobox]')).toHaveCount(0)
   await expect(page.locator('.preview-card')).not.toHaveAttribute('style')
   await page.locator('.advanced-settings summary').click()
   await page.addStyleTag({path:resolve('shared/src/themes.css')})
   for(const palette of ['jade','vermilion','ocean','obsidian'])for(const mode of ['light','dark']){
    await page.evaluate(({palette,mode})=>{document.documentElement.dataset.theme=palette;document.documentElement.dataset.userTheme=mode},{palette,mode})
    for(const selector of ['.logo-preview','.preview-action']){
     const colors=await page.locator(selector).first().evaluate(el=>{
      const sample=document.createElement('span');sample.style.background='var(--primary)';sample.style.color='var(--on-primary)';document.body.append(sample)
      const expected=getComputedStyle(sample),actual=getComputedStyle(el)
      const result={background:actual.backgroundColor,color:actual.color,expectedBackground:expected.backgroundColor,expectedColor:expected.color};sample.remove();return result
     })
     expect(colors.background).toBe(colors.expectedBackground)
     expect(colors.color).toBe(colors.expectedColor)
    }
   }
   await page.evaluate(()=>{document.documentElement.dataset.theme='ocean'})
  }
  const preset=page.viewportSize()!
  for(const width of info.project.name.endsWith('reflow')?[320,640,900,1440]:[preset.width]){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   for(const row of await actions.all())await checkRight(row)
   if(view==='presentation'){
    await actions.locator('button').first().click()
    await checkRight(actions)
    await actions.getByRole('button',{name:locale==='en'?'Cancel confirmation':'取消确认',exact:true}).click()
   }
   await expectNoPageOverflow(page)
   await page.screenshot({path:info.outputPath(`page-actions-${view}-${locale}-${width}.png`)})
  }
  expect(errors).toEqual([])
 })
}

for(const locale of ['en','zh-CN'] as const)for(const kind of ['presentation','points','withdrawal','brand-currency','commission','platform-currency'] as const){
 test(`settings return to the previous page ${kind} (${locale})`,async({page})=>{
  page.setDefaultTimeout(8000)
  const zh=locale==='zh-CN',platform=kind==='commission'||kind==='platform-currency'
  const writes:Array<{body:unknown;key:string|undefined}>=[]
  let fail=true,version=1
  const rules=currencies.map(currency=>({currency,rate:'7',deposit_min:'1',deposit_max:'10000',withdrawal_min:'1',withdrawal_max:'10000',fee:'0',mode:'count',threshold:'1',inherit:false,enabled:true}))
  const cash={version:1,rules,platform_rules:structuredClone(rules)}
  const point={brand_id:gameId,version:1,max_balance_points:null,max_recharge_points:null,max_adjustment_points:null,updated_at:at,audit_log_id:gameId}
  const withdrawal={brand_id:gameId,version:1,points_step:'1',config:{enabled:false,min_points:'1',max_points:null,allowed_sources:['recharge','winning','gift'],review_mode:'manual',turnover_multiple:'1'},updated_at:at,audit_log_id:gameId}
  let commission={version:1,payouts_enabled:false,config:{enabled:false,payout_mode:'manual',calendar:null as unknown}}
  let presentation={brand_id:gameId,version:1,status:'active',base_name:'Trial',config:{...theme},effective:{...theme},updated_at:at,audit_log_id:gameId}
  await page.route('**/api/**',route=>{
   const req=route.request(),path=new URL(req.url()).pathname,reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(req.method()!=='GET'){
    writes.push({body:req.postDataJSON(),key:req.headers()['idempotency-key']});expect(writes.at(-1)!.key).toBeTruthy()
    if(fail)return route.fulfill({status:400,json:{success:false,error:{code:'VALIDATION_FAILED',message:'Trial validation'}}})
    version++
    if(path.endsWith('/point-policy'))return reply({...point,...req.postDataJSON(),version})
    if(path.endsWith('/withdrawal-policy')){Object.assign(withdrawal,req.postDataJSON(),{version});return reply(withdrawal)}
    if(path.endsWith('/brand-presentation')){presentation={...presentation,config:req.postDataJSON().config,effective:req.postDataJSON().config,version};return reply(presentation)}
    if(path.endsWith('/commission-settings')){commission={version,payouts_enabled:req.postDataJSON().payouts_enabled,config:req.postDataJSON().config};return reply(commission)}
    if(path.endsWith('/cash/config')){Object.assign(cash,req.postDataJSON(),{version});return reply(cash)}
    throw Error('Unexpected settings write '+path)
   }
   if(path.endsWith('/platform-games'))return reply({games:[],total_count:'0',limit:50,offset:0})
   if(path.endsWith('/brand-presentation/defaults'))return reply(theme)
   if(path.endsWith('/brand-presentation'))return reply(presentation)
   if(path.endsWith('/point-policy'))return reply(point)
   if(path.endsWith('/withdrawal-policy'))return reply(withdrawal)
   if(path.endsWith('/filter-options'))return reply({items:[],limit:100,offset:0})
   if(path.endsWith('/cash/config'))return reply(cash)
   if(path.endsWith('/cash/destinations'))return reply([])
   if(path.endsWith('/commission-settings'))return reply(commission)
   return route.fallback()
  })
  const grants=platform?['platform_settings.view.platform','platform_settings.write.platform','platform_game.view.platform']:['platform_game.view.brand','brand_presentation.view.brand','brand_presentation.write.brand','point_policy.view.brand','point_policy.write.brand','withdrawal_policy.view.brand','withdrawal_policy.write.brand','game.view.brand']
  await installConsoleSession(page,platform?'platform':'brand',locale,gameId,grants,actor)
  await page.goto(platform?origins.platform:origins.brand)
  await expect(page.locator('.management-console')).toBeVisible()
  if(platform){
   await openPlatformPage(page,zh?'彩种':'Games')
   await openPlatformPage(page,zh?'设置':'Settings')
   await page.locator('.platform-settings-tabs').getByRole('button',{name:kind==='commission'?(zh?'佣金结算':'Commission settlement'):(zh?'币种设置':'Currency settings'),exact:true}).click()
  }else{
   await openBrandPage(page,zh?'彩种管理':'Game management')
   await openBrandPage(page,zh?'品牌设置':'Brand settings')
   await page.locator('.console-tabs').getByRole('button',{name:kind==='presentation'?(zh?'品牌展示':'Presentation'):kind==='withdrawal'?(zh?'提现设置':'Withdrawal settings'):(zh?'资金':'Funds'),exact:true}).click()
  }
  const form=page.locator(kind==='presentation'?'.brand-presentation form':kind==='points'?'.point-policy-settings form':kind==='withdrawal'?'.withdrawal-policy .policy-card form':kind==='commission'?'.commission-fields':'.cash-console form').first()
  if(kind==='presentation')await form.locator('input[type=text]').first().fill('Updated trial')
  if(kind==='points')await form.locator('input').first().fill('1000')
  if(kind==='withdrawal')await form.locator('input[type=checkbox]').first().check()
  const submit=kind==='commission'?page.locator('.commission-fields').locator('xpath=..').locator('button[type=submit]'):form.locator('button[type=submit]')
  await submit.click()
  if(kind==='withdrawal')await page.getByRole('dialog').locator('.button-primary').click()
  await expect.poll(()=>writes.length).toBe(1)
  await expect(page.locator('.toast-error').last()).toBeVisible()
  await expect(form).toBeVisible()
  fail=false
  for(const dismiss of await page.locator('.toast-error button').all())await dismiss.click()
  await expect(submit.locator('xpath=ancestor::form[1]')).not.toHaveAttribute('data-write-locked','true')
  await submit.click()
  if(kind==='withdrawal')await page.getByRole('dialog').locator('.button-primary').click()
  await expect.poll(()=>writes.length).toBe(2)
  await expect(form).toHaveCount(0)
  await expect(page.getByTestId('lottery-catalog-page')).toBeVisible()
  await expectNoPageOverflow(page)
 })
}
