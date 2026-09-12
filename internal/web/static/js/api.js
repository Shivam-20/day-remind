// Prefix absolute app paths with APP_BASE_PATH (from <base href="/remind/">).
// Absolute "/api/..." and "/static/..." ignore <base>; fetch must add the prefix.
const API = {
  base() {
    const href = document.querySelector('base')?.getAttribute('href') || '/';
    return href.replace(/\/$/, '');
  },
  url(path) {
    if (!path) return path;
    if (/^https?:\/\//i.test(path)) return path;
    if (path.startsWith('/')) return this.base() + path;
    return path;
  },
  async request(path, options = {}) {
    const res = await fetch(this.url(path), {
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
      ...options,
    });
    const text = await res.text();
    let data = null;
    if (text) {
      try { data = JSON.parse(text); } catch { data = { error: text }; }
    }
    if (!res.ok) {
      const err = new Error(data?.error || res.statusText || 'request failed');
      err.status = res.status;
      throw err;
    }
    return data;
  },
  get(path) { return this.request(path); },
  post(path, body) { return this.request(path, { method: 'POST', body: JSON.stringify(body || {}) }); },
  patch(path, body) { return this.request(path, { method: 'PATCH', body: JSON.stringify(body || {}) }); },
  delete(path) { return this.request(path, { method: 'DELETE' }); },
};
