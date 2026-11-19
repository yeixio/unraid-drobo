package alerts

import (
	"testing"

	"github.com/yeixio/unraid-drobo/internal/device"
	"github.com/yeixio/unraid-drobo/internal/report"
)

func TestProcess(t *testing.T) {
	var sent []string
	send := func(s, d string, l report.Level) error { sent = append(sent, string(l)+":"+s); return nil }
	rep := func(f ...report.Finding) report.Report {
		return report.Report{Drobos: []report.Drobo{{Name: "Drobo5C", USB: device.USB{Serial: "X"}, Findings: f}}}
	}
	usb2 := report.Finding{Level: report.Warning, Code: "usb2", Title: "Drobo is running at USB 2 speed"}

	st := Process(rep(usb2), Sent{}, send)
	st = Process(rep(usb2), st, send) // no repeat
	if len(sent) != 1 {
		t.Fatalf("want 1 notification, got %v", sent)
	}
	st = Process(report.Report{Error: "boom"}, st, send) // can't see: keep state
	if len(st) != 2 || len(sent) != 1 {
		t.Fatalf("error report changed state: %v %v", st, sent)
	}
	st = Process(report.Report{}, st, send) // unplugged
	if len(sent) != 2 || sent[1] != "alert:Drobo is not connected" {
		t.Fatalf("unplug: %v", sent)
	}
	st = Process(report.Report{}, st, send) // still unplugged: quiet
	st = Process(rep(usb2), st, send)       // back, same problem: only "connected again"
	if len(sent) != 3 || sent[2] != "normal:Drobo is connected again" {
		t.Fatalf("replug: %v", sent)
	}
	sent = sent[:2]
	st = Process(rep(), st, send)
	if len(st) != 1 || len(sent) != 3 || sent[2] != "normal:Drobo: resolved: Drobo is running at USB 2 speed" {
		t.Fatalf("got %v %v", st, sent)
	}
}
