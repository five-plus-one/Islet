"""Retire the migrated image container and update only its health baseline."""
import json, pathlib, shutil, subprocess, urllib.request
root=pathlib.Path('/opt/islet-production')
assert json.load(urllib.request.urlopen('https://img.five-plus-one.com/api/v1/health'))['data']['setup']['complete']
health=subprocess.check_output(['docker','inspect','islet-production-islet-1','--format','{{.State.Health.Status}}'],text=True).strip()
assert health=='healthy'
before=dict(line.split() for line in (root/'backups/20261029/containers.txt').read_text().splitlines())
current=dict(line.split() for line in subprocess.check_output(['docker','ps','--format','{{.Names}} {{.ID}}'],text=True).splitlines())
assert all(current.get(name)==identifier for name,identifier in before.items()), 'An existing service changed'
config=pathlib.Path('/etc/5plus1-ops/healthcheck.json')
shutil.copy2(config,root/'backups/20261029/healthcheck-before.json')
value=json.loads(config.read_text())
value['required_containers']=['islet-production-islet-1' if name=='1Panel-lsky-pro-r9QU' else name for name in value['required_containers']]
value['http_checks'].append({'name':'islet-local','url':'http://172.16.221.217:18091/api/v1/health'})
config.write_text(json.dumps(value,indent=2))
subprocess.run(['docker','update','--restart=no','1Panel-lsky-pro-r9QU'],check=True)
subprocess.run(['docker','stop','1Panel-lsky-pro-r9QU'],check=True)
after=dict(line.split() for line in subprocess.check_output(['docker','ps','--format','{{.Names}} {{.ID}}'],text=True).splitlines())
assert all(after.get(name)==identifier for name,identifier in before.items() if name!='1Panel-lsky-pro-r9QU')
result={'legacy_container':'stopped','other_containers_unchanged':len(before)-1,'islet_health':health,'original_data_retained':True}
(root/'backups/20261029/cutover-result.json').write_text(json.dumps(result))
print(json.dumps(result))
