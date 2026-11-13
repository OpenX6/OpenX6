package main

// Host-only fixture for execution of the actual patched ARM64 hook. The
// private signing keys exist only in this process; only packets leave it.
import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOfflineBrokerDriver(t *testing.T) {
	dir := os.Getenv("X6_OFFLINE_FIXTURE_DIR")
	if dir == "" {
		t.Skip("integration driver only")
	}
	a, k := authFixture(t)
	cases := map[string][]byte{}
	for _, name := range []string{"valid", "partial", "eintr", "wrong-key", "tampered", "max-payload", "native-valid", "native-unknown-cmd", "native-unknown-action", "native-missing-action", "native-max", "native-invalid-json", "native-prefixed", "native-null", "native-empty", "native-tampered-header"} {
		n := nonceFor(t, a)
		payload := sample
		if name == "max-payload" {
			payload = append([]byte{'"'}, bytes.Repeat([]byte{'x'}, maxPacket-110)...)
			payload = append(payload, '"')
		}
		switch name {
		case "native-valid", "native-max", "native-tampered-header":
			payload = []byte(`{"cmd":"DEVICEINFO","action":"get_factory_mode"}`)
			if name == "native-max" {
				payload = append(payload[:len(payload)-1], []byte(`,"padding":"`)...)
				payload = append(payload, bytes.Repeat([]byte{'x'}, maxPacket-108-len(payload)-2)...)
				payload = append(payload, '"', '}')
			}
		case "native-unknown-cmd":
			payload = []byte(`{"cmd":"ABSENT","action":"get_factory_mode"}`)
		case "native-unknown-action":
			payload = []byte(`{"cmd":"DEVICEINFO","action":"absent"}`)
		case "native-missing-action":
			payload = []byte(`{"cmd":"DEVICEINFO"}`)
		case "native-invalid-json":
			payload = []byte(`{"cmd":broken}`)
		case "native-prefixed":
			payload = append(make([]byte, 15), sample...)
		case "native-null":
			payload = []byte(`null`)
		case "native-empty":
			payload = []byte{}
		}
		p := signedPacket(k, n, payload)
		if name == "native-tampered-header" {
			p[104]++
		}
		if name == "wrong-key" {
			_, other := authFixture(t)
			p = signedPacket(other, n, payload)
		}
		if name == "tampered" {
			p[len(p)-3] ^= 1
		}
		cases[name] = p
	}
	cases["raw-json"] = sample
	cases["challenge"] = []byte(challengeRequest)
	l, e := net.Listen("unix", filepath.Join(dir, "auth.sock"))
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	b, _ := json.Marshal(cases)
	if e = os.WriteFile(filepath.Join(dir, "cases.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go a.handle(c)
		}
	}()
	limit := time.Now().Add(90 * time.Second)
	for time.Now().Before(limit) {
		if _, e = os.Stat(filepath.Join(dir, "stop")); e == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("offline driver timed out")
}
