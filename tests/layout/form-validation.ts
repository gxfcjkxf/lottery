import { selectControl } from '../select-control'
import { expect, test } from '@playwright/test'
import { resolve } from 'node:path'
import { expectNoPageOverflow, installLayoutApi, origins } from './fixture'

export function registerFormValidationTests() {
  for (const locale of ['en', 'zh-CN'] as const) {
    test(`four login forms explain invalid passwords without submitting (${locale})`, async ({ page }, info) => {
      await installLayoutApi(page, locale)
      const writes: string[] = []
      page.on('request', request => { if (request.url().includes('/api/') && request.method() !== 'GET') writes.push(request.url()) })
      for (const surface of ['user', 'brand', 'platform', 'agent'] as const) {
        await page.goto(origins[surface] + (surface === 'user' ? '/login' : ''))
        const form = page.locator('form').first()
        await form.locator('input[autocomplete=username]').fill('validation_operator')
        const password = form.locator('input[type=password]').first()
        await password.fill('')
        const submit = form.locator('button[type=submit], button:not([type])').last()
        await submit.click()
        const error = form.locator('.inline-validation-error')
        await expect(error).toContainText(locale === 'en' ? /[Pp]assword/ : '密码')
        await expect(error).not.toContainText(/填写内容过短|填写内容过长|The value is too short|The value is too long|必填项/)
        if (surface === 'agent') {
          await password.pressSequentially('123')
          await expect(form).not.toHaveAttribute('data-write-locked')
          await submit.click()
          await expect(error).toContainText(locale === 'en' ? 'Password must be 6–30 characters.' : '密码长度须为 6–30 个字符')
          await expect(password).toHaveAttribute('aria-describedby', /inline-validation-/)
          const other = locale === 'en' ? 'zh-CN' : 'en'
          await page.locator('.locale-picker__trigger').click()
          await page.locator(`.locale-picker__option[data-code="${other}"]`).click()
          await expect(error).toContainText(other === 'en' ? 'Password must be 6–30 characters.' : '密码长度须为 6–30 个字符')
          await page.screenshot({ path: info.outputPath(`agent-password-validation-${other}.png`) })
          await password.fill('valid-password')
          await expect(error).toHaveCount(0)
          await expect(password).not.toHaveAttribute('aria-invalid')
        }
        await expectNoPageOverflow(page)
      }
      expect(writes).toEqual([])
    })
  }
  for (const surface of ['user', 'brand', 'platform', 'agent'] as const) {
    test(`shared form requirements and recovery remain explicit (${surface})`, async ({page}, info) => {
      const directory = surface === 'brand' ? 'admin-web' : surface + '-web'
      await page.route('**/api/**', route => { throw new Error('The form audit must not access a business API: ' + route.request().url()) })
      await page.route('**/__form-errors', route => route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><body><div id="app" class="${surface==='user'?'':'management-console'}"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import Harness from'/@fs${resolve('tests/layout/FormValidationHarness.vue')}';import'/@fs${resolve('shared/src/styles.css')}';${surface==='agent'?'':`import'/@fs${resolve(directory+'/src/style.css')}';`}import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/form-validation.css')}';${surface==='user'?'':`import'/@fs${resolve('shared/src/management.css')}';`}createApp(Harness).mount('#app')</script></body></html>`}))
      await page.goto(origins[surface]+'/__form-errors')
      const form=page.locator('.validation-harness > form'),error=form.locator('.inline-validation-error'),save=page.locator('#save')
      const account=page.getByRole('textbox',{name:'Account',exact:true}),password=form.locator('input[type=password]').first()
      await account.fill('INVALID ACCOUNT');await page.locator('#amount').focus();await expect(error).toContainText('3–32');await expect(error).toContainText('lowercase')
      await account.fill('valid_account');await expect(error).toHaveCount(0)
      await password.fill('123');await page.locator('#amount').focus();await expect(error).toContainText('6–30 characters')
      await password.fill('😀'.repeat(31));await page.locator('#amount').focus();await expect(error).toContainText('6–30 characters')
      await password.fill('密码密码密码');await expect(error).toHaveCount(0)
      const displayName=page.locator('#display-name'),notes=page.locator('#notes')
      await expect(displayName).not.toHaveAttribute('maxlength')
      await displayName.fill('😀'.repeat(120));await page.locator('#amount').focus();await expect(error).toHaveCount(0)
      await expect(displayName).toHaveValue('😀'.repeat(120))
      await displayName.fill('😀'.repeat(121));await page.locator('#amount').focus();await expect(error).toContainText('120 characters')
      await displayName.fill('显示名称');await expect(error).toHaveCount(0)
      await notes.fill('界'.repeat(500));await page.locator('#amount').focus();await expect(error).toHaveCount(0)
      await notes.fill('😀'.repeat(501));await page.locator('#amount').focus();await expect(error).toContainText('500 characters')
      await notes.fill('😀'.repeat(500));await expect(error).toHaveCount(0)

      for(const [value,text] of [['101','must not exceed 100'],['0','must be at least 1'],['1.25','increments of 0.5']]){
        await page.locator('#amount').fill(value!);await page.locator('#email').focus();await expect(error).toContainText(text!)
      }
      await page.locator('#amount').fill('2.5');await expect(error).toHaveCount(0)
      await page.locator('#email').fill('invalid');await page.locator('#url').focus();await expect(error).toContainText('name@example.com');await page.locator('#email').fill('name@example.com')
      await page.locator('#url').fill('invalid');await page.locator('#email').focus();await expect(error).toContainText('https://example.com');await page.locator('#url').fill('https://example.com')
      await selectControl(page.locator('#status'), '');await save.click();await expect(error).toHaveText('Status is required.');await selectControl(page.locator('#status'), 'active')
      await page.locator('#terms').uncheck();await save.click();await expect(error).toHaveText('Terms is required.');await page.locator('#terms').check()
      await expect(page.locator('#saves')).toHaveText('0');await save.click();await expect(page.locator('#saves')).toHaveText('1')
      await page.locator('#amount').fill('101');await page.locator('#email').focus();await page.locator('#language').click();await expect(error).toHaveText('积分不得大于 100。');await expectNoPageOverflow(page)
      await page.locator('#amount').fill('2');await page.locator('#change').click()
      const dialog=page.getByRole('dialog'),next=dialog.locator('input[autocomplete=new-password]').first(),confirmation=dialog.getByLabel('确认新密码',{exact:true})
      await expect(next).toHaveAttribute('aria-label','新密码')
      await next.fill('valid-password');await confirmation.fill('different-password');await dialog.locator('button[type=submit]').click();await expect(dialog.getByRole('alert')).toHaveText('两次输入的新密码不一致，请重新确认。');await expect(page.locator('#password-saves')).toHaveText('0')
      await confirmation.fill('valid-password');await expect(dialog.getByRole('alert')).toHaveCount(0)
      await next.fill('123');await confirmation.focus();await expect(dialog.getByRole('alert')).toContainText('新密码长度须为 6–30 个字符')
      await next.fill('密码密码密码');await confirmation.fill('密码密码密码');await dialog.locator('button[type=submit]').click();await expect(dialog).toHaveCount(0);await expect(page.locator('#password-saves')).toHaveText('1')
      const preset=page.viewportSize()!
      if(info.project.name.endsWith('reflow'))await page.setViewportSize({width:320,height:720})
      await password.fill('123');await page.locator('#amount').focus();await expectNoPageOverflow(page)
      await page.screenshot({path:info.outputPath(`form-errors-${surface}.png`),fullPage:true})
      if(info.project.name.endsWith('reflow'))await page.setViewportSize(preset)
    })
  }

}
