import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {readFileSync} from 'node:fs'
import {platformLotteryCatalog} from '../../shared/src/platform-lottery'
import {gameId} from './fixture'
import {origins,installLayoutApi,expectNoPageOverflow} from './fixture'

for(const surface of ['user','brand','platform','agent'] as const){
 test(`shared operation toast is usable across views (${surface})`,async({page},info)=>{
  await page.clock.install()
  const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
  await installLayoutApi(page,'zh-CN')
  await page.goto(origins[surface])
  await expect(page.locator('form').first()).toBeVisible()
  await page.waitForLoadState('networkidle')
  await page.evaluate(async path=>{
   const {showOperationToast,dismissOperationToast}=await import(path)
   dismissOperationToast()
   const source=Symbol('result')
   showOperationToast(()=> '旧的投注回执','success',source)
   showOperationToast(()=> '投注成功','success',source)
   showOperationToast(()=> '尚未开盘，暂不可投注。','error',Symbol('page-error'))
   showOperationToast(()=> '尚未开盘，暂不可投注。','error',Symbol('app-error'))
  },'/@fs'+resolve('shared/src/operation-toast.ts'))
  const stack=page.locator('.operation-toast-stack'),success=stack.locator('.toast-success'),failure=stack.locator('.toast-error')
  await expect(stack.locator('.toast')).toHaveCount(2)
  await expect(success).toContainText('投注成功');await expect(stack).not.toContainText('旧的投注回执')
  await expect(failure).toHaveAttribute('role','alert')
  const preset=page.viewportSize()!
  for(const width of info.project.name.endsWith('reflow')?[320,480,900,1280]:[preset.width]){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
   const box=await stack.boundingBox();expect(box).not.toBeNull();expect(box!.x).toBeGreaterThanOrEqual(0);expect(box!.y).toBeGreaterThanOrEqual(0)
   expect(page.viewportSize()!.width-box!.x-box!.width).toBeLessThanOrEqual(21)
   expect(page.viewportSize()!.height-box!.y-box!.height).toBeLessThanOrEqual(35)
   await expect(success.locator('button')).toHaveCSS('width','44px')
   await expect(success.locator('.toast-message')).toHaveCSS('font-size','14px')
  }
  await success.getByRole('button').focus();await page.clock.fastForward(4000)
  await expect(success).toBeVisible();await expect(failure).toHaveCount(0)
  await page.screenshot({path:info.outputPath(`${surface}-toast.png`)})
  await success.getByRole('button').click();await expect(stack).toHaveCount(0)
  expect(errors).toEqual([])
 })
}

for (const locale of ['zh-CN', 'en'] as const) {
 test(`agent session expiry shows one notice for concurrent failures (${locale})`, async ({page}, info) => {
  await page.clock.install()
  await installLayoutApi(page, locale)
  const expired = {success:false,error:{code:'AUTH_SESSION_REVOKED',message:'Session expired'}}
  let failures = 0
  await page.route('**/api/v1/agent/**', async route => {
   const path = new URL(route.request().url()).pathname
   if (path.endsWith('/me')) return route.fulfill({json:{success:true,data:{
    account:{username:'isolated_agent'},
    agent:{id:'test-agent',depth:1,config:{can_create_children:false}},
   }}})
   if (path.endsWith('/auth/config')) return route.fulfill({json:{success:true,data:{captcha_enabled:false}}})
   if (path.endsWith('/commission') || path.endsWith('/wallet')) {
    failures++
    return route.fulfill({status:401,json:expired})
   }
   throw new Error(`Unexpected agent request: ${path}`)
  })
  await page.goto(origins.agent + '/#/commission')
  await expect(page.locator('.login-entry__form')).toBeVisible()
  await expect.poll(() => failures).toBe(2)
  const toast = page.locator('.operation-toast-stack .toast')
  await expect(toast).toHaveCount(1)
  await expect(toast).toContainText(locale === 'zh-CN' ? '登录已失效，请重新登录。' : 'Your session has expired. Please sign in again.')
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`agent-session-expired-${locale}.png`)})
  await page.clock.fastForward(4000)
  await expect(toast).toHaveCount(0)
 })
}

for (const locale of ['en','zh-CN'] as const) for (const stage of ['catalog','schedule','sources'] as const) {
 test(`platform session expiry has one notice from ${stage} (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN',copy=platformLotteryCatalog[locale],errors:string[]=[]
  await installLayoutApi(page,locale)
  page.on('pageerror',e=>errors.push(e.message))
  const model=JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json','utf8')).model
  const game={id:gameId,code:'mark_six',name:'Mark Six',model,timezone:'Asia/Hong_Kong',version:1,connected:false,status:'active',deletable:true,brand_enabled:false,brand_version:0,rule_published:false,sales_status:'unconfigured'}
  let expired=false,failures=0
  await page.route('**/api/v1/platform/**',route=>{
   const path=new URL(route.request().url()).pathname
   const ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/presentation'))return ok({display_name:'Trial platform',logo_text:'T',logo_url:''})
   if(path.endsWith('/me'))return ok({account:{id:gameId,username:'expiry_reviewer',super_admin:true,platform_permissions:['platform_game.view.platform','platform_game.write.platform']}})
   if(path.endsWith('/platform-settings'))return ok({version:1,display_name:'Trial platform',logo_text:'T',logo_url:'',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',updated_at:'2026-11-01T00:00:00Z'})
   if(path.endsWith('/brands'))return ok({items:[]})
   if(expired&&(stage==='catalog'?path==='/api/v1/platform/platform-games':path.endsWith('/automation')||stage==='sources'&&path.endsWith('/draw-sources'))){
    failures++
    return route.fulfill({status:401,json:{success:false,error:{code:'AUTH_SESSION_REVOKED',message:'Session expired'}}})
   }
   if(path==='/api/v1/platform/platform-games')return ok({games:[game],total_count:'1',limit:50,offset:0})
   if(path.endsWith('/official-calendar'))return ok({current_month:'2026-11',months:[],status:'pending',last_success_at:null,last_attempt_at:null,next_attempt_at:null})
   if(path===`/api/v1/platform/platform-games/${gameId}`)return ok({game,periods:[],total_count:'0'})
   throw new Error(`Unexpected expiry-test request: ${path}`)
  })
  await page.goto(origins.platform)
  await expect(page.locator('.management-console')).toBeVisible()
  const more=page.locator('.platform-mobile-nav button[aria-controls="platform-navigation"]')
  if(await more.isVisible())await more.click()
  await page.locator('.sidebar').getByRole('button',{name:zh?'彩种':'Games',exact:true}).click()
  await expect(page.locator('.platform-lottery-catalog-table')).toContainText('Mark Six')
  expired=true
  if(stage==='catalog')await page.locator('[data-testid=lottery-catalog-page]').getByRole('button',{name:copy.filterSearch,exact:true}).click()
  else await page.locator('.platform-lottery-catalog-table tbody tr').first().getByRole('button',{name:stage==='schedule'?copy.timeSettings:copy.drawSources,exact:true}).click()
  await expect(page.locator('.platform-login')).toBeVisible()
  await expect.poll(()=>failures).toBe(stage==='sources'?2:1)
  const notices=page.locator('.operation-toast-stack .toast')
  await expect(notices).toHaveCount(1)
  await expect(notices.locator('.toast-message')).toHaveText(zh?'登录已失效，请重新登录。':'Your session has expired. Please sign in again.')
  const preset=page.viewportSize()!
  for(const width of info.project.name.includes('reflow')?[320,480,900,1280]:[preset.width]){
   if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
  }
  await page.screenshot({path:info.outputPath(`platform-expired-${stage}-${locale}.png`)})
  await expect(notices).toHaveCount(0,{timeout:6000})
  expect(errors).toEqual([])
 })
}
