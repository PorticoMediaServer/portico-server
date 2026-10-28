package assets

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"

	"portico.local/server/internal/storage"
)

type playbackQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type playbackEvidence struct {
	binding        storage.PlaybackBinding
	source, object string
}

func validPlaybackBinding(binding storage.PlaybackBinding) bool {
	r := binding.Inventory
	return binding.ObjectIdentity != "" && r.RootIdentity != "" && filepath.IsAbs(r.Root) && filepath.IsLocal(r.RelativePath) && r.RelativePath != "."
}

// resolvePlaybackEvidence is shared by acceptance and session-less catalogue
// reads. Session byte reads use only persisted evidence, never this selector.
func resolvePlaybackEvidence(ctx context.Context, q playbackQuery, v PlaybackSource, asset, source, incarnation string, generation int64) (*playbackEvidence, error) {
	var evidence playbackEvidence
	binding := &evidence.binding
	var valid bool
	var kind string
	err := q.QueryRowContext(ctx, `SELECT s.id,o.id,s.root,s.root_identity,s.follow_symlinks,o.relative_path,o.object_identity,s.kind,
 o.size=? AND o.modified_ns=? AND o.retired=0 AND o.state='available' AND s.kind IN('local','remote') AND s.enabled=1 AND s.identity_confirmed=1 AND s.health NOT IN('offline','root_changed','removing') AND o.root_incarnation=s.incarnation AND (?='' OR (s.incarnation=? AND s.generation=?))
 FROM inventory_objects o JOIN library_sources s ON s.id=o.source_id
 WHERE o.asset_id=? AND (?='' OR s.id=?)
 ORDER BY (rtrim(s.root,?)||?||o.relative_path=?) DESC,(o.retired=0 AND o.state='available' AND o.root_incarnation=s.incarnation) DESC,s.id,o.id LIMIT 1`, v.Size, v.ModifiedNS, source, incarnation, generation, asset, source, source, string(filepath.Separator), string(filepath.Separator), filepath.Clean(v.Path)).Scan(&evidence.source, &evidence.object, &binding.Inventory.Root, &binding.Inventory.RootIdentity, &binding.Inventory.FollowSymlinks, &binding.Inventory.RelativePath, &binding.ObjectIdentity, &kind, &valid)
	if errors.Is(err, sql.ErrNoRows) && source == "" {
		return nil, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, storage.ErrPlaybackSource
	}
	if err != nil {
		return nil, err
	}
	if !valid || !validPlaybackBinding(*binding) || filepath.Join(binding.Inventory.Root, binding.Inventory.RelativePath) != filepath.Clean(v.Path) {
		return nil, storage.ErrPlaybackSource
	}
	// Provider IDs are not filesystem inodes. Remote bindings retain the managed
	// mount acquisition and size/mtime checks, without claiming object custody.
	binding.Mounted = kind == "remote"
	return &evidence, nil
}

// PinPlaybackBindingTx captures the inventory tuple in the session's acceptance
// transaction, constrained to the physical source selected by that transaction.
// Disc sessions carry their own custody and do not call this function.
func PinPlaybackBindingTx(ctx context.Context, tx *sql.Tx, session string) error {
	var v PlaybackSource
	var asset, source, incarnation string
	var generation int64
	err := tx.QueryRowContext(ctx, `SELECT a.path,pin.size,pin.modified_ns,pin.asset_id,COALESCE(physical.source_id,''),COALESCE(physical.incarnation,''),COALESCE(physical.configuration_generation,0)
 FROM playback_source_pins pin JOIN catalog_assets a ON a.token=pin.asset_id LEFT JOIN playback_physical_source_pins physical ON physical.session_id=pin.session_id WHERE pin.session_id=?`, session).Scan(&v.Path, &v.Size, &v.ModifiedNS, &asset, &source, &incarnation, &generation)
	if err != nil {
		return err
	}
	evidence, err := resolvePlaybackEvidence(ctx, tx, v, asset, source, incarnation, generation)
	if err != nil || evidence == nil {
		return err
	}
	if source == "" || source != evidence.source {
		return storage.ErrPlaybackSource
	}
	binding := evidence.binding
	mode := "inventory"
	if binding.Mounted {
		mode = "mounted"
	}
	_, err = tx.ExecContext(ctx, `UPDATE playback_source_pins SET binding=?,source_id=?,inventory_object_id=?,root=?,root_identity=?,relative_path=?,object_identity=?,follow_symlinks=? WHERE session_id=?`, mode, evidence.source, evidence.object, binding.Inventory.Root, binding.Inventory.RootIdentity, binding.Inventory.RelativePath, binding.ObjectIdentity, binding.Inventory.FollowSymlinks, session)
	return err
}
