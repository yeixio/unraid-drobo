package report

import (
	"testing"

	"github.com/yeixio/unraid-drobo/internal/device"
	"github.com/yeixio/unraid-drobo/internal/esa"
	"github.com/yeixio/unraid-drobo/internal/hfs"
)

func codes(f []Finding) map[string]Level {
	m := map[string]Level{}
	for _, x := range f {
		m[x.Code] = x.Level
	}
	return m
}

func TestHealthyDroboHasNoFindings(t *testing.T) {
	d := Drobo{
		Health:   "Healthy",
		USB:      device.USB{SpeedMbps: 5000, PortMaxMbps: 5000, Version: "3.00"},
		Capacity: &esa.Capacity{TotalProtected: 10e12, UsedProtected: 5e12, FreeProtected: 5e12},
		Bays:     []Bay{{Number: 1, Health: "Good"}, {Number: 2, Empty: true, Health: "Empty"}},
		Volumes: []Volume{{Partition: "/dev/sdi2", Mountpoint: "/mnt/disks/x", FreeBytes: 5.1e12,
			HFS: &hfs.Header{Signature: "H+", Inconsistent: true}}}, // inconsistent is normal while mounted rw
	}
	if f := Assess(d, Settings{}); len(f) != 0 {
		t.Fatalf("want no findings, got %+v", f)
	}
}

func TestProblems(t *testing.T) {
	d := Drobo{
		Health:   "Critical",
		Relayout: true,
		USB:      device.USB{SpeedMbps: 480, PortMaxMbps: 5000, Version: "3.00"},
		Capacity: &esa.Capacity{TotalProtected: 10e12, UsedProtected: 9.6e12, FreeProtected: 0.4e12},
		Bays:     []Bay{{Number: 3, Health: "Failed"}},
		Volumes: []Volume{{Partition: "/dev/sdi2", Mountpoint: "/mnt/disks/x", ReadOnly: true, FreeBytes: 9e12,
			HFS: &hfs.Header{Signature: "H+", CleanUnmount: false, Journaled: true}}},
	}
	got := codes(Assess(d, Settings{WarnPct: 85, CritPct: 95}))
	want := map[string]Level{
		"health": Alert, "relayout": Warning, "bay3-failed": Alert, "full-crit": Alert,
		"overcommit": Warning, "usb2": Warning, "hfs-dirty:/dev/sdi2": Warning, "ro:/dev/sdi2": Warning,
	}
	for k, l := range want {
		if got[k] != l {
			t.Errorf("%s: got %q want %q (all: %v)", k, got[k], l, got)
		}
	}
	f := Assess(d, Settings{})
	if f[0].Level != Alert {
		t.Errorf("alerts should come first: %+v", f[0])
	}
}

func TestUSB2PortOnly(t *testing.T) {
	d := Drobo{USB: device.USB{SpeedMbps: 480, PortMaxMbps: 480, Version: "3.00"}}
	f := Assess(d, Settings{})
	if len(f) != 1 || f[0].Code != "usb2" {
		t.Fatalf("got %+v", f)
	}
}

func TestUnescape(t *testing.T) {
	if got := unescape(`/mnt/disks/The\040Other`); got != "/mnt/disks/The Other" {
		t.Fatal(got)
	}
}
