package main

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

// These are the two real target UnitPath entries omitted by v8. Fixtures use
// the real packaged files; only UID ownership is mapped to the host test user.
func TestAttachedUnitRoots(t *testing.T) {
 stock,e:=os.ReadFile(filepath.Join(os.Getenv("X6_BUILD_TEST_ROOT"),"startup-stock/ins_gui.service"));if e!=nil{t.Fatal(e)}
 roots:=[]struct{name,path string}{{"etc","/etc/systemd/system.attached"},{"run","/run/systemd/system.attached"}}
 cases:=[]struct{name,rel,kind,want string}{
  {"empty","","empty","bridge"},
  {"unrelated_unit","unrelated.service","write","bridge"},
  {"known_stock_regular","ins_gui.service","stock","bridge"},
  {"known_stock_link","ins_gui.service","stock-link","bridge"},
  {"unknown_GUI_unit","ins_gui.service","write","hard"},
  {"masked_GUI_unit","ins_gui.service","mask","hard"},
  {"dangling_GUI_alias","ins_gui.service","dangling","hard"},
  {"unexpected_GUI_alias","ins_gui.service","unexpected-link","hard"},
  {"writable_stock_GUI","ins_gui.service","writable-stock","hard"},
  {"broker_override","x6-owner-gui-auth.service","write","fallback"},
  {"late_override","x6-owner-gui-late-start.service","write","fallback"},
  {"generic_dropin","service.d/unknown.conf","write","hard"},
  {"GUI_prefix_dropin","ins_.service.d/unknown.conf","write","hard"},
  {"GUI_specific_dropin","ins_gui.service.d/unknown.conf","write","hard"},
  {"x6_prefix_dropin","x6-.service.d/unknown.conf","write","fallback"},
  {"owner_prefix_dropin","x6-owner-.service.d/unknown.conf","write","fallback"},
  {"GUI_owner_prefix_dropin","x6-owner-gui-.service.d/unknown.conf","write","fallback"},
  {"broker_specific_dropin","x6-owner-gui-auth.service.d/unknown.conf","write","fallback"},
  {"late_specific_dropin","x6-owner-gui-late-start.service.d/unknown.conf","write","fallback"},
  {"non_conf_file","ins_gui.service.d/ignored.txt","write","bridge"},
 }
 for _,root:=range roots {for _,c:=range cases {t.Run(root.name+"/"+c.name,func(t *testing.T){
  v:=base(t);if e:=os.MkdirAll(v.path(root.path),0755);e!=nil{t.Fatal(e)};p:=root.path+"/"+c.rel
  switch c.kind {
  case "empty":
  case "write":write(t,v,p,[]byte("untrusted fixture"),0644)
  case "stock":write(t,v,p,stock,0644)
  case "writable-stock":write(t,v,p,stock,0666)
  default:
   target:=map[string]string{"stock-link":"/ins/etc/systemd/system/multi-user.target.wants/ins_gui.service","mask":"/dev/null","dangling":"/missing.service","unexpected-link":"/tmp/alias.service"}[c.kind]
   if c.kind=="unexpected-link"{write(t,v,"/tmp/alias.service",stock,0644)}
   if e:=os.Symlink(target,v.path(p));e!=nil{t.Fatal(e)}
  }
  err:=v.check();plan,pe:=v.guiPlan()
  switch c.want {
  case "bridge":
   if err!=nil||pe!=nil||!plan.Bridge||plan.Disabled!=""||len(plan.Args)!=4{t.Fatalf("healthy config rejected: %v %+v %v",err,plan,pe)}
  case "hard":
   if err==nil||pe==nil{t.Fatalf("untrusted GUI configuration accepted: %v %+v %v",err,plan,pe)}
   if !strings.Contains(err.Error(),root.path)||!strings.Contains(pe.Error(),root.path){t.Fatalf("not rejected at attached root: %v %v",err,pe)}
  case "fallback":
   if err==nil||pe!=nil||plan.Bridge||plan.Disabled==""||len(plan.Args)!=2||plan.Args[1]!="/usr/bin/ins_gui"{t.Fatalf("unsafe owner fallback: %v %+v %v",err,plan,pe)}
   if !strings.Contains(err.Error(),root.path)||!strings.Contains(plan.Disabled,root.path){t.Fatal("wrong failure origin")}
  }
  // Guard/plan must not repair, unlink or rewrite any inspected override.
  if c.kind=="write"||c.kind=="stock"||c.kind=="writable-stock" {
   b,e:=os.ReadFile(v.path(p));if e!=nil{t.Fatal(e)}
   want:=[]byte("untrusted fixture");if c.kind!="write"{want=stock};if string(b)!=string(want){t.Fatal("guard mutated input")}
  } else if c.kind!="empty" {if _,e:=os.Lstat(v.path(p));e!=nil{t.Fatal(e)};if _,e:=os.Readlink(v.path(p));e!=nil{t.Fatal(e)}}
  // No persistent identity is created by read-only unit checks.
  if _,e:=os.Lstat(v.path(filepath.Join("/data/x6-owner-gui","server_ed25519")));!os.IsNotExist(e){t.Fatal("guard touched identity")}
 })}}
}
