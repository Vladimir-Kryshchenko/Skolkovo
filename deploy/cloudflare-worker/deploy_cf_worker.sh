#!/bin/bash
# Deploy Cloudflare Worker using API directly (no wrangler login needed)
# Usage: bash deploy_cf_worker.sh YOUR_API_TOKEN YOUR_ACCOUNT_ID

set -e

API_TOKEN="${1:?Usage: $0 <API_TOKEN> <ACCOUNT_ID>}"
ACCOUNT_ID="${2:?Usage: $0 <API_TOKEN> <ACCOUNT_ID>}"
WORKER_NAME="skolkovo-proxy"

echo "🚀 Deploying Cloudflare Worker: $WORKER_NAME"
echo "   Account: $ACCOUNT_ID"

# Read worker code
WORKER_CODE=$(cat <<'EOF'
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
EOF
)

# Deploy worker
echo "📤 Uploading worker code..."
RESPONSE=$(curl -s -X PUT "https://api.cloudflare.com/client/v4/accounts/$ACCOUNT_ID/workers/scripts/$WORKER_NAME" \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/javascript" \
  -d "$WORKER_CODE")

SUCCESS=$(echo "$RESPONSE" | grep -o '"success": true')
if [ "$SUCCESS" = '"success": true' ]; then
  echo "✅ Worker deployed successfully!"
  echo ""
  echo "🌐 Worker URL: https://$WORKER_NAME.YOUR-SUBDOMAIN.workers.dev"
  echo ""
  echo "📝 Next steps:"
  echo "   1. Test worker:"
  echo "      curl https://$WORKER_NAME.YOUR-SUBDOMAIN.workers.dev/health"
  echo "      curl \"https://$WORKER_NAME.YOUR-SUBDOMAIN.workers.dev/?url=https://api.ipify.org\""
  echo ""
  echo "   2. Add to .env:"
  echo "      echo 'PROXY_TYPE=cloudflare' >> /opt/baza-skolkovo/deploy/.env"
  echo "      echo \"PROXY_CLOUDFLARE_URL=https://$WORKER_NAME.YOUR-SUBDOMAIN.workers.dev/?url=\" >> /opt/baza-skolkovo/deploy/.env"
  echo ""
  echo "   3. Restart container:"
  echo "      cd /opt/baza-skolkovo"
  echo "      docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build skolkovo"
  echo ""
  echo "   4. Test fetch:"
  echo "      docker exec baza-skolkovo-skolkovo-1 /app/skolkovo fetch"
else
  echo "❌ Deployment failed!"
  echo "Response: $RESPONSE"
  exit 1
fi
