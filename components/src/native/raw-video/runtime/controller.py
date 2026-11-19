"""Temporary unit-five RAW mode controller; only the selected take owns the acquisition lock."""
import json
import os
from capture_window import MAX_RECORD, RESERVE
from raw_stats import Overlay, select_source, STOCK
import shutil
import subprocess as sp
import time
from pathlib import Path
from mode_session import ModeSession

root=Path('/run/x5-raw-mode')
notice=Path('/run/x5-raw-gallery/notice')
card=Path('/mnt/quickreader')


def publish(file,data):
    file=file if isinstance(file,Path) else root/file
    data=data.encode()
    if file.exists() and file.read_bytes()==data:return
    tmp=file.with_name(file.name+'.new')
    tmp.write_bytes(data)
    tmp.replace(file)


def consume(name):
    file=root/name
    try:file.rename(root/(name+'.read'))
    except FileNotFoundError:return None
    buf=(root/(name+'.read')).read_bytes()
    return buf[0] if len(buf)==1 else None


def api(cmd,action,**params):
    out=sp.check_output(['ins_bll_cli',json.dumps(dict(cmd=cmd,action=action,**params))],timeout=4)
    j=json.loads(out)
    if j.get('ret')!='ok':raise RuntimeError(j)
    return j['data']


def title(fps):
    return {31:"RAW 8K30 Back",32:"RAW 8K30 Front"}.get(fps, f"RAW {fps} fps")


def recording_profile(selection):
    if selection not in (31, 32):
        return selection
    from storage import selected_external
    selected_external(card)
    role = api('config', 'get_general', key='kMediaPanoCamRole').get('cam_role')
    try:
        return {'kCamBack': 31, 'kCamFront': 32}[role]
    except KeyError:
        raise RuntimeError('Cannot read the selected lens')


def idle_title(selection):
    return 'RAW 8K Single Lens' if selection in (31, 32) else title(selection)


def start(fps):
    from storage import selected_external
    selected_external(card)
    session.begin()
    if api('media','get_media_status')['record_status']['status']!='RecordIdle':
        raise RuntimeError('Camera busy')
    if shutil.disk_usage(card).free <= RESERVE + MAX_RECORD:
        raise RuntimeError('Card space low')
    run=root/f'take-{time.monotonic_ns()}'
    run.mkdir()
    for p in (root/str(fps)).iterdir():os.symlink(p,run/p.name)
    if fps in (31,32):
        return start_single(run, fps-31),run
    log=(run/'capture.log').open('wb')
    try:
        process=sp.Popen(['python3',str(run/'priority.py'),str(run/'trial.py'),'card'],cwd=run,stdout=log,stderr=log)
    finally:log.close()
    return process,run


def start_single(run,eye):
    # A separate service runs cleanup even if the UI/controller exits during capture.
    args=['systemd-run','--quiet','--wait','--collect','--unit=x5-raw-single',
          '-p','BindsTo=x5-raw-control.service','-p','After=x5-raw-control.service',
          '-p','TimeoutStopSec=45','-p','WorkingDirectory='+str(run),
          '-p','ExecStopPost=/usr/bin/python3 '+str(run/'single.py')+' restore',
          '--setenv=EYE='+str(eye),'/usr/bin/python3',str(run/'priority.py'),str(run/'single.py')]
    with (run/'capture.log').open('w') as f:
        return sp.Popen(args,stdout=f,stderr=f)
def saved_notice(run,overlay,fps):
    folder=Path((run/'destination').read_text())
    seal=json.loads((folder/'seal.json').read_text())
    warning=' - audio error' if seal.get('audio_status')=='error' else ''
    if fps in (31,32):
        if seal.get('eye')!='CAM'+str(fps-31) or not seal['frames'] or (folder/'capture.x6b').stat().st_size!=seal['bytes']:
            raise ValueError('Single RAW take incomplete')
        return f"X6B saved   {seal['frames']} frames   {seal['gaps']} gaps"+warning
    pairs=seal['pairs']
    lo,hi=seal['request_window']
    gaps=hi-lo+1-pairs
    if not (0<pairs<=seal['received_pairs']<=hi-lo+1 and
            seal['bytes']>0 and (folder/'capture.x6b').stat().st_size==seal['bytes']):
        raise ValueError('RAW take incomplete')
    overlay.finish(fps,seal)
    if not seal.get('metadata_errors'):
        (folder/'.writer.log').unlink(missing_ok=True)
    return f'X6B saved   {pairs} pairs   {gaps} gaps'+warning


def failure_message(run):
    failure = run/'failure'
    if failure.is_file():
        return failure.read_text().strip()
    if not (run/'destination').is_file():
        return 'RAW did not start - check card and camera'
    path = run/'card-data/bench.log'
    if path.exists():
        with path.open('rb') as f:
            f.seek(max(0,path.stat().st_size-8192))
            tail=f.read()
        if b'SSD writev' in tail or b'SSD flush' in tail:
            return 'Card write failed - RAW file incomplete'
    return 'RAW file not sealed - see take log'


session=ModeSession(api)

def main():
    overlay=Overlay()
    fps=0; active=None; take_profile=0; last=0; pano=False; stop=False
    while True:
        time.sleep(.065)
        if not active and time.monotonic()-last>1:
            last=time.monotonic()
            try:pano=api('config','get_general',key='kMediaMode')['lens_mode']=='kPanorama'
            except (sp.SubprocessError,KeyError,ValueError,RuntimeError):pano=False
            publish('pano','1' if pano else '0')
            if not pano and fps:
                fps=0
                try:session.leave()
                except Exception as e:print('RAW restore deferred',e,flush=True)
            if not fps:
                try:session.recover()
                except Exception as e:print('RAW restore deferred',e,flush=True)
        sel=consume('select')
        if sel is not None and not active:
            fps=sel if sel in (30,50,60,31,32) else 0
            try:
                if fps:
                    publish('status','Preparing RAW')
                    session.begin();pano=True
                    publish('status',idle_title(fps))
                else:session.leave()
            except Exception as e:
                publish(notice,str(e)[:60]);publish('last-error',str(e));publish('status','RAW preparation failed')
        overlay.update(take_profile if active else fps,active[1] if active else None,stop)
        event=consume('shutter')
        if active:
            proc,run=active
            if event is not None and not stop:
                (run/'stop').touch();stop=True;publish('status','Finishing RAW')
            if proc.poll() is None:
                if not stop and (run/'capturing').exists():publish('status','REC   '+title(take_profile))
                continue
            try:
                if (run/'failure').is_file():raise RuntimeError((run/'failure').read_text())
                if proc.returncode or not (Path((run/'destination').read_text())/'seal.json').is_file():raise RuntimeError(failure_message(run))
                message=saved_notice(run,overlay,take_profile)
            except Exception as e:
                print(run,e,flush=True);publish(notice,str(e)[:60])
            else:publish(notice,message)
            publish('status',idle_title(fps));active=None;stop=False;publish('busy','0')
        elif event==fps and fps and pano:
            try:
                take_profile=recording_profile(fps)
                active=start(take_profile)
            except Exception as e:
                print(e,flush=True);publish('last-error',str(e));publish(notice,str(e)[:60])
            else:publish('busy','1');publish('status','Starting RAW')
        if not active and (root/'exit').exists():return

if __name__=='__main__':
    try:main()
    finally:select_source(STOCK)
