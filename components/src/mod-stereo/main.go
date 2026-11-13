// Local fixed-purpose recording policy. No network listener or arbitrary commands.
package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "log"
 "os"
 "os/exec"
 "os/signal"
 "path/filepath"
 
 "syscall"
 "time"
)
const base="/run/x6-stereo-control"
const runtime="/run/x6-stereo-control"
func dir(p string) error {s,e:=os.Lstat(p);if e!=nil{return e};if !s.IsDir()||s.Mode().Perm()&0022!=0||s.Sys().(*syscall.Stat_t).Uid!=uint32(os.Geteuid()){return errors.New("unsafe directory")};return nil}
func read(p string)(byte,error){
 if e:=dir(filepath.Dir(p));e!=nil{return 0,e}
 fd,e:=syscall.Open(p,syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0);if e!=nil{return 0,e};f:=os.NewFile(uintptr(fd),p);defer f.Close();s,e:=f.Stat();if e!=nil{return 0,e};if !s.Mode().IsRegular()||s.Sys().(*syscall.Stat_t).Uid!=uint32(os.Geteuid())||s.Mode().Perm()&0022!=0{return 0,errors.New("unsafe preference")};b,e:=io.ReadAll(io.LimitReader(f,2));if e!=nil{return 0,e};if len(b)!=1||b[0]>1{return 0,errors.New("invalid preference")};return b[0],nil
}
func save(p string,v byte)error{
 if v>1{return errors.New("invalid preference")};if e:=dir(filepath.Dir(p));e!=nil{return e}
 if s,e:=os.Lstat(p);e==nil&&!s.Mode().IsRegular(){return errors.New("unsafe destination")}else if e!=nil&&!os.IsNotExist(e){return e}
 f,e:=os.CreateTemp(filepath.Dir(p),".stereo-*");if e!=nil{return e};defer os.Remove(f.Name());if _,e=f.Write([]byte{v});e==nil{e=f.Sync()};ce:=f.Close();if e==nil{e=ce};if e!=nil{return e};if e=os.Rename(f.Name(),p);e!=nil{return e};d,e:=os.Open(filepath.Dir(p));if e!=nil{return e};defer d.Close();return d.Sync()
}
// runner executes one fixed command under a timeout; host tests substitute it.
type runner func(timeout time.Duration,name string,args ...string)([]byte,error)
func command(timeout time.Duration,name string,args ...string)([]byte,error){c,cancel:=context.WithTimeout(context.Background(),timeout);defer cancel();return exec.CommandContext(c,name,args...).CombinedOutput()}
type media struct {Capture struct{Status string `json:"capture_status"`} `json:"capture"`;Record struct{Status string `json:"record_status"`;Time int64 `json:"record_time_ms"`} `json:"record"`;Pipeline struct{Status struct{States []int `json:"status"`} `json:"pipeline_status"`} `json:"pipeline"`}
func getMedia(run runner)(media,error){var m media;b,e:=run(5*time.Second,"/usr/bin/ins_bll_cli",`{"cmd":"share_state","action":"get","type":"media"}`);if e!=nil{return m,e};var r struct{Ret string `json:"ret"`;Data media `json:"data"`};if e=json.Unmarshal(b,&r);e!=nil{return m,e};if r.Ret!="ok"||r.Data.Record.Status==""{return m,errors.New("invalid media response")};return r.Data,nil}

func apply(run runner,v byte) error {
 if v>1{return errors.New("invalid level")}
 m,e:=getMedia(run);if e!=nil{return e};if m.Record.Status!="RecordIdle"||m.Capture.Status!="CaptureIdle"{return errors.New("recording busy")}
 out,e:=run(90*time.Second,"/bin/sh","/usr/local/sbin/x6-mod-stereo-select",fmt.Sprint(v))
 if e!=nil{return fmt.Errorf("apply: %v %.200s",e,out)};return nil
}
const preference="/data/x6-owner-features/mod-controls-v1/stereo.pref"
func initialize(pref,state string)error{v,e:=read(pref);if e!=nil{return e};return save(state,v)}
// handle consumes one GUI request: apply, persist, mirror, or put the previous level back.
func handle(pref,rt string,run runner)error{
 v,err:=read(rt+"/request")
 var previous byte
 if err==nil{previous,err=read(pref)}
 if err==nil{err=apply(run,v)}
 if err==nil{
  err=save(pref,v)
  if err==nil{err=save(rt+"/state",v)}
  if err!=nil{if e:=apply(run,previous);e!=nil{log.Printf("restore previous failed: %v",e)}else{_ = save(pref,previous);_ = save(rt+"/state",previous)}}
 }
 if err!=nil{log.Printf("REJECT: %v",err)}else{log.Printf("APPLIED level=%d",v)}
 if e:=os.Remove(rt+"/request");e!=nil&&!os.IsNotExist(e){log.Printf("cleanup: %v",e)}
 return err
}
func main(){
 if len(os.Args)==2 && os.Args[1]=="init" {
  if os.Geteuid()!=0{log.Fatal("root required")}
  if e:=initialize(preference,runtime+"/state");e!=nil{log.Fatal(e)};return
 }
 if os.Geteuid()!=0{log.Fatal("root required")}
 if e:=dir(runtime);e!=nil{log.Fatal(e)}
 fd,e:=syscall.Open(runtime+"/controller.lock",syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0600);if e!=nil{log.Fatal(e)};defer syscall.Close(fd)
 if e=syscall.Flock(fd,syscall.LOCK_EX|syscall.LOCK_NB);e!=nil{log.Fatal(e)}
 if _,e=read(runtime+"/state");e!=nil{log.Fatal(e)}
 stop:=make(chan os.Signal,1);signal.Notify(stop,syscall.SIGTERM,os.Interrupt)
 tick:=time.NewTicker(500*time.Millisecond);defer tick.Stop()
 log.Print("READY stereo link: native FULL AE and AWB peer copy")
 for{select{case <-stop:return;case <-tick.C:
  if _,e=os.Lstat(runtime+"/request");os.IsNotExist(e){continue}
  handle(preference,runtime,command)
 }}
}
