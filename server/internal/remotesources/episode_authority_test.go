package remotesources

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/storage"
)

type episodeFactsFixture struct {
	mode                                      string
	still, providerResolved, generationNeeded bool
	artworkRevision                           string
}

func (f *episodeFactsFixture) ReadEpisodeStateTx(ctx context.Context, tx *sql.Tx, target EpisodeTarget) (EpisodeState, error) {
	policy, err := catalog.ScanPolicyTx(ctx, tx, target.LibraryID)
	return EpisodeState{PolicyRevision: policy.Revision, ArtworkRevision: f.artworkRevision, Mode: f.mode, ProviderResolved: f.providerResolved, GenerationNeeded: f.generationNeeded, ProviderStill: f.still}, err
}

func TestExplicitEpisodeFenceRechecksRealActorTargetAndSource(t *testing.T) {
	for _, changed := range []string{"profile", "generation", "object", "actor", "range", "unknown_mode", "publication_actor", "publication_profile", "publication_object"} {
		t.Run(changed, func(t *testing.T) {
			ctx := context.Background()
			f := newObservationFixture(t)
			if _, err := f.record(t, scanFacts("a"), 0); err != nil {
				t.Fatal(err)
			}
			catalogRevision := f.catalogObservationRevision(t)
			for _, command := range []struct {
				sql  string
				args []any
			}{
				{`INSERT INTO catalog_assets(token,path,size,modified_ns,container,video_codec,audio_codec,width,height,duration) VALUES('asset',?,16,23,'mkv','h264','aac',1920,1080,60)`, []any{f.file}},
				{`INSERT INTO catalog_entities(public_id,library_id,kind,title,sort_key,sort_head) SELECT X'00112233445566778899AABBCCDDEEFF',id,4,'Episode','episode','e' FROM catalog_libraries WHERE library_id=?`, []any{f.library}},
				{`INSERT INTO catalog_asset_links(entity_id,asset_id) SELECT e.id,a.id FROM catalog_entities e CROSS JOIN catalog_assets a WHERE a.token='asset'`, nil},
				{`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns,state) SELECT 'object',id,'asset',incarnation,'film.mkv',?,'{}',16,23,'available' FROM library_sources WHERE id=?`, []any{catalogRevision, f.library}},
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
			facts := &episodeFactsFixture{mode: "off", artworkRevision: "artwork-1"}
			resolver := EpisodeAuthorityResolver{Sources: f.s, Identities: identities, Episodes: facts}
			request := EpisodeAuthorityRequest{Target: EpisodeTarget{LibraryID: f.library, LibrarySourceID: f.library, ObjectID: "object", SourceRevision: catalogRevision, EpisodeID: episode}, Purpose: EpisodeExplicitOneEpisode, Owner: owner, Operation: "episode_thumbnail", CostClass: "decode"}
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
			facts.still, facts.artworkRevision = true, "artwork-2"
			facts.providerResolved, facts.generationNeeded = true, true
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
			case "unknown_mode":
				facts.mode = "unknown"
			case "publication_actor", "publication_profile", "publication_object":
				publication, e := dbwork.Begin(ctx, f.s.db, dbwork.ClassBackgroundMedia)
				if e != nil {
					t.Fatal(e)
				}
				defer publication.Rollback()
				switch changed {
				case "publication_actor":
					_, e = publication.Tx().ExecContext(ctx, `UPDATE authorization_session_families SET revoked=1 WHERE id=?`, family)
				case "publication_profile":
					list, _ := SourceReadPreset("list")
					_, e = f.s.ApplySourceReadProfileTx(ctx, publication.Tx(), identities, owner, SourceReadProfileChange{SourceID: "remote", Generation: "1", Profile: list})
				case "publication_object":
					_, e = publication.Tx().ExecContext(ctx, `UPDATE inventory_objects SET revision='replacement' WHERE id='object'`)
				}
				if e != nil {
					t.Fatal(e)
				}
				// The recheck sees the caller-owned publication transaction, not
				// a separate earlier read snapshot. This tests the DB guard;
				// no production artwork publication/lease adapter is implied.
				if e = authority.CheckTx(ctx, publication.Tx()); e == nil {
					t.Fatal("publication ignored transaction-local revocation", changed)
				}
				publication.Rollback()
				if e = authority.Check(ctx); e == nil {
					t.Fatal("failed publication fence revived after rollback")
				}
				return
			}
			if mutation != "" {
				if _, err = f.s.db.Exec(mutation, args...); err != nil {
					t.Fatal(err)
				}
			}
			if err = authority.Authorize(ctx, "episode_thumbnail", "decode", true); err == nil {
				t.Fatal("changed authority resumed", changed)
			}
			if changed == "unknown_mode" {
				facts.mode = "off"
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
		if preset == "missing" {
			profile, _ = SourceReadPreset("list")
		}
		profile.Saved = preset != "missing"
		for _, mode := range []string{"provider_then_made", "provider", "off", "unknown"} {
			for _, still := range []bool{false, true} {
				state := EpisodeState{PolicyRevision: 7, ArtworkRevision: "artwork-2", Mode: mode, ProviderResolved: true, GenerationNeeded: true, ProviderStill: still}
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
	state := EpisodeState{PolicyRevision: 7, ArtworkRevision: "artwork-2", Mode: "provider_then_made", ProviderResolved: true, GenerationNeeded: true}
	profile, _ := SourceReadPreset("everything")
	profile.Saved, profile.WholeFileReads = true, false
	if err := episodePolicy(EpisodeAutomatic, state, library, true, profile); err != nil {
		t.Fatal("whole-file switch blocked bounded decode", err)
	}
	if err := episodePolicy(EpisodeAutomatic, state, library, false, SourceReadProfile{}); err != nil {
		t.Fatal("local source required a remote profile", err)
	}
	for _, missing := range []string{"provider_resolution", "generation_needed", "artwork_digest"} {
		t.Run(missing, func(t *testing.T) {
			changed := state
			switch missing {
			case "provider_resolution":
				changed.ProviderResolved = false
			case "generation_needed":
				changed.GenerationNeeded = false
			case "artwork_digest":
				changed.ArtworkRevision = ""
			}
			if err := episodePolicy(EpisodeAutomatic, changed, library, true, profile); err == nil {
				t.Fatal("automatic work accepted unresolved or protected artwork", missing)
			}
			if err := episodePolicy(EpisodeExplicitOneEpisode, changed, library, true, profile); err != nil {
				t.Fatal("automatic artwork fact refused explicit work", missing, err)
			}
		})
	}
	library.Operations = []string{"probe"}
	if err := episodePolicy(EpisodeAutomatic, state, library, true, profile); err == nil {
		t.Fatal("missing library operation authority accepted")
	}
	if err := episodePolicy(EpisodeExplicitOneEpisode, state, library, true, profile); err != nil {
		t.Fatal("explicit command blocked by automatic operation ceiling", err)
	}
}

func TestEpisodeAutomaticScheduleNeverGrantsExplicitAuthority(t *testing.T) {
	if err := episodeAnalysisGate(EpisodeExplicitOneEpisode, EpisodeAnalysisFacts{}); err != nil {
		t.Fatal("automatic schedule blocked explicit purpose", err)
	}
	if err := episodeAnalysisGate(EpisodeAutomatic, EpisodeAnalysisFacts{}); !errors.Is(err, ErrEpisodeAuthority) {
		t.Fatal("missing actual analysis facts granted automatic work", err)
	}
	if err := episodeAnalysisGate(EpisodeAutomatic, EpisodeAnalysisFacts{Revision: 1}); !errors.Is(err, storage.ErrBusy) {
		t.Fatal("closed actual window did not remain pending", err)
	}
	if err := episodeAnalysisGate(EpisodeAutomatic, EpisodeAnalysisFacts{Revision: 1, Open: true}); err != nil {
		t.Fatal("supported actual analysis facts refused", err)
	}
	if err := episodeAnalysisGate("unregistered", EpisodeAnalysisFacts{Revision: 1, Open: true}); !errors.Is(err, ErrEpisodeAuthority) {
		t.Fatal("unregistered purpose accepted schedule as authority", err)
	}
}

func TestEpisodeSourceTierExclusionIsExactAutomaticApplicability(t *testing.T) {
	library := catalog.ScanPolicy{Tier: "complete", Revision: 7, Operations: []string{"episode_thumbnail"}}
	state := EpisodeState{PolicyRevision: 7, ArtworkRevision: "current-artwork", Mode: "provider_then_made", ProviderResolved: true, GenerationNeeded: true}
	for _, preset := range []string{"list", "once", "ask", "missing"} {
		profile, _ := SourceReadPreset(preset)
		profile.Saved = true
		if preset == "missing" {
			profile, _ = SourceReadPreset("list")
		}
		if err := episodePolicy(EpisodeAutomatic, state, library, true, profile); !errors.Is(err, ErrEpisodeAutomaticSourceTier) {
			t.Fatal("known excluded source tier was not examined", preset, err)
		}
		if err := episodePolicy(EpisodeExplicitOneEpisode, state, library, true, profile); err != nil {
			t.Fatal("automatic source exclusion blocked explicit command", preset, err)
		}
	}
	if err := episodePolicy(EpisodeAutomatic, state, library, true, SourceReadProfile{}); !errors.Is(err, ErrEpisodeAuthority) {
		t.Fatal("unknown profile was treated as a positive exclusion", err)
	}
	state.ProviderResolved = false
	list, _ := SourceReadPreset("list")
	if err := episodePolicy(EpisodeAutomatic, state, library, true, list); !errors.Is(err, ErrEpisodeAuthority) {
		t.Fatal("unknown provider authority was treated as a positive exclusion", err)
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
