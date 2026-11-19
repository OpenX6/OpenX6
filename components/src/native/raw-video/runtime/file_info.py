"""Prepare the native info-page fields for one selected, sealed X6B take."""
from datetime import datetime
import json
from pathlib import Path
import re
import sys

def fields(folder, storage):
    folder = Path(folder)
    if not re.fullmatch(r'RAWVIDEO-\d{8}-\d{6}', folder.name):
        raise ValueError('Invalid take name')
    clip = json.loads((folder/'clip.json').read_text())
    seal = json.loads((folder/'seal.json').read_text())
    size = (folder/'capture.x6b').stat().st_size
    fps = clip['fps_setting']
    if clip['layout']=='single-bayer-v1':
        first,last=seal['hardware_window']
        frames=seal['frames']
        if not (clip['eye']==seal['eye'] and clip['size']==[7680,7680] and fps==30 and size==seal['bytes'] and 0<frames<=last-first+1):
            raise ValueError('Incomplete single-eye take')
        shot=datetime.strptime(folder.name,'RAWVIDEO-%Y%m%d-%H%M%S')
        return ['RAW Video', '7680x7680 '+clip['lens'], '30 fps','10-bit Bayer',
                folder.name+'/capture.x6b',shot.strftime('%Y/%m/%d  %H:%M:%S'),
                f'{(last-first+1)/29.969:.2f} sec',f'{size/1048576:.1f} MiB',storage,
                f'{frames} frames / {seal["gaps"]} gaps']
    lo, hi = seal['request_window']
    count = hi-lo+1
    saved = seal['pairs']
    if not (clip['layout']=='paired-bayer-v1' and clip['size']==[3840,3840]
            and fps in (30,50,60) and fps==seal['fps_setting'] and size==seal['bytes']>0
            and hi>=lo>=0 and 0<saved<=seal['received_pairs']<=count):
        raise ValueError('Incomplete or unsupported RAW take')
    duration = count*(1 if fps==50 else 1.001)/fps
    shot = datetime.strptime(folder.name,'RAWVIDEO-%Y%m%d-%H%M%S')
    return ['RAW Video', f'3840x3840/eye', f'{fps} fps', '14-bit Bayer',
            folder.name+'/capture.x6b', shot.strftime('%Y/%m/%d  %H:%M:%S'),
            f'{duration:.2f} sec', f'{size/1048576:.1f} MiB', storage,
            f'{saved} pairs / {count-saved} gaps']

def request_info(folder, storage, destination):
    record = b''
    for value in fields(folder,storage):
        text = value.encode('ascii')
        if len(text)>=80:
            raise ValueError('Info field too long')
        record += text.ljust(80,b'\0')
    pending = destination.with_suffix('.tmp')
    pending.write_bytes(record)
    pending.replace(destination)

if __name__ == '__main__':
    request_info(sys.argv[1],sys.argv[2],Path('/run/x5-raw-mode/selected-info'))
