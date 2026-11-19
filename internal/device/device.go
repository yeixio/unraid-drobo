// Package device finds Drobos attached over USB by walking sysfs.
// It only reads files under /sys and never opens the disks.
package device

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// DroboVendorID is the USB vendor id of Data Robotics / Drobo.
const DroboVendorID = "19b9"

// Known USB product ids. Unknown ids from the Drobo vendor are still tried.
var knownProducts = map[string]string{
	"3444": "Drobo 5D",
}

// Root is the sysfs root; tests point it at a fake tree.
var Root = "/sys"

// USB describes the USB link the Drobo is on.
type USB struct {
	VendorID    string `json:"vendor_id"`
	ProductID   string `json:"product_id"`
	Product     string `json:"product"`
	KnownModel  string `json:"known_model,omitempty"`
	Serial      string `json:"serial,omitempty"`
	Version     string `json:"version"`          // USB spec the device supports, e.g. "3.00"
	SpeedMbps   int    `json:"speed_mbps"`       // negotiated speed: 480 = USB 2, 5000 = USB 3
	PortMaxMbps int    `json:"port_max_mbps"`    // speed of the root hub (what the controller can do)
	Driver      string `json:"driver,omitempty"` // uas or usb-storage
	BusPath     string `json:"bus_path"`         // e.g. 2-1
}

// LUN is one SCSI disk the Drobo presents.
type LUN struct {
	SG    string `json:"sg"`
	Block string `json:"block"`
	// SizeBytes is the size Linux sees for the disk. Drobos report a large
	// "thin" size (often 16 TB or 64 TB), not the real usable space.
	SizeBytes uint64 `json:"size_bytes"`
	Vendor    string `json:"vendor"`
	Model     string `json:"model"`
	Rev       string `json:"rev"`
}

// Drobo is one physical Drobo with its LUNs.
type Drobo struct {
	USB  USB   `json:"usb"`
	LUNs []LUN `json:"luns"`
}

// Discover lists every Drobo-vendor USB device that exposes a SCSI generic node.
func Discover() ([]Drobo, error) {
	sgs, err := filepath.Glob(filepath.Join(Root, "class/scsi_generic/sg*"))
	if err != nil {
		return nil, err
	}
	sort.Slice(sgs, func(i, j int) bool { return sgNum(sgs[i]) < sgNum(sgs[j]) })
	byUSB := map[string]*Drobo{}
	var order []string
	for _, p := range sgs {
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			continue
		}
		usbDir := findUSBDevice(real)
		if usbDir == "" || strings.ToLower(readTrim(filepath.Join(usbDir, "idVendor"))) != DroboVendorID {
			// Fall back to the SCSI vendor string for setups where the USB
			// parent cannot be found (for example a passthrough VM).
			if !strings.EqualFold(readTrim(filepath.Join(p, "device/vendor")), "drobo") {
				continue
			}
		}
		key := usbDir
		if key == "" {
			key = real
		}
		d, ok := byUSB[key]
		if !ok {
			d = &Drobo{}
			if usbDir != "" {
				d.USB = readUSB(usbDir)
			}
			byUSB[key] = d
			order = append(order, key)
		}
		lun := LUN{
			SG:     "/dev/" + filepath.Base(p),
			Vendor: readTrim(filepath.Join(p, "device/vendor")),
			Model:  readTrim(filepath.Join(p, "device/model")),
			Rev:    readTrim(filepath.Join(p, "device/rev")),
		}
		if entries, _ := os.ReadDir(filepath.Join(p, "device/block")); len(entries) > 0 {
			name := entries[0].Name()
			lun.Block = "/dev/" + name
			if sectors, err := strconv.ParseUint(readTrim(filepath.Join(Root, "class/block", name, "size")), 10, 64); err == nil {
				lun.SizeBytes = sectors * 512
			}
		}
		d.LUNs = append(d.LUNs, lun)
	}
	out := make([]Drobo, 0, len(order))
	for _, k := range order {
		out = append(out, *byUSB[k])
	}
	return out, nil
}

// findUSBDevice walks up from a SCSI device to the USB device directory (the
// one with idVendor), and returns "" if there is none.
func findUSBDevice(path string) string {
	for p := filepath.Clean(path); p != "/" && p != "." && p != Root; p = filepath.Dir(p) {
		if _, err := os.Stat(filepath.Join(p, "idVendor")); err == nil {
			return p
		}
	}
	return ""
}

func readUSB(dir string) USB {
	u := USB{
		VendorID:  strings.ToLower(readTrim(filepath.Join(dir, "idVendor"))),
		ProductID: strings.ToLower(readTrim(filepath.Join(dir, "idProduct"))),
		Product:   readTrim(filepath.Join(dir, "product")),
		Serial:    readTrim(filepath.Join(dir, "serial")),
		Version:   strings.TrimSpace(readTrim(filepath.Join(dir, "version"))),
		SpeedMbps: atoi(readTrim(filepath.Join(dir, "speed"))),
		BusPath:   filepath.Base(dir),
	}
	u.KnownModel = knownProducts[u.ProductID]
	// The root hub ("usbN") sits above the device; its speed is the most the
	// controller can do on that bus.
	for p := filepath.Dir(dir); p != "/" && p != "."; p = filepath.Dir(p) {
		if strings.HasPrefix(filepath.Base(p), "usb") {
			u.PortMaxMbps = atoi(readTrim(filepath.Join(p, "speed")))
			break
		}
	}
	// Interface directories look like 2-1:1.0 and link to their driver.
	if ifs, _ := filepath.Glob(filepath.Join(dir, filepath.Base(dir)+":*")); len(ifs) > 0 {
		if link, err := os.Readlink(filepath.Join(ifs[0], "driver")); err == nil {
			u.Driver = filepath.Base(link)
		}
	}
	return u
}

func sgNum(p string) int { return atoi(strings.TrimPrefix(filepath.Base(p), "sg")) }

func atoi(s string) int {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return int(f)
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
