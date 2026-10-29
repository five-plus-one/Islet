"""Apply the verified storage release while preserving deployment settings."""
import copy, hashlib, json, pathlib, re, shutil, subprocess, sys
root=pathlib.Path('/opt/islet-production')
current, target, filename, checksum=sys.argv[1:]
assert all(re.fullmatch(r'islet:production-[0-9.]+',tag) for tag in (current,target))
archive=(root/'releases'/filename).resolve()
assert archive.is_relative_to(root/'releases')
assert hashlib.sha256(archive.read_bytes()).hexdigest()==checksum
path=root/'compose.json';previous=json.loads(path.read_text())
assert previous['services']['islet']['image']==current
shutil.copy2(path,root/'backups/20261029'/('compose-'+current.split(':')[1]+'.json'))
candidate=copy.deepcopy(previous)
candidate['services']['islet']['image']=target
candidate['services']['islet']['healthcheck']={'test':['CMD','wget','-q','-O','/dev/null','http://127.0.0.1:8081/api/v1/health'],'interval':'30s','timeout':'5s','retries':3,'start_period':'15s'}
subprocess.run(['docker','load','-i',str(archive)],check=True)
path.write_text(json.dumps(candidate,indent=2))
subprocess.run(['docker','compose','-f',str(path),'config','--quiet'],check=True)
subprocess.run(['docker','compose','-f',str(path),'up','-d','--no-deps','islet'],check=True)
print('Storage release applied; existing account, database and storage configuration preserved')
