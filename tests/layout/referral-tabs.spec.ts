import { test, expect } from '@playwright/test'
import { installLayoutApi, installWalletLayoutApi, origins, gameId, expectNoPageOverflow } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) {
  test(`referral tabs show own invitees first (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    await installWalletLayoutApi(page)
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    let mode: 'list' | 'empty' | 'error' = 'list'
    const reads: string[] = []
    await page.route('**/api/v1/me/referrals?*', route => {
      expect(route.request().method()).toBe('GET')
      reads.push('referrals')
      if (mode === 'error') return route.fulfill({status:503,json:{success:false,error:{code:'JOIN_CODE_UNAVAILABLE',message:'Unavailable'}}})
      const offset = Number(new URL(route.request().url()).searchParams.get('offset'))
      return route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,limit:20,offset,total_count:mode === 'empty' ? '0' : '21',items:mode === 'empty' ? [] : Array.from({length:offset ? 1 : 20},(_,i)=>({id:`bbbbbbbb-bbbb-4bbb-8bbb-${String(offset+i+1).padStart(12,'0')}`,display_name:offset ? 'Last invited member' : i ? `Invited member ${i}` : 'LongMemberNameThatMustWrapWithoutOverflow1234567890',joined_at:'2026-10-25T04:00:00Z'}))}}})
    })
    await page.route('**/api/v1/me/join-codes?*', route => {
      expect(route.request().method()).toBe('GET'); reads.push('codes')
      return route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,limit:20,offset:0,total_count:'1',items:[{id:gameId,brand_id:gameId,owner_member_id:gameId,kind:'referral',code:'A1B2C3D4E5F607182930ABCD',agent_id:null,status:'active',starts_at:null,expires_at:null,version:1,usable:true,created_at:'2026-10-25T04:00:00Z',updated_at:'2026-10-25T04:00:00Z'}]}}})
    })
    await page.goto(`${origins.user}/invites`)
    const listTab = page.getByRole('tab', {name:locale==='en'?'Referral list':'推荐列表',exact:true})
    const codeTab = page.getByRole('tab', {name:locale==='en'?'Referral code':'推荐码',exact:true})
    await expect(listTab).toHaveAttribute('aria-selected','true')
    await expect(page.locator('.referral-list li')).toHaveCount(20)
    expect(reads).toEqual(['referrals'])
    const preset = page.viewportSize()!
    const widths = info.project.name.includes('reflow') ? [320,620,999,1000,1001,1440] : [preset.width]
    for (const width of widths) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({width,height:720})
      await expectNoPageOverflow(page)
      const alignment = await page.locator('.join-codes-page').evaluate(section => {
        const content = section.closest('main')!;
        const bounds = content.getBoundingClientRect(), pageBounds = section.getBoundingClientRect();
        const style = getComputedStyle(content);
        return {left:pageBounds.left, right:pageBounds.right, contentLeft:bounds.left+parseFloat(style.paddingLeft), contentRight:bounds.right-parseFloat(style.paddingRight)};
      });
      expect(Math.abs(alignment.left-alignment.contentLeft)).toBeLessThanOrEqual(1);
      expect(Math.abs(alignment.right-alignment.contentRight)).toBeLessThanOrEqual(1);
    }
    await page.getByRole('button',{name:locale==='en'?'Next':'下一页',exact:true}).click()
    await expect(page.locator('.referral-list')).toContainText('Last invited member')
    await page.getByRole('button',{name:locale==='en'?'Previous':'上一页',exact:true}).click()
    await expect(page.locator('.referral-list li')).toHaveCount(20)
    await page.screenshot({path:info.outputPath(`referral-list-${locale}.png`),fullPage:true,animations:'disabled'})
    await codeTab.click()
    await expect(page.locator('.registration-link')).toContainText('referral_code=A1B2C3D4E5F607182930ABCD')
    await expect(page.locator('.registration-qr')).toBeVisible()
    await expectNoPageOverflow(page)
    await page.screenshot({path:info.outputPath(`referral-code-${locale}.png`),fullPage:true,animations:'disabled'})
    mode='empty'; await listTab.click()
    await expect(page.getByRole('tabpanel')).toContainText(locale==='en'?'You have not referred any users yet.':'暂无推荐用户。')
    await codeTab.click(); await codeTab.press('ArrowLeft')
    await expect(listTab).toBeFocused()
    await codeTab.click(); mode='error'; await listTab.click()
    await expect(page.getByRole('alert')).toBeVisible()
    mode='list'; await page.getByRole('button',{name:locale==='en'?'Retry':'重试',exact:true}).click()
    await expect(page.locator('.referral-list li')).toHaveCount(20)
    expect(errors).toEqual([])
  })
}
