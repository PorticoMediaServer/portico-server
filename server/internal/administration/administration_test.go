package administration

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
)

func newService(t *testing.T) (*Service, *sql.DB, string) {
	t.Helper()
	root := t.TempDir()
	db, err := persistence.Open(filepath.Join(root, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	service := New(db)
	// The service derives its state directory from the open database file. On a
	// host where the temporary directory is itself a symlink the two spellings
	// differ, so they are compared after resolution and the resolved spelling is
	// what the rest of the test uses.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if service.StateDirectory() != resolved {
		t.Fatalf("state directory %q, want %q", service.StateDirectory(), resolved)
	}
	return service, db, service.StateDirectory()
}

// The composition root builds this service while other work may hold the single
// database connection, so construction must not read anything.
func TestConstructionNeverTouchesTheDatabase(t *testing.T) {
	root := t.TempDir()
	db, err := persistence.Open(filepath.Join(root, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// One connection, held open for the duration: a constructor that queried the
	// database here would block until this transaction ended.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	done := make(chan *Service, 1)
	go func() { done <- New(db) }()
	select {
	case service := <-done:
		if service == nil {
			t.Fatal("no service")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("constructing the administration service blocked on the database")
	}
}

// seedLibrary installs one library with one item backed by two files, which is
// what the deletion tests operate on. The item is created through the write
// API; the returned id is its public id.
func seedLibrary(t *testing.T, db *sql.DB, root string) (string, string, []string) {
	t.Helper()
	library := "lib-1"
	media := filepath.Join(root, "media")
	if err := os.MkdirAll(media, 0700); err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(media, "part1.mkv"), filepath.Join(media, "part2.mkv")}
	for index, path := range paths {
		if err := os.WriteFile(path, []byte(strconv.Itoa(index)+"0123456789"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO libraries(id,name,kind,root) VALUES(?,'Films','movie',?)`, library, media); err != nil {
		t.Fatal(err)
	}
	var entity int64
	err := dbwork.WithWriteTx(ctx, db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		handle, e := compactcatalog.LibraryTx(ctx, tx, library)
		if e != nil {
			return e
		}
		if entity, _, e = compactcatalog.UpsertEntityTx(ctx, tx, compactcatalog.Entity{
			Library: handle, Kind: compactcatalog.Movie,
			Key: compactcatalog.ItemKey(media, paths[0], 0), Title: "The Lighthouse",
		}); e != nil {
			return e
		}
		for index, path := range paths {
			var asset int64
			if asset, _, e = compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{
				Path: path, Size: 11, ModifiedNS: 0,
				Container: "mkv", VideoCodec: "h264", AudioCodec: "aac",
				Width: 1920, Height: 1080, Duration: 60,
			}); e != nil {
				return e
			}
			if e = compactcatalog.LinkAssetTx(ctx, tx, entity, asset, compactcatalog.Link{Part: index}); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var item string
	if err = db.QueryRow(`SELECT pid(public_id) FROM catalog_entities WHERE id=?`, entity).Scan(&item); err != nil {
		t.Fatal(err)
	}
	return library, item, paths
}

// entityID resolves a test item's public id to the integer the tables hold.
func entityID(t *testing.T, db *sql.DB, public string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)`, public).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func addCatalogTestEntity(t *testing.T, db *sql.DB, library, key string, kind compactcatalog.Kind, title string) string {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := dbwork.WithWriteTx(ctx, db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		handle, err := compactcatalog.LibraryTx(ctx, tx, library)
		if err != nil {
			return err
		}
		id, _, err = compactcatalog.UpsertEntityTx(ctx, tx, compactcatalog.Entity{
			Library: handle, Kind: kind, Key: key, Title: title,
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var public string
	if err := db.QueryRow(`SELECT pid(public_id) FROM catalog_entities WHERE id=?`, id).Scan(&public); err != nil {
		t.Fatal(err)
	}
	return public
}

// entityAssets lists a test item's asset tokens in link order.
func entityAssets(t *testing.T, db *sql.DB, id int64) []string {
	t.Helper()
	rows, err := db.Query(`SELECT a.token FROM catalog_asset_links l JOIN catalog_assets a ON a.id=l.asset_id WHERE l.entity_id=? ORDER BY l.part_index`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var token string
		if err = rows.Scan(&token); err != nil {
			t.Fatal(err)
		}
		out = append(out, token)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func allow(context.Context, *sql.Tx) error { return nil }

func TestLibraryContinueWatchingSettingsRoundTrip(t *testing.T) {
	s, db, root := newService(t)
	library, _, _ := seedLibrary(t, db, root)
	ctx := context.Background()
	doc, err := s.LibrarySettingsFor(ctx, allow, library)
	if err != nil || doc.Settings.ContinueWatching.Weeks != 16 || doc.Settings.ContinueWatching.MaximumItems != 40 || doc.Settings.ContinueWatching.IncludeSeasonPremieres == nil || !*doc.Settings.ContinueWatching.IncludeSeasonPremieres || doc.Settings.ContinueWatching.VideoPlayedThreshold != 90 || doc.Settings.ContinueWatching.VideoCompletion != "earliest" {
		t.Fatalf("Continue Watching defaults: %+v %v", doc.Settings.ContinueWatching, err)
	}
	settings := doc.Settings
	settings.ContinueWatching.Weeks = 4
	settings.ContinueWatching.MaximumItems = 12
	settings.ContinueWatching.VideoPlayedThreshold = 85
	settings.ContinueWatching.VideoCompletion = "credits"
	no := false
	settings.ContinueWatching.IncludeSeasonPremieres = &no
	saved, err := s.SaveLibrarySettings(ctx, allow, library, Change[LibrarySettings]{ExpectedRevision: doc.Revision, Settings: settings, OperationID: "test-policy-key-1"})
	if err != nil || saved.Settings.ContinueWatching.Weeks != 4 || saved.Settings.ContinueWatching.MaximumItems != 12 || saved.Settings.ContinueWatching.IncludeSeasonPremieres == nil || *saved.Settings.ContinueWatching.IncludeSeasonPremieres || saved.Settings.ContinueWatching.VideoPlayedThreshold != 85 || saved.Settings.ContinueWatching.VideoCompletion != "credits" {
		t.Fatalf("Continue Watching save: %+v %v", saved.Settings.ContinueWatching, err)
	}
	settings.ContinueWatching.Weeks = 105
	if _, err = s.SaveLibrarySettings(ctx, allow, library, Change[LibrarySettings]{ExpectedRevision: saved.Revision, Settings: settings, OperationID: "test-policy-key-2"}); err == nil {
		t.Fatal("unbounded Continue Watching window accepted")
	}
}

func TestLibrarySettingsWritesEffectivePolicyAndRejectsUnwiredKeys(t *testing.T) {
	service, db, root := newService(t)
	library, _, _ := seedLibrary(t, db, root)
	ctx := context.Background()
	// Online lookups are on by default (0079); an owner who withdrew the
	// server-wide consent must not get it back through a library save.
	if _, err := db.Exec(`UPDATE screen_metadata_consent SET confirmed=0,confirmed_at='2026-09-01T00:00:00Z' WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
	doc, err := service.LibrarySettingsFor(ctx, allow, library)
	if err != nil {
		t.Fatal(err)
	}
	settings := doc.Settings
	settings.AllowMediaDeletion = true
	settings.Providers = []ProviderSelection{{MediaKind: "movie", Provider: "tmdb", APIKey: "secret-key", Language: "en", Region: "GB"}}
	if _, err = service.SaveLibrarySettings(ctx, allow, library, Change[LibrarySettings]{ExpectedRevision: doc.Revision, Settings: settings, OperationID: "test-policy-key-3"}); err == nil {
		t.Fatal("unwired credentials accepted")
	}
	settings.Providers[0].APIKey = ""
	settings.Analysis = []string{"probe", "trickplay"}
	settings.Navigation.TrickplayTileWidth = 480
	saved, err := service.SaveLibrarySettings(ctx, allow, library, Change[LibrarySettings]{ExpectedRevision: doc.Revision, Settings: settings, OperationID: "test-policy-key-4"})
	if err != nil {
		t.Fatal(err)
	}
	var width int
	var operations, stored string
	var confirmed bool
	if err = db.QueryRow(`SELECT trickplay_tile_width,operations_json FROM library_scan_policies WHERE library_id=?`, library).Scan(&width, &operations); err != nil || width != 480 || !contains(operations, "trickplay") {
		t.Fatal(width, operations, err)
	}
	if err = db.QueryRow(`SELECT confirmed FROM screen_metadata_consent WHERE singleton=1`).Scan(&confirmed); err != nil || confirmed {
		t.Fatal("save granted remote consent", confirmed, err)
	}
	if _, err = service.SaveLibrarySettings(ctx, allow, library, Change[LibrarySettings]{ExpectedRevision: doc.Revision, Settings: settings, OperationID: "test-policy-key-5"}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale save", err)
	}
	if _, err = db.Exec(`UPDATE library_scan_policies SET revision=revision+1,operations_json='["probe"]' WHERE library_id=?`, library); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveLibrarySettings(ctx, allow, library, Change[LibrarySettings]{ExpectedRevision: saved.Revision, Settings: settings, OperationID: "test-policy-key-6"}); !errors.Is(err, ErrConflict) {
		t.Fatal("domain change did not invalidate admin revision", err)
	}
	read, err := service.LibrarySettingsFor(ctx, allow, library)
	if err != nil || contains(digestOf(read.Settings.Analysis), "trickplay") {
		t.Fatal(read, err)
	}
	for _, op := range read.Settings.Analysis {
		if op == "trickplay" {
			t.Fatal("stale admin JSON won")
		}
	}
	if err = db.QueryRow(`SELECT body FROM admin_documents WHERE scope=?`, libraryScope(library)).Scan(&stored); err != nil || contains(stored, "secret-key") {
		t.Fatal("credential persisted", err)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

func TestServerWideTrashRetentionGovernsAFreshLibrary(t *testing.T) {
	service, db, root := newService(t)
	library, _, _ := seedLibrary(t, db, root)
	// The owner settings document is the server-wide authority; a library that has
	// not been configured inherits it rather than a value hard-coded here.
	if _, err := db.Exec(`INSERT INTO console_documents VALUES('server',1,'{"library.trashRetentionDays":3}',0)`); err != nil {
		t.Fatal(err)
	}
	document, err := service.LibrarySettingsFor(context.Background(), allow, library)
	if err != nil {
		t.Fatal(err)
	}
	if document.Settings.TrashRetentionDays != 3 {
		t.Fatalf("retention %d, want the server-wide 3", document.Settings.TrashRetentionDays)
	}
}

func TestLibrarySettingsRefusesUnsatisfiedAnalysisDependency(t *testing.T) {
	service, db, root := newService(t)
	library, _, _ := seedLibrary(t, db, root)
	settings := DefaultLibrarySettings()
	// Trickplay needs probe; asking for trickplay alone would never run.
	settings.Analysis = []string{"trickplay"}
	_, err := service.SaveLibrarySettings(context.Background(), allow, library, Change[LibrarySettings]{ExpectedRevision: 1, Settings: settings, OperationID: "test-policy-key-7"})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Fields[0] != "settings.analysis" {
		t.Fatalf("error %v, want a validation failure naming settings.analysis", err)
	}
}

func TestDeletePreviewAndDeleteMoveFilesToTrashAndRestore(t *testing.T) {
	service, db, root := newService(t)
	library, item, paths := seedLibrary(t, db, root)
	ctx := context.Background()
	// Deletion is refused until the library enables it.
	preview, err := service.PreviewDelete(ctx, allow, []string{item})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blocked) != 1 || preview.Confirmation != "The Lighthouse" || preview.ConfirmationKind != "title" {
		t.Fatalf("preview %+v", preview)
	}
	if len(preview.Targets[0].Files) != 2 || !preview.Targets[0].Files[0].Present {
		t.Fatalf("preview must list both present files: %+v", preview.Targets[0].Files)
	}
	request := DeleteRequest{ItemIDs: []string{item}, DeleteFiles: true, Confirmation: "The Lighthouse", ExpectedRevision: preview.Revision, OperationID: "test-policy-key-8"}
	if _, err = service.Delete(ctx, allow, "local:a:b", request); !errors.Is(err, ErrDenied) {
		t.Fatalf("delete error %v, want denied while the library forbids it", err)
	}
	settings := DefaultLibrarySettings()
	settings.AllowMediaDeletion = true
	if _, err = service.SaveLibrarySettings(ctx, allow, library, Change[LibrarySettings]{ExpectedRevision: 1, Settings: settings, OperationID: "test-policy-key-9"}); err != nil {
		t.Fatal(err)
	}
	// A wrong confirmation is refused before anything moves.
	preview, err = service.PreviewDelete(ctx, allow, []string{item})
	if err != nil {
		t.Fatal(err)
	}
	wrong := DeleteRequest{ItemIDs: []string{item}, DeleteFiles: true, Confirmation: "the lighthouse", ExpectedRevision: preview.Revision, OperationID: "test-policy-key-10"}
	if _, err = service.Delete(ctx, allow, "local:a:b", wrong); !errors.Is(err, ErrConfirmation) {
		t.Fatalf("delete error %v, want a confirmation mismatch", err)
	}
	// A stale fence is refused too.
	stale := DeleteRequest{ItemIDs: []string{item}, DeleteFiles: true, Confirmation: "The Lighthouse", ExpectedRevision: preview.Revision + 99, OperationID: "test-policy-key-11"}
	if _, err = service.Delete(ctx, allow, "local:a:b", stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("delete error %v, want a revision conflict", err)
	}
	result, err := service.Delete(ctx, allow, "local:a:b", DeleteRequest{ItemIDs: []string{item}, DeleteFiles: true, Confirmation: "The Lighthouse", ExpectedRevision: preview.Revision, OperationID: "delete-one"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 || result.Receipts[0].FilesMoved != 2 || result.Receipts[0].TrashEntryID == "" {
		t.Fatalf("result %+v", result)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s should have been moved into the trash", path)
		}
	}
	var items int
	if err = db.QueryRow(`SELECT COUNT(*) FROM catalog_entities WHERE public_id=pid_blob(?)`, item).Scan(&items); err != nil || items != 1 {
		t.Fatalf("catalog identity must remain in Trash: %d %v", items, err)
	}
	// Replaying the same operation identifier returns the first outcome.
	replay, err := service.Delete(ctx, allow, "local:a:b", DeleteRequest{ItemIDs: []string{item}, DeleteFiles: true, Confirmation: "The Lighthouse", ExpectedRevision: preview.Revision, OperationID: "delete-one"})
	if err != nil || replay.Removed != 1 {
		t.Fatalf("replay %+v %v", replay, err)
	}
	page, err := service.Trash(ctx, allow, "held", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.HeldCount != 1 || len(page.Items) != 1 || page.Items[0].FileCount != 2 || page.Items[0].ExpiresAt == "" {
		t.Fatalf("trash page %+v", page)
	}
	restored, err := service.RestoreFromTrash(ctx, allow, page.Items[0].ID, "restore-one")
	if err != nil {
		t.Fatal(err)
	}
	if restored.Restored != 2 || restored.Skipped != 0 || restored.Reindex {
		t.Fatalf("restore %+v", restored)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s should be back on disk: %v", path, err)
		}
	}
}

func TestEmptyTrashNeedsTheCountTypedBack(t *testing.T) {
	service, db, root := newService(t)
	library, item, _ := seedLibrary(t, db, root)
	ctx := context.Background()
	settings := DefaultLibrarySettings()
	settings.AllowMediaDeletion = true
	if _, err := service.SaveLibrarySettings(ctx, allow, library, Change[LibrarySettings]{ExpectedRevision: 1, Settings: settings, OperationID: "test-policy-key-12"}); err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewDelete(ctx, allow, []string{item})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Delete(ctx, allow, "local:a:b", DeleteRequest{ItemIDs: []string{item}, DeleteFiles: true, Confirmation: "The Lighthouse", ExpectedRevision: preview.Revision, OperationID: "test-policy-key-13"}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.EmptyTrash(ctx, allow, EmptyTrashRequest{Confirmation: "2", OperationID: "test-policy-key-14"}); !errors.Is(err, ErrConfirmation) {
		t.Fatalf("error %v, want a confirmation mismatch", err)
	}
	// Expired-only keeps an entry whose retention has not run out.
	expired, err := service.EmptyTrash(ctx, allow, EmptyTrashRequest{ExpiredOnly: true, Confirmation: "0", OperationID: "test-policy-key-15"})
	if err != nil || expired.Purged != 0 || expired.Remaining != 1 {
		t.Fatalf("expired sweep %+v %v", expired, err)
	}
	result, err := service.EmptyTrash(ctx, allow, EmptyTrashRequest{Confirmation: "1", OperationID: "empty-trash-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Purged != 1 || result.Remaining != 0 || result.BytesFreed == 0 {
		t.Fatalf("empty %+v", result)
	}
	if entries, err := os.ReadDir(filepath.Join(root, "trash")); err != nil || len(entries) != 0 {
		t.Fatalf("the trash tree should be empty: %v %v", entries, err)
	}
}

func TestBrowseRootsDirectoryPagingAndRefusals(t *testing.T) {
	service, _, root := newService(t)
	ctx := context.Background()
	media := filepath.Join(t.TempDir(), "browse")
	media, err := filepath.EvalSymlinks(filepath.Dir(media))
	if err != nil {
		t.Fatal(err)
	}
	media = filepath.Join(media, "browse")
	service.PickerRoots = []string{media}
	for _, name := range []string{"Alpha", "Beta", "Gamma", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(media, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	roots, err := service.Browse(ctx, "", "", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots.Roots) == 0 || roots.Platform == "" || roots.Separator == "" {
		t.Fatalf("every platform must publish at least one root: %+v", roots)
	}
	if len(roots.Roots) != 1 || roots.Roots[0].Path != media {
		t.Fatalf("picker exposed unconfigured roots: %+v", roots.Roots)
	}
	if err := os.WriteFile(filepath.Join(media, "notes.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	page, err := service.Browse(ctx, media, "", 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 2 || page.Entries[0].Name != "Alpha" || page.NextCursor == "" || page.Writable {
		t.Fatalf("first page %+v", page)
	}
	second, err := service.Browse(ctx, media, page.NextCursor, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Entries) != 1 || second.Entries[0].Name != "Gamma" || second.NextCursor != "" {
		t.Fatalf("second page %+v", second)
	}
	// Files appear only when asked for; dotfiles never do.
	withFiles, err := service.Browse(ctx, media, "", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, entry := range withFiles.Entries {
		names[entry.Name] = true
	}
	if !names["notes.txt"] || names[".hidden"] {
		t.Fatalf("entries %+v", withFiles.Entries)
	}
	if _, err = service.Browse(ctx, "relative/path", "", 0, false); !errors.Is(err, ErrInput) {
		t.Fatalf("error %v, want a refusal for a relative path", err)
	}
	if _, err = service.Browse(ctx, filepath.Join(media, "missing"), "", 0, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error %v, want not found", err)
	}
	if _, err = service.Browse(ctx, root, "", 0, true); !errors.Is(err, ErrDenied) {
		t.Fatalf("state directory was browsable: %v", err)
	}
	if err := os.Symlink(root, filepath.Join(media, "state-link")); err == nil {
		if _, err = service.Browse(ctx, filepath.Join(media, "state-link"), "", 0, true); !errors.Is(err, ErrDenied) {
			t.Fatalf("symlink into state was browsable: %v", err)
		}
	}
	if entries, err := os.ReadDir(media); err != nil {
		t.Fatal(err)
	} else {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".portico-write-probe-") {
				t.Fatal("browse created a write probe")
			}
		}
	}
	// A cursor from one listing is refused by another.
	if _, err = service.Trash(ctx, allow, "held", page.NextCursor, 0); !errors.Is(err, ErrCursor) {
		t.Fatalf("error %v, want a stale cursor refusal", err)
	}
}

func TestDVRTemplatesAndTunerConflicts(t *testing.T) {
	service, _, _ := newService(t)
	ctx := context.Background()
	document, err := service.DVRSettings(ctx, allow)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.FolderTokens) != 0 || len(document.Enumerations["keepMode"]) == 0 {
		t.Fatalf("the page must publish its vocabularies: %+v", document)
	}
	settings := document.Settings
	for _, bad := range []string{"{series}/{unknown}", "../{title}", "/{title}", "Season {season}", "{title:02}"} {
		settings.FolderTemplate = bad
		if _, err = service.SaveDVRSettings(ctx, allow, Change[DVRDefaults]{ExpectedRevision: 1, Settings: settings, OperationID: "test-policy-key-16"}); err == nil {
			t.Fatalf("template %q should have been refused", bad)
		}
	}
	settings.FolderTemplate = ""
	settings.PrePaddingSeconds = 120
	saved, err := service.SaveDVRSettings(ctx, allow, Change[DVRDefaults]{ExpectedRevision: 1, Settings: settings, OperationID: "test-policy-key-17"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 2 {
		t.Fatalf("revision %d", saved.Revision)
	}
	// Three overlapping recordings against two tuners is one conflict.
	list := []TunerAssignment{
		{StartsAt: "2026-09-16T20:00:00Z", EndsAt: "2026-09-16T21:00:00Z"},
		{StartsAt: "2026-09-16T20:30:00Z", EndsAt: "2026-09-16T21:30:00Z"},
		{StartsAt: "2026-09-16T20:45:00Z", EndsAt: "2026-09-16T21:15:00Z"},
	}
	if excess := overlapExcess(list, 2); excess != 1 {
		t.Fatalf("overlap excess %d, want 1", excess)
	}
	if excess := overlapExcess(list, 3); excess != 0 {
		t.Fatalf("overlap excess %d, want 0", excess)
	}
}

func TestMaintenanceRetentionMaximaAndUpdateReport(t *testing.T) {
	service, _, _ := newService(t)
	ctx := context.Background()
	document, err := service.MaintenanceSettings(ctx, allow)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Cadences) == 0 || document.Maxima["logs"] != 365 {
		t.Fatalf("maintenance vocabularies %+v", document)
	}
	settings := document.Settings
	settings.Retention["logs"] = 4000
	if _, err = service.SaveMaintenanceSettings(ctx, allow, Change[MaintenanceSettingsDocument]{ExpectedRevision: 1, Settings: settings, OperationID: "test-policy-key-18"}); err == nil {
		t.Fatal("a retention beyond the published maximum must be refused")
	}
	settings.Retention["logs"] = 365
	settings.Windows[0].Timezone = "Not/AZone"
	if _, err = service.SaveMaintenanceSettings(ctx, allow, Change[MaintenanceSettingsDocument]{ExpectedRevision: 1, Settings: settings, OperationID: "test-policy-key-19"}); err == nil {
		t.Fatal("an unloadable timezone must be refused")
	}
	settings.Windows[0].Timezone = "Europe/London"
	settings.Windows[0].Cadence = "weekends"
	settings.Windows[0].Days = []string{"saturday", "sunday"}
	saved, err := service.SaveMaintenanceSettings(ctx, allow, Change[MaintenanceSettingsDocument]{ExpectedRevision: 1, Settings: settings, OperationID: "maintenance-1"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 2 || saved.Settings.Windows[0].Cadence != "weekends" {
		t.Fatalf("saved %+v", saved.Settings.Windows)
	}
	// With no feed the report says so and never claims an update.
	current := UpdateBuild{Version: "1.2.0", BuildID: "b1"}
	report := service.Updates(ctx, current, "stable", "")
	if report.Status != "no-feed" || report.State != "unconfigured" || report.UpdateAvailable || report.AutoInstall {
		t.Fatalf("report %+v", report)
	}
	service.Fetch = func(context.Context, string) ([]byte, error) {
		return []byte(`{"channels":{"stable":[{"version":"1.1.0"},{"version":"1.3.0","notes":"Faster scans","url":"https://releases.example/notes/1.3.0"}],"beta":[{"version":"2.0.0-rc1"}]}}`), nil
	}
	report = service.Updates(ctx, current, "stable", "https://releases.example/portico.json")
	if report.Status != "update-available" || report.State != "available" || report.Latest == nil || report.Latest.Version != "1.3.0" || report.NotesURL != "https://releases.example/notes/1.3.0" || report.AutoInstall {
		t.Fatalf("report %+v", report)
	}
	// A pre-release sorts below the same numbers without one.
	report = service.Updates(ctx, UpdateBuild{Version: "2.0.0"}, "beta", "https://releases.example/portico.json")
	if report.UpdateAvailable {
		t.Fatalf("2.0.0-rc1 must not read as newer than 2.0.0: %+v", report.Latest)
	}
	if report.State != "current" {
		t.Fatalf("a non-newer release changed the state: %+v", report)
	}
	if safeUpdateURL("javascript:alert(1)") != "" || safeUpdateURL("https://user:secret@releases.example/notes") != "" || safeUpdateURL("http://releases.example/notes") != "" {
		t.Fatal("unsafe release notes URL accepted")
	}
}

func TestMaintenanceSavePreservesOmittedBackgroundPriority(t *testing.T) {
	service, _, _ := newService(t)
	ctx := context.Background()
	current, err := service.MaintenanceSettings(ctx, allow)
	if err != nil {
		t.Fatal(err)
	}
	settings := current.Settings
	settings.BackgroundTaskPriority = "normal"
	saved, err := service.SaveMaintenanceSettings(ctx, allow, Change[MaintenanceSettingsDocument]{ExpectedRevision: current.Revision, Settings: settings, OperationID: "priority-normal"})
	if err != nil {
		t.Fatal(err)
	}
	settings = saved.Settings
	settings.BackgroundTaskPriority = ""
	saved, err = service.SaveMaintenanceSettings(ctx, allow, Change[MaintenanceSettingsDocument]{ExpectedRevision: saved.Revision, Settings: settings, OperationID: "retention-only"})
	if err != nil || saved.Settings.BackgroundTaskPriority != "normal" {
		t.Fatalf("omitted priority reset: %+v, %v", saved.Settings, err)
	}
	loaded, err := service.MaintenanceSettings(ctx, allow)
	if err != nil || loaded.Settings.BackgroundTaskPriority != "normal" {
		t.Fatalf("stored priority: %+v, %v", loaded.Settings, err)
	}
}

func TestStorageReportAndCleanupPolicy(t *testing.T) {
	service, _, root := newService(t)
	ctx := context.Background()
	analysis := filepath.Join(root, "analysis", "tiles")
	if err := os.MkdirAll(analysis, 0700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(analysis, "old.jpg")
	if err := os.WriteFile(old, make([]byte, 2048), 0600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().AddDate(0, 0, -400)
	if err := os.Chtimes(old, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(analysis, "fresh.jpg")
	if err := os.WriteFile(fresh, make([]byte, 1024), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := service.Storage(ctx, allow)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, category := range report.Categories {
		if category.ID == "trickplay" {
			found = category.Bytes == 3072 && category.FileCount == 2 && category.Present
		}
		if category.ID == "recordings" && category.Cleanable {
			t.Fatal("recordings are content and must never be cleanable")
		}
	}
	if !found || report.TotalBytes == 0 {
		t.Fatalf("storage report %+v", report.Categories)
	}
	if _, err = service.Cleanup(ctx, allow, "recordings", CleanupRequest{Confirmation: "recordings", OperationID: "test-policy-key-20"}); !errors.Is(err, ErrDenied) {
		t.Fatalf("error %v, want denied", err)
	}
	if _, err = service.Cleanup(ctx, allow, "logs", CleanupRequest{Confirmation: "wrong", OperationID: "test-policy-key-21"}); !errors.Is(err, ErrConfirmation) {
		t.Fatalf("error %v, want a confirmation mismatch", err)
	}
	// Generated assets are managed by their domain owner, never a raw tree sweep.
	if _, err = service.Cleanup(ctx, allow, "trickplay", CleanupRequest{Confirmation: "trickplay", OperationID: "cleanup-1"}); !errors.Is(err, ErrDenied) {
		t.Fatalf("expected owner-managed cleanup refusal, got %v", err)
	}
	if _, err = os.Stat(old); err != nil {
		t.Fatal("old referenced artifacts must survive")
	}
	if _, err = os.Stat(fresh); err != nil {
		t.Fatal("a file inside the retention window must survive")
	}
}

func base64Raw(value []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	out := make([]byte, 0, (len(value)*8+5)/6)
	bits, count := 0, 0
	for _, b := range value {
		bits = bits<<8 | int(b)
		count += 8
		for count >= 6 {
			count -= 6
			out = append(out, alphabet[(bits>>count)&63])
		}
	}
	if count > 0 {
		out = append(out, alphabet[(bits<<(6-count))&63])
	}
	return string(out)
}
