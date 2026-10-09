package playback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/decoder"
	"portico.local/server/internal/entityid"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/preparedmedia"
	"portico.local/server/internal/segmentmarkers"
	"portico.local/server/internal/subtitles"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ErrIncompatible = errors.New("source requires a playback conversion unavailable on this server")

// ErrGrantEnded refuses a media grant whose presentation is over: ended,
// stopped, failed, expired, revoked, superseded by a new sign-in, or a prepared
// track outside its audio route. A grant is a capability, not a credential, so
// this is a hidden 404 (presentation_ended) the player acts on by asking for a
// new presentation, never a 401 that sends the client to sign in (NEW-35).
var ErrGrantEnded = errors.New("This playback has ended. Start it again.")

type Session struct {
	// AudioPlan is the version 2 plan (spec §18.1: the client decodes), pinned
	// for Playback v1 audio presentations.
	AudioPlan         *AudioPlan `json:"audioPlan,omitempty"`
	PreparedVersionID string     `json:"preparedVersionId,omitempty"`
	SourceBoundary    string     `json:"sourceBoundary,omitempty"`
	Error             string     `json:"error,omitempty"`
	ID                string     `json:"id"`
	Generation        int        `json:"generation"`
	StreamURL         string     `json:"streamUrl"`
	Mode              string     `json:"mode"`
	Duration          float64    `json:"duration"`
	ResumeSeconds     float64    `json:"resumeSeconds"`
	State             string     `json:"state,omitempty"`
}
type Service struct {
	// prefetched counts bytes served to private presentations (audio_plan_v2.go).
	prefetchMu   sync.Mutex
	prefetched   map[string]int64
	db           *sql.DB
	hls          *HLS
	remote       *Remote
	StorageGuard func(string) error
	// AuthorityTx is wired by HTTP composition, never a remote policy lookup.
	AuthorityTx         ControlAuthorityTx
	ContinueAuthorityTx ControlAuthorityTx
	PersonalProgress    func(*sql.Tx, identity.Viewer, string, string, int64, float64, float64, string, bool) error
	Subtitles           *subtitles.Service
	Prepared            *preparedmedia.Service
	// Delivery seams. Owner administration, viewer preferences and hardware
	// detection are all injected; playback owns none of them.
	deliverySettings    DeliverySettings
	deliveryPreferences DeliveryPreferenceSource
	hardware            *decoder.HardwareDetector
	hardwareFFmpeg      string
	// hardwareSeen is the hardware state last reported (review P22): a
	// request reports only a change, never on every session.
	hardwareSeen atomic.Pointer[hardwareState]
	// probe refreshes stream detail at first play; see detail_refresh.go.
	probe      *assets.Probe
	probeSlots chan struct{}
	// MarkersTx is wired by HTTP composition to the analysis viewer projection.
	// Playback publishes segments, it never decides skip safety itself. A nil
	// hook simply leaves markers out of the offer.
	MarkersTx MarkerProjectionTx
}

// MarkerProjectionTx projects the viewer segment markers of one played source
// inside the caller's transaction, under that caller's library authorization.
type MarkerProjectionTx func(context.Context, *sql.Tx, identity.Principal, OffersScope, string) (segmentmarkers.Set, error)

func New(db *sql.DB) *Service { return &Service{db: db} }

// completeDeliveryInputTx adds what every plan for a catalogued file shares: the
// device, the viewer's languages, the file's stream facts and anything this
// device has already reported broken for it. All of it belongs to the same
// immutable decision, so first play, a rung change and an audio change read it
// through this one function.
func (s *Service) completeDeliveryInputTx(ctx context.Context, tx *sql.Tx, p identity.Principal, input *DeliveryInput, values DeliveryPreferenceReader, catalogued bool, explicitAudio *int) error {
	input.Client = clientProfileFor(ctx, tx, p)
	input.PreferredAudioLanguages = preferredAudioLanguages(values)
	input.Server = decoder.CurrentToolchain()
	input.FMP4Output = s.hls.fragmentedOutput()
	input.NoHLS = s.hls == nil
	if explicitAudio != nil {
		input.AudioStream, input.HasAudioChoice = *explicitAudio, true
	}
	if !catalogued {
		return nil
	}
	source, err := loadDeliverySource(ctx, tx, input.SourceID)
	if err != nil {
		return err
	}
	input.Source = &source
	input.Excluded, err = routeFailuresTx(ctx, tx, p, input.SourceID)
	return err
}
func (s *Service) Create(p identity.Principal, item, quality, request string) (Session, error) {
	return s.CreateContext(context.Background(), p, item, quality, request)
}
func (s *Service) CreateContext(ctx context.Context, p identity.Principal, item, quality, request string) (Session, error) {
	return s.createContext(ctx, p, item, quality, request)
}

// SessionReplacement is an explicit legacy transfer, never inferred from a bearer.
// Canonical occurrences must use their controller-owned queue/control routes.
type SessionReplacement struct {
	ID         string `json:"id"`
	Generation int    `json:"generation"`
}

func (s *Service) CreateReplacingContext(ctx context.Context, p identity.Principal, item, quality, request string, replacement *SessionReplacement) (Session, error) {
	return s.createWithReplacement(ctx, p, item, quality, request, replacement)
}
func (s *Service) createContext(ctx context.Context, p identity.Principal, item, quality, request string) (Session, error) {
	return s.createWithReplacement(ctx, p, item, quality, request, nil)
}
func (s *Service) createWithReplacement(ctx context.Context, p identity.Principal, item, quality, request string, replacement *SessionReplacement) (Session, error) {
	return s.create(ctx, p, item, quality, request, replacement, nil)
}

// planInputs is everything one immutable delivery decision is made from.
type planInputs struct {
	input            DeliveryInput
	cfg              DeliveryConfiguration
	values           DeliveryPreferenceReader
	ownerTranscoding bool
}

// planInputsTx resolves owner configuration, the viewer's lane preferences, this
// host's encoder and the file's facts before planning, so one immutable plan
// records every input. Sessions (legacy and v1) and the v1 plan preview share it,
// which is what makes "the same request and profile always produce the same plan"
// true across them.
func (s *Service) planInputsTx(ctx context.Context, tx *sql.Tx, p identity.Principal, aid, container, video, audio string, duration float64, remote, catalogued bool, quality string, explicitAudio *int, v1 *V1Choice) (planInputs, error) {
	cfg := deliveryConfiguration(s.deliverySettings)
	edge := DeliveryContextFrom(ctx)
	values := s.viewerPreferences(tx, p, edge)
	policy := ResolveDeliveryPolicy(values, edge.NetworkClass, edge.ServerLocality, edge.TransportClass, cfg)
	var ownerTranscoding bool
	if e := tx.QueryRow(`SELECT transcoding_enabled FROM playback_owner_policy WHERE singleton=1`).Scan(&ownerTranscoding); e != nil {
		return planInputs{}, e
	}
	var sourceHeight int
	var transfer, primaries string
	if e := tx.QueryRow(`SELECT COALESCE(a.height,0),COALESCE((SELECT f.color_transfer FROM asset_streams f WHERE f.asset_id=a.token AND f.type='video' ORDER BY f.stream_index LIMIT 1),''),COALESCE((SELECT f.color_primaries FROM asset_streams f WHERE f.asset_id=a.token AND f.type='video' ORDER BY f.stream_index LIMIT 1),'') FROM catalog_assets a WHERE a.token=?`, aid).Scan(&sourceHeight, &transfer, &primaries); e != nil {
		return planInputs{}, e
	}
	rungs := QualityOffers(sourceHeight, true, ownerTranscoding, policy)
	rung, ok := SelectedRung(rungs, quality)
	if !ok {
		return planInputs{}, ErrQualityUnavailable
	}
	hardware := s.selectHardware(ctx, cfg)
	target := rung.Target(policy, sourceHeight)
	if v1 != nil {
		target = v1.target(policy, cfg)
	}
	input := DeliveryInput{SourceID: aid, Container: container, VideoCodec: video, AudioCodec: audio, Duration: duration, Height: sourceHeight, ColorTransfer: transfer, ColorPrimaries: primaries, Remote: remote, Policy: policy, Target: target, Config: cfg, TranscodingEnabled: ownerTranscoding, Hardware: hardware}
	if e := s.completeDeliveryInputTx(ctx, tx, p, &input, values, catalogued, explicitAudio); e != nil {
		return planInputs{}, e
	}
	return planInputs{input: input, cfg: cfg, values: values, ownerTranscoding: ownerTranscoding}, nil
}

func (s *Service) create(ctx context.Context, p identity.Principal, item, quality, request string, replacement *SessionReplacement, v1 *V1Choice) (Session, error) {
	if replacement != nil && (!validControlID(replacement.ID) || replacement.Generation < 1) {
		return Session{}, errControlJSON
	}

	if request == "" || len(request) > 128 || !validQualitySelection(quality) {
		return Session{}, errors.New("requestId and a published quality rung are required")
	}
	legacyRequest := request
	request = sessionRequestKey(p, request)
	var prepared *remotePreparation
	if s.remote != nil {
		var exists int
		err := s.db.QueryRow(`SELECT 1 FROM playback_requests WHERE account_id=? AND profile_id=? AND request_id IN(?,?) LIMIT 1`, p.AccountID, p.ProfileID, request, legacyRequest).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			var prepErr error
			prepared, prepErr = s.remote.prepare(ctx, item)
			if prepErr != nil {
				return Session{}, prepErr
			}
		} else if err != nil {
			return Session{}, err
		}
	}
	var selectedAsset string
	{
		if e := s.selectAsset(ctx, item, v1).Scan(&selectedAsset); e != nil {
			return Session{}, e
		}
		s.refreshStreamDetail(ctx, selectedAsset)
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassEstablishedPlayback)
	if e != nil {
		return Session{}, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	authorize := s.AuthorityTx
	if authorize != nil {
		if p, e = authorize(ctx, tx, p, item); e != nil {
			return Session{}, e
		}
	}
	// This item fence belongs above receipt replay AND every original/prepared
	// source branch. A derivative must not resurrect a deleted recording.
	if e = recordingAdmittedTx(ctx, tx, item); e != nil {
		return Session{}, e
	}
	// playback_sessions, playback_requests and progress hold the integer
	// entity id; callers pass the public id, resolved once here.
	entityID, e := entityid.Resolve(ctx, tx, item)
	if errors.Is(e, entityid.ErrNotFound) {
		return Session{}, sql.ErrNoRows
	}
	if e != nil {
		return Session{}, e
	}
	var receipt, originalItem, storedRequest string
	e = tx.QueryRow(`SELECT response,(SELECT pid(public_id) FROM catalog_entities WHERE id=playback_requests.item_id),request_id FROM playback_requests WHERE account_id=? AND profile_id=? AND request_id IN(?,?) ORDER BY request_id=? DESC LIMIT 1`, p.AccountID, p.ProfileID, request, legacyRequest, request).Scan(&receipt, &originalItem, &storedRequest)
	if e == nil {
		if originalItem != item {
			return Session{}, errors.New("requestId already belongs to another item")
		}
		var existing Session
		if e = json.Unmarshal([]byte(receipt), &existing); e != nil {
			return Session{}, e
		}
		var state string
		ownerErr := tx.QueryRow(`SELECT state FROM playback_sessions WHERE id=? AND account_id=? AND profile_id=? AND `+sessionFamilyMatch, existing.ID, p.AccountID, p.ProfileID, p.Hash, p.Hash).Scan(&state)
		if errors.Is(ownerErr, sql.ErrNoRows) {
			var found int
			if e = tx.QueryRow(`SELECT count(*) FROM playback_sessions WHERE id=?`, existing.ID).Scan(&found); e != nil {
				return Session{}, e
			}
			// Never disclose another device's active grant or replay an ambiguous
			// pre-migration receipt as a fresh create.
			if found != 0 || storedRequest == legacyRequest {
				return Session{}, identity.ErrUnauthorized
			}
			state = "stopped"
			existing.StreamURL = ""
		} else if ownerErr != nil {
			return Session{}, ownerErr
		}
		var oldID string
		var oldGeneration int
		bindErr := tx.QueryRowContext(ctx, `SELECT old_session_id,old_generation FROM playback_legacy_replacement_requests WHERE account_id=? AND profile_id=? AND request_id=?`, p.AccountID, p.ProfileID, request).Scan(&oldID, &oldGeneration)
		if bindErr != nil && !errors.Is(bindErr, sql.ErrNoRows) {
			return Session{}, bindErr
		}
		if replacement == nil && bindErr == nil || replacement != nil && (bindErr != nil || oldID != replacement.ID || oldGeneration != replacement.Generation) {
			return Session{}, errors.New("requestId belongs to a different replacement")
		}
		existing.State = state
		if state == "failed" {
			existing.Error = "Playback stopped because the source or conversion failed."
		}
		return existing, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return Session{}, e
	}

	var aid, container, video, audio, sourcePath string
	var assetSize, assetModified int64
	var duration float64
	var derivative *preparedmedia.Version
	{
		e = tx.QueryRow(`SELECT a.token,a.container,a.video_codec,a.audio_codec,a.duration,a.path,a.size,a.modified_ns FROM catalog_entities e JOIN catalog_asset_links l ON l.entity_id=e.id JOIN catalog_assets a ON a.id=l.asset_id WHERE e.public_id=pid_blob(?) AND a.token=? AND a.available=1 AND NOT EXISTS(SELECT 1 FROM dvr_catalog_retirements gone WHERE gone.item_id=e.id)`, item, selectedAsset).Scan(&aid, &container, &video, &audio, &duration, &sourcePath, &assetSize, &assetModified)
		if e != nil {
			return Session{}, e
		}
	}
	if derivative == nil && s.StorageGuard != nil {
		if e = s.StorageGuard(sourcePath); e != nil {
			return Session{}, e
		}
	}
	mode := "direct"
	if container == "strm" {
		if prepared == nil || prepared.assetID != aid {
			return Session{}, ErrIncompatible
		}
		var library string
		if e = tx.QueryRow(`SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.public_id=pid_blob(?)`, item).Scan(&library); e != nil {
			return Session{}, e
		}
		var revision int64
		err := tx.QueryRow(`SELECT revision FROM library_network_policy WHERE library_id=?`, library).Scan(&revision)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Session{}, err
		}
		if revision != prepared.networkRevision {
			return Session{}, identity.ErrUnauthorized
		}
		video, audio, duration = prepared.facts.VideoCodec, prepared.facts.AudioCodec, prepared.facts.Duration
		mode = "remote"
		if _, _, e = compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: sourcePath, Size: assetSize, ModifiedNS: assetModified, Container: container, VideoCodec: video, AudioCodec: audio, Width: prepared.facts.Width, Height: prepared.facts.Height, Duration: duration}); e != nil {
			return Session{}, e
		}
		// The probe refreshed the asset's facts; re-derive availability from it.
		if assetID, e := compactcatalog.AssetByTokenTx(ctx, tx, aid); e != nil {
			return Session{}, e
		} else if assetID == 0 {
			return Session{}, sql.ErrNoRows
		} else if e = compactcatalog.SetAssetAvailableTx(ctx, tx, assetID, true); e != nil {
			return Session{}, e
		}
	}
	var explicitAudio *int
	if v1 != nil && v1.AudioStream != nil {
		explicitAudio = v1.AudioStream
	}
	inputs, e := s.planInputsTx(ctx, tx, p, aid, container, video, audio, duration, mode == "remote", derivative == nil && mode != "remote", quality, explicitAudio, v1)
	if e != nil {
		return Session{}, e
	}
	cfg, values, ownerTranscoding := inputs.cfg, inputs.values, inputs.ownerTranscoding
	delivery, e := planDelivery(inputs.input)
	if e != nil {
		return Session{}, e
	}
	mode = delivery.Mode
	if mode == "hls" {
		if s.hls == nil {
			return Session{}, ErrIncompatible
		}
		mode = "hls"

	}

	replacing := ""
	private := v1 != nil && v1.PrivateFor != ""
	if private {
		// A private presentation shares its owner's slot, and the owner plays on.
		var valid int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM playback_sessions WHERE id=? AND account_id=? AND profile_id=? AND state NOT IN('stopped','ended','failed')`, v1.PrivateFor, p.AccountID, p.ProfileID).Scan(&valid); e != nil {
			return Session{}, e
		}
		if valid != 1 {
			return Session{}, identity.ErrUnauthorized
		}
		replacing = v1.PrivateFor
	} else if replacement != nil {
		var valid int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM playback_sessions WHERE id=? AND generation=? AND account_id=? AND profile_id=? AND `+sessionFamilyMatch+` AND state NOT IN('stopped','ended','failed')`, replacement.ID, replacement.Generation, p.AccountID, p.ProfileID, p.Hash, p.Hash).Scan(&valid); e != nil {
			return Session{}, e
		}
		if valid != 1 {
			return Session{}, identity.ErrUnauthorized
		}
		replacing = replacement.ID
	}

	if e = checkSessionPolicy(ctx, tx, p, mode, replacing); e != nil {
		return Session{}, e
	}

	if !private && replacing != "" {
		if e = terminateControlOccurrence(ctx, tx, replacing, "stopped"); e != nil {
			return Session{}, e
		}
	}

	if mode == "hls" && s.hls.finite != nil {
		delivery.Profile = finiteDeliveryProfile
		delivery.Strategy = DeliveryVideoConversion
		delivery.Reason = ReasonFiniteNormalization
		delivery.ReasonCodes = append(delivery.ReasonCodes, ReasonFiniteNormalization)
		delivery.VideoAction = "convert"
		delivery.OutputVideoCodec = "h264"
	}
	// Enforce policy against the final plan, including finite-timeline conversion.
	if !ownerTranscoding && (delivery.AudioAction == "convert" || delivery.VideoAction == "convert") {
		return Session{}, ErrTranscodingDisabled
	}
	if e = tx.QueryRow(`SELECT a.size,a.modified_ns,COALESCE((SELECT f.revision FROM asset_stream_facts f WHERE f.asset_id=a.token AND f.size=a.size AND f.modified_ns=a.modified_ns),0) FROM catalog_assets a WHERE a.token=?`, aid).Scan(&delivery.SourceSize, &delivery.SourceModifiedNS, &delivery.FactsRevision); e != nil {
		return Session{}, e
	}
	id, grant := identity.Token(), identity.Token()
	if mode == "hls" {
		if e = reserveHLS(tx, id, replacing, &delivery, cfg); e != nil {
			return Session{}, e
		}
	}
	generation := 1
	if e = tx.QueryRow(`SELECT COALESCE(max(generation),0)+1 FROM playback_sessions WHERE session_hash=? AND account_id=? AND profile_id=?`, p.Hash, p.AccountID, p.ProfileID).Scan(&generation); e != nil {
		return Session{}, e
	}
	expires := time.Now().UTC().Add(12 * time.Hour).Format(time.RFC3339)
	_, e = tx.Exec(`INSERT INTO playback_sessions(id,session_hash,account_id,profile_id,item_id,asset_id,generation,state,grant_hash,grant_token,expires_at,request_id,duration,mode) VALUES(?,?,?,?,?,?,?,'starting',?,?,?,?,?,?)`, id, p.Hash, p.AccountID, p.ProfileID, entityID, aid, generation, identity.Digest(grant), grant, expires, request, duration, mode)
	if e != nil {
		return Session{}, e
	}
	pinResult, e := tx.Exec(`INSERT INTO playback_source_pins SELECT ?,token,size,modified_ns FROM catalog_assets WHERE token=?`, id, aid)
	if e != nil {
		return Session{}, e
	}
	pinRows, e := pinResult.RowsAffected()
	if e != nil {
		return Session{}, e
	}
	if pinRows != 1 {
		return Session{}, ErrStaleOffer
	}
	if e = subtitles.AutoSelectTx(ctx, tx, id, generation, item, aid, subtitles.ViewerKey(p.Authority, p.AccountID, p.ProfileID, p.ServerID), automaticSubtitles(values, delivery.Trace)); e != nil {
		return Session{}, e
	}

	if mode == "hls" {
		if e = subtitles.PinManifestTextTx(ctx, tx, id, item, aid, subtitles.ViewerKey(p.Authority, p.AccountID, p.ProfileID, p.ServerID)); e != nil {
			return Session{}, e
		}
	}
	if e = persistDeliveryPlan(tx, id, &delivery); e != nil {
		return Session{}, e
	}
	if e = persistSubtitlePlan(tx, id, aid, mode); e != nil {
		return Session{}, e
	}
	if mode == "remote" {
		if prepared.subtitleEvidence != "" {
			if _, e = tx.Exec(`INSERT INTO subtitle_remote_sessions VALUES(?,?)`, id, prepared.subtitleEvidence); e != nil {
				return Session{}, e
			}
		}
		if _, e = tx.Exec(`INSERT INTO remote_playback VALUES(?,?,?)`, id, prepared.descriptorDigest, prepared.networkRevision); e != nil {
			return Session{}, e
		}
	}
	resume := 0.0
	var resumeMS int64
	_ = tx.QueryRow(`SELECT position FROM progress WHERE profile_id=? AND item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`, identity.PersonalKey(p.Viewer), item).Scan(&resumeMS)
	resume = float64(resumeMS) / 1000
	var itemKindNum int64
	if e = tx.QueryRow(`SELECT kind FROM catalog_entities WHERE public_id=pid_blob(?)`, item).Scan(&itemKindNum); e != nil {
		return Session{}, e
	}
	itemKind, kindErr := compactcatalog.Kind(itemKindNum).Name()
	if kindErr != nil {
		// New playable kinds follow the generic playback path until they gain
		// kind-specific behavior.
		itemKind = ""
	}
	if itemKind != "audiobook_file" && resume >= duration-3 {
		resume = 0
	}
	if e = claimSessionProgress(tx, p, item, id, resume); e != nil {
		return Session{}, e
	}
	streamURL := "/v1/media/" + grant
	if mode == "hls" {
		streamURL += "/master.m3u8"
	}
	responseMode := mode
	if responseMode == "remote" {
		responseMode = "direct"
	}
	result := Session{ID: id, Generation: generation, StreamURL: streamURL, Mode: responseMode, Duration: duration, ResumeSeconds: resume, State: "starting"}
	if private {
		if _, e = tx.ExecContext(ctx, `INSERT INTO playback_private_presentations(session_id,owner_session_id,expires_ms) VALUES(?,?,?)`, id, v1.PrivateFor, v1.PrivateUntilMs); e != nil {
			return Session{}, e
		}
	}
	if itemKind == "song" || itemKind == "audiobook_file" {
		if v1 != nil && v1.AudioPlanV2 {
			if result.AudioPlan, e = s.pinAudioPlan(ctx, tx, result, item, aid, grant, v1, ownerTranscoding); e != nil {
				return Session{}, e
			}
		}
	}
	var boundary string
	err := tx.QueryRow(`SELECT status FROM episode_asset_boundaries WHERE item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND asset_id=?`, item, aid).Scan(&boundary)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Session{}, err
	}
	result.SourceBoundary = boundary
	encoded, e := json.Marshal(result)
	if e != nil {
		return Session{}, e
	}
	if _, e = tx.Exec(`INSERT INTO playback_requests VALUES(?,?,?,?,?)`, p.AccountID, p.ProfileID, request, entityID, string(encoded)); e != nil {
		return Session{}, e
	}
	if replacement != nil {
		if _, e = tx.ExecContext(ctx, `INSERT INTO playback_legacy_replacement_requests VALUES(?,?,?,?,?)`, p.AccountID, p.ProfileID, request, replacement.ID, replacement.Generation); e != nil {
			return Session{}, e
		}
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO playback_physical_source_pins SELECT ?,source.id,source.incarnation,source.generation FROM inventory_objects object JOIN library_sources source ON source.id=object.source_id JOIN catalog_entities item ON item.id=? JOIN catalog_libraries cl ON cl.id=item.library_id AND cl.library_id=source.library_id WHERE object.asset_id=? AND object.retired=0 AND object.state='available' AND object.root_incarnation=source.incarnation AND source.enabled=1 ORDER BY source.id LIMIT 1`, id, entityID, aid); e != nil {
		return Session{}, e
	}
	if e = gated.Commit(); e != nil {
		return Session{}, e
	}
	return result, nil
}

func (s *Service) Get(p identity.Principal, id string) (Session, error) {
	var out Session
	var grant string
	gated2, e := dbwork.BeginSnapshot(context.Background(), s.db)
	if e != nil {
		return out, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	if s.ContinueAuthorityTx != nil {
		if p, e = s.ContinueAuthorityTx(context.Background(), tx, p, ""); e != nil {
			return out, e
		}
	}
	e = tx.QueryRow(`SELECT id,generation,grant_token,duration,state,mode,COALESCE((SELECT json_extract(r.response,'$.sourceBoundary') FROM playback_requests r WHERE r.account_id=playback_sessions.account_id AND r.profile_id=playback_sessions.profile_id AND r.request_id=playback_sessions.request_id),'') FROM playback_sessions WHERE id=? AND account_id=? AND profile_id=? AND `+sessionFamilyMatch, id, p.AccountID, p.ProfileID, p.Hash, p.Hash).Scan(&out.ID, &out.Generation, &grant, &out.Duration, &out.State, &out.Mode, &out.SourceBoundary)

	if e != nil {
		return out, e
	}
	if s.AuthorityTx != nil {
		var item string
		if e = tx.QueryRow(`SELECT pid(e.public_id) FROM playback_sessions ps JOIN catalog_entities e ON e.id=ps.item_id WHERE ps.id=?`, id).Scan(&item); e != nil {
			return out, e
		}
		if _, e = s.AuthorityTx(context.Background(), tx, p, item); e != nil {
			return out, e
		}
	}
	_ = tx.QueryRow(`SELECT version_id FROM prepared_media_session_pins WHERE session_id=?`, id).Scan(&out.PreparedVersionID)
	out.StreamURL = "/v1/media/" + grant
	if out.Mode == "hls" {
		out.StreamURL += "/master.m3u8"
	}
	if out.Mode == "remote" {
		out.Mode = "direct"
	}
	var renderID string
	if queryErr := tx.QueryRow(`SELECT render_id FROM playback_subtitle_presentations WHERE session_id=? AND generation=?`, id, out.Generation).Scan(&renderID); queryErr == nil && renderID != "" {
		out.StreamURL = "/v1/media/" + grant + "/subtitle-video/" + renderID + "/master.m3u8"
		out.Mode = "hls"
	}
	if out.State == "failed" {
		out.Error = "Playback stopped because the source or conversion failed."
	}
	out.AudioPlan, e = readAudioPlan(tx, out.ID, out.Generation)
	return out, e
}
func (s *Service) Progress(p identity.Principal, id string, generation int, sequence int64, position float64, state string) error {
	return s.ProgressContext(context.Background(), p, id, generation, sequence, position, state)
}

// ProgressContext abandons report work when its request or playback timeline
// deadline expires, without stopping the live presentation.
func (s *Service) ProgressContext(ctx context.Context, p identity.Principal, id string, generation int, sequence int64, position float64, state string) error {
	if math.IsNaN(position) || math.IsInf(position, 0) || position < 0 || sequence < 0 {
		return errors.New("invalid progress")
	}
	if state != "playing" && state != "paused" && state != "ended" {
		return errors.New("invalid playback state")
	}
	gated3, e := dbwork.Begin(ctx, s.db, dbwork.ClassEstablishedPlayback)
	if e != nil {
		return e
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	if e = s.progressTx(gated3.Context(), tx, p, id, generation, sequence, position, state); e != nil {
		return e
	}
	return gated3.Commit()
}
func (s *Service) progressTx(ctx context.Context, tx *sql.Tx, p identity.Principal, id string, generation int, sequence int64, position float64, state string) error {
	var e error
	if e = checkSessionOccurrenceTx(ctx, tx, id); e != nil {
		return e
	}
	// Fence authentication and current item policy before accepting any evidence.
	// Live/Library occurrences are in another table and cannot enter this path.
	var observedItem string
	e = tx.QueryRowContext(ctx, `SELECT pid(e.public_id) FROM playback_sessions JOIN catalog_entities e ON e.id=playback_sessions.item_id WHERE playback_sessions.id=? AND playback_sessions.account_id=? AND playback_sessions.profile_id=? AND playback_sessions.generation=? AND `+sessionFamilyMatch, id, p.AccountID, p.ProfileID, generation, p.Hash, p.Hash).Scan(&observedItem)
	if errors.Is(e, sql.ErrNoRows) {
		return nil // Stale/foreign occurrence; no observation or history is changed.
	}
	if e != nil {
		return e
	}
	if s.AuthorityTx != nil {
		if p, e = s.AuthorityTx(ctx, tx, p, observedItem); e != nil {
			return e
		}
	}
	r, e := tx.ExecContext(ctx, `UPDATE playback_sessions SET sequence=?,state=?,session_hash=? WHERE id=? AND generation=? AND sequence<? AND state NOT IN ('stopped','ended','failed')`, sequence, state, p.Hash, id, generation, sequence)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return nil
	}
	_, e = tx.ExecContext(ctx, `UPDATE playback_observations SET reported_at=?,position=min(?,(SELECT duration FROM playback_sessions WHERE id=?)) WHERE session_id=?`, time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), position, id, id)
	if e != nil {
		return e
	}
	var item string
	var duration float64
	var canonical bool
	if e = tx.QueryRowContext(ctx, `SELECT pid(e.public_id),ps.duration,EXISTS(SELECT 1 FROM progress WHERE profile_id=? AND item_id=ps.item_id AND playback_id=ps.id) FROM playback_sessions ps JOIN catalog_entities e ON e.id=ps.item_id WHERE ps.id=?`, identity.PersonalKey(p.Viewer), id).Scan(&item, &duration, &canonical); e != nil {
		return e
	}
	if s.PersonalProgress != nil {
		if e = s.PersonalProgress(tx, p.Viewer, id, item, sequence, position, duration, state, canonical); e != nil {
			return e
		}
	} else {
		positionMS := int64(math.Round(position * 1000))
		_, e = tx.ExecContext(ctx, `UPDATE progress SET position=min(?,CAST((SELECT duration FROM playback_sessions WHERE id=?)*1000 AS INTEGER)),unit=0 WHERE profile_id=? AND playback_id=?`, positionMS, id, identity.PersonalKey(p.Viewer), id)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO progress_activity(profile_id,library_id,item_id,updated_at,state) SELECT ?,cl.library_id,p.item_id,?,? FROM playback_sessions p JOIN catalog_entities i ON i.id=p.item_id JOIN catalog_libraries cl ON cl.id=i.library_id WHERE p.id=? ON CONFLICT(profile_id,item_id) DO UPDATE SET updated_at=excluded.updated_at,state=excluded.state`, identity.PersonalKey(p.Viewer), time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), state, id)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE book_resume SET position=min(?,CAST((SELECT duration FROM playback_sessions WHERE id=?)*1000 AS INTEGER)),unit=0 WHERE profile_id=? AND playback_id=?`, positionMS, id, identity.PersonalKey(p.Viewer), id)
		if e != nil {
			return e
		}

	}
	return nil
}
func (s *Service) Stop(p identity.Principal, id string) error {
	gated4, e := dbwork.Begin(context.Background(), s.db, dbwork.ClassEstablishedPlayback)
	if e != nil {
		return e
	}
	tx := gated4.Tx()
	defer gated4.Rollback()
	var own int
	if e = tx.QueryRow(`SELECT count(*) FROM playback_sessions WHERE id=? AND account_id=? AND profile_id=? AND `+sessionFamilyMatch, id, p.AccountID, p.ProfileID, p.Hash, p.Hash).Scan(&own); e != nil {
		return e
	}
	if own != 1 {
		return nil
	}
	if e = terminateControlOccurrence(context.Background(), tx, id, "stopped"); e != nil {
		return e
	}
	if e = gated4.Commit(); e != nil {
		return e
	}
	s.stopHLS(id)
	return nil
}

// Only callers that have durably stopped this exact occurrence may cancel its
// worker. Internal queue cleanup already proved ownership in its transaction;
// reauthenticating an old request token after family renewal would leak a worker.
func (s *Service) stopHLS(id string) {
	if s.hls == nil {
		return
	}
	s.hls.mu.Lock()
	cancel := s.hls.active[id]
	s.hls.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
func (s *Service) ResolveGrant(grant string) (string, identity.Principal, string, error) {
	return s.ResolveGrantContext(context.Background(), grant)
}

// ResolveGrantContext keeps disconnects and request deadlines attached to the
// grant and continuation checks. It never changes the grant's lifetime.
func (s *Service) ResolveGrantContext(ctx context.Context, grant string) (string, identity.Principal, string, error) {
	return s.resolveGrant(ctx, grant, false)
}
func (s *Service) resolveGrant(ctx context.Context, grant string, initialAudio bool) (_ string, _ identity.Principal, _ string, err error) {
	var aid, item, expires, sessionExpires, state string
	var revoked int
	p := identity.Principal{}
	gated5, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return "", p, "", e
	}
	ctx = gated5.Context()
	tx := gated5.Tx()
	defer gated5.Rollback()
	defer func() {
		// database/sql may finish its cancellation rollback before a query
		// reaches the transaction. Preserve the cancellation cause instead of
		// reporting that race as an invalid or ended presentation.
		if err != nil && ctx.Err() != nil {
			err = ctx.Err()
		}
	}()
	var sid string
	if e = tx.QueryRowContext(ctx, `SELECT id FROM playback_sessions WHERE grant_hash=?`, identity.Digest(grant)).Scan(&sid); e != nil {
		return "", p, "", e
	}
	if e = checkSessionOccurrenceModeTx(ctx, tx, sid, initialAudio); e != nil {
		if errors.Is(e, identity.ErrUnauthorized) {
			e = ErrGrantEnded
		}
		return "", p, "", e
	}
	e = tx.QueryRowContext(ctx, `SELECT ps.asset_id,pid(e.public_id),ps.expires_at,ps.state,s.hash,s.account_id,s.profile_id,s.authority,s.role,s.epoch,s.expires_at,s.revoked FROM playback_sessions ps JOIN catalog_entities e ON e.id=ps.item_id JOIN authorization_access s ON s.hash=ps.session_hash WHERE ps.grant_hash=?`, identity.Digest(grant)).Scan(&aid, &item, &expires, &state, &p.Hash, &p.AccountID, &p.ProfileID, &p.Authority, &p.Role, &p.Epoch, &sessionExpires, &revoked)
	if e == nil && s.ContinueAuthorityTx != nil {
		e = tx.QueryRowContext(ctx, `SELECT value FROM configuration WHERE key='id'`).Scan(&p.ServerID)
	}
	if e == nil && s.ContinueAuthorityTx != nil {
		p, e = s.ContinueAuthorityTx(ctx, tx, p, item)
		if e == nil {
			e = tx.QueryRowContext(ctx, `SELECT expires_at,revoked FROM authorization_access WHERE hash=?`, p.Hash).Scan(&sessionExpires, &revoked)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	ended := state == "stopped" || state == "ended" || state == "failed" || revoked != 0 || expires <= now || sessionExpires <= now
	if errors.Is(e, compactcatalog.ErrBuilding) && !ended {
		// Authentication held; the item has an unpublished catalogue change:
		// retry, don't refuse the grant.
		return "", p, "", e
	}
	if e != nil || ended {
		if cause := ctx.Err(); cause != nil {
			return "", p, "", cause
		}
		return "", p, "", ErrGrantEnded
	}
	if p.Authority == "local" {
		var epoch int
		if tx.QueryRowContext(ctx, `SELECT epoch FROM accounts WHERE id=?`, p.AccountID).Scan(&epoch) != nil || epoch != p.Epoch {
			return "", p, "", ErrGrantEnded
		}
	}
	// Existing open descriptors may finish; a new grant/media open is not proof
	// that those readers retired and must not bypass the logical tombstone.
	if e = recordingAdmittedTx(ctx, tx, item); e != nil {
		return "", p, "", e
	}
	var changed int
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM playback_source_pins pin JOIN playback_sessions ps ON ps.id=pin.session_id JOIN catalog_assets a ON a.token=pin.asset_id WHERE ps.grant_hash=? AND NOT EXISTS(SELECT 1 FROM prepared_media_session_pins pp WHERE pp.session_id=ps.id) AND (ps.asset_id!=pin.asset_id OR a.size!=pin.size OR a.modified_ns!=pin.modified_ns)`, identity.Digest(grant)).Scan(&changed); e != nil {
		return "", p, "", e
	}
	if changed != 0 {
		return "", p, "", ErrStaleChapter
	}
	var isPrepared bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM prepared_media_session_pins pp JOIN playback_sessions ps ON ps.id=pp.session_id WHERE ps.grant_hash=?)`, identity.Digest(grant)).Scan(&isPrepared); e != nil {
		return "", p, "", e
	}
	if !isPrepared && s.StorageGuard != nil {
		var path string
		if e = tx.QueryRowContext(ctx, `SELECT path FROM catalog_assets WHERE token=?`, aid).Scan(&path); e != nil {
			return "", p, "", e
		}
		if e = s.StorageGuard(path); e != nil {
			return "", p, "", e
		}
	}
	return aid, p, item, nil
}
func (s *Service) Cleanup() error {
	if _, e := dbwork.ExecWrite(context.Background(), s.db, dbwork.ClassBackgroundMedia, `DELETE FROM playback_client_profiles WHERE session_hash IN (SELECT session_hash FROM playback_client_profiles WHERE updated_at_ms<? LIMIT 256)`, time.Now().Add(-90*24*time.Hour).UnixMilli()); e != nil {
		return e
	}
	_, e := dbwork.ExecWrite(context.Background(), s.db, dbwork.ClassEstablishedPlayback, `DELETE FROM playback_sessions WHERE expires_at<?`, time.Now().UTC().Format(time.RFC3339))
	return e
}
func (s Session) String() string { return fmt.Sprintf("playback %s generation %d", s.ID, s.Generation) }

func (s *Service) ConfigureHLS(h *HLS) {
	s.hls = h
	if h != nil {
		h.subtitleService = s.Subtitles
	}
}
func (s *Service) Ready(ctx context.Context, out Session) error {
	if out.PreparedVersionID != "" {
		if s.Prepared == nil {
			return ErrIncompatible
		}
		reader, handled, e := s.Prepared.OpenSession(ctx, out.ID)
		if e != nil {
			return e
		}
		if !handled {
			return ErrStaleOffer
		}
		return reader.Close()
	}
	if strings.Contains(out.StreamURL, "/subtitle-video/") {
		return nil
	}
	if out.Mode != "hls" || out.State == "stopped" || out.State == "ended" {
		return nil
	}
	if s.hls == nil {
		return ErrIncompatible
	}
	return s.hls.Ready(ctx, out.ID)
}

// Conversions reports conversion admission against the host's capacity, for
// diagnostics. A zero capacity means this service has no converter.
func (s *Service) Conversions() ConversionCapacity {
	if s == nil {
		return ConversionCapacity{}
	}
	return s.hls.Conversions()
}

func (s *Service) HLSFile(grant, name string) (string, error) {
	if s.hls == nil {
		return "", ErrIncompatible
	}
	return s.hls.File(grant, name)
}

func (s *Service) RemoteReadFailed(grant string) {
	_, _ = dbwork.ExecWrite(context.Background(), s.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_sessions SET state='failed' WHERE grant_hash=? AND mode='remote' AND state NOT IN ('stopped','ended')`, identity.Digest(grant))
}

func (s *Service) HLSFileContext(ctx context.Context, grant, name string) (string, error) {
	if s.hls == nil {
		return "", ErrIncompatible
	}
	var id, profile string
	err := s.db.QueryRowContext(ctx, `SELECT p.id,d.profile FROM playback_sessions p JOIN playback_delivery_plans d ON d.session_id=p.id WHERE p.grant_hash=?`, identity.Digest(grant)).Scan(&id, &profile)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if profile == finiteDeliveryProfile {
		if s.hls.finite == nil {
			return "", ErrFiniteUnsupported
		}
		return s.hls.finite.file(ctx, id, name)
	}
	return s.hls.FileContext(ctx, grant, name)
}

// Preparing Off retains the occurrence and original source pin. This is not a
// new Create request; original HLS work is made attachable before grant rotation.
