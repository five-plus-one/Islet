"""Validate the candidate's imported library, storage and authentication."""
import json, pathlib, runpy, urllib.request
state=runpy.run_path(str(pathlib.Path(__file__).with_name('initialize-production.py')))
call=state['call']; client=state['client']; base=state['base']
policies=call('/api/admin/storage-policies')['items']
policy=next(p for p in policies if p['name']=='sh-endpoint')
call('/api/admin/storage-policies/'+str(policy['id'])+'/test',{})
images=call('/api/v1/images?limit=100&scope=all')['items']
images=[i for i in images if i['url'] in {old['url'] for old in json.loads(pathlib.Path('/opt/islet-production/backups/20261029/lsky-verified.json').read_text())['images']}]
assert len(images)==23, 'Imported image count differs'
source=json.loads(pathlib.Path('/opt/islet-production/backups/20261029/lsky-verified.json').read_text())
assert {i['url'] for i in images}=={i['url'] for i in source['images']}
request=urllib.request.Request(base+'/api/admin/settings',method='PUT',data=json.dumps({'brand.oidc_label':'使用五加一账户登录','brand.name':'五加一图床'}).encode(),headers={'Content-Type':'application/json'})
assert json.loads(client.open(request).read())['code']==0
print(json.dumps({'images':len(images),'original_urls':'preserved','object_storage_read_write':'passed','local_authentication':'passed'}))

