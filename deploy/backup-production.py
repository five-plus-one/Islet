"""Back up Islet's database and encryption configuration to restricted host storage."""
import datetime, gzip, json, os, pathlib, shutil, subprocess
os.umask(0o077)
root=pathlib.Path('/opt/islet-production')
cfg=json.loads((root/'data/islet.json').read_text())
target=root/'backups/daily'/datetime.datetime.now().strftime('%Y%m%d-%H%M%S')
target.mkdir(parents=True,mode=0o700)
environment=dict(os.environ,MYSQL_PWD=cfg['mysql_password'])
result=subprocess.run(['docker','exec','-e','MYSQL_PWD',cfg['mysql_host'],'mysqldump','-u'+cfg['mysql_user'],'--single-transaction','--no-tablespaces',cfg['mysql_database']],env=environment,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
if result.returncode or len(result.stdout)<1000:
    raise RuntimeError('Islet database backup failed')
with gzip.open(target/'database.sql.gz','wb') as output:output.write(result.stdout)
for name,source in [('islet.json',root/'data/islet.json'),('islet.key',root/'data/islet.key'),('compose.json',root/'compose.json'),('credentials.json',root/'credentials.json')]:
    shutil.copyfile(source,target/name)
assert len(gzip.decompress((target/'database.sql.gz').read_bytes()))==len(result.stdout)
print('Islet database and encryption configuration backed up')
