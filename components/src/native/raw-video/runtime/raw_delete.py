"""Resolve confirmed album selections to the same files that were indexed."""
import hashlib
import os
import re
import stat
import shutil
import tempfile
from pathlib import Path

from gallery import dcf_dng

def item_key(volume, path, stamp):
    st = path.stat()
    return hashlib.sha256(f'grid2:{volume}:{path}:{stamp}:{st.st_dev}:{st.st_ino}'.encode()).hexdigest()[:24]

def real_member(root, path):
    if path.resolve(strict=True) != path or not path.is_relative_to(root):
        raise ValueError('Media path changed')
    if path.stat().st_dev != root.stat().st_dev:
        raise ValueError('Storage changed')

def resolve_item(key, cache, volumes):
    data = (cache/key).read_bytes()
    source = Path(os.fsdecode(data[800+158*158*4:-1]))
    for volume, _, root in volumes:
        root = Path(root)
        if source.is_relative_to(root):
            break
    else:
        raise ValueError('Unknown storage')
    real_member(root, source)
    if source.parent.parent == root and re.fullmatch(r'RAWVIDEO-\d{8}-\d{6}',source.parent.name):
        path = source.parent
        if source.name not in ('capture.x6b','result.json'):
            raise ValueError('Unknown take')
        if item_key(volume,path,int(path.stat().st_mtime)) != key:
            raise ValueError('Take changed')
        known = {'capture.x6b','clip.json','seal.json','calibration.json','gyro.csv','gyro-info.json',
                 'recording-status.json','recording-status.json.tmp','capture-tail.log','capture-tail.log.tmp',
                 'audio.aac.part','audio.wav.part','audio-info.json.tmp','clip.json.tmp','seal.json.tmp',
                 'gyro-0.part','gyro-1.part','gyro-info.json.tmp','gyro.csv.tmp',
                 'result.json','cover.rgb','frames.csv','frames.jsonl','gyro.log','CAM0','CAM1'}
        if source.name == 'capture.x6b':
            known.update(('exposure.csv','exposure.csv.tmp','exposure.part','accel.part','accel-info.json.tmp','audio.wav','audio.aac','audio-info.json','.writer.log','accel.csv','accel-info.json','sensor-metadata.json'))
        if source.name == 'result.json':
            # The original on-camera DNG writer keeps its final progress receipt.
            known.add('progress.json')
        for entry in path.iterdir():
            real_member(root,entry)
            if entry.name not in known:
                raise ValueError(f'Unexpected file: {entry.name}')
            if entry.name in ('CAM0','CAM1'):
                if not entry.is_dir():raise ValueError('Invalid eye folder')
                for dng in entry.iterdir():
                    real_member(root,dng)
                    if not dng.is_file() or not re.fullmatch(r'\d+\.dng',dng.name,re.I):
                        raise ValueError('Unexpected eye file')
            elif not entry.is_file():raise ValueError('Invalid take file')
        return [(root,path)]
    if source.suffix.lower() != '.dng' or not source.is_relative_to(root/'DCIM'):
        raise ValueError('Invalid photo')
    if item_key(volume,source,int(source.stat().st_mtime)) != key:
        raise ValueError('Photo changed')
    rows=dcf_dng(root/'MISC/dcf_management.db')
    groups={group for group,p,_ in rows if Path(p)==source}
    if len(groups)!=1:raise ValueError('Photo group changed')
    files=[Path(p) for group,p,_ in rows if group in groups and Path(p).exists()]
    expected=int(data[160:240].split()[0])
    if len(files)!=expected or len(set(files))!=expected:
        raise ValueError('Interval membership changed')
    for p in files:
        real_member(root,p)
        if not p.is_relative_to(root/'DCIM') or p.suffix.lower()!='.dng' or not p.is_file():
            raise ValueError('Invalid DNG group')
    return [(root,p) for p in files]

def delete_requested(cache, volumes):
    req=cache/'confirmed'
    if not req.exists():return False
    claimed=cache/'deleting'
    req.replace(claimed)
    try:
        keys=claimed.read_bytes()
        if not keys or len(keys)%24 or not re.fullmatch(b'[a-f0-9]+',keys):
            raise ValueError('Invalid selection')
        moves=[]
        for off in range(0,len(keys),24):
            moves.extend(resolve_item(keys[off:off+24].decode(),cache,volumes))
        if len(set(moves))!=len(moves):raise ValueError('Duplicate selection')
        trash={};staged=[]
        try:
            for root,path in moves:
                if root not in trash:
                    trash[root]=Path(tempfile.mkdtemp(prefix='.x6-delete-',dir=root))
                dest=trash[root]/str(len(staged))
                path.rename(dest)
                staged.append((path,dest))
        except Exception:
            for path,dest in reversed(staged):
                if not path.exists():dest.rename(path)
            for folder in trash.values():
                if not any(folder.iterdir()):folder.rmdir()
            raise
        for folder in trash.values():shutil.rmtree(folder)
        (cache/'last-delete').write_text(f'Deleted {len(keys)//24}')
    except (OSError,ValueError) as e:
        (cache/'last-delete').write_text(f'Delete failed: {e}')
    finally:claimed.unlink()
    return True
