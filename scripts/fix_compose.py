import re

with open('/opt/baza-skolkovo/deploy/docker-compose.yml', 'r') as f:
    content = f.read()

# Check if PROXY_URL is already in environment
if 'PROXY_URL' in content:
    print('PROXY_URL already in file')
else:
    # Find the environment section of skolkovo service
    # Add PROXY_URL after CHROME_PATH
    content = content.replace('CHROME_PATH: /usr/bin/chromium', 'CHROME_PATH: /usr/bin/chromium\n      PROXY_URL: ${PROXY_URL}')
    
    with open('/opt/baza-skolkovo/deploy/docker-compose.yml', 'w') as f:
        f.write(content)
    print('Updated docker-compose.yml')

# Also ensure .env has PROXY_URL
env_path = '/opt/baza-skolkovo/deploy/.env'
with open(env_path, 'r') as f:
    env_content = f.read()

if 'PROXY_URL=' not in env_content:
    with open(env_path, 'a') as f:
        f.write('\nPROXY_URL=http://ovszbgom:rxcqmbh4ms0u@38.154.203.95:5863')
    print('Updated .env')
else:
    print('.env already has PROXY_URL')
