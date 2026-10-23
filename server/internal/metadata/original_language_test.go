package metadata

import (
	"context"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/metadatalanguage"
	"testing"
)

func TestOriginalLanguagePublicationReachesDetailWithoutOwnerLanguageOverride(t *testing.T) {
	for _, raw := range []string{"jpn", "cn", "xx"} {
		t.Run(raw, func(t *testing.T) {
			s, db, p := screenFixture(t)
			item := publicationFixtureMovie(t, db)
			screenConfirm(t, s)
			p.record.Language = raw
			if err := s.ScreenStep(context.Background()); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"jpn": "ja", "cn": "yue", "xx": ""}[raw]
			got, err := metadatalanguage.Published(context.Background(), db, item.Public)
			if err != nil || got != want {
				t.Fatal("published language", got, want, err)
			}
			detail, err := catalog.New(db).Detail(catalog.Viewer{Profile: "profile", Fence: "f", Libraries: []string{"lib"}}, "server", item.Public, false)
			if err != nil {
				t.Fatal(err)
			}
			if want != "" && (detail.Facts == nil || detail.Facts.OriginalLanguage != want) {
				t.Fatal("detail language", detail.Facts, want)
			}
			if want == "" && detail.Facts != nil && detail.Facts.OriginalLanguage != "" {
				t.Fatal("unknown inferred", detail.Facts)
			}
			for _, field := range repairSchema("item", "movie") {
				if field.Field == "language" || field.Field == "originalLanguage" {
					t.Fatal("original language is not an owner control", field)
				}
			}
			// The editor's different metadata language cannot supply this fact.
			target := RepairTarget{Kind: "item", ID: item.Public}
			state, editErr := s.RepairState(context.Background(), target, allowAll)
			if editErr != nil {
				t.Fatal(editErr)
			}
			preference := "fr"
			if _, editErr = s.Repair(context.Background(), target, RepairCommand{ExpectedRevision: state.Revision, Action: "edit", Fields: map[string]RepairFieldEdit{"metadataLanguage": {Value: &preference}}}, editorActor(), allowAll); editErr != nil {
				t.Fatal(editErr)
			}
			if got, err = metadatalanguage.Published(context.Background(), db, item.Public); err != nil || got != want {
				t.Fatal("editor preference affected original language", got, err)
			}
		})
	}
}

func TestOriginalLanguageUnknownRefreshWithdrawsPriorFactDuringFillMissing(t *testing.T) {
	s, db, p := screenFixture(t)
	item := publicationFixtureMovie(t, db)
	ctx := context.Background()
	screenConfirm(t, s)
	p.record.Language = "ja"
	if err := s.ScreenStep(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := metadatalanguage.Published(ctx, db, item.Public); err != nil || got != "ja" {
		t.Fatal("initial published language", got, err)
	}
	// Expire the fixture's provider receipt and schedule the same identity.
	// Both publications still pass through the actual ScreenStep producer.
	publicationExec(t, db, `UPDATE screen_metadata_policies SET refresh_mode='fill_missing',revision=revision+1 WHERE library_id='lib';
UPDATE screen_metadata_work SET status='pending',generation=generation+1,revision=revision+1,lease='',lease_until='',next_attempt='',error='' WHERE target_kind='item' AND target_id=?;
DELETE FROM metadata_document_freshness`, item.ID)
	p.record.Language = "xx"
	p.record.Overview = "Replacement description"
	if err := s.ScreenStep(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := metadatalanguage.Published(ctx, db, item.Public); err != nil || got != "" {
		t.Fatal("unknown refresh retained prior language", got, err)
	}
	if got := publicationScalar(t, db, `SELECT count(*) FROM screen_metadata_fields WHERE target_kind='item' AND target_id=? AND field='language'`, item.ID); got != "0" {
		t.Fatal("obsolete language remains stored", got)
	}
	if got := publicationScalar(t, db, `SELECT json_extract(value,'$') FROM screen_metadata_fields WHERE target_kind='item' AND target_id=? AND field='overview'`, item.ID); got != "Verified description" {
		t.Fatal("fill-missing description changed", got)
	}
	if got := publicationScalar(t, db, `SELECT count(*) FROM screen_metadata_publications WHERE target_kind='item' AND target_id=? AND decision='accepted'`, item.ID); got != "1" {
		t.Fatal("accepted publication authority", got)
	}
}
