// Prefix absolute app paths when served under a subpath (e.g. /remind/).
const API = {
  base() {
    const href = document.querySelector('base')?.getAttribute('href') || '';
    const fromTag = href.replace(/\/$/, '');
    if (fromTag) return fromTag;
    const match = (location.pathname || '').match(/^(\/remind)(?:\/|$)/);
    return match ? match[1] : '';
  },
  url(path) {
    if (!path) return path;
    if (/^https?:\/\//i.test(path)) return path;
    if (path.startsWith('/')) return this.base() + path;
    return path;
  },
  async request(path, options = {}) {
    const url = this.url(path);
    const res = await fetch(url, {
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
      err.data = data;
      throw err;
    }
    return data;
  },
  get(path) { return this.request(path); },
  post(path, body) { return this.request(path, { method: 'POST', body: JSON.stringify(body || {}) }); },
  patch(path, body) { return this.request(path, { method: 'PATCH', body: JSON.stringify(body || {}) }); },
  delete(path) { return this.request(path, { method: 'DELETE' }); },
};
