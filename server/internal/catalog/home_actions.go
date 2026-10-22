package catalog

import (
	"database/sql"
	"encoding/json"
	"strconv"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/personalstate"
)

// HomeActions names implemented, authorized commands for the displayed entity.
// Device support is deliberately separate; these facts never grant a write.
// Every command rechecks its own authority and current target on execution.
type HomeActions struct {
	Play            bool            `json:"play"`
	Queue           bool            `json:"queue"`
	Playlist        bool            `json:"playlist"`
	Collection      bool            `json:"collection"`
	MyList          bool            `json:"myList"`
	Favorite        bool            `json:"favorite"`
	Watched         bool            `json:"watched"`
	Played          bool            `json:"played"`
	Rating          bool            `json:"rating"`
	EditMetadata    bool            `json:"editMetadata"`
	RefreshMetadata bool            `json:"refreshMetadata"`
	Delete          bool            `json:"delete"`
	Download        bool            `json:"download"`
	Details         bool            `json:"details"`
	State           HomeActionState `json:"state"`
}

type HomeActionState struct {
	Watchlisted bool     `json:"watchlisted"`
	Favorite    bool     `json:"favorite"`
	Watched     bool     `json:"watched"`
	Rating      *float64 `json:"rating"`
	// PersonalRevision belongs to the displayed entity, never its play target.
	PersonalRevision  int64 `json:"personalRevision"`
	ContainerRevision int64 `json:"containerRevision"`
}

// HomeChannelActions applies only to the real source/channel pair. There is no
// catalogue bulk contract for a channel.
type HomeChannelActions struct {
	Play     bool `json:"play"`
	Favorite bool `json:"favorite"`
}

// HomeActionAuthority is resolved by the HTTP command authorizers in the same
// snapshot as Home. None of these facts is inferred from a role by catalog.
type HomeActionAuthority struct {
	Personal, Organize, Bulk, Playback, Queue, Metadata, Refresh, Downloads bool
	DeleteLibrary                                                           func(string) (bool, error)
}

type homeActionFacts struct {
	kind, library string
	available     bool
	state         HomeActionState
}

// As with Home's fallback source walks, a restricted availability proof has a
// fixed candidate budget. A global member count must never reveal availability
// that belongs only to children this viewer cannot see.
const homeActionMemberProofLimit = 64

// ProjectHomeActions decorates only the already-authorized returned Home items.
// It requires the composition snapshot, batches exact entity lookups, and never
// enumerates a whole container or fetches a library. The Home caller retains its
// normal post-read revocation/fence proof before returning these offers.
func (s *Service) ProjectHomeActions(v Viewer, authority HomeActionAuthority, rows []HomeRow, spotlight []HomeSpotlight) error {
	if dbwork.Snapshot(s.Context()) == nil {
		return sql.ErrTxDone
	}
	entries := []*ContentEntry{}
	collections := map[*HomeCollection]*ContentEntry{}
	var collect func([]HomeRow)
	collect = func(rows []HomeRow) {
		for i := range rows {
			row := &rows[i]
			for j := range row.Entries {
				entries = append(entries, &row.Entries[j])
			}
			for j := range row.Resumes {
				entries = append(entries, &row.Resumes[j].Target)
			}
			if row.Collection != nil {
				c := row.Collection
				entry := &ContentEntry{ID: c.Collection.ID, LibraryID: c.Collection.LibraryID, Kind: "collection"}
				collections[c] = entry
				entries = append(entries, entry)
				for j := range row.Collection.Members {
					entries = append(entries, &row.Collection.Members[j])
				}
			}
			collect(row.Children)
		}
	}
	collect(rows)
	for i := range spotlight {
		entries = append(entries, &spotlight[i].Entry)
		if spotlight[i].ResumeTarget != nil {
			entries = append(entries, spotlight[i].ResumeTarget)
		}
	}
	if len(entries) == 0 {
		return nil
	}
	ids, seen := []string{}, map[string]bool{}
	for _, entry := range entries {
		entry.HomeActions = nil
		if !seen[entry.ID] {
			ids = append(ids, entry.ID)
			seen[entry.ID] = true
		}
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	facts := map[string]homeActionFacts{}
	availability := `COALESCE(a.available,links.available>0,0)`
	bound := []any{}
	if v.EffectiveRestrictions().Active() {
		visible, args := v.itemVisibilitySQL("member.id")
		availability = `CASE WHEN k.playable=1 THEN COALESCE(a.available,0) ELSE EXISTS(
		 SELECT 1 FROM (SELECT item_id FROM catalog_browse_memberships WHERE entity_id=e.id ORDER BY item_id LIMIT ` + strconv.Itoa(homeActionMemberProofLimit) + `) candidates
		 CROSS JOIN catalog_entities member ON member.id=candidates.item_id
		 JOIN catalog_item_availability available ON available.entity_id=member.id AND available.available=1 AND available.retired=0
		 WHERE ` + visible + recordingsClause("member.id", v.EffectiveRestrictions()) + `) END`
		bound = args
	}
	bound = append(bound, v.Profile, string(raw), v.Profile, v.Profile)
	// The requested primary keys drive every join. Personal state is the
	// displayed title's row; a show's first episode cannot supply its flags.
	found, err := s.read().Query(`SELECT pid(e.public_id),k.name,l.library_id,
	 `+availability+`,COALESCE(p.watchlisted,0),COALESCE(p.favorite,0),p.rating,
	 COALESCE(p.revision,0),COALESCE(c.watched,`+personalstate.SQL("?", "e.id")+`,0),COALESCE(c.revision,0)
	 FROM json_each(?) requested CROSS JOIN catalog_entities e ON e.public_id=pid_blob(requested.value)
	 JOIN catalog_kinds k ON k.id=e.kind JOIN catalog_libraries l ON l.id=e.library_id
	 LEFT JOIN catalog_item_availability a ON a.entity_id=e.id AND a.retired=0
	 LEFT JOIN catalog_browse_member_links links ON links.entity_id=e.id
	 LEFT JOIN personal_items p ON p.item_id=e.id AND p.profile_id=?
	 LEFT JOIN container_personal_state c ON c.container_id=e.id AND c.kind=k.name AND c.profile_id=?
	 WHERE e.retired=0 AND l.retired=0`, bound...)
	if err != nil {
		return err
	}
	for found.Next() {
		var id string
		var f homeActionFacts
		if err = found.Scan(&id, &f.kind, &f.library, &f.available, &f.state.Watchlisted, &f.state.Favorite, &f.state.Rating, &f.state.PersonalRevision, &f.state.Watched, &f.state.ContainerRevision); err != nil {
			break
		}
		facts[id] = f
	}
	if err == nil {
		err = found.Err()
	}
	found.Close()
	if err != nil {
		return err
	}
	deletion := map[string]bool{}
	for _, entry := range entries {
		f, ok := facts[entry.ID]
		if !ok || f.kind != entry.Kind || f.library != entry.LibraryID || !v.AllowsLibrary(f.library) {
			continue
		}
		leaf := f.kind == "movie" || f.kind == "episode" || f.kind == "song" || f.kind == "audiobook_file"
		container := f.kind == "show" || f.kind == "season" || f.kind == "album" || f.kind == "artist" || f.kind == "book" || f.kind == "collection"
		if !leaf && !container {
			continue
		}
		if entry.Watched != nil {
			f.state.Watched = *entry.Watched
		}
		available := f.available
		if entry.Available != nil && !*entry.Available || entry.Missing {
			available = false
		}
		a := &HomeActions{State: f.state, Details: true}
		a.Play = authority.Playback && available
		a.Queue = authority.Queue && available
		// Playlist jobs expand a real container selector server-side. Explicit
		// ids must remain leaf ids; callers must not send an aggregate as one.
		a.Playlist = authority.Organize && (leaf || container && authority.Bulk)
		a.Collection = authority.Organize && (leaf || container && authority.Bulk)
		a.MyList = authority.Personal && (leaf || containerSaves(f.kind, "watchlisted"))
		a.Favorite = authority.Personal && (leaf || containerSaves(f.kind, "favorite"))
		a.Watched = authority.Personal && (f.kind == "movie" || f.kind == "episode" || f.kind == "show" || f.kind == "season")
		a.Played = authority.Personal && (f.kind == "song" || f.kind == "audiobook_file" || f.kind == "album" || f.kind == "book")
		a.Rating = authority.Personal && leaf
		a.EditMetadata = authority.Metadata && (leaf || f.kind == "show" || f.kind == "season" || f.kind == "album" || f.kind == "artist" || f.kind == "book")
		a.RefreshMetadata = authority.Refresh && (leaf || f.kind == "show")
		a.Download = authority.Downloads && available && (leaf || f.kind == "show" || f.kind == "season" || f.kind == "album" || f.kind == "book")
		if authority.DeleteLibrary != nil && leaf {
			allowed, known := deletion[f.library]
			if !known {
				allowed, err = authority.DeleteLibrary(f.library)
				if err != nil {
					return err
				}
				deletion[f.library] = allowed
			}
			a.Delete = allowed
		}
		entry.HomeActions = a
	}
	for collection, entry := range collections {
		collection.Actions = entry.HomeActions
	}
	return nil
}
