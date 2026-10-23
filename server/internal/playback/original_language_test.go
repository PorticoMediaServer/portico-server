package playback

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"portico.local/server/internal/catalogtest"
)

func TestOriginalLanguageUsesCurrentAcceptedPublication(t *testing.T) {
	c := catalogtest.Open(t)
	movies := c.Library("movies", "Movies", "movie", "/movies")
	tv := c.Library("tv", "TV", "tv", "/tv")
	movie := c.Movie(movies, "/movies/film.mkv", "Film", 2026)
	show := c.Show(tv, "Show", 2026)
	season := c.Season(show, 1)
	episode := c.Episode(show, season, 1, "/tv/show/s01e01.mkv")
	c.Drain()
	ctx := context.Background()
	publish := func(kind string, id int64, library, provider, value string) {
		t.Helper()
		publication := fmt.Sprintf("%s-%d", kind, id)
		raw, _ := json.Marshal(value)
		c.Write(func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `INSERT INTO screen_metadata_work(target_kind,target_id,library_id,title_seed,accepted_publication) VALUES(?,?,?,'Fixture',?) ON CONFLICT(target_kind,target_id) DO UPDATE SET accepted_publication=excluded.accepted_publication`, kind, id, library, publication); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO screen_metadata_publications(id,target_kind,target_id,provider,provider_type,provider_id,payload,confidence,reasons,source_kind,input_digest,query_digest,observed_at,language,region,selection_revision,generation,decision) VALUES(?,?,?,?,'movie','fixture','{}',1,'[]','provider','','','2026-10-23T00:00:00Z','en','',1,1,'accepted')`, publication, kind, id, provider); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO screen_metadata_fields(target_kind,target_id,field,value,provider,source_kind,source_url,confidence,observed_at,language,region,selection_revision,publication_id) VALUES(?,?,'language',?,?,'provider','',1,'2026-10-23T00:00:00Z','en','',1,?)`, kind, id, string(raw), provider, publication)
			return err
		})
	}
	publish("item", movie.ID, "movies", "tmdb", "ja")
	publish("show", show.ID, "tv", "tvdb", "ja")
	// Even a genuine item publication cannot override the episodic show's fact.
	publish("item", episode.ID, "tv", "tmdb", "en")
	read := func(public string) string {
		t.Helper()
		tx, err := c.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		language, err := originalAudioLanguageTx(ctx, tx, public)
		if err != nil {
			t.Fatal(err)
		}
		return language
	}
	for _, it := range []catalogtest.Item{movie, show, season, episode} {
		if got := read(it.Public); got != "ja" {
			t.Fatalf("title %s read %q", it.Public, got)
		}
	}
	if got := read("missing"); got != "" {
		t.Fatalf("invented language %q", got)
	}
	for _, test := range []struct {
		name, statement string
		args            []any
	}{
		{"superseded", `UPDATE screen_metadata_publications SET decision='superseded' WHERE target_kind='item' AND target_id=?`, []any{movie.ID}},
		{"not-current", `UPDATE screen_metadata_work SET accepted_publication='different' WHERE target_kind='item' AND target_id=?`, []any{movie.ID}},
		{"owner-field", `UPDATE screen_metadata_fields SET provider='owner' WHERE target_kind='item' AND target_id=?`, []any{movie.ID}},
		{"different-field", `UPDATE screen_metadata_fields SET field='metadata_language' WHERE target_kind='item' AND target_id=?`, []any{movie.ID}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, err := c.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err = tx.ExecContext(ctx, test.statement, test.args...); err != nil {
				t.Fatal(err)
			}
			got, err := originalAudioLanguageTx(ctx, tx, movie.Public)
			if err != nil || got != "" {
				t.Fatalf("unpublished fact used: %q %v", got, err)
			}
		})
	}
	for _, value := range []string{"und", "xx", "", "ja-JP", "not a language"} {
		t.Run("unknown-"+value, func(t *testing.T) {
			tx, err := c.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			raw, _ := json.Marshal(value)
			if _, err = tx.ExecContext(ctx, `UPDATE screen_metadata_fields SET value=? WHERE target_kind='item' AND target_id=?`, string(raw), movie.ID); err != nil {
				t.Fatal(err)
			}
			got, err := originalAudioLanguageTx(ctx, tx, movie.Public)
			if err != nil || got != "" {
				t.Fatalf("unknown language used: %q %v", got, err)
			}
		})
	}
}
