export default {
  async fetch(request) {
    const url = new URL(request.url);
    if (!url.pathname.startsWith('/remind')) {
      return fetch(request);
    }

    const path = url.pathname + url.search;
    const originURL = `http://161-118-181-5.nip.io${path}`;
    const init = {
      method: request.method,
      redirect: 'manual',
      headers: {
        Host: '161.118.181.5',
        Accept: request.headers.get('Accept') || '*/*',
      },
      cf: { resolveOverride: '161.118.181.5' },
    };
    const cookie = request.headers.get('Cookie');
    if (cookie) init.headers.Cookie = cookie;
    if (request.method !== 'GET' && request.method !== 'HEAD') {
      init.body = await request.arrayBuffer();
      const ct = request.headers.get('Content-Type');
      if (ct) init.headers['Content-Type'] = ct;
    }

    let response = await fetch(originURL, init);
    if (response.status >= 300 && response.status < 400) {
      const dbg = {
        status: response.status,
        server: response.headers.get('server'),
        location: response.headers.get('location'),
      };
      return new Response(JSON.stringify(dbg), { status: 502, headers: { 'content-type': 'application/json' } });
    }

    const out = new Response(response.body, response);
    out.headers.set('cache-control', 'no-store');
    return out;
  },
};
