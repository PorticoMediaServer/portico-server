package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type jobSmokeReport struct {
	DatabasePath         string            `json:"databasePath"`
	DuplicateActiveScans []duplicateScan   `json:"duplicateActiveScans"`
	StaleRunningJobs     []staleRunningJob `json:"staleRunningJobs"`
	CheckedAt            string            `json:"checkedAt"`
}

type duplicateScan struct {
	LibraryID string `json:"libraryId"`
	Count     int    `json:"count"`
}

type staleRunningJob struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	ResourceType   string `json:"resourceType"`
	ResourceID     string `json:"resourceId"`
	UpdatedAt      string `json:"updatedAt"`
	LeaseExpiresAt string `json:"leaseExpiresAt,omitempty"`
}

func main() {
	if len(os.Args) != 2 {
		fail("usage: go run ./scripts/check-active-jobs.go /path/to/portico.db")
	}
	dbPath := strings.TrimSpace(os.Args[1])
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=query_only(ON)")
	if err != nil {
		fail("open database: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		fail("ping database: %v", err)
	}
	if !tableExists(db, "jobs") {
		printReport(jobSmokeReport{DatabasePath: dbPath, CheckedAt: time.Now().UTC().Format(time.RFC3339)})
		return
	}
	columns := tableColumns(db, "jobs")
	report := jobSmokeReport{
		DatabasePath:         dbPath,
		DuplicateActiveScans: duplicateActiveLibraryScans(db, columns),
		StaleRunningJobs:     staleRunningJobs(db, columns, time.Now().UTC().Add(-2*time.Hour).Format(time.RFC3339)),
		CheckedAt:            time.Now().UTC().Format(time.RFC3339),
	}
	printReport(report)
	if len(report.DuplicateActiveScans) > 0 || len(report.StaleRunningJobs) > 0 {
		os.Exit(1)
	}
}

func duplicateActiveLibraryScans(db *sql.DB, columns map[string]bool) []duplicateScan {
	if !columns["type"] || !columns["status"] || !columns["resource_id"] {
		return nil
	}
	resourceTypeFilter := ""
	if columns["resource_type"] {
		resourceTypeFilter = " AND resource_type = 'library'"
	}
	rows, err := db.Query(`
		SELECT resource_id, COUNT(*)
		FROM jobs
		WHERE type = 'library_scan' AND status IN ('queued', 'running') AND resource_id <> ''` + resourceTypeFilter + `
		GROUP BY resource_id
		HAVING COUNT(*) > 1
		ORDER BY resource_id`)
	if err != nil {
		fail("query duplicate active scans: %v", err)
	}
	defer rows.Close()
	var out []duplicateScan
	for rows.Next() {
		var item duplicateScan
		if err := rows.Scan(&item.LibraryID, &item.Count); err != nil {
			fail("scan duplicate active scans: %v", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		fail("read duplicate active scans: %v", err)
	}
	return out
}

func staleRunningJobs(db *sql.DB, columns map[string]bool, cutoff string) []staleRunningJob {
	for _, column := range []string{"id", "type", "status", "updated_at"} {
		if !columns[column] {
			return nil
		}
	}
	resourceTypeExpr := "''"
	if columns["resource_type"] {
		resourceTypeExpr = "resource_type"
	}
	resourceIDExpr := "''"
	if columns["resource_id"] {
		resourceIDExpr = "resource_id"
	}
	leaseExpr := "''"
	leaseFilter := ""
	if columns["lease_expires_at"] {
		leaseExpr = "lease_expires_at"
		leaseFilter = " AND (lease_expires_at = '' OR lease_expires_at <= ?)"
	}
	args := []any{cutoff}
	if leaseFilter != "" {
		args = append(args, time.Now().UTC().Format(time.RFC3339))
	}
	rows, err := db.Query(`
		SELECT id, type, `+resourceTypeExpr+`, `+resourceIDExpr+`, updated_at, `+leaseExpr+`
		FROM jobs
		WHERE status = 'running' AND updated_at <= ?`+leaseFilter+`
		ORDER BY updated_at ASC`, args...)
	if err != nil {
		fail("query stale running jobs: %v", err)
	}
	defer rows.Close()
	var out []staleRunningJob
	for rows.Next() {
		var item staleRunningJob
		if err := rows.Scan(&item.ID, &item.Type, &item.ResourceType, &item.ResourceID, &item.UpdatedAt, &item.LeaseExpiresAt); err != nil {
			fail("scan stale running jobs: %v", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		fail("read stale running jobs: %v", err)
	}
	return out
}

func tableExists(db *sql.DB, table string) bool {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
		fail("check table existence: %v", err)
	}
	return count > 0
}

func tableColumns(db *sql.DB, table string) map[string]bool {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		fail("read table columns: %v", err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			fail("scan table columns: %v", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		fail("read table columns: %v", err)
	}
	return columns
}

func printReport(report jobSmokeReport) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fail("encode report: %v", err)
	}
}

func fail(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
