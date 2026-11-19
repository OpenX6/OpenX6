"""Record the built-in microphone alongside one RAW take, using an independent process."""
from contextlib import contextmanager
import ctypes as c
import json
import re
import os
from pathlib import Path
import select
import struct
import subprocess as sp
import sys
import time

from deferred_meta import anchor, put

RATE = 48000
CHANNELS = 2
BYTES = 3
STRIDE = CHANNELS * BYTES
BLOCK = 960
AAC_DELAY = 2048  # Measured with a per-channel impulse through the camera's FDK LC pipeline.


def capture_route():
    query = json.dumps(dict(cmd='config',action='get_general',key='kAudioAlgoMode'))
    response = json.loads(sp.check_output(['ins_bll_cli',query],timeout=3))
    mode = response['data']['algo_mode']
    listing = sp.check_output(['pactl','list','sources'],text=True,timeout=3,env={**os.environ,'LC_ALL':'C'})
    primary = next(section for section in listing.split('Source #') if '\n\tName: source_track0\n' in section)
    master = re.search(r'device.master_device = "([^"]+)"',primary)
    if not master or master[1] == 'source.null':
        raise RuntimeError('Camera microphone route unavailable')
    if master[1]=='ambisonic' and mode!='kAmbisonic':
        raise RuntimeError('Audio route is still switching')
    if mode=='kAmbisonic' and master[1]=='apu-arm-processor':
        raise RuntimeError('360 audio route is still switching')
    return mode,master[1]


def adts_samples(path):
    frames=0
    with path.open('rb') as f:
        size=os.fstat(f.fileno()).st_size
        while f.tell()<size:
            h=f.read(7)
            if (len(h)!=7 or h[0]!=255 or h[1]&0xf6!=0xf0 or
                    h[2]>>2!=19 or ((h[2]&1)<<2 | h[3]>>6)!=4 or h[6]&3):
                raise ValueError('Unexpected AAC frame')
            length=(h[3]&3)<<11 | h[4]<<3 | h[5]>>5
            if length<7 or f.tell()+length-7>size:
                raise ValueError('Incomplete AAC frame')
            f.seek(length-7,1)
            frames+=1
    return frames*1024


@contextmanager
def output(path,spatial):
    if not spatial:
        with path.open('xb') as f:
            f.write(wav_header(0))
            yield f
            size=f.tell()-44
            f.seek(0)
            f.write(wav_header(size))
            f.flush()
            os.fsync(f.fileno())
        return
    caps='audio/x-unaligned-raw,format=S16LE,rate=48000,channels=4,layout=interleaved,channel-mask=(bitmask)0x107'
    args=['gst-launch-1.0','-q','fdsrc','fd=0','!',caps,'!','rawaudioparse',
          'use-sink-caps=true','!','fdkaacenc','bitrate=192000','!',
          'audio/mpeg,mpegversion=4,stream-format=adts','!','filesink','location='+str(path)]
    proc=sp.Popen(args,stdin=sp.PIPE,stdout=sp.DEVNULL)
    try:
        yield proc.stdin
        proc.stdin.close()
        if proc.wait(timeout=4):raise RuntimeError('AAC encoder failed')
        with path.open('rb') as f:os.fsync(f.fileno())
    finally:
        if proc.poll() is None:
            proc.kill()
            proc.wait()
        proc.stdin.close()


class Spec(c.Structure):
    _fields_ = [('format', c.c_int), ('rate', c.c_uint32), ('channels', c.c_uint8)]


class Attributes(c.Structure):
    _fields_ = [(k, c.c_uint32) for k in ('maxlength','tlength','prebuf','minreq','fragsize')]


def wav_header(size):
    return struct.pack('<4sI4s4sIHHIIHH4sI', b'RIFF', size+36, b'WAVE', b'fmt ',16,
                       1, CHANNELS, RATE, RATE*STRIDE, STRIDE, BYTES*8, b'data', size)


def record(folder):
    lib = c.CDLL('libpulse-simple.so.0')
    err = c.c_int()
    lib.pa_simple_new.argtypes = [c.c_char_p,c.c_char_p,c.c_int,c.c_char_p,c.c_char_p,
                                  c.POINTER(Spec),c.c_void_p,c.POINTER(Attributes),c.POINTER(c.c_int)]
    lib.pa_simple_new.restype = c.c_void_p
    lib.pa_simple_read.argtypes = [c.c_void_p,c.c_void_p,c.c_size_t,c.POINTER(c.c_int)]
    lib.pa_simple_get_latency.argtypes = [c.c_void_p,c.POINTER(c.c_int)]
    lib.pa_simple_get_latency.restype = c.c_uint64
    lib.pa_simple_free.argtypes = [c.c_void_p]
    mode,master = capture_route()
    spatial = master=='ambisonic'
    source = 'ambisonic' if spatial else 'source_track0'
    channels = 4 if spatial else CHANNELS
    stride = 8 if spatial else STRIDE
    name = 'audio.aac' if spatial else 'audio.wav'
    spec = Spec(3 if spatial else 9,RATE,channels)  # PA_SAMPLE_S16LE / S24LE
    # Pulse's spatial source exposes FL,FC,FR,RC. Request FL,FR,FC,RC
    # explicitly to match the decoded channel sequence of the native video.
    native_map = b'front-left,front-right,front-center,rear-center' if spatial else b'front-left,front-right'
    mapping = (c.c_uint32*33)()
    pulse = c.CDLL('libpulse.so.0')
    pulse.pa_channel_map_parse.argtypes=[c.c_void_p,c.c_char_p]
    pulse.pa_channel_map_parse.restype=c.c_void_p
    if not pulse.pa_channel_map_parse(mapping,native_map):raise RuntimeError('Invalid microphone channel map')
    attr = Attributes(0xffffffff,0xffffffff,0xffffffff,0xffffffff,BLOCK*stride)
    handle = lib.pa_simple_new(b'unix:/var/run/pulse/native', b'RAW Video', 2, source.encode(),
           b'Camera microphone', c.byref(spec), mapping, c.byref(attr), c.byref(err))
    if not handle:
        raise RuntimeError(f'Microphone open failed ({err.value})')
    samples = 0
    timestamps = []
    block = c.create_string_buffer(BLOCK*stride)
    temp = folder/(name+'.part')
    try:
        with output(temp,spatial) as f:
            while True:
                if lib.pa_simple_read(handle, block, len(block), c.byref(err)) < 0:
                    raise RuntimeError(f'Microphone read failed ({err.value})')
                # At the read cursor the NEXT sample lies source latency behind now.
                # Bracket the API call and keep the original observations for desktop audit.
                if samples % RATE == 0 or not timestamps:
                    before = time.monotonic_ns()
                    latency = lib.pa_simple_get_latency(handle,c.byref(err))
                    a = anchor()
                    if latency == 2**64-1:
                        raise RuntimeError(f'Microphone clock unavailable ({err.value})')
                    counter = a['counter_ticks']*10**9//a['counter_hz']
                    mid_anchor = (a['mono_before_ns']+a['mono_after_ns'])//2
                    cursor = counter + (before-mid_anchor)//2 - latency*1000
                    timestamps.append(dict(sample_index=samples+BLOCK, counter_ns=cursor,
                        source_latency_us=latency, query_span_ns=a['mono_after_ns']-before))
                f.write(block.raw)
                samples += BLOCK
                if samples == BLOCK:
                    print(name,flush=True)
                if select.select([sys.stdin],[],[],0)[0]:
                    break
        delay = AAC_DELAY if spatial else 0
        padding = adts_samples(temp)-delay-samples if spatial else 0
        if not 0<=padding<1024:raise RuntimeError('AAC duration mismatch')
        temp.replace(folder/name)
        first=timestamps[0]
        put(folder/'audio-info.json',dict(version=2,file=name,source=source,route_master=master,camera_audio_mode=mode,
            format='AAC-LC-ADTS' if spatial else 'PCM_S24LE',channels=channels,
            codec_delay_samples=delay,trailing_padding_samples=padding,sample_rate_hz=RATE,sample_frames=samples,
            clock='cntvct_ns',timestamp_method='PulseAudio read cursor minus reported latency',
            first_sample_counter_ns=first['counter_ns']-first['sample_index']*10**9//RATE,
            anchors=timestamps))
    finally:
        lib.pa_simple_free(handle)


class Microphone:
    def __init__(self,folder):
        self.proc=sp.Popen([sys.executable,str(Path(__file__).resolve()),str(folder)],
                           stdin=sp.PIPE,stdout=sp.PIPE)
        try:
            self.filename=self.proc.stdout.readline().decode().strip() if select.select([self.proc.stdout],[],[],6)[0] else None
            if self.filename not in ('audio.aac','audio.wav'):
                raise RuntimeError('Microphone did not deliver audio')
        except BaseException:
            self.proc.kill()
            self.proc.wait()
            raise
        finally:self.proc.stdout.close()

    def finish(self):
        # A broken stdin must not leave the microphone process running forever.
        try:
            self.proc.stdin.close()
        except (BrokenPipeError, OSError):
            pass
        try:result=self.proc.wait(timeout=6)
        except sp.TimeoutExpired:
            self.proc.kill()
            self.proc.wait()
            raise RuntimeError('Microphone did not finish')
        if result:raise RuntimeError('Audio not finalized')


if __name__=='__main__':
    record(Path(sys.argv[1]))
