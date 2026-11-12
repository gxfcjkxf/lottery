import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'
import { realpathSync } from 'node:fs'

test.beforeEach(async ({ page }) => {
  // A local Vue fixture exercises real DOM/model/validation order without
  // authenticating or writing any business data.
  const shared = `/@fs/${resolve('shared')}`
  const vue = `/@fs/${realpathSync(resolve('shared/node_modules/vue/dist/vue.esm-bundler.js'))}`
  await page.route('**/__input-normalization', route => route.fulfill({
    contentType: 'text/html',
    body: `<div id="fixture"></div><script type="module">
      import { createApp } from '${vue}';
      import { installInputNormalization } from '${shared}/src/input-normalization.ts';
      import { vInlineValidation, setInlineValidationLocale } from '${shared}/src/form-validation.ts';
      window.stopNormalization = installInputNormalization(document);
      setInlineValidationLocale('en');
      createApp({
        directives: { inlineValidation: vInlineValidation },
        data: () => ({ client: '', password: '', notes: '', lazy: '', search: '', receipt: '', terms: false }),
        template: \`<form v-inline-validation @submit.prevent="receipt = JSON.stringify({ client, password, notes, lazy })">
          <input aria-label="Client" v-model="client" pattern="[1-9][0-9]*" required>
          <input aria-label="Password" type="password" v-model="password">
          <textarea aria-label="Notes" v-model="notes"></textarea>
          <input aria-label="Lazy" v-model.lazy="lazy">
          <input aria-label="Terms" type="checkbox" v-model="terms">
          <button>Save</button><output>{{ receipt }}</output>
        </form><input aria-label="Search" v-model="search" @keydown.enter="receipt = search">
        <span data-search>{{ search }}</span>\`
      }).mount('#fixture');
    </script>`,
  }))
  await page.goto('http://localhost:5173/__input-normalization')
  await expect(page.getByRole('button', { name: 'Save' })).toBeVisible()
})

test('cleans before validation and synchronizes regular, password, textarea and lazy models', async ({ page }) => {
  await page.getByLabel('Client', { exact: true }).fill(' \u200b9267434645\u2060 ')
  await page.getByLabel('Password').fill('  pass word  ')
  await page.getByLabel('Notes').fill('\t\n hello\nworld \n\t')
  await page.getByLabel('Lazy').fill('  lazy value  ')
  // Submit without blur: the currently focused lazy binding must also update.
  await page.locator('form').evaluate((form: HTMLFormElement) => form.requestSubmit())
  await expect(page.locator('output')).toHaveText(JSON.stringify({
    client: '9267434645', password: 'pass word', notes: 'hello\nworld', lazy: 'lazy value',
  }))
  await expect(page.locator('.inline-validation-error')).toHaveCount(0)
  await expect(page.getByLabel('Client', { exact: true })).toHaveValue('9267434645')
})

test('cleans pasted values before input handlers and standalone Enter actions', async ({ page }) => {
  const search = page.getByLabel('Search')
  await search.evaluate((control: HTMLInputElement) => {
    control.value = '\u200b\t copied words \u00a0\u2060'
    control.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertFromPaste' }))
  })
  await expect(search).toHaveValue('copied words')
  await expect(page.locator('[data-search]')).toHaveText('copied words')
  await search.fill('  next words  ')
  await search.press('Enter')
  await expect(page.locator('output')).toHaveText('next words')
})

test('preserves word spacing during typing and ignores composition and non-text controls', async ({ page }) => {
  const search = page.getByLabel('Search')
  await search.pressSequentially('two words ')
  await expect(search).toHaveValue('two words ')
  await search.evaluate(control => control.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, isComposing: true })))
  await expect(search).toHaveValue('two words ')
  await page.getByLabel('Terms').check()
  await expect(search).toHaveValue('two words')
  await expect(page.getByLabel('Terms')).toBeChecked()
})
