#!/usr/bin/python3
"""Create only absent preferences. Never replace holder identity or saved values."""
import os,stat,struct,tempfile
from pathlib import Path

def prepare(root=Path('/'),uid=0):
 def path(p):return root/p.lstrip('/')
 def directory(p):
  if p==root:return
  directory(p.parent)
  try:p.mkdir(mode=0o700)
  except FileExistsError:pass
  s=p.lstat()
  if not stat.S_ISDIR(s.st_mode) or s.st_uid!=uid or s.st_mode&0o022:raise RuntimeError('unsafe directory: '+str(p))
 def preference(name,default,valid):
  p=path('/data/x6-owner-features/'+name);directory(p.parent)
  try:
   fd=os.open(p,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK)
  except FileNotFoundError:
   fd,tmp=tempfile.mkstemp(prefix='.openx6-default-',dir=p.parent)
   try:
    if os.write(fd,default)!=len(default):raise OSError('short preference write')
    os.fsync(fd);os.close(fd);fd=-1
    # link is a no-replace publication; preserve a concurrent existing choice.
    try:os.link(tmp,p)
    except FileExistsError:pass
   finally:
    if fd>=0:os.close(fd)
    os.unlink(tmp)
   fd=os.open(p.parent,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
   try:os.fsync(fd)
   finally:os.close(fd)
   return preference(name,default,valid)
  try:
   s=os.fstat(fd);b=os.read(fd,len(default)+1)
   if not stat.S_ISREG(s.st_mode) or s.st_uid!=uid or s.st_nlink!=1 or stat.S_IMODE(s.st_mode)!=0o600 or not valid(b):raise RuntimeError('invalid preference: '+name)
  finally:os.close(fd)
 byte=lambda b:len(b)==1 and b[0]<=1
 preference('stats-v1/enabled.pref',b'\0',byte)
 preference('preview-v1/disabled',b'\0',byte)
 preference('custom-bitrate-v1/value',struct.pack('<I',260),lambda b:len(b)==4 and 1<=int.from_bytes(b,'little')<=340)
 # Owner default (2026-11-03): 120 MP DNGs are written uncompressed (lossless, ~241 MB, DNG write
 # ~0.2 s instead of ~3.8 s, 16K interval ~5 s/frame). ins_mediad reads this flag at every 120 MP DNG
 # write (x6_fast_dng); a saved 0 keeps compressed DNGs and is never replaced.
 preference('interval-v1/fast-dng',b'\1',byte)
 for d in ['stats-overlay','preview-control','interval-controls','ssd','nativefish']:directory(path('/run/x6-'+d))
 for d in ['interval-controls-v1','ssd-v1']:directory(path('/data/x6-owner-features/'+d))

if __name__=='__main__':
 if os.geteuid()!=0:raise SystemExit('root required')
 prepare()
