package remotesources

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/storage"
)

// A source's own page reads these facts: what it is doing this minute, when
// Portico last checked it, and which libraries use its folders. Everything is
// something the server knows now; nothing is estimated or drawn ahead of time.
type SourceLibraryUse struct {
	LibraryID string `json:"libraryId"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Folders   int    `json:"folders"`
}

type SourceStatus struct {
	SourceID         string             `json:"sourceId"`
	Generation       string             `json:"generation"`
	State            string             `json:"state"`
	ErrorCode        string             `json:"errorCode,omitempty"`
	RangeSupport     string             `json:"rangeSupport"`
	LastCheckedAt    string             `json:"lastCheckedAt"`
	ConnectionsInUse int                `json:"connectionsInUse"`
	PlaybackReads    int                `json:"playbackReads"`
	AnalysisReads    int                `json:"analysisReads"`
	ScanIntervalSecs int                `json:"scanIntervalSeconds"`
	Libraries        []SourceLibraryUse `json:"libraries"`
}

// ReadSourceStatus is a bounded read of one source's present state. It lists
// no provider folder and opens no connection: the live counts are the reads the
// server itself is holding for this source.
func (s *Service) ReadSourceStatus(ctx context.Context, identities *identity.Service, owner identity.Principal, source string) (SourceStatus, error) {
	if s == nil || s.db == nil || source == "" {
		return SourceStatus{}, storage.ErrRemoteConfig
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return SourceStatus{}, err
	}
	defer snapshot.Rollback()
	if err = currentLocalOwnerTx(ctx, snapshot.Tx(), identities, owner); err != nil {
		return SourceStatus{}, err
	}
	row, err := sourceRowTx(ctx, snapshot.Tx(), source)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SourceStatus{}, storage.ErrRemoteOffline
		}
		return SourceStatus{}, err
	}
	out := SourceStatus{SourceID: row.ID, Generation: strconv.FormatInt(row.Generation, 10), State: row.State, ErrorCode: row.ErrorCode, RangeSupport: row.RangeSupport, Libraries: []SourceLibraryUse{}}
	if code := s.pacerCode(row.ID, out.Generation); code != "" && out.ErrorCode == "" {
		out.State, out.ErrorCode = code, code
	}
	out.PlaybackReads, out.AnalysisReads, out.ConnectionsInUse = s.liveReads(source)
	var observed int64
	if err = snapshot.Tx().QueryRowContext(ctx, `SELECT COALESCE(MAX(observed_ms),0) FROM remote_source_webdav_tests WHERE source_id=? AND source_generation=?`, row.ID, row.Generation).Scan(&observed); err != nil {
		return SourceStatus{}, err
	}
	if observed > 0 {
		out.LastCheckedAt = time.UnixMilli(observed).UTC().Format(time.RFC3339)
	}
	rows, err := snapshot.Tx().QueryContext(ctx, `SELECT l.id,l.name,l.kind,COUNT(*) FROM library_sources s JOIN libraries l ON l.id=s.library_id WHERE s.configured_root=? OR substr(s.configured_root,1,length(?)+1)=?||'/' GROUP BY l.id,l.name,l.kind ORDER BY l.name,l.id`, row.RootPath, row.RootPath, row.RootPath)
	if err != nil {
		return SourceStatus{}, err
	}
	for rows.Next() {
		var use SourceLibraryUse
		if err = rows.Scan(&use.LibraryID, &use.Name, &use.Kind, &use.Folders); err != nil {
			break
		}
		out.Libraries = append(out.Libraries, use)
	}
	readErr := rows.Err()
	rows.Close()
	if err != nil {
		return SourceStatus{}, err
	}
	if readErr != nil {
		return SourceStatus{}, readErr
	}
	if err = snapshot.Tx().QueryRowContext(ctx, `SELECT COALESCE(MIN(NULLIF(interval_seconds,0)),0) FROM library_sources WHERE configured_root=? OR substr(configured_root,1,length(?)+1)=?||'/'`, row.RootPath, row.RootPath, row.RootPath).Scan(&out.ScanIntervalSecs); err != nil {
		return SourceStatus{}, err
	}
	return out, nil
}

// liveReads counts the provider reads this server holds for one source now.
// Reads of one file share a provider stream, so connections never exceed reads.
func (s *Service) liveReads(id string) (playback, analysis, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, read := range s.active[id] {
		total++
		switch read.lane {
		case "playback":
			playback++
		case "analysis", "inventory":
			analysis++
		}
	}
	return playback, analysis, total
}

// pacerCode is the same provider backoff the list shows, so the page and the
// list never disagree about a source that is waiting.
func (s *Service) pacerCode(id, generation string) string {
	if s.sourcePacerBackoff(id, generation) {
		return "rate_limited"
	}
	return ""
}
