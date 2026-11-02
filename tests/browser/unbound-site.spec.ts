import { test, expect } from '@playwright/test'
const origin = process.env.TEST_USER_ORIGIN ?? 'http://localhost:5173'
for (const locale of ['en','zh-CN'] as const) for (const path of ['/login','/account']) {
 test(`unbound host has no brand chrome or business error (${locale} ${path})`,async({page})=>{
  await page.context().addCookies([{name:'lottery-locale',value:locale,url:origin}])
  let release!:()=>void, requested!:()=>void
  const waiting=new Promise<void>(resolve=>{requested=resolve})
  const gate=new Promise<void>(resolve=>{release=resolve})
  const calls:string[]=[]
  await page.route('**/api/**',async route=>{
   const request=route.request(),url=new URL(request.url())
   expect(request.method()).toBe('GET');calls.push(url.pathname)
   if(url.pathname==='/api/v1/display-settings')return route.fulfill({json:{success:true,data:{timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}}})
   expect(['/api/v1/context','/api/v1/me']).toContain(url.pathname)
   requested();await gate
   return route.fulfill({status:404,json:{success:false,error:{code:'BRAND_NOT_FOUND',message:'Unknown brand'}}})
  })
  await page.goto(origin+path,{waitUntil:'domcontentloaded'})
  await waiting
  await expect(page.locator('.login-entry,.workspace-shell')).toHaveCount(0)
  release()
  const title=locale==='en'?'Site unavailable':'站点不可用'
  await expect(page.getByRole('heading',{name:title,exact:true})).toBeVisible()
  await expect(page).toHaveTitle(title)
  await expect(page.locator('.login-entry,.workspace-shell,.toast-error')).toHaveCount(0)
  await expect(page.getByRole('button')).toHaveCount(0)
  await expect(page.locator('body')).not.toContainText('Luma')
  await expect(page.locator('link[rel=manifest]')).toHaveCount(0)
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
  expect(calls.every(url=>['/api/v1/display-settings','/api/v1/context','/api/v1/me'].includes(url))).toBe(true)
 })
}
