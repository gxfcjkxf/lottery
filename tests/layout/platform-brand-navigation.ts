import { expectNavigationStyle } from './navigation-style'
import { selectControl } from '../select-control'
import { expect, test } from '@playwright/test'
import { origins, gameId, installLayoutApi, expectNoPageOverflow, expectCompactPageLayout } from './fixture'

export function registerPlatformBrandNavigationTests(reflow = false) {
  for (const locale of ['en', 'zh-CN'] as const) {
    test(`brand console navigation shares the management style (${locale})`, async ({ page }, info) => {
      await installLayoutApi(page, locale)
      await page.route('**/api/v1/admin/**', async route => {
        expect(route.request().method()).toBe('GET')
        const path = new URL(route.request().url()).pathname
        const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
        if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'navigation_reviewer', super_admin: false, brand_ids: [gameId], permissions_by_brand: { [gameId]: ['user.view.brand'] } } })
        if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, name: 'Aurora', code: 'aurora', status: 'active' }] })
        if (path.endsWith('/users')) return reply({ items: [], limit: 100, offset: 0, total_count: '0' })
        throw Error(`Unexpected brand navigation request: ${path}`)
      })
      await page.goto(origins.brand)
      await expect(page.locator('.management-console')).toBeVisible()
      const preset = page.viewportSize()!
      for (const width of reflow ? [320, 700, 701, 900, 901, 1440] : [preset.width]) {
        if (reflow) await page.setViewportSize({ width, height: preset.height })
        await expectNavigationStyle(page, '.sidebar .nav-item.active')
        const mobile = page.locator('.mobile-nav')
        if (await mobile.isVisible()) {
          await mobile.getByRole('button').last().click()
          await expect(page.locator('.mobile-more-menu')).toBeVisible()
          await page.locator('.mobile-more-menu').getByRole('button', { name: locale === 'en' ? 'Member management' : '会员管理', exact: true }).click()
          await expect(page.locator('.mobile-more-menu')).toBeHidden()
        }
        await expectNoPageOverflow(page)
        await page.screenshot({ path: info.outputPath(`brand-console-navigation-${locale}-${width}.png`) })
      }
    })
    test(`platform brand navigation groups three pages (${locale})`, async ({ page }, info) => {
      const zh = locale === 'zh-CN'
      await installLayoutApi(page, locale)
      await page.route('**/api/v1/platform/**', async route => {
        const path = new URL(route.request().url()).pathname
        const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
        if (path.endsWith('/presentation')) return reply({ display_name: 'Layout', logo_text: 'L', logo_url: '' })
        if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'operator', super_admin: true, platform_permissions: [] } })
        if (path.endsWith('/platform-settings')) return reply({ timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss', display_name: 'Layout', logo_text: 'L', logo_url: '', version: 1, updated_at: '2026-10-18T00:00:00Z' })
        if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'aurora', name: 'Aurora', status:'active',member_count:'12',agent_count:'3',recharge_quota_available:'1000',created_at:'2026-10-24T00:00:00Z' }] })
        if (path.endsWith('/users')) { const url = new URL(route.request().url()); return reply({ items: [], limit: Number(url.searchParams.get('limit')), offset: Number(url.searchParams.get('offset')), total_count: '0' }) }
        if (path.endsWith('/accounts') || path.endsWith('/notification-templates')) return reply({ items: [] })
        throw Error(`Unexpected navigation request: ${path}`)
      })
      await page.goto(origins.platform)
      await expect(page.locator('.management-console')).toBeVisible()
      await expectNavigationStyle(page, '.sidebar .nav-item.selected')
      await expect(page.locator('.sidebar button[aria-controls="platform-brand-tabs"]')).toHaveAttribute('aria-expanded', 'false')
      await expect(page.getByTestId('platform-brand-tabs')).toHaveCount(0)
      const preset = page.viewportSize()!
      for (const width of reflow ? [320, 640, 641, 900, 901, 1440] : [preset.width]) {
        if (reflow) {
          await page.setViewportSize({ width, height: preset.height })
          await page.locator('.topbar .locale-picker__trigger').evaluate(async el => {
            getComputedStyle(el).width
            await Promise.all(el.getAnimations().map(animation => animation.finished))
          })
        }
        const menu = page.locator('button[aria-controls="platform-navigation"]')
        if (await menu.isVisible() && await menu.getAttribute('aria-expanded') === 'false') await menu.click()
        const parent = page.locator('.sidebar button[aria-controls="platform-brand-tabs"]')
        await expect(parent).toHaveText(zh ? '品牌' : 'Brands')
        if (await parent.getAttribute('aria-expanded') === 'false') await parent.click()
        const children = page.getByTestId('platform-brand-tabs')
        const names = zh ? ['品牌列表', '后台账号', '会员管理'] : ['Brand list', 'Back-office accounts', 'Member management']
        await expect(children.getByRole('button')).toHaveText(names)
        await parent.click()
        await expect(children).toBeHidden()
        await parent.click()
        for (const name of names) {
          if (await menu.isVisible() && await menu.getAttribute('aria-expanded') === 'false') await menu.click()
          if(await parent.getAttribute('aria-expanded')==='false')await parent.click()
          const child = children.getByRole('button', { name, exact: true })
          await child.click()
          await expect(children.locator('button.selected')).toHaveText(name)
          await expect(page.locator('.breadcrumb')).toContainText(name)
          if (name === names[0]) {
            const panel = page.getByTestId('platform-brand-list')
            const frame = (await panel.boundingBox())!
            const fields = await panel.locator('form input, form .select-control-trigger, form .query-submit').all()
            for (const field of fields) {
              const box = (await field.boundingBox())!
              expect(box.y - frame.y).toBeGreaterThanOrEqual(12)
              expect(box.x - frame.x).toBeGreaterThanOrEqual(12)
              expect(frame.x + frame.width - box.x - box.width).toBeGreaterThanOrEqual(12)
            }
            await expectNoPageOverflow(page)
            await page.screenshot({ path: info.outputPath(`brand-filters-${locale}-${width}.png`) })
          }
          if (name !== names[0]) {
            const picker = page.locator('.topbar-brand-picker')
            await expect(picker.getByRole('combobox', { name: zh ? '品牌' : 'Brand', exact: true })).toBeVisible()
            await expect(picker.locator('.field-label')).toHaveCount(0)
            await expect(page.locator('.breadcrumb')).toContainText(name)
            await expect(page.getByTestId('platform-brand-required')).toHaveText(zh ? '请先选择品牌' : "Please select a brand first")
            await expect(page.locator('.content .panel, .content [data-testid="platform-access"]')).toHaveCount(0)
          }
          if (await menu.isVisible()) await expect(menu).toHaveAttribute('aria-expanded', 'false')
          await expectCompactPageLayout(page)
          await expectNoPageOverflow(page)
          // The brand picker must not squeeze breadcrumbs into a tall vertical strip.
          expect((await page.locator('.topbar').boundingBox())!.height).toBeLessThanOrEqual(112)
        }
        if (await menu.isVisible()) await menu.click()
        await page.screenshot({ path: info.outputPath(`brand-navigation-${locale}-${width}.png`) })
        await expectNoPageOverflow(page)
      }
      const menu = page.locator('button[aria-controls="platform-navigation"]')
      const hint = page.getByTestId('platform-brand-required')
      const hintText = zh ? '请先选择品牌' : "Please select a brand first"
      const openMenu = async () => {
        if (await menu.isVisible() && await menu.getAttribute('aria-expanded') === 'false') await menu.click()
      }
      for (const label of zh ? ['奖励', '注单', '提现'] : ['Rewards', 'Bets', 'Withdrawals']) {
        await openMenu()
        await page.locator('.sidebar').getByRole('button', { name: label, exact: true }).click()
        await expect(hint).toHaveText(hintText)
        await expect(page.locator('.content [data-testid="platform-access"]')).toHaveCount(0)
      }
      for (const [label, testId] of [[zh ? '运营' : 'Operations', 'platform-operations-tabs'], [zh ? '资金' : 'Finance', 'platform-wallet-tabs'], [zh ? '佣金' : 'Commission', 'platform-commission-secondary-nav']]) {
        await openMenu()
        await page.locator('.sidebar').getByRole('button', { name: label, exact: true }).click()
        const tabs = page.getByTestId(testId).locator('button')
        for (let index = 0; index < await tabs.count(); index++) {
          await openMenu()
          const tabName=await tabs.nth(index).textContent()
          await tabs.nth(index).click()
          if(tabName===(zh?'通知模板':'Notification templates'))await expect(hint).toHaveCount(0)
          else await expect(hint).toHaveText(hintText)
        }
      }
      await openMenu()
      const brandParent=page.locator('.sidebar button[aria-controls="platform-brand-tabs"]')
      if(await brandParent.getAttribute('aria-expanded')==='false')await brandParent.click()
      await page.getByTestId('platform-brand-tabs').getByRole('button', { name: zh ? '会员管理' : 'Member management', exact: true }).click()
      await selectControl(page.locator(".topbar-brand-picker .select-control-trigger"), gameId)
      await expect(hint).toHaveCount(0)
    })
  }
}

