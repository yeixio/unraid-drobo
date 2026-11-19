//go:build linux

// Package scsi sends read-only SCSI commands through the Linux SG_IO ioctl.
//
// Adapted from droboctl (https://github.com/ryngrn/droboctl),
// Copyright (c) 2026 Ryan Green, MIT License. See NOTICE.
package scsi

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	sgIO           = 0x2285
	sgDxferFromDev = -3
)

type sgIOHdr struct {
	InterfaceID    int32
	DxferDirection int32
	CmdLen         uint8
	MxSbLen        uint8
	IOVecCount     uint16
	DxferLen       uint32
	Dxferp         uintptr
	Cmdp           uintptr
	Sbp            uintptr
	Timeout        uint32
	Flags          uint32
	PackID         int32
	UsrPtr         uintptr
	Status         uint8
	MaskedStatus   uint8
	MsgStatus      uint8
	SbLenWr        uint8
	HostStatus     uint16
	DriverStatus   uint16
	Resid          int32
	Duration       uint32
	Info           uint32
}

// allowed lists the only command opcodes this package will send. Every one of
// them moves data from the device to the host and changes nothing on it.
var allowed = map[byte]string{
	0x12: "INQUIRY",
	0x5a: "MODE SENSE(10)",
}

// Read sends a data-in command and returns the bytes the device sent back.
func Read(device string, cdb []byte, allocation int) ([]byte, error) {
	if len(cdb) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	if _, ok := allowed[cdb[0]]; !ok {
		return nil, fmt.Errorf("refusing SCSI opcode 0x%02x: only read-only commands are allowed", cdb[0])
	}
	f, err := os.OpenFile(device, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", device, err)
	}
	defer f.Close()

	data := make([]byte, allocation)
	sense := make([]byte, 64)
	hdr := sgIOHdr{
		InterfaceID:    int32('S'),
		DxferDirection: sgDxferFromDev,
		CmdLen:         uint8(len(cdb)),
		MxSbLen:        uint8(len(sense)),
		DxferLen:       uint32(len(data)),
		Dxferp:         uintptr(unsafe.Pointer(&data[0])),
		Cmdp:           uintptr(unsafe.Pointer(&cdb[0])),
		Sbp:            uintptr(unsafe.Pointer(&sense[0])),
		Timeout:        5000,
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), sgIO, uintptr(unsafe.Pointer(&hdr)))
	if errno != 0 {
		return nil, fmt.Errorf("SG_IO: %w", errno)
	}
	if hdr.Status != 0 || hdr.HostStatus != 0 || hdr.DriverStatus != 0 {
		return nil, fmt.Errorf("SCSI %s failed: status=0x%02x host=0x%04x driver=0x%04x sense=%x",
			allowed[cdb[0]], hdr.Status, hdr.HostStatus, hdr.DriverStatus, sense[:hdr.SbLenWr])
	}
	n := len(data) - int(hdr.Resid)
	if n < 0 || n > len(data) {
		n = len(data)
	}
	return data[:n], nil
}

// Inquiry returns the standard INQUIRY data (vendor, product, revision).
func Inquiry(device string) (vendor, product, revision string, err error) {
	b, err := Read(device, []byte{0x12, 0, 0, 0, 96, 0}, 96)
	if err != nil {
		return "", "", "", err
	}
	if len(b) < 36 {
		return "", "", "", fmt.Errorf("short INQUIRY reply: %d bytes", len(b))
	}
	trim := func(x []byte) string {
		s := string(x)
		for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == 0) {
			s = s[:len(s)-1]
		}
		return s
	}
	return trim(b[8:16]), trim(b[16:32]), trim(b[32:36]), nil
}
