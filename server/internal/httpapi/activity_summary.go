package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/administration"
	"portico.local/server/internal/buildinfo"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/playbackv1"
)

type ActivitySummary struct {
	Scope              playback.ActivityScope      `json:"scope"`
	ObservedAt         string                      `json:"observedAt"`
	ServerName         string                      `json:"serverName"`
	Streams            playbackv1.ActivitySessions `json:"streams"`
	Scans              []ActivitySummaryScan       `json:"scans"`
	Tasks              ActivitySummaryTasks        `json:"tasks"`
	Update             ActivitySummaryUpdate       `json:"update"`
	Health             ActivitySummaryHealth       `json:"health"`
	LastScanFinishedAt string                      `json:"lastScanFinishedAt,omitempty"`
}
type ActivitySummaryScan struct {
	LibraryID   string `json:"libraryId"`
	LibraryName string `json:"libraryName"`
	Kind        string `json:"kind"`
	Discovered  int64  `json:"discovered"`
	Analyzed    int64  `json:"analyzed"`
	Total       *int64 `json:"total,omitempty"`
	Current     string `json:"current,omitempty"`
	Phase       string `json:"phase"`
}
type ActivitySummaryTasks struct {
	Count int64                 `json:"count"`
	Items []ActivitySummaryTask `json:"items"`
}
type ActivitySummaryTask struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	LibraryID   string `json:"libraryId,omitempty"`
	LibraryName string `json:"libraryName,omitempty"`
	State       string `json:"state"`
	Phase       string `json:"phase"`
	Processed   *int64 `json:"processed,omitempty"`
	Total       *int64 `json:"total,omitempty"`
	Unit        string `json:"unit,omitempty"`
}
type ActivitySummaryUpdate struct {
	Available bool   `json:"available"`
	State     string `json:"state"`
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
}
type ActivitySummaryHealth struct {
	Degraded bool `json:"degraded"`
}

func registerActivitySummary(registry *apikit.Registry, d Dependencies) error {
	return apikit.Register(registry, apikit.Route[struct{}, ActivitySummary]{Metadata: apikit.Metadata{Admission: laneAdmin, ID: "get_admin_activity", Method: "GET", Path: "/v1/admin/activity", Summary: "Read the owner's bounded server activity summary", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Cost: apikit.PageSized, Status: 200, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "internal"}}, Handler: func(ctx context.Context, r *http.Request, _ struct{}) (ActivitySummary, error) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		p, e := d.ownerContext(ctx, r)
		if e != nil {
			return ActivitySummary{}, foundationAuthError(e)
		}
		if r.URL.RawQuery != "" {
			return ActivitySummary{}, &apierror.Error{Code: "invalid_request"}
		}
		out, e := d.readActivitySummary(ctx, p)
		if e != nil {
			return ActivitySummary{}, foundationAuthError(e)
		}
		if _, e = d.ownerContext(ctx, r); e != nil {
			return ActivitySummary{}, foundationAuthError(e)
		}
		return out, nil
	}})
}
func (d Dependencies) readActivitySummary(ctx context.Context, p identity.Principal) (ActivitySummary, error) {
	out := ActivitySummary{ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ServerName: d.Identity.Name(), Scans: []ActivitySummaryScan{}, Tasks: ActivitySummaryTasks{Items: []ActivitySummaryTask{}}}
	out.Scope = playback.ActivityScope{ServerID: d.Identity.ID(), ViewerFence: fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d", presentationIdentity(p), p.Epoch))))}
	service := d.v1
	if service == nil {
		service = &playbackv1.Service{DB: d.DB}
	}
	var e error
	if out.Streams, e = service.ActivitySummarySessions(ctx); e != nil {
		return out, e
	}
	gate, e := dbwork.BeginSnapshot(ctx, d.DB)
	if e != nil {
		return out, e
	}
	defer gate.Rollback()
	tx := gate.Tx()
	if e = d.ownerAuthorityTx(ctx, tx, p); e != nil {
		return out, e
	}
	if e = d.activitySummaryScans(ctx, tx, &out); e != nil {
		return out, e
	}
	if e = d.activitySummaryTasks(ctx, tx, &out); e != nil {
		return out, e
	}
	if e = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX((SELECT r.finished_at FROM inventory_runs r WHERE r.source_id=s.id AND r.finished_at<>'' ORDER BY r.finished_at DESC LIMIT 1)),'') FROM library_sources s`).Scan(&out.LastScanFinishedAt); e != nil {
		return out, e
	}
	if e = gate.Commit(); e != nil {
		return out, e
	}
	info := buildinfo.Info()
	current := administration.UpdateBuild{Version: info["version"], BuildID: info["buildId"], SourceDigest: info["sourceDigest"], BuiltAt: info["builtAt"]}
	out.Update = ActivitySummaryUpdate{State: "unconfigured", Current: current.Version}
	if d.Administration != nil {
		channel, feed := d.updatePolicy(ctx, p)
		u := d.Administration.CachedUpdates(ctx, current, channel, feed)
		out.Update = ActivitySummaryUpdate{Available: u.UpdateAvailable, State: u.State, Current: u.Current.Version}
		if u.Latest != nil {
			out.Update.Latest = u.Latest.Version
		}
	}
	if d.Watchdog != nil {
		out.Health.Degraded = d.Watchdog.Report().State != dbwork.HealthyState
	}
	return out, nil
}
func (d Dependencies) activitySummaryScans(ctx context.Context, tx *sql.Tx, out *ActivitySummary) error {
	rows, e := tx.QueryContext(ctx, `SELECT l.id,l.name,l.kind,r.job_id,r.phase,r.discovered,r.analyzed,r.inventory_completed_at
 FROM inventory_source_active a CROSS JOIN inventory_runs r ON r.job_id=a.job_id CROSS JOIN jobs j ON j.id=r.job_id AND j.status IN('queued','running') CROSS JOIN library_sources s ON s.id=a.source_id CROSS JOIN libraries l ON l.id=s.library_id ORDER BY l.id,r.job_id`)
	if e != nil {
		return e
	}
	defer rows.Close()
	indexes := map[string]int{}
	unknown := map[string]bool{}
	for rows.Next() {
		var id, name, kind, job, phase, complete string
		var discovered, analyzed int64
		if e = rows.Scan(&id, &name, &kind, &job, &phase, &discovered, &analyzed, &complete); e != nil {
			return e
		}
		n, ok := indexes[id]
		if !ok {
			n = len(out.Scans)
			indexes[id] = n
			total := int64(0)
			out.Scans = append(out.Scans, ActivitySummaryScan{LibraryID: id, LibraryName: name, Kind: kind, Phase: phase, Total: &total})
		}
		scan := &out.Scans[n]
		scan.Discovered += discovered
		scan.Analyzed += analyzed
		if complete == "" {
			unknown[id] = true
			scan.Phase = phase
		}
		if scan.Total != nil {
			*scan.Total += discovered
		}
		if d.Ingestion != nil {
			if file := d.Ingestion.CurrentActivityFile(job); file != "" && scan.Current == "" {
				scan.Current = file
			}
		}
	}
	for n := range out.Scans {
		if unknown[out.Scans[n].LibraryID] {
			out.Scans[n].Total = nil
		}
	}
	return rows.Err()
}

// Only work queues are read here, never a library/inventory walk. Source joins
// start at the active operations/stages; item enrichment stays capped at three.
const activityTasksCTE = `WITH tasks AS (
 SELECT c.id,c.kind,COALESCE(l.id,'') library_id,COALESCE(l.name,'') library_name,c.state,c.phase,
 CASE WHEN p.id IS NOT NULL THEN p.done+p.failed ELSE c.processed END processed,
 CASE WHEN p.selection_complete=1 THEN p.total END total,CASE WHEN p.id IS NOT NULL THEN 'items' ELSE '' END unit,c.created_ms ordering
 FROM console_operations c LEFT JOIN libraries l ON l.id=c.resource LEFT JOIN personal_jobs p ON p.id=c.resource
 WHERE c.state IN('queued','running','reconciling')
 UNION ALL
 SELECT 'analysis:'||a.job_id||':'||a.object_id||':'||a.stage,a.stage,l.id,l.name,'running',a.stage,a.processed_bytes,
 CASE WHEN a.stage='checksum' THEN o.size END,'bytes',a.updated_ms
 FROM inventory_source_active active CROSS JOIN inventory_runs r ON r.job_id=active.job_id
 CROSS JOIN jobs j ON j.id=r.job_id AND j.status='running'
 CROSS JOIN analysis_stage_runs a ON a.job_id=r.job_id AND a.state='running' AND a.policy_revision=r.policy_revision
 CROSS JOIN inventory_objects o ON o.id=a.object_id AND o.revision=a.source_revision
 CROSS JOIN library_sources s ON s.id=r.source_id CROSS JOIN libraries l ON l.id=s.library_id
 UNION ALL
 SELECT 'segments:'||f.library_id,'segments',f.library_id,COALESCE(l.name,''),'queued','segment_evidence',NULL,NULL,'files',MIN(f.updated_ms)
 FROM segment_files f LEFT JOIN libraries l ON l.id=f.library_id WHERE f.state='pending' GROUP BY f.library_id
 ) `

func (d Dependencies) activitySummaryTasks(ctx context.Context, tx *sql.Tx, out *ActivitySummary) error {
	if e := tx.QueryRowContext(ctx, activityTasksCTE+`SELECT count(*) FROM tasks`).Scan(&out.Tasks.Count); e != nil {
		return e
	}
	rows, e := tx.QueryContext(ctx, activityTasksCTE+`SELECT id,kind,library_id,library_name,state,phase,processed,total,unit FROM tasks ORDER BY ordering,id LIMIT 3`)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var v ActivitySummaryTask
		if e = rows.Scan(&v.ID, &v.Kind, &v.LibraryID, &v.LibraryName, &v.State, &v.Phase, &v.Processed, &v.Total, &v.Unit); e != nil {
			return e
		}
		out.Tasks.Items = append(out.Tasks.Items, v)
	}
	return rows.Err()
}
