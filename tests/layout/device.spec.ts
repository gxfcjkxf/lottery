import { registerOddsCatalogTests } from './odds-catalog'
registerOddsCatalogTests()
import { registerStateReviewTests } from './state-review'
registerStateReviewTests()
import { registerConfiguredSelectionTests } from './configured-selection'
import { registerFormValidationTests } from './form-validation'
registerFormValidationTests()
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
import { origins, gameId, installLayoutApi, installOrdersLayoutApi, installWalletLayoutApi, expectNoPageOverflow, expectInsideViewport, expectRefreshControls, expectCompactDisplayRows, expectPlayTabLabelsFit } from './fixture'

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
    await expectPlayTabLabelsFit(page)
    await expect(page.locator('.configured-live .game-results-link')).toHaveCount(0)
    await expectNoPageOverflow(page)
    await page.screenshot({path:info.outputPath(`configured-betting-${locale}.png`),fullPage:true})
    await page.goto(`${origins.user}/help`)
    const help = page.locator('.help-page')
    await expect(help.locator('details')).toHaveCount(6)
    await expect(page.locator('.workspace-page-title')).toHaveText(locale==='en'?'Help centre':'帮助中心')
    await expect(page.locator('.workspace-page-title')).toBeVisible()
    await expect(help.locator('.help-intro,a[href^="mailto:"]')).toHaveCount(0)
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
    const gameFilter = page.locator('.workspace-game-picker .select-control-trigger')
    await expect(page.locator('.history-filters select')).toHaveCount(0)
    for (const index of [0, 1, 2, 3]) {
      await gameFilter.click()
      const choices = page.locator('.workspace-game-picker [role="option"]')
      await expect(choices).toHaveCount(4)
      await choices.nth(index).click()
      const empty = page.locator('.order-history-empty')
      await expect(empty).toHaveText(locale === 'en' ? 'No bet orders match the current game and filters.' : '当前彩种和筛选条件下暂无投注记录。')
      await expect(page.locator('#user-page-refresh > button')).toHaveCount(0)
      await expectRefreshControls(page)
      layouts.push(await page.locator('.order-history').evaluate(el => {
        const empty = el.querySelector('.order-history-empty')!
        const rect = empty.getBoundingClientRect()
        // Browser transforms can introduce sub-pixel floating-point noise.
        const px = (value: number) => Math.round(value * 1000) / 1000
        return { left: px(rect.left), top: px(rect.top), width: px(rect.width), height: px(rect.height), gap: getComputedStyle(el).gap, align: getComputedStyle(empty).textAlign }
      }))
      await expectNoPageOverflow(page)
    }
    expect(layouts[1]).toEqual(layouts[0])
    expect(layouts[2]).toEqual(layouts[0])
    expect(layouts[3]).toEqual(layouts[0])
    const compactSpacing = await page.evaluate(() => matchMedia('(max-width:900px), (pointer:coarse)').matches)
    expect(layouts[0]).toMatchObject({ align: 'center', gap: compactSpacing ? '8px' : '12px' })
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
        await expect(page.locator('.draw-archive .archive-filter .select-control-trigger')).toHaveCount(1)
        await expect(page.locator('.draw-archive [role=tablist]')).toHaveCount(0)
        expect(await page.locator('.draw-archive input[type=text],.draw-archive input[type=search]').evaluateAll(inputs => inputs.filter(input => !input.closest('.select-control')).length)).toBe(0)
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
  test(`wallet overview and ledger balances keep point states distinct (${locale})`, async ({ page }, info) => {
    await installLayoutApi(page, locale);
    await installWalletLayoutApi(page);
    let ledgerRequests = 0;
    page.on('request', request => { if (new URL(request.url()).pathname === '/api/v1/wallet/ledger') ledgerRequests++; });
    await page.goto(`${origins.user}/wallet`);
    const wallet = page.locator('.wallet-summary');
    const headerRefresh = page.locator('#user-page-refresh > button');
    await expect(headerRefresh).toHaveCount(0);
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
    await expect(headerRefresh).toHaveCount(0);
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
    await expect(headerRefresh).toHaveCount(0);
    await expect(wallet.locator('.totals, .bucket-grid, .wallet-links')).toHaveCount(0);
    await expect(wallet.locator('.totals p, .source-head p, .panel-title p')).toHaveCount(0);
    await expect(wallet.locator('.ledger-type b')).toHaveText(locale === 'en' ? 'Points frozen' : '积分冻结');
    expect(ledgerRequests).toBeGreaterThan(0);
    // A freeze moves points between states: the net total is zero, while available points decrease.
    await expect(wallet.locator('.ledger-amount strong')).toHaveText('−10');
    await expect(wallet.locator('.ledger-balance-after')).toHaveText('120');
    await expect(wallet.locator('.ledger-details')).toHaveCount(0);
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expectRefreshControls(page);
    await expectNoPageOverflow(page);
    await page.screenshot({ path: info.outputPath(`wallet-ledger-${locale}.png`) });
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

import { registerMemberBatchDeleteTests } from './member-batch-delete'
registerMemberBatchDeleteTests(false)
