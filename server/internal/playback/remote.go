package playback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
	"portico.local/server/internal/subtitles"
	"strings"
	"sync"
	"time"
)

type Remote struct {
	AnalysisRoot      func(context.Context, string, string) (*storage.RootLease, error)
	SubtitleInputs    subtitles.SourceInputs
	SubtitleInspector subtitles.SourceInspector
	streams           chan struct{}
	probes            chan struct{}
	db                *sql.DB
	io                *storage.Client
	probe             assets.Probe
	resolver          remotemedia.Resolver
}
type remotePreparation struct {
	subtitleEvidence          string
	assetID, descriptorDigest string
	networkRevision           int64
	facts                     assets.Facts
}

func NewRemote(db *sql.DB, io *storage.Client, probe assets.Probe) *Remote {
	return &Remote{db: db, io: io, probe: probe, streams: make(chan struct{}, 8), probes: make(chan struct{}, 2)}
}
func (r *Remote) SetRoots(ctx context.Context, library string, roots []string) ([]remotemedia.Approval, error) {
	return r.SetRootsAuthorized(ctx, library, roots, nil, nil)
}
func (r *Remote) SetRootsAuthorized(ctx context.Context, library string, roots []string, expected *int64, authorize func(*sql.Tx) error) ([]remotemedia.Approval, error) {
	if len(roots) > 8 {
		return nil, errors.New("at most eight network roots may be approved per library")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	approvals := []remotemedia.Approval{}
	policy := remotemedia.Policy{Resolver: r.resolver}
	for _, root := range roots {
		a, e := policy.Approve(ctx, root)
		if e != nil {
			return nil, e
		}
		approvals = append(approvals, a)
	}
	raw, _ := json.Marshal(approvals)
	gated, e := dbwork.Begin(ctx, r.db, dbwork.ClassEstablishedPlayback)
	if e != nil {
		return nil, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if authorize != nil {
		if e = authorize(tx); e != nil {
			return nil, e
		}
	}
	if expected != nil {
		var current int64
		e = tx.QueryRowContext(ctx, `SELECT revision FROM library_network_policy WHERE library_id=?`, library).Scan(&current)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return nil, e
		}
		if current != *expected {
			return nil, ErrSourcePolicyChanged
		}
	}
	if _, e = tx.Exec(`INSERT INTO library_network_policy VALUES(?,1,?) ON CONFLICT(library_id) DO UPDATE SET revision=revision+1,approvals_json=excluded.approvals_json`, library, string(raw)); e != nil {
		return nil, e
	}
	if _, e = tx.Exec(`UPDATE playback_sessions SET state='stopped' WHERE mode='remote' AND item_id IN (SELECT e.id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE cl.library_id=?)`, library); e != nil {
		return nil, e
	}
	return approvals, gated.Commit()
}
func (r *Remote) policy(library string) (remotemedia.Policy, int64, error) {
	var rev int64
	var raw string
	p := remotemedia.Policy{Resolver: r.resolver}
	e := r.db.QueryRow(`SELECT revision,approvals_json FROM library_network_policy WHERE library_id=?`, library).Scan(&rev, &raw)
	if errors.Is(e, sql.ErrNoRows) {
		return p, 0, nil
	}
	if e != nil {
		return p, rev, e
	}
	e = json.Unmarshal([]byte(raw), &p.Approvals)
	return p, rev, e
}
func (r *Remote) source(ctx context.Context, asset, library string) (*remotemedia.Client, string, int64, error) {
	var path string
	var modified, size int64
	if e := r.db.QueryRow(`SELECT path,size,modified_ns FROM catalog_assets WHERE token=? AND available=1 AND container='strm'`, asset).Scan(&path, &size, &modified); e != nil {
		return nil, "", 0, e
	}
	descriptor, e := r.io.ReadDescriptor(ctx, library, path)
	if e != nil {
		return nil, "", 0, remotemedia.ErrUnavailable
	}
	if descriptor.Size != size || descriptor.ModifiedNS != modified {
		return nil, "", 0, errors.New("STRM descriptor changed; rescan before playback")
	}
	locator, e := remotemedia.ParseDescriptor([]byte(descriptor.Data))
	if e != nil {
		return nil, "", 0, e
	}
	policy, rev, e := r.policy(library)
	if e != nil {
		return nil, "", 0, e
	}
	client, e := remotemedia.New(ctx, locator, policy)
	return client, identity.Digest(descriptor.Data), rev, e
}
func (r *Remote) prepare(ctx context.Context, item string) (*remotePreparation, error) {
	var asset, library, container string
	if e := r.db.QueryRow(`SELECT a.token,cl.library_id,a.container FROM catalog_entities e JOIN catalog_asset_links l ON l.entity_id=e.id JOIN catalog_assets a ON a.id=l.asset_id JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.public_id=pid_blob(?) AND a.available=1 ORDER BY l.part_index,a.token LIMIT 1`, item).Scan(&asset, &library, &container); e != nil {
		return nil, e
	}
	if container != "strm" {
		return nil, nil
	}
	if r.SubtitleInspector != nil {
		info, e := r.SubtitleInspector.InspectSubtitleSource(ctx, item, asset)
		if e != nil {
			return nil, e
		}
		if !strings.Contains(info.Container, "mov") || info.VideoCodec != "h264" || (info.AudioCodec != "aac" && info.AudioCodec != "mp3" && info.AudioCodec != "") {
			return nil, remotemedia.ErrUnsupported
		}
		revision := info.NetworkPolicyRevision
		return &remotePreparation{assetID: asset, descriptorDigest: info.DescriptorDigest, networkRevision: revision, subtitleEvidence: info.Evidence, facts: assets.Facts{Container: "strm", VideoCodec: info.VideoCodec, AudioCodec: info.AudioCodec, Width: info.Width, Height: info.Height, Duration: float64(info.DurationUS) / 1e6, OriginUS: info.OriginUS, TimingKnown: true}}, nil
	}
	select {
	case r.probes <- struct{}{}:
		defer func() { <-r.probes }()
	default:
		return nil, remotemedia.ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client, digest, revision, e := r.source(ctx, asset, library)
	if e != nil {
		return nil, e
	}
	defer client.Close()
	bridge, close, e := client.Bridge(ctx)
	if e != nil {
		return nil, e
	}
	defer close()
	facts, e := r.probe.InspectRemoteMP4(ctx, bridge)
	if e != nil {
		return nil, remotemedia.ErrUnsupported
	}
	if facts.VideoCodec != "h264" || (facts.AudioCodec != "aac" && facts.AudioCodec != "mp3" && facts.AudioCodec != "") {
		return nil, remotemedia.ErrUnsupported
	}
	return &remotePreparation{assetID: asset, descriptorDigest: digest, networkRevision: revision, facts: facts}, nil
}
func (r *Remote) open(ctx context.Context, grant, method, rangeHeader string) (*http.Response, func(), bool, error) {
	var session, asset, library, mode, digest string
	var revision int64
	e := r.db.QueryRow(`SELECT ps.id,ps.asset_id,cl.library_id,ps.mode FROM playback_sessions ps JOIN catalog_entities i ON i.id=ps.item_id JOIN catalog_libraries cl ON cl.id=i.library_id WHERE ps.grant_hash=?`, identity.Digest(grant)).Scan(&session, &asset, &library, &mode)
	if e != nil {
		return nil, nil, false, e
	}
	if mode != "remote" {
		return nil, nil, false, nil
	}
	select {
	case r.streams <- struct{}{}:
	default:
		return nil, nil, true, remotemedia.ErrBusy
	}
	retained := false
	defer func() {
		if !retained {
			<-r.streams
		}
	}()
	if e = r.db.QueryRow(`SELECT descriptor_digest,network_revision FROM remote_playback WHERE session_id=?`, session).Scan(&digest, &revision); e != nil {
		return nil, nil, true, e
	}
	if r.SubtitleInputs != nil {
		var item, expected string
		if e = r.db.QueryRowContext(ctx, `SELECT pid(e.public_id),p.evidence FROM playback_sessions ps JOIN catalog_entities e ON e.id=ps.item_id JOIN subtitle_remote_sessions p ON p.session_id=ps.id WHERE ps.id=?`, session).Scan(&item, &expected); e != nil {
			return nil, nil, true, e
		}
		input, e := r.SubtitleInputs.OpenSubtitleInput(ctx, item, asset, session)
		if e != nil {
			return nil, nil, true, e
		}
		if input.Evidence() != expected {
			input.Close()
			return nil, nil, true, identity.ErrUnauthorized
		}
		response, e := subtitleSourceResponse(ctx, input, method, rangeHeader)
		if e != nil {
			input.Close()
			return nil, nil, true, e
		}
		retained = true
		var once sync.Once
		return response, func() { once.Do(func() { response.Body.Close(); input.Close(); <-r.streams }) }, true, nil
	}
	client, currentDigest, currentRevision, e := r.source(ctx, asset, library)
	if e != nil {
		return nil, nil, true, e
	}
	if currentDigest != digest || currentRevision != revision {
		client.Close()
		return nil, nil, true, identity.ErrUnauthorized
	}
	response, e := client.Open(ctx, method, rangeHeader)
	if e != nil {
		client.Close()
		return nil, nil, true, e
	}
	retained = true
	var once sync.Once
	return response, func() { once.Do(func() { response.Body.Close(); client.Close(); <-r.streams }) }, true, nil
}
func (s *Service) ConfigureRemote(remote *Remote) { s.remote = remote }
func (s *Service) ApproveNetworkRoots(ctx context.Context, library string, roots []string) ([]remotemedia.Approval, error) {
	if s.remote == nil {
		return nil, remotemedia.ErrUnsupported
	}
	return s.remote.SetRoots(ctx, library, roots)
}

// OpenRemote opens a remote (network) source behind a grant. decode resolves the
// grant as the version 2 audio routes do, so a prepared (private) presentation's
// grant opens too (NEW-33); every other media route passes false.
func (s *Service) OpenRemote(ctx context.Context, grant, method, rangeHeader string, decode bool) (*http.Response, func(), bool, error) {
	if s.remote == nil {
		return nil, nil, false, nil
	}
	if _, _, _, e := s.resolveGrant(grant, decode); e != nil {
		return nil, nil, false, e
	}
	return s.remote.open(ctx, grant, method, rangeHeader)
}

func (s *Service) ApproveNetworkRootsAuthorized(ctx context.Context, library string, roots []string, expected *int64, authorize func(*sql.Tx) error) ([]remotemedia.Approval, error) {
	if s.remote == nil || authorize == nil {
		return nil, identity.ErrUnauthorized
	}
	return s.remote.SetRootsAuthorized(ctx, library, roots, expected, authorize)
}
