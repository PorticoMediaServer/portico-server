package metadata

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/metadataprovider"
)

type AttentionCandidate struct {
	Key        string                         `json:"key"`
	Identity   metadataprovider.ScreenID      `json:"identity"`
	Title      string                         `json:"title"`
	Year       int                            `json:"year"`
	Confidence float64                        `json:"confidence"`
	Reasons    []string                       `json:"reasons"`
	Orders     []metadataprovider.ScreenOrder `json:"orders"`
}
type AttentionEntry struct {
	LibraryID  string               `json:"libraryId"`
	ItemKind   string               `json:"itemKind"`
	ParentID   string               `json:"parentId"`
	Actions    []string             `json:"actions"`
	EntityID   string               `json:"entityId"`
	Kind       string               `json:"kind"`
	Title      string               `json:"title"`
	Revision   int64                `json:"revision"`
	Reason     string               `json:"reason"`
	Confidence float64              `json:"confidence"`
	Candidates []AttentionCandidate `json:"candidates"`
}
type AttentionPage struct {
	Total      int64            `json:"total"`
	LibraryID  string           `json:"libraryId"`
	Entries    []AttentionEntry `json:"entries"`
	NextCursor string           `json:"nextCursor"`
}

// Attention reads one indexed page, then at most three candidates per entry.
func (s *Service) Attention(ctx context.Context, library, cursor string, limit int) (AttentionPage, error) {
	return s.AttentionFiltered(ctx, library, cursor, limit, true)
}

func (s *Service) AttentionFiltered(ctx context.Context, library, cursor string, limit int, includeInProgress bool) (AttentionPage, error) {
	out := AttentionPage{LibraryID: library, Entries: []AttentionEntry{}}
	var after int64
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(raw) > 64 {
			return out, ErrRepairInput
		}
		after, err = strconv.ParseInt(string(raw), 10, 64)
		if err != nil || after < 1 {
			return out, ErrRepairInput
		}
	}
	if limit < 1 || limit > 50 {
		return out, ErrRepairInput
	}
	read, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return out, err
	}
	defer read.Rollback()
	tx := read.Tx()
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM libraries WHERE id=?)`, library).Scan(&exists); err != nil {
		return out, err
	}
	if library != "" && !exists {
		return out, sql.ErrNoRows
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM screen_metadata_attention a JOIN catalog_entities e ON e.id=a.entity_id JOIN catalog_libraries cl ON cl.id=e.library_id AND cl.retired=0 WHERE e.retired=0 AND (?='' OR a.library_id=?) AND a.dismissed_generation<>a.generation AND a.reason NOT IN('pending','searching','pending_search','pending_episodes','pending_children','pending_apply')`, library, library).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.library_id,a.entity_id,a.target_kind,e.title,w.generation,a.reason,a.confidence,k.name,COALESCE(pid(parent.public_id),''),p.enabled,c.confirmed FROM screen_metadata_attention a JOIN catalog_entities e ON e.id=a.entity_id JOIN screen_metadata_work w ON w.target_kind=a.target_kind AND w.target_id=a.entity_id
 JOIN catalog_kinds k ON k.id=e.kind LEFT JOIN catalog_entities parent ON parent.id=w.parent_id
 JOIN screen_metadata_policies p ON p.library_id=a.library_id CROSS JOIN screen_metadata_consent c
 JOIN catalog_libraries cl ON cl.id=e.library_id AND cl.retired=0
 WHERE e.retired=0 AND c.singleton=1 AND (?='' OR a.library_id=?) AND a.entity_id>? AND a.dismissed_generation<>a.generation AND (? OR a.reason NOT IN('pending','searching','pending_search','pending_episodes','pending_children','pending_apply')) ORDER BY a.entity_id LIMIT ?`, library, library, after, includeInProgress, limit+1)
	if err != nil {
		return out, err
	}
	ids := []int64{}
	for rows.Next() {
		var e AttentionEntry
		var id int64
		var enabled, confirmed bool
		if err = rows.Scan(&e.LibraryID, &id, &e.Kind, &e.Title, &e.Revision, &e.Reason, &e.Confidence, &e.ItemKind, &e.ParentID, &enabled, &confirmed); err != nil {
			rows.Close()
			return out, err
		}
		if len(out.Entries) == limit {
			out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(ids[len(ids)-1], 10)))
			break
		}
		e.Actions = []string{"dismiss"}
		if e.ItemKind == "episode" {
			if e.ParentID != "" {
				e.Actions = append(e.Actions, "parent")
			}
		} else if enabled && confirmed {
			e.Actions = append(e.Actions, "accept", "search", "id")
		}
		e.Candidates = []AttentionCandidate{}
		out.Entries = append(out.Entries, e)
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for i, id := range ids {
		out.Entries[i].EntityID = entityPublic(ctx, tx, id)
		rows, err = tx.QueryContext(ctx, `SELECT candidate_key,payload,confidence,reasons FROM screen_metadata_candidates WHERE target_kind=? AND target_id=? ORDER BY confidence DESC,candidate_key LIMIT 3`, out.Entries[i].Kind, id)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var c AttentionCandidate
			var payload, reasons string
			var r metadataprovider.ScreenRecord
			if err = rows.Scan(&c.Key, &payload, &c.Confidence, &reasons); err != nil {
				rows.Close()
				return out, err
			}
			if err = json.Unmarshal([]byte(payload), &r); err != nil {
				rows.Close()
				return out, err
			}
			if err = json.Unmarshal([]byte(reasons), &c.Reasons); err != nil {
				rows.Close()
				return out, err
			}
			c.Identity = r.Identity
			c.Title = r.Title
			c.Year = r.Year
			c.Orders = screenRecordOrders(r)
			out.Entries[i].Candidates = append(out.Entries[i].Candidates, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	for i := range out.Entries {
		if len(out.Entries[i].Candidates) == 0 {
			actions := []string{}
			for _, action := range out.Entries[i].Actions {
				if action != "accept" {
					actions = append(actions, action)
				}
			}
			out.Entries[i].Actions = actions
		}
	}
	return out, nil
}

type AttentionCommand struct {
	Kind             string `json:"kind"`
	EntityID         string `json:"entityId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	Action           string `json:"action"`
	CandidateKey     string `json:"candidateKey,omitempty"`
	Provider         string `json:"provider,omitempty"`
	ProviderID       string `json:"providerId,omitempty"`
	Query            string `json:"query,omitempty"`
	Order            string `json:"order,omitempty"`
}

func (s *Service) AttentionAction(ctx context.Context, library string, in AttentionCommand, actor MBActor, authorize func(*sql.Tx) error) error {
	if (in.Kind != "item" && in.Kind != "show") || in.ExpectedRevision < 1 {
		return ErrRepairInput
	}
	// Bind all commands to the listed library, even when a caller supplies another ID.
	var found string
	// Resolve through the public-id codec inside a snapshot, never compare public text to an integer.
	read, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return e
	}
	entity, e := resolveEntity(ctx, read.Tx(), in.EntityID)
	if e == nil {
		e = read.Tx().QueryRowContext(ctx, `SELECT library_id FROM screen_metadata_work WHERE target_kind=? AND target_id=?`, in.Kind, entity).Scan(&found)
	}
	read.Rollback()
	if e != nil {
		return e
	}
	if found != library {
		return ErrRepairInput
	}
	if in.Action != "dismiss" {
		var kind string
		if e = dbwork.QueryRow(ctx, s.db, `SELECT k.name FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind WHERE e.id=?`, entity).Scan(&kind); e != nil {
			return e
		}
		if kind == "episode" {
			return ErrRepairInput
		}
	}
	switch in.Action {
	case "accept", "search":
		return s.attentionObservedAction(ctx, library, in, actor, authorize)
	case "id":
		if hint, ok := metadataprovider.ParseScreenHint(in.ProviderID, "movie"); ok && hint.Provider == "imdb" {
			in.Action = "search"
			in.Query = "imdb:" + hint.ID
			return s.attentionObservedAction(ctx, library, in, actor, authorize)
		}
	case "dismiss":
	default:
		return ErrRepairInput
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassInteractive)
	if e != nil {
		return e
	}
	defer gated.Rollback()
	tx := gated.Tx()
	if authorize != nil {
		if e = authorize(tx); e != nil {
			return e
		}
	}
	b, e := readScreenBase(ctx, tx, in.Kind, in.EntityID)
	if e != nil {
		return e
	}
	if b.Library != library {
		return ErrRepairInput
	}
	if b.Generation != in.ExpectedRevision {
		return ErrScreenConflict
	}
	if in.Action == "dismiss" {
		_, e = tx.ExecContext(ctx, `UPDATE screen_metadata_attention SET dismissed_generation=generation WHERE entity_id=?`, b.Entity)
	} else {
		if b.ItemKind == "episode" {
			return ErrRepairInput
		}
		typ := b.ItemKind
		if in.Provider == "anilist" {
			typ = "anime"
		}
		id := metadataprovider.ScreenID{Provider: in.Provider, Type: typ, ID: in.ProviderID}
		if hint, ok := metadataprovider.ParseScreenHint(in.ProviderID, typ); ok {
			if in.Provider != "" && hint.Provider != in.Provider {
				return ErrRepairInput
			}
			id = hint
		}
		if !metadataprovider.ValidScreenID(id) {
			return ErrRepairInput
		}
		t := RepairTarget{in.Kind, in.EntityID}
		before, ent, e := readRepairSnapshot(ctx, tx, t)
		if e != nil {
			return e
		}
		revision, e := repairRevision(ctx, tx, t, before, ent)
		if e != nil {
			return e
		}
		order := in.Order
		if order == "" {
			order = "official"
		}
		if in.Provider == "anilist" && order == "official" {
			order = "seasonal"
		}
		if e = setScreenOwnerIntent(ctx, tx, b, id, order, actor, false); e != nil {
			return e
		}
		e = recordRepair(ctx, tx, t, before, revision, "identify", actor.Authority+":"+actor.AccountID+":"+actor.ProfileID)
		if e != nil {
			return e
		}
	}
	if e != nil {
		return e
	}
	if authorize != nil {
		if e = authorize(tx); e != nil {
			return e
		}
	}
	return gated.Commit()
}

// The list's revision is the input generation, not worker leases/status. Retry
// only the old route's incidental revision races; candidate input digests still
// reject a choice whose evidence or authority really changed.
func (s *Service) attentionObservedAction(ctx context.Context, library string, in AttentionCommand, actor MBActor, authorize func(*sql.Tx) error) error {
	for attempt := 0; attempt < 3; attempt++ {
		read, e := dbwork.BeginSnapshot(ctx, s.db)
		if e != nil {
			return e
		}
		b, e := readScreenBase(ctx, read.Tx(), in.Kind, in.EntityID)
		read.Rollback()
		if e != nil {
			return e
		}
		if b.Library != library {
			return ErrRepairInput
		}
		if b.Generation != in.ExpectedRevision {
			return ErrScreenConflict
		}
		provider := in.Provider
		if provider == "" && len(b.Providers) > 0 {
			provider = b.Providers[0]
		}
		if in.Action == "accept" {
			e = s.SelectScreen(ctx, in.Kind, in.EntityID, ScreenSelection{Actor: actor, ExpectedRevision: b.Revision, CandidateKey: in.CandidateKey, Order: in.Order}, authorize)
		} else {
			e = s.SearchScreen(ctx, in.Kind, in.EntityID, ScreenSearch{ExpectedRevision: b.Revision, Provider: provider, Query: in.Query}, authorize)
		}
		if !errors.Is(e, ErrScreenConflict) {
			return e
		}
	}
	return ErrScreenConflict
}

// AttentionCounts reports exact waiting counts without loading titles or candidates.
type AttentionLibraryCount struct {
	LibraryID string `json:"libraryId"`
	Total     int64  `json:"total"`
}
type AttentionCounts struct {
	Total     int64                   `json:"total"`
	Libraries []AttentionLibraryCount `json:"libraries"`
}

func (s *Service) AttentionCounts(ctx context.Context) (AttentionCounts, error) {
	out := AttentionCounts{Libraries: []AttentionLibraryCount{}}
	rows, err := s.db.QueryContext(ctx, `SELECT l.id,count(a.entity_id) FROM libraries l JOIN catalog_libraries cl ON cl.library_id=l.id AND cl.retired=0
 LEFT JOIN (screen_metadata_attention a JOIN catalog_entities e ON e.id=a.entity_id AND e.retired=0) ON a.library_id=l.id AND a.dismissed_generation<>a.generation AND a.reason NOT IN('pending','searching','pending_search','pending_episodes','pending_children','pending_apply') GROUP BY l.id ORDER BY l.id`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v AttentionLibraryCount
		if err = rows.Scan(&v.LibraryID, &v.Total); err != nil {
			return out, err
		}
		out.Total += v.Total
		out.Libraries = append(out.Libraries, v)
	}
	return out, rows.Err()
}
