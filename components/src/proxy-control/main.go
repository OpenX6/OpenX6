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
 "strings"
 "syscall"
 "time"
)
const base="/data/x6-owner-features/proxy-v1"
const runtime="/run/x6-proxy-control"
func dir(p string) error {s,e:=os.Lstat(p);if e!=nil{return e};if !s.IsDir()||s.Mode().Perm()&0022!=0||s.Sys().(*syscall.Stat_t).Uid!=0{return errors.New("unsafe directory")};return nil}
func read(p string)(byte,error){
 if e:=dir(filepath.Dir(p));e!=nil{return 0,e}
 fd,e:=syscall.Open(p,syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0);if e!=nil{return 0,e};f:=os.NewFile(uintptr(fd),p);defer f.Close();s,e:=f.Stat();if e!=nil{return 0,e};if !s.Mode().IsRegular()||s.Sys().(*syscall.Stat_t).Uid!=0||s.Mode().Perm()&0022!=0{return 0,errors.New("unsafe preference")};b,e:=io.ReadAll(io.LimitReader(f,2));if e!=nil{return 0,e};if len(b)!=1||b[0]>1{return 0,errors.New("invalid preference")};return b[0],nil
}
func save(p string,v byte)error{
 if v>1{return errors.New("invalid preference")};if e:=dir(filepath.Dir(p));e!=nil{return e}
 if s,e:=os.Lstat(p);e==nil&&!s.Mode().IsRegular(){return errors.New("unsafe destination")}else if e!=nil&&!os.IsNotExist(e){return e}
 f,e:=os.CreateTemp(filepath.Dir(p),".proxy-*");if e!=nil{return e};defer os.Remove(f.Name());if _,e=f.Write([]byte{v});e==nil{e=f.Sync()};ce:=f.Close();if e==nil{e=ce};if e!=nil{return e};if e=os.Rename(f.Name(),p);e!=nil{return e};d,e:=os.Open(filepath.Dir(p));if e!=nil{return e};defer d.Close();return d.Sync()
}
func command(name string,args ...string)([]byte,error){c,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel();return exec.CommandContext(c,name,args...).CombinedOutput()}
type media struct {Record struct{Status string `json:"record_status"`;Time int64 `json:"record_time_ms"`} `json:"record"`;Pipeline struct{Status struct{States []int `json:"status"`} `json:"pipeline_status"`} `json:"pipeline"`}
func getMedia()(media,error){var m media;b,e:=command("/usr/bin/ins_bll_cli",`{"cmd":"share_state","action":"get","type":"media"}`);if e!=nil{return m,e};var r struct{Ret string `json:"ret"`;Data media `json:"data"`};if e=json.Unmarshal(b,&r);e!=nil{return m,e};if r.Ret!="ok"||r.Data.Record.Status==""{return m,errors.New("invalid media response")};return r.Data,nil}
type policy struct{running,stopped,known,square bool;last int64;attempts int}
func(p *policy)want(enabled bool,m media)bool{
 running:=m.Record.Status=="RecordRunning"
 if !running {if m.Record.Status=="RecordIdle"{*p=policy{}};return false}
 if !p.running||m.Record.Time<p.last {*p=policy{running:true}}
 p.last=m.Record.Time
 return enabled&&!p.stopped&&p.attempts<3
}
func stopProxy()error{b,e:=command("/usr/bin/ins_ibus_cli","--run","call-method","kCamera","kMediad","Pipeline","PipelineStopStream","StreamTypeSub");if e!=nil||!strings.Contains(string(b),"Error code: Success(0)"){return fmt.Errorf("proxy stop failed: %v %.300s",e,b)};return nil}
func main(){
 log.SetFlags(log.LstdFlags|log.LUTC)
 if os.Geteuid()!=0{log.Fatal("root required")};if e:=dir(base);e!=nil{log.Fatal(e)}
 if e:=os.Mkdir(runtime,0700);e!=nil&&!os.IsExist(e){log.Fatal(e)};if e:=dir(runtime);e!=nil{log.Fatal(e)}
 fd,e:=syscall.Open(runtime+"/lock",syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0600);if e!=nil{log.Fatal(e)};defer syscall.Close(fd);if e=syscall.Flock(fd,syscall.LOCK_EX|syscall.LOCK_NB);e!=nil{log.Fatal(e)}
 enabled,e:=read(base+"/disabled");if e!=nil{log.Fatal(e)};if e=save(runtime+"/state",enabled);e!=nil{log.Fatal(e)}
 stop:=make(chan os.Signal,1);signal.Notify(stop,syscall.SIGTERM,os.Interrupt);defer signal.Stop(stop)
 fast:=time.NewTicker(200*time.Millisecond);defer fast.Stop();poll:=time.NewTicker(2*time.Second);defer poll.Stop();var p policy
 log.Printf("READY disabled=%d",enabled)
 for{select{
 case <-stop:return
 case <-fast.C:
  if _,e=os.Lstat(runtime+"/request");os.IsNotExist(e){continue}
  v,err:=read(runtime+"/request");if err==nil{err=save(base+"/disabled",v)};if err==nil{enabled=v;err=save(runtime+"/state",v)}
  if err!=nil{log.Printf("REJECT preference: %v",err)}else{log.Printf("POLICY disabled=%d; Off affects next recording",v)}
  if err=os.Remove(runtime+"/request");err!=nil&&!os.IsNotExist(err){log.Printf("request cleanup: %v",err)}
 case <-poll.C:
  m,err:=getMedia();if err!=nil{log.Printf("media query: %v",err);continue}
  if p.want(enabled==1,m) && p.canStop(m,squareRecording,liveWriters){p.attempts++;if err=stopProxy();err!=nil{log.Printf("STOP_FAILED attempt=%d %v",p.attempts,err)}else{p.stopped=true;log.Printf("PROXY_STOPPED record_ms=%d",m.Record.Time)}}
 }}
}
