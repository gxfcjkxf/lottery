const CACHE_NAME = 'luma-static-v1'
const IMMUTABLE_ASSET = /^\/assets\/.+-[A-Za-z0-9_-]{8,}\.(?:js|css|woff2?|png|svg|webp)$/

self.addEventListener('install', event => { event.waitUntil(self.skipWaiting()) })
self.addEventListener('activate', event => { event.waitUntil(self.clients.claim()) })
self.addEventListener('fetch', event => {
  const request = event.request
  const url = new URL(request.url)
  if (request.method !== 'GET' || url.origin !== self.location.origin || url.pathname.startsWith('/api/') || url.pathname.startsWith('/login') || url.pathname.startsWith('/register') || !IMMUTABLE_ASSET.test(url.pathname)) return
  event.respondWith(caches.open(CACHE_NAME).then(async cache => {
    const cached = await cache.match(request)
    if (cached) return cached
    const response = await fetch(request)
    if (response.ok && response.type === 'basic') await cache.put(request, response.clone())
    return response
  }))
})
