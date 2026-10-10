import type { Page } from '@playwright/test'

export async function selectLanguage(page: Page, code: string) {
  const picker = page.locator('.locale-picker').filter({ visible: true }).first()
  await picker.locator('.locale-picker__trigger').click()
  await picker.getByRole('button', { name: code === 'en' ? 'English' : '简体中文', exact: true }).click()
}
