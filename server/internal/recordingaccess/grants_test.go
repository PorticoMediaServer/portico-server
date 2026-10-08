package recordingaccess

import (
	"context"
	"path/filepath"
	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/persistence"
	"testing"
)

func TestRecordingGrantExplicitInheritanceRevocationAndMembership(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err = db.Exec(`INSERT INTO accounts VALUES('owner','owner',x'00','primary',1); INSERT INTO direct_profiles(id,account_id,name,is_primary) VALUES('child','owner','Child',0); INSERT INTO live_source_identities VALUES('source');`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	owner := livechannels.Owner{Authority: "local", AccountID: "owner", ProfileID: "primary"}
	child := owner
	child.ProfileID = "child"
	p := Policy{}
	if err = p.Durable(ctx, tx, owner, "source", ""); err == nil {
		t.Fatal("owner received implicit permission")
	}
	grant, err := SaveGrantTx(ctx, tx, Grant{Owner: owner, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Durable(ctx, tx, owner, "source", ""); err != nil {
		t.Fatal(err)
	}
	if err = GrantedTx(ctx, tx, child); err == nil {
		t.Fatal("child inherited without explicit choice")
	}
	grant.InheritProfiles = true
	grant, err = SaveGrantTx(ctx, tx, grant)
	if err != nil {
		t.Fatal(err)
	}
	if err = GrantedTx(ctx, tx, child); err != nil {
		t.Fatal(err)
	}
	childGrant, err := SaveGrantTx(ctx, tx, Grant{Owner: child, Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = GrantedTx(ctx, tx, child); err == nil {
		t.Fatal("explicit profile denial lost")
	}
	childGrant.Enabled = true
	if _, err = SaveGrantTx(ctx, tx, childGrant); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE direct_profiles SET deleted=1 WHERE id='child'`); err != nil {
		t.Fatal(err)
	}
	if _, err = p.MemberTx(ctx, tx, child); err == nil {
		t.Fatal("grant restored a retired identity")
	}
	grant.Enabled = false
	if _, err = SaveGrantTx(ctx, tx, grant); err != nil {
		t.Fatal(err)
	}
	if err = p.Durable(ctx, tx, owner, "source", ""); err == nil {
		t.Fatal("revoked owner still records")
	}
}
