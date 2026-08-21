package app

import "testing"

func TestRecordLogRespectsTroubleshootingLogLevel(t *testing.T) {
	server := newScannerTestServer(t)
	server.recordLog("debug", "hidden debug", nil)
	if len(server.listLogEvents(10)) != 0 {
		t.Fatalf("debug log should be suppressed by the default info log level")
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'troubleshooting'`, `{"logLevel":"debug","keepLogsDays":14}`); err != nil {
		t.Fatalf("set debug log level: %v", err)
	}
	server.recordLog("debug", "visible debug", nil)
	events := server.listLogEvents(10)
	if len(events) != 1 || events[0].Level != "debug" {
		t.Fatalf("debug log was not retained: %#v", events)
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'troubleshooting'`, `{"logLevel":"warn","keepLogsDays":14}`); err != nil {
		t.Fatalf("set warn log level: %v", err)
	}
	server.recordLog("info", "hidden info", nil)
	server.recordLog("warn", "visible warn", nil)
	server.recordLog("error", "visible error", nil)
	events = server.listLogEvents(10)
	counts := map[string]int{}
	for _, event := range events {
		counts[event.Level]++
	}
	if len(events) != 3 || counts["debug"] != 1 || counts["warn"] != 1 || counts["error"] != 1 || counts["info"] != 0 {
		t.Fatalf("warn log level should suppress new info entries while retaining warn/error entries: %#v", events)
	}
}

func TestNotificationSettingsFilterDashboardAlerts(t *testing.T) {
	server := newScannerTestServer(t)
	server.recordLog("warn", "warn alert", nil)
	server.recordLog("error", "error alert", nil)
	if len(server.dashboardAlerts()) != 2 {
		t.Fatalf("expected default warn+error alerts")
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'notifications'`, `{"enabled":true,"minAlertLevel":"error"}`); err != nil {
		t.Fatalf("save notification settings: %v", err)
	}
	alerts := server.dashboardAlerts()
	if len(alerts) != 1 || alerts[0].Level != "error" {
		t.Fatalf("expected only error alert, got %#v", alerts)
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'notifications'`, `{"enabled":false,"minAlertLevel":"warn"}`); err != nil {
		t.Fatalf("disable notification settings: %v", err)
	}
	if alerts := server.dashboardAlerts(); len(alerts) != 0 {
		t.Fatalf("expected alerts disabled, got %#v", alerts)
	}
}
