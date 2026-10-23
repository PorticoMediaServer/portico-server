package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/sonic"
)

func sonicStoredFixture(t *testing.T, c *catalogtest.Catalog, item catalogtest.Item, x, y float32) {
	t.Helper()
	values := make([]float32, sonic.Dimensions)
	values[0] = x
	values[1] = y
	vector, err := sonic.Normalize(values)
	if err != nil {
		t.Fatal(err)
	}
	data, err := vector.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprint(item.ID)
	c.Exec(`UPDATE catalog_assets SET audio_codec='flac',duration=2 WHERE id=?`, item.Asset)
	c.Exec(`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns) SELECT ?,src.id,a.token,src.incarnation,a.path,'r','{}',a.size,a.modified_ns FROM catalog_assets a JOIN catalog_asset_links link ON link.asset_id=a.id JOIN catalog_entities item ON item.id=link.entity_id JOIN catalog_libraries library ON library.id=item.library_id JOIN library_sources src ON src.library_id=library.library_id WHERE a.id=? LIMIT 1`, id, item.Asset)
	c.Exec(`INSERT INTO analysis_results(id,object_id,asset_id,source_revision,root_incarnation,configuration_generation,policy_revision,stage,algorithm,evidence,tool_digest,duration_us,summary_json,created_ms) SELECT ?,?,o.asset_id,o.revision,src.incarnation,src.generation,1,'sonic','portico-analysis-3:sonic:fixture','fixture','fixture',2000000,'{}',1 FROM inventory_objects o JOIN library_sources src ON src.id=o.source_id WHERE o.id=?`, id, id, id)
	c.Exec(`INSERT INTO analysis_heads(object_id,source_revision,stage,result_id) VALUES(?,'r','sonic',?)`, id, id)
	c.Exec(`INSERT INTO music_sonic_embeddings(result_id,entity_id,source_start_us,source_end_us,model_id,model_version,model_sha256,dimensions,vector) VALUES(?,?,0,2000000,?,?,?,200,?)`, id, item.ID, sonic.ModelID, sonic.ModelVersion, sonic.ModelSHA256, data)
}

func TestMusicSonicMeansUseOnlyCurrentProfilePermittedSongs(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("music", "Music", "music", "/music")
	artist := c.Artist(lib, "Artist")
	album := c.Album(artist, "Album", 2026)
	one := c.Song(album, 1, "/music/one.flac", "One")
	hidden := c.Song(album, 2, "/music/private.flac", "Private")
	sonicStoredFixture(t, c, one, 1, 0)
	sonicStoredFixture(t, c, hidden, 0, 1)
	c.Attributes(hidden.ID, "label", "private")
	c.Drain()
	s := New(c.DB)
	read := func(viewer Viewer, kind int) []sonicGroup {
		t.Helper()
		out := []sonicGroup{}
		err := dbwork.WithReadSnapshot(context.Background(), c.DB, func(ctx context.Context) error {
			return s.WithContext(ctx).sonicGroups(viewer, kind, 0, func(g sonicGroup) { out = append(out, g) })
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	viewer := Viewer{Profile: "local:a:open", Libraries: []string{"music"}}
	open := read(viewer, 6)
	if len(open) != 1 || open[0].vector[0] < .70 || open[0].vector[1] < .70 {
		t.Fatal("not an equal-track mean", open)
	}
	viewer.Profile = "local:a:restricted"
	viewer.Restrictions = identity.ContentRestrictions{BlockedLabels: []string{"private"}}
	private := read(viewer, 6)
	if len(private) != 1 || private[0].vector[0] != 1 || private[0].vector[1] != 0 {
		t.Fatal("hidden song influenced album", private)
	}
	artists := read(viewer, 5)
	if len(artists) != 1 || artists[0].id != artist.ID || artists[0].vector != private[0].vector {
		t.Fatal("artist mean crossed visibility", artists)
	}
	c.Exec(`UPDATE music_sonic_embeddings SET model_sha256='old' WHERE entity_id=?`, one.ID)
	if actual := read(viewer, 6); len(actual) != 0 {
		t.Fatal("partially analyzed album claimed a current mean", actual)
	}
	c.Exec(`UPDATE music_sonic_embeddings SET model_sha256=? WHERE entity_id=?`, sonic.ModelSHA256, one.ID)
	c.Exec(`UPDATE inventory_objects SET revision='replaced' WHERE id=?`, fmt.Sprint(one.ID))
	if actual := read(viewer, 7); len(actual) != 0 {
		t.Fatal("replaced source vector survived", actual)
	}
	c.Exec(`UPDATE inventory_objects SET revision='r' WHERE id=?`, fmt.Sprint(one.ID))
	c.Exec(`INSERT INTO library_sonic_settings(library_id,enabled,revision) VALUES('music',0,1)`)
	if actual := read(viewer, 6); len(actual) != 0 {
		t.Fatal("off library influenced sound selection", actual)
	}
	viewer.Libraries = []string{"other"}
	if actual := read(viewer, 7); len(actual) != 0 {
		t.Fatal("scope widened", actual)
	}
	if _, err := s.ListeningSimilarBySound(viewer, one.Public, 12); err != sql.ErrNoRows {
		t.Fatal("seed outside profile admitted", err)
	}
}
