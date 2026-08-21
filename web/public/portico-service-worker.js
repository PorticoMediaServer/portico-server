const CACHE_NAME = 'portico-hosted-assets-v2';
const RETIRED_CACHE_PREFIXES = ['portico-hosted-shell-', 'portico-hosted-assets-'];

self.addEventListener('install', (event) => {
  event.waitUntil(Promise.resolve());
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(caches.keys().then((names) => Promise.all(
    names.filter((name) => RETIRED_CACHE_PREFIXES.some((prefix) => name.startsWith(prefix)) && name !== CACHE_NAME).map((name) => caches.delete(name)),
  )));
  self.clients.claim();
});

function cacheableContentAddressedAsset(request, url) {
  if (request.method !== 'GET' || url.origin !== self.location.origin) return false;
  return url.pathname.startsWith('/assets/')
    && ['script', 'style', 'font', 'image'].includes(request.destination);
}

self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);
  if (!cacheableContentAddressedAsset(event.request, url)) return;

  event.respondWith(caches.open(CACHE_NAME).then(async (cache) => {
    const cached = await cache.match(event.request);
    if (cached) return cached;
    const response = await fetch(event.request);
    if (response.ok) await cache.put(event.request, response.clone());
    return response;
  }));
});
