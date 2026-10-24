package subtitles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

func capturePlaybackInvocation(t *testing.T, f *subtitleFixture) PlaybackSourceInvocation {
	t.Helper()
	ctx := context.Background()
	snapshot, p, err := f.s.readTx(ctx, f.p, f.item.Public)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Rollback()
	scope, err := sessionTx(ctx, snapshot.Tx(), p, f.item.Public, "session")
	if err != nil {
		t.Fatal(err)
	}
	i, ok := PlaybackSourceInvocationFromContext(f.s.playbackSourceContext(ctx, p, f.item.Public, "session", scope))
	if !ok {
		t.Fatal("authorized session invocation was not captured")
	}
	return i
}

func checkPlaybackInvocation(ctx context.Context, db *sql.DB, i PlaybackSourceInvocation) error {
	snapshot, err := dbwork.BeginSnapshot(ctx, db)
	if err != nil {
		return err
	}
	defer snapshot.Rollback()
	return i.CheckTx(ctx, snapshot.Tx())
}

func TestPlaybackSourceInvocationRetainsOnlyCapturedSessionFacts(t *testing.T) {
	f := newSubtitleFixture(t)
	i := capturePlaybackInvocation(t, f)
	facts := i.Facts()
	if facts.Principal != f.p || facts.ItemID != f.item.Public || facts.AssetID != f.item.Token || facts.SessionID != "session" || facts.SessionGeneration != 1 || facts.SourceSize != 1000 || facts.SourceModifiedNS != 1 {
		t.Fatal("captured session/pin facts changed", facts)
	}
	// Accessors return copies. Changing a caller's copy cannot retarget the
	// continuation or turn the context into a different actor/source grant.
	facts.Principal.Hash, facts.AssetID, facts.SessionID, facts.SessionGeneration = "foreign", "foreign", "foreign", 99
	if got := i.Facts(); got.Principal != f.p || got.AssetID != f.item.Token || got.SessionID != "session" || got.SessionGeneration != 1 {
		t.Fatal("caller retargeted captured authority", got)
	}
	if err := checkPlaybackInvocation(context.Background(), f.db, i); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{i, i.Facts()} {
		raw, err := json.Marshal(value)
		if err != nil || string(raw) != "{}" {
			t.Fatal("private actor/session facts entered JSON", string(raw), err)
		}
	}
	if _, ok := PlaybackSourceInvocationFromContext(context.Background()); ok {
		t.Fatal("missing invocation fabricated context authority")
	}
	if err := (PlaybackSourceInvocation{}).CheckTx(context.Background(), nil); !errors.Is(err, ErrUnavailable) {
		t.Fatal("empty invocation accepted", err)
	}
}

func TestPlaybackSourceInvocationRechecksCurrentActorSessionAndPin(t *testing.T) {
	for name, change := range map[string]func(*subtitleFixture){
		"actor denied": func(f *subtitleFixture) { f.allowed = false },
		"generation": func(f *subtitleFixture) {
			f.catalog.Exec(`UPDATE playback_sessions SET generation=2 WHERE id='session'`)
		},
		"stopped": func(f *subtitleFixture) {
			f.catalog.Exec(`UPDATE playback_sessions SET state='stopped' WHERE id='session'`)
		},
		"expired": func(f *subtitleFixture) {
			f.catalog.Exec(`UPDATE playback_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE id='session'`)
		},
		"foreign principal": func(f *subtitleFixture) {
			f.catalog.Exec(`UPDATE playback_sessions SET session_hash='foreign' WHERE id='session'`)
		},
		"unavailable asset": func(f *subtitleFixture) {
			f.catalog.Exec(`UPDATE catalog_assets SET available=0 WHERE token=?`, f.item.Token)
		},
		"changed source pin": func(f *subtitleFixture) {
			f.catalog.Exec(`UPDATE catalog_assets SET size=1001 WHERE token=?`, f.item.Token)
			f.catalog.Exec(`UPDATE playback_source_pins SET size=1001 WHERE session_id='session'`)
		},
		"foreign pin": func(f *subtitleFixture) {
			library := f.catalog.Library("foreign-library", "Foreign", "movie", "/foreign")
			other := f.catalog.Movie(library, "/foreign/other.mp4", "Foreign", 2024)
			f.catalog.Exec(`UPDATE playback_source_pins SET asset_id=? WHERE session_id='session'`, other.Token)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newSubtitleFixture(t)
			i := capturePlaybackInvocation(t, f)
			change(f)
			if err := checkPlaybackInvocation(context.Background(), f.db, i); err == nil {
				t.Fatal("changed actor/session/source pin accepted")
			}
		})
	}
}

type invocationSourceInputs struct {
	open func(context.Context, string, string, string) (RenderInput, error)
}

func (i invocationSourceInputs) OpenSubtitleInput(ctx context.Context, item, source, session string) (RenderInput, error) {
	return i.open(ctx, item, source, session)
}

type invocationRenderInput struct {
	validate func(context.Context) error
	closed   *int
}

func (*invocationRenderInput) Evidence() string { return "representation" }
func (*invocationRenderInput) Size() int64      { return 1000 }
func (*invocationRenderInput) ReadExtent(context.Context, int64, int64) ([]byte, error) {
	return nil, ErrUnsupported
}
func (i *invocationRenderInput) Validate(ctx context.Context) error { return i.validate(ctx) }
func (i *invocationRenderInput) Close() error                       { *i.closed++; return nil }

func TestSelectedPlaybackSourceCarriesActualInvocationAfterCommit(t *testing.T) {
	f := newSubtitleFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	receipt, err := f.s.Mutate(ctx, f.p, f.item.Public, uploadMutation("source-invocation", plainSubtitle, f.item.Token))
	if err != nil {
		t.Fatal(err)
	}
	f.catalog.Exec(`UPDATE subtitle_render_revisions SET source_evidence='representation' WHERE resource_id=?`, receipt.ResourceID)
	f.catalog.Exec(`INSERT INTO subtitle_remote_sessions(session_id,evidence) VALUES('session','representation')`)
	opens, validations, closes := 0, 0, 0
	f.s.SourceInputs = invocationSourceInputs{open: func(read context.Context, item, source, session string) (RenderInput, error) {
		opens++
		i, ok := PlaybackSourceInvocationFromContext(read)
		if !ok || read.Done() != ctx.Done() || item != f.item.Public || source != f.item.Token || session != "session" {
			t.Fatal("source input lost the authorized invocation/lifetime")
		}
		facts := i.Facts()
		if facts.Principal != f.p || facts.SessionGeneration != 1 || facts.AssetID != source || facts.ItemID != item || facts.SessionID != session {
			t.Fatal("caller fields were guessed or retargeted", facts)
		}
		if err := checkPlaybackInvocation(read, f.db, i); err != nil {
			t.Fatal("continuation failed outside the committed command snapshot", err)
		}
		return &invocationRenderInput{closed: &closes, validate: func(check context.Context) error {
			validations++
			if check != ctx {
				t.Fatal("source validation detached from the original invocation")
			}
			return checkPlaybackInvocation(check, f.db, i)
		}}, nil
	}}
	plan, err := f.s.Select(ctx, f.p, f.item.Public, "session", SelectRequest{OperationID: "select-source-invocation", Generation: 1, ExpectedRevision: 1, Mode: "track", ResourceID: receipt.ResourceID, ResourceRevision: receipt.Revision, OffsetUS: "0"})
	if err != nil || plan.Selected == nil || plan.Selected.ID != receipt.ResourceID || opens != 1 || validations != 1 || closes != 1 {
		t.Fatal("selected source propagation failed", plan, err, opens, validations, closes)
	}
}

func TestPlaybackSourceInvocationCancellationDoesNotAuthorize(t *testing.T) {
	f := newSubtitleFixture(t)
	i := capturePlaybackInvocation(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := checkPlaybackInvocation(ctx, f.db, i); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled invocation became a read grant", err)
	}
}
