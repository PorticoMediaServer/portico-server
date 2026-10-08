package livechannels

// Remote source setup is deliberately two-phase: preview fetches into a private,
// expiring encrypted snapshot; only SaveRemote publishes configuration and guide.
// No URL or credential is included in a projection, error, receipt, or log.
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/worker"

	"portico.local/server/internal/remotemedia"
)

type ChannelMapping struct {
	ChannelKey string `json:"channelKey"`
	GuideKey   string `json:"guideKey"`
}

type RemoteDraft struct {
	ID               string `json:"id"`
	ExpectedRevision int64  `json:"expectedRevision"`
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	Locator          string `json:"locator"`
	GuideURL         string `json:"guideUrl"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	// These exact roots are an explicit owner confirmation, never discovery data.
	ConfirmedLANRoots     []string         `json:"confirmedLanRoots"`
	Mappings              []ChannelMapping `json:"mappings"`
	RefreshSeconds        int              `json:"refreshSeconds"`
	OwnerLimit            int              `json:"ownerLimit"`
	UseDiscoveredCapacity bool             `json:"useDiscoveredCapacity"`
	ViewerAccess          string           `json:"viewerAccess"`
}

type remoteConfig struct {
	Draft         RemoteDraft            `json:"draft"`
	Approvals     []remotemedia.Approval `json:"approvals"`
	PhysicalCount int                    `json:"physicalCount"`
}

type remoteSnapshot struct {
	Config remoteConfig `json:"config"`
	Input  SourceInput  `json:"input"`
}

type RemotePreview struct {
	ID                 string       `json:"previewId"`
	Preview            Preview      `json:"preview"`
	DiscoveredCapacity int          `json:"discoveredCapacity"`
	CapacityMode       string       `json:"capacityMode"`
	MappingKeys        []MappingKey `json:"mappingKeys"`
	ExpiresAt          string       `json:"expiresAt"`
}

type MappingKey struct {
	ChannelKey string `json:"channelKey"`
	Name       string `json:"name"`
	GuideKey   string `json:"guideKey"`
}

type RemoteStatus struct {
	SourceID    string `json:"sourceId"`
	State       string `json:"state"`
	ErrorCode   string `json:"errorCode"`
	NextRefresh string `json:"nextRefresh"`
	Failures    int    `json:"failures"`
}

type SourceFetcher struct{ Resolver remotemedia.Resolver }

// maxSealed bounds a stored payload the server wrote itself. A remote preview's snapshot holds the
// fetched playlist and guide (up to MaxUploadBytes together) as JSON, which escapes the guide's
// XML markup (`<` becomes six bytes), so the bound leaves generous room. It was 128 KiB, left
// over from encrypted payloads, and refused every real guide: the preview succeeded and the save
// answered guide_unavailable.
const maxSealed = 8 * MaxUploadBytes

func (s *Store) unseal(raw []byte, _ string) (string, error) {
	if len(raw) == 0 || len(raw) > maxSealed || !utf8.Valid(raw) {
		return "", ErrUnavailable
	}
	return string(raw), nil
}

func (f SourceFetcher) config(ctx context.Context, d RemoteDraft) (remoteConfig, error) {
	c := remoteConfig{Draft: d, Approvals: []remotemedia.Approval{}}
	if !opaque(d.ID) || d.ExpectedRevision < 0 || !validText(d.Name, 120) || !validLocator(d.Locator) || (d.GuideURL != "" && !validLocator(d.GuideURL)) || d.OwnerLimit < 0 || d.OwnerLimit > 256 || d.RefreshSeconds < 300 || d.RefreshSeconds > 86400 || len(d.ConfirmedLANRoots) > 8 || len(d.Mappings) > MaxChannels || len(d.Username) > 512 || len(d.Password) > 2048 || (d.ViewerAccess != "owner-only" && d.ViewerAccess != "server-members") {
		return c, ErrInvalid
	}
	if d.Kind != "m3u" && d.Kind != "xtream" && d.Kind != "hdhomerun" {
		return c, ErrInvalid
	}
	if d.Kind != "xtream" && (d.Username != "" || d.Password != "") {
		return c, ErrInvalid
	}
	if d.Kind == "xtream" && (!validText(d.Username, 512) || !validText(d.Password, 2048)) {
		return c, ErrInvalid
	}
	p := remotemedia.Policy{Resolver: f.Resolver}
	for _, root := range d.ConfirmedLANRoots {
		a, e := p.Approve(ctx, root)
		if e != nil {
			return c, sourceFetchError(e)
		}
		c.Approvals = append(c.Approvals, a)
	}
	return c, nil
}

func (s *Store) PreviewRemote(ctx context.Context, a Authority, f SourceFetcher, d RemoteDraft) (RemotePreview, error) {
	out := RemotePreview{}
	fence := ""
	e := s.transaction(ctx, a, true, func(tx *sql.Tx) error {
		var err error
		fence, _, err = authorize(ctx, tx, a, true)
		if err != nil {
			return err
		}
		var rev int64
		err = tx.QueryRowContext(ctx, `SELECT revision FROM live_sources WHERE id=?`, d.ID).Scan(&rev)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ErrUnavailable
		}
		if rev != d.ExpectedRevision {
			return ErrConflict
		}
		return nil
	})
	if e != nil {
		return out, e
	}
	c, e := f.config(ctx, d)
	if e != nil {
		return out, e
	}
	input, physical, e := f.fetch(ctx, c)
	if e != nil {
		return out, e
	}
	c.PhysicalCount = physical
	input.TunerCount = effectiveCapacity(physical, d.OwnerLimit, d.UseDiscoveredCapacity)
	preview, e := PreviewSource(input)
	if e != nil {
		return out, e
	}
	parsed, e := parse(input)
	if e != nil {
		return out, e
	}
	if e = f.coverStreams(ctx, &c, parsed); e != nil {
		return out, e
	}
	token, e := nonceID()
	if e != nil {
		return out, e
	}
	payload, e := json.Marshal(remoteSnapshot{Config: c, Input: input})
	if e != nil {
		return out, ErrInvalid
	}
	sealed, e := s.seal(string(payload), "preview:"+token)
	if e != nil {
		return out, e
	}
	expiry := time.Now().Add(20 * time.Minute)
	e = s.transaction(ctx, a, true, func(tx *sql.Tx) error {
		current, _, err := authorize(ctx, tx, a, true)
		if err != nil {
			return err
		}
		if current != fence {
			return ErrDenied
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM live_remote_previews WHERE expires_ms<?`, time.Now().UnixMilli()); err != nil {
			return ErrUnavailable
		}
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM live_remote_previews`).Scan(&count); err != nil {
			return ErrUnavailable
		}
		if count >= 128 {
			return ErrUnavailable
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO live_remote_previews VALUES(?,?,?,?,?,?)`, token, d.ID, d.ExpectedRevision, fence, sealed, expiry.UnixMilli())
		if err != nil {
			return ErrUnavailable
		}
		return nil
	})
	if e != nil {
		return out, e
	}
	keys := make([]MappingKey, 0, len(parsed.channels))
	for _, ch := range parsed.channels {
		guide := ch.key
		for _, m := range input.Mappings {
			if m.ChannelKey == ch.key {
				guide = m.GuideKey
				break
			}
		}
		keys = append(keys, MappingKey{ch.key, ch.name, guide})
	}
	mode := "defaulted"
	if d.UseDiscoveredCapacity && physical > 0 {
		mode = "discovered"
	}
	if d.OwnerLimit > 0 {
		mode = "configured"
	}
	return RemotePreview{ID: token, Preview: preview, DiscoveredCapacity: physical, CapacityMode: mode, MappingKeys: keys, ExpiresAt: expiry.UTC().Format(time.RFC3339)}, nil
}

func effectiveCapacity(physical, limit int, usePhysical bool) int {
	if !usePhysical {
		physical = 0
	}
	if physical > 0 && (limit == 0 || physical < limit) {
		return physical
	}
	return limit
}

func (s *Store) SaveRemote(ctx context.Context, a Authority, previewID, requestID string) (Source, error) {
	if !opaque(previewID) || !opaque(requestID) {
		return Source{}, ErrInvalid
	}
	var snap remoteSnapshot
	var replay *Source
	e := s.transaction(ctx, a, true, func(tx *sql.Tx) error {
		var sealed []byte
		var bound string
		err := tx.QueryRowContext(ctx, `SELECT preview_id,sealed FROM live_remote_save_bindings WHERE request_id=?`, requestID).Scan(&bound, &sealed)
		if err == nil {
			if bound != previewID {
				return ErrConflict
			}
			var status string
			err = tx.QueryRowContext(ctx, `SELECT status FROM live_operations WHERE request_id=?`, requestID).Scan(&status)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return ErrUnavailable
			}
			if status == "published" {
				v, e := operationSource(ctx, tx, requestID)
				if e != nil {
					return e
				}
				replay = &v
				return nil
			}
			if status == "superseded" {
				return ErrConflict
			}
			raw, e := s.unseal(sealed, "remote-save:"+requestID)
			if e != nil {
				return ErrConflict
			}
			if json.Unmarshal([]byte(raw), &snap) != nil {
				return ErrUnavailable
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return ErrUnavailable
		}
		var used bool
		if tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM live_operations WHERE request_id=?)`, requestID).Scan(&used) != nil {
			return ErrUnavailable
		}
		if used {
			return ErrConflict
		}
		var fence string
		var expiry int64
		err = tx.QueryRowContext(ctx, `SELECT sealed,fence,expires_ms FROM live_remote_previews WHERE id=?`, previewID).Scan(&sealed, &fence, &expiry)
		if err != nil || expiry <= time.Now().UnixMilli() {
			return ErrConflict
		}
		current, _, err := authorize(ctx, tx, a, true)
		if err != nil {
			return err
		}
		if current != fence {
			return ErrDenied
		}
		raw, err := s.unseal(sealed, "preview:"+previewID)
		if err != nil {
			return err
		}
		if json.Unmarshal([]byte(raw), &snap) != nil {
			return ErrUnavailable
		}
		sealed, err = s.seal(raw, "remote-save:"+requestID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO live_remote_save_bindings VALUES(?,?,?,?,?)`, requestID, previewID, snap.Input.ID, sealed, time.Now().UnixMilli())
		if err != nil {
			return ErrUnavailable
		}
		return nil
	})
	if e != nil {
		return Source{}, e
	}
	if replay != nil {
		return *replay, nil
	}
	snap.Input.RequestID = requestID
	raw, e := json.Marshal(snap.Config)
	if e != nil {
		return Source{}, ErrInvalid
	}
	sealed, e := s.seal(string(raw), "remote:"+snap.Input.ID)
	if e != nil {
		return Source{}, e
	}
	return s.save(ctx, a, snap.Input, false, func(tx *sql.Tx) error {
		// The receipt remains replayable without retaining a second credential copy.
		if _, e := tx.ExecContext(ctx, `UPDATE live_remote_save_bindings SET sealed=x'' WHERE request_id=?`, requestID); e != nil {
			return ErrUnavailable
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO live_remote_configs(source_id,sealed,next_due_ms) VALUES(?,?,?) ON CONFLICT(source_id) DO UPDATE SET sealed=excluded.sealed,next_due_ms=excluded.next_due_ms,failures=0,state='healthy',error_code='',lease_token='',lease_until_ms=0`, snap.Input.ID, sealed, time.Now().Add(time.Duration(snap.Config.Draft.RefreshSeconds)*time.Second).UnixMilli()); err != nil {
			return ErrUnavailable
		}
		physical := 0
		if snap.Config.Draft.UseDiscoveredCapacity {
			physical = snap.Config.PhysicalCount
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO live_source_settings VALUES(?,?,?,?) ON CONFLICT(source_id) DO UPDATE SET physical_count=excluded.physical_count,owner_limit=excluded.owner_limit,viewer_access=excluded.viewer_access`, snap.Input.ID, physical, snap.Config.Draft.OwnerLimit, snap.Config.Draft.ViewerAccess)
		if err != nil {
			return ErrUnavailable
		}
		return nil
	})
}

func (s *Store) RemoteStatuses(ctx context.Context, a Authority) ([]RemoteStatus, error) {
	out := []RemoteStatus{}
	err := s.snapshot(ctx, a, true, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, `SELECT source_id,state,error_code,next_due_ms,failures FROM live_remote_configs ORDER BY source_id`)
		if e != nil {
			return ErrUnavailable
		}
		defer rows.Close()
		for rows.Next() {
			var v RemoteStatus
			var due int64
			if rows.Scan(&v.SourceID, &v.State, &v.ErrorCode, &due, &v.Failures) != nil {
				return ErrUnavailable
			}
			v.NextRefresh = time.UnixMilli(due).UTC().Format(time.RFC3339)
			out = append(out, v)
		}
		if rows.Err() != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, err
}

func (s *Store) RequestRefresh(ctx context.Context, a Authority, sourceID string, revision int64) error {
	if !opaque(sourceID) || revision < 1 {
		return ErrInvalid
	}
	return s.transaction(ctx, a, true, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, `UPDATE live_remote_configs SET next_due_ms=0 WHERE source_id=? AND EXISTS(SELECT 1 FROM live_sources s WHERE s.id=source_id AND s.revision=? AND s.state='active')`, sourceID, revision)
		if e != nil {
			return ErrUnavailable
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrConflict
		}
		return nil
	})
}

// RefreshOne is one bounded durable claim. A stale worker cannot publish after
// an edit/disable or another claim. Failed refresh leaves the active generation.
func (s *Store) RefreshOne(ctx context.Context, f SourceFetcher) (bool, error) {
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if e != nil {
		return false, ErrUnavailable
	}
	tx := gated.Tx()
	defer gated.Rollback()
	var sid, name string
	var sealed []byte
	var rev int64
	now := time.Now()
	e = tx.QueryRowContext(ctx, `SELECT c.source_id,c.sealed,s.revision,s.name FROM live_remote_configs c JOIN live_sources s ON s.id=c.source_id WHERE s.state='active' AND c.next_due_ms<=? AND c.lease_until_ms<=? ORDER BY c.next_due_ms,c.source_id LIMIT 1`, now.UnixMilli(), now.UnixMilli()).Scan(&sid, &sealed, &rev, &name)
	if errors.Is(e, sql.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, ErrUnavailable
	}
	lease, e := nonceID()
	if e != nil {
		return false, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE live_remote_configs SET lease_token=?,lease_until_ms=?,state='refreshing' WHERE source_id=?`, lease, now.Add(2*time.Minute).UnixMilli(), sid)
	if e != nil {
		return false, ErrUnavailable
	}
	if gated.Commit() != nil {
		return false, ErrUnavailable
	}
	var c remoteConfig
	raw, e := s.unseal(sealed, "remote:"+sid)
	if e == nil {
		e = json.Unmarshal([]byte(raw), &c)
	}
	authority := func(ctx context.Context, tx *sql.Tx, owner bool) (string, func(string, string) bool, error) {
		var valid bool
		err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM live_remote_configs c JOIN live_sources s ON s.id=c.source_id WHERE c.source_id=? AND c.lease_token=? AND c.lease_until_ms>? AND s.revision=? AND s.state='active')`, sid, lease, time.Now().UnixMilli(), rev).Scan(&valid)
		if err != nil {
			return "", nil, ErrUnavailable
		}
		if !valid {
			return "", nil, ErrConflict
		}
		return "refresh:" + lease, func(string, string) bool { return true }, nil
	}
	if e == nil {
		var in SourceInput
		var physical int
		fetchCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
		in, physical, e = f.fetch(fetchCtx, c)
		cancel()
		if e == nil {
			in.ID = sid
			in.Name = name
			in.ExpectedRevision = rev
			in.RequestID = lease
			// Never silently increase a confirmed hardware commitment on rediscovery.
			_ = physical
			in.TunerCount = effectiveCapacity(c.PhysicalCount, c.Draft.OwnerLimit, c.Draft.UseDiscoveredCapacity)
			_, e = s.save(ctx, authority, in, true, nil)
		}
	}
	code := ""
	state := "healthy"
	fail := 0
	if e != nil {
		code = "source-refresh-failed"
		state = "degraded"
		fail = 1
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	next := time.Now().Add(time.Duration(c.Draft.RefreshSeconds) * time.Second)
	if c.Draft.RefreshSeconds < 300 {
		next = time.Now().Add(5 * time.Minute)
	}
	// Bounded exponential retry delay; no secret/error text is persisted.
	if fail != 0 {
		var n int
		_ = s.db.QueryRowContext(finishCtx, `SELECT failures FROM live_remote_configs WHERE source_id=?`, sid).Scan(&n)
		if n > 6 {
			n = 6
		}
		next = time.Now().Add(time.Duration(30*(1<<n)) * time.Second)
	}
	_, finish := dbwork.ExecWrite(finishCtx, s.db, dbwork.ClassFrom(finishCtx, dbwork.ClassInteractive), `UPDATE live_remote_configs SET state=?,error_code=?,failures=CASE WHEN ?=0 THEN 0 ELSE failures+1 END,next_due_ms=?,lease_token='',lease_until_ms=0 WHERE source_id=? AND lease_token=?`, state, code, fail, next.UnixMilli(), sid, lease)
	if finish != nil {
		return true, ErrUnavailable
	}
	return true, e
}

func (s *Store) RunRefresh(ctx context.Context, f SourceFetcher) {
	// Guide refresh is bulk provider work; its writes are not a viewer's writes.
	ctx = dbwork.WithClass(ctx, dbwork.ClassBackgroundMedia)
	// A guide refresh becomes due because a source was added, a schedule was
	// written or a generation was superseded — all of them commits. The five
	// second tick asked an empty queue seventeen thousand times a day for
	// nothing; a wake asks once, when there is something.
	wake := worker.NewSignal()
	unregister := dbwork.WakeOnTables(wake, "live_*")
	defer unregister()
	var pruned time.Time
	worker.Run(ctx, "livechannels.guide-refresh", wake, func(ctx context.Context) time.Duration {
		worked := false
		for n := 0; n < 4; n++ {
			claimed, _ := s.RefreshOne(ctx, f)
			if !claimed || ctx.Err() != nil {
				break
			}
			worked = true
		}
		// Superseded guide generations are removed here, one per pass while any
		// remain, so a backlog drains steadily without ever owning the writer.
		if time.Since(pruned) >= retentionInterval {
			if found, _ := s.PruneSupersededGeneration(ctx); !found {
				pruned = time.Now()
			} else {
				worked = true
			}
		}
		if worked {
			// There is usually more behind a claimed refresh.
			return 50 * time.Millisecond
		}
		return 0
	})
}

func (s *Store) Remove(ctx context.Context, a Authority, sid string, revision int64, requestID string) error {
	if !opaque(sid) || !opaque(requestID) || revision < 1 {
		return ErrInvalid
	}
	return s.transaction(ctx, a, true, func(tx *sql.Tx) error {
		var old string
		var oldRev int64
		e := tx.QueryRowContext(ctx, `SELECT request_id,revision FROM live_source_removals WHERE source_id=?`, sid).Scan(&old, &oldRev)
		if e == nil {
			if old == requestID && oldRev == revision {
				return nil
			}
			return ErrConflict
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return ErrUnavailable
		}
		var n int
		if tx.QueryRowContext(ctx, `SELECT count(*) FROM live_source_dependencies WHERE source_id=?`, sid).Scan(&n) != nil {
			return ErrUnavailable
		}
		if n != 0 {
			return ErrDependencies
		}
		res, e := tx.ExecContext(ctx, `UPDATE live_sources SET state='disabled',revision=revision+1 WHERE id=? AND revision=?`, sid, revision)
		if e != nil {
			return ErrUnavailable
		}
		count, _ := res.RowsAffected()
		if count != 1 {
			return ErrConflict
		}
		for _, query := range []string{`UPDATE live_remote_save_bindings SET sealed=x'' WHERE source_id=?`, `DELETE FROM live_remote_configs WHERE source_id=?`, `DELETE FROM live_remote_previews WHERE source_id=?`, `UPDATE live_channel_versions SET locator_sealed=x'' WHERE generation_id IN (SELECT id FROM live_generations WHERE source_id=?)`} {
			if _, e = tx.ExecContext(ctx, query, sid); e != nil {
				return ErrUnavailable
			}
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO live_source_removals VALUES(?,?,?,?)`, sid, requestID, revision, time.Now().UTC().Format(time.RFC3339))
		if e != nil {
			return ErrUnavailable
		}
		return nil
	})
}

var ErrDependencies = errors.New("This source is still in use. Disable it, then resolve its recording, rule, or playback references before removal.")

func sourceFetchError(e error) error {
	if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return e
	}
	var confirm *remotemedia.ConfirmationRequired
	if errors.As(e, &confirm) {
		return &LANConfirmationRequired{Roots: []string{confirm.Root}}
	}
	if errors.Is(e, remotemedia.ErrPolicy) {
		return ErrNetworkPolicy
	}
	if errors.Is(e, remotemedia.ErrDenied) {
		return ErrSourceAuthentication
	}
	return ErrSourceUnavailable
}

var ErrNetworkPolicy = errors.New("This destination is outside the confirmed source network policy.")

// LANConfirmationRequired (CD-06) answers a preview whose source, guide or
// streams are on the owner's own network and not yet confirmed. Roots are the
// exact values to send back in confirmedLanRoots once the owner agrees; nothing
// is fetched from them until then. It is also ErrNetworkPolicy.
type LANConfirmationRequired struct{ Roots []string }

func (e *LANConfirmationRequired) Error() string {
	return "This source is on your own network. Confirm that Portico may connect to it."
}
func (e *LANConfirmationRequired) Is(t error) bool { return t == ErrNetworkPolicy }

// maxConfirmedRoots matches the draft's limit on confirmedLanRoots.
const maxConfirmedRoots = 8

// coverStreams makes sure every stream a previewed source will play is reachable
// under the owner's confirmations, before anything is saved. A stream on a
// non-public address that no confirmed root covers is either on a device the
// owner already confirmed (every address it resolves to is already approved:
// an HDHomeRun serves its lineup on :80 and its streams on :5004), which is
// approved with it, or it needs its own confirmation, which the preview asks for
// in one answer. Unresolvable stream hosts are left to fail at playback, as a
// public stream that is down would.
func (f SourceFetcher) coverStreams(ctx context.Context, c *remoteConfig, parsed parsedSource) error {
	p := remotemedia.Policy{Approvals: c.Approvals, Resolver: f.Resolver}
	approved := map[string]bool{}
	for _, a := range c.Approvals {
		for _, v := range a.Addresses {
			approved[v] = true
		}
	}
	seen := map[string]bool{}
	var need []string
	for _, ch := range parsed.channels {
		d, e := p.Classify(ctx, ch.locator)
		if e != nil {
			if errors.Is(e, remotemedia.ErrPolicy) {
				return ErrNetworkPolicy
			}
			continue
		}
		if d.Covered || d.Public || seen[d.Root] {
			continue
		}
		seen[d.Root] = true
		sameDevice := len(d.Addresses) > 0
		for _, v := range d.Addresses {
			sameDevice = sameDevice && approved[v]
		}
		if sameDevice {
			c.Approvals = append(c.Approvals, remotemedia.Approval{Root: d.Root, Addresses: d.Addresses})
			p.Approvals = c.Approvals
			continue
		}
		need = append(need, d.Root)
		if len(need) > maxConfirmedRoots {
			return ErrNetworkPolicy
		}
	}
	if len(need) > 0 {
		return &LANConfirmationRequired{Roots: need}
	}
	return nil
}

var ErrSourceAuthentication = errors.New("The provider refused access. Check the source credentials.")
var ErrSourceUnavailable = errors.New("The provider could not be read within its response limits. The previous guide is unchanged.")

func (s *Store) ConsumerAccessTx(ctx context.Context, tx *sql.Tx, sid string, owner bool) bool {
	if owner {
		return true
	}
	var access string
	return tx.QueryRowContext(ctx, `SELECT viewer_access FROM live_source_settings WHERE source_id=?`, sid).Scan(&access) == nil && access == "server-members"
}

func sourceHash(in SourceInput) string {
	b, _ := json.Marshal(in.Mappings)
	return id(in.ID, fmt.Sprint(in.ExpectedRevision), in.Name, in.Playlist, in.Guide, fmt.Sprint(in.TunerCount), string(b))
}
