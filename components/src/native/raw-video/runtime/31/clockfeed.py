from pathlib import Path
import os,re,struct
P=Path.cwd()
inst=Path('/sys/kernel/tracing/instances/openx6_single_raw')
with (P/'clockmap').open('w+b') as f:
 f.truncate(16384*32)
 with (inst/'trace_pipe').open() as pipe:
  (P/'clock-ready').touch()
  hardware={}; pending={}
  for line in pipe:
   if len(hardware)>128:hardware.pop(next(iter(hardware)))
   if len(pending)>128:pending.pop(next(iter(pending)))
   if ': sof:' in line:
    hit=re.search(r'frame=(\d+) trigger=1 sof=(\d+)',line)
    if not hit:continue
    hw,clock=map(int,hit.groups());hardware[clock]=hw
   elif ': expo:' in line:
    hit=re.search(r'req=(\d+) clk=(\d+) exp=(\d+).* dur=(\d+)',line)
    if not hit:continue
    req,clock,exp,dur=map(int,hit.groups());pending[clock]=(req,exp,dur)
   else:continue
   if clock in hardware and clock in pending:
    req,exp,dur=pending.pop(clock);hw=hardware.pop(clock)
    if req>0:
     os.pwrite(f.fileno(),struct.pack('<QQQ',hw,exp,dur),32*(req%16384)+8)
     os.pwrite(f.fileno(),struct.pack('<Q',req),32*(req%16384))
