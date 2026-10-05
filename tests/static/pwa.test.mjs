import assert from 'node:assert/strict'
import { readFile, access } from 'node:fs/promises'
import { test } from 'node:test'
import vm from 'node:vm'

async function worker(app) {
  const handlers = {}
  const deleted = []
  const cache = { match: async () => undefined, put: async () => {}, addAll: async () => {} }
  const origin = 'https://brand.example'
  const scope = {
    self: { location: { origin }, addEventListener: (name, handler) => { handlers[name] = handler }, skipWaiting: async () => {}, clients: { claim: async () => {} } },
    URL,
    caches: { open: async () => cache, match: cache.match, keys: async () => ['unrelated-app', 'aurora-admin-demo-old'], delete: async key => { deleted.push(key) } },
    fetch: async () => ({ ok: true, type: 'basic', clone: () => ({}) }),
  }
  vm.runInNewContext(await readFile(`${app}/public/sw.js`, 'utf8'), scope)
  return {
    deleted,
    async handles(path, method = 'GET') {
      let response
      handlers.fetch({ request: { url: `${origin}${path}`, method, mode: 'navigate' }, respondWith: promise => { response = promise } })
      if (response) await response
      return Boolean(response)
    },
    async activate() { let task; handlers.activate({ waitUntil: promise => { task = promise } }); await task },
  }
}

for (const app of ['user-web', 'admin-web']) {
  test(`${app} caches Vite static assets but not API, HTML, auth, or writes`, async () => {
    const sw = await worker(app)
    assert.equal(await sw.handles('/assets/index-DpASG_Mw.js'), true)
    for (const path of ['/', '/api/v1/context', '/wallet', '/login', '/sw.js']) {
      assert.equal(await sw.handles(path), false, path)
    }
    assert.equal(await sw.handles('/assets/index-DpASG_Mw.js', 'POST'), false)
    await sw.activate()
    assert.equal(sw.deleted.includes('unrelated-app'), false)
  })
  test(`${app} manifest references present local icons`, async () => {
    const manifest = JSON.parse(await readFile(`${app}/public/manifest.webmanifest`, 'utf8'))
    assert.equal(manifest.display, 'standalone')
    assert.ok(manifest.icons.length > 0)
    for (const icon of manifest.icons) await access(`${app}/public${icon.src}`)
  })
}
