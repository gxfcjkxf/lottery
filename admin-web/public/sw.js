const CACHE = 'aurora-admin-demo-v1'
self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(CACHE).then((cache) => cache.addAll(['/manifest.webmanifest'])))
  self.skipWaiting()
})
self.addEventListener('activate', (event) => {
  event.waitUntil(caches.keys().then((keys) => Promise.all(keys.filter((key) => key.startsWith('aurora-admin-demo-') && key !== CACHE).map((key) => caches.delete(key)))))
  self.clients.claim()
})
self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url)
  if (event.request.method !== 'GET' || url.origin !== self.location.origin || url.pathname.startsWith('/api')) return
  const isStaticFile = url.pathname === '/manifest.webmanifest' || url.pathname.startsWith('/assets/')
  if (!isStaticFile) return
  event.respondWith(caches.match(event.request).then((cached) => cached || fetch(event.request).then((response) => {
    if (response.ok && response.type === 'basic') caches.open(CACHE).then((cache) => cache.put(event.request, response.clone()))
    return response
  })))
})
