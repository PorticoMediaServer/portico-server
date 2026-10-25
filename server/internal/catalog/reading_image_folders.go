package catalog

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/readingfolder"
	"portico.local/server/internal/storage"
)

// refreshImageFolderTx is called after the scan commits this directory page.
// An in-progress/failed observation withholds the old binding without deleting
// its accepted publication or personal state. Only complete accepted inventory
// may reconcile a current binding.
func refreshImageFolderTx(ctx context.Context, tx *sql.Tx, job string, source LibrarySource, directory string) error {
	if source.Kind != "local" {
		return nil
	}
	var kind string
	if err := tx.QueryRowContext(ctx, `SELECT kind FROM libraries WHERE id=?`, source.LibraryID).Scan(&kind); err != nil {
		return err
	}
	if kind != "comic" {
		return nil
	}
	current, err := InventoryFenceTx(ctx, tx, job)
	if err != nil {
		return err
	}
	if current.ID != source.ID || current.Generation != source.Generation || current.Incarnation != source.Incarnation || current.RootIdentity != source.RootIdentity {
		return context.Canceled
	}
	if err = retireImageFolderTx(ctx, tx, source.ID, directory); err != nil {
		return err
	}
	var state, directoryID, directoryRevision string
	var skipped, entries int
	if err = tx.QueryRowContext(ctx, `SELECT state,identity,revision,skipped,entries FROM inventory_directories WHERE job_id=? AND relative_path=?`, job, directory).Scan(&state, &directoryID, &directoryRevision, &skipped, &entries); err != nil {
		return err
	}
	if state != "done" || skipped != 0 || entries > readingfolder.MaxPages+1 {
		return nil
	}
	prefix := directory + string(filepath.Separator)
	if directory == "." {
		prefix = ""
	}
	var unsafe bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_scan_issues WHERE job_id=? AND (relative_path=? OR (instr(relative_path,?)=1 AND instr(substr(relative_path,length(?)+1),?)=0)))`, job, directory, prefix, prefix, string(filepath.Separator)).Scan(&unsafe); err != nil {
		return err
	}
	if unsafe {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT o.id,o.relative_path,o.object_identity,o.revision,o.evidence_json,o.size,o.modified_ns,a.id FROM inventory_objects o JOIN catalog_assets a ON a.token=o.asset_id WHERE o.source_id=? AND o.root_incarnation=? AND o.directory_path=? AND o.seen_job=? AND o.retired=0 AND o.state='available' AND o.unsupported_reason='' AND (lower(substr(o.relative_path,-4)) IN('.jpg','.png') OR lower(substr(o.relative_path,-5)) IN('.jpeg','.webp') OR o.relative_path=?) ORDER BY o.relative_path LIMIT 4098`, source.ID, source.Incarnation, directory, job, filepath.Join(directory, "ComicInfo.xml"))
	if err != nil {
		return err
	}
	v := readingfolder.Inventory{SourceID: source.ID, Generation: source.Generation, Incarnation: source.Incarnation, RootIdentity: source.RootIdentity, Directory: directory, DirectoryIdentity: directoryID, DirectoryRevision: directoryRevision, Job: job}
	for rows.Next() {
		var m readingfolder.Member
		if err = rows.Scan(&m.ObjectID, &m.RelativePath, &m.ObjectIdentity, &m.Revision, &m.Evidence, &m.Size, &m.ModifiedNS, &m.Asset); err != nil {
			rows.Close()
			return err
		}
		if readingfolder.IsMember(m.RelativePath) {
			v.Members = append(v.Members, m)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(v.Members) == 0 {
		return nil
	}
	accepted, revision, err := readingfolder.Seal(v)
	if err != nil {
		return InventoryIssueTx(ctx, tx, job, storage.InventoryIssue{Path: directory, Reason: "image_folder_incomplete", Protect: true})
	}
	library, err := compactcatalog.LibraryTx(ctx, tx, source.LibraryID)
	if err != nil {
		return err
	}
	title := filepath.Base(directory)
	if directory == "." {
		title = filepath.Base(source.ResolvedPath)
	}
	if strings.TrimSpace(title) == "" {
		return nil
	}
	item, _, err := compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Comic, Key: "reading:image-folder:" + compactcatalog.ItemKey(source.ID, storage.StableObjectIdentity(directoryID, source.RootIdentity), 0), Title: title, Added: catalogedNow()})
	if err != nil {
		return err
	}
	var totalSize int64
	for _, member := range accepted.Members {
		totalSize += member.Size
	}
	anchor := accepted.Members[0].Asset
	if err = compactcatalog.LinkAssetTx(ctx, tx, item, anchor, compactcatalog.Link{}); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO reading_publications(item_id,asset_id,format) VALUES(?,?,'image-folder') ON CONFLICT(item_id,asset_id) DO NOTHING`, item, anchor); err != nil {
		return err
	}
	var previousRevision string
	err = tx.QueryRowContext(ctx, `SELECT member_revision FROM reading_image_folders WHERE asset_id=?`, anchor).Scan(&previousRevision)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO reading_image_folders(asset_id,source_id,generation,root_incarnation,root_identity,directory_path,directory_identity,directory_object,directory_revision,job_id,member_revision,member_count,total_size,available) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,0) ON CONFLICT(asset_id) DO UPDATE SET generation=excluded.generation,directory_path=excluded.directory_path,directory_object=excluded.directory_object,root_incarnation=excluded.root_incarnation,root_identity=excluded.root_identity,directory_identity=excluded.directory_identity,directory_revision=excluded.directory_revision,job_id=excluded.job_id,member_revision=excluded.member_revision,member_count=excluded.member_count,total_size=excluded.total_size,available=0 WHERE reading_image_folders.source_id=excluded.source_id AND reading_image_folders.directory_object=excluded.directory_object`, anchor, source.ID, source.Generation, source.Incarnation, source.RootIdentity, directory, directoryID, storage.StableObjectIdentity(directoryID, source.RootIdentity), directoryRevision, job, revision, len(accepted.Members), totalSize)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrAdminQuery
	}
	if previousRevision != revision {

		if _, err = tx.ExecContext(ctx, `DELETE FROM reading_image_members WHERE folder_asset_id=?`, anchor); err != nil {
			return err
		}
		for i, m := range accepted.Members {
			if _, err = tx.ExecContext(ctx, `INSERT INTO reading_image_members(folder_asset_id,position,inventory_object_id,relative_path,object_identity,revision,evidence_json,size,modified_ns) VALUES(?,?,?,?,?,?,?,?,?)`, anchor, i, m.ObjectID, m.RelativePath, m.ObjectIdentity, m.Revision, m.Evidence, m.Size, m.ModifiedNS); err != nil {
				return err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE reading_image_folders SET available=1 WHERE asset_id=? AND source_id=? AND directory_path=?`, anchor, source.ID, directory); err != nil {
		return err
	}
	if err = touchImageFolderTx(ctx, tx, item); err != nil {
		return err
	}
	return restoreRetainedTx(ctx, tx, item)
}

// A member moved after a completed observation invalidates the old directory;
// it cannot be republished from that now-stale completion.
func retireImageFolderTx(ctx context.Context, tx *sql.Tx, sourceID, directory string) error {
	rows, err := tx.QueryContext(ctx, `SELECT p.item_id,p.asset_id FROM reading_image_folders f JOIN reading_publications p ON p.asset_id=f.asset_id WHERE f.source_id=? AND f.directory_path=? AND f.available=1 LIMIT 2`, sourceID, directory)
	if err != nil {
		return err
	}
	type link struct{ item, asset int64 }
	var old []link
	for rows.Next() {
		var v link
		if err = rows.Scan(&v.item, &v.asset); err != nil {
			rows.Close()
			return err
		}
		old = append(old, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(old) > 1 {
		return ErrAdminQuery
	}
	if _, err = tx.ExecContext(ctx, `UPDATE reading_image_folders SET available=0 WHERE source_id=? AND directory_path=? AND available=1`, sourceID, directory); err != nil {
		return err
	}
	for _, v := range old {
		if err = touchImageFolderTx(ctx, tx, v.item); err != nil {
			return err
		}
	}
	return nil
}

// Availability is a catalogue change, never a deletion of an accepted version.
func touchImageFolderTx(ctx context.Context, tx *sql.Tx, item int64) error {
	for _, domain := range []int{compactcatalog.DomainAvailability, compactcatalog.DomainItemMetrics, compactcatalog.DomainBrowseRows} {
		if err := compactcatalog.TouchTx(ctx, tx, domain, item); err != nil {
			return err
		}
	}
	return nil
}
