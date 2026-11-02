import type { Locator } from '@playwright/test'

/** Exercise the same visible popup used by mouse and touch users. */
export async function selectControl(trigger: Locator, choice: string | number | { value?: string; label?: string; index?: number }) {
  trigger = trigger.and(trigger.page().getByRole('combobox'))
  await trigger.click()
  const id = await trigger.getAttribute('aria-controls')
  if (!id) throw new Error('Expected the shared SelectControl trigger')
  const options = trigger.page().locator(`[id=${JSON.stringify(id)}]`).getByRole('option')
  const value = typeof choice === 'object' ? choice : { value: String(choice) }
  if (value.index !== undefined) await options.nth(value.index).click()
  else if (value.label !== undefined) await trigger.page().locator(`[id=${JSON.stringify(id)}]`).getByRole('option', { name: value.label, exact: true }).click()
  else if (value.value !== undefined) await options.and(trigger.page().locator(`[data-value=${JSON.stringify(value.value)}]`)).click()
  else throw new Error('Select a value, label or index')
}
