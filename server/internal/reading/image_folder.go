package reading

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/mediasource"
	"portico.local/server/internal/readingfolder"
	"portico.local/server/internal/sourceaccess"
	"portico.local/server/internal/storage"
)

// imageFolderEligibleSQL never promotes a partial/stale directory into a book.
// Every captured direct member must still name the same accepted source object.
const imageFolderEligibleSQL = `(p.format<>'image-folder' OR EXISTS(
 SELECT 1 FROM reading_image_folders folder WHERE folder.asset_id=p.asset_id
 AND s.kind='local' AND folder.source_id=s.id AND folder.root_incarnation=s.incarnation
 AND folder.root_identity=s.root_identity AND folder.member_revision<>''
 AND folder.member_count BETWEEN 1 AND 4097
 AND folder.total_size=(SELECT sum(size) FROM reading_image_members members WHERE members.folder_asset_id=folder.asset_id)
 AND (SELECT count(*) FROM reading_image_members members WHERE members.folder_asset_id=folder.asset_id)=folder.member_count
 AND NOT EXISTS(SELECT 1 FROM reading_image_members member LEFT JOIN inventory_objects actual ON actual.id=member.inventory_object_id
 WHERE member.folder_asset_id=folder.asset_id AND (actual.id IS NULL OR actual.source_id<>s.id OR actual.root_incarnation<>s.incarnation
 OR actual.retired<>0 OR actual.state<>'available' OR actual.unsupported_reason<>'' OR actual.seen_job<>folder.job_id
 OR actual.relative_path<>member.relative_path OR actual.object_identity<>member.object_identity OR actual.revision<>member.revision
 OR actual.evidence_json<>member.evidence_json OR actual.size<>member.size OR actual.modified_ns<>member.modified_ns))))`

func (f *Source) checkImageDirectory(ctx context.Context) error {
	if f.Format != "image-folder" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := f.validateCurrent(ctx); err != nil {
		return err
	}
	var directory, identity, revision string
	err := f.service.DB.QueryRowContext(ctx, `SELECT directory_path,directory_identity,directory_revision FROM reading_image_folders WHERE asset_id=? AND source_id=? AND root_incarnation=? AND member_revision=?`, f.Asset, f.rootID, f.incarnation, f.folderRevision).Scan(&directory, &identity, &revision)
	if err == sql.ErrNoRows {
		return ErrChanged
	}
	if err != nil {
		return err
	}
	if directory != filepath.Dir(f.relative) {
		return ErrChanged
	}
	// Re-enumerate the ctx complete direct inventory, not only the current
	// page or directory mtime: modifying a different page also retires handles.
	rows, err := f.service.DB.QueryContext(ctx, `SELECT relative_path,object_identity,evidence_json,size,modified_ns FROM reading_image_members WHERE folder_asset_id=? ORDER BY position LIMIT 4098`, f.Asset)
	if err != nil {
		return err
	}
	expected := make(map[string]readingfolder.Member)
	for rows.Next() {
		var m readingfolder.Member
		if err = rows.Scan(&m.RelativePath, &m.ObjectIdentity, &m.Evidence, &m.Size, &m.ModifiedNS); err != nil {
			rows.Close()
			return err
		}
		if len(expected) >= readingfolder.MaxPages+1 {
			rows.Close()
			return ErrLimit
		}
		expected[m.RelativePath] = m
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	cursor := ""
	total, found := 0, 0
	seen := make(map[string]bool)
	for {
		if err = f.validateCurrent(ctx); err != nil {
			return err
		}
		page, err := f.service.Storage.InventoryPage(ctx, "reading-folder:"+f.rootID, storage.InventoryRequest{Root: f.root, RootIdentity: f.rootIdentity, RelativePath: directory, DirectoryRevision: revision, Cursor: cursor, Limit: 128})
		if err != nil {
			return err
		}
		if !storage.SameRoot(page.RootIdentity, f.rootIdentity) || page.DirectoryIdentity != identity || page.DirectoryRevision != revision || page.Unstable || len(page.Issues) != 0 || page.Pending || len(page.Entries) > 128 {
			return ErrChanged
		}
		total += len(page.Entries)
		if total > readingfolder.MaxPages+1 {
			return ErrLimit
		}
		for _, entry := range page.Entries {
			relative, err := filepath.Rel(f.root, entry.Path)
			if err != nil || !filepath.IsLocal(relative) || filepath.Dir(relative) != directory {
				return ErrChanged
			}
			if entry.Directory || !readingfolder.IsMember(relative) {
				continue
			}
			m, ok := expected[relative]
			if !ok || seen[relative] || entry.Size != m.Size || entry.ModifiedNS != m.ModifiedNS || storage.StableObjectIdentity(entry.ObjectIdentity, f.rootIdentity) != m.ObjectIdentity {
				return ErrChanged
			}
			var evidence mediasource.InventoryEvidence
			if json.Unmarshal([]byte(m.Evidence), &evidence) != nil {
				return ErrInvalid
			}
			if evidence.Object != m.ObjectIdentity || evidence.ChangeToken != "" && evidence.ChangeToken != entry.ChangeToken {
				return ErrChanged
			}
			seen[relative] = true
			found++
		}
		if page.Complete {
			if page.NextCursor != "" || found != len(expected) {
				return ErrChanged
			}
			break
		}
		if page.NextCursor == "" || page.NextCursor == cursor || len(page.Entries) == 0 {
			return ErrChanged
		}
		cursor = page.NextCursor
	}

	return f.validateCurrent(ctx)
}

func (f *Source) imageFolder() (*Comic, error) {
	if f.Format != "image-folder" || f.Sequential() || f.service.Storage.IsRemote(f.path) {
		return nil, ErrInvalid
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	var v readingfolder.Inventory
	v.SourceID, v.Incarnation, v.RootIdentity = f.rootID, f.incarnation, f.rootIdentity
	err := f.service.DB.QueryRowContext(f.ctx, `SELECT directory_path,directory_identity,directory_revision,job_id FROM reading_image_folders WHERE asset_id=? AND source_id=? AND root_incarnation=? AND member_revision=?`, f.Asset, f.rootID, f.incarnation, f.folderRevision).Scan(&v.Directory, &v.DirectoryIdentity, &v.DirectoryRevision, &v.Job)
	if err != nil {
		return nil, ErrChanged
	}
	rows, err := f.service.DB.QueryContext(f.ctx, `SELECT member.inventory_object_id,member.relative_path,member.object_identity,member.revision,member.evidence_json,member.size,member.modified_ns,a.id FROM reading_image_members member JOIN inventory_objects o ON o.id=member.inventory_object_id JOIN catalog_assets a ON a.token=o.asset_id WHERE member.folder_asset_id=? ORDER BY member.position LIMIT 4098`, f.Asset)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var m readingfolder.Member
		if err = rows.Scan(&m.ObjectID, &m.RelativePath, &m.ObjectIdentity, &m.Revision, &m.Evidence, &m.Size, &m.ModifiedNS, &m.Asset); err != nil {
			rows.Close()
			return nil, err
		}
		v.Members = append(v.Members, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	accepted, revision, err := readingfolder.Seal(v)
	if err != nil || revision != f.folderRevision || accepted.Members[0].Asset != f.Asset || accepted.Members[0].RelativePath != f.relative {
		return nil, ErrChanged
	}
	if err = f.Validate(); err != nil {
		return nil, err
	}
	c := &Comic{}
	private := make(map[string]readingfolder.Member, len(accepted.Members))
	for _, m := range accepted.Members {
		if readingfolder.IsMetadata(m.RelativePath) {
			b, err := f.readImageMember(f.ctx, m)
			if err != nil {
				return nil, err
			}
			metadata, err := comicMetadata(b)
			if err != nil {
				return nil, err
			}
			c.Metadata = metadata
			continue
		}
		// Opaque page handles disclose neither directory names nor filesystem paths.
		name := fmt.Sprintf("page-%06d", len(c.Names)+1)
		c.Names = append(c.Names, name)
		private[name] = m
	}
	c.pageReader = func(ctx context.Context, name string, limit int64) ([]byte, error) {
		member, ok := private[name]
		if !ok || limit <= 0 || member.Size > limit || member.Size > MaxMember {
			return nil, ErrInvalid
		}
		return f.readImageMember(ctx, member)
	}
	return c, nil
}

func (f *Source) readImageMember(parent context.Context, m readingfolder.Member) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	stop := context.AfterFunc(f.ctx, cancel)
	defer func() { stop(); cancel() }()
	if err := f.checkImageDirectory(ctx); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.Size <= 0 || m.Size > MaxMember || !filepath.IsLocal(m.RelativePath) || filepath.Dir(m.RelativePath) != filepath.Dir(f.relative) {
		return nil, ErrInvalid
	}
	lease, err := f.roots.Borrow(ctx, sourceaccess.Authority{OwnerID: "read-folder:" + identity.Token(), RootID: f.rootID, RootPath: f.root, RelativePath: m.RelativePath, Revision: f.incarnation + ":" + strconv.FormatInt(f.generation, 10), Lifetime: ctx, Validate: func(ctx context.Context) error { return f.validateCurrent(ctx) }})
	if err != nil {
		return nil, err
	}
	reader, err := f.service.Storage.OpenObservedPlayback(ctx, lease)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	check := func() error {
		if err := f.validateCurrent(ctx); err != nil {
			return err
		}
		observed, err := reader.Observe(ctx)
		if err != nil {
			return err
		}
		_, root, ok := strings.Cut(observed.RootBindingID, ":")
		if !ok || !storage.SameRoot(f.rootIdentity, root) || !observed.SizeKnown || !observed.ModifiedKnown || observed.Size != m.Size || observed.ModifiedNS != m.ModifiedNS {
			return ErrChanged
		}
		var evidence mediasource.InventoryEvidence
		if json.Unmarshal([]byte(m.Evidence), &evidence) != nil {
			return ErrInvalid
		}
		actual := storage.StableObjectIdentity(observed.ObjectBindingID, root)
		if actual != m.ObjectIdentity || evidence.Object != actual || evidence.ChangeToken != "" && evidence.ChangeToken != observed.ChangeToken {
			return ErrChanged
		}
		return ctx.Err()
	}
	if err = check(); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]byte, 0, int(m.Size))
	for int64(len(out)) < m.Size {
		if err = check(); err != nil {
			return nil, err
		}
		length := min(int64(storage.ObservedReadLimit), m.Size-int64(len(out)))
		part, err := reader.ReadExtent(ctx, int64(len(out)), int(length))
		if err != nil {
			return nil, err
		}
		if len(part) != int(length) {
			return nil, ErrChanged
		}
		out = append(out, part...)
	}
	if err = check(); err != nil {
		return nil, err
	}
	if err = f.checkImageDirectory(ctx); err != nil {
		return nil, err
	}
	return out, nil
}
