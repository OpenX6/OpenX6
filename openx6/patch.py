"""Exact-hash, bounded non-overlapping binary patch recipes."""
import base64, zlib
from .common import require, sha

def make_recipe(base, target):
    edits=[]; n=len(target)
    for pos in range(0,n,4096):
        a=base[pos:pos+4096];b=target[pos:pos+4096]
        if a==b:continue
        i=0
        while i<len(b):
            if i<len(a) and a[i]==b[i]:i+=1;continue
            start=i;i+=1
            while i<len(b) and (i>=len(a) or a[i]!=b[i]):i+=1
            raw=b[start:i]
            edits.append(dict(offset=pos+start,bytes=len(raw),before=sha(base[pos+start:pos+i]),data=base64.b64encode(zlib.compress(raw,9)).decode()))
    return dict(version=1,base_sha256=sha(base),base_bytes=len(base),target_sha256=sha(target),target_bytes=n,edits=edits)

def apply_recipe(base, spec):
    require(spec['version']==1 and len(base)==spec['base_bytes'] and sha(base)==spec['base_sha256'],'Patch base rejected')
    n=spec['target_bytes'];require(type(n) is int and 0<n<=256*1024*1024,'Patch output size rejected')
    output=bytearray(base[:n]);output.extend(bytes(n-len(output)))
    end=0
    for e in spec['edits']:
        p=e['offset'];size=e['bytes'];require(type(p) is int and type(size) is int and p>=end and size>0 and p+size<=n,'Overlapping/out-of-bounds patch')
        require(sha(base[p:p+size])==e['before'],'Patch original bytes rejected')
        dec=zlib.decompressobj();raw=dec.decompress(base64.b64decode(e['data'],validate=True),size+1)
        require(len(raw)==size and dec.eof and not dec.unused_data and not dec.unconsumed_tail,'Patch literal size rejected')
        output[p:p+size]=raw;end=p+size
    require(sha(output)==spec['target_sha256'],'Patched component digest rejected')
    return bytes(output)
