"""Run in a fresh mount namespace on the confirmed camera. Only tmpfs is writable."""
import runpy
import ctypes as c, hashlib, http.client, os, shutil, signal, subprocess, sys, time
from pathlib import Path

root=Path.cwd()
host,port,stream_port,token,mode,version=sys.argv[1:]
port=int(port)
lib=c.CDLL(None,use_errno=True)
def ck(value):
    if value<0:raise OSError(c.get_errno(),os.strerror(c.get_errno()))
def mount(src,dst,fs,flags,opts):
    def ptr(v):return None if v is None else os.fsencode(v)
    ck(lib.mount(ptr(src),ptr(dst),ptr(fs),c.c_ulong(flags),ptr(opts)))
def bind(src,dst,device=False):
    dst.parent.mkdir(parents=True,exist_ok=True)
    dst.touch()
    mount(src,dst,None,4096,None)
    mount(None,dst,None,4096|32|2|(0 if device else 1),None)
def note(*fields):
    print(*fields,flush=True)
def agreed():
    check = runpy.run_path(str(root/'profile.py'))['check_installed']
    check(version, '/ins/etc/version.txt', '/usr/bin/ins_otad')
    assert all(l.startswith('/dev/zram') for l in Path('/proc/swaps').read_text().splitlines()[1:])
    mounts=Path('/proc/mounts').read_text().splitlines()
    assert any(l.split()[1:3]==['/run','tmpfs'] for l in mounts)
    available = int(next(l.split()[1] for l in Path('/proc/meminfo').read_text().splitlines() if l.startswith('MemAvailable:')))
    if available < 2300 * 1024:
        raise RuntimeError('Insufficient memory for isolated export')
    assert int(os.environ.get('NO_INSTALL','0'))==1
def on_parent_exit():
    ck(lib.prctl(1,signal.SIGKILL,0,0,0))
    if os.getppid()==1:os._exit(112)
def jailed(case):
    r,w=os.pipe()
    pid=os.fork()
    if pid==0:
        try:
            os.dup2(w,3,inheritable=True)
            on_parent_exit()
            ck(lib.prctl(38,1,0,0,0))
            class Limit(c.Structure):_fields_=[('cur',c.c_ulong),('max',c.c_ulong)]
            ck(lib.setrlimit(4,c.byref(Limit(0,0))))
            os.dup2(w,3,inheritable=True)
            with open('/dev/null','r+b',buffering=0) as null:
                for fd in (0,1,2):os.dup2(null.fileno(),fd)
            os.closerange(4,65536)
            os.chroot(str(jail));os.chdir('/')
            os.setgroups([]);os.setgid(1000);os.setuid(1000)
            # Verify the preload is readable as the worker before starting the vendor executable.
            with open('/caller.so', 'rb') as preload:
                if preload.read(4) != b'\x7fELF':os._exit(111)
            os.execve('/worker',['/worker',case,'/source.bin'],dict(LD_PRELOAD='/caller.so',LD_LIBRARY_PATH='/lib',LC_ALL='C'))
        except BaseException as exc:
            os.write(3,('LAUNCH '+type(exc).__name__+' '+str(exc)+' '+str(exc.__traceback__.tb_lineno)+'\n').encode())
        finally:os._exit(110)
    os.close(w)
    data=b''
    while True:
        b=os.read(r,2048)
        if not b:break
        if len(data)<8192:data+=b
    os.close(r);_,st=os.waitpid(pid,0)
    note(case,'wait',st,'status',data.decode(errors='replace').strip())
    return st,data

spec={'qcs8625_ota.zip':(1733914751,'3805d2d958a05ffa3f2cfb47ceaee1d2ab50d2674ba810b938dd26346e845aa5'),
      'MCUFw-V1.2.0.0.bin':(130048,'9c1f0427e8562b25d64b360c530f1266d10d6cf0f40dcf3ff2dcee33c447991b'),
      'cs_press-V84.11.1.8.nfw':(44160,'c14400a71f1ea7518f9f05ae953d796c1533a5012133618df8fb4974952f5239')}
def filehash(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for b in iter(lambda:f.read(2**20),b''):h.update(b)
    return h.hexdigest()
agreed()
os.umask(0o022)
ck(lib.unshare(0x00020000))
mount(None,'/',None,(1<<18)|16384,None)
jail=root/'jail'
jail.mkdir()
mount('tmpfs',jail,'tmpfs',2,'size=1800m,mode=0755')
fuse=None
try:
    for d in ('in','out'): (jail/d).mkdir()
    mount('tmpfs',jail/'out','tmpfs',2|4|8,'size=1750m,mode=0700,uid=1000,gid=1000')
    for n in """ld-linux-aarch64.so.1 libQseeComApi.so.1 libbase.so.0 libc.so.6 libcutils.so.0 libdmabufheap.so.0 liberpc_c.so libgcc_s.so.1 libins_base.so libins_config_definitions.so libins_error_code_string.so libins_ibus.so libins_json_helper.so libins_plugin_ibus_for_transport.so libins_share_state.so libins_trace.so libinsqsee.so.0 libion.so.0 libkcp.so liblog.so.0 libm.so.6 libminkdescriptor.so.1 libpthread.so.0 libqcbor.so.0 librt.so.1 libstdc++.so.6 libunwind.so.8 libutils.so.0""".split():
        src=next(p for d in ('/lib','/usr/lib') if (p:=Path(d)/n).is_file())
        bind(src.resolve(),jail/'lib'/n)
    bind('/usr/bin/ins_otad',jail/'worker');bind(root/'caller.so',jail/'caller.so')
    for p in Path('/firmware/image').glob('ins_ta64.*'):bind(p,jail/'firmware/image'/p.name)
    for n in ('smcinvoke','dma_heap/qcom,qseecom','dma_heap/qcom,qseecom-ta','dma_heap/system','dma_heap/qcom,system','urandom','null'):
        bind('/dev/'+n,jail/'dev'/n,True)
    (jail/'source.bin').symlink_to('/in/official.bin')
    fuse=subprocess.Popen([str(root/'fusein'),str(jail/'in'),host,stream_port,token],preexec_fn=on_parent_exit)
    for i in range(80):
        if (jail/'in/official.bin').is_file():break
        if fuse.poll() is not None:raise RuntimeError('input reader exit '+str(fuse.returncode))
        time.sleep(.1)
    assert (jail/'in/official.bin').stat().st_size==1734100608
    assert jailed('guard')[0]==0
    assert jailed('public')[0]==0
    if mode=='full':
        st,data=jailed('cancel')
        assert st!=0 and b'rc=-219936' in data and not list((jail/'out').iterdir())
        st,data=jailed('official')
        assert st==0 and b'prep=0 rc=0' in data
        assert sorted(p.name for p in (jail/'out').iterdir())==sorted(spec)
        for name,(size,digest) in spec.items():
            p=jail/'out'/name
            assert not p.is_symlink() and p.is_file() and p.stat().st_size==size and filehash(p)==digest
            note('VERIFIED',name,size,digest)
        for name,(size,_) in spec.items():
            con=http.client.HTTPConnection(host,port,timeout=90)
            with (jail/'out'/name).open('rb') as f:
                con.request('PUT','/'+token+'/'+name,body=f,headers={'Content-Length':str(size)})
                reply=con.getresponse();assert reply.status==201
            con.close()
    note('ACCEPTED',mode)
finally:
    if fuse is not None and fuse.poll() is None:
        fuse.kill();fuse.wait()
    ck(lib.umount2(os.fsencode(jail),2));jail.rmdir()
    note('PRIVATE-MOUNTS-RELEASED')
