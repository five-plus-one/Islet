"""Compare the legacy CDN against authenticated OSS reads and archive originals."""
import base64, hashlib, hmac, json, pathlib, urllib.parse, urllib.request
from email.utils import formatdate

root=pathlib.Path('/opt/islet-production/backups/20261029')
source=json.loads((root/'lsky-export.json').read_text())
policy=source['policy']; mismatches=[]; total=0
objects=root/'originals'; objects.mkdir(mode=0o700,exist_ok=True)
for index,item in enumerate(source['images']):
    date=formatdate(usegmt=True)
    canonical='GET\n\n\n'+date+'\n/'+policy['bucket']+'/'+item['key']
    signature=base64.b64encode(hmac.new(policy['secret_key'].encode(),canonical.encode(),hashlib.sha1).digest()).decode()
    endpoint=urllib.parse.urlparse(policy['endpoint'])
    url='https://'+policy['bucket']+'.'+endpoint.hostname+'/'+urllib.parse.quote(item['key'],safe='/')
    request=urllib.request.Request(url,headers={'Date':date,'Authorization':'OSS '+policy['access_key']+':'+signature})
    original=urllib.request.urlopen(request,timeout=90).read()
    public=urllib.request.urlopen(item['url'],timeout=90).read()
    if original != public: raise RuntimeError('CDN differs from original at index '+str(index))
    checksum=hashlib.sha1(original).hexdigest()
    if checksum != item['sha1']: mismatches.append(index)
    item['sha1']=checksum
    (objects/(str(index)+'.bin')).write_bytes(original)
    total+=len(original)
(root/'lsky-verified.json').write_text(json.dumps(source))
(root/'verification.json').write_text(json.dumps({'images':len(source['images']),'bytes':total,'legacy_digest_mismatch_indices':mismatches,'origin_matches_cdn':True}))
print((root/'verification.json').read_text())
