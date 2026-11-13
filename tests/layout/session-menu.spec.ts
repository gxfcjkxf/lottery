import { test, expect } from '@playwright/test'
import { installLayoutApi, installWalletLayoutApi, origins, gameId, expectNoPageOverflow } from './fixture'

for (const locale of ['en','zh-CN'] as const) {
  test(`sign out lives in navigation and preserves session on failure (${locale})`, async ({page}, info) => {
    await installLayoutApi(page,locale)
    await installWalletLayoutApi(page)
    let signedIn=true, fail=true, writes=0
    await page.route('**/api/v1/me',route=>signedIn ? route.fulfill({json:{success:true,data:{user:{id:gameId,username:'layout-member',status:'active'},member:{id:gameId,brand_id:gameId,status:'normal',display_name:'Layout member',joined_at:'2026-10-17T00:00:00Z'}}}}) : route.fulfill({status:401,json:{success:false,error:{code:'AUTH_SESSION_REVOKED',message:'Signed out'}}}))
    // Explicit synthetic logout response; no account or database is changed.
    await page.route('**/api/v1/auth/logout',async route=>{
      expect(route.request().method()).toBe('POST'); writes++
      if(fail)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Unavailable'}}})
      signedIn=false;return route.fulfill({json:{success:true,data:{}}})
    })
    await page.goto(`${origins.user}/account`)
    const name=locale==='en'?'Sign out':'退出登录'
    await expect(page.locator('.account-identity')).toContainText('Layout member')
    await expect(page.locator('main').getByRole('button',{name,exact:true})).toHaveCount(0)
    const preset=page.viewportSize()!
    const widths=info.project.name.includes('reflow')?[320,620,999,1000,1001,1440]:[preset.width]
    for(const width of widths){
      if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:720})
      if(width>=1000)await expect(page.locator('.workspace-sidebar').getByRole('button',{name,exact:true})).toBeVisible()
      await expectNoPageOverflow(page)
    }
    await page.locator('.workspace-menu-trigger').click()
    const menu=page.locator('.workspace-menu'), logout=menu.getByRole('button',{name,exact:true})
    await expect(logout).toBeVisible()
    expect((await logout.boundingBox())!.height).toBeGreaterThanOrEqual(info.project.name.includes('reflow')?32:44)
    await page.screenshot({path:info.outputPath(`session-menu-${locale}.png`),animations:'disabled'})
    await logout.click()
    await expect(page.locator('.operation-toast-stack').getByRole('alert')).toBeVisible()
    await expect(page).toHaveURL(/\/account$/)
    expect(signedIn).toBe(true)
    fail=false
    await expect(logout).not.toHaveAttribute('aria-disabled','true')
    await logout.click()
    await expect(page).toHaveURL(/\/login$/)
    expect(writes).toBe(2)
    await expect(page.locator('.workspace-signout')).toHaveCount(0)
  })
}
