import urllib.request
import json
import sys

TOKEN = sys.argv[1]
ACCOUNT_ID = sys.argv[2]
SUBDOMAIN = sys.argv[3]

URL = f"https://api.cloudflare.com/client/v4/accounts/{ACCOUNT_ID}/workers/subdomain"

data = json.dumps({"subdomain": SUBDOMAIN}).encode('utf-8')
req = urllib.request.Request(URL, data=data, headers={
    'Authorization': f'Bearer {TOKEN}',
    'Content-Type': 'application/json'
})

try:
    with urllib.request.urlopen(req) as response:
        print(response.read().decode())
except urllib.error.HTTPError as e:
    print(f'HTTP Error: {e.code}')
    print(e.read().decode())
