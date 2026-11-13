package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
)

const stageLeasePath = "/data/x6-sd-update/private-stage.json"

var stageNameRE = regexp.MustCompile(`^OpenX6-private-stage-[0-9a-f]{32}\.img$`)

type StageLease struct {
	Native                           bool `json:",omitempty"`
	Version                          int
	Card, Digest, Boot, Image, Phase string
	Bytes, Device, Inode             uint64
	CardMountID                      int
}

func stageMayRecover(phase string) bool {
	return phase == "idle" || phase == "resident" || phase == "staged" || phase == "completed"
}
func decodeStageLease(b []byte) (StageLease, error) {
	var s StageLease
	if len(b) > 4096 {
		return s, fail("oversize stage lease")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(&s); e != nil {
		return s, e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return s, fail("trailing lease data")
	}
	if s.Version != 1 || !validID(s.Card) || !validID(s.Digest) || s.Boot == "" || !stageNameRE.MatchString(s.Image) || s.Bytes < 128<<20 || (!s.Native && s.Bytes > 5<<30) || s.Bytes > 1<<63-1-(64<<20) || s.CardMountID <= 0 {
		return s, fail("invalid private stage lease")
	}
	switch s.Phase {
	case "creating", "allocated", "mounted", "published":
	default:
		return s, fmt.Errorf("invalid stage phase")
	}
	return s, nil
}

// Lease JSON is bounded, not a fixed-width key/signature. Require a private
// 0600 file, then decode the actual number of bytes read.
func loadStageLeaseAt(path string, p policy) (StageLease, error) {
	f, e := openChecked(path, p, false)
	if e != nil {
		return StageLease{}, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return StageLease{}, e
	}
	if st.Mode().Perm() != 0600 || st.Size() > 4096 {
		return StageLease{}, fail("unsafe lease file")
	}
	b, e := io.ReadAll(io.LimitReader(f, 4097))
	if e != nil {
		return StageLease{}, e
	}
	return decodeStageLease(b)
}
