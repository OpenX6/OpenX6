package main
import("strings";"testing")
func TestGUIInvocation(t *testing.T){
 good:=guiLoader+"\x00--preload\x00"+guiBridge+"\x00/usr/bin/ins_gui\x00"
 for _,x:=range []struct{name,s string;want bool}{
  {"good",good,true},{"other-app",strings.ReplaceAll(good,"/usr/bin/ins_gui","/bin/sh"),false},
  {"other-bridge",strings.ReplaceAll(good,guiBridge,"/tmp/bridge.so"),false},
  {"extra-args",good+"extra\x00",false},{"no-bridge",guiLoader+"\x00/usr/bin/ins_gui\x00",false},
 }{t.Run(x.name,func(t *testing.T){if validGUIInvocation([]byte(x.s))!=x.want{t.Fatal(x)}})}
}
func TestGUIMapping(t *testing.T){
 for _,x:=range []struct{name,line string;want bool}{
  {"good","1000-2000 r-xp 0000 00:17 3365 /run/x6-vr180/gui",true},
  {"other-inode","1000-2000 r-xp 0000 00:17 3366 /run/x6-vr180/gui",false},
  {"other-device","1000-2000 r-xp 0000 00:18 3365 /run/x6-vr180/gui",false},
  {"not-executable","1000-2000 r--p 0000 00:17 3365 /run/x6-vr180/gui",false},
  {"deleted","1000-2000 r-xp 0000 00:17 3365 /run/x6-vr180/gui (deleted)",false},
  {"missing","",false},
 }{t.Run(x.name,func(t *testing.T){if mappedExecutable([]byte(x.line),23,3365)!=x.want{t.Fatal(x)}})}
}
