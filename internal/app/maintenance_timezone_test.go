package app

import (
	"testing"
	"time"
)

func TestMaintenanceTimezoneDefaultsToExplicitUTC(t *testing.T) {
	if got := normalizeMaintenanceTimezone(""); got != "UTC" {
		t.Fatalf("empty maintenance timezone = %q, want UTC", got)
	}
	if got := normalizeMaintenanceTimezone("not/a-zone"); got != "UTC" {
		t.Fatalf("invalid maintenance timezone = %q, want UTC", got)
	}
}

func TestMaintenanceWindowUsesConfiguredLocalTimezoneAcrossDST(t *testing.T) {
	location := maintenanceLocation("America/Halifax")
	// The UTC instant is 02:30 local before the spring transition. The
	// scheduler must evaluate the configured local clock, not the host/UTC hour.
	beforeDST := time.Date(2026, time.March, 8, 6, 30, 0, 0, time.UTC)
	local := beforeDST.In(location)
	if local.Hour() != 3 {
		t.Fatalf("DST transition local hour = %d, want 3", local.Hour())
	}
	if withinScheduledWindow(local, 2, 5) != true {
		t.Fatal("local maintenance window did not include the post-transition local time")
	}
	if withinScheduledDays(local, "weekends") != true {
		t.Fatal("DST transition Sunday was not recognized as a configured weekend")
	}
}

func TestMaintenanceWindowLabelIncludesTimezone(t *testing.T) {
	label := scheduledWindowLabelInTimezone("custom", "America/Halifax", 2, 5)
	if label != "02:00-05:00 America/Halifax" {
		t.Fatalf("timezone label = %q", label)
	}
	if got := scheduledWindowLabelInTimezone("always", "America/Halifax", 0, 0); got != "Any time" {
		t.Fatalf("always label = %q", got)
	}
}
