package main
import("testing";"path/filepath";"bytes")
func TestFreshInstallDistinctSSHIdentities(t *testing.T){a:=filepath.Join(t.TempDir(),"ssh","host");b:=filepath.Join(t.TempDir(),"ssh","host");ka,e:=loadHostKey(a);if e!=nil{t.Fatal(e)};kb,e:=loadHostKey(b);if e!=nil{t.Fatal(e)};if bytes.Equal(ka.PublicKey().Marshal(),kb.PublicKey().Marshal()){t.Fatal("identities cloned")};again,e:=loadHostKey(a);if e!=nil{t.Fatal(e)};if !bytes.Equal(ka.PublicKey().Marshal(),again.PublicKey().Marshal()){t.Fatal("rotated")};t.Log("two fresh roots get distinct SSH identities; restart preserves identity")}
