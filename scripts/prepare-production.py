"""Prepare an isolated Islet deployment on the verified application host.

The OAuth client secret is read from stdin. All private output stays in root-only files.
Existing database containers and services are not restarted.
"""
import json, os, pathlib, secrets, shutil, subprocess, sys

root = pathlib.Path('/opt/islet-production')
if subprocess.check_output(['hostname'], text=True).strip() != 'RainYun-H1ce5bsB':
    raise RuntimeError('Unexpected deployment host')
if shutil.disk_usage('/').free < 3 * 1024**3:
    raise RuntimeError('Insufficient free space for deployment and recovery')
os.umask(0o077)
backup = root / 'backups/20261029'
backup.mkdir(parents=True, exist_ok=True)
private = root / 'credentials.json'
if private.exists():
    credentials = json.loads(private.read_text())
else:
    credentials = {'database': secrets.token_hex(24), 'password': secrets.token_hex(24), 'username': 'islet-admin'}
    private.write_text(json.dumps(credentials))
oauth = sys.stdin.read().strip()
if len(oauth) < 16 or '\n' in oauth:
    raise RuntimeError('Missing OAuth client credential')
container = '1Panel-mysql-cluster-YpnH'
environment = json.loads(subprocess.check_output(['docker','inspect',container,'--format','{{json .Config.Env}}']))
mysql_password = dict(item.split('=',1) for item in environment)['MYSQL_ROOT_PASSWORD']
process_env = dict(os.environ, MYSQL_PWD=mysql_password)
prefix = ['docker','exec','-i','-e','MYSQL_PWD',container]
database_backup = backup / 'lsky.sql'
if not database_backup.exists():
    with database_backup.open('wb') as out:
        subprocess.run(prefix+['mysqldump','-uroot','--single-transaction','--routines','--triggers','--no-tablespaces','5plus1img'],env=process_env,stdout=out,check=True)
if database_backup.stat().st_size < 1000:
    raise RuntimeError('Source database backup is incomplete')
sql = "CREATE DATABASE IF NOT EXISTS islet CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci; CREATE USER IF NOT EXISTS 'islet'@'%' IDENTIFIED BY '" + credentials['database'] + "'; GRANT ALL ON islet.* TO 'islet'@'%';"
result = subprocess.run(prefix+['mysql','-uroot'],input=sql.encode(),env=process_env,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
if result.returncode:
    raise RuntimeError('Islet database preparation failed')
data = root/'data'; data.mkdir(exist_ok=True);os.chown(data,10001,10001)
cfg = data/'islet.json'
if not cfg.exists():
    cfg.write_text(json.dumps({'app_env':'production','mysql_host':container,'mysql_port':'3306','mysql_database':'islet','mysql_user':'islet','mysql_password':credentials['database'],'auth_mode':'local','oidc_issuer':'https://oauth.five-plus-one.com/oidc','oidc_client_id':'rybk2us4vclda5j8j2du6','oidc_scopes':'openid profile email','admin_base_url':'https://img.five-plus-one.com'}))
    os.chown(cfg,10001,10001)
compose = {'name':'islet-production','services':{'islet':{'image':'islet:production-20261029.01','restart':'unless-stopped','environment':{'APP_ENV':'production','CONFIG_FILE':'/data/islet.json','ADMIN_BASE_URL':'https://img.five-plus-one.com','OIDC_CLIENT_ID':'rybk2us4vclda5j8j2du6','OIDC_CLIENT_SECRET':oauth,'TZ':'Asia/Shanghai'},'ports':['172.16.221.217:18090:8080','172.16.221.217:18091:8081'],'volumes':[str(data)+':/data'],'networks':['db-network']}},'networks':{'db-network':{'external':True}}}
target = root/'compose.json'
if target.exists():
    raise RuntimeError('Candidate already exists; compare it before replacing')
target.write_text(json.dumps(compose,indent=2))
subprocess.run(['docker','compose','-f',str(target),'config','--quiet'],check=True)
print('Source database backed up; isolated database and candidate Compose prepared')
