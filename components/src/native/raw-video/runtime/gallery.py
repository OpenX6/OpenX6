"""Build the read-only RAW browser from native DCF groups and committed RAW takes."""
import json, os, ctypes as c, struct, sys, time
from pathlib import Path
from covers import photo_cover, WIDTH, HEIGHT

VOLUMES=('/ins/media_rw/emulated','/mnt/sdcard','/mnt/quickreader')
RUNTIME=Path('/run/x5-raw-gallery')


def dcf_dng(db):
    lib=c.CDLL('libsqlite3.so.0')
    conn=c.c_void_p()
    lib.sqlite3_open_v2.argtypes=[c.c_char_p,c.POINTER(c.c_void_p),c.c_int,c.c_char_p]
    if lib.sqlite3_open_v2(os.fsencode(db),c.byref(conn),1,None):
        if conn:lib.sqlite3_close(conn)
        return []
    rows=[]
    callback=c.CFUNCTYPE(c.c_int,c.c_void_p,c.c_int,c.POINTER(c.c_char_p),c.POINTER(c.c_char_p))
    @callback
    def append(_,n,v,keys):
        if n==3 and v[0] and v[1]:rows.append((int(v[0]),os.fsdecode(v[1]),int(v[2] or b'-1')))
        return 0
    sql=b"""select group_id,path,cam_submode from file_meta where group_id in
    (select group_id from file_meta group by group_id having count(*)=sum(lower(substr(name,-4))='.dng'))
    order by group_id,group_seq,id"""
    try:
        if lib.sqlite3_exec(conn,sql,append,None,None):raise OSError('DCF query failed')
    finally:lib.sqlite3_close(conn)
    return rows


def entries(root):
    root=Path(root)
    db=root/'MISC/dcf_management.db'
    if db.is_file():
        groups={}
        for group,path,submode in dcf_dng(db):
            groups.setdefault((group,submode),[]).append(Path(path))
        for (_,submode),paths in groups.items():
            files=[p for p in paths if p.is_relative_to(root/'DCIM') and p.is_file()]
            if not files:continue
            p=files[0];kind='RAW interval' if submode==2 else 'RAW photo' if len(paths)==1 else 'RAW group'
            yield (p.stat().st_mtime,kind,p,len(files),None)
    for folder in root.glob('RAWVIDEO-*'):
        try:
            info=json.loads((folder/'result.json').read_text())
            count=int(info['saved_pairs']);drops=int(info['dropped_pairs'])
            if count>0 and info['state'] in ('saved','saved_with_gaps','error'):
                yield (folder.stat().st_mtime,'RAW video',folder,count,drops)
        except (FileNotFoundError,ValueError,KeyError):continue


def publish(index,rows):
    if not rows:
        label=b'No RAW captures'.ljust(192,b'\0')
        tmp=RUNTIME/'card.part';tmp.write_bytes(label+bytes(WIDTH*HEIGHT*3));tmp.replace(RUNTIME/'card')
        return
    entry=rows[index]
    date,kind,path,count,drops=entry
    if kind=='RAW video':
        text=f'{count} pairs  /  {drops} gaps'
        try:rgb=(path/'cover.rgb').read_bytes()
        except OSError:rgb=b''
    else:
        text=(path.stem if count==1 else f'{count} DNG')
        try:rgb=photo_cover(path)
        except (ValueError,OSError):rgb=b''
    if len(rgb)!=WIDTH*HEIGHT*3:rgb=bytes(WIDTH*HEIGHT*3)
    title=f'{kind}   {index+1} / {len(rows)}'
    record=lambda t:t.encode('ascii','replace')[:63].ljust(64,b'\0')
    data=record(title)+record(time.strftime('%m-%d %H:%M:%S',time.localtime(date)))+record(text)+rgb
    tmp=RUNTIME/'card.part';tmp.write_bytes(data);tmp.replace(RUNTIME/'card')


def main():
    last=None;page=0;rows=[]
    while True:
        try:
            req=(RUNTIME/'request').read_bytes()
            if req!=last and len(req)==8:
                page,serial=struct.unpack('<II',req)
                rows=sorted((r for volume in VOLUMES for r in entries(volume)),key=lambda r:r[0],reverse=True)
                publish(page%len(rows) if rows else 0,rows)
                last=req
        except (OSError,ValueError,KeyError,struct.error) as e: print(e,flush=True)
        time.sleep(.18)
if __name__=='__main__':main()
