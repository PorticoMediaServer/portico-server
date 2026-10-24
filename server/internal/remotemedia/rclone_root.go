package remotemedia

import "context"

// CheckRoot is connection-test metadata only, never file/content authority.
// Upstream StatJSON(remote="",dirsOnly=true) calls the backend List before
// returning one directory item. A VFS root/readiness or fsinfo alone need not
// contact the backend. Hashes, MIME, modtime and object payloads are not read.
// rclone may buffer its underlying directory list; the response here is one
// bounded item, and the caller's exclusive-test context supplies the deadline.
func (m *RcloneMetadata) CheckRoot(ctx context.Context, check func(context.Context) error) error {
	ctx, release, err := m.endpoint.budget.enter(ctx)
	if err != nil {
		return err
	}
	defer release()
	var raw struct {
		Item *struct {
			Path  string
			IsDir bool
		}
	}
	if err = m.request(ctx, "operations/stat", map[string]any{"fs": m.fs, "remote": "", "opt": map[string]any{"dirsOnly": true, "showHash": false, "noModTime": true, "noMimeType": true}}, &raw, check); err != nil {
		return err
	}
	if raw.Item == nil || raw.Item.Path != "" || !raw.Item.IsDir {
		return ErrDAVResponse
	}
	return check(ctx)
}
