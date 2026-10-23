import { registerConfiguredSelectionTests } from './configured-selection'
registerConfiguredSelectionTests()
import { registerPlatformBrandNavigationTests } from './platform-brand-navigation'
import { registerPlatformOverviewTests } from './platform-overview'
import { registerBrandOverviewTests } from './brand-overview'
registerBrandOverviewTests()
registerPlatformOverviewTests()
registerPlatformBrandNavigationTests()
import { expect, test } from '@playwright/test'
import { registerBuiltinRoleTests } from './builtin-roles'
registerBuiltinRoleTests()
import { registerAssociationNameTests } from './association-names'
registerAssociationNameTests()
import {registerConfiguredSaleTests} from './workspace'
registerConfiguredSaleTests()
import { registerWorkspaceTests } from './workspace'
registerWorkspaceTests()
import { origins, gameId, installLayoutApi, installOrdersLayoutApi, installWalletLayoutApi, expectNoPageOverflow, expectInsideViewport, expectRefreshControls, expectCompactDisplayRows } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) {
  test(`login forms and betting layout use the device viewport (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    for (const url of [`${origins.user}/login`, `${origins.user}/register`, origins.agent, origins.brand, origins.platform]) {
      await page.goto(url)
      await expectCompactDisplayRows(page)
      const form = page.locator('form').first()
      await expect(form).toBeVisible()
      if (url === origins.platform) await expect(page).toHaveTitle('Layout trial')
      const password = form.locator('input[type=password]').first()
      await expect(form.locator('input[autocomplete=username]')).not.toHaveAttribute('placeholder', /.*/)
      await expect(password).not.toHaveAttribute('placeholder', /.*/)
      const fieldStyle = await password.evaluate(el => {
        const style = getComputedStyle(el)
        return { height: el.getBoundingClientRect().height, fontSize: parseFloat(style.fontSize), borderRadius: style.borderRadius, radiusToken: style.getPropertyValue('--radius-sm').trim() }
      })
      expect(fieldStyle.height).toBeGreaterThanOrEqual(44)
      expect(fieldStyle.fontSize).toBeGreaterThanOrEqual(16)
      expect(fieldStyle.borderRadius).toBe(fieldStyle.radiusToken)
      await password.fill('layout-only-password')
      await password.focus()
      await password.scrollIntoViewIfNeeded()
      await expectInsideViewport(password)
      const submit = form.locator('button[type=submit], button:not([type])').last()
      expect((await submit.boundingBox())!.height).toBeGreaterThanOrEqual(44)
      const primary = await submit.evaluate(el => getComputedStyle(el).backgroundImage)
      expect(primary).toBe('none')
      await submit.scrollIntoViewIfNeeded()
      await expectInsideViewport(submit)
      if (url === origins.agent) {
        const language = page.locator('.locale-picker__trigger')
        await language.click()
        await expect(page.locator('.locale-picker__option')).toHaveCount(2)
        await expectInsideViewport(page.locator('.locale-picker__list'))
        await page.locator(`.locale-picker__option[data-code="${locale === 'en' ? 'zh-CN' : 'en'}"]`).click()
        await expect(page.locator('#agent-login-title')).toHaveText(locale === 'en' ? '登录' : 'Sign in')
        await page.reload()
        await expect(page.locator('#agent-login-title')).toHaveText(locale === 'en' ? '登录' : 'Sign in')
        await page.locator('.locale-picker__trigger').click()
        await page.locator(`.locale-picker__option[data-code="${locale}"]`).click()
      }
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`login-${new URL(url).port}-${new URL(url).pathname.replaceAll('/', '')}-${locale}.png`), fullPage: true })
    }
    await installOrdersLayoutApi(page)
    await page.goto(`${origins.user}/configured/${gameId}`)
    await expect(page.locator('.configured-options button')).toHaveCount(49)
    await expect(page.locator('.configured-live .game-results-link')).toHaveCount(0)
    await expectNoPageOverflow(page)
    await page.screenshot({path:info.outputPath(`configured-betting-${locale}.png`),fullPage:true})
    await page.goto(`${origins.user}/help`)
    const help = page.locator('.help-page')
    await expect(help.locator('details')).toHaveCount(6)
    await expect(help.locator('.page-heading,.help-intro,a[href^="mailto:"]')).toHaveCount(0)
    for (const item of await help.locator('details').all()) {
      if (await item.getAttribute('open') === null) await item.locator('summary').click()
      await expect(item.locator('p')).toBeVisible()
    }
    const reminder = help.locator('.help-responsible')
    await reminder.scrollIntoViewIfNeeded()
    await expectInsideViewport(reminder)
    await expectNoPageOverflow(page)
    await page.screenshot({ path: info.outputPath(`help-${locale}.png`), fullPage: true })
    await page.goto(`${origins.user}/orders`)
    const layouts: unknown[] = []
    const gameFilter = page.locator('.history-filters select')
    await expect(gameFilter.locator('option')).toHaveCount(4)
    for (const index of [0, 1, 2, 3]) {
      if (index > 0) {
        const game = await gameFilter.locator('option').nth(index).getAttribute('value')
        await Promise.all([
          page.waitForResponse(response => {
            const url = new URL(response.url())
            return url.pathname === '/api/v1/betting-history' && url.searchParams.get('game_id') === game
          }),
          gameFilter.selectOption({ index }),
        ])
        await expect(gameFilter).toBeEnabled()
      }
      const empty = page.locator('.order-history-empty')
      await expect(empty).toHaveText(locale === 'en' ? 'No tickets yet' : '暂无注单')
      const refresh = page.locator('#user-page-refresh > button')
      await expect(refresh).toHaveClass('button button-secondary')
      await expect(refresh).toBeEnabled()
      await page.locator('.workspace-topbar > button.icon-button').click();
      await refresh.click()
      await expect(empty).toBeVisible()
      await expect(refresh).toBeEnabled()
      await refresh.evaluate(async el => { await Promise.all(el.getAnimations().map(animation => animation.finished)) })
      await expectRefreshControls(page)
      if (index === 1) {
        await page.keyboard.press('Tab')
        await refresh.focus()
        await expect(refresh).toBeFocused()
        await expect(refresh).toHaveCSS('outline-style', 'solid')
      }
      // Compare each game filter in the same idle pointer state after hover transitions finish.
      await page.mouse.move(0, 0)
      await refresh.evaluate(async el => {
        getComputedStyle(el).backgroundColor
        await Promise.all(el.getAnimations().map(animation => animation.finished))
      })
      layouts.push(await page.locator('.order-history').evaluate(el => {
        const button = document.querySelector('#user-page-refresh > button')!
        const empty = el.querySelector('.order-history-empty')!
        const rect = empty.getBoundingClientRect()
        // Browser transforms can introduce sub-pixel floating-point noise.
        const px = (value: number) => Math.round(value * 1000) / 1000
        return { left: px(rect.left), top: px(rect.top), width: px(rect.width), height: px(rect.height), gap: getComputedStyle(el).gap, align: getComputedStyle(empty).textAlign, background: getComputedStyle(button).backgroundColor, border: getComputedStyle(button).borderColor, buttonHeight: px(button.getBoundingClientRect().height) }
      }))
      await page.locator('.workspace-menu-close').click();
      await expectNoPageOverflow(page)
    }
    expect(layouts[1]).toEqual(layouts[0])
    expect(layouts[2]).toEqual(layouts[0])
    expect(layouts[3]).toEqual(layouts[0])
    expect(layouts[0]).toMatchObject({ align: 'center', gap: '12px' })
    await page.screenshot({ path: info.outputPath(`orders-${locale}.png`), fullPage: true })
    await page.goto(`${origins.user}/wallet`)
    const zeroWallet = page.locator('.wallet-summary')
    await expect(zeroWallet.locator('.total-card')).toHaveCount(3)
    await expect(zeroWallet.locator('.total-primary strong')).toHaveText('0')
    await expect(zeroWallet.locator('.wallet-links a')).toHaveCount(2)
    await expectRefreshControls(page)
    await expectNoPageOverflow(page)
    for (const path of ['/wallet/ledger', '/notifications', '/results']) {
      await page.goto(`${origins.user}${path}`)
      const empty = page.locator('.empty-state').filter({ visible: true }).first()
      await expect(empty).toBeVisible()
      await expect(empty).toHaveCSS('text-align', 'center')
      await expect(empty).toHaveCSS('padding', '16px')
      await expect(empty).toHaveCSS('font-size', '14px')
      await expect(empty).toHaveCSS('margin', '0px')
      if (path === '/results') {
        await expect(page.locator('.draw-results .empty-state')).toBeVisible()
        await expect(page.locator('.dr-disclaimer')).toHaveCount(0)
        await expect(page.locator('.draw-archive select')).toHaveCount(1)
        await expect(page.locator('.draw-archive input,.draw-archive [role=tablist]')).toHaveCount(0)
        await expect(empty).toHaveCSS('justify-content', 'center')
      } else await expect(empty).toHaveCSS('justify-content', 'center')
      await expect(page.locator('[role="alert"]')).toHaveCount(0)
      await expectRefreshControls(page)
      await empty.scrollIntoViewIfNeeded()
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`empty-${path.slice(1).replaceAll('/', '-')}-${locale}.png`), fullPage: true })
    }
    expect(errors).toEqual([])
    const metrics = await page.evaluate(() => ({ viewport: { width: innerWidth, height: innerHeight }, screen: { width: screen.width, height: screen.height }, dpr: devicePixelRatio }))
    expect(metrics.viewport).toEqual(info.project.use.viewport)
    expect(metrics.dpr).toBe(info.project.use.deviceScaleFactor)
    if (info.project.use.contextOptions?.screen) expect(metrics.screen).toEqual(info.project.use.contextOptions.screen)
    await info.attach('viewport.json', { body: JSON.stringify(metrics), contentType: 'application/json' })
  })
  test(`wallet overview and expandable changes keep point states distinct (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale);
    await installWalletLayoutApi(page);
    let ledgerRequests = 0;
    page.on('request', request => { if (new URL(request.url()).pathname === '/api/v1/wallet/ledger') ledgerRequests++; });
    await page.goto(`${origins.user}/wallet`);
    const wallet = page.locator('.wallet-summary');
    const headerRefresh = page.locator('#user-page-refresh > button');
    await expect(headerRefresh).toHaveCount(1);
    await expect(wallet.locator('[data-refresh-button]')).toHaveCount(0);
    await expect(page.locator('.workspace-topbar')).toBeVisible();
    await expect(page.locator('.workspace-overview')).toBeHidden();
    await expect(wallet.locator('.total-card')).toHaveCount(3);
    await expect(wallet.locator('.total-primary strong')).toHaveText('170');
    await expect(wallet.locator('.wallet-available b')).toHaveText(locale === 'en' ? '120 pts' : '120 分');
    await expect(wallet.locator('.metric-amount strong')).toHaveText(['50', '40']);
    await expect(wallet.locator('.wallet-available')).toHaveCSS('color', 'rgb(255, 255, 255)');
    await expect(wallet.locator('.bucket-grid, .ledger-panel')).toHaveCount(0);
    await expect(wallet.locator('.wallet-links a')).toHaveCount(2);
    expect(ledgerRequests).toBe(0);
    await expect(wallet.locator('.total-label').last()).toHaveText(locale === 'en' ? 'Withdrawal points' : '提现积分');
    await expectNoPageOverflow(page);
    await page.screenshot({ path: info.outputPath(`wallet-overview-${locale}.png`) });
    await wallet.locator('a[href="/wallet/buckets"]').click();
    await expect(page).toHaveURL(`${origins.user}/wallet/buckets`);
    await expect(headerRefresh).toHaveCount(1);
    await expect(wallet.locator('.totals, .ledger-panel, .wallet-links')).toHaveCount(0);
    await expect(wallet.locator('.source-head h3')).toHaveCount(4);
    expect(ledgerRequests).toBe(0);
    const bucketRows = await wallet.locator('.bucket-source dl > div').evaluateAll(rows => rows.map(row => {
      const label = row.querySelector('dt')!.getBoundingClientRect(), value = row.querySelector('dd')!.getBoundingClientRect();
      return { labelRight: label.right, labelBottom: label.bottom, valueLeft: value.left, valueTop: value.top };
    }));
    expect(bucketRows).toHaveLength(16);
    for (const row of bucketRows) { expect(row.valueLeft).toBeGreaterThanOrEqual(row.labelRight); expect(row.valueTop).toBeLessThan(row.labelBottom); }
    await expectNoPageOverflow(page);
    await page.screenshot({ path: info.outputPath(`wallet-buckets-${locale}.png`) });
    await page.goto(`${origins.user}/wallet`);
    await wallet.locator('a[href="/wallet/ledger"]').click();
    await expect(page).toHaveURL(`${origins.user}/wallet/ledger`);
    await expect(headerRefresh).toHaveCount(1);
    await expect(wallet.locator('.totals, .bucket-grid, .wallet-links')).toHaveCount(0);
    await expect(wallet.locator('.totals p, .source-head p, .panel-title p')).toHaveCount(0);
    await expect(wallet.locator('.entry-head b')).toHaveText(locale === 'en' ? 'Points frozen' : '积分冻结');
    expect(ledgerRequests).toBeGreaterThan(0);
    // A freeze moves points between states: the net total is zero, while available points decrease.
    await expect(wallet.locator('.entry-change strong')).toHaveText('−10');
    const details = wallet.locator('.ledger-details');
    await expect(details).not.toHaveAttribute('open');
    await expect(wallet.locator('.snapshot-grid')).toBeHidden();
    await expect(wallet.locator('.entry-reason')).toBeHidden();
    await expectRefreshControls(page);
    await expectNoPageOverflow(page);
    await page.screenshot({ path: info.outputPath(`wallet-ledger-${locale}.png`) });
    await details.locator('summary').click();
    await expect(details).toHaveAttribute('open', '');
    await expect(wallet.locator('.snapshot-grid')).toBeVisible();
    await expect(wallet.locator('.entry-reason')).toBeVisible();
    await expect(wallet.locator('.snapshot-bucket')).toHaveCount(16);
    const firstRow = await wallet.locator('.snapshot-grid > span').allTextContents();
    expect(firstRow.slice(0, 6)).toEqual(['130', '−10', '120', '10', '10', '20']);
    await expectNoPageOverflow(page);
    await page.screenshot({ path: info.outputPath(`wallet-details-${locale}.png`) });
    await details.locator('summary').click();
    await expect(wallet.locator('.snapshot-grid')).toBeHidden();
  });

}

import { registerBrandOperationLayoutTests } from './brand-operation'
registerBrandOperationLayoutTests(false)
import { registerMemberDeleteTests } from './member-delete'
registerMemberDeleteTests(false)

import { registerManagementConsoleTests } from './management-console'
registerManagementConsoleTests()

import {registerConfiguredHistoryLayoutTests} from './workspace'
registerConfiguredHistoryLayoutTests()

import { registerBrandStatusTests } from './brand-status'
registerBrandStatusTests(false)

import { registerBrandSettingsTests } from './brand-settings'
registerBrandSettingsTests(false)

import { registerSecuritySettingsTests } from './security-settings'
registerSecuritySettingsTests(false)

import { registerActionEditorTests } from "./action-editors"
registerActionEditorTests(false)

import { registerPlatformSettingsEditorTests } from "./platform-settings"
registerPlatformSettingsEditorTests(false)

import { registerAgentNavigationTests } from "./agent-navigation"
registerAgentNavigationTests(false)

import { registerMemberManagementTests } from "./member-management"
registerMemberManagementTests(false)

import { registerPlatformMemberManagementTests } from "./platform-member-management"
registerPlatformMemberManagementTests(false)

import { registerCompactListTests } from './compact-lists'
registerCompactListTests(false)

import { registerAgentDirectoryTests } from "./agent-directory"
registerAgentDirectoryTests(false)

import { registerControlSizingTests } from "./control-sizing"
registerControlSizingTests(false)

import { registerAgentModeChangeTests } from './agent-mode-change'
registerAgentModeChangeTests(false)

import { registerAgentPortalTests } from './agent-portal'
registerAgentPortalTests(false)
import { registerSharedAccountMenuTests } from './agent-portal'
registerSharedAccountMenuTests()
