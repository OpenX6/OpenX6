package main
import("bytes";"fmt";"os";"strconv";"strings";"syscall")
const guiLoader="/lib/ld-linux-aarch64.so.1"
const guiBridge="/usr/lib/libx6-owner-gui-bridge.so"
func validGUIInvocation(b []byte)bool{return bytes.Equal(b,[]byte(guiLoader+"\x00--preload\x00"+guiBridge+"\x00/usr/bin/ins_gui\x00"))}
func mappedExecutable(maps []byte,dev,ino uint64)bool{
 major:=(dev>>8)&0xfff | (dev>>32)&0xfffff000
 minor:=dev&0xff | (dev>>12)&0xffffff00
 for _,l:=range strings.Split(string(maps),"\n"){
  f:=strings.Fields(l);if len(f)<6||!strings.Contains(f[1],"x")||strings.HasSuffix(l," (deleted)"){continue}
  d:=strings.Split(f[3],":");if len(d)!=2{continue};a,e:=strconv.ParseUint(d[0],16,64);if e!=nil{continue};b,e:=strconv.ParseUint(d[1],16,64);if e!=nil{continue};i,e:=strconv.ParseUint(f[4],10,64)
  if e==nil&&a==major&&b==minor&&i==ino{return true}
 };return false
}
func processMatches(pid int,path string,cfg SDPolicy)bool{
 exe:=fmt.Sprintf("/proc/%d/exe",pid);h,e:=fileDigest(exe);if e!=nil{return false}
 if member(h,cfg.Files[path]){return true}
 if path!="/usr/bin/ins_gui"||!member(h,cfg.Files[guiLoader]){return false}
 target,e:=os.Readlink(exe);if e!=nil||target!=guiLoader{return false}
 cmd,e:=os.ReadFile(fmt.Sprintf("/proc/%d/cmdline",pid));if e!=nil||!validGUIInvocation(cmd){return false}
 maps,e:=os.ReadFile(fmt.Sprintf("/proc/%d/maps",pid));if e!=nil{return false}
 for _,p:=range []string{path,guiBridge}{
  hash,e:=fileDigest(p);if e!=nil||!member(hash,cfg.Files[p]){return false}
  st,e:=os.Stat(p);if e!=nil{return false};s,ok:=st.Sys().(*syscall.Stat_t)
  if !ok||!mappedExecutable(maps,uint64(s.Dev),s.Ino){return false}
 }
 return true
}
