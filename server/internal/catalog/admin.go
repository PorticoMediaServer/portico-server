package catalog

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/dbwork"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var ErrLibraryConfigurationConflict = errors.New("library configuration changed; reload before renaming")
var ErrAdminQuery = errors.New("invalid library management query")

type AdminScope struct {
	ServerID    string `json:"serverId"`
	ViewerFence string `json:"viewerFence"`
}
type AdminSource struct {
	Path         string `json:"path"`
	Availability string `json:"availability"`
	Basis        string `json:"basis"`
	ObservedAt   string `json:"observedAt,omitempty"`
	Message      string `json:"message,omitempty"`
}
type AdminJob struct {
	SourceID    string   `json:"sourceId"`
	Phase       string   `json:"phase"`
	Analyzed    int64    `json:"analyzed"`
	Warnings    int64    `json:"warnings"`
	PauseReason string   `json:"pauseReason"`
	ID          string   `json:"id"`
	LibraryID   string   `json:"libraryId"`
	LibraryName string   `json:"libraryName"`
	LibraryKind string   `json:"libraryKind"`
	Status      string   `json:"status"`
	Processed   int      `json:"processed"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   *string  `json:"updatedAt"`
	FinishedAt  *string  `json:"finishedAt"`
	Error       string   `json:"error,omitempty"`
	ErrorCode   string   `json:"errorCode,omitempty"`
	Actions     []string `json:"actions"`
}
type AdminLibrary struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Kind     string      `json:"kind"`
	Revision int64       `json:"revision"`
	Source   AdminSource `json:"source"`
	LastScan *AdminJob   `json:"lastScan"`
	Actions  []string    `json:"actions"`
	// Attention counts the files the library couldn't turn into items (an
	// episode name that needs assignment; GET /v1/libraries/{id}/episode-issues
	// lists them), so a problem never hides behind a clean scan. Absent when
	// there are none; atLeast when there are more than it counts.
	Attention *AdminAttention `json:"attention,omitempty"`
}

// AdminAttention is "N files need attention" for one library.
type AdminAttention struct {
	Files   int  `json:"files"`
	AtLeast bool `json:"atLeast,omitempty"`
}

// attentionCap bounds the count per library: a seek over the issues-only
// index, never a walk of the library.
const attentionCap = 1000

type AdminLibraries struct {
	Scope      AdminScope     `json:"scope"`
	Revision   int64          `json:"revision"`
	Items      []AdminLibrary `json:"items"`
	NextCursor string         `json:"nextCursor"`
}
type AdminJobs struct {
	Scope      AdminScope `json:"scope"`
	Revision   int64      `json:"revision"`
	Items      []AdminJob `json:"items"`
	NextCursor string     `json:"nextCursor"`
}
type AdminRequest struct {
	ServerID, ViewerFence, Profile, LibraryID, Status, Cursor string
	Limit                                                     int
}

func adminJobAction(j *AdminJob) {
	j.Actions = []string{}
	if j.Status == "queued" || j.Status == "running" {
		j.Actions = []string{"pause", "cancel"}
	} else if j.Status == "paused" {
		j.Actions = []string{"resume", "cancel"}
	} else {
		j.Actions = []string{"retry"}
	}
	if j.Error == "scan_source_unavailable" && j.Status == "failed" {
		j.ErrorCode = "source_unavailable"
		j.Error = "The source folder could not be reached. Existing library entries were kept. Restore source access, check the folder, then retry."
	} else if j.Error != "" {
		j.Error = "Scan could not finish. Check source access and ffprobe, then retry."
	}
}
func (s *Service) adminRevision(ctx context.Context) (int64, error) {
	var n int64
	e := s.read().QueryRowContext(ctx, `SELECT revision FROM admin_revision WHERE id=1`).Scan(&n)
	return n, e
}
func (s *Service) adminCursor(ctx context.Context, r AdminRequest, view string) (cursorScope, ContentRevision, cursorValue, error) {
	scope := cursorScope{Profile: r.Profile, Viewer: r.ViewerFence, View: view, Library: r.LibraryID, Category: r.Status, Limit: r.Limit}
	n, e := s.adminRevision(ctx)
	rev := ContentRevision{Catalog: n}
	var c cursorValue
	if e == nil && r.Cursor != "" {
		c, e = s.decodeRevisionCursor(r.Cursor, scope, rev)
	}
	return scope, rev, c, e
}
func adminLimit(r *AdminRequest) error {
	if r.Limit == 0 {
		r.Limit = 40
	}
	if r.Limit < 1 || r.Limit > 40 || len(r.Cursor) > 4096 {
		return ErrAdminQuery
	}
	return nil
}

// AdminJobs reads one page of scan jobs in one read snapshot: the page and
// the revision it reports agree, so a scan that updates its job while the page
// is read never fails the read (NEW-38). Only a cursor from another revision
// is stale.
func (s *Service) AdminJobs(ctx context.Context, r AdminRequest) (out AdminJobs, err error) {
	err = dbwork.WithReadSnapshot(ctx, s.db, func(ctx context.Context) error {
		var e error
		out, e = s.WithContext(ctx).adminJobs(ctx, r)
		return e
	})
	return out, err
}
func (s *Service) adminJobs(ctx context.Context, r AdminRequest) (AdminJobs, error) {
	out := AdminJobs{Scope: AdminScope{r.ServerID, r.ViewerFence}, Items: []AdminJob{}}
	if e := adminLimit(&r); e != nil {
		return out, e
	}
	valid := r.Status == ""
	for _, v := range []string{"queued", "running", "paused", "complete", "complete_with_warnings", "failed", "cancelled"} {
		valid = valid || r.Status == v
	}
	if !valid {
		return out, ErrAdminQuery
	}
	scope, rev, c, e := s.adminCursor(ctx, r, "admin_jobs")
	if e != nil {
		return out, e
	}
	out.Revision = rev.Catalog
	where := "1=1"
	args := []any{}
	if r.LibraryID != "" {
		var found int
		if e = s.read().QueryRowContext(ctx, `SELECT 1 FROM libraries WHERE id=?`, r.LibraryID).Scan(&found); e != nil {
			return out, e
		}
		where += " AND j.library_id=?"
		args = append(args, r.LibraryID)
	}
	if r.Status != "" {
		where += " AND j.status=?"
		args = append(args, r.Status)
	}
	if r.Cursor != "" {
		where += " AND (j.created_at,j.id)<(?,?)"
		args = append(args, c.Value, c.ID)
	}
	args = append(args, r.Limit+1)
	rows, e := s.read().QueryContext(ctx, `SELECT j.id,j.library_id,l.name,l.kind,j.status,j.processed,j.created_at,o.updated_at,o.finished_at,j.error,COALESCE(ir.source_id,''),COALESCE(ir.phase,''),COALESCE(ir.analyzed,0),COALESCE(ir.warnings,0),COALESCE(ir.pause_reason,'') FROM jobs j JOIN libraries l ON l.id=j.library_id LEFT JOIN inventory_runs ir ON ir.job_id=j.id LEFT JOIN job_observations o ON o.job_id=j.id WHERE `+where+` ORDER BY j.created_at DESC,j.id DESC LIMIT ?`, args...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var j AdminJob
		if e = rows.Scan(&j.ID, &j.LibraryID, &j.LibraryName, &j.LibraryKind, &j.Status, &j.Processed, &j.CreatedAt, &j.UpdatedAt, &j.FinishedAt, &j.Error, &j.SourceID, &j.Phase, &j.Analyzed, &j.Warnings, &j.PauseReason); e != nil {
			rows.Close()
			return out, e
		}
		adminJobAction(&j)
		out.Items = append(out.Items, j)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Items) > r.Limit {
		out.Items = out.Items[:r.Limit]
		last := out.Items[len(out.Items)-1]
		out.NextCursor, e = s.encodeRevisionCursor(cursorValue{Scope: scope, Value: last.CreatedAt, ID: last.ID, Expires: time.Now().Add(30 * time.Minute).Unix()}, rev)
		if e != nil {
			return out, e
		}
	}
	return out, nil
}

// AdminLibraries reads one page of libraries in one read snapshot (see
// AdminJobs): a scan in progress never fails the owner's library list.
func (s *Service) AdminLibraries(ctx context.Context, r AdminRequest) (out AdminLibraries, err error) {
	err = dbwork.WithReadSnapshot(ctx, s.db, func(ctx context.Context) error {
		var e error
		out, e = s.WithContext(ctx).adminLibraries(ctx, r)
		return e
	})
	return out, err
}
func (s *Service) adminLibraries(ctx context.Context, r AdminRequest) (AdminLibraries, error) {
	out := AdminLibraries{Scope: AdminScope{r.ServerID, r.ViewerFence}, Items: []AdminLibrary{}}
	if e := adminLimit(&r); e != nil {
		return out, e
	}
	scope, rev, c, e := s.adminCursor(ctx, r, "admin_libraries")
	if e != nil {
		return out, e
	}
	out.Revision = rev.Catalog
	where := "1=1"
	args := []any{}
	if r.LibraryID != "" {
		where += " AND l.id=?"
		args = append(args, r.LibraryID)
	}
	if r.Cursor != "" {
		where += " AND (l.name COLLATE NOCASE>? COLLATE NOCASE OR (l.name COLLATE NOCASE=? COLLATE NOCASE AND l.id>?))"
		args = append(args, c.Value, c.Value, c.ID)
	}
	args = append(args, r.Limit+1)
	ids := []string{}
	rows, e := s.read().QueryContext(ctx, `SELECT l.id,l.name,l.kind,c.revision,l.root,j.id,j.status,j.processed,j.created_at,o.updated_at,o.finished_at,j.error FROM libraries l JOIN library_configuration c ON c.library_id=l.id LEFT JOIN jobs j ON j.id=(SELECT id FROM jobs WHERE library_id=l.id ORDER BY created_at DESC,id DESC LIMIT 1) LEFT JOIN job_observations o ON o.job_id=j.id WHERE `+where+` ORDER BY l.name COLLATE NOCASE,l.id LIMIT ?`, args...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var l AdminLibrary
		var id, status, created, updated, finished, message sql.NullString
		var processed sql.NullInt64
		if e = rows.Scan(&l.ID, &l.Name, &l.Kind, &l.Revision, &l.Source.Path, &id, &status, &processed, &created, &updated, &finished, &message); e != nil {
			rows.Close()
			return out, e
		}
		l.Source.Availability = "unknown"
		l.Source.Basis = "not_checked"
		l.Actions = []string{"rename", "scan"}
		ids = append(ids, l.ID)
		if id.Valid {
			j := AdminJob{ID: id.String, LibraryID: l.ID, Status: status.String, Processed: int(processed.Int64), CreatedAt: created.String, Error: message.String}
			if updated.Valid {
				j.UpdatedAt = &updated.String
			}
			if finished.Valid {
				j.FinishedAt = &finished.String
			}
			adminJobAction(&j)
			l.LastScan = &j
		}
		out.Items = append(out.Items, l)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Items) > r.Limit {
		out.Items = out.Items[:r.Limit]
		last := out.Items[len(out.Items)-1]
		out.NextCursor, e = s.encodeRevisionCursor(cursorValue{Scope: scope, Value: last.Name, ID: last.ID, Expires: time.Now().Add(30 * time.Minute).Unix()}, rev)
		if e != nil {
			return out, e
		}
	}
	for i := range out.Items {
		var n int
		if e = s.read().QueryRowContext(ctx, `SELECT count(*) FROM (SELECT 1 FROM episodic_sources WHERE library_id=? AND issue<>'' LIMIT ?)`, out.Items[i].ID, attentionCap+1).Scan(&n); e != nil {
			return out, e
		}
		if n > 0 {
			out.Items[i].Attention = &AdminAttention{Files: min(n, attentionCap), AtLeast: n > attentionCap}
		}
	}
	return out, nil
}
func (s *Service) AdminLibrary(ctx context.Context, r AdminRequest, inspect bool) (AdminLibrary, error) {
	out, e := s.AdminLibraries(ctx, r)
	if e != nil {
		return AdminLibrary{}, e
	}
	if len(out.Items) != 1 {
		return AdminLibrary{}, sql.ErrNoRows
	}
	l := out.Items[0]
	if inspect {
		if s.storage == nil {
			l.Source.Message = "Source checks are unavailable on this server."
			return l, nil
		}
		check, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_, e = s.storage.InspectRoot(check, l.Source.Path)
		if ctx.Err() != nil {
			return l, ctx.Err()
		}
		l.Source.Basis = "probe"
		l.Source.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
		l.Source.Availability = "available"
		if e != nil {
			l.Source.Availability = "unavailable"
			l.Source.Message = "Source could not be reached. Check storage access or its managed mount, then retry."
		}
	}
	// The library as one snapshot read it; the source probe above runs after
	// that snapshot is released, so it never holds one across I/O.
	return l, nil
}

// OrderLibraries stores the owner's order of the libraries: the listed ids first, in that
// order, and any library not listed after them in the order it already had. Every listed id
// must be a library; an id may appear once.
func (s *Service) OrderLibraries(ctx context.Context, ids []string, authorize func(*sql.Tx) error) error {
	if len(ids) == 0 || len(ids) > 1000 || authorize == nil {
		return ErrAdminQuery
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || len(id) > 128 || seen[id] {
			return ErrAdminQuery
		}
		seen[id] = true
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if e != nil {
		return e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if e = authorize(tx); e != nil {
		return e
	}
	rows, e := tx.QueryContext(ctx, `SELECT id FROM libraries ORDER BY position,name,id`)
	if e != nil {
		return e
	}
	current := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		current = append(current, id)
	}
	if e = rows.Close(); e != nil {
		return e
	}
	known := map[string]bool{}
	for _, id := range current {
		known[id] = true
	}
	for _, id := range ids {
		if !known[id] {
			return sql.ErrNoRows
		}
	}
	order := append([]string{}, ids...)
	for _, id := range current {
		if !seen[id] {
			order = append(order, id)
		}
	}
	for i, id := range order {
		if _, e = tx.ExecContext(ctx, `UPDATE libraries SET position=? WHERE id=? AND position<>?`, i+1, id, i+1); e != nil {
			return e
		}
	}
	return gated.Commit()
}

func (s *Service) RenameLibrary(ctx context.Context, id, name string, expected int64, authorize func(*sql.Tx) error) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 || !utf8.ValidString(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 || expected < 1 {
		return ErrAdminQuery
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if e != nil {
		return e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if authorize == nil {
		return ErrAdminQuery
	}
	if e = authorize(tx); e != nil {
		return e
	}
	var revision int64
	var current string
	if e = tx.QueryRowContext(ctx, `SELECT c.revision,l.name FROM library_configuration c JOIN libraries l ON l.id=c.library_id WHERE l.id=?`, id).Scan(&revision, &current); e != nil {
		return e
	}
	if revision != expected {
		return ErrLibraryConfigurationConflict
	}
	if current != name {
		if _, e = tx.ExecContext(ctx, `UPDATE libraries SET name=? WHERE id=?`, name, id); e != nil {
			return e
		}
	}
	return gated.Commit()
}
