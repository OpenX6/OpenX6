package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClosedTransportEligibility(t *testing.T) {
	for _, name := range []string{"closed", "open-device", "read-worker", "write-worker", "paused", "uninterruptible", "running", "different-wait", "wrong-start", "wrong-exe", "missing-fd-dir"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			p := filepath.Join(root, "42")
			for _, d := range []string{p + "/fd", p + "/task/42"} {
				if e := os.MkdirAll(d, 0700); e != nil {
					t.Fatal(e)
				}
			}
			state := "S"
			start := "98765"
			exe := "/usr/bin/ins_usb"
			wait := "futex_wait_queue"
			comm := "ins_usb"
			switch name {
			case "paused":
				state = "T"
			case "uninterruptible":
				state = "D"
			case "running":
				state = "R"
			case "different-wait":
				wait = "do_epoll_wait"
			case "wrong-start":
				start = "98766"
			case "wrong-exe":
				exe = "/tmp/ins_usb"
			case "read-worker":
				comm = "simple_usb_read"
			case "write-worker":
				comm = "simple_usb_writ"
			}
			for f, s := range map[string]string{"comm": "ins_usb\n", "stat": "42 (ins_usb) " + state + " " + strings.Repeat("0 ", 18) + start + " 0 0\n", "wchan": wait, "task/42/comm": comm} {
				if e := os.WriteFile(p+"/"+f, []byte(s), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if e := os.Symlink(exe, p+"/exe"); e != nil {
				t.Fatal(e)
			}
			if name == "open-device" {
				if e := os.Symlink("/dev/ins_gadget", p+"/fd/23"); e != nil {
					t.Fatal(e)
				}
			}
			if name == "missing-fd-dir" {
				if e := os.Remove(p + "/fd"); e != nil {
					t.Fatal(e)
				}
			}
			e := (procReceiver{root}).closedTransport(receiverProcess{42, "98765"})
			if (e == nil) != (name == "closed") {
				t.Fatalf("wrong cleanup eligibility: %v", e)
			}
		})
	}
}
