import { expect, test } from '@playwright/test'
import { origins, gameId, installLayoutApi, expectNoPageOverflow, expectCompactPageLayout } from './fixture'

export function registerPlatformBrandNavigationTests(reflow = false) {
  for (const locale of ['en', 'zh-CN'] as const) {
    test(`platform brand navigation groups three pages (${locale})`, async ({ page }, info) => {
      const zh = locale === 'zh-CN'
      await installLayoutApi(page, locale)
      await page.route('**/api/v1/platform/**', async route => {
        const path = new URL(route.request().url()).pathname
        const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
        if (path.endsWith('/presentation')) return reply({ display_name: 'Layout', logo_text: 'L', logo_url: '' })
        if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'operator', super_admin: true, platform_permissions: [] } })
        if (path.endsWith('/platform-settings')) return reply({ timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss', display_name: 'Layout', logo_text: 'L', logo_url: '', version: 1, updated_at: '2026-10-18T00:00:00Z' })
        if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, code: 'aurora', name: 'Aurora', status: 'active' }] })
        if (path.endsWith('/users')) { const url = new URL(route.request().url()); return reply({ items: [], limit: Number(url.searchParams.get('limit')), offset: Number(url.searchParams.get('offset')), total_count: '0' }) }
        if (path.endsWith('/accounts')) return reply({ items: [] })
        throw Error(`Unexpected navigation request: ${path}`)
      })
      await page.goto(origins.platform)
      await expect(page.locator('.management-console')).toBeVisible()
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
          const child = children.getByRole('button', { name, exact: true })
          await child.click()
          await expect(children.locator('button.selected')).toHaveText(name)
          await expect(page.locator('.breadcrumb')).toContainText(name)
          if (name !== names[0]) {
            const picker = page.locator('.topbar-brand-picker')
            await expect(picker.getByRole('combobox', { name: zh ? '品牌' : 'Brand', exact: true })).toBeVisible()
            await expect(picker.locator('span')).toHaveCount(0)
            await expect(page.locator('.content > .page-heading h1')).toHaveText(name)
            await expect(page.getByTestId('platform-brand-required')).toHaveText(zh ? '请先选择品牌' : 'Please select a brand first')
            await expect(page.locator('.content .panel, .content [data-testid="platform-access"]')).toHaveCount(0)
          }
          if (await menu.isVisible()) await expect(menu).toHaveAttribute('aria-expanded', 'false')
          await expectCompactPageLayout(page)
          await expectNoPageOverflow(page)
        }
        if (await menu.isVisible()) await menu.click()
        await page.screenshot({ path: info.outputPath(`brand-navigation-${locale}-${width}.png`) })
        await expectNoPageOverflow(page)
      }
      const menu = page.locator('button[aria-controls="platform-navigation"]')
      const hint = page.getByTestId('platform-brand-required')
      const hintText = zh ? '请先选择品牌' : 'Please select a brand first'
      const openMenu = async () => {
        if (await menu.isVisible() && await menu.getAttribute('aria-expanded') === 'false') await menu.click()
      }
      for (const label of zh ? ['人工奖励', '投注订单', '彩种', '开奖', '提现订单'] : ['Manual rewards', 'Bet orders', 'Games', 'Draws', 'Withdrawals']) {
        await openMenu()
        await page.locator('.sidebar > button').getByText(label, { exact: true }).click()
        await expect(hint).toHaveText(hintText)
        await expect(page.locator('.content [data-testid="platform-access"]')).toHaveCount(0)
      }
      for (const [label, testId] of [[zh ? '运营' : 'Operations', 'platform-operations-tabs'], [zh ? '会员积分' : 'Member points', 'platform-wallet-tabs'], [zh ? '佣金' : 'Commission', 'platform-commission-secondary-nav']]) {
        await openMenu()
        await page.locator('.sidebar > button').getByText(label, { exact: true }).click()
        const tabs = page.getByTestId(testId).locator('button')
        for (let index = 0; index < await tabs.count(); index++) {
          await openMenu()
          await tabs.nth(index).click()
          await expect(hint).toHaveText(hintText)
        }
      }
      await openMenu()
      await page.getByTestId('platform-brand-tabs').getByRole('button', { name: zh ? '会员管理' : 'Member management', exact: true }).click()
      await page.locator('.topbar-brand-picker select').selectOption(gameId)
      await expect(hint).toHaveCount(0)
    })
  }
}

