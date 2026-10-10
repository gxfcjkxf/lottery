import { expect, test } from '@playwright/test'

const apps = {
  user: { port: 15173, cache: 'lottery-user-static-v1', name: 'Lottery user portal' },
  brand: { port: 15174, cache: 'lottery-brand-static-v1', name: 'Brand administration' },
  platform: { port: 15175, cache: 'lottery-platform-static-v1', name: 'Platform administration' },
} as const

type AppName = keyof typeof apps

function appForProject(projectName: string): AppName {
  const app = projectName.split('-')[0]
  if (app === 'user' || app === 'brand' || app === 'platform') return app
  throw new Error(`Unexpected PWA project: ${projectName}`)
}

test('production manifest, service worker cache boundary, and offline asset behavior', async ({ page, request, baseURL }, testInfo) => {
  const appName = appForProject(testInfo.project.name)
  const app = apps[appName]
  const origin = `http://127.0.0.1:${app.port}`
  expect(baseURL).toBe(origin)

  await page.goto('/')
  if (appName === 'brand') {
    await expect(page).toHaveTitle('Brand administration')
    const description = await page.locator('meta[name="description"]').getAttribute('content')
    expect(description).toBeTruthy()
    expect(description).not.toMatch(/demo|prototype|演示|原型/i)
  }
  const firstRegistration = await page.evaluate(async () => {
    const registration = await navigator.serviceWorker.ready
    return { scope: registration.scope, script: registration.active?.scriptURL }
  })
  expect(firstRegistration).toEqual({ scope: `${origin}/`, script: `${origin}/sw.js` })

  await page.reload()
  const readyRegistration = await page.evaluate(async () => {
    const registration = await navigator.serviceWorker.ready
    return { scope: registration.scope, script: registration.active?.scriptURL }
  })
  expect(readyRegistration).toEqual(firstRegistration)
  await expect.poll(() => page.evaluate(() => navigator.serviceWorker.controller?.scriptURL ?? null)).toBe(`${origin}/sw.js`)

  const manifest = await page.evaluate(async () => {
    const link = document.querySelector<HTMLLinkElement>('link[rel="manifest"]')
    if (!link) throw new Error('Production page has no manifest link')
    const response = await fetch(link.href)
    if (!response.ok) throw new Error(`Manifest request failed: ${response.status}`)
    return await response.json() as {
      id?: string
      scope?: string
      display?: string
      name?: string
      icons?: Array<{ src: string; sizes?: string; type?: string }>
    }
  })
  expect(manifest.id).toBe('/')
  expect(manifest.scope).toBe('/')
  expect(manifest.display).toBe('standalone')
  expect(manifest.name).toBe(app.name)
  expect(manifest.name).not.toMatch(/demo|prototype|演示|原型/i)

  for (const size of ['192x192', '512x512']) {
    const icon = manifest.icons?.find(candidate => candidate.sizes?.split(/\s+/).includes(size))
    expect(icon, `manifest must declare a ${size} PNG icon`).toBeDefined()
    expect(icon?.type).toBe('image/png')
    const dimensions = await page.evaluate(async (src) => {
      const response = await fetch(new URL(src, location.href))
      if (!response.ok) throw new Error(`Icon request failed: ${response.status}`)
      if (response.headers.get('content-type')?.split(';')[0] !== 'image/png') throw new Error('Icon response is not PNG')
      const bitmap = await createImageBitmap(await response.blob())
      const result = { width: bitmap.width, height: bitmap.height }
      bitmap.close()
      return result
    }, icon!.src)
    const expectedPixels = Number(size.slice(0, size.indexOf('x')))
    expect(dimensions).toEqual({ width: expectedPixels, height: expectedPixels })
  }

  const appleTouchIcon = await page.locator('link[rel="apple-touch-icon"]').getAttribute('href')
  expect(appleTouchIcon, 'production page must link an Apple touch icon').toBeTruthy()
  const appleDimensions = await page.evaluate(async (src) => {
    const response = await fetch(new URL(src!, location.href))
    if (!response.ok) throw new Error(`Apple touch icon request failed: ${response.status}`)
    if (response.headers.get('content-type')?.split(';')[0] !== 'image/png') throw new Error('Apple touch icon response is not PNG')
    const bitmap = await createImageBitmap(await response.blob())
    const result = { width: bitmap.width, height: bitmap.height }
    bitmap.close()
    return result
  }, appleTouchIcon)
  expect(appleDimensions).toEqual({ width: 180, height: 180 })

  const staticAssetUrl = await page.evaluate(() => {
    const entry = performance.getEntriesByType('resource')
      .map(resource => new URL(resource.name))
      .find(url => url.origin === location.origin && /^\/assets\/.+-[A-Za-z0-9_-]{8,}\.(?:js|css)$/.test(url.pathname))
    if (!entry) throw new Error('No hashed production JavaScript or CSS asset was loaded')
    return entry.href
  })
  const networkAssetResponse = await request.get(staticAssetUrl)
  expect(networkAssetResponse.ok()).toBe(true)
  const originalAssetBody = await networkAssetResponse.text()

  let serviceWorkerAssetResponse = false
  const assetResponseListener = (response: import('@playwright/test').Response) => {
    if (response.url() === staticAssetUrl && response.fromServiceWorker()) serviceWorkerAssetResponse = true
  }
  page.on('response', assetResponseListener)
  const browserAssetBody = await page.evaluate(async ({ url, cacheName }) => {
    const cache = await caches.open(cacheName)
    const cachedRequest = (await cache.keys()).find(request => request.url === url)
    if (!cachedRequest) throw new Error(`Production worker did not cache ${url}`)
    return await (await fetch(cachedRequest)).text()
  }, { url: staticAssetUrl, cacheName: app.cache })
  page.off('response', assetResponseListener)
  expect(browserAssetBody).toBe(originalAssetBody)
  expect(serviceWorkerAssetResponse).toBe(true)

  let apiRequestNumber = 0
  const apiPath = '/api/v1/pwa-private-check'
  const apiRoute = `${origin}${apiPath}`
  await page.route(apiRoute, async route => {
    apiRequestNumber += 1
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      headers: { 'cache-control': 'private, no-store' },
      body: JSON.stringify({ source: 'synthetic-playwright-route', nonce: `synthetic-${apiRequestNumber}`, method: route.request().method() }),
    })
  })

  const syntheticGetResults = await page.evaluate(async path => {
    const fetchPrivate = async () => {
      const response = await fetch(path, { cache: 'no-store' })
      if (!response.ok) throw new Error(`Synthetic private API GET failed: ${response.status}`)
      return await response.json()
    }
    return [await fetchPrivate(), await fetchPrivate()]
  }, apiPath)
  expect(syntheticGetResults).toEqual([
    { source: 'synthetic-playwright-route', nonce: 'synthetic-1', method: 'GET' },
    { source: 'synthetic-playwright-route', nonce: 'synthetic-2', method: 'GET' },
  ])
  const syntheticPostResult = await page.evaluate(async path => {
    const response = await fetch(path, { method: 'POST', body: JSON.stringify({ private: true }) })
    return await response.json()
  }, apiPath)
  expect(syntheticPostResult).toEqual({ source: 'synthetic-playwright-route', nonce: 'synthetic-3', method: 'POST' })

  const cacheState = await page.evaluate(async () => {
    const names = await caches.keys()
    const entries = await Promise.all(names.map(async name => ({
      name,
      requests: await (await caches.open(name)).keys().then(keys => keys.map(request => request.url)),
    })))
    return { names, entries }
  })
  expect(cacheState.names).toEqual([app.cache])
  expect(cacheState.entries).toHaveLength(1)
  const [assetCache] = cacheState.entries
  expect(assetCache.name).toBe(app.cache)
  expect(assetCache.requests.length).toBeGreaterThan(0)
  for (const cachedUrl of assetCache.requests) {
    const url = new URL(cachedUrl)
    expect(url.origin).toBe(origin)
    expect(url.search).toBe('')
    expect(url.pathname).toMatch(/^\/assets\/.+-[A-Za-z0-9_-]{8,}\.(?:js|css|woff2?|png|svg|webp)$/)
    expect(url.pathname).not.toMatch(/(?:^|\/)(?:api|login|register|account|wallet|orders|private)(?:\/|$)/i)
  }
  expect(assetCache.requests).toContain(staticAssetUrl)
  expect(assetCache.requests.some(url => new URL(url).pathname === apiPath)).toBe(false)

  const loginNavigation = await page.goto('/login')
  expect(loginNavigation?.ok()).toBe(true)
  expect(loginNavigation?.headers()['content-type']).toContain('text/html')
  const loginCacheKeys = await page.evaluate(async () => (await Promise.all((await caches.keys()).map(async name =>
    (await (await caches.open(name)).keys()).map(request => new URL(request.url).pathname),
  ))).flat())
  expect(loginCacheKeys.some(path => /(?:^|\/)login(?:\/|$)/i.test(path))).toBe(false)
  expect(loginCacheKeys.some(path => path === '/' || path.endsWith('.html'))).toBe(false)

  await page.unroute(apiRoute)
  const offlinePreconditions = await page.evaluate(async ({ assetUrl, cacheName }) => {
    const cache = await caches.open(cacheName)
    const cachedRequest = (await cache.keys()).find(request => request.url === assetUrl)
    const stored = cachedRequest ? await cache.match(cachedRequest) : undefined
    return {
      controller: navigator.serviceWorker.controller?.scriptURL ?? null,
      cachedAssetStatus: stored?.status ?? null,
    }
  }, { assetUrl: staticAssetUrl, cacheName: app.cache })
  expect(offlinePreconditions.controller).toBe(`${origin}/sw.js`)
  expect(offlinePreconditions.cachedAssetStatus, JSON.stringify(offlinePreconditions)).toBe(200)
  await page.context().setOffline(true)
  await expect.poll(() => page.evaluate(() => navigator.onLine)).toBe(false)
  const offlineResults = await page.evaluate(async ({ assetUrl, privatePath, cacheName }) => {
    const privateResponse = await fetch(privatePath, { cache: 'no-store' }).then(
      response => ({ ok: response.ok, status: response.status }),
      error => ({ error: String(error) }),
    )
    const privatePostResponse = await fetch(privatePath, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ private: true }),
    }).then(
      response => ({ ok: response.ok, status: response.status }),
      error => ({ error: String(error) }),
    )
    const cache = await caches.open(cacheName)
    const cachedRequest = (await cache.keys()).find(request => request.url === assetUrl)
    if (!cachedRequest) throw new Error(`Production worker cache lost ${assetUrl}`)
    const cachedAsset = await fetch(cachedRequest).then(
      async response => ({ ok: response.ok, body: await response.text() }),
      error => ({ error: String(error) }),
    )
    const cacheNames = await caches.keys()
    const cachedUrls = await Promise.all(cacheNames.map(async name =>
      (await (await caches.open(name)).keys()).map(request => request.url),
    )).then(urls => urls.flat())
    return { privateResponse, privatePostResponse, cachedAsset, cacheNames, cachedUrls }
  }, { assetUrl: staticAssetUrl, privatePath: apiPath, cacheName: app.cache })
  expect('error' in offlineResults.privateResponse, JSON.stringify(offlineResults.privateResponse)).toBe(true)
  expect('error' in offlineResults.privatePostResponse, JSON.stringify(offlineResults.privatePostResponse)).toBe(true)
  expect('error' in offlineResults.cachedAsset, JSON.stringify(offlineResults.cachedAsset)).toBe(false)
  expect(offlineResults.cacheNames).toEqual([app.cache])
  expect(offlineResults.cachedUrls.slice().sort()).toEqual(assetCache.requests.slice().sort())
  expect(offlineResults.cachedUrls.some(url => new URL(url).pathname === apiPath)).toBe(false)
  if (!('error' in offlineResults.cachedAsset)) {
    expect(offlineResults.cachedAsset.ok).toBe(true)
    expect(offlineResults.cachedAsset.body).toBe(originalAssetBody)
  }
})
