// Command drobo-status reads Drobo status for the Unraid plugin.
//
//	drobo-status json [--max-age 20]   print the report as JSON (cached)
//	drobo-status check                 refresh and send Unraid notifications
//	drobo-status text                  print a readable summary
//	drobo-status version
//
// It only ever reads from the Drobo.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/yeixio/unraid-drobo/internal/alerts"
	"github.com/yeixio/unraid-drobo/internal/report"
)

var version = "dev"

var (
	configFile = "/boot/config/plugins/drobo/drobo.cfg"
	stateDir   = "/var/local/emhttp/drobo"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "json":
		err = cmdJSON(os.Args[2:])
	case "check":
		err = cmdCheck()
	case "text":
		err = cmdText()
	case "version":
		fmt.Println(version)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "drobo-status:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: drobo-status json [--max-age seconds] | check | text | version")
	os.Exit(2)
}

func cmdJSON(args []string) error {
	fs := flag.NewFlagSet("json", flag.ExitOnError)
	maxAge := fs.Int("max-age", 20, "reuse a report younger than this many seconds")
	fs.Parse(args)
	b, err := cached(time.Duration(*maxAge) * time.Second)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(b)
	return err
}

func cmdCheck() error {
	cfg := loadConfig()
	r, _, err := fresh(cfg)
	if err != nil {
		return err
	}
	if cfg["NOTIFY"] == "no" {
		return nil
	}
	path := filepath.Join(stateDir, "notified.json")
	return alerts.Save(path, alerts.Process(r, alerts.Load(path), alerts.Unraid))
}

func cmdText() error {
	r, _, err := fresh(loadConfig())
	if err != nil {
		return err
	}
	if r.Error != "" {
		return fmt.Errorf("%s", r.Error)
	}
	if len(r.Drobos) == 0 {
		fmt.Println("No Drobo found.")
		return nil
	}
	for _, d := range r.Drobos {
		fmt.Printf("%s  health=%s  usb=%dMbps (port max %d, %s)  protocol=%s\n", d.Name, or(d.Health, "unknown"), d.USB.SpeedMbps, d.USB.PortMaxMbps, or(d.USB.Driver, "?"), or(d.Protocol, "-"))
		if d.Error != "" {
			fmt.Println("  error:", d.Error)
		}
		if c := d.Capacity; c != nil {
			fmt.Printf("  protected: %d used / %d total, %d free (bytes)\n", c.UsedProtected, c.TotalProtected, c.FreeProtected)
		}
		for _, l := range d.LUNs {
			fmt.Printf("  disk %s (%s): %d bytes as Linux sees it\n", l.Block, l.SG, l.SizeBytes)
		}
		for _, b := range d.Bays {
			fmt.Printf("  bay %d: %-7s %s %d bytes %dC errors=%d\n", b.Number, b.Health, b.Model, b.TotalCapacity, b.TemperatureC, b.ErrorCount)
		}
		for _, v := range d.Volumes {
			fmt.Printf("  volume %s on %q ro=%v size=%d free=%d", v.Partition, v.Mountpoint, v.ReadOnly, v.SizeBytes, v.FreeBytes)
			if v.HFS != nil {
				fmt.Printf(" hfs clean=%v journaled=%v repair=%v", v.HFS.CleanUnmount, v.HFS.Journaled, v.HFS.Inconsistent)
			}
			fmt.Println()
		}
		for _, f := range d.Findings {
			fmt.Printf("  [%s] %s: %s\n", f.Level, f.Title, f.Detail)
		}
	}
	return nil
}

func or(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// cached returns the last report if it is young enough, otherwise a new one.
func cached(maxAge time.Duration) ([]byte, error) {
	path := filepath.Join(stateDir, "status.json")
	if st, err := os.Stat(path); err == nil && time.Since(st.ModTime()) < maxAge {
		if b, err := os.ReadFile(path); err == nil {
			return b, nil
		}
	}
	_, b, err := fresh(loadConfig())
	return b, err
}

// fresh builds a new report under a lock, so the page and the cron check
// never talk to the Drobo at the same time, and saves it for the page.
func fresh(cfg map[string]string) (report.Report, []byte, error) {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return report.Report{}, nil, err
	}
	lock, err := os.OpenFile(filepath.Join(stateDir, "lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return report.Report{}, nil, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return report.Report{}, nil, err
	}
	r := report.Build(report.Settings{WarnPct: atoi(cfg["WARN_PCT"], 85), CritPct: atoi(cfg["CRIT_PCT"], 95)})
	b, err := json.Marshal(r)
	if err != nil {
		return r, nil, err
	}
	path := filepath.Join(stateDir, "status.json")
	if err := os.WriteFile(path+".tmp", b, 0o644); err == nil {
		os.Rename(path+".tmp", path)
	}
	return r, b, nil
}

// loadConfig reads KEY="value" lines.
func loadConfig() map[string]string {
	m := map[string]string{}
	f, err := os.Open(configFile)
	if err != nil {
		return m
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || strings.HasPrefix(k, "#") {
			continue
		}
		m[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return m
}

func atoi(s string, d int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > 100 {
		return d
	}
	return n
}
