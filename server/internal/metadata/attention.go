package metadata

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
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
	EntityID   string               `json:"entityId"`
	Kind       string               `json:"kind"`
	Title      string               `json:"title"`
	Revision   int64                `json:"revision"`
	Reason     string               `json:"reason"`
	Confidence float64              `json:"confidence"`
	Candidates []AttentionCandidate `json:"candidates"`
}
type AttentionPage struct {
	LibraryID  string           `json:"libraryId"`
	Entries    []AttentionEntry `json:"entries"`
	NextCursor string           `json:"nextCursor"`
}

// Attention reads one indexed page, then at most three candidates per entry.
func (s *Service) Attention(ctx context.Context, library, cursor string, limit int) (AttentionPage, error) {
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
	if !exists {
		return out, sql.ErrNoRows
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.entity_id,a.target_kind,e.title,w.revision,a.reason,a.confidence FROM screen_metadata_attention a JOIN catalog_entities e ON e.id=a.entity_id JOIN screen_metadata_work w ON w.target_kind=a.target_kind AND w.target_id=a.entity_id WHERE a.library_id=? AND a.entity_id>? AND a.dismissed_generation<>a.generation ORDER BY a.entity_id LIMIT ?`, library, after, limit+1)
	if err != nil {
		return out, err
	}
	ids := []int64{}
	for rows.Next() {
		var e AttentionEntry
		var id int64
		if err = rows.Scan(&id, &e.Kind, &e.Title, &e.Revision, &e.Reason, &e.Confidence); err != nil {
			rows.Close()
			return out, err
		}
		if len(out.Entries) == limit {
			out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(ids[len(ids)-1], 10)))
			break
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
	switch in.Action {
	case "accept":
		return s.SelectScreen(ctx, in.Kind, in.EntityID, ScreenSelection{Actor: actor, ExpectedRevision: in.ExpectedRevision, CandidateKey: in.CandidateKey, Order: in.Order}, authorize)
	case "search":
		return s.SearchScreen(ctx, in.Kind, in.EntityID, ScreenSearch{ExpectedRevision: in.ExpectedRevision, Provider: in.Provider, Query: in.Query}, authorize)
	case "dismiss", "id":
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
	if b.Revision != in.ExpectedRevision {
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
