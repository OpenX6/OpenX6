"""Collect per-take geometry and IMU separately from the Bayer compression workers."""
import ctypes, json, os
from pathlib import Path
from contextlib import contextmanager, ExitStack
from motion_stream import MotionStream
from take_status import capture_status, finish_optional, finish_audio

def atomic(path, data):
    temp=path.with_suffix(path.suffix+'.tmp')
    with temp.open('wb') as f:
        f.write(data);f.flush();os.fsync(f.fileno())
    temp.replace(path)
    fd=os.open(path.parent,os.O_DIRECTORY)
    try:os.fsync(fd)
    finally:os.close(fd)

def put(path, data):
    atomic(path,json.dumps(data,separators=(',',':')).encode())

def anchor():
    p=(ctypes.c_uint64*5)()
    ctypes.CDLL(str(Path(__file__).resolve().parent/'writer.so')).anchor(p)
    return dict(zip(('mono_before_ns','counter_ticks','counter_hz','boottime_ns','mono_after_ns'),p))

AXES = dict(frame='Android device natural orientation, screen facing viewer',
            x_positive='screen right', y_positive='screen top', z_positive='out of screen toward viewer',
            device_to_optical_axis_transform=None)
UNITS = {'gyro':'rad/s', 'accel':'m/s^2'}

def finish_stream(stream, path, sensor, start, end):
    result = stream.finish()
    path.replace(path.with_name(sensor + '.csv'))
    if result['samples'] > 1 and result['last_counter_ticks'] > result['first_counter_ticks'] and start:
        result['mean_sample_hz'] = ((result['samples'] - 1) * start['counter_hz']
                                    / (result['last_counter_ticks'] - result['first_counter_ticks']))
    put(path.with_name(sensor + '-info.json'),
        dict(unit=UNITS[sensor], axes=AXES, clock='device_counter',
             clock_start=start, clock_end=end, **result))

def report_accel_error(folder, exc):
    import sys
    print('Accelerometer unavailable:', exc, file=sys.stderr, flush=True)
    try:
        put(folder/'accel-info.json', dict(status='unavailable', samples=0, unit=UNITS['accel'],
                                          axes=AXES, error=str(exc)))
    except OSError as info_error:
        print('Cannot save accelerometer status:', info_error, file=sys.stderr, flush=True)


def finish_accel(stream, path, start, end):
    try:
        finish_stream(stream, path, 'accel', start, end)
    except (OSError, RuntimeError) as exc:
        report_accel_error(path.parent, exc)
        raise


@contextmanager
def recording(folder, fps, *, audio=False):
    eyes = json.loads(Path(__file__).resolve().with_name('raw-sensor-modes.json').read_text())[str(fps)]
    factory = json.loads(Path('/ins/setting/factory/system_config/kFactory/Global.json').read_text())['kFactory']
    put(folder/'calibration.json', {k:factory[k] for k in ('eis_calib','offset_calib')})
    start, end = {}, {}
    with capture_status(folder, Path.cwd()), ExitStack() as cleanup:
        # Start acceleration alongside gyro warm-up; an unavailable auxiliary channel
        # must not hold up the shutter or interrupt the existing Bayer/audio path.
        for attempt in range(2):
            path = folder / f'gyro-{attempt}.part'
            gyro = MotionStream(path, 'gyro')
            if gyro.ready.wait(5):
                cleanup.callback(finish_optional, folder, 'gyro', finish_stream, gyro, path, 'gyro', start, end)
                break
            gyro.finish()
            path.unlink()
        else:
            raise RuntimeError('Gyroscope did not deliver a sample before capture')
        accel_path = folder/'accel.part'
        from accel_retry import AccelCapture
        try:
            accel = AccelCapture(accel_path)
        except (OSError, RuntimeError) as exc:
            report_accel_error(folder, exc)
        else:
            cleanup.callback(finish_optional, folder, 'accel', finish_accel, accel, accel_path, start, end)
        audio_fields = {}
        if audio:
            from raw_audio import Microphone
            microphone = Microphone(folder)
            cleanup.callback(finish_audio, folder, microphone)
            audio_fields = dict(audio_file=microphone.filename, audio_timing='audio-info.json')
        start.update(anchor())
        put(folder/'sensor-metadata.json', dict(schema='raw-sensor-metadata-v1', fps_setting=fps, eyes=eyes,
            timing_source='active sensor mode descriptor: nominal row time = 1e9 / (frame length lines * descriptor fps)',
            skew_definition='time between first and last output row; (output height - 1) * nominal row time',
            window_convention='analog ROI in inclusive sensor register coordinates; digital crop in post-binning output pixel units',
            motion_axes=AXES, timestamp_column='counter_ticks', counter_hz=start['counter_hz'],
            streams={s:dict(info=s+'-info.json', csv=s+'.csv', unit=unit, requested_hz=200) for s,unit in UNITS.items()}))
        put(folder/'clip.json', dict(exposure_file='exposure.csv', exposure_source='CamX PropertyIDSensorActualExposureTime; empty when unavailable', layout='paired-bayer-v1', fps_setting=fps, clock_start=start, size=[3840,3840],
            sensor_metadata='sensor-metadata.json', color_calibration='Development DNG reference matrix and neutral', **audio_fields))
        try:
            yield
        finally:
            finish_optional(folder, 'clock_end', lambda: end.update(anchor()))
