import { expect, test } from '@playwright/test'
import { selectLanguage } from '../language-switcher'

for (const port of [5183, 5184, 5185]) {
  test(`shared language picker on ${port}`, async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.goto(`http://127.0.0.1:${port}/`)
    const picker = page.locator('.locale-picker').filter({ visible: true }).first()
    const trigger = picker.locator('.locale-picker__trigger')
    await expect(trigger).toBeVisible()
    await expect(trigger).toHaveAttribute('aria-expanded', 'false')
    await selectLanguage(page, 'en')
    await trigger.click()
    await expect(picker.getByRole('button', { name: 'English', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await picker.getByRole('button', { name: '简体中文', exact: true }).click()
    await expect(trigger).toHaveAccessibleName('语言')
    await trigger.click()
    await expect(picker.getByRole('button', { name: '简体中文', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await page.keyboard.press('Escape')
    await expect(trigger).toBeFocused()
    await expect(picker.locator('.locale-picker__list')).toHaveCount(0)
    await trigger.click()
    await page.locator('h1').first().click()
    await expect(picker.locator('.locale-picker__list')).toHaveCount(0)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true)
    expect(errors).toEqual([])
  })
}
