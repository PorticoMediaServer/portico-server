package administration

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestDeleteVersionKeepsTitleAndOtherFilesAndBindsPreview(t *testing.T) {
	s, db, _, item, paths := deletionFixture(t)
	ctx := context.Background()
	var asset string
	if err := db.QueryRow(`SELECT token FROM catalog_assets WHERE path=?`, paths[0]).Scan(&asset); err != nil {
		t.Fatal(err)
	}
	p, err := s.PreviewDeleteVersion(ctx, allow, item, asset)
	if err != nil {
		t.Fatal(err)
	}
	if p.TotalFiles != 1 || p.Targets[0].Files[0].Path != paths[0] {
		t.Fatal(p)
	}
	in := VersionDeleteRequest{Confirmation: p.Confirmation, PreviewKey: p.PreviewKey, OperationID: "delete-version"}
	bad := in
	bad.PreviewKey = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if _, err = s.DeleteVersion(ctx, allow, item, asset, bad); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	r, err := s.DeleteVersion(ctx, allow, item, asset, in)
	if err != nil || r.Removed != 1 || r.Receipts[0].FilesDeleted != 1 {
		t.Fatal(r, err)
	}
	if _, err = os.Stat(paths[0]); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err = os.Stat(paths[1]); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM catalog_entities WHERE public_id=pid_blob(?)`, item).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	replay, err := s.DeleteVersion(ctx, allow, item, asset, in)
	if err != nil || replay.Removed != 1 {
		t.Fatal(replay, err)
	}
	if err = db.QueryRow(`SELECT token FROM catalog_assets WHERE path=?`, paths[1]).Scan(&asset); err != nil {
		t.Fatal(err)
	}
	p, err = s.PreviewDeleteVersion(ctx, allow, item, asset)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DeleteVersion(ctx, allow, item, asset, VersionDeleteRequest{Confirmation: p.Confirmation, PreviewKey: p.PreviewKey, OperationID: "delete-last-version"})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM catalog_missing_titles m JOIN catalog_entities e ON e.id=m.entity_id WHERE e.public_id=pid_blob(?)`, item).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}
