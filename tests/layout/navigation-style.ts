import { expect, type Page } from '@playwright/test'

export async function expectNavigationStyle(page: Page, selectedSelector: string) {
  const sidebar = page.locator('.sidebar')
  await expect(sidebar).toHaveCSS('background-color', 'rgb(255, 255, 255)')
  await expect(sidebar).toHaveCSS('background-image', 'none')
  const selected = page.locator(selectedSelector).first()
  await expect(selected).toHaveCSS('background-color', 'rgb(236, 253, 245)')
  await expect(selected).toHaveCSS('color', 'rgb(4, 120, 87)')
  await expect(selected).toHaveCSS('font-weight', '650')
  await expect(selected.locator('.site-icon')).toHaveCSS('color', 'rgb(5, 150, 105)')
  const item = sidebar.locator('.nav-item:not(.active):not(.selected)').first()
  await expect(item).toHaveCSS('color', 'rgb(75, 85, 99)')
  await expect(item.locator('.site-icon')).toHaveCSS('color', 'rgb(107, 114, 128)')
}
