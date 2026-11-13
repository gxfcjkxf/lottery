import { expect, type Page, type Locator } from '@playwright/test'

export async function tokenStyle(element: Locator, property: string, token: string) {
  return element.evaluate((node, { property, token }) => {
    const probe = document.createElement('span')
    probe.style.setProperty(property, `var(${token})`)
    node.append(probe)
    const value = getComputedStyle(probe).getPropertyValue(property)
    probe.remove()
    return value
  }, { property, token })
}
export async function expectNavigationStyle(page: Page, selectedSelector: string) {
  const sidebar = page.locator('.sidebar')
  await expect(sidebar).toHaveCSS('background-color', await tokenStyle(sidebar, 'background-color', '--console-sidebar-bg'))
  await expect(sidebar).toHaveCSS('background-image', await tokenStyle(sidebar, 'background-image', '--console-sidebar-art'))
  const selected = page.locator(selectedSelector).first()
  await expect(selected).toHaveCSS('background-color', await tokenStyle(selected, 'background-color', '--console-sidebar-active-bg'))
  await expect(selected).toHaveCSS('color', await tokenStyle(selected, 'color', '--console-sidebar-active-ink'))
  await expect(selected).toHaveCSS('font-weight', '650')
  await expect(selected.locator('.site-icon').first()).toHaveCSS('color', await tokenStyle(selected, 'color', '--primary'))
  const item = sidebar.locator('.nav-item:not(.active):not(.selected)').first()
  await expect(item).toHaveCSS('color', await tokenStyle(item, 'color', '--console-sidebar-ink'))
  await expect(item.locator('.site-icon').first()).toHaveCSS('color', await tokenStyle(item, 'color', '--console-sidebar-icon'))
}
