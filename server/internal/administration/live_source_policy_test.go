package administration

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"portico.local/server/internal/livechannels"
)

func TestLiveSourcePolicyCASRollbackAndImportUseOneNormalizedStore(t *testing.T) {
	s, db, _ := newService(t)
	ctx := context.Background()
	store, err := livechannels.New(db)
	if err != nil {
		t.Fatal(err)
	}
	authority := func(context.Context, *sql.Tx, bool) (string, func(string, string) bool, error) {
		return "owner", func(string, string) bool { return true }, nil
	}
	in := livechannels.SourceInput{ID: strings.Repeat("9a", 24), Name: "Policy fixture", RequestID: strings.Repeat("8b", 24), TunerCount: 1,
		Playlist: "#EXTM3U\n#EXTINF:-1 tvg-id=\"news\" group-title=\"News\",News\nhttps://fixture.invalid/news\n#EXTINF:-1 tvg-id=\"sport\" group-title=\"Sports\",Sport\nhttps://fixture.invalid/sport\n"}
	source, err := store.Save(ctx, authority, in)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := s.LiveSourceConfiguration(ctx, nil, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	doc.Settings.ChannelSelection = "groups"
	doc.Settings.EnabledGroups = []string{"News"}
	doc.Settings.EnabledChannels = []string{"sport"}
	doc.Settings.Filters.Keywords.Values = []string{"News"} // Inactive in Groups.
	doc.Settings.Logos.Mode = "import"
	change := Change[LiveSourceSettings]{ExpectedRevision: doc.Revision, OperationID: "live-source-group-choice", Settings: doc.Settings}
	saved, err := s.SaveLiveSourceConfiguration(ctx, nil, source.ID, change)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.SaveLiveSourceConfiguration(ctx, nil, source.ID, change)
	if err != nil || !reflect.DeepEqual(saved, replay) {
		t.Fatalf("owner replay: %+v %v", replay, err)
	}
	change.OperationID = "live-source-stale-write"
	if _, err = s.SaveLiveSourceConfiguration(ctx, nil, source.ID, change); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale policy write: %v", err)
	}
	in.ExpectedRevision, in.RequestID = source.Revision, strings.Repeat("7c", 24)
	source, err = store.Save(ctx, authority, in)
	if err != nil || source.Channels != 1 {
		t.Fatalf("source refresh ignored chosen group: %+v %v", source, err)
	}
	// The same complete policy remains available after import, including the
	// choices of inactive modes. It is not reconstructed from a drawn default.
	current, err := s.LiveSourceConfiguration(ctx, nil, source.ID)
	if err != nil || current.Revision != saved.Revision || !reflect.DeepEqual(current.Settings, saved.Settings) {
		t.Fatalf("persisted policy changed during import: %+v %v", current, err)
	}
	if _, err = db.Exec(`CREATE TRIGGER reject_live_source_filter BEFORE INSERT ON live_source_preference_items WHEN NEW.value='reject-filter' BEGIN SELECT RAISE(ABORT,'test rejected filter'); END`); err != nil {
		t.Fatal(err)
	}
	bad := current.Settings
	bad.ChannelSelection = "filter"
	bad.Filters.Keywords.Values = []string{"reject-filter"}
	if _, err = s.SaveLiveSourceConfiguration(ctx, nil, source.ID, Change[LiveSourceSettings]{ExpectedRevision: current.Revision, OperationID: "live-source-child-rollback", Settings: bad}); err == nil {
		t.Fatal("child write rejection committed")
	}
	after, err := s.LiveSourceConfiguration(ctx, nil, source.ID)
	if err != nil || after.Revision != current.Revision || !reflect.DeepEqual(after.Settings, current.Settings) {
		t.Fatalf("child failure changed parent or lists: %+v %v", after, err)
	}
	after.Settings.ChannelSelection = "picked"
	after.Settings.EnabledChannels = []string{}
	if _, err = s.SaveLiveSourceConfiguration(ctx, nil, source.ID, Change[LiveSourceSettings]{ExpectedRevision: after.Revision, OperationID: "live-source-choose-none", Settings: after.Settings}); err != nil {
		t.Fatal(err)
	}
	in.ExpectedRevision, in.RequestID = source.Revision, strings.Repeat("6d", 24)
	if source, err = store.Save(ctx, authority, in); err != nil || source.Channels != 0 {
		t.Fatalf("empty chosen list became All: %+v %v", source, err)
	}
	var graphs int
	if err = db.QueryRow(`SELECT count(*) FROM admin_documents WHERE scope=?`, liveSourceScope(source.ID)).Scan(&graphs); err != nil || graphs != 0 {
		t.Fatalf("source policy mirrored to a JSON graph: %d %v", graphs, err)
	}
}
