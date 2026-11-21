import { expect, type Locator } from '@playwright/test'

export async function expectDialogFieldColumns(dialog: Locator) {
 const rows = await dialog.locator('.ui-form > label, .ui-form > fieldset > label, .ui-form > fieldset > .ui-form-field, label.ui-field-row, .ui-form > .form-row > label, .cash-account-field, .period-form-fields > div.period-field').evaluateAll(labels => labels.flatMap(label => {
  const control = label.querySelector(':scope > input:not([type="checkbox"], [type="radio"]), :scope > textarea, :scope > .select-control, :scope > .association-search, :scope > .association-filter, :scope > .am-ratio-input, :scope > .platform-temporal-input, :scope > .ui-form-field-value, :scope > .cash-account-control, :scope > .period-time-controls')
  if (!control) return []
  const text = [...label.childNodes].find(node => node.nodeType === Node.TEXT_NODE ? Boolean(node.textContent?.trim()) : node instanceof HTMLElement && node !== control && !node.matches('small, .field-error, .inline-validation-error') && Boolean(node.textContent?.trim()))!
  const range = document.createRange()
  range.selectNodeContents(text)
  const factLabel = label.closest('.detail-modal')?.querySelector('dl dt'), factValue = label.closest('.detail-modal')?.querySelector('dl dd')
  const title = range.getBoundingClientRect(), field = control.getBoundingClientRect(), row = label.getBoundingClientRect()
  return [{ factLeft: factLabel?.getBoundingClientRect().left, factRight: factValue?.getBoundingClientRect().right, facts: Boolean(label.closest('.ui-facts-form')), numeric: control.matches('input[type="number"], input[inputmode="decimal"], input.ui-number-input, .am-ratio-input'), titleLeft: title.left, rowLeft: row.left, fieldWidth: field.width, align: getComputedStyle(label.matches('.cash-account-field') ? label.querySelector('.cash-account-label')! : label).textAlign, titleRight: title.right, fieldLeft: field.left, fieldRight: field.right, rowRight: row.right, titleTop: title.top, titleBottom: title.bottom, fieldTop: field.top, fieldBottom: field.bottom }]
 }))
 expect(rows.length).toBeGreaterThan(0)
 for (const row of rows) {
  if (row.facts) {
   expect(['start', 'left']).toContain(row.align)
   expect(Math.abs(row.titleLeft - row.rowLeft)).toBeLessThanOrEqual(1)
   expect(Math.abs(row.fieldRight - row.rowRight)).toBeLessThanOrEqual(1)
   if (row.factLeft !== undefined) expect(Math.abs(row.titleLeft - row.factLeft)).toBeLessThanOrEqual(1)
   if (row.factRight !== undefined) expect(Math.abs(row.fieldRight - row.factRight)).toBeLessThanOrEqual(1)
  } else {
   expect(['end', 'right']).toContain(row.align)
   expect(Math.abs(row.fieldLeft - rows[0]!.fieldLeft)).toBeLessThanOrEqual(1)
   expect(Math.abs(row.fieldLeft - row.titleRight - 8)).toBeLessThanOrEqual(1)
  }
  if (row.numeric) expect(row.fieldWidth).toBeLessThanOrEqual(160)
  expect(row.fieldRight).toBeLessThanOrEqual(row.rowRight + 1)
  expect(row.fieldTop).toBeLessThan(row.titleBottom)
  expect(row.titleTop).toBeLessThan(row.fieldBottom)
 }
}
