#!/bin/bash
# Test free proxies for dochub.sk.ru access

echo "=== Testing free proxies for dochub.sk.ru ==="

# Get proxy list
proxies=$(curl -s --max-time 10 'https://api.proxyscrape.com/v2/?request=displayproxies&protocol=http&timeout=5000&country=all' | head -20)

for proxy in $proxies; do
    echo "Testing $proxy..."
    
    # Test IP
    ip_result=$(timeout 5 curl -s -o /dev/null -w "%{http_code} %{ip}" --proxy "http://$proxy" https://api.ipify.org 2>/dev/null)
    echo "  IP test: $ip_result"
    
    # Test dochub.sk.ru
    dochub_result=$(timeout 5 curl -s -o /dev/null -w "%{http_code}" --proxy "http://$proxy" https://dochub.sk.ru/foundation/documents/rss.aspx 2>/dev/null)
    echo "  dochub.sk.ru: HTTP $dochub_result"
    
    if [ "$dochub_result" = "200" ]; then
        echo "  ✅ WORKING!"
        echo "$proxy" >> /tmp/working_proxies.txt
    fi
    
    echo "---"
done

echo "Done! Working proxies saved to /tmp/working_proxies.txt"
cat /tmp/working_proxies.txt 2>/dev/null
