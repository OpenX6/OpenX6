package main
import("encoding/hex";"strconv";"strings")
// Linux may expose an IPv4-capable wildcard listener in tcp6. This is local
// readiness evidence only, not a replacement for an authenticated remote login.
func ownerPortsListening(tables ...[]byte)bool{
 ports:=map[uint64]bool{2222:false,56444:false}
 for _,table:=range tables{for _,line:=range strings.Split(string(table),"\n"){
  f:=strings.Fields(line);if len(f)<4||f[3]!="0A"{continue}
  a:=strings.Split(f[1],":");if len(a)!=2||(len(a[0])!=8&&len(a[0])!=32)||len(a[1])!=4{continue}
  if _,e:=hex.DecodeString(a[0]);e!=nil{continue};port,e:=strconv.ParseUint(a[1],16,16);if e!=nil{continue}
  if _,ok:=ports[port];ok{ports[port]=true}
 }}
 return ports[2222]&&ports[56444]
}
