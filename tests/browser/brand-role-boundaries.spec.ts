import { test, expect, type APIResponse } from '@playwright/test'

const origin = 'http://localhost:5174'
const brand = '0199a000-0000-7000-8000-000000000001'
const harbor = '0199a000-0000-7000-8000-000000000002'
const password = 'role-boundary-local-password-2026'

async function data<T>(response: APIResponse, status = 200): Promise<T> {
  expect(response.status()).toBe(status)
  return (await response.json()).data
}

test('finance, support and combined roles retain exact brand grants after browser login', async ({ page, browser }, info) => {
  test.setTimeout(60_000)
  if (!process.env.TEST_ADMIN_USERNAME || !process.env.TEST_ADMIN_PASSWORD) {
    throw new Error('Explicit isolated administrator credentials are required')
  }
  const headers = { Origin: origin, 'X-Brand-ID': brand }
  await data(await page.request.post(origin + '/api/v1/admin/auth/login', {
    headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
    data: { identifier: process.env.TEST_ADMIN_USERNAME, password: process.env.TEST_ADMIN_PASSWORD },
  }))
  const suffix = crypto.randomUUID().replaceAll('-', '').slice(0, 12)
  const definitions = [
    { code: 'finance', permissions: ['brand.view.brand', 'wallet.view.brand', 'recharge.view.brand'] },
    { code: 'support', permissions: ['brand.view.brand', 'user.view.brand'] },
  ]
  const roles: string[] = []
  for (const definition of definitions) {
    const role = await data<{ id: string }>(await page.request.post(origin + '/api/v1/admin/roles', {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
      data: { code: `${definition.code}_${suffix}`, name: definition.code, permissions: definition.permissions, reason: 'isolated role boundary verification' },
    }), 201)
    roles.push(role.id)
  }
  for (const scenario of [
    { name: 'finance', ids: [roles[0]], permissions: definitions[0].permissions },
    { name: 'support', ids: [roles[1]], permissions: definitions[1].permissions },
    { name: 'combined', ids: roles, permissions: ['brand.view.brand', 'user.view.brand', 'wallet.view.brand', 'recharge.view.brand'] },
  ]) {
    const username = `${scenario.name}_${suffix}`
    await data(await page.request.post(origin + '/api/v1/admin/accounts', {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
      data: { username, password, role_ids: scenario.ids, reason: 'isolated role account verification' },
    }), 201)
    const context = await browser.newContext({ viewport: info.project.use.viewport,
      storageState: { cookies: [], origins: [{ origin, localStorage: [{ name: 'lottery.admin.locale', value: 'zh-CN' }] }] } })
    try {
      const staff = await context.newPage()
      await staff.goto(origin)
      await expect(staff.locator('.side-nav')).toHaveCount(0)
      await expect(staff.locator('.mobile-nav')).toHaveCount(0)
      await staff.getByLabel('账号', { exact: true }).fill(username)
      await staff.getByLabel('密码', { exact: true }).fill(password)
      await staff.getByRole('button', { name: '登录并加载真实成员', exact: true }).click()
      await expect(staff.getByLabel('选择真实后台品牌', { exact: true })).toBeVisible()
      const me = await data<{ account: { brand_ids: string[]; permissions_by_brand: Record<string, string[]> } }>(
        await context.request.get(origin + '/api/v1/admin/me'))
      expect(me.account.brand_ids).toEqual([brand])
      expect(me.account.permissions_by_brand[brand].slice().sort()).toEqual(scenario.permissions.slice().sort())
      expect(new Set(me.account.permissions_by_brand[brand]).size).toBe(scenario.permissions.length)
      await staff.reload()
      await expect(staff.getByLabel('选择真实后台品牌', { exact: true })).toBeVisible()
      for (const path of ['/roles', '/accounts']) {
        expect((await context.request.get(origin + '/api/v1/admin' + path, { headers })).status()).toBe(403)
      }
      expect((await context.request.get(origin + '/api/v1/admin/users', { headers: { ...headers, 'X-Brand-ID': harbor } })).status()).toBe(403)
      const members = await context.request.get(origin + '/api/v1/admin/users', { headers })
      expect(members.status()).toBe(scenario.name === 'finance' ? 403 : 200)
      const recharges = await context.request.get(origin + '/api/v1/admin/recharges', { headers })
      expect(recharges.status()).toBe(scenario.name === 'support' ? 403 : 200)
      expect((await context.request.post(origin + '/api/v1/admin/roles', {
        headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() },
        data: { code: `denied_${scenario.name}_${suffix}`, name: 'Denied role', permissions: ['brand.view.brand'], reason: 'verify missing role write grant' },
      })).status()).toBe(403)
      expect(await staff.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true)
    } finally {
      await context.close()
    }
  }
})
