import yaml

with open('/opt/baza-skolkovo/deploy/docker-compose.yml', 'r') as f:
    data = yaml.safe_load(f)

skolkovo = data['services']['skolkovo']
if 'network_mode' in skolkovo:
    del skolkovo['network_mode']
    print('Removed network_mode')

# Add ports back
skolkovo['ports'] = [
    '8080:8080', '8090:8090', '8091:8091',
    '8092:8092', '8093:8093', '8094:8094', '9090:9090'
]
print('Added ports back')

with open('/opt/baza-skolkovo/deploy/docker-compose.yml', 'w') as f:
    yaml.dump(data, f, default_flow_style=False)
    
print('Updated docker-compose.yml')
