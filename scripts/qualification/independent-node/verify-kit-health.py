#!/usr/bin/env python3
"""Verify management after a disposable literal kit installation, without applying."""
import http.cookies
import json
import os
from pathlib import Path
import ssl
import subprocess
import urllib.error
import urllib.parse
import urllib.request

if os.geteuid()!=0 or os.environ.get('INDEPENDENT_NODE_FIXTURE')!='1':
    raise SystemExit('explicit disposable root fixture required')
root=Path('/etc/node-qualification')
app=json.loads((root/'envelope.json').read_text())['application']
base='https://'+app['management']['hostname']
class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self,*unused):return None
opener=urllib.request.build_opener(urllib.request.HTTPSHandler(context=ssl.create_default_context()),NoRedirect)
def request(path,body=None,cookie=None):
    headers={'Origin':base}
    if cookie:headers['Cookie']=cookie
    try:
        with opener.open(urllib.request.Request(base+path,data=body,headers=headers),timeout=15) as response:
            return response.status,response.headers,response.read()
    except urllib.error.HTTPError as error:return error.code,error.headers,error.read()
code,_,raw=request('/health/live');health=json.loads(raw)
assert code==200 and health['installation_state']=='installed-unlinked'
for key in ('installation_complete','data_ready','management_ready'):assert health[key] is True
for key in ('processing_enabled','authority_enabled','pod_ready','link_ready'):assert health[key] is False
assert health['node_id']==app['node_id'] and health['installation_id']==app['installation_id']
assert request('/')[0]==401
login=json.loads((root/'login.json').read_text())
code,headers,_=request('/login',urllib.parse.urlencode(login).encode());assert code==303
cookies=http.cookies.SimpleCookie();cookies.load(headers['Set-Cookie'])
cookie='; '.join(k+'='+v.value for k,v in cookies.items())
assert request('/',cookie=cookie)[0]==200
assert request('/activate',b'',cookie)[0]==409
assert request('/logout',b'',cookie)[0]==303
assert request('/',cookie=cookie)[0]==401
subprocess.run(['/usr/local/sbin/siemcore-apply-update','health'],check=True,capture_output=True)
(root/'kit-management-result.json').write_text(json.dumps(dict(node_id=app['node_id'],tls_login_logout=True,activation_rejected=True,root_hook_health=True,health=health),indent=2))
print('PASS: root hook health, trusted TLS authentication/logout, activation disabled; node '+app['node_id'])
