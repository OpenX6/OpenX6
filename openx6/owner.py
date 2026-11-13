"""Public-only holder profiles. No key generation, extraction or implicit owner."""
import base64, json, struct
from pathlib import Path
from .common import require, regular, save, sha

SSH = '/ins/etc/x6_owner_authorized_keys'
ADB = '/adb_keys'
ED = {purpose: '/ins/etc/x6_owner_'+purpose+'_ed25519.pub'
      for purpose in ('gui', 'factory', 'update')}
PATHS = {SSH, ADB, *ED.values()}

def unique_object(pairs):
    result={}
    for key,value in pairs:
        require(key not in result,'Duplicate owner profile field')
        result[key]=value
    return result

def decode64(s):
    b = base64.b64decode(s, validate=True)
    require(base64.b64encode(b).decode() == s, 'Non-canonical base64')
    return b

def ed25519_public(b):
    # Reject noncanonical, off-curve and non-prime-order points. A seed has the
    # same length as a public key: callers must explicitly supply PUBLIC files.
    require(len(b) == 32, 'Ed25519 public key must be 32 raw bytes')
    p = 2**255-19; order = 2**252+27742317777372353535851937790883648493
    v = int.from_bytes(b, 'little'); y = v & (2**255-1); sign = v >> 255
    require(y < p, 'Noncanonical Ed25519 public point')
    d = -121665*pow(121666, -1, p) % p
    den = (d*y*y+1) % p; require(den != 0, 'Invalid Ed25519 point')
    x2 = (y*y-1)*pow(den, -1, p) % p; x = pow(x2, (p+3)//8, p)
    if x*x % p != x2: x = x*pow(2, (p-1)//4, p) % p
    require(x*x % p == x2 and not (x == 0 and sign), 'Invalid Ed25519 point')
    if x & 1 != sign: x = p-x
    def add(a, b):
        x,y=a; u,v=b; t=d*x*u*y*v % p
        require((1+t)%p != 0 and (1-t)%p != 0, 'Invalid point addition')
        return ((x*v+y*u)*pow(1+t,-1,p)%p, (y*v+x*u)*pow(1-t,-1,p)%p)
    q=(x,y); result=(0,1)
    for bit in bin(order)[2:]:
        result=add(result,result)
        if bit=='1':result=add(result,q)
    require(q != (0,1) and result == (0,1), 'Ed25519 public key must have prime order')
    return b

def line_fields(b):
    require(len(b)<4096 and b'PRIVATE KEY' not in b and b'\r' not in b,
            'Public key line rejected')
    s=b.decode('ascii').strip()
    require('\n' not in s and all(ord(c)>=32 for c in s), 'Exactly one public key required')
    return s.split()

def ssh_public(b):
    words=line_fields(b)
    require(2<=len(words)<=3 and words[0] in ('ssh-ed25519','ssh-rsa'),
            'Use one SSH Ed25519 or RSA public key without options')
    wire=decode64(words[1]); fields=[]; pos=0
    while pos<len(wire):
        require(pos+4<=len(wire), 'Truncated SSH public key')
        n=struct.unpack_from('>I',wire,pos)[0];pos+=4
        require(0<n<=len(wire)-pos,'Invalid SSH public field')
        fields.append(wire[pos:pos+n]);pos+=n
    require(fields and fields[0].decode('ascii')==words[0], 'SSH key type mismatch')
    if words[0]=='ssh-ed25519':
        require(len(fields)==2,'SSH Ed25519 field count');ed25519_public(fields[1])
    else:
        require(len(fields)==3,'SSH RSA field count')
        for f in fields[1:]:
            require(not f[0]&128 and not (len(f)>1 and f[0]==0 and not f[1]&128), 'Noncanonical RSA integer')
        e,n=(int.from_bytes(f,'big') for f in fields[1:])
        require(e==65537 and 2048<=n.bit_length()<=8192 and n%2==1,'Unsupported RSA key')
    # Do not embed local user/machine comments in a firmware image.
    return (' '.join(words[:2])+'\n').encode()

def adb_public(b):
    words=line_fields(b);require(1<=len(words)<=2,'Exactly one ADB public key required')
    raw=decode64(words[0]);require(len(raw)==524,'ADB key must be Android RSA2048 format')
    size,inv=struct.unpack_from('<II',raw);n=int.from_bytes(raw[8:264],'little')
    rr=int.from_bytes(raw[264:520],'little');e=struct.unpack_from('<I',raw,520)[0]
    require(size==64 and e==65537 and n.bit_length()==2048 and n%2==1,'Invalid ADB modulus/exponent')
    require((n*inv)&0xffffffff==0xffffffff and rr==pow(2,4096,n),'Invalid ADB Montgomery parameters')
    return (words[0]+'\n').encode()

def validate_owner(path,p=None):
    with regular(path) as f:
        raw=f.read(32769);require(len(raw)<=32768,'Owner profile too large');o=json.loads(raw,object_pairs_hook=unique_object)
    require(set(o)=={'schema','files'} and o['schema']==1 and set(o['files'])==PATHS,'Owner profile fields rejected')
    result={}
    for name,encoded in o['files'].items():
        require(isinstance(encoded,str) and len(encoded)<=5500,'Public key input too large')
        b=decode64(encoded)
        result[name]=ssh_public(b) if name==SSH else adb_public(b) if name==ADB else ed25519_public(b)
    return result

def create_profile(files,dest):
    require(set(files)==PATHS,'All five public roles are required')
    result={}
    for name,path in files.items():
        with regular(path) as f:b=f.read(4097)
        require(len(b)<=4096,'Public key file too large')
        result[name]=ssh_public(b) if name==SSH else adb_public(b) if name==ADB else ed25519_public(b)
    dest=Path(dest);require(not dest.exists(),'Refuse existing owner profile')
    data=dict(schema=1,files={k:base64.b64encode(v).decode() for k,v in result.items()})
    with dest.open('xb') as f:f.write(json.dumps(data,indent=2).encode()+b'\n')
    dest.chmod(0o600)
    return {name:sha(b) for name,b in result.items()}
