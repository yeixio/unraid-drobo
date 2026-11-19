// Package hfs reads the HFS+ volume header to tell whether a volume was
// cleanly unmounted. It opens the partition read-only and reads 512 bytes.
package hfs

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Volume header attribute bits (Apple TN1150).
const (
	attrUnmounted    = 1 << 8  // set when the volume was cleanly unmounted
	attrInconsistent = 1 << 11 // set when the volume needs repair
	attrJournaled    = 1 << 13
)

// Header is the part of the HFS+ volume header we care about.
type Header struct {
	Signature    string `json:"signature"` // "H+" or "HX"
	Version      uint16 `json:"version"`
	CleanUnmount bool   `json:"clean_unmount"`
	Inconsistent bool   `json:"inconsistent"`
	Journaled    bool   `json:"journaled"`
	BlockSize    uint32 `json:"block_size"`
	TotalBlocks  uint32 `json:"total_blocks"`
	FreeBlocks   uint32 `json:"free_blocks"`
}

// Read reads the volume header from a partition such as /dev/sdi2.
func Read(partition string) (Header, error) {
	f, err := os.Open(partition)
	if err != nil {
		return Header{}, err
	}
	defer f.Close()
	buf := make([]byte, 512)
	if _, err := f.ReadAt(buf, 1024); err != nil && err != io.EOF {
		return Header{}, err
	}
	return Parse(buf)
}

// Parse decodes a 512-byte volume header.
func Parse(b []byte) (Header, error) {
	if len(b) < 52 {
		return Header{}, fmt.Errorf("short volume header")
	}
	sig := string(b[0:2])
	if sig != "H+" && sig != "HX" {
		return Header{}, fmt.Errorf("not an HFS+ volume (signature %q)", sig)
	}
	attr := binary.BigEndian.Uint32(b[4:8])
	return Header{
		Signature:    sig,
		Version:      binary.BigEndian.Uint16(b[2:4]),
		CleanUnmount: attr&attrUnmounted != 0,
		Inconsistent: attr&attrInconsistent != 0,
		Journaled:    attr&attrJournaled != 0,
		BlockSize:    binary.BigEndian.Uint32(b[40:44]),
		TotalBlocks:  binary.BigEndian.Uint32(b[44:48]),
		FreeBlocks:   binary.BigEndian.Uint32(b[48:52]),
	}, nil
}
