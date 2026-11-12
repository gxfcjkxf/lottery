import { test, expect } from '@playwright/test'
import { installLayoutApi, origins, expectNoPageOverflow } from './fixture'

for (const locale of ['en', 'zh-CN'] as const) test(`compact authentication keeps fields inline and actions reachable ${locale}`, async ({ page }, info) => {
  await installLayoutApi(page, locale)
  const preset = page.viewportSize()!
  for (const origin of Object.values(origins)) {
    for (const width of info.project.name.includes('reflow') ? [320, 390, 700] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: 720 })
      await page.goto(origin + (origin === origins.user ? '/login' : ''))
      const card = page.locator('.login-entry__card')
      await expect(card).toBeVisible()
      const form = card.locator('form')
      const geometry = await card.evaluate(el => {
        const fields = [...el.querySelectorAll('.login-entry__field')].map(field => {
          const label = field.querySelector('.login-entry__label')!.getBoundingClientRect()
          const input = field.querySelector('input')!.getBoundingClientRect()
          return { inline: label.right <= input.left, height: input.height }
        })
        const submit = el.querySelector('form button[type="submit"], form button:not([type])')!.getBoundingClientRect()
        return { fields, submitBottom: submit.bottom, viewportHeight: innerHeight, cardHeight: el.getBoundingClientRect().height }
      })
      for (const field of geometry.fields) { expect(field.inline).toBe(true); expect(field.height).toBeGreaterThanOrEqual(44) }
      if (preset.height >= 600 || info.project.name.includes('reflow')) expect(geometry.submitBottom).toBeLessThanOrEqual(geometry.viewportHeight)
      await expectNoPageOverflow(page)
      await form.locator('input[type=password]').fill('layout-only-password')
      await page.screenshot({ path: info.outputPath(`compact-${new URL(origin).port}-${locale}-${width}.png`), fullPage: true })
    }
  }
})
