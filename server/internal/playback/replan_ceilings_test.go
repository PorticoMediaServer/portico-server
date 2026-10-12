package playback

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/subtitles"
)

type replanFixture struct {
	t    *testing.T
	db   *sql.DB
	s    *Service
	h    *HLS
	p    identity.Principal
	item string
}

// newReplanFixture has one 1080p title in a container that is repackaged
// rather than played as it is. bitrate is the whole file's.
func newReplanFixture(t *testing.T, bitrate int64) *replanFixture {
	t.Helper()
	db, s, p, _, item, _ := activityFixture(t)
	h := &HLS{db: db}
	s.ConfigureHLS(h)
	updateCatalogAsset(t, db, firstCatalogAssetToken(t, db), func(a *compactcatalog.Asset) {
		a.Container, a.Width, a.Height, a.Duration, a.Size = "mkv", 1920, 1080, 100, bitrate*100/8
	})
	return &replanFixture{t: t, db: db, s: s, h: h, p: p, item: item}
}

func (f *replanFixture) configure(cfg DeliveryConfiguration) {
	f.s.ConfigureDelivery(fixedSettings{cfg}, nil, nil, "")
	f.h.ConfigureSettings(fixedSettings{cfg})
}

func (f *replanFixture) start(request string, choice V1Choice) Session {
	f.t.Helper()
	session, err := f.s.CreateV1(context.Background(), f.p, f.item, request, choice, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	return session
}

func (f *replanFixture) plan(session string) *DeliveryPlan {
	f.t.Helper()
	plan, err := loadDeliveryPlan(f.db, session)
	if err != nil || plan == nil {
		f.t.Fatal(plan, err)
	}
	return plan
}

func (f *replanFixture) subtitle(ctx context.Context, session string, resource *subtitles.Resource) error {
	return dbwork.WithWriteTx(ctx, f.db, dbwork.ClassEstablishedPlayback, func(tx *sql.Tx) error {
		return f.s.ChangeSubtitleDeliveryTx(ctx, tx, f.p, session, resource)
	})
}

// asStoredBefore rewrites a presentation's stored plan to the shape plans had
// before they kept their target, with a field a later build might add, and
// records the v1 session it belongs to as the v1 layer does.
func (f *replanFixture) asStoredBefore(session string) {
	f.t.Helper()
	mustAudioExec(f.t, f.db, `UPDATE playback_delivery_plans SET policy_json=json_set(json_remove(policy_json,'$.sessionTarget'),'$.addedLater',json('{"a":1}')) WHERE session_id=?`, session)
	mustAudioExec(f.t, f.db, `INSERT INTO playback_v1_sessions(id,device_id,account_id,profile_id,authority,start_key,start_digest,kind,role,state,request,revision,generation,media_session_id,lease_expires_ms,created_ms,updated_ms) VALUES(?,'device',?,?,'local',?,'','vod','viewer','playing','{"quality":{"mode":"original"}}',1,1,?,0,0,0)`, "v1-"+session, f.p.AccountID, f.p.ProfileID, session, session)
	if plan := f.plan(session); plan.Target != nil || plan.Policy == nil {
		f.t.Fatalf("the rewritten plan is not the earlier shape: %+v", plan)
	}
}

var styledSubtitle = &subtitles.Resource{ID: "styled", Revision: 1, Format: "ass"}

// within reports whether a planned value respects a ceiling (zero: none).
func within(value, ceiling int) bool { return ceiling <= 0 || value <= ceiling }

// A v1 presentation stored before plans kept their ceilings cannot be planned
// again: what it started under is not recoverable, and it must not be planned
// under no ceiling at all. A legacy session still names its rung.
func TestReplanRefusesAV1PresentationWhoseCeilingsWereNotStored(t *testing.T) {
	for _, test := range []struct {
		name   string
		choice V1Choice
	}{
		{"no limit under the member's remote cap", V1Choice{Remote: true, AdminMaxVideoBitrateBPS: 2_000_000}},
		{"the viewer's limit", V1Choice{Limit: true, MaxVideoBitrateBPS: 1_500_000, MaxVideoHeight: 480}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newReplanFixture(t, 40_000_000)
			session := f.start("earlier", test.choice)
			started := f.plan(session.ID)
			f.asStoredBefore(session.ID)
			for _, resource := range []*subtitles.Resource{styledSubtitle, nil} {
				if err := f.subtitle(context.Background(), session.ID, resource); !errors.Is(err, ErrQualityUnavailable) {
					t.Fatalf("a subtitle re-plan (burn-in %v) of a presentation with no stored ceilings: %v", resource != nil, err)
				}
			}
			if _, err := f.h.downgradeCopyPlan(context.Background(), session.ID, f.plan(session.ID)); !errors.Is(err, ErrQualityUnavailable) {
				t.Fatalf("a conversion fallback of a presentation with no stored ceilings: %v", err)
			}
			if after := f.plan(session.ID); after.VideoBitrateBPS != started.VideoBitrateBPS || after.TargetHeight != started.TargetHeight || after.BurnIn != nil {
				t.Fatalf("a refused re-plan changed the stored plan: %+v", after)
			}
		})
	}
	t.Run("a legacy session", func(t *testing.T) {
		f := newReplanFixture(t, 40_000_000)
		session, err := f.s.Create(f.p, f.item, "auto", "legacy")
		if err != nil {
			t.Fatal(err)
		}
		if err = f.subtitle(context.Background(), session.ID, styledSubtitle); err != nil {
			t.Fatalf("a legacy session's burn-in re-plan: %v", err)
		}
		if plan := f.plan(session.ID); plan.BurnIn == nil || plan.Target != nil || plan.QualityID != "auto" {
			t.Fatalf("legacy re-plan: %+v", plan)
		}
	})
}

// A copied stream that has to be converted after all is converted under the
// presentation's ceilings, and so is every subtitle change after it.
func TestConversionFallbackKeepsThePresentationCeilings(t *testing.T) {
	for _, test := range []struct {
		name    string
		choice  V1Choice
		bitrate int
	}{
		{"the member's remote cap", V1Choice{Remote: true, AdminMaxVideoBitrateBPS: 2_000_000}, 2_000_000},
		{"the viewer's limit", V1Choice{Limit: true, MaxVideoBitrateBPS: 2_000_000, MaxVideoHeight: 1080}, 2_000_000},
	} {
		t.Run(test.name, func(t *testing.T) {
			// 1.6 Mbit/s: within the ceiling, so the picture is copied.
			f := newReplanFixture(t, 1_600_000)
			session := f.start("copied", test.choice)
			started := f.plan(session.ID)
			if started.VideoAction != "copy" || started.Target == nil {
				t.Fatalf("the presentation did not start as a copy under a stored target: %+v", started)
			}
			converted, err := f.h.downgradeCopyPlan(context.Background(), session.ID, started)
			if err != nil {
				t.Fatalf("conversion fallback: %v", err)
			}
			if converted.VideoAction != "convert" || converted.VideoBitrateBPS < 1 || converted.VideoBitrateBPS > test.bitrate || converted.QualityID != started.QualityID {
				t.Fatalf("the fallback converts at %d bit/s (%s) under a %d bit/s ceiling", converted.VideoBitrateBPS, converted.QualityID, test.bitrate)
			}
			for _, resource := range []*subtitles.Resource{styledSubtitle, nil} {
				if err = f.subtitle(context.Background(), session.ID, resource); err != nil {
					t.Fatal(err)
				}
				if plan := f.plan(session.ID); plan.VideoBitrateBPS > test.bitrate || plan.Target == nil || (resource != nil) != (plan.BurnIn != nil) {
					t.Fatalf("after the fallback a subtitle change (burn-in %v) planned %d bit/s: %+v", resource != nil, plan.VideoBitrateBPS, plan)
				}
			}
		})
	}
}

// A re-plan never plans above what the presentation started under, whatever
// the owner's ceilings have become since, and never above those either.
func TestReplanNeverRaisesACeiling(t *testing.T) {
	remote := WithDeliveryContext(context.Background(), DeliveryContext{NetworkClass: NetworkRemote})
	t.Run("the owner's ceilings were lifted after the start", func(t *testing.T) {
		f := newReplanFixture(t, 40_000_000)
		strict := DefaultDeliveryConfiguration()
		strict.MaxVideoBitrateBPS, strict.MaxVideoHeight, strict.MaxAudioBitrateBPS = 3_000_000, 480, 96_000
		f.configure(strict)
		session := f.start("capped", V1Choice{Remote: true, AdminMaxVideoBitrateBPS: 2_000_000})
		started := f.plan(session.ID)
		if started.VideoAction != "convert" || started.VideoBitrateBPS > 2_000_000 || started.TargetHeight > 480 {
			t.Fatalf("the presentation did not start under its ceilings: %+v", started)
		}
		f.configure(DefaultDeliveryConfiguration())
		for _, resource := range []*subtitles.Resource{styledSubtitle, nil} {
			if err := f.subtitle(remote, session.ID, resource); err != nil {
				t.Fatal(err)
			}
			plan := f.plan(session.ID)
			if plan.VideoBitrateBPS > started.VideoBitrateBPS || plan.TargetHeight > started.TargetHeight || !within(plan.AudioBitrateBPS, 96_000) {
				t.Fatalf("a re-plan (burn-in %v) rose to %d bit/s at %dp, %d bit/s audio; the presentation started at %d bit/s at %dp", resource != nil, plan.VideoBitrateBPS, plan.TargetHeight, plan.AudioBitrateBPS, started.VideoBitrateBPS, started.TargetHeight)
			}
		}
	})
	t.Run("the owner's ceilings were lowered after the start", func(t *testing.T) {
		f := newReplanFixture(t, 40_000_000)
		session := f.start("lowered", V1Choice{Remote: true, AdminMaxVideoBitrateBPS: 2_000_000})
		strict := DefaultDeliveryConfiguration()
		strict.MaxVideoBitrateBPS, strict.MaxVideoHeight = 1_000_000, 360
		f.configure(strict)
		for _, resource := range []*subtitles.Resource{styledSubtitle, nil} {
			if err := f.subtitle(remote, session.ID, resource); err != nil {
				t.Fatal(err)
			}
			if plan := f.plan(session.ID); plan.VideoBitrateBPS > 1_000_000 || plan.TargetHeight > 360 {
				t.Fatalf("a re-plan (burn-in %v) planned %d bit/s at %dp above the owner's 1000000 bit/s at 360p", resource != nil, plan.VideoBitrateBPS, plan.TargetHeight)
			}
		}
		// Lifting them again does not give back what the member's cap never allowed.
		f.configure(DefaultDeliveryConfiguration())
		if err := f.subtitle(remote, session.ID, styledSubtitle); err != nil {
			t.Fatal(err)
		}
		if plan := f.plan(session.ID); plan.VideoBitrateBPS > 2_000_000 {
			t.Fatalf("a later re-plan escaped the member's cap: %d bit/s", plan.VideoBitrateBPS)
		}
	})
	t.Run("every ceiling of a stored target", func(t *testing.T) {
		ceilings := []int{0, 96_000, 480, 2_000_000, 8_000_000}
		for _, class := range []NetworkClass{NetworkUnknown, NetworkRemote, NetworkCellular} {
			for _, stored := range ceilings {
				for _, owner := range ceilings {
					cfg := DefaultDeliveryConfiguration()
					cfg.MaxVideoBitrateBPS, cfg.MaxAudioBitrateBPS, cfg.MaxVideoHeight = owner, owner, owner
					was := QualityTarget{RungID: "limit", MaxVideoBitrateBPS: stored, MaxAudioBitrateBPS: stored, MaxVideoHeight: stored}
					now := narrowTarget(was, ResolvedDeliveryPolicy{NetworkClass: class}, cfg)
					for name, pair := range map[string][2]int{"video bitrate": {was.MaxVideoBitrateBPS, now.MaxVideoBitrateBPS}, "audio bitrate": {was.MaxAudioBitrateBPS, now.MaxAudioBitrateBPS}, "height": {was.MaxVideoHeight, now.MaxVideoHeight}} {
						if pair[0] > 0 && (pair[1] <= 0 || pair[1] > pair[0]) {
							t.Fatalf("%s ceiling %d became %d (owner %d, %s)", name, pair[0], pair[1], owner, class)
						}
						if name != "video bitrate" && owner > 0 && (pair[1] <= 0 || pair[1] > owner) {
							t.Fatalf("%s ceiling %d ignores the owner's %d", name, pair[1], owner)
						}
					}
					if class != NetworkUnknown && owner > 0 && (now.MaxVideoBitrateBPS <= 0 || now.MaxVideoBitrateBPS > owner) {
						t.Fatalf("a remote video bitrate ceiling %d ignores the owner's %d", now.MaxVideoBitrateBPS, owner)
					}
				}
			}
		}
	})
}

// Everything a plan keeps beside its resolved policy survives a reload and a
// re-plan together, with a field a later build might add.
func TestStoredPlanExtrasSurviveReloadAndReplan(t *testing.T) {
	f := newReplanFixture(t, 40_000_000)
	asset := firstCatalogAssetToken(t, f.db)
	updateCatalogAsset(t, f.db, asset, func(a *compactcatalog.Asset) { a.AudioCodec = "truehd" })
	mustAudioExec(t, f.db, `INSERT INTO asset_stream_facts(asset_id,revision,size,modified_ns,fingerprint,detail_version) SELECT token,1,size,modified_ns,'test',99 FROM catalog_assets WHERE token=?`, asset)
	mustAudioExec(t, f.db, `INSERT INTO asset_streams(asset_id,stream_index,type,codec,language,title,channels,channel_layout,is_default,is_forced,detail_json) VALUES(?,0,'video','h264','','',0,'',1,0,'{"width":1920,"height":1080,"bitDepth":8}'),(?,1,'audio','truehd','eng','',6,'5.1',1,0,'{"sampleRate":96000}')`, asset, asset)
	session := f.start("extras", V1Choice{Limit: true, MaxVideoBitrateBPS: 1_500_000, MaxVideoHeight: 480})
	mustAudioExec(t, f.db, `UPDATE playback_delivery_plans SET policy_json=json_set(policy_json,'$.addedLater',json('{"a":1}')) WHERE session_id=?`, session.ID)
	stored := f.plan(session.ID)
	if stored.Target == nil || stored.Target.MaxVideoHeight != 480 || stored.AudioSampleRate != 48000 || stored.Policy == nil || stored.Policy.NetworkClass != NetworkUnknown {
		t.Fatalf("reloaded plan: target %+v, audio rate %d, policy %+v", stored.Target, stored.AudioSampleRate, stored.Policy)
	}
	if err := f.subtitle(context.Background(), session.ID, styledSubtitle); err != nil {
		t.Fatal(err)
	}
	if plan := f.plan(session.ID); plan.BurnIn == nil || plan.Target == nil || plan.Target.MaxVideoHeight != 480 || plan.AudioSampleRate != 48000 || plan.VideoBitrateBPS > 1_500_000 {
		t.Fatalf("re-planned plan: target %+v, audio rate %d, %d bit/s", plan.Target, plan.AudioSampleRate, plan.VideoBitrateBPS)
	}
}
