"""One headless sensor. The independent systemd job restores the stock camera on exit."""
import csv
import json
import os
import re
import signal
import subprocess as sp
import sys
import time
from pathlib import Path

RUN = Path.cwd()
ROOT = Path('/run/x5-raw-mode')
TR = Path('/sys/kernel/tracing')
NAME = 'openx6_single_raw'
INSTANCE = TR/'instances'/NAME
CARD = Path('/mnt/quickreader')
EYE = int(os.environ['EYE'])
sys.path.insert(0, str(ROOT/str(31+EYE)))
from deferred_meta import recording, put, anchor
from storage import selected_external
from raw_ae import RawAutoExposure, start_commands
from take_status import save_stop_reason, finalize_single, finish_optional

def system(*args):
    sp.run(['systemctl',*args],check=True,timeout=45)

def event(kind, spec):
    fd=os.open(TR/(kind+'probe_events'),os.O_APPEND|os.O_WRONLY)
    try:os.write(fd,spec.encode())
    finally:os.close(fd)

def restore():
    if INSTANCE.exists():
        (INSTANCE/'events/enable').write_text('0')
        INSTANCE.rmdir()
    for kind,name in [('u','expo'),('k','sof')]:
        if (TR/'events'/NAME/name).exists():
            event(kind,'-:'+NAME+'/'+name)
    if (RUN/'media-paused').exists():
        system('start','qmmf-server','ins_mediad','pulseaudio')
    for name in ['hal','selector','haltest','clockmap','client-ready','capture-go','clock-ready']:
        (RUN/name).unlink(missing_ok=True)

def child_stop(p):
    if p is not None and p.poll() is None:
        p.terminate()
        try:p.wait(3)
        except sp.TimeoutExpired:p.kill();p.wait()

def run():
    from capture_window import RESERVE
    state,dev=selected_external(CARD)
    mode=json.loads(sp.check_output(['ins_bll_cli','{"cmd":"config","action":"get_general","key":"kMediaMode"}']))['data']
    assert mode=={'lens_mode':'kPanorama','shoot_mode':'kNormalVideo'}
    ae=sp.check_output(['ins_ibus_cli','--run','call-method','kCamera','kMediad','Image','ImageGetAEParam'],text=True,timeout=4)
    def field(name):
        return re.search(r'^\s*'+name+r':\s*(-?\d+|kAuto|kManual)',ae,re.M)[1]
    shutter=int(field('shutter'));iso=int(field('iso'))
    if not shutter or iso<=0:raise RuntimeError('Exposure reading unavailable')
    ms=max(1,min(33,round(shutter if shutter>0 else 1e6/abs(shutter))))
    shutter_mode,iso_mode=field('shutter_auto_switch'),field('iso_auto_switch')
    auto=shutter_mode=='kAuto' or iso_mode=='kAuto'
    settings=dict(shutter_readback=shutter,shutter_mode=shutter_mode,iso=iso,
                  iso_mode=iso_mode,requested_exposure_ms=ms,
                  policy='continuous RAW metering for Auto controls' if auto else 'manual')
    ae_control=None
    if auto:
        ae_control=RawAutoExposure(EYE,ms,iso,shutter_mode=='kAuto',iso_mode=='kAuto')
    else:
        (RUN/'commands').write_text(start_commands(EYE,ms,iso)+'W:86401\nD\nQ\n')
    folder=CARD/time.strftime('RAWVIDEO-%Y%m%d-%H%M%S')
    assert selected_external(CARD)==(state,dev)
    folder.mkdir();(RUN/'destination').write_text(str(folder))
    (RUN/'card-data').mkdir();(RUN/'card-data/bench.log').symlink_to(folder/'.writer.log')
    client=clock=None
    def interrupted(*_):raise InterruptedError('Capture interrupted')
    signal.signal(signal.SIGTERM,interrupted)
    try:
        with recording(folder,30,audio=True):
            clip=json.loads((folder/'clip.json').read_text())
            clip.update(exposure_clock='device counter, nanoseconds', motion_clock='device counter, ticks',
                        cfa_pattern=[1,2,0,1], black_level=64, white_level=1023, layout='single-bayer-v1',size=[7680,7680],eye=f'CAM{EYE}',lens=['Back','Front'][EYE],
                        sensor_bits=10,codec='X6S8B02',compression='lossless',frames_file='frames.csv',
                        exposure_settings=settings)
            put(folder/'clip.json',clip)
            if (RUN/'stop').exists():raise RuntimeError('Stopped before first frame')
            pulse=sp.check_output(['pidof','pulseaudio']).strip()
            (RUN/'media-paused').touch()
            system('stop','--job-mode=ignore-dependencies','ins_mediad','qmmf-server')
            assert pulse==sp.check_output(['pidof','pulseaudio']).strip(), 'microphone service restarted'
            env=dict(os.environ,CARD_RUN=str(folder),CARD_DEVICE=dev)
            if auto: env["X5_RAW_AUTO_EXPOSURE"]="1"
            client=sp.Popen([sys.executable,str(RUN/'client.py')],env=env,stdout=sp.DEVNULL,stderr=sp.DEVNULL)
            for _ in range(200):
                if (RUN/'client-ready').exists():break
                if client.poll() is not None:raise RuntimeError('RAW client startup failed')
                time.sleep(.05)
            else:raise RuntimeError('RAW client timed out')
            INSTANCE.mkdir()
            (INSTANCE/'buffer_percent').write_text('0')
            event('k',f'p:{NAME}/sof camera:__cam_req_mgr_process_req frame=+8($arg2):s64 trigger=+16($arg2):u32 sof=+24($arg2):u64')
            event('u',f'p:{NAME}/expo {RUN}/hal:0x108af7c node=+72(%x19):string req=+8(%x20):u64 clk=+24(%x20):u64 exp=%x0:u64 dur=%x22:u64')
            for n in ['sof','expo']:(INSTANCE/'events'/NAME/n/'enable').write_text('1')
            clock=sp.Popen([sys.executable,str(RUN/'clockfeed.py')])
            for _ in range(100):
                if (RUN/'clock-ready').exists():break
                if clock.poll() is not None:raise RuntimeError('Exposure clock startup failed')
                time.sleep(.03)
            else:raise RuntimeError('No exposure clock')
            (RUN/'capture-go').touch()
            limit=time.monotonic()+17;last_bytes=0;stopping=None
            while client.poll() is None:
                now=time.monotonic()
                if clock.poll() is not None:
                    save_stop_reason(RUN, 'exposure-clock-stopped')
                    raise RuntimeError('Exposure clock stopped')
                if stopping is None:
                    if selected_external(CARD)!=(state,dev):
                        save_stop_reason(RUN, 'storage-changed')
                        raise RuntimeError('External SSD changed')
                    fs=os.statvfs(folder)
                    if fs.f_bavail*fs.f_frsize<RESERVE or (RUN/'stop').exists():
                        save_stop_reason(RUN, 'card-space' if fs.f_bavail*fs.f_frsize<RESERVE else 'shutter')
                        (RUN/'stop').touch();stopping=now
                    path=folder/'capture.x6b'
                    size=path.stat().st_size if path.is_file() else 0
                    if size>last_bytes:
                        last_bytes=size;limit=now+5
                        if not (RUN/'capturing').exists():(RUN/'capturing').touch()
                    if now>limit:
                        save_stop_reason(RUN, 'frame-timeout')
                        raise RuntimeError('Timed out waiting for RAW frames')
                elif now-stopping>30:
                    save_stop_reason(RUN, 'stop-timeout')
                    raise RuntimeError('Recording did not finish')
                if ae_control: ae_control.tick(stopping is not None)
                time.sleep(.09 if auto else .15)
            if client.returncode:
                save_stop_reason(RUN, 'writer-error')
                raise RuntimeError('RAW writer failed')
            finish_optional(folder, 'clip_clock_end', lambda: clip.update(clock_end=anchor()))
            put(folder/'clip.json',clip)
        assert selected_external(CARD)==(state,dev)
        finalize_single(folder, RUN, EYE)
    finally:
        if ae_control: ae_control.close()
        child_stop(client);child_stop(clock);restore()

if __name__=='__main__':
    if sys.argv[1:] == ['restore']:restore()
    else:
        try:run()
        except Exception as exc:
            (RUN/'failure').write_text(str(exc) or 'Single RAW take incomplete')
            raise
