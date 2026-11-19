// Package report gathers everything the plugin shows about each Drobo and
// turns it into plain-language warnings.
package report

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/yeixio/unraid-drobo/internal/device"
	"github.com/yeixio/unraid-drobo/internal/esa"
	"github.com/yeixio/unraid-drobo/internal/hfs"
	"github.com/yeixio/unraid-drobo/internal/scsi"
)

// Settings are the user-tunable thresholds.
type Settings struct {
	WarnPct int // Drobo protected space used, percent, for a warning
	CritPct int // ... and for an alert
}

// Level is how serious a finding is. It matches Unraid's notify levels.
type Level string

const (
	Normal  Level = "normal"
	Warning Level = "warning"
	Alert   Level = "alert"
)

// Finding is one thing worth telling the user about.
type Finding struct {
	Level  Level  `json:"level"`
	Code   string `json:"code"` // stable id, used to avoid repeat notifications
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// Bay is one drive bay.
type Bay struct {
	esa.Slot
	Number int    `json:"number"`
	Empty  bool   `json:"empty"`
	Health string `json:"health"`
	SSD    bool   `json:"ssd"`
}

// Volume is a partition on a Drobo LUN.
type Volume struct {
	Partition  string      `json:"partition"`
	FSType     string      `json:"fs_type,omitempty"`
	HFS        *hfs.Header `json:"hfs,omitempty"`
	Mountpoint string      `json:"mountpoint,omitempty"`
	ReadOnly   bool        `json:"read_only"`
	SizeBytes  uint64      `json:"size_bytes,omitempty"`
	FreeBytes  uint64      `json:"free_bytes,omitempty"`
	UsedBytes  uint64      `json:"used_bytes,omitempty"`
}

// Drobo is the report for one unit.
type Drobo struct {
	Name     string        `json:"name"`
	USB      device.USB    `json:"usb"`
	LUNs     []device.LUN  `json:"luns"`
	Protocol string        `json:"protocol,omitempty"`
	Health   string        `json:"health,omitempty"` // Healthy, Warning, Critical
	Status   *esa.Status   `json:"status,omitempty"`
	Relayout bool          `json:"relayout"`
	Capacity *esa.Capacity `json:"capacity,omitempty"`
	Bays     []Bay         `json:"bays"`
	Volumes  []Volume      `json:"volumes"`
	Error    string        `json:"error,omitempty"`
	Findings []Finding     `json:"findings"`
}

// Report is the whole output.
type Report struct {
	Time   time.Time `json:"time"`
	Drobos []Drobo   `json:"drobos"`
	Error  string    `json:"error,omitempty"`
}

// MountsFile is /proc/mounts; tests may change it.
var MountsFile = "/proc/mounts"

// Build reads every Drobo and returns the report.
func Build(s Settings) Report {
	r := Report{Time: time.Now().UTC(), Drobos: []Drobo{}}
	ds, err := device.Discover()
	if err != nil {
		r.Error = err.Error()
		return r
	}
	mounts := readMounts()
	for _, d := range ds {
		r.Drobos = append(r.Drobos, readOne(d, mounts, s))
	}
	return r
}

func readOne(d device.Drobo, mounts map[string]mount, s Settings) Drobo {
	out := Drobo{USB: d.USB, LUNs: d.LUNs, Bays: []Bay{}, Volumes: []Volume{}}
	out.Name = d.USB.Product
	if out.Name == "" && len(d.LUNs) > 0 {
		out.Name = strings.TrimSpace(d.LUNs[0].Vendor + " " + d.LUNs[0].Model)
	}
	if out.Name == "" {
		out.Name = "Drobo"
	}

	if len(d.LUNs) > 0 {
		if err := readESA(&out, d.LUNs[0].SG); err != nil {
			out.Error = err.Error()
		}
	}
	for _, l := range d.LUNs {
		out.Volumes = append(out.Volumes, volumes(l.Block, mounts)...)
	}
	out.Findings = Assess(out, s)
	return out
}

func readESA(out *Drobo, sg string) error {
	page := func(sub byte, n int) ([]byte, error) {
		cdb := esa.BuildModeSense10(sub, uint16(n))
		return scsi.Read(sg, cdb[:], n)
	}
	raw, err := page(esa.ProtocolVersion, 64)
	if err != nil {
		return fmt.Errorf("could not read the Drobo's management pages on %s: %w", sg, err)
	}
	maj, min, err := esa.ParseProtocolVersion(raw)
	if err != nil {
		return err
	}
	out.Protocol = fmt.Sprintf("%d.%d", maj, min)

	if raw, err = page(esa.CapacityInfo, 64); err != nil {
		return fmt.Errorf("capacity: %w", err)
	}
	c, err := esa.ParseCapacity(raw)
	if err != nil {
		return err
	}
	out.Capacity = &c

	if raw, err = page(esa.StatusInfo, 64); err != nil {
		return fmt.Errorf("status: %w", err)
	}
	st, err := esa.ParseStatus(raw)
	if err != nil {
		return err
	}
	out.Status = &st
	out.Health = st.Severity()
	out.Relayout = st.RelayoutCount > 0

	if raw, err = page(esa.SlotInfo2, 1308); err != nil {
		return fmt.Errorf("bays: %w", err)
	}
	slots, err := esa.ParseSlotInfo2(raw)
	if err != nil {
		return err
	}
	n := 0
	for _, sl := range slots {
		b := Bay{Slot: sl, Empty: sl.Empty(), SSD: sl.IsSSD()}
		if !b.SSD {
			n++
			b.Number = n
		}
		if b.Empty {
			b.Health = "Empty"
		} else {
			b.Health = sl.Health()
		}
		out.Bays = append(out.Bays, b)
	}
	return nil
}

type mount struct {
	point string
	fs    string
	ro    bool
}

func readMounts() map[string]mount {
	m := map[string]mount{}
	f, err := os.Open(MountsFile)
	if err != nil {
		return m
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 || !strings.HasPrefix(fields[0], "/dev/") {
			continue
		}
		dev := fields[0]
		if real, err := filepath.EvalSymlinks(dev); err == nil {
			dev = real
		}
		ro := false
		for _, o := range strings.Split(fields[3], ",") {
			if o == "ro" {
				ro = true
			}
		}
		if _, seen := m[dev]; !seen {
			m[dev] = mount{point: unescape(fields[1]), fs: fields[2], ro: ro}
		}
	}
	return m
}

// unescape undoes the octal escapes /proc/mounts uses for spaces etc.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			var v int
			if _, err := fmt.Sscanf(s[i+1:i+4], "%o", &v); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func volumes(block string, mounts map[string]mount) []Volume {
	if block == "" {
		return nil
	}
	name := filepath.Base(block)
	parts, _ := filepath.Glob(filepath.Join(device.Root, "class/block", name, name+"*"))
	sort.Strings(parts)
	cands := []string{}
	for _, p := range parts {
		cands = append(cands, "/dev/"+filepath.Base(p))
	}
	if len(cands) == 0 {
		cands = []string{block}
	}
	var out []Volume
	for _, dev := range cands {
		v := Volume{Partition: dev}
		if h, err := hfs.Read(dev); err == nil {
			hh := h
			v.HFS = &hh
			v.FSType = "hfsplus"
		}
		if m, ok := mounts[dev]; ok {
			v.Mountpoint, v.ReadOnly = m.point, m.ro
			v.FSType = m.fs
			var st syscall.Statfs_t
			if err := syscall.Statfs(m.point, &st); err == nil {
				bs := uint64(st.Bsize)
				v.SizeBytes = st.Blocks * bs
				v.FreeBytes = st.Bavail * bs
				v.UsedBytes = (st.Blocks - st.Bfree) * bs
			}
		} else if v.HFS != nil {
			bs := uint64(v.HFS.BlockSize)
			v.SizeBytes = uint64(v.HFS.TotalBlocks) * bs
			v.FreeBytes = uint64(v.HFS.FreeBlocks) * bs
			v.UsedBytes = v.SizeBytes - v.FreeBytes
		}
		// Skip small helper partitions (the EFI one) that are not mounted.
		if v.HFS == nil && v.Mountpoint == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}
