package main
import("testing";"strings")
func TestOwnerListenerFamilies(t *testing.T){
 v4:=func(port string)[]byte{return []byte("0: 012AA8C0:"+port+" 00000000:0000 0A\n")}
 v6:=func(port string)[]byte{return []byte("0: "+strings.Repeat("0",32)+":"+port+" "+strings.Repeat("0",32)+":0000 0A\n")}
 for _,tc:=range []struct{name string;tables [][]byte;want bool}{
  {"v4",[][]byte{append(v4("08AE"),v4("DC7C")...)},true},
  {"mixed",[][]byte{v4("DC7C"),v6("08AE")},true},
  {"v6",[][]byte{append(v6("08AE"),v6("DC7C")...)},true},
  {"ssh-only",[][]byte{v6("08AE")},false},
  {"gui-only",[][]byte{v4("DC7C")},false},
  {"wrong-port",[][]byte{v6("08AF"),v4("DC7C")},false},
  {"not-listening",[][]byte{[]byte(strings.ReplaceAll(string(v6("08AE"))," 0A"," 01")),v4("DC7C")},false},
  {"bad-address",[][]byte{[]byte("0: invalid:08AE x 0A\n"),v4("DC7C")},false},
  {"empty",nil,false},
 }{t.Run(tc.name,func(t *testing.T){if ownerPortsListening(tc.tables...)!=tc.want{t.Fatal(tc.name)}})}
}
