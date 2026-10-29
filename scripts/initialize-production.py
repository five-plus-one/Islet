"""Initialize the isolated candidate using host-local protected credentials."""
import http.cookiejar, json, os, pathlib, urllib.request

root = pathlib.Path('/opt/islet-production')
cfg = json.loads((root/'data/islet.json').read_text())
credentials = json.loads((root/'credentials.json').read_text())
base = os.environ.get('ISLET_PRODUCTION_URL','http://172.16.221.217:18091')
client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
def call(path, body=None):
    request = urllib.request.Request(base+path, data=None if body is None else json.dumps(body).encode(), headers={'Content-Type':'application/json'})
    response = json.loads(client.open(request, timeout=60).read())
    if response.get('code') != 0:
        raise RuntimeError('Candidate request failed: '+path)
    return response['data']
if call('/api/setup/status')['status'] != 'ready':
    database = {k:v for k,v in cfg.items() if k.startswith('mysql_')}
    call('/api/setup/test/database',database)
    call('/api/setup/verify/admin',dict(mode='local',username=credentials['username'],password=credentials['password']))
    call('/api/setup/complete',dict(database=database,site_title='五加一图床',public_base_url='https://img.five-plus-one.com'))
call('/api/v1/auth/local/login',dict(username=credentials['username'],password=credentials['password']))
me = call('/api/v1/auth/me')
assert me['role'] == 'super_admin'
print(json.dumps({'status':call('/api/setup/status')['status'],'owner_id':me['id'],'local_login':'passed'}))
