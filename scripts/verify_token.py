import urllib.request
import json
import sys

# Token from user
TOKEN = sys.argv[1]
URL = "https://api.cloudflare.com/client/v4/user/tokens/verify"

req = urllib.request.Request(URL, headers={'Authorization': f'Bearer {TOKEN}'})
try:
    with urllib.request.urlopen(req) as response:
        print(response.read().decode())
except urllib.error.HTTPError as e:
    print(f'HTTP Error: {e.code}')
    print(e.read().decode())
