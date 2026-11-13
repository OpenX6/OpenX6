package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPhoneConsentUpload(t *testing.T) {
	raw := make([]byte, 10000)
	copy(raw, "MWFI")
	copy(raw[8:], "c9")
	binary.LittleEndian.PutUint32(raw[96:], 1)
	copy(raw[len(raw)-4416:], "CFSI")
	copy(raw[len(raw)-256:], "CWFI")
	for _, tc := range []struct {
		name string
		code int
	}{{"approved", 200}, {"first-cancel", 403}, {"second-cancel", 403}, {"disconnected", 403}, {"busy", 403}, {"broken-tail", 400}, {"plaintext", 400}, {"two-files", 400}, {"truncated", 400}, {"no-space", 507}, {"handoff-error", 409}} {
		t.Run(tc.name, func(t *testing.T) {
			data := bytes.Clone(raw)
			switch tc.name {
			case "broken-tail":
				data[len(data)-256] = 0
			case "plaintext":
				data[96] = 0
			}
			b := new(bytes.Buffer)
			mp := multipart.NewWriter(b)
			part, _ := mp.CreateFormFile("file", "Insta360X6FW.bin")
			part.Write(data)
			if tc.name == "two-files" {
				part, _ = mp.CreateFormFile("file", "second")
				part.Write(data)
			}
			mp.Close()
			if tc.name == "truncated" {
				b.Truncate(b.Len() - 20)
			}
			body := b.Bytes()
			r := httptest.NewRequest("POST", "/receive", bytes.NewReader(body))
			r.Header.Set("Content-Type", mp.FormDataContentType())
			pages := ""
			spooled := false
			dispatched := false
			g := phoneEndpoint{connected: func(string) bool { return tc.name != "disconnected" }, ready: func() bool { return tc.name != "busy" },
				question: func(n byte, live func() bool) bool {
					pages += fmt.Sprint(n)
					return live() && !(n == 1 && tc.name == "first-cancel" || n == 2 && tc.name == "second-cancel")
				},
				spool: func(int64) (*os.File, error) {
					spooled = true
					if tc.name == "no-space" {
						return nil, io.ErrShortWrite
					}
					return os.CreateTemp(t.TempDir(), "file")
				},
				install: func(u phoneFile) error {
					if pages != "12" || u.digest != sha256.Sum256(raw) {
						t.Fatal("wrong bytes or consent")
					}
					if tc.name == "handoff-error" {
						return io.ErrClosedPipe
					}
					s := makeSetup(t, "1.2.36", false)
					p := s.p
					p.Package = filepath.Join(p.Boundary, "mobile", "package.bin")
					if err := stageInput(u.f, p, nil, u.inspect, nil); err != nil {
						t.Fatal(err)
					}
					m, err := stagedRecord(p)
					if err != nil || m.Digest != u.digest || !m.Native {
						t.Fatal(err, m)
					}
					if _, err = verify(p, p.Package, "", false); err != nil {
						t.Fatal(err)
					}
					dispatched = true
					return nil
				}}
			w := httptest.NewRecorder()
			g.ServeHTTP(w, r)
			if w.Code != tc.code || dispatched != (tc.code == 200) || (tc.code == 403 && spooled) {
				t.Fatal(w.Code, dispatched, spooled, w.Body.String())
			}
			if strings.Contains(tc.name, "cancel") {
				if (tc.name == "first-cancel" && pages != "1") || (tc.name == "second-cancel" && pages != "12") {
					t.Fatal(pages)
				}
				old := pages
				w = httptest.NewRecorder()
				g.ServeHTTP(w, r)
				if pages != old || w.Code != 403 {
					t.Fatal("retry re-prompted")
				}
			}
		})
	}
}
