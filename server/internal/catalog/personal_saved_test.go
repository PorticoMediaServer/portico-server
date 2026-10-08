package catalog

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/identity"
)

func savedFixture(t *testing.T) (*Service, *sql.DB, identity.Viewer) {
	t.Helper()
	c := catalogtest.Open(t)
	library := c.Library("library", "Library", "movie", "/fixture")
	c.Movie(library, "/fixture/item.mkv", "Movie", 2020)
	c.Drain()
	return New(c.DB), c.DB, identity.Viewer{Authority: "local", AccountID: "account", ProfileID: "profile"}
}

func phase34SavedFixtureItem(t *testing.T, db *sql.DB) catalogtest.Item {
	t.Helper()
	var item catalogtest.Item
	if err := db.QueryRow(`SELECT l.entity_id,a.id,a.token FROM catalog_assets a JOIN catalog_asset_links l ON l.asset_id=a.id WHERE a.path='/fixture/item.mkv'`).Scan(&item.ID, &item.Asset, &item.Token); err != nil {
		t.Fatal(err)
	}
	item.Public = catalogtest.New(t, db).Public(item.ID)
	return item
}

func TestPersonalQualifiedScopeConflictResolutionAndReplay(t *testing.T) {
	s, _, v := savedFixture(t)
	item := phase34SavedFixtureItem(t, s.db)
	key := identity.PersonalKey(v)
	yes, no := true, false
	offline := func(device string, watched *bool) PersonalMutation {
		return PersonalMutation{OperationID: device, ExpectedRevision: 0, Watched: watched, Offline: &PersonalOfflineMutation{DeviceID: device, DeviceMutationID: device, Sequence: 1, BaseRevision: 0, AuthoredAt: time.Now().UTC().Format(time.RFC3339)}}
	}
	first, e := s.SetPersonal("local:account", key, item.Public, offline("device-a", &yes), nil)
	if e != nil || !first.Watched {
		t.Fatal(first, e)
	}
	conflict, e := s.SetPersonal("local:account", key, item.Public, offline("device-b", &no), nil)
	if e != nil || conflict.Watched || conflict.Status != "needs-resolution" || len(conflict.Conflicts) != 1 || len(conflict.Conflicts[0].Choices) != 2 {
		t.Fatal(conflict, e)
	}
	resolve := PersonalMutation{OperationID: "resolve", ExpectedRevision: conflict.Revision, Resolution: &PersonalResolution{ConflictID: conflict.Conflicts[0].ID, ChoiceOperationID: "device-a"}}
	resolved, e := s.SetPersonal("local:account", key, item.Public, resolve, nil)
	if e != nil || !resolved.Watched || resolved.Status != "current" {
		t.Fatal(resolved, e)
	}
	retry, e := s.SetPersonal("local:account", key, item.Public, resolve, nil)
	if e != nil || retry.Revision != resolved.Revision {
		t.Fatal(retry, e)
	}
	v.Authority = "hosted"
	other, e := s.Personal(identity.PersonalKey(v), item.Public)
	if e != nil || other.Watched || other.Revision != 0 {
		t.Fatal("scope leakage", other, e)
	}
}
func TestPersonalManualFenceAndIndependentHistoryClear(t *testing.T) {
	s, db, v := savedFixture(t)
	item := phase34SavedFixtureItem(t, db)
	key := identity.PersonalKey(v)
	if _, e := db.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES(?,?,0,0,0,'play')`, key, item.ID); e != nil {
		t.Fatal(e)
	}
	evidence := func(seq int64, pos float64) {
		t.Helper()
		tx, e := db.Begin()
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback()
		if e = s.AcceptPlaybackProgress(tx, v, "play", item.Public, seq, pos, 100, "playing", true); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(); e != nil {
			t.Fatal(e)
		}
	}
	evidence(1, 40)
	p, e := s.Personal(key, item.Public)
	if e != nil || p.ProgressSeconds != 40 || p.Watched {
		t.Fatal(p, e)
	}
	yes := true
	p, e = s.SetPersonal("local:account", key, item.Public, PersonalMutation{OperationID: "manual", ExpectedRevision: p.Revision, Watched: &yes}, nil)
	if e != nil {
		t.Fatal(e)
	}
	evidence(2, 50)
	p, e = s.Personal(key, item.Public)
	if e != nil || !p.Watched || p.ProgressSeconds != 0 {
		t.Fatal("old writer overwrote manual choice", p, e)
	}
	var revision int64
	if e = db.QueryRow(`SELECT revision FROM personal_activity_revisions WHERE profile_id=?`, key).Scan(&revision); e != nil {
		t.Fatal(e)
	}
	noop := func(*sql.Tx) error { return nil }
	if _, e = s.MutateActivity(key, ActivityMutation{OperationID: "clear", ExpectedRevision: revision, Action: "clear-history"}, noop); e != nil {
		t.Fatal(e)
	}
	p, e = s.Personal(key, item.Public)
	if e != nil || !p.Watched {
		t.Fatal("history clear reset watched", p, e)
	}
	var count int
	if e = db.QueryRow(`SELECT count(*) FROM personal_history WHERE profile_id=?`, key).Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
}
func TestSavedCollectionStableSetMembershipAndCurrentRoles(t *testing.T) {
	s, _, v := savedFixture(t)
	item := phase34SavedFixtureItem(t, s.db)
	a := ResourceActor{Authority: v.Authority, AccountID: v.AccountID, ProfileID: v.ProfileID}
	name := "Collection"
	noop := func(*sql.Tx) error { return nil }
	create := SavedResourceMutation{OperationID: "create", Kind: "collection", Name: &name}
	receipt, e := s.MutateSavedResource(a, "", "create", create, noop)
	if e != nil {
		t.Fatal(e)
	}
	added, e := s.MutateSavedResource(a, receipt.ResourceID, "entries", SavedResourceMutation{OperationID: "add", ExpectedRevision: receipt.Revision, AddItemIDs: []string{item.Public, item.Public}}, noop)
	if e != nil {
		t.Fatal(e)
	}
	first, e := s.SavedResourceContent(Viewer{Libraries: []string{"library"}}, "server", "fence", receipt.ResourceID, a, "", 40)
	if e != nil || len(first.Entries) != 1 {
		t.Fatal(first, e)
	}
	added, e = s.MutateSavedResource(a, receipt.ResourceID, "entries", SavedResourceMutation{OperationID: "add-again", ExpectedRevision: added.Revision, AddItemIDs: []string{item.Public}}, noop)
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.SavedResourceContent(Viewer{Libraries: []string{"library"}}, "server", "fence", receipt.ResourceID, a, "", 40)
	if e != nil || len(second.Entries) != 1 || second.Entries[0].ID != first.Entries[0].ID {
		t.Fatal(second, e)
	}
	other := ResourceActor{Authority: "local", AccountID: "other", ProfileID: "other"}
	if _, e = s.MutateSavedResource(other, receipt.ResourceID, "delete", SavedResourceMutation{OperationID: "denied", ExpectedRevision: added.Revision}, noop); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("unshared actor admitted", e)
	}
	if _, e = s.MutateSavedResource(a, receipt.ResourceID, "delete", SavedResourceMutation{OperationID: "delete", ExpectedRevision: added.Revision}, noop); e != nil {
		t.Fatal(e)
	}
	replay, e := s.MutateSavedResource(a, "", "create", create, noop)
	if e != nil || replay.ResourceID != receipt.ResourceID || replay.Deleted {
		t.Fatal("original receipt changed", replay, e)
	}
	deleted, _, e := s.SavedResourceReceiptStatus(receipt.ResourceID, a)
	if e != nil || !deleted {
		t.Fatal("replay resurrected resource", e)
	}
}
