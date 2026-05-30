# Cloudflare Worker — резидентный прокси для скачивания документов
# Деплой: npx wrangler deploy proxy.js --name skolkovo-proxy

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    // Health check
    if (url.pathname === '/health') {
      return new Response(JSON.stringify({ 
        status: 'ok', 
        service: 'skolkovo-proxy',
        timestamp: new Date().toISOString()
      }), {
        headers: { 'Content-Type': 'application/json', 'Access-Control-Allow-Origin': '*' }
      });
    }

    // CORS
    if (request.method === 'OPTIONS') {
      return new Response(null, {
        headers: {
          'Access-Control-Allow-Origin': '*',
          'Access-Control-Allow-Methods': 'GET, POST, OPTIONS',
          'Access-Control-Allow-Headers': 'Content-Type, Cookie, Authorization',
        }
      });
    }

    const targetUrl = url.searchParams.get('url');
    if (!targetUrl) {
      return new Response(JSON.stringify({ error: 'Missing ?url parameter' }), {
        status: 400,
        headers: { 'Content-Type': 'application/json', 'Access-Control-Allow-Origin': '*' }
      });
    }

    // Validate domain
    try {
      const target = new URL(targetUrl);
      if (!target.hostname.includes('dochub.sk.ru') && !target.hostname.includes('sk.ru')) {
        return new Response(JSON.stringify({ error: 'Only dochub.sk.ru and sk.ru allowed' }), {
          status: 403,
          headers: { 'Content-Type': 'application/json', 'Access-Control-Allow-Origin': '*' }
        });
      }
    } catch {
      return new Response(JSON.stringify({ error: 'Invalid URL' }), {
        status: 400,
        headers: { 'Content-Type': 'application/json', 'Access-Control-Allow-Origin': '*' }
      });
    }

    // Fetch through Cloudflare (residential IP)
    const headers = new Headers();
    headers.set('User-Agent', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36');
    headers.set('Accept', 'application/pdf,application/msword,application/vnd.openxmlformats-officedocument.*,*/*');
    headers.set('Accept-Language', 'ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7');
    
    if (request.headers.has('Cookie')) {
      headers.set('Cookie', request.headers.get('Cookie'));
    }

    const response = await fetch(targetUrl, {
      method: 'GET',
      headers: headers,
      redirect: 'follow',
    });

    if (response.status === 403) {
      return new Response(JSON.stringify({ 
        error: 'Blocked by WAF', 
        target: targetUrl 
      }), {
        status: 403,
        headers: { 'Content-Type': 'application/json', 'Access-Control-Allow-Origin': '*' }
      });
    }

    const respHeaders = new Headers();
    respHeaders.set('Content-Type', response.headers.get('Content-Type') || 'application/octet-stream');
    respHeaders.set('Access-Control-Allow-Origin', '*');
    respHeaders.set('Access-Control-Expose-Headers', 'Content-Disposition, Content-Length');

    return new Response(response.body, {
      status: response.status,
      headers: respHeaders
    });
  }
};
