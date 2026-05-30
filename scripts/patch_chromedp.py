with open('/opt/baza-skolkovo/src/fetcher/fetcher.go', 'r') as f:
    content = f.read()

if 'chromedp.ProxyServer' not in content:
    content = content.replace(
        'chromedp.Flag("disable-extensions", true),',
        'chromedp.Flag("disable-extensions", true),\n                chromedp.ProxyServer(f.ProxyURL),'
    )
    with open('/opt/baza-skolkovo/src/fetcher/fetcher.go', 'w') as f:
        f.write(content)
    print('Added ProxyServer to chromedp options')
else:
    print('ProxyServer already present')
