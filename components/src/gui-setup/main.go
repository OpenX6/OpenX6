// Owner GUI startup only. No network, OTA, slot or partition operation.
// Holder private keys are never accepted. The device TLS seed is generated
// only on the target at first start, preserved thereafter, and never printed.
package main

import (
 "openx6.local/components/holder"
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "syscall"
 "time"
 "context"
)

type fileSpec struct { SHA string; Mode os.FileMode }
type view struct { root string; uid, gid uint32 }
func (v view) path(p string) string { if v.root=="" {return p}; return filepath.Join(v.root,p) }
func (v view) stat(p string) (os.FileInfo,error) {
 s,e:=os.Lstat(v.path(p));if e!=nil{return nil,e};t,ok:=s.Sys().(*syscall.Stat_t)
 if !ok||t.Uid!=v.uid||t.Gid!=v.gid{return nil,fmt.Errorf("owner: %s",p)}
 return s,nil
}
func (v view) dir(p string, create bool) error {
 if create {if e:=os.Mkdir(v.path(p),0700);e!=nil&&!os.IsExist(e){return e}}
 s,e:=v.stat(p);if e!=nil{return e};if !s.IsDir()||s.Mode().Perm()!=0700{return fmt.Errorf("private directory: %s",p)};return nil
}
func (v view) openRegular(p string, mode os.FileMode, size int64) (*os.File,error) {
 s,e:=v.stat(p);if e!=nil{return nil,e}
 if !s.Mode().IsRegular()||s.Mode().Perm()!=mode||(size>=0&&s.Size()!=size){return nil,fmt.Errorf("file metadata: %s",p)}
 fd,e:=syscall.Open(v.path(p),syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0);if e!=nil{return nil,e}
 f:=os.NewFile(uintptr(fd),p);now,e:=f.Stat();if e!=nil{f.Close();return nil,e}
 if !os.SameFile(s,now){f.Close();return nil,fmt.Errorf("file replaced: %s",p)}
 return f,nil
}
func (v view) regular(p string, mode os.FileMode, size int64) ([]byte,error) {
 f,e:=v.openRegular(p,mode,size);if e!=nil{return nil,e};defer f.Close();return io.ReadAll(f)
}
func (v view) verify(p string,spec fileSpec) error {
 f,e:=v.openRegular(p,spec.Mode,-1);if e!=nil{return e};defer f.Close()
 // The real GUI is over 100 MB: stream the hash, never allocate its whole
 // contents in each concurrent startup guard.
 h:=sha256.New();if _,e=io.Copy(h,f);e!=nil{return e}
 if hex.EncodeToString(h.Sum(nil))!=spec.SHA{return fmt.Errorf("digest: %s",p)};return nil
}
func (v view) noGlobalPreload() error {
 if _,e:=os.Lstat(v.path("/etc/ld.so.preload"));e==nil||!os.IsNotExist(e){return errors.New("unexpected global preload")};return nil
}
func (v view) core() error {
 for _,p:=range []string{"/usr/bin/ins_gui","/usr/lib/libins_gui_wrap.so","/lib/ld-linux-aarch64.so.1"} {
  // The actual image has regular files and directories at all these paths.
  // Do not silently follow an overlay-provided loader or library symlink.
  for d:=filepath.Dir(p);d!="/";d=filepath.Dir(d) {
   st,e:=v.stat(d);if e!=nil{return e};if !st.IsDir()||st.Mode().Perm()&0022!=0{return fmt.Errorf("unsafe core ancestor: %s",d)}
  }
  if e:=v.verify(p,expectedFiles[p]);e!=nil{return e}
 }
 if e:=v.noGlobalPreload();e!=nil{return e}
 for _,p:=range []string{"/lib/systemd/system/ins_gui.service","/ins/etc/systemd/system/multi-user.target.wants/ins_gui.service","/lib/systemd/system/ins_gui.service.d/50-owner-gui.conf"}{if e:=v.verify(p,expectedFiles[p]);e!=nil{return e}}
 return v.unitGuard([]string{"ins_gui.service"},[]string{"service.d","ins_.service.d","ins_gui.service.d"})
}
type launchPlan struct { Args []string; Bridge bool; Disabled string }
func (v view) guiPlan() (launchPlan,error) {
 p:=launchPlan{Args:[]string{"/lib/ld-linux-aarch64.so.1","/usr/bin/ins_gui"}}
 if e:=v.core();e!=nil{return p,e}
 // Owner integration failure disables only the optional bridge. The patched
 // GUI and wrapper remain mandatory; no legacy unauthenticated code is used.
 if e:=v.checkFiles();e!=nil{p.Disabled=e.Error();return p,nil}
 if e:=v.dir("/run/x6-owner-gui-bridge",false);e!=nil{p.Disabled=e.Error();return p,nil}
 if _,e:=os.Lstat(v.path("/run/x6-owner-gui-bridge/dispatch.sock"));e==nil||!os.IsNotExist(e){p.Disabled="preexisting bridge socket";return p,nil}
 p.Args=[]string{"/lib/ld-linux-aarch64.so.1","--preload","/usr/lib/libx6-owner-gui-bridge.so","/usr/bin/ins_gui"};p.Bridge=true;return p,nil
}
func (v view) check() error {
 if key,err:=holder.Read(v.path("/"),holder.GUI);err!=nil || len(key)!=32{return errors.New("local GUI client key rejected")}
 return v.checkFiles()
}
// The vendor GUI domain only checks slot files. The authentication broker
// checks the local client key before it can accept TLS or signed commands.
func (v view) checkFiles() error {
 if e:=v.core();e!=nil{return e}
 for p,s:=range expectedFiles {if e:=v.verify(p,s);e!=nil{return e}}
 return v.unitGuard([]string{"ins_gui.service","x6-owner-gui-auth.service","x6-owner-gui-late-start.service"},[]string{"service.d","ins_.service.d","ins_gui.service.d","x6-.service.d","x6-owner-.service.d","x6-owner-gui-.service.d","x6-owner-gui-auth.service.d","x6-owner-gui-late-start.service.d"})
}
func (v view) unitGuard(units,dirs []string) error {
 bases:=[]string{"/etc/systemd/system.control","/run/systemd/system.control","/run/systemd/transient","/run/systemd/generator.early","/etc/systemd/system","/etc/systemd/system.attached","/run/systemd/system","/run/systemd/system.attached","/run/systemd/generator","/usr/local/lib/systemd/system","/usr/lib/systemd/system","/lib/systemd/system","/run/systemd/generator.late"}
 for _,base:=range bases {
  for _,unit:=range units {
   p:=base+"/"+unit;s,e:=v.stat(p);if os.IsNotExist(e){continue};if e!=nil{return e}
   // Defensive compatibility for a known stock unit represented as a link;
   // no claim that this image requires such a link or an ins.mount exists.
   if spec,ok:=expectedFiles[p];ok {if e:=v.verify(p,spec);e!=nil{return e}
   } else if unit=="ins_gui.service" {
    if s.Mode()&os.ModeSymlink!=0 {
     link,e:=os.Readlink(v.path(p));if e!=nil{return e}
     if !filepath.IsAbs(link){link=filepath.Clean(filepath.Join(base,link))}
     if link!="/ins/etc/systemd/system/multi-user.target.wants/ins_gui.service"&&link!="/lib/systemd/system/ins_gui.service" {return fmt.Errorf("unit link override: %s",p)}
     if e:=v.verify(link,stockGUIUnit);e!=nil{return e}
    } else if e:=v.verify(p,stockGUIUnit);e!=nil{return e}
   } else {return fmt.Errorf("unit override: %s",p)}
  }
  for _,dir:=range dirs {
   found,e:=filepath.Glob(v.path(base+"/"+dir+"/*.conf"));if e!=nil{return e}
   for _,f:=range found {rel:=strings.TrimPrefix(f,v.root);spec,ok:=expectedFiles[rel];if !ok{return fmt.Errorf("unit drop-in override: %s/%s",base,dir)};if e:=v.verify(rel,spec);e!=nil{return e}}
  }
 }
 return nil
}
// Atomically publish a complete new file with a no-overwrite link. A restart
// never replaces a published identity. Temporary leftovers are not used.
func (v view) create(p string,b []byte,mode os.FileMode) error {
 parent:=filepath.Dir(v.path(p));f,e:=os.CreateTemp(parent,".identity-tmp-");if e!=nil{return e};tmp:=f.Name();defer os.Remove(tmp)
 if e=f.Chmod(mode);e!=nil{f.Close();return e};if _,e=f.Write(b);e!=nil{f.Close();return e}
 if e=f.Sync();e!=nil{f.Close();return e};if e=f.Close();e!=nil{return e}
 if e=os.Link(tmp,v.path(p));e!=nil{return e};d,e:=os.Open(parent);if e!=nil{return e};defer d.Close();return d.Sync()
}
func (v view) identity() error {
 if e:=v.dir("/data/x6-owner-gui",true);e!=nil{return e}
 const private="/data/x6-owner-gui/server_ed25519";const public="/data/x6-owner-gui/server_ed25519.pub"
 seed,e:=v.regular(private,0600,32)
 if os.IsNotExist(e) {
  // An orphan public identity is not silently replaced by a new one.
  if _,pe:=os.Lstat(v.path(public));pe==nil||!os.IsNotExist(pe){return errors.New("orphan public identity")}
  seed=make([]byte,32);if _,e=rand.Read(seed);e!=nil{return e}
  if e=v.create(private,seed,0600);e!=nil{return e}
 } else if e!=nil{return e}
 defer func(){for i:=range seed{seed[i]=0}}()
 key:=ed25519.NewKeyFromSeed(seed);defer func(){for i:=range key{key[i]=0}}()
 want:=key.Public().(ed25519.PublicKey);pub,e:=v.regular(public,0600,32)
 if os.IsNotExist(e){return v.create(public,want,0600)}
 if e!=nil{return e};if string(pub)!=string(want){return errors.New("device public identity mismatch")};return nil
}
func cleanEnv() []string {
 result:=[]string{}
 for _,s:=range os.Environ(){k,_,_:=strings.Cut(s,"=");if strings.HasPrefix(k,"LD_")||k=="GLIBC_TUNABLES"{continue};result=append(result,s)}
 return result
}
func ctl(args ...string) error {
 ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,"/bin/systemctl",args...);cmd.Env=cleanEnv();cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr
 return cmd.Run()
}
func main() {
 if os.Geteuid()!=0||os.Getegid()!=0||len(os.Args)!=2{fmt.Fprintln(os.Stderr,"OWNER_GUI_START_REFUSED: root and one mode required");os.Exit(73)}
 v:=view{};var e error
 switch os.Args[1] {
 case "broker-prepare":
  if e=v.check();e==nil{if e=v.dir("/run/x6-owner-gui",false);e==nil{e=v.identity()}}
 case "gui-exec":
  var p launchPlan;p,e=v.guiPlan()
  if e==nil {if !p.Bridge{fmt.Fprintln(os.Stderr,"OWNER_GUI_DISABLED:",p.Disabled)};e=syscall.Exec(p.Args[0],p.Args,cleanEnv())}

 case "late-start":
  if e=v.check();e==nil{if e=ctl("daemon-reload");e==nil{e=ctl("--no-block","start","x6-owner-gui-auth.service")}}
 default:e=errors.New("unknown mode")
 }
 if e!=nil{fmt.Fprintln(os.Stderr,"OWNER_GUI_START_REFUSED:",e);os.Exit(73)}
 fmt.Println("OWNER_GUI_START_PREPARED",os.Args[1])
}
