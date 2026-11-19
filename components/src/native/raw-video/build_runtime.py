"""Build the RAW capture and extraction programs for the target architecture."""
import hashlib, io, json, subprocess, tarfile, tempfile
from pathlib import Path

def build(src, zig, strip, original):
    def sha(b): return hashlib.sha256(b).hexdigest()
    plan=json.loads((src/'tap.json').read_bytes())
    assert sha(original)==plan['base']
    tap=bytearray(original)
    for off, old, new in plan['edits']:
        old=bytes.fromhex(old)
        assert tap[off:off+len(old)]==old
        tap[off:off+len(old)]=bytes.fromhex(new)
    assert sha(tap)==plan['result']
    buf=io.BytesIO()
    with tempfile.TemporaryDirectory() as tmp,tarfile.open(fileobj=buf,mode='w') as archive:
        def add(name,data):
            member=tarfile.TarInfo(name);member.mode=0o700;member.size=len(data)
            archive.addfile(member,io.BytesIO(data))
        def binary(name, flags):
            exe=Path(tmp)/'compiled'
            subprocess.run([zig,'cc','-target','aarch64-linux-gnu.2.35','-O3','-g0',*flags,'-o',exe],check=True)
            subprocess.run([strip,'--strip-all',exe],check=True)
            return exe.read_bytes()
        for file in sorted((src/'runtime').rglob('*')):
            if file.is_file():add(str(file.relative_to(src/'runtime')),file.read_bytes())
        clock=binary('clock',['-fPIC','-shared',src/'anchor.c'])
        single = binary('single8k', ['-mcpu=cortex_a76','-fPIC','-shared','-I'+str(src/'30'),
                                        src/'single8k/compress.c','-lpthread','-ldl','-lm'])
        for profile in (31,32):
            add(f'{profile}/sink.so',single)
            add(f'{profile}/writer.so',clock)
        for rate in (30,50,60):
            inc=src/str(rate)
            card=binary('card',['-mcpu=baseline+dotprod+lse','-DFAST=1','-I'+str(inc),inc/'capture.c','-pthread'])
            add(f'{rate}/card',card)
            dec=inc/'decoder'
            flags=json.loads((dec/'flags.json').read_text())
            add(f'{rate}/extract{rate}',binary('extract',['-I'+str(dec),*flags,src/'extract.c']))
            add(f'{rate}/writer.so',clock)
            add(f'{rate}/rt-tap.so',tap)
    return buf.getvalue()
