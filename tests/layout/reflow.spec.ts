import { registerConfiguredSelectionTests } from './configured-selection'
registerConfiguredSelectionTests()
import { registerPlatformBrandNavigationTests } from './platform-brand-navigation'
import { registerPlatformOverviewTests } from './platform-overview'
import { registerBrandOverviewTests } from './brand-overview'
registerBrandOverviewTests(true)
registerPlatformOverviewTests(true)
registerPlatformBrandNavigationTests(true)
import { expect, test } from '@playwright/test'
import { registerBuiltinRoleTests } from './builtin-roles'
registerBuiltinRoleTests()
import { registerAssociationNameTests } from './association-names'
registerAssociationNameTests()
import { registerPublicErrorTests } from './public-errors'
registerPublicErrorTests()
import { origins, gameId, installLayoutApi, installOrdersLayoutApi, expectNoPageOverflow, expectInsideViewport, expectRefreshControls, expectCompactDisplayRows } from './fixture'

// Extra widths are layout boundaries, not claims about particular phone models.
const widths = [...new Set([
  ...Array.from({ length: 9 }, (_, i) => 320 + i * 20),
  ...[620, 640, 700, 768, 820, 900, 1000, 1080, 1200, 1440].flatMap(width => [width - 1, width, width + 1]),
  1024,
])].sort((a, b) => a - b)

for (const locale of ['en', 'zh-CN'] as const) {
 for (const group of ['entry', 'account', 'betting'] as const) {
  test(`continuous widths and breakpoints remain usable (${locale}, ${group})`, async ({ page }, info) => {
    await installLayoutApi(page, locale)
    const pages = {
      entry: [`${origins.user}/login`, origins.agent, origins.brand, origins.platform, `${origins.user}/help`],
      account: [`${origins.user}/orders`, `${origins.user}/wallet`, `${origins.user}/wallet/buckets`, `${origins.user}/wallet/ledger`, `${origins.user}/notifications`, `${origins.user}/results`],
      betting: [`${origins.user}/configured/${gameId}`],
    }
    // Keep the same width coverage in independent page groups within the normal timeout.
    if (group !== 'entry') await installOrdersLayoutApi(page)
    for (const url of pages[group]) {
      if (group === 'entry' && url.endsWith('/help')) await installOrdersLayoutApi(page)
      await page.goto(url)
      if (url.includes('/configured/')) await expect(page.locator('.configured-options button')).toHaveCount(49)
      else if (url.endsWith('/help')) await expect(page.locator('.faq-list details')).toHaveCount(6)
      else if (url.endsWith('/orders')) await expect(page.locator('.order-history-empty')).toBeVisible()
      else if (/\/wallet(?:\/buckets)?$/.test(url)) await expect(page.locator(url.endsWith('/buckets') ? '.bucket-grid' : '.wallet-links')).toBeVisible()
      else if (/\/(ledger|notifications|results)$/.test(url)) await expect(page.locator('.empty-state').filter({ visible: true }).first()).toBeVisible()
      else await expect(page.locator('form').first()).toBeVisible()
      if (url.endsWith('/results')) await expect(page.locator('.dr-disclaimer')).toHaveCount(0)
      if(group==='account')await page.locator('.workspace-topbar > button.icon-button').click()
      for (const width of widths) {
        await test.step(`${url}: ${width}×720`, async () => {
          await page.setViewportSize({ width, height: 720 })
          await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
          await expectNoPageOverflow(page)
          if (width === 320 && [`${origins.user}/login`, origins.brand, origins.platform].includes(url)) await expectCompactDisplayRows(page)
          if (/\/(orders|wallet|notifications|results)$/.test(url)) await expectRefreshControls(page)
          if (url.includes('/configured/')) {
            await expect(page.locator('.configured-selection')).toBeVisible()
          }
        })
      }
    }
    await page.screenshot({ path: info.outputPath(`reflow-${locale}-${group}.png`) })
    await info.attach('reflow.json', { body: JSON.stringify({ widths, heights: group === 'betting' ? [320, 420, 568, 900] : [] }), contentType: 'application/json' })
  })
 }
}

import { registerBrandOperationLayoutTests } from './brand-operation'
registerBrandOperationLayoutTests(true)
import { registerMemberDeleteTests } from './member-delete'
registerMemberDeleteTests(true)

test('half-screen menu keeps language, refresh and close inline at 320px',async({page})=>{
 await installLayoutApi(page,'zh-CN');await installOrdersLayoutApi(page)
 await page.setViewportSize({width:320,height:568})
 await page.goto(`${origins.user}/wallet`);await expect(page.locator('.wallet-links')).toBeVisible()
 await page.locator('.workspace-topbar').getByRole('button',{name:'菜单',exact:true}).click()
 const menu=page.locator('.workspace-menu'),language=menu.locator('.locale-picker__trigger'),refresh=menu.locator('#user-page-refresh button'),close=menu.locator('.workspace-menu-close')
 await expect.poll(()=>menu.evaluate(el=>Math.abs(el.getBoundingClientRect().left))).toBeLessThan(1)
 for(const control of [language,refresh,close]) await expectInsideViewport(control)
 const boxes=await Promise.all([language,refresh,close].map(control=>control.boundingBox()))
 for(const box of boxes){expect(box!.width).toBeGreaterThanOrEqual(44);expect(box!.y).toBeCloseTo(boxes[0]!.y,0)}
 expect(boxes[2]!.x+boxes[2]!.width).toBeLessThanOrEqual(160)
 await language.click()
 const list=menu.locator('.locale-picker__list');await expectInsideViewport(list)
 const bounds=(await list.boundingBox())!
 expect(bounds.x).toBeGreaterThanOrEqual(0);expect(bounds.x+bounds.width).toBeLessThanOrEqual(160)
 await expect(list.locator('button')).toHaveCount(2)
 await expectNoPageOverflow(page)
})


import {registerConfiguredSaleTests} from './workspace'
registerConfiguredSaleTests()
import {registerWorkspaceTests} from './workspace'
registerWorkspaceTests()

import {registerConfiguredHistoryLayoutTests} from './workspace'
registerConfiguredHistoryLayoutTests()

import { registerBrandStatusTests } from './brand-status'
registerBrandStatusTests(true)

import { registerBrandSettingsTests } from './brand-settings'
registerBrandSettingsTests(true)

import { registerSecuritySettingsTests } from './security-settings'
registerSecuritySettingsTests(true)

import { registerActionEditorTests } from "./action-editors"
registerActionEditorTests(true)

import { registerPlatformSettingsEditorTests } from "./platform-settings"
registerPlatformSettingsEditorTests(true)

import { registerAgentNavigationTests } from "./agent-navigation"
registerAgentNavigationTests(true)

import { registerManagementButtonTests } from "./management-buttons"
registerManagementButtonTests()

import { registerMemberManagementTests } from "./member-management"
registerMemberManagementTests(true)

import { registerPlatformMemberManagementTests } from "./platform-member-management"
registerPlatformMemberManagementTests(true)

import { registerCompactListTests } from './compact-lists'
registerCompactListTests(true)

import { registerAgentDirectoryTests } from "./agent-directory"
registerAgentDirectoryTests(true)

import { registerControlSizingTests } from "./control-sizing"
registerControlSizingTests(true)

import { registerAgentModeChangeTests } from './agent-mode-change'
registerAgentModeChangeTests(true)

import { registerAgentPortalTests } from './agent-portal'
registerAgentPortalTests(true)
import { registerSharedAccountMenuTests } from './agent-portal'
registerSharedAccountMenuTests()
