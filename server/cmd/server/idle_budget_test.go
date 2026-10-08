//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"portico.local/server/internal/testtier"
	"sort"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
)

type idleBudgetReport struct {
	WindowSeconds float64                        `json:"windowSeconds"`
	Statements    []uint64                       `json:"statements"`
	Top           []dbwork.StatementProfileEntry `json:"top,omitempty"`
}

// The default mode catches accidentally restored subsecond polling in a real
// server. PORTICO_FULL_IDLE=1 observes six complete minutes, including fallback
// maintenance timers. Both modes use a fresh local state with no provider,
// Hosted, DNS, remote-source, or discovery configuration.
func TestServerIdleStatementBudget(t *testing.T) {
	if os.Getenv("PORTICO_IDLE_HELPER") == "1" {
		t.Skip("parent only")
	}
	testtier.Media(t, "a 60 s idle window of a real server process")
	binary, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-test.run=^TestServerIdleBudgetHelper$", "-test.v")
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "PORTICO_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	full := os.Getenv("PORTICO_FULL_IDLE")
	reportPath := filepath.Join(root, "idle.json")
	cmd.Env = append(cmd.Env, "PORTICO_IDLE_HELPER=1", "PORTICO_IDLE_REPORT="+reportPath, "PORTICO_FULL_IDLE="+full, "PORTICO_STATE_DIR="+filepath.Join(root, "state"), "PORTICO_BIND=127.0.0.1:0", "PORTICO_DISCOVERY=0")
	output, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("isolated idle server: %v\n%s", e, output)
	}
	raw, e := os.ReadFile(reportPath)
	if e != nil {
		t.Fatal(e)
	}
	var report idleBudgetReport
	if e = json.Unmarshal(raw, &report); e != nil || len(report.Statements) == 0 {
		t.Fatal(string(raw), e)
	}
	t.Logf("isolated full-server idle: %s", raw)
	for i, count := range report.Statements {
		// Each observation is a complete minute. Comparing a fractional
		// two-second window rounded down to zero classified one late startup
		// statement as a broken idle budget.
		ceiling := uint64(10)
		if count > ceiling {
			t.Errorf("idle window %d: %d SQL statements in %.0fs; budget %d", i+1, count, report.WindowSeconds, ceiling)
		}
	}
}

func TestServerIdleBudgetHelper(t *testing.T) {
	if os.Getenv("PORTICO_IDLE_HELPER") != "1" {
		t.Skip("isolated subprocess only")
	}
	done := make(chan error, 1)
	ready := make(chan struct{})
	maintenanceDone := make(chan struct{})
	serverReadyHook = func() { close(ready) }
	deferredMaintenanceDoneHook = func() { close(maintenanceDone) }
	go func() {
		// The router and deferred one-shot maintenance each publish an explicit
		// completion signal. The latter may legitimately be library-sized; a
		// fixed sleep after process start cannot distinguish it from idle work.
		select {
		case <-ready:
		case <-time.After(2 * time.Minute):
			done <- fmt.Errorf("server did not finish startup")
			return
		}
		select {
		case <-maintenanceDone:
		case <-time.After(2 * time.Minute):
			done <- fmt.Errorf("deferred startup maintenance did not finish")
			return
		}
		// Let commit subscribers finish their first pass after maintenance.
		time.Sleep(2 * time.Second)
		window, windows := time.Minute, 1
		if os.Getenv("PORTICO_FULL_IDLE") == "1" {
			window, windows = time.Minute, 6
		}
		report := idleBudgetReport{WindowSeconds: window.Seconds()}
		for i := 0; i < windows; i++ {
			dbwork.ProfileStatements(true)
			before := dbwork.Reads().Statements
			timer := time.NewTimer(window)
			<-timer.C
			report.Statements = append(report.Statements, dbwork.Reads().Statements-before)
			profile := dbwork.StatementProfile()
			dbwork.ProfileStatements(false)
			sort.Slice(profile, func(i, j int) bool { return profile[i].Count > profile[j].Count })
			if len(profile) > 12 {
				profile = profile[:12]
			}
			report.Top = profile
		}
		raw, e := json.Marshal(report)
		if e == nil {
			e = os.WriteFile(os.Getenv("PORTICO_IDLE_REPORT"), raw, 0600)
		}
		done <- e
		process, e := os.FindProcess(os.Getpid())
		if e == nil {
			_ = process.Signal(os.Interrupt)
		}
	}()
	if e := run(); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal(fmt.Errorf("idle observation did not finish"))
	}
}
