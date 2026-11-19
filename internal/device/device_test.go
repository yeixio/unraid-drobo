package device

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	Root = root
	defer func() { Root = "/sys" }()

	hub := filepath.Join(root, "devices/pci0000:00/0000:00:14.0/usb2")
	write(t, filepath.Join(hub, "speed"), "5000")
	usb := filepath.Join(hub, "2-1")
	write(t, filepath.Join(usb, "idVendor"), "19b9")
	write(t, filepath.Join(usb, "idProduct"), "4044")
	write(t, filepath.Join(usb, "product"), "Drobo5C")
	write(t, filepath.Join(usb, "speed"), "480")
	write(t, filepath.Join(usb, "version"), " 3.00")
	iface := filepath.Join(usb, "2-1:1.0")
	drv := filepath.Join(root, "bus/usb/drivers/uas")
	os.MkdirAll(drv, 0o755)
	os.MkdirAll(iface, 0o755)
	os.Symlink(drv, filepath.Join(iface, "driver"))
	sdev := filepath.Join(iface, "host5/target5:0:0/5:0:0:0")
	write(t, filepath.Join(sdev, "vendor"), "Drobo")
	write(t, filepath.Join(sdev, "model"), "5C")
	write(t, filepath.Join(sdev, "rev"), "4.1")
	os.MkdirAll(filepath.Join(sdev, "block/sdi"), 0o755)
	sg := filepath.Join(sdev, "scsi_generic/sg8")
	os.MkdirAll(sg, 0o755)
	os.Symlink(sdev, filepath.Join(sg, "device"))
	os.MkdirAll(filepath.Join(root, "class/scsi_generic"), 0o755)
	os.Symlink(sg, filepath.Join(root, "class/scsi_generic/sg8"))
	write(t, filepath.Join(root, "class/block/sdi/size"), "34359738368")

	// An unrelated USB disk must be ignored.
	other := filepath.Join(hub, "2-2")
	write(t, filepath.Join(other, "idVendor"), "0781")
	osg := filepath.Join(other, "2-2:1.0/host6/target6:0:0/6:0:0:0/scsi_generic/sg9")
	os.MkdirAll(osg, 0o755)
	os.Symlink(osg, filepath.Join(root, "class/scsi_generic/sg9"))

	ds, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 1 {
		t.Fatalf("want 1 Drobo, got %d: %+v", len(ds), ds)
	}
	d := ds[0]
	if d.USB.SpeedMbps != 480 || d.USB.PortMaxMbps != 5000 || d.USB.Driver != "uas" || d.USB.Version != "3.00" {
		t.Errorf("usb: %+v", d.USB)
	}
	if len(d.LUNs) != 1 || d.LUNs[0].SG != "/dev/sg8" || d.LUNs[0].Block != "/dev/sdi" || d.LUNs[0].SizeBytes != 34359738368*512 {
		t.Errorf("luns: %+v", d.LUNs)
	}
}
