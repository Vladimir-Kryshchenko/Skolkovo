import re

with open('/opt/baza-skolkovo/src/fetcher/fetcher.go', 'r') as f:
    content = f.read()

# Logic to inject
proxy_logic = '''
	// Proxy Logic
	proxyType := os.Getenv("PROXY_TYPE")
	proxyURL := os.Getenv("PROXY_CLOUDFLARE_URL")
	actualFileURL := fileURL

	if proxyType == "cloudflare" && proxyURL != "" {
		actualFileURL = proxyURL + url.QueryEscape(fileURL)
		log.Printf("[fetcher] Using Cloudflare proxy: %s", actualFileURL)
	}

'''

old_download_line = '\tdata, err := f.download(ctx, fileURL, viewerURL, cookies)'
new_download_block = proxy_logic + '\tdata, err := f.download(ctx, actualFileURL, viewerURL, cookies)'

if old_download_line in content:
    content = content.replace(old_download_line, new_download_block)
    
    # Add import if missing
    if '"net/url"' not in content:
        content = content.replace('"os"', '"os"\n\t"net/url"')
        
    with open('/opt/baza-skolkovo/src/fetcher/fetcher.go', 'w') as f:
        f.write(content)
    print('SUCCESS: fetcher.go updated')
else:
    print('ERROR: Could not find download line')
    for i, line in enumerate(content.split('\n')):
        if 'f.download' in line:
            print(f'Line {i}: {line}')
