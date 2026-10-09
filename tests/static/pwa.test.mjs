import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { test } from 'node:test'
import vm from 'node:vm'

const apps = ['user-web', 'admin-web', 'platform-web']
async function worker(app, { status = 200, type = 'basic', headers = { 'Content-Type': 'application/javascript' } } = {}) {
  const handlers = {}, entries = new Map(), opened = [], deleted = []
  let network = 0
  const origin = 'https://brand.example'
  const cache = { match: async request => entries.get(request.url), put: async (request, response) => entries.set(request.url, response) }
  const scope = {
    self: { location: { origin }, addEventListener: (name, handler) => { handlers[name] = handler }, skipWaiting: async () => {}, clients: { claim: async () => {} } },
    URL,
    caches: { open: async name => { opened.push(name); return cache }, keys: async () => ['unrelated-app'], delete: async key => { deleted.push(key) } },
    fetch: async () => { network++; const response = { status, type, headers: new Headers(headers), marker: network, clone: () => response }; return response },
  }
  vm.runInNewContext(await readFile(`${app}/public/sw.js`, 'utf8'), scope)
  return {
    entries, opened, deleted, network: () => network,
    async handles(path, { method = 'GET', mode = 'cors', foreign = false } = {}) {
      let task
      const request = { url: `${foreign ? 'https://other.example' : origin}${path}`, method, mode }
      handlers.fetch({ request, respondWith: promise => { task = promise } })
      if (task) await task
      return Boolean(task)
    },
    async activate() { let task; handlers.activate({ waitUntil: promise => { task = promise } }); await task },
  }
}

for (const [index, app] of apps.entries()) {
  const asset = '/assets/index-DpASG_Mw.js'
  test(`${app} caches a verified immutable asset only in its own namespace`, async () => {
    const sw = await worker(app)
    assert.equal(await sw.handles(asset), true)
    assert.equal(await sw.handles(asset), true)
    assert.equal(sw.network(), 1)
    assert.equal(sw.entries.size, 1)
    assert.ok(sw.opened.every(name => name === ['lottery-user-static-v1', 'lottery-brand-static-v1', 'lottery-platform-static-v1'][index]))
    await sw.activate()
    assert.deepEqual(sw.deleted, [])
    for (const path of ['/', '/api/v1/context', '/api/v1/wallet', '/api/v1/admin/me', '/api/v1/platform/me', '/wallet', '/orders', '/login', '/register', '/sw.js', '/manifest.webmanifest', '/assets/private.json', `${asset}?token=test`]) {
      assert.equal(await sw.handles(path), false, path)
    }
    assert.equal(await sw.handles(asset, { method: 'POST' }), false)
    assert.equal(await sw.handles(asset, { mode: 'navigate' }), false)
    assert.equal(await sw.handles(asset, { foreign: true }), false)
    assert.equal(sw.network(), 1)
  })

  test(`${app} never caches HTML errors or responses marked private or no-store`, async () => {
    for (const options of [
      { headers: { 'Content-Type': 'text/html' } },
      { headers: { 'Content-Type': 'application/javascript', 'Cache-Control': 'no-store' } },
      { headers: { 'Content-Type': 'application/javascript', 'Cache-Control': 'private, max-age=600' } },
      { status: 404 }, { type: 'cors' },
    ]) {
      const sw = await worker(app, options)
      assert.equal(await sw.handles(asset), true)
      assert.equal(await sw.handles(asset), true)
      assert.equal(sw.entries.size, 0)
      assert.equal(sw.network(), 2)
    }
  })

  test(`${app} manifest and Apple touch links reference real PNG icons of the declared size`, async () => {
    const manifest = JSON.parse(await readFile(`${app}/public/manifest.webmanifest`, 'utf8'))
    assert.equal(manifest.id, '/')
    assert.equal(manifest.scope, '/')
    assert.equal(manifest.start_url, '/')
    assert.equal(manifest.display, 'standalone')
    assert.equal(manifest.prefer_related_applications, false)
    assert.doesNotMatch(manifest.name + manifest.description, /demo|prototype|演示/i)
    for (const size of [192, 512]) {
      const icon = manifest.icons.find(item => item.sizes === `${size}x${size}`)
      assert.equal(icon.type, 'image/png')
      const bytes = await readFile(`${app}/public${icon.src}`)
      assert.equal(bytes.subarray(0, 8).toString('hex'), '89504e470d0a1a0a')
      assert.equal(bytes.readUInt32BE(16), size)
      assert.equal(bytes.readUInt32BE(20), size)
    }
    const touch = await readFile(`${app}/public/icons/apple-touch-icon.png`)
    assert.equal(touch.readUInt32BE(16), 180)
    assert.equal(touch.readUInt32BE(20), 180)
    assert.match(await readFile(`${app}/index.html`, 'utf8'), /apple-touch-icon/)
  })
}
