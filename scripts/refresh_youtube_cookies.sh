#!/bin/bash
# refresh_youtube_cookies.sh — 从 Chrome CDP 导出 YouTube cookies
# Usage: ./refresh_youtube_cookies.sh [output_path]

OUTPUT="${1:-$HOME/guan/code/ytb2bili-go/data/cookies/youtube_cookies.txt}"
mkdir -p "$(dirname "$OUTPUT")"

python3 << 'PYEOF'
import http.client
import json
import sys

output_path = """${OUTPUT}"""

try:
    conn = http.client.HTTPConnection('127.0.0.1', 9222, timeout=5)
    conn.request('GET', '/json')
    resp = conn.getresponse()
    data = json.loads(resp.read())
    if not data:
        print("❌ Chrome CDP not available")
        sys.exit(1)
    
    ws_url = data[0].get('webSocketDebuggerUrl', '')
    
    import websocket
    ws = websocket.create_connection(ws_url, timeout=10)
    ws.send(json.dumps({'id': 1, 'method': 'Network.getAllCookies'}))
    result = json.loads(ws.recv())
    ws.close()
    
    cookies = result.get('result', {}).get('cookies', [])
    yt_cookies = [c for c in cookies if '.youtube.com' in c.get('domain', '')]
    
    count = 0
    with open(output_path, 'w') as f:
        f.write('# Netscape HTTP Cookie File\n')
        f.write('# Generated from Chrome CDP\n\n')
        for c in yt_cookies:
            path = c.get('path', '/')
            secure = 'TRUE' if c.get('secure', False) else 'FALSE'
            expires = c.get('expires', None)
            if expires is None or expires <= 0:
                continue  # skip invalid expiry
            name = c.get('name', '')
            value = c.get('value', '')
            f.write(f'.youtube.com\tTRUE\t{path}\t{secure}\t{int(expires)}\t{name}\t{value}\n')
            count += 1
    
    print(f"✅ Refreshed {count} YouTube cookies → {output_path}")
    
except Exception as e:
    print(f"❌ Failed: {e}")
    sys.exit(1)
PYEOF
