import { test, expect } from '@playwright/test'
import { createRequire } from 'node:module'
import { installLayoutApi, installOrdersLayoutApi, origins, gameId, expectNoPageOverflow } from './fixture'
import { selectControl } from '../select-control'
const sharedRequire = createRequire(process.cwd() + '/shared/package.json')
let image: Buffer
test.beforeAll(async () => { image = await sharedRequire('qrcode').toBuffer('https://t.me/brand_support', { width: 320, margin: 2 }) })
const asset = '/api/v1/brand-assets/' + 'a'.repeat(64) + '.png'
const ids = ['bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1', 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2', 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb3']
const contacts = () => ({ items: [
  { id: ids[0], kind: 'wechat', value: 'brand_wechat', image_url: asset },
  { id: ids[1], kind: 'telegram', value: '@brand_support', image_url: null as string | null },
  { id: ids[2], kind: 'whatsapp', value: '+6581234567', image_url: null as string | null },
], default_id: ids[0] as string | null })

test('user support dragging stays visible and separates dragging from opening', async ({ page, browserName, isMobile }, info) => {
  await installLayoutApi(page, 'en')
  await installOrdersLayoutApi(page)
  await page.route('**/api/v1/brand-contacts', route => route.fulfill({ json: { success: true, data: contacts() } }))
  await page.route('**/api/v1/brand-assets/*', route => route.fulfill({ contentType: 'image/png', body: image }))
  await page.goto(`${origins.user}/account`)
  const support = page.getByRole('button', { name: 'Support', exact: true })
  await expect(support).toBeVisible()
  const initial = (await support.boundingBox())!
  const start = { x: initial.x + initial.width / 2, y: initial.y + initial.height / 2 }
  if (browserName === 'chromium' && isMobile) {
    const session = await page.context().newCDPSession(page)
    await session.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [start] })
    await session.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: start.x - 100, y: start.y + 60 }] })
    await session.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
    await session.detach()
  } else {
    await page.mouse.move(start.x, start.y)
    await page.mouse.down()
    await page.mouse.move(start.x - 100, start.y + 60, { steps: 8 })
    await page.mouse.up()
  }
  await expect(page.getByRole('dialog')).toHaveCount(0)
  const moved = (await support.boundingBox())!
  expect(moved.x).toBeCloseTo(initial.x - 100, 0)
  expect(moved.y).toBeCloseTo(initial.y + 60, 0)
  await page.screenshot({ path: info.outputPath('support-dragged.png') })
  if (isMobile) await support.tap()
  else await support.click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.getByRole('dialog').getByRole('button', { name: 'Close', exact: true }).click()
  await page.mouse.move(moved.x + moved.width / 2, moved.y + moved.height / 2)
  await page.mouse.down()
  const viewport = page.viewportSize()!
  await page.mouse.move(viewport.width + 100, viewport.height + 100, { steps: 8 })
  await page.mouse.up()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  if (info.project.name.includes('reflow')) await page.setViewportSize({ width: 320, height: 480 })
  await expect.poll(async () => {
    const box = (await support.boundingBox())!, size = page.viewportSize()!
    return box.x >= 12 && box.y >= 12 && box.x + box.width <= size.width - 11 && box.y + box.height <= size.height - 11
  }).toBe(true)
  await expectNoPageOverflow(page)
  await support.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('dialog')).toBeVisible()
})

for (const locale of ['en', 'zh-CN'] as const) {
  test(`brand contact settings upload clear default and frozen retry (${locale})`, async ({ page }, info) => {
    const zh = locale === 'zh-CN'
    await installLayoutApi(page, locale)
    await page.route('**/api/v1/brand-assets/*', route => route.fulfill({ contentType: 'image/png', body: image }))
    let record = { brand_id: gameId, version: 1, status: 'active', config: contacts(), audit_log_id: gameId }
    let lost = true
    const writes: { body: string | null; key: string | undefined }[] = []
    await page.route('**/api/v1/admin/**', async route => {
      const req = route.request(), path = new URL(req.url()).pathname
      const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
      if (path.endsWith('/me')) return reply({ account: { id: gameId, username: 'contacts_reviewer', super_admin: false, brand_ids: [gameId], permissions: [], permissions_by_brand: { [gameId]: ['brand_presentation.view.brand', 'brand_presentation.write.brand'] }, platform_permissions: [] } })
      if (path.endsWith('/brands')) return reply({ items: [{ id: gameId, name: 'Aurora', code: 'aurora', status: 'active', member_count: '12', agent_count: '3', recharge_quota_available: '1000', created_at: '2026-10-24T00:00:00Z' }], total_count: '1' })
      if (path.endsWith('/brand-contacts/image')) {
        expect(req.headers()['x-brand-id']).toBe(gameId)
        expect(req.headers()['content-type']).toBe('image/png')
        return reply({ url: asset })
      }
      if (path.endsWith('/brand-contacts')) {
        expect(req.headers()['x-brand-id']).toBe(gameId)
        if (req.method() === 'GET') return reply(record)
        writes.push({ body: req.postData(), key: req.headers()['idempotency-key'] })
        const body = req.postDataJSON()
        record = { ...record, version: body.version + 1, config: body.config }
        if (lost) { lost = false; return route.abort('failed') }
        return reply(record)
      }
      return route.fallback()
    })
    await page.goto(origins.brand)
    await expect(page.locator('.mobile-nav:visible, .side-nav:visible').first()).toBeVisible()
    if (await page.locator('.mobile-nav').isVisible()) {
      await page.locator('.mobile-nav button').last().click()
      await page.locator('.mobile-more-menu').getByRole('button', { name: zh ? '品牌设置' : 'Brand settings', exact: true }).click()
    } else await page.locator('.side-nav').getByRole('button', { name: zh ? '品牌设置' : 'Brand settings', exact: true }).click()
    await page.locator('.console-tabs').getByRole('button', { name: zh ? '联系方式' : 'Contact details', exact: true }).click()
    const panel = page.locator('.brand-contacts'), entries = panel.locator('fieldset')
    await expect(entries).toHaveCount(3)
    await expect(entries.first().locator('img')).toBeVisible()
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320, 640, 700, 1000, 1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: 800 })
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`brand-contacts-${locale}-${width}.png`), fullPage: true })
    }
    await entries.first().getByRole('button', { name: zh ? '清除图片' : 'Clear image', exact: true }).click()
    await expect(entries.first().locator('img')).toHaveCount(0)
    await expect(entries.first().getByLabel(zh ? '号码' : 'Number', { exact: true })).toHaveValue('brand_wechat')
    await entries.nth(2).locator('input[type=file]').setInputFiles({ name: 'whatsapp.png', mimeType: 'image/png', buffer: image })
    await expect(entries.nth(2).locator('img')).toBeVisible()
    await entries.nth(1).getByRole('radio').check()
    await panel.getByRole('button', { name: zh ? '保存' : 'Save', exact: true }).click()
    await expect(panel.getByRole('button', { name: zh ? '重试保存' : 'Retry save', exact: true })).toBeEnabled()
    await expect(entries.first().getByLabel(zh ? '号码' : 'Number', { exact: true })).toBeDisabled()
    await panel.getByRole('button', { name: zh ? '重试保存' : 'Retry save', exact: true }).click()
    await expect(panel.getByRole('button', { name: zh ? '保存' : 'Save', exact: true })).toBeEnabled()
    expect(writes).toHaveLength(2)
    expect(writes[0]).toEqual(writes[1])
    expect(record.config.default_id).toBe(ids[1])
    expect(record.config.items[0].image_url).toBeNull()
    expect(record.config.items[2].image_url).toBe(asset)
    await panel.getByRole('button', { name: zh ? '添加联系方式' : 'Add contact', exact: true }).click()
    await expect(entries).toHaveCount(4)
    await selectControl(entries.last().getByRole('combobox'), 'whatsapp')
    await entries.last().getByLabel(zh ? '号码' : 'Number', { exact: true }).fill('+6587654321')
    await entries.last().getByRole('button', { name: zh ? '删除' : 'Remove', exact: true }).click()
    await expect(entries).toHaveCount(3)
    await expectNoPageOverflow(page)
  })

  test(`user contacts navigation and default support image priority (${locale})`, async ({ page }, info) => {
    const zh = locale === 'zh-CN'
    await installLayoutApi(page, locale)
    await installOrdersLayoutApi(page)
    let config = contacts()
    await page.route('**/api/v1/brand-contacts', route => route.fulfill({ json: { success: true, data: config } }))
    await page.route('**/api/v1/brand-assets/*', route => route.fulfill({ contentType: 'image/png', body: image }))
    await page.goto(`${origins.user}/account`)
    const tools = page.locator('.account-tools a')
    await expect(tools).toHaveText(zh ? ['通知', '推荐码', '联系方式', '帮助中心'] : ['Notifications', 'Referral codes', 'Contact details', 'Help centre'])
    const entry = page.locator('a[href="/contacts"]:visible').first()
    if (!await entry.count()) await page.getByRole('button', { name: zh ? '菜单' : 'Menu', exact: true }).click()
    await entry.click()
    await expect(page).toHaveURL(/\/contacts$/)
    await expect(page.locator('.contact-option')).toHaveCount(3)
    const support = page.getByRole('button', { name: zh ? '客服' : 'Support', exact: true })
    const preset = page.viewportSize()!
    for (const width of info.project.name.includes('reflow') ? [320, 640, 700, 1000, 1440] : [preset.width]) {
      if (info.project.name.includes('reflow')) await page.setViewportSize({ width, height: 800 })
      const box = await support.boundingBox()
      expect(Math.abs(box!.y + box!.height / 2 - page.viewportSize()!.height / 2)).toBeLessThan(2)
      await expectNoPageOverflow(page)
      await page.screenshot({ path: info.outputPath(`contact-page-${locale}-${width}.png`) })
      await support.click()
      const dialog = page.getByRole('dialog')
      await expect(dialog).toHaveAttribute('aria-label', zh ? '联系方式' : 'Contact details')
      await expect(dialog.locator('.contact-option')).toHaveCount(3)
      await expect(dialog.locator('.contact-option').first()).toContainText(zh ? '微信' : 'WeChat')
      await expect(dialog.locator('.contact-thumbnail')).toHaveAttribute('src', asset)
      const imageWidth = await dialog.locator('.contact-thumbnail').evaluate(el => el.getBoundingClientRect().width / el.closest('.contact-option')!.getBoundingClientRect().width)
      expect(imageWidth).toBeCloseTo(0.3, 2)
      await dialog.locator('.contact-image-button').click()
      const preview = page.locator('.contact-image-preview')
      await expect(preview).toBeVisible()
      await expectNoPageOverflow(page)
      await preview.getByRole('button', { name: zh ? '关闭' : 'Close', exact: true }).click()
      await expect(dialog).toBeVisible()
      await expect(dialog.getByRole('link', { name: '@brand_support' })).toHaveAttribute('href', 'https://t.me/brand_support')
      await expect(dialog.getByRole('link', { name: '+6581234567' })).toHaveAttribute('href', 'https://wa.me/6581234567')
      await expect(dialog).not.toContainText('brand_wechat')
      await page.screenshot({ path: info.outputPath(`support-${locale}-${width}.png`), fullPage: true })
      await dialog.getByRole('button', { name: zh ? '关闭' : 'Close', exact: true }).click()
    }
    await expect(page.locator('.contact-option').filter({ hasText: 'Telegram' })).toContainText('@brand_support')
    await expect(page.locator('.contact-option').filter({ hasText: 'Telegram' }).getByRole('link')).toHaveAttribute('href', 'https://t.me/brand_support')
    await expect(page.locator('.contact-option').filter({ hasText: 'WhatsApp' }).getByRole('link')).toHaveAttribute('href', 'https://wa.me/6581234567')
    await page.locator('.contact-image-button').click()
    const preview = page.locator('.contact-image-preview')
    await expect(preview).toBeVisible()
    await expect(preview.locator('img')).toHaveAttribute('src', asset)
    await expectNoPageOverflow(page)
    await page.screenshot({ path: info.outputPath(`contact-preview-${locale}.png`) })
    await preview.getByRole('button', { name: zh ? '关闭' : 'Close', exact: true }).click()
    await expect(page.locator('.contact-image-button')).toBeFocused()
    await expect(page.locator('.contact-default-badge')).toHaveCount(0)
    config = { ...config, default_id: ids[1] }
    await page.reload()
    await expect(page.locator('.contact-option').first()).toContainText('Telegram')
    await expect(page.locator('.contact-option').nth(1)).toContainText(zh ? '微信' : 'WeChat')
    await support.click()
    await expect(page.getByRole('dialog')).toContainText('@brand_support')
    await expect(page.getByRole('dialog').locator('.contact-option').first()).toContainText('Telegram')
    await expect(page.getByRole('dialog').getByRole('link', { name: '@brand_support' })).toHaveAttribute('href', 'https://t.me/brand_support')
    await page.getByRole('dialog').getByRole('button', { name: zh ? '关闭' : 'Close', exact: true }).click()
    config = { ...config, items: config.items.map(item => ({ ...item, image_url: asset })) }
    await page.reload()
    await support.click()
    await expect(page.getByRole('dialog').locator('.contact-thumbnail')).toHaveCount(3)
    await expect(page.getByRole('dialog').locator('.contact-option').first()).toContainText('Telegram')
    await page.getByRole('dialog').getByRole('button', { name: zh ? '关闭' : 'Close', exact: true }).click()
    config = { ...config, items: config.items.map(item => item.kind === 'telegram' ? { ...item, image_url: null, value: 'not a username' } : item.kind === 'whatsapp' ? { ...item, image_url: null, value: 'invalid number' } : item) }
    await page.reload()
    await expect(page.locator('.contact-option a')).toHaveCount(0)
    config = { items: [], default_id: null }
    await page.reload()
    await expect(support).toHaveCount(0)
    await expect(page.locator('.brand-contact-page')).toContainText(zh ? '暂无联系方式' : 'No contact details')
  })
}
