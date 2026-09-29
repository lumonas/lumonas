const CACHE = 'lumonas-shell-v1'
self.addEventListener('install', (event) => event.waitUntil(caches.open(CACHE).then((cache) => cache.addAll(['/', '/manifest.webmanifest', '/icon.svg']))))
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()))
self.addEventListener('fetch', (event) => {
  const request = event.request
  const url = new URL(request.url)
  if (request.method !== 'GET' || url.origin !== self.location.origin || url.pathname.startsWith('/api/')) return
  event.respondWith(fetch(request).then((response) => {
    if (response.ok && (request.mode === 'navigate' || url.pathname.startsWith('/assets/'))) {
      const copy = response.clone(); caches.open(CACHE).then((cache) => cache.put(request, copy))
    }
    return response
  }).catch(() => caches.match(request).then((cached) => cached || caches.match('/'))))
})
