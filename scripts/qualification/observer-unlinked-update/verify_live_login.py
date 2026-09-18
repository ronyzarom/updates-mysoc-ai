#!/usr/bin/env python3
"""Authenticated live acceptance; credentials/cookies never printed or recorded.
Creates and logs out only its own test session. Does not mutate product settings.
"""
import json,pathlib,http.client,urllib.parse,ssl
import argparse
p=argparse.ArgumentParser();p.add_argument('--hostname',required=True);p.add_argument('--credentials',required=True);p.add_argument('--version',required=True);p.add_argument('--output',required=True);args=p.parse_args()
host=args.hostname
creds=json.loads(pathlib.Path(args.credentials).read_text())
def request(method,path,body=None,headers=None):
 c=http.client.HTTPSConnection(host,timeout=15,context=ssl.create_default_context())
 try:
  c.request(method,path,body=body,headers=headers or {});r=c.getresponse();return r.status,dict((k.lower(),v) for k,v in r.getheaders()),r.read()
 finally:c.close()
results={}
code,h,b=request('GET','/');assert code==401 and 'www-authenticate' not in h and b'<form' in b;results['anonymous_root']=code
code,h,b=request('GET','/login');assert code==200 and b'<form' in b;results['login_page']=code
code,h,b=request('GET','/observer-icon.svg');assert code==200 and 'image/svg+xml' in h.get('content-type','');results['icon']=code
form=urllib.parse.urlencode({'username':creds['username'],'password':'deliberately-invalid-acceptance-probe'})
headers={'Origin':'https://'+host,'Content-Type':'application/x-www-form-urlencoded'}
code,h,b=request('POST','/login',form,headers);assert code==401 and 'www-authenticate' not in h;results['wrong_password']=code
form=urllib.parse.urlencode({'username':creds['username'],'password':creds['password']})
code,h,b=request('POST','/login',form,headers);assert code==303 and h.get('location')=='/';results['form_login']=code
cookie=h['set-cookie'];assert all(x in cookie for x in ['__Host-observer-session=','Path=/','Secure','HttpOnly','SameSite=Strict','Max-Age=28800'])
results['secure_session_attributes']=True
session=cookie.split(';',1)[0]
code,h,b=request('GET','/',headers={'Cookie':session});assert code==200;results['session_root']=code
code,h,b=request('POST','/logout',headers={'Cookie':session,'Origin':'https://invalid.example'});assert code==403;results['cross_origin_rejected']=code
code,h,b=request('POST','/logout',headers={'Cookie':session,'Origin':'https://'+host});assert code==303;results['logout']=code
code,h,b=request('GET','/',headers={'Cookie':session});assert code==401;results['logged_out_session_rejected']=code
code,h,b=request('GET','/health/live');health=json.loads(b);assert code==200 and health['version']==args.version;results['health']=health
pathlib.Path(args.output).write_text(json.dumps(results,indent=2)+'\n')
print(json.dumps(results))
