import { expect, test } from '@playwright/test'
import { gameId, origins, installLayoutApi, expectNoPageOverflow } from './fixture'

const at = '2026-10-21T00:00:00Z'

export function registerBrandSettingsTests(reflow = false) {
  for (const locale of ['en', 'zh-CN'] as const) {
    test(`brand settings save presentation without domain controls (${locale})`, async ({ page }, info) => {
      const zh = locale === 'zh-CN'
      await installLayoutApi(page, locale)
      await page.route('**/api/v1/brand-assets/*.png', route => route.fulfill({ contentType: 'image/png', body: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg==', 'base64') }))
      const errors: string[] = [], requests: string[] = []
      page.on('pageerror', error => errors.push(error.message))
      const effective = { display_name: 'Aurora', logo_text: 'A', logo_url: null, favicon_url: null,
        default_locale: 'en',
        available_locales: ['en', 'zh-CN'], content: { en: { tagline: '', announcement: '' }, 'zh-CN': { tagline: '', announcement: '' } } }
      let presentation = { brand_id: gameId, version: 4, status: 'active', base_name: 'Aurora', config: Object.fromEntries(Object.keys(effective).map(key => [key, null])), effective, updated_at: at, audit_log_id: gameId }
      let policy = { brand_id: gameId, version: 4, config: { age_enabled: false, minimum_age: null, region_enabled: false, allowed_countries: [], identity_enabled: false, account_risk_enabled: false, betting_risk_enabled: false, exclusion_enabled: false, responsible_gambling_enabled: false }, updated_at: at, audit_log_id: gameId }
      const auth = { version: 4, captcha_enabled: false, telegram_enabled: false, telegram_client_id: '', }
      const attempts: { path: string; body: string | null; key: string | undefined }[] = []
      let loseReceipt = true
      const receipts = new Map<string, typeof presentation>()
      await page.route('**/api/v1/admin/**', async route => {
        const req = route.request(), path = new URL(req.url()).pathname
        requests.push(`${req.method()} ${path}`)
        const reply = (data: unknown, status = 200) => route.fulfill({ status, json: { success: true, data } })
        if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'settings_reviewer', super_admin: false, brand_ids: [gameId], permissions: [], permissions_by_brand: { [gameId]: ['brand_presentation.view.brand', 'brand_presentation.write.brand', 'brand_domains.view.brand', 'brand_domains.write.brand', 'auth_config.view.brand', 'auth_config.write.brand', 'compliance_policy.view.brand', 'compliance_policy.write.brand', 'compliance_check.view.brand', 'compliance_check.run.brand'] }, platform_permissions: [] } })
        if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, name: 'Aurora', code: 'aurora', status:'active',member_count:'12',agent_count:'3',recharge_quota_available:'1000',created_at:'2026-10-24T00:00:00Z' }] })
        expect(req.headers()['x-brand-id']).toBe(gameId)
        if (req.method() === 'GET') {
          if (path.endsWith('/brand-presentation')) return reply(presentation)
          if (path.endsWith('/brand-presentation/defaults')) return reply(effective)
          if (path.endsWith('/auth-settings')) return reply(auth)
          if (path.endsWith('/compliance-policy')) return reply(policy)
          throw Error(`Unexpected settings read: ${path}`)
        }
        if (path.endsWith('/brand-presentation/logo')) return reply({ url: '/api/v1/brand-assets/' + 'a'.repeat(64) + '.png' })
        const body = req.postDataJSON(), key = req.headers()['idempotency-key']
        attempts.push({ path, body: req.postData(), key })
        expect(key).toBeTruthy()
        if(req.method()!=='DELETE')expect(body.reason).toBe('')
        if (path.endsWith('/brand-presentation')) {
          expect(req.method()).toBe('PUT')
          if (receipts.has(key)) return reply(receipts.get(key))
          expect(body.version).toBe(presentation.version)
          presentation = { ...presentation, version: body.version + 1, config: body.config, effective: { ...Object.fromEntries(Object.entries(effective).map(([key,value]) => [key, body.config[key] ?? value])) } as typeof effective }
          receipts.set(key, presentation)
          if (loseReceipt) { loseReceipt = false; return route.abort('failed') }
          return reply(presentation)
        }
        if (path.endsWith('/compliance-policy')) {
          expect(req.method()).toBe('PUT')
          expect(body.version).toBe(policy.version)
          policy = { ...policy, version: body.version + 1, config: body.config }
          return reply(policy)
        }
        if (path.endsWith('/compliance-checks')) {
          expect(req.method()).toBe('POST')
          expect(body.version).toBe(policy.version)
          return reply({ id: gameId, brand_id: gameId, policy_version: policy.version, config: policy.config, operation: body.operation, decision: 'allow', checks: ['age', 'region', 'identity', 'account_risk', 'betting_risk', 'exclusion', 'responsible_gambling'].map(check => ({ check, enabled: false, decision: 'allow', reason_code: 'CHECK_DISABLED' })), adapter_mode: 'stub', created_by: gameId, reason: '', audit_log_id: gameId, created_at: at }, 201)
        }
        throw Error(`Unexpected settings write: ${path}`)
      })
      await page.goto(origins.brand)
      await expect(page.locator('.management-console')).toBeVisible()
      if (await page.locator('.mobile-nav').isVisible()) {
        await page.locator('.mobile-nav button').last().click()
        await page.locator('.mobile-more-menu').getByRole('button', { name: zh ? '品牌设置' : 'Brand settings', exact: true }).click()
      } else await page.locator('.side-nav').getByRole('button', { name: zh ? '品牌设置' : 'Brand settings', exact: true }).click()
      const tabs = page.locator('.console-tabs button')
      const dialog = page.getByRole('dialog', { name: zh ? '确认设置变更' : 'Confirm changes', exact: true })
      const dismissToasts = async () => { while (await page.locator('.toast-close').count()) await page.locator('.toast-close').first().click() }
      const preset = page.viewportSize()!
      const checkLayout = async (name: string) => {
        for (const width of reflow ? [320, 359, 360, 361, 640, 699, 700, 701, 900, 1440] : [preset.width]) {
          if (reflow) await page.setViewportSize({ width, height: preset.height })
          await expectNoPageOverflow(page)
        }
        await expect(page.locator('.brand-settings-panel .refresh-icon-control, .presentation-history, .compliance .records, .auth-settings .audit')).toHaveCount(0)
        await page.screenshot({ path: info.outputPath(`settings-${name}-${locale}.png`) })
      }
      await tabs.nth(1).click()
      const display = page.locator('.brand-presentation')
      await expect(display.locator('.presentation-fields')).toBeVisible()
      await checkLayout('presentation')
      const nameWidth = await display.locator('.brand-name-field input').evaluate(input => input.getBoundingClientRect().width)
      expect(nameWidth).toBeLessThanOrEqual(400)
      const actions = await display.locator('.reset-settings').evaluate(row => {
        const buttons = [...row.querySelectorAll('button')].map(button => button.getBoundingClientRect())
        return { width: row.clientWidth, buttons: buttons.map(rect => ({width: rect.width, top: rect.top})) }
      })
      expect(actions.buttons).toHaveLength(2)
      if (actions.width >= actions.buttons[0].width + actions.buttons[1].width + 8) expect(actions.buttons[0].top).toBe(actions.buttons[1].top)

      await expect(tabs).toHaveCount(7)
      await expect(display.locator('.inherit-toggle')).toHaveCount(0)
      await expect(display.locator('details')).not.toHaveAttribute('open')
      await display.locator('details summary').click()
      await expect(display.locator('details input[type=color]')).toHaveCount(5)
      await expect(display.locator('details input[type=color]').first()).toBeVisible()
      await checkLayout('presentation-advanced')
      await display.locator('details summary').click()
      await display.locator('input[type=file]').setInputFiles({ name: 'logo.png', mimeType: 'image/png', buffer: Buffer.from('synthetic image') })
      await expect(display.locator('.logo-editor img')).toHaveAttribute('src', /brand-assets/)
      await expect.poll(() => display.locator('.logo-editor img').evaluate((node: HTMLImageElement) => node.complete && node.naturalWidth > 0)).toBe(true)
      await display.getByRole('button', { name: zh ? '恢复默认' : 'Restore defaults', exact: true }).click()
      await display.getByRole('button', { name: zh ? '确认恢复' : 'Confirm reset', exact: true }).click()
      await expect(display.locator('.logo-editor img')).toHaveCount(0)
      await expect(display.locator('.presentation-preview')).toContainText('Aurora')
      await display.locator('input[type="text"]').first().fill('Aurora preview')
      await display.locator('button[type="submit"]').click()
      await expect(page.locator('.toast-error')).toBeVisible()
      await expect(dialog).toHaveCount(0)
      await dismissToasts()
      await display.getByRole('button', {name:zh?'重新读取':'Reload',exact:true}).click()
      await expect(display.locator('input[type="text"]').first()).toHaveValue('Aurora preview')
      await expect(display.locator('form')).not.toHaveAttribute('data-write-locked')
      await display.locator('button[type="submit"]').click()
      await expect(page.locator('.toast-success')).toContainText(zh ? '设置已保存' : 'Settings saved')
      await expect(dialog).toHaveCount(0)
      expect(attempts).toHaveLength(2)
      expect(attempts[0].key).not.toBe(attempts[1].key)
      await dismissToasts()
      await expect(tabs).toHaveCount(7)
      await expect(tabs.filter({hasText:zh?'域名':'Domains'})).toHaveCount(0)
      expect(requests.some(request => request.includes('brand-domains'))).toBe(false)
      expect(requests.some(request => /auth-settings|compliance/.test(request))).toBe(false)
      expect(requests.filter(request => /history|gates|GET.*compliance-checks/.test(request))).toEqual([])
      expect(errors).toEqual([])
    })
  }
}
