import { test, expect } from '@playwright/test'
import { build } from '../../admin-web/node_modules/vite'
import { createServer, type Server } from 'node:http'
import { cp, mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { extname, join, resolve } from 'node:path'

let directory: string, published: string, origin: string, server: Server
const id = 'aaaaaaaa-aaaa-4aaa-8aaa-000000000001'
const at = '2026-11-02T00:00:00Z'

test.beforeAll(async () => {
  directory = await mkdtemp(join(tmpdir(), 'lottery-deployment-'))
  published = join(directory, 'current')
  for (const release of ['a', 'b']) {
    const result = await build({
      root: resolve('admin-web'), configFile: resolve('admin-web/vite.config.ts'), logLevel: 'error',
      build: { outDir: join(directory, release), emptyOutDir: true },
      plugins: [{ name: 'deployment-test-release', augmentChunkHash: () => release }],
    })
    if (Array.isArray(result) || !('output' in result)) throw new Error('Expected a completed production build')
    for (const item of result.output) if (item.type === 'chunk') expect(item.dynamicImports).toEqual([])
  }
  server = createServer(async (request, response) => {
    const pathname = new URL(request.url!, 'http://localhost').pathname
    const file = resolve(published, '.' + (pathname === '/' ? '/index.html' : pathname))
    if (!file.startsWith(published + '/')) { response.writeHead(404).end(); return }
    try {
      const bytes = await readFile(file)
      const types: Record<string, string> = { '.html': 'text/html', '.js': 'application/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.json': 'application/json' }
      response.writeHead(200, { 'Content-Type': types[extname(file)] ?? 'application/octet-stream', 'Cache-Control': 'no-store' }).end(bytes)
    } catch { response.writeHead(404).end() }
  })
  await new Promise<void>(done => server.listen(0, '127.0.0.1', done))
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('Expected an independent HTTP test port')
  origin = `http://127.0.0.1:${address.port}`
})
test.afterAll(async () => {
  if (server) { server.closeAllConnections(); await new Promise<void>((done, reject) => server.close(error => error ? reject(error) : done())) }
  if (directory) await rm(directory, { recursive: true, force: true })
})

for (const locale of ['en', 'zh-CN'] as const) test(`an open page survives replacing all old assets (${locale})`, async ({ page }, info) => {
  await rm(published, { recursive: true, force: true })
  await cp(join(directory, 'a'), published, { recursive: true })
  await page.context().addCookies([{ name: 'lottery-locale', value: locale, url: origin }])
  let signedIn = false
  const errors: string[] = [], assets: string[] = [], reads: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('request', request => { if (new URL(request.url()).pathname.startsWith('/assets/')) assets.push(request.url()) })
  await page.route('**/api/**', route => {
    const path = new URL(route.request().url()).pathname
    const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
    if (path === '/api/v1/display-settings') return reply({ theme:'jade',timezone: 'Asia/Singapore', date_format: 'YYYY-MM-DD', time_format: 'HH:mm:ss' })
    if (path === '/api/v1/admin/auth/config') return reply({ captcha_enabled: false })
    if (path === '/api/v1/admin/auth/login') {
      expect(route.request().method()).toBe('POST')
      expect(route.request().postDataJSON()).toMatchObject({ identifier: 'deployment_reviewer', password: 'unsent-draft-123' })
      signedIn = true; return reply({})
    }
    expect(route.request().method()).toBe('GET')
    if (path === '/api/v1/admin/me') return signedIn
      ? reply({ account: { id, username: 'deployment_reviewer', super_admin: false, brand_ids: [id], permissions: [], permissions_by_brand: { [id]: ['commission.view.brand', 'audit.view.brand'] }, platform_permissions: [] } })
      : route.fulfill({ status: 401, json: { success: false, error: { code: 'UNAUTHORIZED', message: 'Sign in required' } } })
    if (path === '/api/v1/admin/brands') return reply({ items: [{ id, code: 'deployment', name: 'Deployment trial', status: 'active', allow_custom_games: true, member_count: '0', agent_count: '0', recharge_quota_available: '0', created_at: at }] })
    expect(route.request().headers()['x-brand-id']).toBe(id)
    reads.push(path)
    if (path.endsWith('/filter-options')) return reply({ brand_id: id, items: [], limit: 100, offset: 0 })
    if (path.endsWith('/commission-cycles')) return reply({ brand_id: id, items: [], total_count: '0', limit: 20, offset: 0 })
    if (path.endsWith('/audit')) return reply({ items: [], total_count: '0' })
    throw new Error(`Unexpected deployment test request: ${path}`)
  })
  await page.goto(origin)
  const account = page.locator('input[autocomplete=username]'), password = page.locator('input[autocomplete=current-password]')
  await account.fill('deployment_reviewer'); await password.fill('unsent-draft-123')
  const entry = await page.locator('script[type=module]').getAttribute('src')
  expect(entry).toBeTruthy()
  const loadedAssets = [...assets]
  // Physically replace the only published directory: there is no old resource store.
  await rm(published, { recursive: true })
  await cp(join(directory, 'b'), published, { recursive: true })
  expect((await page.request.get(origin + entry)).status()).toBe(404)
  await expect(account).toHaveValue('deployment_reviewer'); await expect(password).toHaveValue('unsent-draft-123')
  await page.locator('.login-entry__form .button-primary').click()
  await expect(page.locator('.management-console')).toBeVisible()
  for (const name of locale === 'en' ? ['Commission management', 'Audit log'] : ['佣金管理', '审计日志']) {
    const dismiss = page.locator('.operation-toast-stack button')
    while (await dismiss.count()) await dismiss.first().click()
    const more = page.locator('.mobile-nav > button').last(), mobile = await more.isVisible()
    if (mobile) await more.click()
    await page.locator(mobile ? '.mobile-more-menu' : '.side-nav').getByRole('button', { name, exact: true }).click()
    await expect(page.locator('.breadcrumbs b')).toHaveText(name)
    await expect(page.locator('.global-loading-overlay')).toHaveCount(0)
  }
  await expect.poll(() => reads.some(path => path.endsWith('/commission-cycles'))).toBe(true)
  await expect.poll(() => reads.some(path => path.endsWith('/audit'))).toBe(true)
  expect(assets).toEqual(loadedAssets)
  expect(errors).toEqual([])
  expect(await page.evaluate(() => performance.getEntriesByType('navigation').length)).toBe(1)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true)
  await page.screenshot({ path: info.outputPath(`after-deployment-${locale}.png`) })
})
