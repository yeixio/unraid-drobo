package hfs

import (
	"encoding/binary"
	"testing"
)

func TestParse(t *testing.T) {
	b := make([]byte, 512)
	copy(b, "H+")
	binary.BigEndian.PutUint16(b[2:4], 4)
	binary.BigEndian.PutUint32(b[4:8], attrUnmounted|attrJournaled)
	binary.BigEndian.PutUint32(b[40:44], 4096)
	binary.BigEndian.PutUint32(b[44:48], 1000)
	binary.BigEndian.PutUint32(b[48:52], 250)
	h, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if !h.CleanUnmount || !h.Journaled || h.Inconsistent || h.BlockSize != 4096 || h.FreeBlocks != 250 {
		t.Fatalf("got %+v", h)
	}
	b[0] = 'X'
	if _, err := Parse(b); err == nil {
		t.Fatal("expected error for bad signature")
	}
}
