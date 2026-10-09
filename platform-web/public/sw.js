const CACHE_NAME = 'lottery-platform-static-v1'
const IMMUTABLE_ASSET = /^\/assets\/.+-[A-Za-z0-9_-]{8,}\.(js|css|woff2?|png|svg|webp)$/
const CONTENT_TYPES = {
  js: ['text/javascript', 'application/javascript'], css: ['text/css'],
  woff: ['font/woff', 'application/font-woff'], woff2: ['font/woff2'],
  png: ['image/png'], svg: ['image/svg+xml'], webp: ['image/webp'],
}

self.addEventListener('install', event => { event.waitUntil(self.skipWaiting()) })
self.addEventListener('activate', event => { event.waitUntil(self.clients.claim()) })
self.addEventListener('fetch', event => {
  const request = event.request
  const url = new URL(request.url)
  const asset = IMMUTABLE_ASSET.exec(url.pathname)
  if (request.method !== 'GET' || request.mode === 'navigate' || url.origin !== self.location.origin || url.search || !asset) return
  event.respondWith(caches.open(CACHE_NAME).then(async cache => {
    const cached = await cache.match(request)
    if (cached) return cached
    const response = await fetch(request)
    const type = (response.headers.get('Content-Type') || '').split(';')[0].trim().toLowerCase()
    const policy = response.headers.get('Cache-Control') || ''
    if (response.status === 200 && response.type === 'basic' && CONTENT_TYPES[asset[1]].includes(type) && !/\b(?:no-store|private)\b/i.test(policy)) {
      await cache.put(request, response.clone())
    }
    return response
  }))
})
