#!/bin/bash
# Test script for proxy client on production server
# Usage: bash /tmp/test_proxy.sh

echo "=== Proxy Client Test ==="

# Test 1: Direct fetch (should fail with 403)
echo ""
echo "1. Testing direct fetch (should fail with 403)..."
result=$(docker exec baza-skolkovo-skolkovo-1 sh -c 'curl -s -o /dev/null -w "%{http_code}" https://dochub.sk.ru/foundation/documents/m/docs/24905/download.aspx 2>/dev/null')
echo "   Result: HTTP $result"

# Test 2: Check proxy env vars
echo ""
echo "2. Checking proxy environment variables..."
docker exec baza-skolkovo-skolkovo-1 sh -c 'echo PROXY_TYPE=$PROXY_TYPE && echo PROXY_CLOUDFLARE_URL=$PROXY_CLOUDFLARE_URL'

# Test 3: Check if proxy_client.go is compiled
echo ""
echo "3. Checking proxy client compilation..."
docker exec baza-skolkovo-skolkovo-1 sh -c 'ls -la /app/skolkovo 2>/dev/null | head -1'

# Test 4: Check current IP
echo ""
echo "4. Checking current server IP..."
docker exec baza-skolkovo-skolkovo-1 sh -c 'curl -s https://api.ipify.org 2>/dev/null || echo "Cannot reach api.ipify.org"'

# Test 5: Test RSS feed (should work)
echo ""
echo "5. Testing RSS feed access..."
result=$(docker exec baza-skolkovo-skolkovo-1 sh -c 'curl -s -o /dev/null -w "%{http_code}" https://dochub.sk.ru/foundation/documents/rss.aspx 2>/dev/null')
echo "   Result: HTTP $result"

# Test 6: Check document count in DB
echo ""
echo "6. Checking document status in database..."
docker exec baza-skolkovo-postgres-1 psql -U skolkovo -d skolkovo -c "SELECT status, count(*) FROM documents GROUP BY status ORDER BY status;" 2>/dev/null

# Test 7: Check files downloaded
echo ""
echo "7. Checking downloaded files..."
docker exec baza-skolkovo-skolkovo-1 sh -c 'ls -la /data/docs/На_проверке/Загружено/ 2>/dev/null | head -10 || echo "No downloaded files"'

echo ""
echo "=== Test Complete ==="
