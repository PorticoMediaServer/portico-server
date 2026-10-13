package ingestion

import (
	"context"
	"portico.local/server/internal/catalog"
)

type ScanIssue struct {
	Line       int    `json:"line"`
	Sequence   int64  `json:"sequence"`
	Path       string `json:"path"`
	Reason     string `json:"reason"`
	MessageKey string `json:"messageKey"`
	Warning    bool   `json:"warning"`
}
type ScanIssues struct {
	JobID        string      `json:"jobId"`
	Items        []ScanIssue `json:"items"`
	NextSequence int64       `json:"nextSequence"`
	HasMore      bool        `json:"hasMore"`
}

func (s *Service) Issues(ctx context.Context, library, job string, after int64, limit int) (ScanIssues, error) {
	out := ScanIssues{JobID: job, Items: []ScanIssue{}, NextSequence: after}
	if after < 0 || limit < 1 || limit > 128 {
		return out, catalog.ErrAdminQuery
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jobs j JOIN inventory_runs r ON r.job_id=j.id WHERE j.id=? AND j.library_id=?)`, job, library).Scan(&exists); err != nil {
		return out, err
	}
	if !exists {
		return out, catalog.ErrAdminQuery
	}
	rows, err := s.db.QueryContext(ctx, `SELECT sequence,relative_path,reason,line,warning FROM inventory_scan_issues WHERE job_id=? AND sequence>? ORDER BY sequence LIMIT ?`, job, after, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var issue ScanIssue
		if err = rows.Scan(&issue.Sequence, &issue.Path, &issue.Reason, &issue.Line, &issue.Warning); err != nil {
			return out, err
		}
		issue.MessageKey = scanIssueMessageKey(issue.Reason)
		out.Items = append(out.Items, issue)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.HasMore = true
		out.Items = out.Items[:limit]
	}
	if len(out.Items) > 0 {
		out.NextSequence = out.Items[len(out.Items)-1].Sequence
	}
	return out, nil
}
func scanIssueMessageKey(reason string) string {
	switch reason {
	case "embedded_name_fields_skipped", "directory_changed", "directory_unreadable", "entry_unreadable", "symlink_unreadable", "symlink_outside_root", "directory_alias", "ignored_name", "ignored_rule", "ignore_unreadable", "invalid_ignore_pattern", "ignore_file_budget_exceeded", "ignore_pattern_budget_exceeded", "ignore_pattern_count_exceeded", "path_depth_exceeded", "ignore_diagnostic_budget_exceeded", "unsupported_disc_layout", "unsupported_disc_image", "source_revision_changed":
		return "scan.issue." + reason
	}
	return "scan.issue.analysis_failed"
}
