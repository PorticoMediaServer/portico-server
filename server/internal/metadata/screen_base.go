package metadata

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/entityid"
	"portico.local/server/internal/metadataprovider"
)

const screenAlgorithm = "screen-title-year-margin-v1"

var ErrScreenConflict = errors.New("screen metadata state changed; refresh before selecting")
var errScreenStale = errors.New("screen publication authority changed")

type ScreenProvider interface {
	SearchScreen(context.Context, string, string, int, string, string) ([]metadataprovider.ScreenRecord, error)
	ScreenDetails(context.Context, string, string, string, string) (metadataprovider.ScreenRecord, error)
}
type screenEpisodeProvider interface {
	ScreenEpisode(context.Context, string, int, int, string, string) (metadataprovider.ScreenRecord, error)
}
type screenEdition struct {
	AssetID string `json:"assetId"`
	Label   string `json:"label"`
	Source  string `json:"source"`
}
type screenBase struct {
	IdentityLocked                                                             bool
	OwnerDigest                                                                string
	TargetKind, TargetID, Library, LibraryKind, ItemKind, Title, Query, Status string
	// Entity is the integer catalog_entities row TargetID (the public id)
	// names. Every INTEGER catalogue reference in these queries uses it.
	Entity int64
	// ParentEntity is the integer row of an episode's show.
	ParentEntity                                                                                    int64
	Year                                                                                            int
	Revision, Selection, Generation                                                                 int64
	Provider, ProviderType, ProviderID, Mode, Order, Requested, Accepted, Parent, ParentPublication string
	ParentProvider, ParentProviderType, ParentProviderID, ParentOrder                               string
	ParentSelection                                                                                 int64
	Season, Number                                                                                  int
	Numbering, LocalIdentity                                                                        string
	Incarnation                                                                                     string
	SourceRevision, IdentityRevision, DescriptionRevision, HierarchyRevision                        int64
	Configuration, ScanRevision, PolicyRevision, ConsentRevision                                    int64
	Confirmed, Enabled, Available, LocalAllowed                                                     bool
	Language, Region, RefreshMode                                                                   string
	Providers                                                                                       []string
	SourceDigest                                                                                    string
	Names                                                                                           []string
	NFO                                                                                             []assets.VideoNFO
	Existing                                                                                        []metadataprovider.ScreenID
	Editions                                                                                        []screenEdition
	AnimeSeasonID                                                                                   string
	AnimeSeasonRevision                                                                             int64
}
type screenClaim struct {
	Base                 screenBase
	Token, Digest, Until string
	Attempts             int
}

// resolveEntity maps a public catalogue id to its integer entity row. An
// unknown or malformed id reads as not found, preserving the old
// sql.ErrNoRows paths. Every function in this lane resolves once and passes
// the int64 to its statements (Rewrite Map §1).
func resolveEntity(ctx context.Context, tx *sql.Tx, public string) (int64, error) {
	id, err := entityid.Resolve(ctx, tx, public)
	if errors.Is(err, entityid.ErrNotFound) {
		return 0, sql.ErrNoRows
	}
	return id, err
}

// entityPublic maps an integer entity row back to its public id. A row that
// lost its entity reads as empty, never as an error.
func entityPublic(ctx context.Context, tx *sql.Tx, id int64) string {
	public, err := entityid.Public(ctx, tx, id)
	if err != nil {
		return ""
	}
	return public
}

// scanParentID reads screen_metadata_work.parent_id, which the baseline
// types INTEGER but defaults to ”.
func scanParentID(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case []byte:
		parsed, _ := strconv.ParseInt(string(n), 10, 64)
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(n, 10, 64)
		return parsed
	}
	return 0
}
func screenDigest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func screenTargetAssets(kind string) string {
	if kind == "show" {
		return `SELECT DISTINCT a.token FROM catalog_asset_links al JOIN catalog_assets a ON a.id=al.asset_id JOIN catalog_episodes e ON e.entity_id=al.entity_id WHERE e.show_id=?`
	}
	return `SELECT a.token FROM catalog_asset_links al JOIN catalog_assets a ON a.id=al.asset_id WHERE al.entity_id=?`
}

// Reads only durable observations. The matcher never opens a source, sidecar or
// media file. Unbounded show inventories are streamed into a bounded digest.
func readScreenBase(ctx context.Context, tx *sql.Tx, kind, id string) (screenBase, error) {
	var b screenBase
	b.TargetKind, b.TargetID = kind, id
	entity, err := resolveEntity(ctx, tx, id)
	if err != nil {
		return b, err
	}
	b.Entity = entity
	var providers, operations, tier string
	var parentRaw any
	err = tx.QueryRowContext(ctx, `SELECT w.library_id,l.kind,w.title_seed,CASE WHEN w.year_override>0 THEN w.year_override ELSE w.year_seed END,w.query_override,w.revision,w.selection_revision,w.generation,w.status,w.provider,w.provider_type,w.provider_id,w.selection_mode,w.episode_order,w.requested_provider,w.accepted_publication,w.parent_id,p.revision,p.enabled,p.providers,p.language,p.region,p.refresh_mode,c.revision,c.confirmed,COALESCE(lc.revision,0),sp.revision,sp.tier,sp.operations_json
  FROM screen_metadata_work w JOIN libraries l ON l.id=w.library_id JOIN screen_metadata_policies p ON p.library_id=w.library_id CROSS JOIN screen_metadata_consent c LEFT JOIN library_configuration lc ON lc.library_id=l.id JOIN library_scan_policies sp ON sp.library_id=l.id WHERE w.target_kind=? AND w.target_id=? AND c.singleton=1 AND l.kind IN('movie','tv','anime')`, kind, entity).Scan(&b.Library, &b.LibraryKind, &b.Title, &b.Year, &b.Query, &b.Revision, &b.Selection, &b.Generation, &b.Status, &b.Provider, &b.ProviderType, &b.ProviderID, &b.Mode, &b.Order, &b.Requested, &b.Accepted, &parentRaw, &b.PolicyRevision, &b.Enabled, &providers, &b.Language, &b.Region, &b.RefreshMode, &b.ConsentRevision, &b.Confirmed, &b.Configuration, &b.ScanRevision, &tier, &operations)
	if err != nil {
		return b, err
	}
	b.Parent = entityPublic(ctx, tx, scanParentID(parentRaw))
	if err = json.Unmarshal([]byte(providers), &b.Providers); err != nil {
		return b, err
	}
	var ops []string
	if err = json.Unmarshal([]byte(operations), &ops); err != nil {
		return b, err
	}
	for _, op := range ops {
		if op == "local_metadata" && tier != "file_list_only" {
			b.LocalAllowed = true
		}
	}
	if kind == "show" {
		b.ItemKind = "show"
		err = tx.QueryRowContext(ctx, `SELECT incarnation,source_revision,hierarchy_revision,selection_revision,descriptive_revision FROM tvdb_publication_heads WHERE show_id=?`, entity).Scan(&b.Incarnation, &b.SourceRevision, &b.HierarchyRevision, &b.IdentityRevision, &b.DescriptionRevision)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT k.name,h.incarnation,h.source_revision,h.identity_revision,h.metadata_revision FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind JOIN metadata_publication_heads h ON h.item_id=e.id WHERE e.id=?`, entity).Scan(&b.ItemKind, &b.Incarnation, &b.SourceRevision, &b.IdentityRevision, &b.DescriptionRevision)
	}
	if err != nil {
		return b, err
	}
	if kind == "show" {
		if err = tx.QueryRowContext(ctx, `SELECT CASE WHEN count(DISTINCT numbering)=1 THEN min(numbering) ELSE 'mixed' END FROM catalog_episodes WHERE show_id=?`, entity).Scan(&b.Numbering); err != nil {
			return b, err
		}
	}
	if b.ItemKind == "episode" {
		var show int64
		err = tx.QueryRowContext(ctx, `SELECT e.show_id,COALESCE(s.number,-1),e.number,e.numbering,e.local_identity_status,w.accepted_publication,w.selection_revision,w.provider,w.provider_type,w.provider_id,w.episode_order FROM catalog_episodes e LEFT JOIN catalog_seasons s ON s.entity_id=e.season_id JOIN screen_metadata_work w ON w.target_kind='show' AND w.target_id=e.show_id WHERE e.entity_id=?`, entity).Scan(&show, &b.Season, &b.Number, &b.Numbering, &b.LocalIdentity, &b.ParentPublication, &b.ParentSelection, &b.ParentProvider, &b.ParentProviderType, &b.ParentProviderID, &b.ParentOrder)
		if err != nil {
			return b, err
		}
		b.Parent = entityPublic(ctx, tx, show)
		b.ParentEntity = show
		err = tx.QueryRowContext(ctx, `SELECT anilist_id,revision FROM screen_anime_seasons WHERE show_id=? AND season_number=?`, show, b.Season).Scan(&b.AnimeSeasonID, &b.AnimeSeasonRevision)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return b, err
		}
	}
	h := sha256.New()
	enc := json.NewEncoder(h)
	rows, err := tx.QueryContext(ctx, `SELECT id,kind,generation,incarnation,root_identity,identity_confirmed,enabled,health,configured_root,root,follow_symlinks FROM library_sources WHERE library_id=? ORDER BY id`, b.Library)
	if err != nil {
		return b, err
	}
	enabledSource := false
	for rows.Next() {
		var sid, sk, inc, rootID, health, configured, root string
		var gen int64
		var confirmed, enabled, follow bool
		if err = rows.Scan(&sid, &sk, &gen, &inc, &rootID, &confirmed, &enabled, &health, &configured, &root, &follow); err != nil {
			rows.Close()
			return b, err
		}
		if enabled && health != "offline" && health != "removing" && health != "root_changed" {
			enabledSource = true
		}
		_ = enc.Encode([]any{sid, sk, gen, inc, rootID, confirmed, enabled, health, configured, root, follow})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return b, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT a.token,a.path,a.size,a.modified_ns,a.available,COALESCE(o.id,''),COALESCE(o.revision,''),COALESCE(o.root_incarnation,''),COALESCE(o.state,''),COALESCE(o.retired,0),COALESCE(s.enabled,0),COALESCE(s.incarnation,''),COALESCE(s.health,'') FROM catalog_assets a LEFT JOIN inventory_objects o ON o.asset_id=a.token LEFT JOIN library_sources s ON s.id=o.source_id WHERE a.token IN(`+screenTargetAssets(kind)+`) ORDER BY a.id,o.id`, entity)
	if err != nil {
		return b, err
	}
	names := map[string]bool{}
	for rows.Next() {
		var aid, path, oid, rev, inc, state, sinc, health string
		var size, modified int64
		var available, retired, enabled bool
		if err = rows.Scan(&aid, &path, &size, &modified, &available, &oid, &rev, &inc, &state, &retired, &enabled, &sinc, &health); err != nil {
			rows.Close()
			return b, err
		}
		_ = enc.Encode([]any{aid, path, size, modified, available, oid, rev, inc, state, retired, enabled, sinc, health})
		good := available && (oid == "" && enabledSource || oid != "" && !retired && state == "available" && enabled && inc == sinc && health != "offline" && health != "removing" && health != "root_changed")
		if good {
			b.Available = true
		}
		if kind == "item" && b.ItemKind == "movie" && len(b.Names) < 4 {
			n := filepath.Base(path)
			if !names[n] {
				b.Names = append(b.Names, n)
				names[n] = true
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return b, err
	}
	// Ignore stale observations from a different media-file revision. Duplicate
	// tvshow.nfo copies collapse by digest; conflicting copies remain visible.
	if b.LocalAllowed {
		nfoKind := "movie"
		if kind == "show" {
			nfoKind = "show"
		} else if b.ItemKind == "episode" {
			nfoKind = "episode"
		}
		rows, err = tx.QueryContext(ctx, `SELECT DISTINCT n.digest,n.payload FROM video_nfo_evidence n JOIN catalog_assets a ON a.token=n.asset_id WHERE n.asset_id IN(`+screenTargetAssets(kind)+`) AND n.kind=? AND n.source_size=a.size AND n.source_modified=a.modified_ns ORDER BY n.digest,n.payload`, entity, nfoKind)
		if err != nil {
			return b, err
		}
		for rows.Next() {
			var digest, payload string
			if err = rows.Scan(&digest, &payload); err != nil {
				rows.Close()
				return b, err
			}
			_ = enc.Encode([]string{digest, payload})
			if len(b.NFO) < 64 {
				var n assets.VideoNFO
				if err = json.Unmarshal([]byte(payload), &n); err != nil {
					rows.Close()
					return b, err
				}
				if b.ItemKind == "episode" && !screenNFOCoordinates(n, b) {
					continue
				}
				b.NFO = append(b.NFO, n)
			} else {
				rows.Close()
				return b, errors.New("conflicting_local_metadata_budget")
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return b, err
		}
	}
	b.SourceDigest = hex.EncodeToString(h.Sum(nil))
	if kind == "item" {
		rows, err = tx.QueryContext(ctx, `SELECT provider,CAST(provider_id AS TEXT) FROM provider_evidence WHERE item_id=? AND provider IN('tmdb','tvdb','anilist') ORDER BY provider`, entity)
		if err != nil {
			return b, err
		}
		for rows.Next() {
			v := metadataprovider.ScreenID{Type: b.ItemKind}
			if err = rows.Scan(&v.Provider, &v.ID); err != nil {
				rows.Close()
				return b, err
			}
			if v.Provider == "anilist" {
				v.Type = "anime"
			}
			if metadataprovider.ValidScreenID(v) {
				b.Existing = append(b.Existing, v)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return b, err
		}
	} else if b.Provider == "" {
		var id int64
		err = tx.QueryRowContext(ctx, `SELECT provider_id FROM tvdb_jobs WHERE show_id=?`, entity).Scan(&id)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return b, err
		}
		if id > 0 {
			b.Existing = append(b.Existing, metadataprovider.ScreenID{Provider: "tvdb", Type: "show", ID: strings.TrimSpace(intString(id))})
		}
	}
	// Owner changes participate independently of catalog projection triggers.
	locks, err := relationshipLocks(ctx, tx, RepairTarget{kind, id})
	if err != nil {
		return b, err
	}
	b.IdentityLocked = locks["identity"]
	b.OwnerDigest = screenDigest(locks)
	return b, nil
}
func screenNFOCoordinates(n assets.VideoNFO, b screenBase) bool {
	if b.Numbering == "absolute" {
		return n.Absolute != nil && *n.Absolute == b.Number || n.Absolute == nil && n.Episode != nil && *n.Episode == b.Number && n.Season == nil
	}
	return n.Episode != nil && *n.Episode == b.Number && (n.Season == nil || *n.Season == b.Season)
}
func screenBaseAuthority(b screenBase) screenBase {
	// Job revision/status track UI progress, not the authority of an observed
	// candidate. Generation and selection revision still fence every publication.
	b.Revision = 0
	b.Status = ""
	return b
}
func screenInputDigest(b screenBase) string { return screenDigest(screenBaseAuthority(b)) }
