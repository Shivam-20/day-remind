const CACHE = 'day-remind-v1';
const SHELL = [
  '/',
  '/static/css/theme.css',
  '/static/css/app.css',
  '/static/js/api.js',
  '/static/js/components.js',
  '/static/js/admin.js',
  '/static/js/app.js',
  '/static/manifest.json',
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE).then((cache) => cache.addAll(SHELL)).then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))
    ).then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (event) => {
  const { request } = event;
  if (request.method !== 'GET') return;
  const url = new URL(request.url);
  if (url.pathname.startsWith('/api/')) return;
  if (url.pathname.startsWith('/static/') || url.pathname === '/' || url.pathname === '/portal') {
    event.respondWith(
      caches.match(request).then((cached) =>
        cached || fetch(request).then((response) => {
          if (response.ok && url.pathname.startsWith('/static/')) {
            const copy = response.clone();
            caches.open(CACHE).then((cache) => cache.put(request, copy));
          }
          return response;
        })
      )
    );
  }
});
