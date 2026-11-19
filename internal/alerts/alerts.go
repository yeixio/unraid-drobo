// Package alerts sends an Unraid notification when a finding first appears
// and another when it clears, remembering what it already sent.
package alerts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/yeixio/unraid-drobo/internal/report"
)

// NotifyScript is Unraid's notification helper.
var NotifyScript = "/usr/local/emhttp/webGui/scripts/notify"

// Sent maps "<drobo key>/<finding code>" to the finding that was announced.
type Sent map[string]report.Finding

const (
	presentKey = "_present"
	goneKey    = "_gone"
)

// Notifier sends one notification. Tests replace it.
type Notifier func(subject, detail string, level report.Level) error

// Unraid sends through the notify script.
func Unraid(subject, detail string, level report.Level) error {
	return exec.Command(NotifyScript, "-e", "Drobo", "-s", subject, "-d", detail,
		"-i", string(level), "-l", "/Main").Run()
}

// Process compares the report with what was sent before, notifies about new
// and cleared problems, and returns the new state.
func Process(r report.Report, before Sent, send Notifier) Sent {
	if r.Error == "" && len(r.Drobos) == 0 {
		// Unplugged, powered off, or re-enumerating. Say so once, and keep
		// the rest of the state so nothing re-alerts when it comes back.
		if _, wasThere := before[presentKey]; wasThere {
			_ = send("Drobo is not connected", "Unraid can no longer see the Drobo. Check its power and USB cable.", report.Alert)
			out := Sent{}
			for k, v := range before {
				if k != presentKey {
					out[k] = v
				}
			}
			out[goneKey] = report.Finding{Level: report.Alert, Code: "gone"}
			return out
		}
		return before
	}
	now := Sent{}
	if len(r.Drobos) > 0 {
		now[presentKey] = report.Finding{Level: report.Normal, Code: "present"}
		if _, wasGone := before[goneKey]; wasGone {
			_ = send("Drobo is connected again", "Unraid can see the Drobo again.", report.Normal)
		}
	}
	for _, d := range r.Drobos {
		key := d.USB.Serial
		if key == "" {
			key = d.USB.BusPath
		}
		for _, f := range d.Findings {
			if f.Level == report.Normal {
				continue
			}
			id := key + "/" + f.Code
			now[id] = f
			if prev, ok := before[id]; !ok || prev.Level != f.Level {
				_ = send(d.Name+": "+f.Title, f.Detail, f.Level)
			}
		}
	}
	// If the report itself failed or no Drobo was found, don't announce
	// everything as fixed; keep the old state until we can see again.
	if r.Error != "" {
		return before
	}
	for id, f := range before {
		if id == presentKey || id == goneKey {
			continue
		}
		if _, still := now[id]; !still {
			_ = send("Drobo: resolved: "+f.Title, "This is no longer reported.", report.Normal)
		}
	}
	return now
}

// Load reads the saved state; a missing file is an empty state.
func Load(path string) Sent {
	s := Sent{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

// Save writes the state atomically.
func Save(path string, s Sent) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.Marshal(s)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
