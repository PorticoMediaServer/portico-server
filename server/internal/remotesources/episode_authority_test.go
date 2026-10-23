package remotesources

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
)

type episodeFactsFixture struct {
	mode, providerState string
	still               bool
	artworkRevision     int64
	limits              ReadLimits
}

func (f *episodeFactsFixture) ReadEpisodeStateTx(ctx context.Context, tx *sql.Tx, target EpisodeTarget) (EpisodeState, error) {
	policy, err := catalog.ScanPolicyTx(ctx, tx, target.LibraryID)
	return EpisodeState{PolicyRevision: policy.Revision, ArtworkRevision: f.artworkRevision, Mode: f.mode, ProviderStillKnown: true, ProviderStill: f.still}, err
}

func (f *episodeFactsFixture) ReadEpisodeWorkTx(context.Context, *sql.Tx, EpisodeTarget, EpisodePurpose) (EpisodeWorkState, error) {
	return EpisodeWorkState{ServerRevision: 1, LibraryRevision: 1, ProviderRevision: 1, ReadLeaseRevision: 1, RemoteProcessGeneration: "fixture-process", ProviderState: f.providerState, Server: ReadCeiling{Known: true}, Library: ReadCeiling{Known: true}, Provider: ReadCeiling{Known: true}}, nil
}

func (f *episodeFactsFixture) CheckEpisodeReadLease(_ context.Context, binding EpisodeReadLeaseBinding) error {
	if binding.Purpose != EpisodeExplicitOneEpisode || binding.Limits != f.limits || binding.RemoteProcessGeneration != "fixture-process" || binding.ReadLeaseRevision != 1 || binding.Observation.SourceID != "remote" || binding.Observation.Generation != "1" || binding.RootIncarnation == "" || binding.LibrarySourceGeneration < 1 {
		return ErrEpisodeAuthority
	}
	return nil
}

func TestExplicitEpisodeFenceRechecksRealActorTargetAndSource(t *testing.T) {
	for _, changed := range []string{"profile", "generation", "object", "actor", "range", "library", "provider"} {
		t.Run(changed, func(t *testing.T) {
			ctx := context.Background()
			f := newObservationFixture(t)
			if _, err := f.record(t, scanFacts("a"), 0); err != nil {
				t.Fatal(err)
			}
			for _, command := range []struct {
				sql  string
				args []any
			}{
				{`INSERT INTO catalog_assets(token,path,size,modified_ns,container,video_codec,audio_codec,width,height,duration) VALUES('asset',?,16,23,'mkv','h264','aac',1920,1080,60)`, []any{f.file}},
				{`INSERT INTO catalog_entities(public_id,library_id,kind,title,sort_key,sort_head) SELECT X'00112233445566778899AABBCCDDEEFF',id,4,'Episode','episode','e' FROM catalog_libraries WHERE library_id=?`, []any{f.library}},
				{`INSERT INTO catalog_asset_links(entity_id,asset_id) SELECT e.id,a.id FROM catalog_entities e CROSS JOIN catalog_assets a WHERE a.token='asset'`, nil},
				{`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns,state) SELECT 'object',id,'asset',incarnation,'film.mkv','1:a','{}',16,23,'available' FROM library_sources WHERE id=?`, []any{f.library}},
			} {
				if _, err := f.s.db.Exec(command.sql, command.args...); err != nil {
					t.Fatal(err)
				}
			}
			identities, owner, family := observationOwner(t, f)
			var episode string
			if err := f.s.db.QueryRow(`SELECT pid(public_id) FROM catalog_entities WHERE kind=4`).Scan(&episode); err != nil {
				t.Fatal(err)
			}
			facts := &episodeFactsFixture{mode: "off", providerState: "ready", artworkRevision: 1, limits: ReadLimits{Connections: 2, RequestsPerSecond: 2}}
			resolver := EpisodeAuthorityResolver{Sources: f.s, Identities: identities, Episodes: facts, Work: facts}
			request := EpisodeAuthorityRequest{Target: EpisodeTarget{LibraryID: f.library, LibrarySourceID: f.library, ObjectID: "object", SourceRevision: "1:a", EpisodeID: episode}, Purpose: EpisodeExplicitOneEpisode, Owner: owner, Operation: "episode_thumbnail", CostClass: "decode"}
			gated, err := dbwork.BeginSnapshot(ctx, f.s.db)
			if err != nil {
				t.Fatal(err)
			}
			authority, err := resolver.ResolveEpisodeAuthorityTx(ctx, gated.Tx(), request)
			gated.Rollback()
			if err != nil {
				t.Fatal(err)
			}
			if err = authority.Authorize(ctx, "episode_thumbnail", "decode", true); err != nil {
				t.Fatal("explicit Off/missing-profile refused", err)
			}
			facts.still, facts.artworkRevision = true, 2
			if err = authority.Authorize(ctx, "episode_thumbnail", "decode", true); err != nil {
				t.Fatal("provider still affected explicit work", err)
			}
			var mutation string
			var args []any
			switch changed {
			case "profile":
				mutation = `INSERT INTO remote_source_read_profiles VALUES('remote',1,'file_list_only',0,2,2,0)`
			case "generation":
				mutation = `UPDATE remote_sources SET generation=generation+1 WHERE id='remote'`
			case "object":
				mutation = `UPDATE inventory_objects SET revision='replacement' WHERE id='object'`
			case "actor":
				mutation, args = `UPDATE authorization_session_families SET revoked=1 WHERE id=?`, []any{family}
			case "range":
				mutation = `UPDATE remote_sources SET range_support='unsupported' WHERE id='remote'`
			case "library":
				mutation = `UPDATE library_scan_policies SET revision=revision+1`
			case "provider":
				facts.providerState = "throttled"
			}
			if mutation != "" {
				if _, err = f.s.db.Exec(mutation, args...); err != nil {
					t.Fatal(err)
				}
			}
			if err = authority.Authorize(ctx, "episode_thumbnail", "decode", true); err == nil {
				t.Fatal("changed authority resumed", changed)
			}
			if changed == "provider" {
				facts.providerState = "ready"
				if err = authority.Check(ctx); err == nil {
					t.Fatal("terminal old generation revived")
				}
			}
		})
	}
}

func TestEpisodeAutomaticAndExplicitPolicyMatrix(t *testing.T) {
	library := catalog.ScanPolicy{Tier: "complete", Revision: 7, Operations: []string{"probe", "episode_thumbnail"}}
	for _, preset := range []string{"list", "once", "everything", "ask", "missing"} {
		profile, _ := SourceReadPreset(preset)
		profile.Saved = preset != "missing"
		for _, mode := range []string{"provider_then_made", "provider", "off", "unknown"} {
			for _, still := range []bool{false, true} {
				state := EpisodeState{PolicyRevision: 7, ArtworkRevision: 2, Mode: mode, ProviderStillKnown: true, ProviderStill: still}
				for _, purpose := range []EpisodePurpose{EpisodeAutomatic, EpisodeExplicitOneEpisode} {
					want := mode != "unknown"
					if purpose == EpisodeAutomatic {
						want = mode == "provider_then_made" && !still && preset == "everything"
					}
					err := episodePolicy(purpose, state, library, true, profile)
					if (err == nil) != want {
						t.Errorf("preset=%s mode=%s still=%v purpose=%s: %v", preset, mode, still, purpose, err)
					}
				}
			}
		}
	}
	state := EpisodeState{PolicyRevision: 7, ArtworkRevision: 2, Mode: "provider_then_made", ProviderStillKnown: true}
	profile, _ := SourceReadPreset("everything")
	profile.Saved, profile.WholeFileReads = true, false
	if err := episodePolicy(EpisodeAutomatic, state, library, true, profile); err != nil {
		t.Fatal("whole-file switch blocked bounded decode", err)
	}
	if err := episodePolicy(EpisodeAutomatic, state, library, false, SourceReadProfile{}); err != nil {
		t.Fatal("local source required a remote profile", err)
	}
	library.Operations = []string{"probe"}
	if err := episodePolicy(EpisodeAutomatic, state, library, true, profile); err == nil {
		t.Fatal("missing library operation authority accepted")
	}
	if err := episodePolicy(EpisodeExplicitOneEpisode, state, library, true, profile); err != nil {
		t.Fatal("explicit command blocked by automatic operation ceiling", err)
	}
}

func TestEpisodeOperationalLimitsAndPurposeUseDifferentScheduleGates(t *testing.T) {
	work := EpisodeWorkState{ServerRevision: 1, LibraryRevision: 2, ProviderRevision: 3, ReadLeaseRevision: 1, ProviderState: "ready",
		Server:   ReadCeiling{Known: true, Limits: ReadLimits{Connections: 3, BytesPerSecond: 10_000_000}},
		Library:  ReadCeiling{Known: true, Limits: ReadLimits{RequestsPerSecond: 2}},
		Provider: ReadCeiling{Known: true}}
	source := ReadLimits{Connections: 8, RequestsPerSecond: 10}
	got, err := episodeWorkLimits(EpisodeExplicitOneEpisode, work, source)
	if err != nil || got != (ReadLimits{Connections: 3, RequestsPerSecond: 2, BytesPerSecond: 10_000_000}) {
		t.Fatal("lowest operational ceilings did not win", got, err)
	}
	if _, err = episodeWorkLimits(EpisodeAutomatic, work, source); err == nil {
		t.Fatal("automatic work used absent window/schedule authority")
	}
	work.HeavyWindowRevision, work.AnalysisScheduleRevision = 4, 5
	work.HeavyWindowOpen, work.AnalysisScheduleOpen = true, true
	if _, err = episodeWorkLimits(EpisodeAutomatic, work, source); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"", "throttled", "quota", "unavailable"} {
		work.ProviderState = state
		if _, err = episodeWorkLimits(EpisodeExplicitOneEpisode, work, source); err == nil {
			t.Fatal("explicit command bypassed provider state", state)
		}
	}
	work.ProviderState, work.Server.Known = "ready", false
	if _, err = episodeWorkLimits(EpisodeExplicitOneEpisode, work, source); err == nil {
		t.Fatal("unknown server limit silently treated as unlimited")
	}
}

func TestEpisodeAuthorityFlagCannotEscalateCapturedPurpose(t *testing.T) {
	for _, purpose := range []EpisodePurpose{EpisodeAutomatic, EpisodeExplicitOneEpisode} {
		a := &EpisodeAuthority{request: EpisodeAuthorityRequest{Purpose: purpose}}
		if err := a.Authorize(context.Background(), "episode_thumbnail", "decode", purpose != EpisodeExplicitOneEpisode); !errors.Is(err, ErrEpisodeAuthority) {
			t.Fatal("wrong helper invocation was accepted", err)
		}
		if err := a.Check(context.Background()); !errors.Is(err, ErrEpisodeAuthority) {
			t.Fatal("failed fence revived", err)
		}
	}
	var resolver EpisodeAuthorityResolver
	if _, err := resolver.ResolveEpisodeAuthorityTx(context.Background(), nil, EpisodeAuthorityRequest{}); !errors.Is(err, ErrEpisodeAuthority) {
		t.Fatal("missing checked producer granted authority", err)
	}
	if err := (&EpisodeAuthority{}).Check(context.Background()); !errors.Is(err, ErrEpisodeAuthority) {
		t.Fatal("zero authority did not fail closed", err)
	}
	request := EpisodeAuthorityRequest{Target: EpisodeTarget{LibraryID: "library", LibrarySourceID: "source", ObjectID: "object", SourceRevision: "revision", EpisodeID: "episode"}, Purpose: EpisodeAutomatic, JobID: "scan", Operation: "episode_thumbnail", CostClass: "decode"}
	if !validEpisodeRequest(request) {
		t.Fatal("valid automatic request refused")
	}
	request.Owner.Hash = "owner-token-digest"
	if validEpisodeRequest(request) {
		t.Fatal("ambiguous actor and job purpose accepted")
	}
	request.Owner.Hash, request.CostClass = "", "metadata-only"
	if validEpisodeRequest(request) {
		t.Fatal("cost label substituted for decode authority")
	}
}
