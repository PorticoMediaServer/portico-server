package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/personalstate"
)

var ErrRequestConflict = errors.New("download request idempotency key reused")
var errRequestUnavailable = errors.New("download request authorization not configured")

func (s *Service) SetRequestAccess(access RequestAccess) {
	s.accessMu.Lock()
	s.access = access
	s.accessMu.Unlock()
	s.signal()
}

// RequestAccess rechecks the session/device and supplies current visibility.
// Until the router installs it, background requests wait without failing.
type RequestAccess func(context.Context, *sql.Tx, identity.Principal, string, string) (string, []any, error)
type ContainerTarget struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type EpisodePolicy struct {
	Episodes string `json:"episodes"`
	KeepNext int    `json:"keepNext,omitempty"`
}
type ContainerRequest struct {
	OperationID string          `json:"operationId"`
	Target      ContainerTarget `json:"target"`
	DeviceID    string          `json:"deviceId"`
	Quality     string          `json:"quality"`
	Policy      EpisodePolicy   `json:"policy"`
}
type RequestView struct {
	RequestID  string          `json:"requestId"`
	Total      int             `json:"total"`
	TotalKnown bool            `json:"totalKnown"`
	State      string          `json:"state"`
	ErrorCode  string          `json:"errorCode,omitempty"`
	Ready      int             `json:"ready"`
	Preparing  int             `json:"preparing"`
	Queued     int             `json:"queued"`
	Failed     int             `json:"failed"`
	Paused     int             `json:"paused"`
	Items      []RequestMember `json:"items"`
	NextCursor string          `json:"nextCursor,omitempty"`
}
type RequestMember struct {
	ItemID      string       `json:"itemId"`
	Reason      string       `json:"reason,omitempty"`
	Preparation *Preparation `json:"preparation,omitempty"`
}

const requestBatchSize = 20

type requestBatchObserverKey struct{}

func observeRequestBatch(ctx context.Context, phase string, n int) {
	if f, ok := ctx.Value(requestBatchObserverKey{}).(func(string, int)); ok {
		f(phase, n)
	}
}
func (s *Service) requestAccess(ctx context.Context, tx *sql.Tx, p identity.Principal, device string) (string, []any, error) {
	s.accessMu.RLock()
	access := s.access
	s.accessMu.RUnlock()
	if access == nil {
		return "", nil, errRequestUnavailable
	}
	allowed, e := ProfileAllowsDownloads(tx, p.Viewer)
	if e != nil {
		return "", nil, e
	}
	if !allowed {
		return "", nil, ErrPolicy
	}
	return access(ctx, tx, p, device, "i.id")
}
func normalizeContainerRequest(r *ContainerRequest) error {
	if !validID.MatchString(r.OperationID) || !validID.MatchString(r.DeviceID) || !validID.MatchString(r.Target.ID) || preparationQuality(r.Quality) != nil {
		return ErrInput
	}
	switch r.Target.Kind {
	case "show", "season", "album", "book", "playlist":
	default:
		return ErrInput
	}
	switch r.Policy.Episodes {
	case "all", "unwatched":
		if r.Policy.KeepNext != 0 {
			return ErrInput
		}
	case "next":
		if r.Policy.KeepNext == 0 {
			r.Policy.KeepNext = 1
		}
		if r.Policy.KeepNext < 1 || r.Policy.KeepNext > 10000 {
			return ErrInput
		}
	default:
		return ErrInput
	}
	return nil
}

// The row is the single idempotency receipt; member rows are domain work, not receipts.
func (s *Service) SubmitContainer(ctx context.Context, p identity.Principal, r ContainerRequest) (RequestView, error) {
	out := RequestView{Items: []RequestMember{}}
	if e := normalizeContainerRequest(&r); e != nil {
		return out, e
	}
	raw, _ := json.Marshal(r)
	hash := identity.Digest(string(raw))
	id := identity.Token()
	e := dbwork.WithWriteTx(ctx, s.db, dbwork.ClassForegroundTransfer, func(tx *sql.Tx) error {
		visible, bound, e := s.requestAccess(ctx, tx, p, r.DeviceID)
		if e != nil {
			return e
		}
		var old string
		e = tx.QueryRowContext(ctx, `SELECT id,request_hash FROM download_requests WHERE profile_key=? AND operation_id=?`, operations.ViewerKey(p), r.OperationID).Scan(&id, &old)
		if e == nil {
			if old != hash {
				return ErrRequestConflict
			}
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		fence, e := requestFence(ctx, tx, p, r.Target, visible, bound)
		if e != nil {
			return e
		}
		query, values := requestSelection(r.Target)
		var anyVisible bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(`+query+` AND (`+visible+`))`, append(values, bound...)...).Scan(&anyVisible); e != nil {
			return e
		}
		if !anyVisible {
			return ErrNotFound
		}
		principal, _ := json.Marshal(p)
		var active int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM download_requests WHERE profile_key=? AND state IN('capturing','admitting')`, operations.ViewerKey(p)).Scan(&active); e != nil {
			return e
		}
		if active >= 100 {
			return ErrCapacity
		}
		target, e := requestTargetID(ctx, tx, r.Target)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO download_requests(id,profile_key,operation_id,request_hash,principal,device_id,target_kind,target_id,quality,episodes,keep_next,selection_fence,created_ms,updated_ms) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, operations.ViewerKey(p), r.OperationID, hash, string(principal), r.DeviceID, r.Target.Kind, target, r.Quality, r.Policy.Episodes, r.Policy.KeepNext, fence, s.millis(), s.millis())
		return e
	})
	if e != nil {
		return out, e
	}
	s.signal()
	return s.ContainerRequest(ctx, p, id, "", 100)
}

// requestTargetID resolves a container target's public id to the integer the
// download_requests row stores: a playlist's id, else the container's entity.
func requestTargetID(ctx context.Context, tx *sql.Tx, t ContainerTarget) (int64, error) {
	if t.Kind == "playlist" {
		var id int64
		e := tx.QueryRowContext(ctx, `SELECT id FROM catalog_playlists WHERE token=?`, t.ID).Scan(&id)
		if errors.Is(e, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		if e != nil {
			return 0, e
		}
		return id, nil
	}
	id, e := entityid.Resolve(ctx, tx, t.ID)
	if errors.Is(e, entityid.ErrNotFound) {
		return 0, ErrNotFound
	}
	return id, e
}

// requestTargetPublic inverts requestTargetID for a stored row: the playlist's
// token, else the container's public id.
func requestTargetPublic(ctx context.Context, tx *sql.Tx, kind string, id int64) (string, error) {
	if kind == "playlist" {
		var token string
		e := tx.QueryRowContext(ctx, `SELECT token FROM catalog_playlists WHERE id=?`, id).Scan(&token)
		if errors.Is(e, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		if e != nil {
			return "", e
		}
		return token, nil
	}
	public, e := entityid.Public(ctx, tx, id)
	if errors.Is(e, entityid.ErrNotFound) {
		return "", ErrNotFound
	}
	return public, e
}

// Fence only revisions and resource authority, never a materialized member list.
func requestFence(ctx context.Context, tx *sql.Tx, p identity.Principal, t ContainerTarget, visible string, args []any) (string, error) {
	parts := []any{visible, args}
	libraryWhere := ""
	libraries := []any{}
	if t.Kind == "playlist" {
		var revision int64
		var allowed bool
		e := tx.QueryRowContext(ctx, `SELECT revision,((owner_authority=? AND owner_account=? AND owner_profile=?) OR EXISTS(SELECT 1 FROM playlist_shares WHERE playlist_id=catalog_playlists.token AND authority=? AND account_id=? AND profile_id=?)) FROM catalog_playlists WHERE token=? AND deleted=0`, p.Authority, p.AccountID, p.ProfileID, p.Authority, p.AccountID, p.ProfileID, t.ID).Scan(&revision, &allowed)
		if errors.Is(e, sql.ErrNoRows) || e == nil && !allowed {
			return "", ErrNotFound
		}
		if e != nil {
			return "", e
		}
		parts = append(parts, revision)
		libraryWhere = `l.library_id IN(SELECT cl.library_id FROM catalog_playlists playlist JOIN catalog_playlist_entries member ON member.playlist_id=playlist.id JOIN catalog_entities i ON i.id=member.item_id JOIN catalog_libraries cl ON cl.id=i.library_id WHERE playlist.token=? AND (` + visible + `))`
		libraries = append([]any{t.ID}, args...)
	} else {
		var library string
		kind := map[string]int{"show": 2, "season": 3, "album": 6, "book": 8}[t.Kind]
		e := tx.QueryRowContext(ctx, `SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.public_id=pid_blob(?) AND e.kind=? LIMIT 1`, t.ID, kind).Scan(&library)
		if errors.Is(e, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		if e != nil {
			return "", e
		}
		libraryWhere = `l.library_id=?`
		libraries = []any{library}
	}
	rows, e := tx.QueryContext(ctx, `SELECT l.library_id,l.revision,COALESCE(v.revision,0) FROM library_revisions l LEFT JOIN viewer_revisions v ON v.library_id=l.library_id AND v.profile_id=? WHERE `+libraryWhere+` ORDER BY l.library_id`, append([]any{identity.PersonalKey(p.Viewer)}, libraries...)...)
	if e != nil {
		return "", e
	}
	defer rows.Close()
	for rows.Next() {
		var lib string
		var cat, personal int64
		if e = rows.Scan(&lib, &cat, &personal); e != nil {
			return "", e
		}
		parts = append(parts, []any{lib, cat, personal})
	}
	if e = rows.Err(); e != nil {
		return "", e
	}
	raw, _ := json.Marshal(parts)
	return identity.Digest(string(raw)), nil
}
func requestSelection(t ContainerTarget) (string, []any) {
	// Every path starts at a container index, never at the whole catalogue.
	switch t.Kind {
	case "show", "season":
		field := "show.public_id=pid_blob(?)"
		if t.Kind == "season" {
			field = "season.public_id=pid_blob(?)"
		}
		return `SELECT pid(i.public_id) id,printf('%020d:%020d:',COALESCE(s.number,0),CASE WHEN e.numbering='date' THEN e.air_date ELSE e.number END)||pid(i.public_id) AS position FROM catalog_episodes e JOIN catalog_entities show ON show.id=e.show_id LEFT JOIN catalog_entities season ON season.id=e.season_id LEFT JOIN catalog_seasons s ON s.entity_id=season.id JOIN catalog_entities i ON i.id=e.entity_id WHERE ` + field, []any{t.ID}
	case "album":
		return `SELECT pid(i.public_id) id,printf('%020d:%020d:',COALESCE(s.disc_number,0),COALESCE(s.track_number,0))||pid(i.public_id) AS position FROM catalog_entities album JOIN catalog_songs s ON s.album_id=album.id JOIN catalog_entities i ON i.id=s.entity_id WHERE album.public_id=pid_blob(?)`, []any{t.ID}
	case "book":
		return `SELECT pid(i.public_id) id,printf('%020d:%020d:',COALESCE(b.disc_number,0),COALESCE(b.part_number,0))||pid(i.public_id) AS position FROM catalog_entities book JOIN catalog_book_files b ON b.book_id=book.id JOIN catalog_entities i ON i.id=b.entity_id WHERE book.public_id=pid_blob(?)`, []any{t.ID}
	default:
		return `SELECT pid(i.public_id) id,p.order_key||'!'||p.token AS position FROM catalog_playlists playlist JOIN catalog_playlist_entries p ON p.playlist_id=playlist.id JOIN catalog_entities i ON i.id=p.item_id WHERE playlist.token=? AND p.id=(SELECT first.id FROM catalog_playlist_entries first WHERE first.playlist_id=p.playlist_id AND first.item_id=p.item_id ORDER BY first.order_key,first.token LIMIT 1)`, []any{t.ID}
	}
}
func (s *Service) advanceRequests(ctx context.Context) error {
	rows, e := s.db.QueryContext(ctx, `SELECT id FROM download_requests WHERE state IN('capturing','admitting') ORDER BY updated_ms,id LIMIT 4`)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		if e = s.advanceRequest(ctx, id); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) advanceRequest(ctx context.Context, id string) error {
	return dbwork.WithWriteTx(ctx, s.db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
		var raw, device, quality, state, fence, key, after, policy string
		var target ContainerTarget
		var targetEntity int64
		var total, admitted, keep int
		e := tx.QueryRowContext(ctx, `SELECT principal,device_id,target_kind,target_id,quality,episodes,keep_next,state,selection_fence,cursor_key,cursor_id,total,admitted FROM download_requests WHERE id=?`, id).Scan(&raw, &device, &target.Kind, &targetEntity, &quality, &policy, &keep, &state, &fence, &key, &after, &total, &admitted)
		if e != nil {
			return e
		}
		if state != "capturing" && state != "admitting" {
			return nil
		}
		var p identity.Principal
		if e = json.Unmarshal([]byte(raw), &p); e != nil {
			return e
		}
		fail := func(code string) error {
			_, e := tx.ExecContext(ctx, `UPDATE download_requests SET state='failed',error_code=?,updated_ms=? WHERE id=?`, code, s.millis(), id)
			return e
		}
		target.ID, e = requestTargetPublic(ctx, tx, target.Kind, targetEntity)
		if errors.Is(e, ErrNotFound) {
			return fail("target_not_found")
		}
		if e != nil {
			return e
		}
		visible, args, e := s.requestAccess(ctx, tx, p, device)
		if errors.Is(e, identity.ErrUnauthorized) || errors.Is(e, identity.ErrContentRestricted) || errors.Is(e, ErrPolicy) {
			return fail("authority_revoked")
		}
		if e != nil {
			return e
		}
		current, e := requestFence(ctx, tx, p, target, visible, args)
		if errors.Is(e, ErrNotFound) {
			return fail("target_not_found")
		}
		if e != nil {
			return e
		}
		if state == "capturing" {
			if current != fence {
				return fail("selection_changed")
			}
			query, bound := requestSelection(target)
			query += ` AND (` + visible + `) AND NOT EXISTS(SELECT 1 FROM dvr_catalog_retirements d WHERE d.item_id=i.id)`
			bound = append(bound, args...)
			if policy != "all" {
				query += ` AND NOT (` + personalstate.SQL("?", "i.id") + `)`
				bound = append(bound, identity.PersonalKey(p.Viewer))
			}
			limit := requestBatchSize
			if policy == "next" {
				limit = min(limit, keep-total)
			}
			rows, e := tx.QueryContext(ctx, `SELECT id,position FROM (`+query+`) WHERE (position,id)>(?,?) ORDER BY position,id LIMIT ?`, append(bound, key, after, limit)...)
			if e != nil {
				return e
			}
			type member struct{ id, key string }
			members := []member{}
			for rows.Next() {
				var m member
				if e = rows.Scan(&m.id, &m.key); e != nil {
					rows.Close()
					return e
				}
				members = append(members, m)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			observeRequestBatch(ctx, "capture", len(members))
			for _, m := range members {
				total++
				entity, e := entityid.Resolve(ctx, tx, m.id)
				if errors.Is(e, entityid.ErrNotFound) {
					return fail("target_not_found")
				}
				if e != nil {
					return e
				}
				if _, e = tx.ExecContext(ctx, `INSERT INTO download_request_items(request_id,ordinal,item_id) VALUES(?,?,?)`, id, total, entity); e != nil {
					return e
				}
				key, after = m.key, m.id
			}
			if len(members) < limit || policy == "next" && total >= keep {
				state = "admitting"
			}
			_, e = tx.ExecContext(ctx, `UPDATE download_requests SET state=?,cursor_key=?,cursor_id=?,total=?,updated_ms=? WHERE id=?`, state, key, after, total, s.millis(), id)
			return e
		}
		rows, e := tx.QueryContext(ctx, `SELECT ordinal,COALESCE((SELECT pid(public_id) FROM catalog_entities WHERE id=download_request_items.item_id),'') FROM download_request_items WHERE request_id=? AND ordinal>? ORDER BY ordinal LIMIT ?`, id, admitted, requestBatchSize)
		if e != nil {
			return e
		}
		type member struct {
			n  int
			id string
		}
		members := []member{}
		for rows.Next() {
			var m member
			if e = rows.Scan(&m.n, &m.id); e != nil {
				rows.Close()
				return e
			}
			members = append(members, m)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		observeRequestBatch(ctx, "admit", len(members))
		for _, m := range members {
			var allowed bool
			if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_entities i WHERE i.public_id=pid_blob(?) AND (`+visible+`))`, append([]any{m.id}, args...)...).Scan(&allowed); e != nil {
				return e
			}
			preparation, reason := "", ""
			if !allowed {
				reason = ReasonItemDeleted
			} else {
				preparation, reason, e = s.admitRequestItem(ctx, tx, p, device, id, m.id, quality)
				if e != nil {
					return e
				}
			}
			if _, e = tx.ExecContext(ctx, `UPDATE download_request_items SET preparation_id=?,reason=? WHERE request_id=? AND ordinal=?`, preparation, reason, id, m.n); e != nil {
				return e
			}
			admitted = m.n
		}
		if admitted == total {
			state = "complete"
		}
		_, e = tx.ExecContext(ctx, `UPDATE download_requests SET state=?,admitted=?,updated_ms=? WHERE id=?`, state, admitted, s.millis(), id)
		return e
	})
}
func (s *Service) admitRequestItem(ctx context.Context, tx *sql.Tx, p identity.Principal, device, batch, item, quality string) (string, string, error) {
	entity, e := entityid.Resolve(ctx, tx, item)
	if errors.Is(e, entityid.ErrNotFound) {
		return "", ReasonItemDeleted, nil
	}
	if e != nil {
		return "", "", e
	}
	var id string
	e = tx.QueryRowContext(ctx, `SELECT id FROM download_preparations WHERE profile_key=? AND item_id=? AND quality=? AND state IN('queued','running','ready','paused','failed')`, operations.ViewerKey(p), entity, quality).Scan(&id)
	if e == nil {
		return id, "", nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return "", "", e
	}
	source, e := resolveSource(ctx, tx, item)
	if reason := sourceReason(source, e); reason != "" {
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return "", "", e
		}
		return "", reason, nil
	}
	estimate, estimated := estimateBytes(quality, source)
	if e = storageRoom(tx, estimate); errors.Is(e, ErrStorageFull) {
		return "", ReasonStorageFull, nil
	}
	if e != nil {
		return "", "", e
	}
	id = identity.Token()
	_, e = tx.ExecContext(ctx, `INSERT INTO download_preparations(id,profile_key,authority,account_id,profile_id,item_id,library_id,quality,origin,batch_id,state,bytes_total,estimated,created_ms,updated_ms) VALUES(?,?,?,?,?,?,?,?,'container',?,'queued',?,?,?,?)`, id, operations.ViewerKey(p), p.Authority, p.AccountID, p.ProfileID, entity, source.Library, quality, batch, estimate, estimated, s.millis(), s.millis())
	if e != nil {
		return "", "", e
	}
	raw, _ := json.Marshal(p)
	_, e = tx.ExecContext(ctx, `INSERT INTO download_preparation_authority(preparation_id,principal,device_id) VALUES(?,?,?)`, id, string(raw), device)
	return id, "", e
}

// Counts and members are filtered by today's visibility, not a stored unrestricted total.
func (s *Service) ContainerRequest(ctx context.Context, p identity.Principal, id, cursor string, limit int) (RequestView, error) {
	out := RequestView{RequestID: id, Items: []RequestMember{}}
	if !validID.MatchString(id) || limit < 1 || limit > 100 {
		return out, ErrInput
	}
	after := 0
	if cursor != "" && !validID.MatchString(cursor) {
		return out, ErrInput
	}
	snap, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return out, e
	}
	defer snap.Rollback()
	tx := snap.Tx()
	var device string
	var total, admitted int
	e = tx.QueryRowContext(ctx, `SELECT device_id,state,error_code,total,admitted FROM download_requests WHERE id=? AND profile_key=?`, id, operations.ViewerKey(p)).Scan(&device, &out.State, &out.ErrorCode, &total, &admitted)
	if errors.Is(e, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	// Return a visible member ID rather than an ordinal that exposes hidden gaps.
	if cursor != "" {
		cursorEntity, e := entityid.Resolve(ctx, tx, cursor)
		if errors.Is(e, entityid.ErrNotFound) {
			return out, ErrInput
		}
		if e != nil {
			return out, e
		}
		e = tx.QueryRowContext(ctx, `SELECT ordinal FROM download_request_items WHERE request_id=? AND item_id=?`, id, cursorEntity).Scan(&after)
		if errors.Is(e, sql.ErrNoRows) {
			return out, ErrInput
		}
		if e != nil {
			return out, e
		}
	}
	// Other devices of this profile may inspect it, but not borrow the source device's authority.
	visible, args, e := s.requestAccess(ctx, tx, p, "")
	if e != nil {
		return out, e
	}
	out.TotalKnown = out.State != "capturing" && (out.State != "failed" || admitted > 0)
	from := ` FROM download_request_items m JOIN catalog_entities i ON i.id=m.item_id LEFT JOIN download_preparations d ON d.id=m.preparation_id WHERE m.request_id=? AND (` + visible + `)`
	bound := append([]any{id}, args...)
	rows, e := tx.QueryContext(ctx, `SELECT CASE WHEN m.reason<>'' OR (m.preparation_id='' AND EXISTS(SELECT 1 FROM download_requests r WHERE r.id=m.request_id AND r.state='failed')) THEN 'failed' ELSE COALESCE(d.state,'queued') END,count(*)`+from+` GROUP BY 1`, bound...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var state string
		var n int
		if e = rows.Scan(&state, &n); e != nil {
			rows.Close()
			return out, e
		}
		out.Total += n
		switch state {
		case StateReady:
			out.Ready += n
		case StateRunning:
			out.Preparing += n
		case StateQueued:
			out.Queued += n
		case StatePaused:
			out.Paused += n
		default:
			out.Failed += n
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.QueryContext(ctx, `SELECT m.ordinal,pid(i.public_id),m.preparation_id,m.reason`+from+` AND m.ordinal>? ORDER BY m.ordinal LIMIT ?`, append(bound, after, limit+1)...)
	if e != nil {
		return out, e
	}
	type row struct {
		n                  int
		item, prep, reason string
	}
	members := []row{}
	for rows.Next() {
		var m row
		if e = rows.Scan(&m.n, &m.item, &m.prep, &m.reason); e != nil {
			rows.Close()
			return out, e
		}
		members = append(members, m)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(members) > limit {
		members = members[:limit]
		out.NextCursor = members[limit-1].item
	}
	settings, e := readSettings(tx)
	if e != nil {
		return out, e
	}
	for _, m := range members {
		entry := RequestMember{ItemID: m.item, Reason: m.reason}
		if m.prep == "" && out.State == "failed" {
			entry.Reason = out.ErrorCode
		}
		if m.prep != "" {
			r, e := readPreparation(tx.QueryRowContext(ctx, `SELECT `+preparationColumns+` FROM download_preparations WHERE id=? AND profile_key=?`, m.prep, operations.ViewerKey(p)))
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return out, e
			}
			if e == nil {
				prep := s.publish(r, settings.RetentionDays)
				entry.Preparation = &prep
			} else {
				entry.Reason = ReasonRemoved
			}
		}
		out.Items = append(out.Items, entry)
	}
	if out.State == "complete" && out.Queued+out.Preparing+out.Paused > 0 {
		out.State = "preparing"
	}
	return out, snap.Commit()
}
