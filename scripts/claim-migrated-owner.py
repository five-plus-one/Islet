"""Assign the library to the signed-in OAuth identity matching the legacy owner."""
import json, os, pathlib, subprocess, sys
container='1Panel-mysql-cluster-YpnH'
env=json.loads(subprocess.check_output(['docker','inspect',container,'--format','{{json .Config.Env}}']))
password=dict(x.split('=',1) for x in env)['MYSQL_ROOT_PASSWORD']
environment=dict(os.environ,MYSQL_PWD=password)
def sql(statement):
    result=subprocess.run(['docker','exec','-i','-e','MYSQL_PWD',container,'mysql','-uroot','-N','-B'],input=statement.encode(),env=environment,stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True)
    return result.stdout.decode().strip()
rows=sql("SELECT n.id FROM islet.users n JOIN 5plus1img.users o ON LOWER(n.email)=LOWER(o.email) WHERE n.auth_source='oidc' AND n.sso_id IS NOT NULL AND n.deleted_at IS NULL;").splitlines()
if len(rows)!=1 or not rows[0].isdigit():
    raise RuntimeError('A unique verified sign-in matching the legacy owner is required')
owner=int(rows[0])
if '--apply' not in sys.argv:
    print(json.dumps({'matched_legacy_owner':True,'oauth_user_id':owner}));sys.exit()
sql("START TRANSACTION; UPDATE islet.users SET role='super_admin',group_id=1,status='active',updated_at=NOW() WHERE id="+str(owner)+"; UPDATE islet.images SET user_id="+str(owner)+" WHERE user_id=1; UPDATE islet.installation_states SET owner_user_id="+str(owner)+" WHERE id=1; COMMIT;")
record={'owner_id':owner,'matching_legacy_email':True,'local_recovery_account_retained':True}
pathlib.Path('/opt/islet-production/backups/20261029/owner-migration.json').write_text(json.dumps(record))
print(json.dumps(record))
