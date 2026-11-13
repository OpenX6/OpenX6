package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
)

type digestProgress func(done, total int64)

// All validated offsets are half-open. Hash the overlapping ranges while
// reading the underlying package once. Keep prefix.Sum non-destructive so the
// same SHA-256 state can continue through the certificate and check trailer.
func scanContent(file io.ReaderAt, size, payload, end, cert int64, embedded bool, moduleHex, prefixHex, expected []byte, progress digestProgress) error {
	if payload < 0 || end < payload || cert < end || cert > size || size-cert < embeddedOffset+embeddedSize {
		return fail("invalid digest range")
	}
	mod, e := hex.DecodeString(string(moduleHex))
	if e != nil || len(mod) != sha256.Size {
		return fail("digest encoding invalid")
	}
	prefix, e := hex.DecodeString(string(prefixHex))
	if e != nil || len(prefix) != sha256.Size {
		return fail("digest encoding invalid")
	}
	whole, part := sha256.New(), sha256.New()
	src := io.NewSectionReader(file, 0, size)
	buf := make([]byte, 256<<10)
	var done int64
	if progress != nil {
		progress(0, size)
	}
	feed := func(until int64, out io.Writer) error {
		for done < until {
			n := int64(len(buf))
			if n > until-done {
				n = until - done
			}
			b := buf[:n]
			if _, err := io.ReadFull(src, b); err != nil {
				return err
			}
			if _, err := out.Write(b); err != nil {
				return err
			}
			done += n
			if progress != nil && done < size {
				progress(done, size)
			}
		}
		return nil
	}
	if e = feed(payload, whole); e != nil {
		return e
	}
	if e = feed(end, io.MultiWriter(whole, part)); e != nil {
		return e
	}
	if !bytes.Equal(part.Sum(nil), mod) {
		return fail("digest mismatch")
	}
	if e = feed(cert, whole); e != nil {
		return e
	}
	if !bytes.Equal(whole.Sum(nil), prefix) {
		return fail("digest mismatch")
	}
	if embedded {
		if e = feed(cert+embeddedOffset, whole); e != nil {
			return e
		}
		// Only the fixed-size embedded owner record is represented by zeros.
		if e = feed(cert+embeddedOffset+embeddedSize, io.Discard); e != nil {
			return e
		}
		whole.Write(make([]byte, embeddedSize))
	}
	if e = feed(size, whole); e != nil {
		return e
	}
	if !bytes.Equal(whole.Sum(nil), expected) {
		return fail("owner-signed file digest mismatch")
	}
	return nil
}

type observedHash struct {
	io.Writer
	report      digestProgress
	done, total int64
}

func (w *observedHash) Write(b []byte) (int, error) {
	n, e := w.Writer.Write(b)
	w.done += int64(n)
	if w.report != nil && w.done < w.total {
		w.report(w.done, w.total)
	}
	return n, e
}
