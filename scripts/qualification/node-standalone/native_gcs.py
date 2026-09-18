"""Retrieve only synthetic manifest objects with the approved fixture credential."""
import base64,gzip,hashlib,json,time,urllib.parse,urllib.request
from cryptography.hazmat.primitives import hashes,serialization
from cryptography.hazmat.primitives.asymmetric import padding

def verify_objects(credential,bucket,rows,marker):
 key=json.loads(credential.read_text());now=int(time.time())
 encode=lambda b:base64.urlsafe_b64encode(b).rstrip(b'=')
 claim={'iss':key['client_email'],'scope':'https://www.googleapis.com/auth/devstorage.read_write','aud':'https://oauth2.googleapis.com/token','iat':now,'exp':now+1200}
 message=encode(b'{"alg":"RS256","typ":"JWT"}')+b'.'+encode(json.dumps(claim,separators=(',',':')).encode())
 private=serialization.load_pem_private_key(key['private_key'].encode(),password=None)
 assertion=(message+b'.'+encode(private.sign(message,padding.PKCS1v15(),hashes.SHA256()))).decode()
 request=urllib.request.Request('https://oauth2.googleapis.com/token',data=urllib.parse.urlencode({'grant_type':'urn:ietf:params:oauth:grant-type:jwt-bearer','assertion':assertion}).encode())
 with urllib.request.urlopen(request,timeout=30) as response:token=json.load(response)['access_token']
 found=[]
 for row in rows:
  if not row.get('uploaded_at'):continue
  if row.get('cloud_bucket')!=bucket or not row.get('s3_key'):raise ValueError('fixture_manifest_destination_mismatch')
  object_url='https://storage.googleapis.com/storage/v1/b/'+urllib.parse.quote(bucket,safe='')+'/o/'+urllib.parse.quote(row['s3_key'],safe='')
  request=urllib.request.Request(object_url+'?alt=media',headers={'Authorization':'Bearer '+token})
  with urllib.request.urlopen(request,timeout=30) as response:raw=response.read(32*1024*1024+1)
  if len(raw)>32*1024*1024:raise ValueError('fixture_object_too_large')
  if hashlib.sha256(raw).hexdigest()!=row['checksum']:raise ValueError('gcs_object_checksum_mismatch')
  try:decoded=gzip.decompress(raw)
  except (OSError,EOFError):decoded=raw
  if marker.encode() in decoded:found.append(row['s3_key'])
 # Keep the synthetic object for independent product-team retrieval proof.
 # No bucket enumeration, generic deletion, or live database reads.
 return found
