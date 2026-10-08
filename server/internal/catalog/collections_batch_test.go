package catalog

import (
	"database/sql"
	"testing"
)

func allow(*sql.Tx) error { return nil }

func TestSavedCollectionVisibilityAndEntryOutcomes(t *testing.T) {
	_, s, names := phase34ListFixture(t)
	pub := func(name string) string { return names[name].Public }
	owner := ResourceActor{Authority: "local", AccountID: "account", ProfileID: "owner"}
	stranger := ResourceActor{Authority: "local", AccountID: "account", ProfileID: "stranger"}
	name := "Shared Picks"
	private := "private"
	server := "server"
	created, e := s.MutateSavedResource(owner, "", "create", SavedResourceMutation{OperationID: "create-1", Kind: "collection", Name: &name, Visibility: &private}, allow)
	if e != nil {
		t.Fatal(e)
	}
	id := created.ResourceID
	if _, e = s.SavedResource("s", "f", id, stranger); e != sql.ErrNoRows {
		t.Fatal("private collection exposed", e)
	}
	updated, e := s.MutateSavedResource(owner, id, "update", SavedResourceMutation{OperationID: "publish-1", ExpectedRevision: created.Revision, Visibility: &server}, allow)
	if e != nil {
		t.Fatal(e)
	}
	shared, e := s.SavedResource("s", "f", id, stranger)
	if e != nil {
		t.Fatal("server visibility not readable", e)
	}
	// Visibility publishes; it never delegates editing.
	if shared.Role != "viewer" || shared.Visibility != "server" {
		t.Fatal("role or visibility", shared)
	}
	for _, action := range shared.Actions {
		if action == "entries" || action == "update" || action == "delete" {
			t.Fatal("server visibility granted a write", shared.Actions)
		}
	}
	list, e := s.SavedResources("s", "f", stranger, "collection", "", false, 40)
	if e != nil || len(list.Resources) != 1 || list.Resources[0].ID != id {
		t.Fatal("server collection missing from member list", list, e)
	}
	entries, e := s.MutateSavedResource(owner, id, "entries", SavedResourceMutation{OperationID: "entries-1", ExpectedRevision: updated.Revision, AddItemIDs: []string{pub("a"), pub("b"), "missing"}}, allow)
	if e != nil {
		t.Fatal(e)
	}
	if entries.Entries == nil || len(entries.Entries.Added) != 2 || len(entries.Entries.Failed) != 1 || entries.Entries.Failed[0].Code != "not_found" {
		t.Fatal("entry outcomes", entries.Entries)
	}
	repeat, e := s.MutateSavedResource(owner, id, "entries", SavedResourceMutation{OperationID: "entries-2", ExpectedRevision: entries.Revision, AddItemIDs: []string{pub("a")}, RemoveEntryIDs: []string{"not-an-entry"}}, allow)
	if e != nil {
		t.Fatal(e)
	}
	if len(repeat.Entries.Unchanged) != 1 || repeat.Entries.Unchanged[0] != pub("a") || len(repeat.Entries.Failed) != 1 {
		t.Fatal("repeat outcomes", repeat.Entries)
	}
	content, e := s.SavedResourceContent(Viewer{Libraries: []string{"m"}}, "s", "f", id, owner, "", 40)
	if e != nil || len(content.Entries) != 2 {
		t.Fatal("membership", content, e)
	}
	removed, e := s.MutateSavedResource(owner, id, "entries", SavedResourceMutation{OperationID: "entries-3", ExpectedRevision: repeat.Revision, RemoveEntryIDs: []string{content.Entries[0].ID}}, allow)
	if e != nil {
		t.Fatal(e)
	}
	if len(removed.Entries.Removed) != 1 || len(removed.Entries.Failed) != 0 {
		t.Fatal("removal outcomes", removed.Entries)
	}
	bad := "public"
	if _, e = s.MutateSavedResource(owner, id, "update", SavedResourceMutation{OperationID: "publish-2", ExpectedRevision: removed.Revision, Visibility: &bad}, allow); e == nil {
		t.Fatal("unknown visibility accepted")
	}
}

func TestLibraryCollectionBatchReportsEachItem(t *testing.T) {
	_, s, names := phase34ListFixture(t)
	pub := func(name string) string { return names[name].Public }
	collection, e := s.CreateCollection("m", "Night Drives")
	if e != nil {
		t.Fatal(e)
	}
	out, e := s.SetCollectionItems(collection.ID, []string{pub("a"), pub("b"), pub("outside"), "missing"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	// An item in another library and an item that is gone are both outcomes,
	// never a rejected batch.
	if len(out.Entries.Added) != 2 || len(out.Entries.Failed) != 2 || out.Collection.ItemCount != 2 {
		t.Fatal("additions", out)
	}
	again, e := s.SetCollectionItems(collection.ID, []string{pub("a")}, []string{pub("b"), pub("c")})
	if e != nil {
		t.Fatal(e)
	}
	if len(again.Entries.Unchanged) != 1 || len(again.Entries.Removed) != 1 || len(again.Entries.Failed) != 1 || again.Collection.ItemCount != 1 {
		t.Fatal("mixed batch", again.Entries, again.Collection)
	}
	if _, e = s.SetCollectionItems(collection.ID, nil, nil); e == nil {
		t.Fatal("empty batch accepted")
	}
	if _, e = s.SetCollectionItems(collection.ID, []string{pub("a"), pub("a")}, nil); e == nil {
		t.Fatal("duplicate item accepted")
	}
}
