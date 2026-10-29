"""Switch only the image host upstream, retaining its exact previous config."""
import pathlib, shutil, subprocess

if subprocess.check_output(['hostname'],text=True).strip()!='RainYun-5q0GYZBf':
    raise RuntimeError('Unexpected gateway')
path=pathlib.Path('/opt/1panel/www/sites/img-5plus1/proxy/root.conf')
previous=path.read_text()
if '172.16.221.217:40027' not in previous:
    raise RuntimeError('Image upstream has changed')
backup=pathlib.Path('/opt/1panel/www/sites/img-5plus1/root.before-islet-20261029.conf')
if backup.exists(): raise RuntimeError('Backup already exists')
shutil.copy2(path,backup)
headers='''
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_http_version 1.1;
    proxy_read_timeout 180s;
    proxy_send_timeout 180s;
'''
candidate='client_max_body_size 110m;\n'
for location,port in [('/i/',18090),('/t/',18090),('/',18091)]:
    candidate+='location ^~ '+location+' {\n    proxy_pass http://172.16.221.217:'+str(port)+';\n'+headers+'}\n'
path.write_text(candidate)
try:
    subprocess.run(['docker','exec','1Panel-openresty-lfiO','nginx','-t'],check=True)
    subprocess.run(['docker','exec','1Panel-openresty-lfiO','nginx','-s','reload'],check=True)
except Exception:
    path.write_text(previous)
    subprocess.run(['docker','exec','1Panel-openresty-lfiO','nginx','-s','reload'],check=True)
    raise
print('Image host switched to Islet; previous upstream configuration retained')
