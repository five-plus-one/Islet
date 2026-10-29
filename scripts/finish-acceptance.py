"""Verify and remove the generated release fixture through authenticated APIs."""
import hashlib,json,pathlib,runpy,urllib.request
state=runpy.run_path(str(pathlib.Path(__file__).with_name('initialize-production.py')))
items=state['call']('/api/v1/images?scope=all&limit=100')['items']
fixtures=[item for item in items if item['original_name']=='上线验收.png' and item['size']==245]
assert len(fixtures)==1
item=fixtures[0]
data=urllib.request.urlopen(item['url']).read()
assert hashlib.sha256(data).hexdigest()=='db074be21fbda8c1769fc18f95b2574b7683a85c8c0bb83190dd84d31b62e7ab'
assert urllib.request.urlopen(item['thumb_url']).status==200
trend=state['call']('/api/admin/stats/trend?days=14')['items']
assert trend[-1]['count']>0,'Today upload missing from trend'
for suffix in ('','/purge'):
    request=urllib.request.Request(state['base']+'/api/v1/images/'+str(item['id'])+suffix,method='DELETE')
    assert json.loads(state['client'].open(request).read())['code']==0
remaining=state['call']('/api/v1/images?scope=all&limit=100')['items']
assert len(remaining)==23
print(json.dumps({'production_upload':'verified','preview':'verified','trend':'verified','library_count':len(remaining),'generated_fixture':'removed'}))
