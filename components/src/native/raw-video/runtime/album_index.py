"""Read camera media into a transient thumbnail index; all output stays in /run."""
from raw_delete import item_key, delete_requested
import json
import os
from pathlib import Path
import subprocess
import time
from datetime import datetime
from covers import WIDTH, HEIGHT, frame_cover, photo_cover
from gallery import entries
from file_info import fields

HOME = Path('/run/x5-raw-mode')
CACHE = HOME/'album'
# The firmware's storage enum is SD, internal, external.
VOLUMES = [(1,'Internal Storage','/ins/media_rw/emulated'),(0,'SD Card','/mnt/sdcard'),(2,'External SSD','/mnt/quickreader')]

def atomic(path,data):
    part=path.with_suffix('.part')
    part.write_bytes(data)
    part.replace(path)

def sealed_cover(folder,fps):
    directory=HOME/str(fps)
    with subprocess.Popen([str(directory/f'extract{fps}'),str(folder/'capture.x6b'),'0'],stdout=subprocess.PIPE) as child:
        raw=child.stdout.read(3840*3840*2)
        while child.stdout.read(1048576):pass
        if child.wait() or len(raw)!=3840*3840*2:raise ValueError('Incomplete preview frame')
    return frame_cover(raw,json.loads((HOME/'color-reference.json').read_text()))

def record(path,kind,values,thumbnail,stamp,storage):
    key=item_key(storage,path,stamp)
    dest=CACHE/key
    if not dest.exists():
        rgb=thumbnail()
        if len(rgb)!=WIDTH*HEIGHT*3:raise ValueError('Invalid cover')
        tile=bytearray()
        for y in range(158):
            for x in range(158):
                i=3*((y*HEIGHT//158)*WIDTH+(x*HEIGHT//158+(WIDTH-HEIGHT)//2))
                tile.extend(rgb[i:i+3]);tile.append(255)
        badge=(HOME/('badge-'+('x6b'+values[2].split()[0] if values[3] in ('10-bit Bayer','14-bit Bayer') else 'sequence' if kind==1 else 'interval' if 'interval' in values[0] else 'photo'))).read_bytes()
        for y in range(23):
            for x in range(122):
                off=4*((7+y)*158+x+5)
                v=badge[y*122+x]
                tile[off:off+4]=bytes([v,v,v,255])
        encoded=[v.encode('ascii','replace') for v in values]
        if len(encoded)!=10 or any(not v or len(v)>=80 for v in encoded):raise ValueError('Invalid fields')
        source=path/'capture.x6b' if values[3] in ('10-bit Bayer','14-bit Bayer') else path/'result.json' if kind==1 else path
        atomic(dest,b''.join(v.ljust(80,b'\0') for v in encoded)+tile+os.fsencode(source)+b'\0')
    return stamp,kind,key

def scan(volume,label,root):
    rows=[]
    if root.is_dir():
        for folder in root.glob('RAWVIDEO-*'):
            try:
                values=fields(folder,label)
                fps=int(values[2].split()[0])
                rows.append(record(folder,1,values,lambda:(folder/'cover.rgb').read_bytes() if values[3]=='10-bit Bayer' else sealed_cover(folder,fps),int(folder.stat().st_mtime),volume))
            except (KeyError,OSError,ValueError):continue
        for stamp,kind,path,count,gaps in entries(root):
            try:
                date=datetime.fromtimestamp(stamp).strftime('%Y/%m/%d  %H:%M:%S')
                if kind=='RAW video':
                    fps=json.loads((path/'clip.json').read_text())['fps_setting']
                    date=datetime.strptime(path.name,'RAWVIDEO-%Y%m%d-%H%M%S').strftime('%Y/%m/%d  %H:%M:%S')
                    seconds=(count+gaps)*(1 if fps==50 else 1.001)/fps
                    values=['RAW Video','3840x3840/eye',f'{fps} fps','DNG sequence',path.name,date,f'{seconds:.2f} sec','--',label,f'{count} pairs / {gaps} gaps']
                    thumb=lambda:(path/'cover.rgb').read_bytes()
                else:
                    pieces=path.stem.split('_')
                    if len(pieces)>=3:
                        try:date=datetime.strptime(pieces[1]+pieces[2],'%Y%m%d%H%M%S').strftime('%Y/%m/%d  %H:%M:%S')
                        except ValueError:pass
                    values=[kind,'DNG',f'{count} file'+('s' if count!=1 else ''),'Bayer',path.name,date,'--',f'{path.stat().st_size/1048576:.1f} MiB' if count==1 else '--',label,f'{count} DNG']
                    thumb=lambda:photo_cover(path)
                rows.append(record(path,1 if kind=='RAW video' else 2,values,thumb,int(stamp),volume))
            except (ValueError,KeyError,OSError):continue
    rows.sort(reverse=True)
    return rows

def publish(volume,rows):
    rows=sorted(rows,reverse=True)
    changed=False
    for f in (0,1,2):
        data=b''.join(k.encode() for _,kind,k in rows if not f or f==kind)
        path=CACHE/f'list-{volume}-{f}'
        if not path.exists() or path.read_bytes()!=data:
            atomic(path,data)
            changed=True
    return changed

if __name__=='__main__':
    CACHE.mkdir(exist_ok=True)
    while not (HOME/'exit').exists():
        if (HOME/'busy').read_text().strip()=='0':
            deleted=delete_requested(CACHE,VOLUMES)
            media={}
            for volume,label,root in VOLUMES:
                try:media[volume]=scan(volume,label,Path(root))
                except (OSError, ValueError) as exc:
                    print('Album rescan:',label,exc,flush=True)
                    media[volume]=[]
            changed=False
            for selection,sources in enumerate(((0,), (1,), (0,1), (2,), (1,2))):
                changed |= publish(selection,[row for v in sources for row in media[v]])
            if deleted:atomic(CACHE/'refresh',b'1')
            elif changed:atomic(CACHE/'refresh-index',b'1')
        for _ in range(25):
            if (CACHE/'confirmed').exists():break
            time.sleep(.2)
