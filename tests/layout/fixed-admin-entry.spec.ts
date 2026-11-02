import {expect,test} from '@playwright/test'
import {gameId,origins,installLayoutApi,expectNoPageOverflow} from './fixture'
const second='aaaaaaaa-aaaa-4aaa-8aaa-000000000002',foreign='aaaaaaaa-aaaa-4aaa-8aaa-000000000003'
for(const locale of ['zh-CN','en'] as const)for(const multiple of [false,true])test(`fixed admin authorized brands ${locale} ${multiple?'multiple':'single'}`,async({page},info)=>{
 await installLayoutApi(page,locale)
 const requests:string[]=[],errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('request',r=>{if(new URL(r.url()).pathname==='/api/v1/context')requests.push(r.url())})
 const brands=[{id:gameId,code:'first',name:'First Brand',status:'active',allow_custom_games:false,has_custom_games:false},...(multiple?[{id:second,code:'second',name:'Second Brand',status:'active',allow_custom_games:false,has_custom_games:false},{id:foreign,code:'foreign',name:'Foreign Brand',status:'active',allow_custom_games:false,has_custom_games:false}]:[])]
 await page.route('**/api/v1/admin/**',route=>{
  const path=new URL(route.request().url()).pathname
  if(path.endsWith('/me'))return route.fulfill({json:{success:true,data:{account:{id:gameId,username:'fixed_admin',super_admin:false,brand_ids:multiple?[gameId,second]:[gameId],permissions:[],permissions_by_brand:{[gameId]:[],[second]:[]}}}}})
  if(path.endsWith('/brands'))return route.fulfill({json:{success:true,data:{items:brands}}})
  throw new Error('Unexpected admin request '+path)
 })
 await page.goto(origins.brand)
 await expect(page.locator('.topbar')).toBeVisible()
 if(multiple){
  const choose=page.locator('.admin-brand-picker')
  await expect(choose).toBeVisible();await choose.getByRole('combobox').click();await expect(choose.getByRole('option')).toHaveCount(3)
  await choose.getByRole('option',{name:'First Brand',exact:true}).click();await expect(page.locator('.breadcrumbs')).toContainText('First Brand')
  await choose.getByRole('combobox').click();await choose.getByRole('option',{name:'Second Brand',exact:true}).click();await expect(page.locator('.breadcrumbs')).toContainText('Second Brand')
 }else{await expect(page.locator('.breadcrumbs')).toContainText('First Brand');await expect(page.locator('.admin-brand-picker')).toHaveCount(0)}
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`admin-${locale}-${multiple}.png`),fullPage:true})
 expect(requests).toEqual([]);expect(errors).toEqual([])
})

for(const locale of ['en','zh-CN'] as const)test(`site entry publishes one global loading status (${locale})`,async({page},info)=>{
 await installLayoutApi(page,locale)
 const errors:string[]=[];page.on('pageerror',cause=>errors.push(cause.message))
 let release!:()=>void
 const contextReady=new Promise<void>(resolve=>{release=resolve})
 await page.route('**/api/v1/context',async route=>{await contextReady;await route.fallback()})
 try{
  await page.goto(origins.user+'/login',{waitUntil:'domcontentloaded'})
  const placeholder=page.locator('.site-entry-loading'),overlay=page.locator('.global-loading-overlay')
  await expect(placeholder).toHaveAttribute('data-global-loading','true')
  await expect(placeholder).toBeHidden();await expect(overlay).toHaveCount(1);await expect(overlay).toBeVisible()
  await expect(overlay).toContainText((await placeholder.textContent())!.trim())
  await expect(page.locator('input[autocomplete=username]')).toHaveCount(0)
  const preset=page.viewportSize()!
  for(const width of info.project.name.endsWith('reflow')?[320,1440]:[preset.width]){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
   await page.screenshot({path:info.outputPath(`site-entry-loading-${locale}-${width}.png`)})
  }
 }finally{release()}
 await expect(page.locator('input[autocomplete=username]')).toBeVisible()
 await expect(page.locator('.site-entry-loading,.global-loading-overlay')).toHaveCount(0)
 expect(errors).toEqual([])
})
