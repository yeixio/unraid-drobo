package report

import (
	"fmt"
	"math"
)

// Assess turns a Drobo's raw state into findings, most serious first.
func Assess(d Drobo, s Settings) []Finding {
	if s.WarnPct <= 0 {
		s.WarnPct = 85
	}
	if s.CritPct <= 0 {
		s.CritPct = 95
	}
	var f []Finding
	add := func(l Level, code, title, detail string) {
		f = append(f, Finding{Level: l, Code: code, Title: title, Detail: detail})
	}

	if d.Error != "" {
		add(Warning, "esa", "Can't read the Drobo's own status",
			"Unraid can see the disk, but the Drobo didn't answer the status request, so bay and capacity details are missing. "+d.Error)
	}

	switch d.Health {
	case "Critical":
		add(Alert, "health", "Drobo reports a critical problem",
			"The Drobo's lights are probably red. Check the bays below; data may not be protected.")
	case "Warning":
		add(Warning, "health", "Drobo reports a warning",
			"The Drobo's lights are probably yellow. Check the bays and free space below.")
	}
	if d.Relayout {
		add(Warning, "relayout", "Drobo is rebuilding data protection",
			"Don't remove or swap drives until this finishes. It can take hours to days, and the Drobo is slower meanwhile.")
	}

	for _, b := range d.Bays {
		if b.Empty {
			continue
		}
		label := fmt.Sprintf("Bay %d", b.Number)
		if b.SSD {
			label = "The cache SSD"
		}
		code := fmt.Sprintf("bay%d", b.Number)
		if b.SSD {
			code = "cache"
		}
		switch b.Health {
		case "Failed":
			add(Alert, code+"-failed", label+" has failed",
				fmt.Sprintf("%s (%s, serial %s). Replace it with a drive of the same size or larger.", b.Model, human(b.TotalCapacity), b.Serial))
		case "Warning":
			add(Warning, code+"-warning", label+" needs attention",
				fmt.Sprintf("%s (%s) is reporting problems and may fail soon.", b.Model, human(b.TotalCapacity)))
		}
	}

	if c := d.Capacity; c != nil && c.TotalProtected > 0 {
		pct := float64(c.UsedProtected) * 100 / float64(c.TotalProtected)
		detail := fmt.Sprintf("%s of %s used (%.0f%%), %s free.", human(c.UsedProtected), human(c.TotalProtected), pct, human(c.FreeProtected))
		switch {
		case pct >= float64(s.CritPct):
			add(Alert, "full-crit", "Drobo is almost full", detail+" Add or upgrade a drive now; a full Drobo slows down and can stop accepting writes.")
		case pct >= float64(s.WarnPct):
			add(Warning, "full-warn", "Drobo is getting full", detail+" Plan to add or upgrade a drive.")
		}

		// The volume's own view of free space versus the Drobo's. A Drobo
		// presents a "thin" disk bigger than its drives, so the filesystem
		// can believe it has room the Drobo doesn't have.
		var fsFree uint64
		for _, v := range d.Volumes {
			fsFree += v.FreeBytes
		}
		if fsFree > c.FreeProtected {
			gap := fsFree - c.FreeProtected
			if float64(gap) > math.Max(0.05*float64(c.TotalProtected), 100e9) {
				l := Normal
				if pct >= float64(s.WarnPct) {
					l = Warning
				}
				add(l, "overcommit", "Unraid shows more free space than the Drobo has",
					fmt.Sprintf("The volume reports %s free, but the Drobo only has %s of real protected space. Treat %s as the true limit; this is normal for Drobos, which present a larger disk than their drives.",
						human(fsFree), human(c.FreeProtected), human(c.FreeProtected)))
			}
		}
	}

	u := d.USB
	if u.SpeedMbps > 0 && u.SpeedMbps < 5000 && u.Version >= "3" {
		detail := fmt.Sprintf("The Drobo can do USB 3 (5 Gbps) but is linked at %s. Copies and scans will be much slower.", speed(u.SpeedMbps))
		if u.PortMaxMbps >= 5000 {
			detail += " The port supports USB 3, so try a different USB 3 cable, re-seat the plug, or power-cycle the Drobo."
		} else if u.PortMaxMbps > 0 {
			detail += " The port it's on is USB 2 only; move it to a USB 3 port."
		}
		add(Warning, "usb2", "Drobo is running at USB 2 speed", detail)
	}

	for _, v := range d.Volumes {
		h := v.HFS
		where := v.Partition
		if v.Mountpoint != "" {
			where = v.Mountpoint
		}
		mountedRW := v.Mountpoint != "" && !v.ReadOnly
		// While Linux has HFS+ mounted read-write it marks the volume
		// "in use", so the dirty and repair bits only mean something when
		// the volume is not mounted read-write.
		if h != nil && !mountedRW {
			if !h.CleanUnmount {
				add(Warning, "hfs-dirty:"+v.Partition, "Volume was not cleanly unmounted",
					where+" was last disconnected without unmounting. Linux will only mount it read-only until it is repaired (fsck.hfsplus, or First Aid on a Mac).")
			} else if h.Inconsistent {
				add(Warning, "hfs-repair:"+v.Partition, "Volume needs repair",
					where+" is flagged as needing repair. Run fsck.hfsplus, or First Aid on a Mac.")
			}
		}
		if v.Mountpoint != "" && v.ReadOnly {
			detail := where + " is mounted read-only, so nothing can be added to it."
			if h != nil && h.Journaled {
				detail += " The volume is journaled; Linux only writes to journaled HFS+ when it is mounted with the force option or the journal is turned off on a Mac."
			}
			add(Warning, "ro:"+v.Partition, "Volume is read-only", detail)
		}
	}
	if len(d.Volumes) > 0 {
		mounted := false
		for _, v := range d.Volumes {
			if v.Mountpoint != "" {
				mounted = true
			}
		}
		if !mounted {
			add(Normal, "unmounted", "Volume is not mounted", "Mount it from Unassigned Devices to use it.")
		}
	}

	order := map[Level]int{Alert: 0, Warning: 1, Normal: 2}
	for i := 1; i < len(f); i++ {
		for j := i; j > 0 && order[f[j].Level] < order[f[j-1].Level]; j-- {
			f[j], f[j-1] = f[j-1], f[j]
		}
	}
	if f == nil {
		f = []Finding{}
	}
	return f
}

func speed(mbps int) string {
	switch {
	case mbps >= 1000:
		return fmt.Sprintf("%g Gbps", float64(mbps)/1000)
	case mbps == 480:
		return "USB 2 (480 Mbps)"
	default:
		return fmt.Sprintf("%d Mbps", mbps)
	}
}

func human(n uint64) string {
	const (
		GB = 1e9
		TB = 1e12
	)
	switch {
	case n >= TB:
		return fmt.Sprintf("%.2f TB", float64(n)/TB)
	case n >= GB:
		return fmt.Sprintf("%.1f GB", float64(n)/GB)
	default:
		return fmt.Sprintf("%.0f MB", float64(n)/1e6)
	}
}
