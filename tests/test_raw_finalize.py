"""Exercise production RAW finalization with failed optional streams, no camera I/O."""
import ast
import csv
import importlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import types
import unittest
from unittest.mock import Mock, patch

ROOT = Path(os.environ['RAW_RUNTIME_ROOT']) if 'RAW_RUNTIME_ROOT' in os.environ else Path(__file__).resolve().parents[1]/'components/src/native/raw-video/runtime'
PROFILE = os.environ.get('RAW_TEST_PROFILE', '30')
sys.dont_write_bytecode = True
sys.path.insert(0, str(ROOT/PROFILE))
# Paired finalizer tests also exercise the shared helper under single-profile imports.
# Single profiles do not call this path in production.
sys.path.append(str(ROOT/'30'))
import deferred_meta as meta
import take_status as status
import capture_window as window
import raw_audio

class FinalizeTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.folder = self.root/'RAWVIDEO-20261118-011647';self.folder.mkdir()
        self.run = self.root/'run';self.run.mkdir()
        (self.run/'destination').write_text(str(self.folder))
        (self.run/'capture.log').write_text('synthetic capture log\n')
        self.oldcwd = Path.cwd();os.chdir(self.run)
        self.done = []
        self.audio_failure = None
        self.gyro_failure = None
        self.accel_failure = None
        outer = self
        class Stream:
            def __init__(self, path, kind='accel'):
                self.path=path;self.kind=kind;path.write_text('sample\n')
                self.ready=Mock();self.ready.wait.return_value=True
            def finish(self):
                outer.done.append(self.kind)
                failure=outer.gyro_failure if self.kind=='gyro' else outer.accel_failure
                if failure:raise failure
                return dict(status='ok',samples=2,first_counter_ticks=10,last_counter_ticks=20)
        class Mic:
            filename = 'audio.aac'
            def __init__(self, folder):
                self.folder=folder;(folder/'audio.aac.part').write_bytes(b'original partial audio')
            def finish(self):
                outer.done.append('audio')
                if outer.audio_failure:raise outer.audio_failure
                (self.folder/'audio.aac.part').rename(self.folder/'audio.aac')
                meta.put(self.folder/'audio-info.json',dict(file='audio.aac',sample_frames=1920,first_sample_counter_ns=123))
        read_text = Path.read_text
        def read(path,*a,**k):
            if str(path)=='/ins/setting/factory/system_config/kFactory/Global.json':
                return json.dumps(dict(kFactory=dict(eis_calib='geometry',offset_calib='offset')))
            return read_text(path,*a,**k)
        self.patches = [patch.object(meta,'MotionStream',Stream),
            patch.dict(sys.modules,{'accel_retry':types.SimpleNamespace(AccelCapture=Stream)}),
            patch.object(raw_audio,'Microphone',Mic),patch.object(Path,'read_text',read),
            patch.object(meta,'anchor',return_value=dict(counter_hz=19200000,counter_ticks=123,mono_before_ns=1,mono_after_ns=2))]
        for p in self.patches:p.start()
    def tearDown(self):
        for p in reversed(self.patches):p.stop()
        os.chdir(self.oldcwd);self.tmp.cleanup()
    def paired(self):
        (self.folder/'capture.x6b').write_bytes(b'0123456789abcdef')
        (self.folder/'exposure.part').write_text('10,1000,1001\n11,1002,1003\n12,1004,1005\n')
        data=self.run/'card-data';data.mkdir()
        (data/'bench.log').write_text('disk,10,0,8,0,1\ndisk,12,8,8,1,2\nflushed,2,16,3\n')
        (data/'window').write_text('[10,12,3,null]')
        status.save_stop_reason(self.run,'shutter')
    def seal_pair(self):
        status.finalize_paired(self.folder,self.run,int(PROFILE) if PROFILE in ('30','50','60') else 30)
        return json.loads((self.folder/'seal.json').read_text())
    def test_normal_audio_exposure_and_seal(self):
        with meta.recording(self.folder,30,audio=True):self.paired()
        seal=self.seal_pair();self.assertEqual(seal['audio_status'],'ok');self.assertEqual(seal['stop_reason'],'shutter')
        self.assertEqual(seal['metadata_errors'],[]);self.assertEqual(seal['pairs'],2)
        with (self.folder/'exposure.csv').open() as f:
            self.assertEqual([r['request'] for r in csv.DictReader(f)],['10','12'])
        clip=json.loads((self.folder/'clip.json').read_text());self.assertEqual(clip['audio_timing'],'audio-info.json')
        self.assertEqual(self.done[0],'audio');self.assertCountEqual(self.done,['audio','gyro','accel'])
    def test_audio_timeout_does_not_block_raw_or_imu(self):
        self.audio_failure=RuntimeError('Microphone did not finish')
        with meta.recording(self.folder,30,audio=True):self.paired()
        seal=self.seal_pair();self.assertEqual(seal['audio_status'],'error');self.assertEqual(seal['exposure_status'],'ok')
        self.assertIn('audio',[r['component'] for r in seal['metadata_errors']])
        self.assertEqual(self.done[0],'audio');self.assertCountEqual(self.done,['audio','gyro','accel'])
        clip=json.loads((self.folder/'clip.json').read_text());self.assertNotIn('audio_file',clip);self.assertNotIn('audio_timing',clip)
        self.assertEqual((self.folder/'audio.aac.part').read_bytes(),b'original partial audio')
        self.assertTrue((self.folder/'gyro-info.json').exists());self.assertTrue((self.folder/'accel-info.json').exists())
    def test_audio_value_error_and_multiple_optional_failures(self):
        self.audio_failure=ValueError('AAC duration mismatch');self.gyro_failure=OSError('gyro failed');self.accel_failure=RuntimeError('accel failed')
        with meta.recording(self.folder,30,audio=True):self.paired()
        seal=self.seal_pair();self.assertEqual({e['component'] for e in seal['metadata_errors']},{'audio','gyro','accel'})
        self.assertEqual(self.done[0],'audio');self.assertCountEqual(self.done,['audio','gyro','accel'])
    def test_end_clock_failure_still_seals(self):
        with patch.object(meta,'anchor',side_effect=[dict(counter_hz=19200000),OSError('clock unavailable')]):
            with meta.recording(self.folder,30,audio=True):self.paired()
        seal=self.seal_pair();self.assertIn('clock_end',[e['component'] for e in seal['metadata_errors']])
    def test_capture_failure_keeps_reason_and_no_seal(self):
        with self.assertRaisesRegex(RuntimeError,'writer failed'):
            with meta.recording(self.folder,30,audio=True):
                self.paired();status.save_stop_reason(self.run,'writer-error');raise RuntimeError('writer failed')
        d=json.loads((self.folder/'recording-status.json').read_text())
        self.assertEqual(d['phase'],'failed');self.assertEqual(d['stop_reason'],'writer-error');self.assertFalse((self.folder/'seal.json').exists())
    def test_bad_flush_cannot_be_marked_complete(self):
        with meta.recording(self.folder,30,audio=True):self.paired()
        (self.folder/'capture.x6b').write_bytes(b'truncated')
        with self.assertRaisesRegex(ValueError,'flush receipt'):self.seal_pair()
        self.assertFalse((self.folder/'seal.json').exists())
        self.assertEqual(json.loads((self.folder/'recording-status.json').read_text())['raw_status'],'unsealed')
    def test_duplicate_flush_rejected(self):
        with meta.recording(self.folder,30,audio=True):self.paired()
        p=self.run/'card-data/bench.log';p.write_text(p.read_text()+'flushed,2,16,4\n')
        with self.assertRaises(ValueError):self.seal_pair()
        self.assertFalse((self.folder/'seal.json').exists())
    def test_no_frames_rejected(self):
        with meta.recording(self.folder,30,audio=True):self.paired()
        (self.run/'card-data/bench.log').write_text('flushed,0,16,1\n')
        with self.assertRaises(ValueError):self.seal_pair()
    def test_single_both_eyes_audio_error(self):
        self.audio_failure=RuntimeError('Audio not finalized')
        for eye in (0,1):
            with self.subTest(eye=eye):
                if eye:
                    for p in self.folder.iterdir():p.unlink()
                with meta.recording(self.folder,30,audio=True):
                    clip=json.loads((self.folder/'clip.json').read_text());clip.update(layout='single-bayer-v1',eye=f'CAM{eye}')
                    meta.put(self.folder/'clip.json',clip)
                    (self.folder/'capture.x6b').write_bytes(b'12345678')
                    (self.run/'writer-receipt').write_text('2 3 8')
                    (self.folder/'frames.csv').write_text('request,hardware,exposure,duration\n10,100,123456,1000\n11,101,123457,1001\n')
                    (self.folder/'exposure.csv').write_text('request,cam0_exposure_ns,cam1_exposure_ns\n'+''.join(f'{i},{d},\n' if eye==0 else f'{i},,{d}\n' for i,d in [(10,1000),(11,1001)]))
                    status.save_stop_reason(self.run,'shutter')
                status.finalize_single(self.folder,self.run,eye)
                seal=json.loads((self.folder/'seal.json').read_text());self.assertEqual(seal['frames'],2);self.assertEqual(seal['audio_status'],'error')
    def test_single_mismatched_writer_unsealed(self):
        (self.folder/'clip.json').write_text('{"layout":"single-bayer-v1","eye":"CAM0"}')
        (self.folder/'capture.x6b').write_bytes(b'bad');(self.run/'writer-receipt').write_text('2 3 8')
        with self.assertRaises(ValueError):status.finalize_single(self.folder,self.run,0)
        self.assertFalse((self.folder/'seal.json').exists())
    def test_stop_causes_persist_even_without_frames(self):
        for reason in ['shutter','frame-timeout','card-space']:
            with self.subTest(reason=reason):
                (self.run/'stop').unlink(missing_ok=True)
                if reason=='shutter':(self.run/'stop').touch()
                r,w=os.pipe()
                try:
                    with os.fdopen(r,'rb',buffering=0) as pipe, patch.object(window.time,'monotonic',side_effect=[0,13]), patch.object(window.shutil,'disk_usage',return_value=types.SimpleNamespace(free=0 if reason=='card-space' else 10**12)):
                        with self.assertRaisesRegex(RuntimeError,'Too few'):window.feed(pipe,io.BytesIO(),self.run,self.root,io.StringIO())
                finally:os.close(w)
                self.assertEqual(json.loads((self.folder/'recording-status.json').read_text())['stop_reason'],reason)
    def test_real_trace_two_pairs_and_stop(self):
        trace=[]
        for req in (1,13,14):
            for eye in (0,1):
                trace.append(f'x: primary: MainVideo0_Sensor0_cam{eye}" req={req} clk=0 exp={req*33366667} applied_ptr=0x10 dur=1000\n')
                trace.append(f'x 1.000000: fence: MainVideo0_IFE0_cam{eye}" req={req} req2={req} port=9 status=0 buffer fd={10+eye}\n')
        r,w=os.pipe();os.write(w,''.join(trace).encode());os.close(w)
        sink=io.BytesIO();dur=io.StringIO();outer=self
        class Writer:
            def write(self,data):
                sink.write(data)
                if sink.getvalue().count(b'\n')==2:(outer.run/'stop').touch()
        with os.fdopen(r,'rb',buffering=0) as pipe,patch.object(window.shutil,'disk_usage',return_value=types.SimpleNamespace(free=10**12)):
            first,last,count,period=window.feed(pipe,Writer(),self.run,self.root,dur)
        self.assertEqual((first,last,count),(13,14,2));self.assertEqual(period,33366667)
        self.assertEqual(dur.getvalue(),'13,1000,1000\n14,1000,1000\n')
        self.assertEqual(status.stop_reason(self.run),'shutter')
    def test_diagnostic_write_failure_not_audio_failure(self):
        with patch.object(status,'write_json',side_effect=OSError('read only')):
            self.assertTrue(status.finish_optional(self.folder,'audio',lambda:None))
    def test_production_measure_with_audio_timeout_seals_before_return(self):
        if PROFILE not in ('30','50','60'):
            self.skipTest('paired trial entry point only')
        self.folder.rmdir()  # Production measure creates its take folder.
        self.audio_failure=RuntimeError('Microphone did not finish')
        tree=ast.parse((ROOT/PROFILE/'trial.py').read_text())
        measure=next(n for n in ast.walk(tree) if isinstance(n,ast.FunctionDef) and n.name=='measure')
        instance=self.run/'trace';(instance/'events/test').mkdir(parents=True)
        (instance/'trace_pipe').write_bytes(b'')
        (instance/'events/test/enable').touch()
        outer=self
        class Worker:
            def __init__(self,*a,**kw):
                self.log=kw['stdout'];self.log.write(b'ready\n');self.log.flush();self.stdin=io.BytesIO();self.result=None
            def poll(self):return self.result
            def wait(self,timeout=None):
                self.log.write(b'disk,10,0,8,0,1\ndisk,12,8,8,1,2\nflushed,2,16,3\n');self.result=0;return 0
            def kill(self):self.result=-9
        def feed(pipe,writer,run,card,durations):
            (outer.folder/'capture.x6b').write_bytes(b'0123456789abcdef')
            for req in (10,11,12):durations.write(f'{req},1000,1001\n')
            status.save_stop_reason(run,'frame-timeout')
            return 10,12,3,33366667
        ns=dict(p=self.run,dest=self.folder,root=self.root,card=self.root,device='external',recording_drive=lambda:(self.root,'external'),
                sp=types.SimpleNamespace(Popen=Worker,PIPE=-1,check_output=lambda *a,**k:b'123'),
                pid='123',sys=types.SimpleNamespace(argv=['trial.py','card']),time=__import__('time'),i=instance,g='test',json=json)
        exec(compile(ast.Module(body=[measure],type_ignores=[]),'production-measure','exec'),ns)
        with patch.object(window,'feed',feed):result=ns['measure']('card-data')
        self.assertEqual(result,33366667)
        seal=json.loads((self.folder/'seal.json').read_text())
        self.assertEqual(seal['audio_status'],'error');self.assertEqual(seal['stop_reason'],'frame-timeout')
        self.assertEqual(seal['pairs'],2)

    def test_saved_notice_reports_audio_error_and_keeps_diagnostic_log(self):
        self.audio_failure=RuntimeError('Audio not finalized')
        with meta.recording(self.folder,30,audio=True):self.paired()
        self.seal_pair()
        (self.folder/'.writer.log').write_text('preserve diagnosis')
        tree=ast.parse((ROOT/'controller.py').read_text())
        fn=next(n for n in tree.body if isinstance(n,ast.FunctionDef) and n.name=='saved_notice')
        ns=dict(Path=Path,json=json)
        exec(compile(ast.Module(body=[fn],type_ignores=[]),'production-saved-notice','exec'),ns)
        msg=ns['saved_notice'](self.run,Mock(),30)
        self.assertIn('X6B saved',msg);self.assertIn('audio error',msg)
        self.assertTrue((self.folder/'.writer.log').exists())

    def test_bounded_log_tail(self):
        (self.run/'capture.log').write_bytes(b'x'*100000)
        status.copy_log_tail(self.folder,self.run)
        self.assertEqual((self.folder/'capture-tail.log').stat().st_size,65536)

class IntegrationTests(unittest.TestCase):
    def test_microphone_timeout_kills_and_reaps(self):
        mic = object.__new__(raw_audio.Microphone)
        mic.proc = Mock()
        mic.proc.wait.side_effect = [subprocess.TimeoutExpired('audio', 6), 0]
        with self.assertRaisesRegex(RuntimeError, 'Microphone did not finish'):
            mic.finish()
        mic.proc.kill.assert_called_once()
        self.assertEqual(mic.proc.wait.call_count, 2)
    def test_microphone_broken_pipe_still_reaped(self):
        mic = object.__new__(raw_audio.Microphone)
        mic.proc = Mock()
        mic.proc.stdin.close.side_effect = BrokenPipeError('encoder exited')
        mic.proc.wait.return_value = 1
        with self.assertRaisesRegex(RuntimeError, 'Audio not finalized'):
            mic.finish()
        mic.proc.wait.assert_called_once_with(timeout=6)

    def test_all_profiles_ship_same_status_and_guarded_callbacks(self):
        contents=[]
        for rate in (30,50,60,31,32):
            p=ROOT/str(rate);contents.append((p/'take_status.py').read_bytes())
            tree=ast.parse((p/'deferred_meta.py').read_text())
            calls=[ast.unparse(n) for n in ast.walk(tree) if isinstance(n,ast.Call)]
            self.assertIn('cleanup.callback(finish_audio, folder, microphone)',calls)
            self.assertTrue(any(x.startswith('capture_status(folder,') for x in calls))
            source=(p/('trial.py' if rate in (30,50,60) else 'single.py')).read_text()
            self.assertIn('finalize_paired(dest, p, '+str(rate)+')' if rate in (30,50,60) else 'finalize_single(folder, RUN, EYE)',source)
            self.assertNotIn("info['sample_frames']>0",source)
        self.assertEqual(len(set(contents)),1)
    def test_album_accepts_diagnostics_and_partial_audio_but_rejects_unknown(self):
        sys.path.insert(0,str(ROOT))
        import raw_delete
        with tempfile.TemporaryDirectory() as t:
            root=Path(t).resolve();folder=root/'RAWVIDEO-20261118-011647';folder.mkdir();cache=root/'cache';cache.mkdir()
            for name in ['capture.x6b','clip.json','seal.json','recording-status.json','capture-tail.log','audio.aac.part','audio.wav.part','gyro-0.part']:(folder/name).touch()
            key=raw_delete.item_key('disk',folder,int(folder.stat().st_mtime))
            (cache/key).write_bytes(b'\0'*(800+158*158*4)+os.fsencode(folder/'capture.x6b')+b'\0')
            self.assertEqual(raw_delete.resolve_item(key,cache,[('disk',None,root)]),[(root,folder)])
            (folder/'unrelated-user-file').touch()
            # Refresh selection after directory mtime changes, still reject unknown member.
            key=raw_delete.item_key('disk',folder,int(folder.stat().st_mtime));(cache/key).write_bytes(b'\0'*(800+158*158*4)+os.fsencode(folder/'capture.x6b')+b'\0')
            with self.assertRaisesRegex(ValueError,'Unexpected file'):raw_delete.resolve_item(key,cache,[('disk',None,root)])

if __name__=='__main__':unittest.main()
