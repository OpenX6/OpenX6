"""Per-take diagnostics and independent RAW sealing; no per-frame status writes."""
from contextlib import contextmanager
import csv
import itertools
import json
import os
from pathlib import Path
import sys
import time


def write_json(path, data):
    from deferred_meta import put
    put(path, data)


def read_status(folder):
    path = folder/'recording-status.json'
    if path.exists():
        return json.loads(path.read_text())
    return dict(version=1, phase='preparing', stop_reason='unknown', errors=[])


def update(folder, **fields):
    # Diagnostics cannot turn otherwise valid RAW into a failed recording.
    try:
        data = read_status(folder)
        data.update(fields)
        write_json(folder/'recording-status.json', data)
    except Exception as exc:
        print('RAW status write failed:', exc, file=sys.stderr, flush=True)


def error(folder, component, exc):
    print('RAW', component, type(exc).__name__, str(exc), file=sys.stderr, flush=True)
    try:
        data = read_status(folder)
        data['errors'].append(dict(component=component, type=type(exc).__name__,
                                   message=str(exc), monotonic_ns=time.monotonic_ns()))
        write_json(folder/'recording-status.json', data)
    except Exception as status_error:
        print('RAW error receipt failed:', status_error, file=sys.stderr, flush=True)


def finish_optional(folder, component, callback, *args):
    try:
        callback(*args)
        update(folder, **{component+'_status': 'ok'})
        return True
    except Exception as exc:
        error(folder, component, exc)
        update(folder, **{component+'_status': 'error'})
        return False


def finish_audio(folder, microphone):
    def checked_finish():
        microphone.finish()
        info = json.loads((folder/'audio-info.json').read_text())
        if info.get('file') != microphone.filename or info.get('sample_frames', 0) <= 0:
            raise ValueError('Audio receipt disagrees with microphone output')
        if not (folder/microphone.filename).stat().st_size:
            raise ValueError('Empty audio file')
    ok = finish_optional(folder, 'audio', checked_finish)
    if not ok:
        # Keep original/partial audio for recovery, but do not advertise a broken
        # timing/file reference to a desktop importer. Never invent an audio origin.
        def detach():
            clip = json.loads((folder/'clip.json').read_text())
            for key in ('audio_file', 'audio_timing'):
                clip.pop(key, None)
            clip['audio_status'] = 'error'
            clip['recording_status'] = 'recording-status.json'
            write_json(folder/'clip.json', clip)
        finish_optional(folder, 'audio_reference_cleanup', detach)


def stop_reason(run):
    try:
        return (run/'stop-reason').read_text().strip() or 'unknown'
    except FileNotFoundError:
        return 'unknown'


def save_stop_reason(run, reason):
    (run/'stop-reason').write_text(reason)
    try:
        folder = Path((run/'destination').read_text())
        update(folder, stop_reason=reason, stop_monotonic_ns=time.monotonic_ns())
    except OSError as exc:
        print('RAW stop receipt unavailable:', exc, file=sys.stderr, flush=True)


def copy_log_tail(folder, run):
    try:
        with (run/'capture.log').open('rb') as f:
            f.seek(max(0, os.fstat(f.fileno()).st_size-65536))
            data = f.read(65536)
        from deferred_meta import atomic
        atomic(folder/'capture-tail.log', data)
    except FileNotFoundError:
        pass
    except OSError as exc:
        error(folder, 'capture_log', exc)


@contextmanager
def capture_status(folder, run):
    update(folder, phase='capturing', started_monotonic_ns=time.monotonic_ns())
    failed = False
    try:
        yield
    except BaseException as exc:
        failed = True
        error(folder, 'capture', exc)
        if stop_reason(run) == 'unknown':
            try:
                save_stop_reason(run, 'interrupted' if isinstance(exc, (KeyboardInterrupt, InterruptedError)) else 'capture-error')
            except OSError as io_error:
                error(folder, 'stop_reason', io_error)
        raise
    finally:
        update(folder, phase='failed' if failed else 'finalizing', stop_reason=stop_reason(run),
               capture_end_monotonic_ns=time.monotonic_ns())
        copy_log_tail(folder, run)


def seal_take(folder, run, receipt):
    status = read_status(folder)
    receipt.update(stop_reason=stop_reason(run), recording_status='recording-status.json',
                   metadata_errors=status['errors'], audio_status=status.get('audio_status', 'not-requested'))
    write_json(folder/'seal.json', receipt)
    update(folder, phase='complete', raw_status='sealed', stop_reason=receipt['stop_reason'],
           completed_monotonic_ns=time.monotonic_ns())
    copy_log_tail(folder, run)


def finalize_paired(folder, run, fps):
    try:
        log = run/'card-data/bench.log'
        with log.open() as f:
            flushed = [r for r in csv.reader(f) if r and r[0] == 'flushed']
        if len(flushed) != 1 or int(flushed[0][2]) != (folder/'capture.x6b').stat().st_size:
            raise ValueError('RAW flush receipt does not match file size')
        lo, hi, received, io_error = json.loads((run/'card-data/window').read_text())
        pairs, size = int(flushed[0][1]), int(flushed[0][2])
        if not 0 < pairs <= received <= hi-lo+1 or size <= 0:
            raise ValueError('Invalid RAW frame count')
        from exposure_sidecar import exposure_receipt
        exposure = exposure_receipt(folder, log, lo, hi, pairs, io_error)
        seal_take(folder, run, dict(fps_setting=fps, **exposure, request_window=[lo, hi],
                                   received_pairs=received, pairs=pairs, bytes=size))
    except Exception as exc:
        error(folder, 'raw_finalize', exc)
        update(folder, raw_status='unsealed', phase='failed')
        raise


def finalize_single(folder, run, eye):
    try:
        clip = json.loads((folder/'clip.json').read_text())
        size = (folder/'capture.x6b').stat().st_size
        written, peak, written_bytes = map(int, (run/'writer-receipt').read_text().split())
        if not (size == written_bytes and size > 0 and written > 0 and 0 < peak <= 16):
            raise ValueError('Single RAW writer receipt does not match file')
        first = last = None
        count = 0
        with (folder/'frames.csv').open() as f, (folder/'exposure.csv').open() as e:
            for a, b in itertools.zip_longest(csv.DictReader(f), csv.DictReader(e)):
                if a is None or b is None:
                    raise ValueError('Single RAW exposure/frame count mismatch')
                req, hw, exposure, duration = map(int, a.values())
                if not (int(b['request']) == req and int(b[f'cam{eye}_exposure_ns']) == duration
                        and not b[f'cam{1-eye}_exposure_ns'] and exposure > 0 and duration > 0
                        and (first is None or hw > last)):
                    raise ValueError('Single RAW exposure/frame mismatch')
                if first is None:
                    first = hw
                last = hw
                count += 1
        if count != written:
            raise ValueError('Single RAW count differs from writer receipt')
        seal_take(folder, run, dict(fps_setting=30, layout=clip['layout'], eye=clip['eye'],
                  hardware_window=[first, last], frames=count, queue_peak=peak,
                  gaps=last-first+1-count, bytes=size, exposure_status='ok'))
        # Keep the writer log when optional metadata failed, to permit diagnosis.
        if not read_status(folder)['errors']:
            (folder/'.writer.log').unlink(missing_ok=True)
    except Exception as exc:
        error(folder, 'raw_finalize', exc)
        update(folder, raw_status='unsealed', phase='failed')
        raise
