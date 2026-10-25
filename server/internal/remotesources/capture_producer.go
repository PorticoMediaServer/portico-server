package remotesources

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/ownedcapture"
	"portico.local/server/internal/storage"
)

// CaptureAuthorizeTx is the normal playback/content authority callback. It is
// called in the transaction that admits or rechecks a capture; it resolves the
// current actor, item membership and cached hosted/content restrictions. It is
// never a boolean, a role test or a local-owner shortcut.
type CaptureAuthorizeTx func(context.Context, *sql.Tx, identity.Principal, string) (identity.Principal, error)

// CaptureProducerOptions wires the source producer into the existing playback
// entry. The callback is required and is expected to be the existing playback
// authority/family continuation, not a new permission layer.
type CaptureProducerOptions struct {
	Sources     *Service
	Identities  *identity.Service
	AuthorizeTx CaptureAuthorizeTx
}

type captureProducer struct {
	sources    *Service
	identities *identity.Service
	authorize  CaptureAuthorizeTx
}

// NewCaptureProducer builds the source adapter only. The owned artifact store,
// capture journal, receipt and lifecycle stay in ownedcapture.
func NewCaptureProducer(options CaptureProducerOptions) (ownedcapture.Producer, error) {
	if options.Sources == nil || options.Identities == nil || options.AuthorizeTx == nil {
		return nil, storage.ErrRemoteConfig
	}
	return &captureProducer{sources: options.Sources, identities: options.Identities, authorize: options.AuthorizeTx}, nil
}

type captureTarget struct {
	principal identity.Principal
	family    identity.FamilyState

	sourceID         string
	configGeneration string
	mountID          string
	configDigest     string

	librarySourceID   string
	libraryID         string
	incarnation       string
	libraryGeneration int64
	rootIdentity      string

	inventoryObjectID     string
	inventoryRevision     int64
	inventoryRelativePath string
	assetID               string
	itemID                string

	observation  ObservationRef
	scanRevision string
	size         int64
	sequential   bool
	path         string
	remoteRoot   string

	profile  SourceReadProfile
	settings advancedRow
	floor    int64
	applied  bool
}

type capturePhysicalRow struct {
	captureID             string
	sourceID              string
	configGeneration      string
	mountID               string
	librarySourceID       string
	libraryID             string
	incarnation           string
	libraryGeneration     int64
	rootIdentity          string
	inventoryObjectID     string
	inventoryRevision     int64
	inventoryRelativePath string
	configDigest          string
	observationSource     string
	observationGeneration string
	observationObject     string
	observationRevision   int64
	scanRevision          string
	size                  int64
	sequential            bool
}

type captureActorRow struct {
	captureID        string
	serverID         string
	accountID        string
	profileID        string
	familyID         string
	deviceID         string
	authority        string
	role             string
	grant            string
	epoch            int
	itemID           string
	assetID          string
	authorityWitness string
}

type capturePolicyRow struct {
	captureID         string
	profileRevision   int64
	settingsRevision  int64
	cacheBytes        int64
	cacheFloorBytes   int64
	appliedBytes      int64
	appliedFloorBytes int64
	applied           bool
	policyWitness     string
}

type captureChargeRow struct {
	captureID     string
	sourceID      string
	acquisitionID string
	reservationID string
	chargedBytes  int64
	state         string
	volumeKey     string
	createdMs     int64
	expiresMs     int64
	committedMs   int64
}

type captureRows struct {
	charge   captureChargeRow
	physical capturePhysicalRow
	actor    captureActorRow
	policy   capturePolicyRow
}

func capturePolicyWitness(t *captureTarget) string {
	parts := []string{
		"portico.remote-capture.policy.v1", t.sourceID, t.configGeneration, t.mountID,
		t.librarySourceID, t.libraryID, t.incarnation, strconv.FormatInt(t.libraryGeneration, 10), t.rootIdentity,
		t.inventoryObjectID, strconv.FormatInt(t.inventoryRevision, 10), t.observation.Object,
		strconv.FormatInt(t.observation.Revision, 10), t.scanRevision, strconv.FormatInt(t.size, 10),
		strconv.FormatBool(t.sequential), strconv.FormatInt(t.profile.Revision, 10),
		strconv.FormatInt(t.settings.revision, 10), strconv.FormatInt(t.settings.values.CacheBytes, 10),
		strconv.FormatInt(t.settings.values.CacheFloorBytes, 10), strconv.FormatInt(t.settings.appliedBytes, 10),
		strconv.FormatInt(t.settings.appliedFloor, 10), strconv.FormatBool(t.applied),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func (t *captureTarget) binding(captureID, acquisitionID, reservationID string) ownedcapture.Binding {
	return ownedcapture.Binding{
		SourceID:         t.sourceID,
		ConfigGeneration: t.configGeneration,
		ObjectID:         t.inventoryObjectID,
		ObjectRevision:   strconv.FormatInt(t.inventoryRevision, 10),
		AssetID:          t.assetID,
		AcquisitionID:    acquisitionID,
		ReservationID:    reservationID,
		PolicyWitness:    capturePolicyWitness(t),
		AuthorityWitness: captureID,
	}
}

func (p *captureProducer) resolveTargetTx(ctx context.Context, tx *sql.Tx, assetID string) (captureTarget, error) {
	var out captureTarget
	if tx == nil || assetID == "" {
		return out, ownedcapture.ErrInput
	}
	rows, err := tx.QueryContext(ctx, `SELECT o.id,o.revision,o.source_id,o.relative_path,o.asset_id,o.size,
 s.id,s.library_id,s.root,s.generation,s.incarnation,s.root_identity,
 r.root,r.mount_id,r.config
 FROM inventory_objects o
 JOIN library_sources s ON s.id=o.source_id
 JOIN remote_sources r ON r.id=s.remote_source_id
 WHERE o.asset_id=? AND o.retired=0 AND o.state='available' AND s.enabled=1 AND s.kind='remote' AND r.removed=0
 AND EXISTS(SELECT 1 FROM inventory_disc_facts d WHERE d.object_id=o.id AND d.kind IN('bluray','dvd'))
 ORDER BY o.id LIMIT 2`, assetID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	type candidate struct {
		objectID, assetID, sourceID, relative                string
		revision, size                                       int64
		librarySourceID, libraryID, libraryRoot, incarnation string
		libraryGeneration                                    int64
		rootIdentity                                         string
		remoteRoot, mountID                                  string
		config                                               []byte
	}
	list := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.objectID, &c.revision, &c.sourceID, &c.relative, &c.assetID, &c.size,
			&c.librarySourceID, &c.libraryID, &c.libraryRoot, &c.libraryGeneration, &c.incarnation, &c.rootIdentity,
			&c.remoteRoot, &c.mountID, &c.config); err != nil {
			return out, err
		}
		if c.assetID != assetID {
			return out, ownedcapture.ErrCurrent
		}
		list = append(list, c)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(list) != 1 {
		return out, ownedcapture.ErrInput
	}
	c := list[0]
	if c.size <= 0 || !strings.HasSuffix(strings.ToLower(c.relative), ".iso") {
		return out, ownedcapture.ErrInput
	}
	var items []string
	itemRows, err := tx.QueryContext(ctx, `SELECT DISTINCT pid(i.public_id) FROM inventory_objects o
 JOIN catalog_assets a ON a.token=o.asset_id
 JOIN catalog_asset_links ia ON ia.asset_id=a.id
 JOIN catalog_entities i ON i.id=ia.entity_id
 WHERE o.id=? AND i.retired=0 LIMIT 2`, c.objectID)
	if err != nil {
		return out, err
	}
	for itemRows.Next() {
		var id string
		if err = itemRows.Scan(&id); err != nil {
			itemRows.Close()
			return out, err
		}
		items = append(items, id)
	}
	itemRows.Close()
	if err = itemRows.Err(); err != nil {
		return out, err
	}
	if len(items) != 1 {
		return out, ownedcapture.ErrInput
	}
	out.inventoryObjectID = c.objectID
	out.inventoryRevision = c.revision
	out.inventoryRelativePath = filepath.ToSlash(c.relative)
	out.assetID = assetID
	out.itemID = items[0]
	out.sourceID = c.sourceID
	out.libraryID = c.libraryID
	out.librarySourceID = c.librarySourceID
	out.incarnation = c.incarnation
	out.libraryGeneration = c.libraryGeneration
	out.rootIdentity = c.rootIdentity
	out.remoteRoot = c.remoteRoot
	out.mountID = c.mountID
	out.path = filepath.Join(c.libraryRoot, filepath.FromSlash(c.relative))
	out.size = c.size
	rel, err := filepath.Rel(filepath.Clean(c.remoteRoot), filepath.Clean(out.path))
	if err != nil || !filepath.IsLocal(rel) {
		return out, ownedcapture.ErrInput
	}
	out.observation.Object = filepath.ToSlash(rel)
	generation, err := observationGenerationTx(ctx, tx, c.sourceID)
	if err != nil {
		return out, err
	}
	out.configGeneration = generation
	out.observation.Generation = generation
	sum := sha256.Sum256(c.config)
	out.configDigest = hex.EncodeToString(sum[:])
	if out.observation.Generation != generation {
		return out, storage.ErrRemoteChanged
	}
	scan, err := readScanObservationTx(ctx, tx, c.sourceID, out.observation.Object)
	if err != nil {
		return out, err
	}
	if scan.Ref.SourceID != c.sourceID || scan.Ref.Generation != generation || scan.Ref.Revision < 1 || scan.Observation.Facts.Size != c.size {
		return out, storage.ErrRemoteChanged
	}
	out.observation = scan.Ref
	out.scanRevision = scan.Observation.ScanRevision
	profile, err := SourceReadProfileTx(ctx, tx, c.sourceID)
	if err != nil {
		return out, err
	}
	out.profile = profile
	settings, err := readAdvancedTx(ctx, tx, c.sourceID)
	if err != nil {
		return out, err
	}
	out.settings = settings
	if !settings.saved || settings.values.CacheBytes <= 0 {
		return out, storage.ErrRemoteLimit
	}
	out.applied = settings.saved
	if c.mountID != "" {
		out.applied = settings.appliedBytes == settings.values.CacheBytes && (settings.values.CacheFloorBytes == 0 || settings.appliedFloor == settings.values.CacheFloorBytes)
		if !out.applied {
			return out, storage.ErrRemoteLimit
		}
	}
	out.floor = settings.values.CacheFloorBytes
	if out.floor == 0 && c.mountID != "" {
		out.floor = settings.appliedFloor
	}
	sequential, err := sourceObjectSequentialTx(ctx, tx, c.sourceID, out.path)
	if err != nil {
		return out, err
	}
	if !sequential {
		return out, storage.ErrRemoteRange
	}
	out.sequential = true
	return out, nil
}

func captureRowTx(ctx context.Context, tx *sql.Tx, captureID string) (captureRows, error) {
	var out captureRows
	if tx == nil || captureID == "" {
		return out, ownedcapture.ErrInput
	}
	err := tx.QueryRowContext(ctx, `SELECT capture_id,source_id,acquisition_id,reservation_id,charged_bytes,state,volume_key,created_ms,expires_ms,committed_ms FROM remote_source_captures WHERE capture_id=?`, captureID).Scan(
		&out.charge.captureID, &out.charge.sourceID, &out.charge.acquisitionID, &out.charge.reservationID, &out.charge.chargedBytes, &out.charge.state, &out.charge.volumeKey, &out.charge.createdMs, &out.charge.expiresMs, &out.charge.committedMs)
	if err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `SELECT capture_id,source_id,config_generation,mount_id,library_source_id,library_id,incarnation,library_generation,root_identity,inventory_object_id,inventory_revision,inventory_relative_path,config_digest,observation_source,observation_generation,observation_object,observation_revision,scan_revision,size,sequential FROM remote_source_capture_physical WHERE capture_id=?`, captureID).Scan(
		&out.physical.captureID, &out.physical.sourceID, &out.physical.configGeneration, &out.physical.mountID, &out.physical.librarySourceID, &out.physical.libraryID, &out.physical.incarnation, &out.physical.libraryGeneration, &out.physical.rootIdentity, &out.physical.inventoryObjectID, &out.physical.inventoryRevision, &out.physical.inventoryRelativePath, &out.physical.configDigest, &out.physical.observationSource, &out.physical.observationGeneration, &out.physical.observationObject, &out.physical.observationRevision, &out.physical.scanRevision, &out.physical.size, &out.physical.sequential)
	if err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `SELECT capture_id,server_id,account_id,profile_id,family_id,device_id,authority,role,grant,epoch,item_id,asset_id,authority_witness FROM remote_source_capture_actor WHERE capture_id=?`, captureID).Scan(
		&out.actor.captureID, &out.actor.serverID, &out.actor.accountID, &out.actor.profileID, &out.actor.familyID, &out.actor.deviceID, &out.actor.authority, &out.actor.role, &out.actor.grant, &out.actor.epoch, &out.actor.itemID, &out.actor.assetID, &out.actor.authorityWitness)
	if err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `SELECT capture_id,profile_revision,cache_settings_revision,cache_bytes,cache_floor_bytes,applied_bytes,applied_floor_bytes,applied,policy_witness FROM remote_source_capture_policy WHERE capture_id=?`, captureID).Scan(
		&out.policy.captureID, &out.policy.profileRevision, &out.policy.settingsRevision, &out.policy.cacheBytes, &out.policy.cacheFloorBytes, &out.policy.appliedBytes, &out.policy.appliedFloorBytes, &out.policy.applied, &out.policy.policyWitness)
	return out, err
}

func sameCaptureBinding(b ownedcapture.Binding, rows captureRows) bool {
	return b.SourceID == rows.charge.sourceID &&
		b.SourceID == rows.physical.sourceID &&
		b.ConfigGeneration == rows.physical.configGeneration &&
		b.ObjectID == rows.physical.inventoryObjectID &&
		b.ObjectRevision == strconv.FormatInt(rows.physical.inventoryRevision, 10) &&
		b.AssetID == rows.actor.assetID &&
		b.AcquisitionID == rows.charge.acquisitionID &&
		b.ReservationID == rows.charge.reservationID &&
		b.PolicyWitness == rows.policy.policyWitness &&
		b.AuthorityWitness == rows.actor.authorityWitness
}

func sameTarget(a, b captureTarget) bool {
	return a.sourceID == b.sourceID && a.configGeneration == b.configGeneration && a.mountID == b.mountID &&
		a.librarySourceID == b.librarySourceID && a.libraryID == b.libraryID && a.incarnation == b.incarnation &&
		a.libraryGeneration == b.libraryGeneration && a.rootIdentity == b.rootIdentity &&
		a.inventoryObjectID == b.inventoryObjectID && a.inventoryRevision == b.inventoryRevision &&
		a.assetID == b.assetID && a.itemID == b.itemID && a.observation == b.observation &&
		a.scanRevision == b.scanRevision && a.size == b.size && a.profile.Revision == b.profile.Revision &&
		a.settings.revision == b.settings.revision && a.settings.values.CacheBytes == b.settings.values.CacheBytes &&
		a.settings.values.CacheFloorBytes == b.settings.values.CacheFloorBytes && a.settings.appliedBytes == b.settings.appliedBytes &&
		a.settings.appliedFloor == b.settings.appliedFloor && a.applied == b.applied && a.floor == b.floor && a.sequential == b.sequential
}

func (p *captureProducer) checkTargetTx(ctx context.Context, tx *sql.Tx, t captureTarget) error {
	if tx == nil {
		return ownedcapture.ErrInput
	}
	var objectID, sourceID, assetID string
	var revision int64
	var retired, enabled, removed int
	var state, relative string
	var size int64
	var librarySourceID, libraryID, libraryRoot, incarnation, rootIdentity string
	var libraryGeneration int64
	var mountID, remoteRoot string
	var config []byte
	err := tx.QueryRowContext(ctx, `SELECT o.id,o.revision,o.source_id,o.asset_id,o.retired,o.state,o.relative_path,o.size,
 s.id,s.library_id,s.root,s.generation,s.incarnation,s.root_identity,s.enabled,
 r.root,r.mount_id,r.removed,r.config
 FROM inventory_objects o
 JOIN library_sources s ON s.id=o.source_id
 JOIN remote_sources r ON r.id=s.remote_source_id
 WHERE o.id=?`, t.inventoryObjectID).Scan(&objectID, &revision, &sourceID, &assetID, &retired, &state, &relative, &size,
		&librarySourceID, &libraryID, &libraryRoot, &libraryGeneration, &incarnation, &rootIdentity, &enabled,
		&remoteRoot, &mountID, &removed, &config)
	if err != nil {
		return ownedcapture.ErrCurrent
	}
	if objectID != t.inventoryObjectID || revision != t.inventoryRevision || sourceID != t.sourceID || assetID != t.assetID ||
		retired != 0 || state != "available" || !strings.EqualFold(filepath.Ext(relative), ".iso") || relative != t.inventoryRelativePath || size != t.size ||
		librarySourceID != t.librarySourceID || libraryID != t.libraryID || libraryRoot != "" && filepath.Join(libraryRoot, filepath.FromSlash(relative)) != t.path ||
		libraryGeneration != t.libraryGeneration || incarnation != t.incarnation || rootIdentity != t.rootIdentity || enabled != 1 ||
		remoteRoot != t.remoteRoot || mountID != t.mountID || removed != 0 {
		return ownedcapture.ErrCurrent
	}
	sum := sha256.Sum256(config)
	if hex.EncodeToString(sum[:]) != t.configDigest {
		return ownedcapture.ErrCurrent
	}
	generation, err := observationGenerationTx(ctx, tx, t.sourceID)
	if err != nil || generation != t.configGeneration {
		return ownedcapture.ErrCurrent
	}
	profile, err := SourceReadProfileTx(ctx, tx, t.sourceID)
	if err != nil || profile.Revision != t.profile.Revision {
		return ownedcapture.ErrCurrent
	}
	settings, err := readAdvancedTx(ctx, tx, t.sourceID)
	if err != nil || settings.revision != t.settings.revision || settings.values.CacheBytes != t.settings.values.CacheBytes ||
		settings.values.CacheFloorBytes != t.settings.values.CacheFloorBytes || settings.appliedBytes != t.settings.appliedBytes ||
		settings.appliedFloor != t.settings.appliedFloor || settings.saved != t.settings.saved {
		return ownedcapture.ErrCurrent
	}
	applied := settings.saved
	if t.mountID != "" {
		applied = settings.appliedBytes == settings.values.CacheBytes && (settings.values.CacheFloorBytes == 0 || settings.appliedFloor == settings.values.CacheFloorBytes)
	}
	if applied != t.applied {
		return ownedcapture.ErrCurrent
	}
	scan, err := readScanObservationTx(ctx, tx, t.sourceID, t.observation.Object)
	if err != nil || scan.Ref != t.observation || scan.Observation.ScanRevision != t.scanRevision || scan.Observation.Facts.Size != t.size {
		return ownedcapture.ErrCurrent
	}
	var disc int
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_disc_facts WHERE object_id=? AND kind IN('bluray','dvd'))`, t.inventoryObjectID).Scan(&disc); err != nil || disc != 1 {
		return ownedcapture.ErrCurrent
	}
	sequential, err := sourceObjectSequentialTx(ctx, tx, t.sourceID, t.path)
	if err != nil || !sequential {
		return ownedcapture.ErrCurrent
	}
	itemRows, err := tx.QueryContext(ctx, `SELECT DISTINCT pid(i.public_id) FROM inventory_objects o
 JOIN catalog_assets a ON a.token=o.asset_id
 JOIN catalog_asset_links ia ON ia.asset_id=a.id
 JOIN catalog_entities i ON i.id=ia.entity_id
 WHERE o.id=? AND i.retired=0 LIMIT 2`, t.inventoryObjectID)
	if err != nil {
		return ownedcapture.ErrCurrent
	}
	items := []string{}
	for itemRows.Next() {
		var itemID string
		if err = itemRows.Scan(&itemID); err != nil {
			itemRows.Close()
			return ownedcapture.ErrCurrent
		}
		items = append(items, itemID)
	}
	itemRows.Close()
	if err = itemRows.Err(); err != nil || len(items) != 1 || items[0] != t.itemID {
		return ownedcapture.ErrCurrent
	}
	return nil
}

func (p *captureProducer) checkActorTx(ctx context.Context, tx *sql.Tx, a captureActorRow) (identity.Principal, error) {
	expected := identity.Principal{
		Viewer: identity.Viewer{ServerID: a.serverID, AccountID: a.accountID, ProfileID: a.profileID, Authority: a.authority, Role: a.role},
		Epoch:  a.epoch, DeviceID: a.deviceID, Grant: a.grant,
	}
	family, err := p.identities.FamilyAuthorityTx(ctx, tx, a.familyID, expected)
	if err != nil {
		return identity.Principal{}, ownedcapture.ErrCurrent
	}
	if family.ID != a.familyID || family.DeviceID != a.deviceID {
		return identity.Principal{}, ownedcapture.ErrCurrent
	}
	var currentToken string
	if err = tx.QueryRowContext(ctx, `SELECT t.token_hash FROM authorization_session_families f JOIN authorization_family_tokens t ON t.family_id=f.id AND t.generation=f.current_generation WHERE f.id=? AND f.revoked=0 AND t.retired=0`, a.familyID).Scan(&currentToken); err != nil {
		return identity.Principal{}, ownedcapture.ErrCurrent
	}
	expected.Hash = currentToken
	current, err := p.authorize(ctx, tx, expected, a.itemID)
	if err != nil {
		return identity.Principal{}, err
	}
	if current.ServerID != a.serverID || current.AccountID != a.accountID || current.ProfileID != a.profileID ||
		current.Authority != a.authority || current.Role != a.role || current.Grant != a.grant || current.Epoch != a.epoch {
		return identity.Principal{}, ownedcapture.ErrCurrent
	}
	return current, nil
}

func (p *captureProducer) checkBindingTx(ctx context.Context, tx *sql.Tx, b ownedcapture.Binding) error {
	if tx == nil || b.AuthorityWitness == "" {
		return ownedcapture.ErrInput
	}
	rows, err := captureRowTx(ctx, tx, b.AuthorityWitness)
	if err != nil {
		return err
	}
	if !sameCaptureBinding(b, rows) {
		return ownedcapture.ErrCurrent
	}
	if rows.charge.state != "pending" && rows.charge.state != "committed" {
		return ownedcapture.ErrCurrent
	}
	if rows.charge.expiresMs <= time.Now().UnixMilli() {
		return ownedcapture.ErrCurrent
	}
	target := captureTarget{
		sourceID: rows.physical.sourceID, configGeneration: rows.physical.configGeneration, mountID: rows.physical.mountID,
		configDigest: rows.physical.configDigest, librarySourceID: rows.physical.librarySourceID, libraryID: rows.physical.libraryID,
		incarnation: rows.physical.incarnation, libraryGeneration: rows.physical.libraryGeneration, rootIdentity: rows.physical.rootIdentity,
		inventoryObjectID: rows.physical.inventoryObjectID, inventoryRevision: rows.physical.inventoryRevision,
		inventoryRelativePath: rows.physical.inventoryRelativePath, assetID: rows.actor.assetID, itemID: rows.actor.itemID,
		observation:  ObservationRef{SourceID: rows.physical.observationSource, Generation: rows.physical.observationGeneration, Object: rows.physical.observationObject, Revision: rows.physical.observationRevision},
		scanRevision: rows.physical.scanRevision, size: rows.physical.size, sequential: rows.physical.sequential,
		profile:  SourceReadProfile{SourceID: rows.physical.sourceID, Revision: rows.policy.profileRevision},
		settings: advancedRow{saved: true, revision: rows.policy.settingsRevision, values: SourceAdvanced{CacheBytes: rows.policy.cacheBytes, CacheFloorBytes: rows.policy.cacheFloorBytes}, appliedBytes: rows.policy.appliedBytes, appliedFloor: rows.policy.appliedFloorBytes},
		applied:  rows.policy.applied, floor: rows.policy.cacheFloorBytes,
	}
	// The physical path is private source data, reconstructed from the exact
	// inventory row and library source; it is never part of Binding.
	var root string
	if err = tx.QueryRowContext(ctx, `SELECT s.root FROM library_sources s WHERE s.id=? AND s.library_id=? AND s.enabled=1`, rows.physical.librarySourceID, rows.physical.libraryID).Scan(&root); err != nil {
		return ownedcapture.ErrCurrent
	}
	target.path = filepath.Join(root, filepath.FromSlash(rows.physical.inventoryRelativePath))
	var remoteRoot string
	if err = tx.QueryRowContext(ctx, `SELECT root FROM remote_sources WHERE id=? AND removed=0`, rows.physical.sourceID).Scan(&remoteRoot); err != nil {
		return ownedcapture.ErrCurrent
	}
	target.remoteRoot = remoteRoot
	if capturePolicyWitness(&target) != b.PolicyWitness {
		return ownedcapture.ErrCurrent
	}
	if err = p.checkTargetTx(ctx, tx, target); err != nil {
		return err
	}
	_, err = p.checkActorTx(ctx, tx, rows.actor)
	return err
}

func (p *captureProducer) CheckTx(ctx context.Context, tx *sql.Tx, b ownedcapture.Binding) error {
	return p.checkBindingTx(ctx, tx, b)
}

type captureAcquisition struct {
	producer  *captureProducer
	captureID string
	binding   ownedcapture.Binding
	lease     *SourceReadLease
	ctx       context.Context
	cancel    context.CancelFunc
	size      int64
	expires   time.Time
	disk      *diskGrowthReservation
	mu        sync.Mutex
	seq       *SourceSequential
	persisted int64
	closed    bool
}

func (a *captureAcquisition) Context() context.Context      { return a.ctx }
func (a *captureAcquisition) Binding() ownedcapture.Binding { return a.binding }
func (a *captureAcquisition) Size() int64                   { return a.size }
func (a *captureAcquisition) ExpiresAt() time.Time          { return a.expires }
func (a *captureAcquisition) Check(ctx context.Context) error {
	if err := a.lease.Check(ctx); err != nil {
		return err
	}
	view, err := dbwork.BeginSnapshot(ctx, a.producer.sources.db)
	if err != nil {
		return err
	}
	err = a.producer.CheckTx(ctx, view.Tx(), a.binding)
	view.Rollback()
	if err != nil {
		return err
	}
	a.mu.Lock()
	seq := a.seq
	a.mu.Unlock()
	if seq != nil {
		return seq.Check(ctx)
	}
	return nil
}
func (a *captureAcquisition) CheckTx(ctx context.Context, tx *sql.Tx) error {
	if err := a.lease.CheckTx(ctx, tx); err != nil {
		return err
	}
	return a.producer.CheckTx(ctx, tx, a.binding)
}
func (a *captureAcquisition) Read(p []byte) (int, error) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return 0, storage.ErrRemoteOffline
	}
	seq := a.seq
	a.mu.Unlock()
	if seq == nil {
		next, err := a.lease.OpenSequential(a.ctx)
		if err != nil {
			return 0, err
		}
		a.mu.Lock()
		a.seq = next
		seq = next
		a.mu.Unlock()
	}
	n, err := seq.Read(p)
	if err != nil {
		a.mu.Lock()
		if a.seq == seq {
			_ = seq.Close()
			a.seq = nil
		}
		a.mu.Unlock()
	}
	return n, err
}
func (a *captureAcquisition) Persisted(ctx context.Context, total int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if total < a.persisted || total < 0 || total > a.size {
		return ownedcapture.ErrIncomplete
	}
	delta := total - a.persisted
	a.persisted = total
	if delta > 0 && a.disk != nil {
		a.disk.settle(delta)
	}
	return nil
}
func (a *captureAcquisition) CommitTx(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return storage.ErrRemoteConfig
	}
	return a.producer.commitCaptureTx(ctx, tx, a.captureID, a.binding)
}
func (a *captureAcquisition) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	seq := a.seq
	a.seq = nil
	lease := a.lease
	a.mu.Unlock()
	if seq != nil {
		_ = seq.Close()
	}
	if lease != nil {
		_ = lease.Close()
	}
	if a.cancel != nil {
		a.cancel()
	}
	a.producer.sources.forgetCaptureActive(a.captureID)
	return nil
}

func (p *captureProducer) commitCaptureTx(ctx context.Context, tx *sql.Tx, captureID string, b ownedcapture.Binding) error {
	if err := p.checkBindingTx(ctx, tx, b); err != nil {
		return err
	}
	var state string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM remote_source_captures WHERE capture_id=?`, captureID).Scan(&state); err != nil {
		return ownedcapture.ErrCurrent
	}
	if state == "committed" {
		return nil
	}
	if state != "pending" {
		return ownedcapture.ErrCurrent
	}
	result, err := tx.ExecContext(ctx, `UPDATE remote_source_captures SET state='committed',committed_ms=? WHERE capture_id=? AND state='pending'`, time.Now().UnixMilli(), captureID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ownedcapture.ErrCurrent
	}
	return nil
}

type captureSourceLease struct {
	producer *captureProducer
	binding  ownedcapture.Binding
	ctx      context.Context
}

func (l *captureSourceLease) Context() context.Context { return l.ctx }
func (l *captureSourceLease) Check(ctx context.Context) error {
	view, err := dbwork.BeginSnapshot(ctx, l.producer.sources.db)
	if err != nil {
		return err
	}
	defer view.Rollback()
	return l.producer.CheckTx(ctx, view.Tx(), l.binding)
}
func (l *captureSourceLease) CheckTx(ctx context.Context, tx *sql.Tx) error {
	return l.producer.CheckTx(ctx, tx, l.binding)
}
func (l *captureSourceLease) Close() error { return nil }

func (p *captureProducer) Lease(ctx context.Context, b ownedcapture.Binding) (ownedcapture.SourceLease, error) {
	view, err := dbwork.BeginSnapshot(ctx, p.sources.db)
	if err != nil {
		return nil, err
	}
	defer view.Rollback()
	rows, err := captureRowTx(ctx, view.Tx(), b.AuthorityWitness)
	if err != nil {
		return nil, err
	}
	if !sameCaptureBinding(b, rows) || rows.charge.state != "committed" || rows.charge.expiresMs <= time.Now().UnixMilli() {
		return nil, ownedcapture.ErrCurrent
	}
	if err = p.CheckTx(ctx, view.Tx(), b); err != nil {
		return nil, err
	}
	return &captureSourceLease{producer: p, binding: b, ctx: ctx}, nil
}

func (p *captureProducer) ReleaseTx(ctx context.Context, tx *sql.Tx, b ownedcapture.Binding) error {
	if tx == nil || b.AuthorityWitness == "" {
		return ownedcapture.ErrInput
	}
	rows, err := captureRowTx(ctx, tx, b.AuthorityWitness)
	if err != nil {
		return err
	}
	if !sameCaptureBinding(b, rows) {
		return ownedcapture.ErrCurrent
	}
	for _, query := range []string{
		`DELETE FROM remote_source_capture_physical WHERE capture_id=?`,
		`DELETE FROM remote_source_capture_actor WHERE capture_id=?`,
		`DELETE FROM remote_source_capture_policy WHERE capture_id=?`,
		`DELETE FROM remote_source_captures WHERE capture_id=?`,
	} {
		if _, err = tx.ExecContext(ctx, query, rows.charge.captureID); err != nil {
			return err
		}
	}
	p.sources.forgetCaptureActive(rows.charge.captureID)
	p.sources.forgetCaptureGrowth(rows.charge.captureID)
	return nil
}

func captureUsageRead(ctx context.Context, service *Service, source string) (prefix, capture int64, err error) {
	view, err := dbwork.BeginSnapshot(ctx, service.db)
	if err != nil {
		return 0, 0, err
	}
	defer view.Rollback()
	err = view.Tx().QueryRowContext(ctx, `SELECT COALESCE((SELECT SUM(used_bytes) FROM remote_source_cache_entries WHERE source_id=?1),0),COALESCE((SELECT capture_bytes FROM remote_source_capture_quota WHERE source_id=?1),0)`, source).Scan(&prefix, &capture)
	if prefix < 0 || capture < 0 {
		return 0, 0, storage.ErrRemoteLimit
	}
	return prefix, capture, err
}

func captureCeilingAllows(prefix, reserved, capture, growth, limit int64) bool {
	if prefix < 0 || reserved < 0 || capture < 0 || growth <= 0 || limit <= 0 {
		return false
	}
	if prefix > limit || reserved > limit || capture > limit {
		return false
	}
	remaining := limit - prefix
	if reserved > remaining {
		return false
	}
	remaining -= reserved
	if capture > remaining {
		return false
	}
	remaining -= capture
	return growth <= remaining
}

func (p *captureProducer) Admit(ctx context.Context, captureID string, request ownedcapture.Request, dest ownedcapture.Destination) (ownedcapture.Acquisition, error) {
	if ctx == nil || len(captureID) != 64 || dbwork.InWriteTx(ctx) {
		return nil, ownedcapture.ErrInput
	}
	if _, err := hex.DecodeString(captureID); err != nil {
		return nil, ownedcapture.ErrInput
	}
	// Preliminary read-only resolution chooses the source budget before any
	// writer is opened. The writer below re-proves the exact same target.
	view, err := dbwork.BeginSnapshot(ctx, p.sources.db)
	if err != nil {
		return nil, err
	}
	first, err := p.resolveTargetTx(ctx, view.Tx(), request.AssetID)
	view.Rollback()
	if err != nil {
		return nil, err
	}
	budget := p.sources.budgetFor(first.sourceID)
	budget.mu.Lock()
	gated, err := dbwork.Begin(ctx, p.sources.db, dbwork.ClassPlaybackStart)
	if err != nil {
		budget.mu.Unlock()
		return nil, err
	}
	defer gated.Rollback()
	committed := false
	activeSet := false
	var lease *SourceReadLease
	var disk *diskGrowthReservation
	var growthBytes int64
	defer func() {
		if committed {
			return
		}
		if lease != nil {
			_ = lease.Close()
		}
		if activeSet {
			p.sources.forgetCaptureActive(captureID)
			p.sources.forgetCaptureGrowth(captureID)
		}
		if disk != nil {
			disk.settle(growthBytes)
		}
		budget.mu.Unlock()
	}()
	target, err := p.resolveTargetTx(ctx, gated.Tx(), request.AssetID)
	if err != nil || !sameTarget(first, target) {
		return nil, ownedcapture.ErrCurrent
	}
	p.sources.cacheMu.Lock()
	currentBudget := p.sources.cacheBudgets[target.sourceID]
	p.sources.cacheMu.Unlock()
	if currentBudget != budget {
		return nil, ownedcapture.ErrCurrent
	}
	family, err := p.identities.SessionFamilyTx(ctx, gated.Tx(), request.Principal)
	if err != nil {
		return nil, ownedcapture.ErrCurrent
	}
	if request.Principal.DeviceID != "" && request.Principal.DeviceID != family.DeviceID {
		return nil, ownedcapture.ErrCurrent
	}
	authorized, err := p.authorize(ctx, gated.Tx(), request.Principal, target.itemID)
	if err != nil {
		return nil, err
	}
	if authorized.ServerID != request.Principal.ServerID || authorized.AccountID != request.Principal.AccountID ||
		authorized.ProfileID != request.Principal.ProfileID || authorized.Authority != request.Principal.Authority ||
		authorized.Role != request.Principal.Role || authorized.Grant != request.Principal.Grant || authorized.Epoch != request.Principal.Epoch {
		return nil, ownedcapture.ErrCurrent
	}
	if authorized.DeviceID != "" && family.DeviceID != "" && authorized.DeviceID != family.DeviceID {
		return nil, ownedcapture.ErrCurrent
	}
	target.principal = authorized
	target.family = family
	expires := request.Principal.ExpiresAt
	if !family.AuthorizationHorizon.IsZero() && (expires.IsZero() || family.AuthorizationHorizon.Before(expires)) {
		expires = family.AuthorizationHorizon
	}
	if expires.IsZero() || !expires.After(time.Now()) {
		return nil, ownedcapture.ErrCurrent
	}
	if limit := time.Now().Add(ownedcapture.MaxCaptureLifetime); expires.After(limit) {
		expires = limit
	}
	admission := SourceSessionAdmission{
		Purpose: SourcePlayback, Observation: target.observation, OnChanged: func() {},
		CheckTx: func(check context.Context, tx *sql.Tx, binding SourceReadBinding) error {
			if binding.Purpose != SourcePlayback || binding.Observation != target.observation || binding.Path != target.path ||
				binding.Root != target.remoteRoot || binding.ScanRevision != target.scanRevision || binding.Size != target.size || !binding.Sequential {
				return storage.ErrRemoteChanged
			}
			if err := p.checkTargetTx(check, tx, target); err != nil {
				return err
			}
			if _, err := p.authorize(check, tx, target.principal, target.itemID); err != nil {
				return err
			}
			return nil
		},
	}
	lease, err = p.sources.CaptureReadSourceTx(ctx, gated.Tx(), admission)
	if err != nil {
		return nil, err
	}
	prefix, capture, err := captureUsageRead(ctx, p.sources, target.sourceID)
	if err != nil {
		return nil, err
	}
	if !captureCeilingAllows(prefix, budget.reserved, capture, target.size, target.settings.values.CacheBytes) {
		return nil, storage.ErrRemoteLimit
	}
	volume, err := dest.VolumeKey()
	if err != nil {
		return nil, err
	}
	growthBytes = target.size
	disk = reserveVolumeGrowth(volume, target.floor, growthBytes, func(floor, total int64) bool {
		return dest.SpaceFor(floor, total)
	})
	if disk == nil {
		return nil, storage.ErrRemoteLimit
	}
	acquisitionID := identity.Token()
	reservationID := identity.Token()
	binding := target.binding(captureID, acquisitionID, reservationID)
	now := time.Now().UnixMilli()
	if _, err = gated.Tx().ExecContext(ctx, `INSERT INTO remote_source_captures(capture_id,source_id,acquisition_id,reservation_id,charged_bytes,state,volume_key,created_ms,expires_ms,committed_ms) VALUES(?,?,?,?,?, 'pending',?,?,?,0)`,
		captureID, target.sourceID, acquisitionID, reservationID, target.size, volume, now, expires.UnixMilli()); err != nil {
		return nil, err
	}
	if _, err = gated.Tx().ExecContext(ctx, `INSERT INTO remote_source_capture_physical(capture_id,source_id,config_generation,mount_id,library_source_id,library_id,incarnation,library_generation,root_identity,inventory_object_id,inventory_revision,inventory_relative_path,config_digest,observation_source,observation_generation,observation_object,observation_revision,scan_revision,size,sequential) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,1)`,
		captureID, target.sourceID, target.configGeneration, target.mountID, target.librarySourceID, target.libraryID, target.incarnation, target.libraryGeneration, target.rootIdentity, target.inventoryObjectID, target.inventoryRevision, target.inventoryRelativePath, target.configDigest, target.observation.SourceID, target.observation.Generation, target.observation.Object, target.observation.Revision, target.scanRevision, target.size); err != nil {
		return nil, err
	}
	if _, err = gated.Tx().ExecContext(ctx, `INSERT INTO remote_source_capture_actor(capture_id,server_id,account_id,profile_id,family_id,device_id,authority,role,grant,epoch,item_id,asset_id,authority_witness) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		captureID, target.principal.ServerID, target.principal.AccountID, target.principal.ProfileID, family.ID, family.DeviceID, target.principal.Authority, target.principal.Role, target.principal.Grant, target.principal.Epoch, target.itemID, target.assetID, captureID); err != nil {
		return nil, err
	}
	if _, err = gated.Tx().ExecContext(ctx, `INSERT INTO remote_source_capture_policy(capture_id,profile_revision,cache_settings_revision,cache_bytes,cache_floor_bytes,applied_bytes,applied_floor_bytes,applied,policy_witness) VALUES(?,?,?,?,?,?,?,?,?)`,
		captureID, target.profile.Revision, target.settings.revision, target.settings.values.CacheBytes, target.settings.values.CacheFloorBytes, target.settings.appliedBytes, target.settings.appliedFloor, boolInt(target.applied), binding.PolicyWitness); err != nil {
		return nil, err
	}
	// The active map closes the tiny Admit-commit/builder-journal gap: bounded
	// recovery skips this charge until the original acquisition closes.
	p.sources.rememberCaptureActive(captureID)
	p.sources.rememberCaptureGrowth(captureID, disk)
	activeSet = true
	if err = gated.Commit(); err != nil {
		return nil, err
	}
	committed = true
	budget.used = prefix
	budget.limit = target.settings.values.CacheBytes
	budget.loaded = true
	budget.mu.Unlock()
	return &captureAcquisition{producer: p, captureID: captureID, binding: binding, lease: lease, ctx: ctx, size: target.size, expires: time.UnixMilli(expires.UnixMilli()).UTC(), disk: disk}, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *Service) rememberCaptureActive(captureID string) {
	s.captureMu.Lock()
	if s.capturesActive == nil {
		s.capturesActive = map[string]bool{}
	}
	s.capturesActive[captureID] = true
	s.captureMu.Unlock()
}

func (s *Service) forgetCaptureActive(captureID string) {
	s.captureMu.Lock()
	delete(s.capturesActive, captureID)
	s.captureMu.Unlock()
}

func (s *Service) captureIsActive(captureID string) bool {
	s.captureMu.Lock()
	defer s.captureMu.Unlock()
	return s.capturesActive[captureID]
}

func (s *Service) rememberCaptureGrowth(captureID string, disk *diskGrowthReservation) {
	if disk == nil {
		return
	}
	s.captureMu.Lock()
	if s.captureGrowth == nil {
		s.captureGrowth = map[string]*diskGrowthReservation{}
	}
	s.captureGrowth[captureID] = disk
	s.captureMu.Unlock()
}

func (s *Service) forgetCaptureGrowth(captureID string) {
	s.captureMu.Lock()
	disk := s.captureGrowth[captureID]
	delete(s.captureGrowth, captureID)
	s.captureMu.Unlock()
	if disk != nil {
		disk.settle(int64(^uint64(0) >> 1))
	}
}

// recoverCaptureCharges releases only a pending source charge whose builder
// journal row never existed and whose original acquisition is no longer active.
// The matrix writer then the CAS in the delete prevents a racing builder
// CheckTx/journal insert from starting scratch after the uncharge. It never
// touches artifacts and never reopens a provider.
func (s *Service) recoverCaptureCharges(ctx context.Context) {
	if s == nil || s.db == nil {
		return
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassMaintenance)
	if err != nil {
		return
	}
	defer gated.Rollback()
	rows, err := gated.Tx().QueryContext(ctx, `SELECT c.capture_id FROM remote_source_captures c WHERE c.state='pending' AND NOT EXISTS(SELECT 1 FROM owned_capture_sources o WHERE o.capture_id=c.capture_id) ORDER BY c.capture_id LIMIT 32`)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return
		}
		if !s.captureIsActive(id) {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return
	}
	released := []string{}
	for _, id := range ids {
		result, err := gated.Tx().ExecContext(ctx, `DELETE FROM remote_source_captures WHERE capture_id=? AND state='pending' AND NOT EXISTS(SELECT 1 FROM owned_capture_sources WHERE capture_id=?)`, id, id)
		if err != nil {
			return
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			continue
		}
		for _, query := range []string{
			`DELETE FROM remote_source_capture_physical WHERE capture_id=?`,
			`DELETE FROM remote_source_capture_actor WHERE capture_id=?`,
			`DELETE FROM remote_source_capture_policy WHERE capture_id=?`,
		} {
			if _, err = gated.Tx().ExecContext(ctx, query, id); err != nil {
				return
			}
		}
		released = append(released, id)
	}
	if err = gated.Commit(); err != nil {
		return
	}
	for _, id := range released {
		s.forgetCaptureGrowth(id)
	}
}
