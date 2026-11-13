package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcReceiverIdentity(t *testing.T) {
	for _, name := range []string{"exact", "same-name-wrong-exe", "deleted-exe", "zombie", "unrelated", "malformed", "vanished", "exe-missing-live"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "42")
			if e := os.Mkdir(p, 0700); e != nil {
				t.Fatal(e)
			}
			comm, exe, state := "ins_usb", "/usr/bin/ins_usb", "S"
			switch name {
			case "same-name-wrong-exe":
				exe = "/tmp/ins_usb"
			case "deleted-exe":
				exe += " (deleted)"
			case "zombie":
				state = "Z"
				exe = ""
			case "exe-missing-live":
				exe = ""
			case "unrelated":
				comm = "other"
				exe = "/bin/other"
			case "vanished":
				got, e := (procReceiver{dir}).Snapshot()
				if e != nil || len(got) != 0 {
					t.Fatal("vanished proc not ignored")
				}
				return
			}
			if e := os.WriteFile(p+"/comm", []byte(comm+"\n"), 0600); e != nil {
				t.Fatal(e)
			}
			stat := "42 (" + comm + ") " + state + " " + strings.Repeat("0 ", 18) + "98765 0 0\n"
			if name == "malformed" {
				stat = "invalid"
			}
			if e := os.WriteFile(p+"/stat", []byte(stat), 0600); e != nil {
				t.Fatal(e)
			}
			if exe != "" {
				if e := os.Symlink(exe, p+"/exe"); e != nil {
					t.Fatal(e)
				}
			}
			got, e := (procReceiver{dir}).Snapshot()
			bad := name == "same-name-wrong-exe" || name == "deleted-exe" || name == "malformed" || name == "exe-missing-live"
			if (e != nil) != bad {
				t.Fatalf("unexpected error: %v", e)
			}
			if name == "exe-missing-live" {
				var pending *receiverExitPending
				if !errors.As(e, &pending) || pending.Process != (receiverProcess{42, "98765"}) || pending.State != "S" || len(got) != 0 {
					t.Fatalf("missing exe must remain unresolved: %v %v", got, e)
				}
			}
			if name == "exact" && (len(got) != 1 || got[0] != (receiverProcess{42, "98765"})) {
				t.Fatalf("bad identity: %v", got)
			}
			if (name == "zombie" || name == "unrelated") && len(got) != 0 {
				t.Fatal("unrelated/dead process selected")
			}
		})
	}
}
