import yaml

with open('/opt/baza-skolkovo/deploy/docker-compose.yml', 'r') as f:
    data = yaml.safe_load(f)

# Update skolkovo service
skolkovo = data['services']['skolkovo']

# Set network mode
skolkovo['network_mode'] = 'container:vpn'

# Hardcode DSN to ensure it works
skolkovo['environment']['POSTGRES_DSN'] = 'postgres://skolkovo:042ef8bd8df407146ffc5fc3585ec761@postgres:5432/skolkovo?sslmode=disable'

# Ensure other env vars are set
skolkovo['environment']['ADMIN_USER'] = '${ADMIN_USER:-admin}'
skolkovo['environment']['ADMIN_PASSWORD'] = '${ADMIN_PASSWORD:-admin}'
skolkovo['environment']['MCP_API_KEY'] = '${MCP_API_KEY:-change-me-please}'
skolkovo['environment']['PROXY_URL'] = '${PROXY_URL}'

# Remove ports section entirely to avoid conflict with network_mode: container
if 'ports' in skolkovo:
    del skolkovo['ports']
    print("Removed 'ports' section from skolkovo service.")

with open('/opt/baza-skolkovo/deploy/docker-compose.yml', 'w') as f:
    yaml.dump(data, f, default_flow_style=False)

print('Updated docker-compose.yml')
