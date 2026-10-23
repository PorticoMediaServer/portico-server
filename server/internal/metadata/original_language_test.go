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
