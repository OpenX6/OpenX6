from pathlib import Path
import os,sys,struct,subprocess as sp,hashlib,ctypes
P=Path.cwd()
def check(ok):
 if not ok:raise OSError(ctypes.get_errno(),os.strerror(ctypes.get_errno()))
c=ctypes.CDLL(None,use_errno=True)
check(c.unshare(0x20000)==0)
check(c.mount(None,b'/',None,ctypes.c_ulong(0x40000|0x4000),None)==0)
def bind(src,dst):
 check(c.mount(os.fsencode(src),os.fsencode(dst),None,ctypes.c_ulong(4096),None)==0)
def scoped(dst,base,changes,oldsha):
 src=P/base
 data=bytearray(Path(dst).read_bytes());assert hashlib.sha256(data).hexdigest() in (oldsha if isinstance(oldsha,tuple) else (oldsha,))
 for off,old,new in changes:
  assert data[off:off+len(old)]==old,(off,data[off:off+len(old)].hex())
  data[off:off+len(old)]=new
 src.write_bytes(data)
 os.setxattr(src,'security.selinux',os.getxattr(dst,'security.selinux'))
 src.chmod(0o700)
 bind(src,dst)
# Preserve all matching criteria except the Raw target's maximum height.
scoped('/usr/lib/com.qti.chiusecaseselector.so','selector',[(0xb0930,struct.pack('<I',6144),struct.pack('<I',7680))],'c10310dd6cb50ed0ef84794171b99f5ff8f172b8e193a556c83f7814b356c1e2')
def word(x):return struct.pack('<I',x)
# Only reached by this isolated single-stream RAW10 validation run.
hal='/usr/lib/hw/camera.qcom.so'
scoped(hal,'hal',[(0x7cdc10,word(0x360048a0),word(0xd503201f))],('25e3605f33ad201b82a06ea6ee85f6b6f1fa6a3095d144bbef35a8036a9adca3','8386b304b83aab877c469703bae50fd8150ac26a9cf5d730d4c90725bb673a97','32d68d317c49729d58d89ae7d740f10707e524abbd7330c5b6ac06f35f246f37'))
dst='/usr/bin/camx-hal3-test'
scoped(dst,'haltest',[(0x33908,word(0x37f80300),word(0xd503201f)),(0x33914,word(0x540002a0),word(0xd503201f))],'36b81defe5f922431e8e7790e4e3663df1b0565b3ee2d142aa87048dd0d885a3')
(P/'scratch').mkdir(exist_ok=True);bind(P/'scratch','/data/misc/camera')
os.environ.update(LD_PRELOAD=str(P/'sink.so'),LD_LIBRARY_PATH='/usr/lib/camera/components:/usr/lib/camera',ADSP_LIBRARY_PATH='/usr/lib;/system/lib/rfsa/adsp;/system/vendor/lib/rfsa/adsp;/dsp;/usr/share/dsp-libs')
os.environ['DSP_LIBRARY_PATH']=os.environ['ADSP_LIBRARY_PATH']
# Register client inode probes before executing the test binary.
stage=P/'client-ready';stage.touch()
import time
for _ in range(500):
 if (P/'capture-go').exists():break
 time.sleep(.05)
else:sys.exit(6)
os.execv(dst,[dst,'-f',str(P/'commands')])
