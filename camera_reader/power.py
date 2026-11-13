"""Apply the two native idle preferences before a camera transfer."""
import shlex

# Send one preference at a time to fit the stock GUI command limit. Keep the
# complete config object on the camera, including any network credentials.
_SCRIPT = """import json,subprocess,sys
k,f,v=sys.argv[1:]
v=json.loads(v)
def call(action,**kw):
 q=dict(cmd="config",action=action,key=k,**kw)
 r=json.loads(subprocess.check_output(["/usr/bin/ins_bll_cli",json.dumps(q)]))
 assert r["ret"]=="ok"
 return r
def get():
 return call("get_general")["data"]
d=get()
assert type(d[f]) is type(v)
if d[f]!=v:
 d[f]=v
 call("set_general",value=d)
r=get()[f]
assert type(r) is type(v) and r==v
print("OK")
"""
SETTINGS = (
    ('kSystemAutoShutdown', 'auto_power_off_time', '0'),
    ('kNetdConfig', 'wifi_enable_strategy', '"kAlwaysOn"'),
)
NOTICE = ('OpenX6 turns off auto power-off and Wi-Fi auto-off on this camera. '
          'These preferences stay in effect after the transfer.\n'
          'OpenX6 将关闭相机自动关机和 Wi-Fi 自动关闭，传输结束后仍保留这两项设置。')


def prepare(session, execute, progress, cancel=None):
    progress('Checking auto power-off and Wi-Fi auto-off / 正在设置并核对自动关机和 Wi-Fi 自动关闭')
    for key, field, value in SETTINGS:
        if cancel is not None and cancel.is_set():
            raise InterruptedError('Export canceled / 已取消')
        command = shlex.join(['python3', '-E', '-c', _SCRIPT, key, field, value])
        try:
            if execute(session, command).strip() != b'OK':
                raise ValueError('Missing settings readback')
        except (OSError, EOFError, ValueError, RuntimeError) as exc:
            raise RuntimeError(f'Could not verify {field}. Reconnect to the X6 Wi-Fi and retry.') from exc
