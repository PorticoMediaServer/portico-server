package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// eraseSecondaryProfileTx permanently removes one non-primary viewing
// profile. Profile-owned viewer state is deleted, durable account assets such
// as DVR recordings are reassigned to the primary profile, and every
// account-wide profile-selection proof is invalidated because the directory
// revision changed.
func (s *Server) eraseSecondaryProfileTx(ctx context.Context, tx *sql.Tx, accountID, profileID, requiredOrigin, now string) (string, error) {
	if tx == nil {
		return "", errors.New("profile erasure transaction is required")
	}
	accountID = strings.TrimSpace(accountID)
	profileID = strings.TrimSpace(profileID)
	if accountID == "" || profileID == "" {
		return "", errProfileNotFound
	}
	targetDigest := profileErasureTargetDigest(accountID, profileID)
	var existingOperationID string
	if err := tx.QueryRowContext(ctx, `SELECT operation_id FROM profile_erasure_receipts WHERE target_digest = ?`, targetDigest).Scan(&existingOperationID); err == nil {
		return existingOperationID, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if err := verifyProfileErasureInventoryTx(ctx, tx); err != nil {
		return "", err
	}
	var primary int
	var origin string
	if err := tx.QueryRowContext(ctx, `
		SELECT is_primary, origin
		FROM profiles
		WHERE id = ? AND account_id = ?`, profileID, accountID).Scan(&primary, &origin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errProfileNotFound
		}
		return "", err
	}
	if primary == 1 {
		return "", errors.New("the primary profile cannot be deleted")
	}
	if requiredOrigin != "" && origin != requiredOrigin {
		return "", errHostedProfileLocalPIN
	}
	var primaryProfileID string
	if err := tx.QueryRowContext(ctx, `
		SELECT id FROM profiles
		WHERE account_id = ? AND is_primary = 1 AND disabled_at = ''`, accountID).Scan(&primaryProfileID); err != nil {
		return "", fmt.Errorf("load primary profile for erasure: %w", err)
	}

	// Remove privacy-sensitive history projections before deleting their source
	// sessions; the rollup table intentionally has no profile foreign key.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM dashboard_playback_rollups
		WHERE session_id IN (
			SELECT id FROM playback_sessions WHERE user_id = ? AND profile_id = ?
		)`, accountID, profileID); err != nil {
		return "", fmt.Errorf("erase profile playback rollups: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM viewer_notifications
		WHERE (account_id = ? AND profile_id = ?)
		   OR source_feedback_id IN (
			SELECT id FROM viewer_feedback WHERE account_id = ? AND profile_id = ?
		   )`, accountID, profileID, accountID, profileID); err != nil {
		return "", fmt.Errorf("erase profile notifications: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM viewer_notification_revisions WHERE account_id = ? AND profile_id = ?`, accountID, profileID); err != nil {
		return "", fmt.Errorf("erase profile notification revision: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM viewer_preference_document_quarantine WHERE account_id = ? AND profile_id = ?`, accountID, profileID); err != nil {
		return "", fmt.Errorf("erase quarantined profile preferences: %w", err)
	}
	// Installation preference documents are account-scoped, but their
	// lastProfileId may still point at the erased viewer. Move that reference to
	// the primary profile and advance its CAS revision in the same transaction.
	if _, err := tx.ExecContext(ctx, `
		UPDATE viewer_preference_documents
		SET values_json = json_set(values_json, '$.lastProfileId', ?),
		    revision = revision + 1,
		    updated_at = ?
		WHERE account_id = ? AND scope_type = 'account-server-installation'
		  AND json_valid(values_json)
		  AND json_extract(values_json, '$.lastProfileId') = ?`, primaryProfileID, now, accountID, profileID); err != nil {
		return "", fmt.Errorf("repair account installation profile preference: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM live_tv_tuner_allocations
		WHERE allocation_kind = 'live_session' AND consumer_id IN (
			SELECT id FROM playback_sessions WHERE user_id = ? AND profile_id = ?
		)`, accountID, profileID); err != nil {
		return "", fmt.Errorf("release profile Live TV tuner allocations: %w", err)
	}

	for _, statement := range []string{
		`DELETE FROM sessions WHERE user_id = ? AND profile_id = ?`,
		`DELETE FROM native_refresh_tokens WHERE user_id = ? AND profile_id = ?`,
		`DELETE FROM playlists WHERE user_id = ? AND profile_id = ?`,
		`DELETE FROM saved_views WHERE user_id = ? AND profile_id = ?`,
		`DELETE FROM playback_media_grants WHERE principal_user_id = ? AND profile_id = ?`,
		`DELETE FROM media_download_grants WHERE principal_user_id = ? AND profile_id = ?`,
		`DELETE FROM playback_sessions WHERE user_id = ? AND profile_id = ?`,
		`DELETE FROM playback_receivers WHERE user_id = ? AND profile_id = ?`,
		`DELETE FROM viewer_preference_documents WHERE account_id = ? AND profile_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, statement, accountID, profileID); err != nil {
			return "", fmt.Errorf("erase profile-owned state: %w", err)
		}
	}

	// Recordings are durable household assets. Deleting a viewing profile must
	// not unexpectedly discard recordings or partial files; ownership moves to
	// the account's primary profile.
	if _, err := tx.ExecContext(ctx, `UPDATE live_tv_recording_rules SET profile_id = ?, updated_at = ? WHERE user_id = ? AND profile_id = ?`, primaryProfileID, now, accountID, profileID); err != nil {
		return "", fmt.Errorf("reassign profile recording rules: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE live_tv_recordings SET profile_id = ?, updated_at = ? WHERE user_id = ? AND profile_id = ?`, primaryProfileID, now, accountID, profileID); err != nil {
		return "", fmt.Errorf("reassign profile recordings: %w", err)
	}

	// Directory mutations invalidate proofs for the whole account, not just the
	// removed profile, so a stale proof cannot select against an old directory.
	if _, err := tx.ExecContext(ctx, `DELETE FROM profile_selection_grants WHERE account_id = ?`, accountID); err != nil {
		return "", fmt.Errorf("revoke profile selection grants: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM profile_account_authentications WHERE account_id = ?`, accountID); err != nil {
		return "", fmt.Errorf("revoke profile account authentications: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM local_profile_admin_proofs WHERE account_id = ?`, accountID); err != nil {
		return "", fmt.Errorf("revoke profile administration proofs: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE automatic_profile_selection_trusts SET revoked_at = ?, updated_at = ? WHERE account_id = ? AND revoked_at = ''`, now, now, accountID); err != nil {
		return "", fmt.Errorf("revoke automatic profile trusts: %w", err)
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM profiles WHERE id = ? AND account_id = ? AND is_primary = 0`, profileID, accountID)
	if err != nil {
		return "", fmt.Errorf("delete profile: %w", err)
	}
	if rows, err := result.RowsAffected(); err == nil && rows != 1 {
		return "", errProfileNotFound
	}
	operationID := randomID("erase")
	if _, err := tx.ExecContext(ctx, `INSERT INTO profile_erasure_receipts (operation_id, target_digest, account_id, erased_at) VALUES (?, ?, ?, ?)`, operationID, targetDigest, accountID, now); err != nil {
		return "", fmt.Errorf("record profile erasure receipt: %w", err)
	}
	return operationID, nil
}

func profileErasureTargetDigest(accountID, profileID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(accountID) + "\x00" + strings.TrimSpace(profileID)))
	return hex.EncodeToString(digest[:])
}

// verifyProfileErasureInventoryTx makes schema drift fail closed. Any new
// relational profile column must either cascade from profiles(id) or receive
// an explicit disposition in eraseSecondaryProfileTx.
func verifyProfileErasureInventoryTx(ctx context.Context, tx *sql.Tx) error {
	explicit := map[string]bool{
		"sessions.profile_id":                              true,
		"native_refresh_tokens.profile_id":                 true,
		"playlists.profile_id":                             true,
		"saved_views.profile_id":                           true,
		"playback_media_grants.profile_id":                 true,
		"media_download_grants.profile_id":                 true,
		"playback_sessions.profile_id":                     true,
		"playback_receivers.profile_id":                    true,
		"viewer_preference_documents.profile_id":           true,
		"viewer_preference_document_quarantine.profile_id": true,
		"viewer_notifications.profile_id":                  true,
		"viewer_notification_receipts.profile_id":          true,
		"viewer_notification_revisions.profile_id":         true,
		"live_tv_recording_rules.profile_id":               true,
		"live_tv_recordings.profile_id":                    true,
	}
	tables, err := tx.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return fmt.Errorf("inspect profile erasure schema: %w", err)
	}
	var names []string
	for tables.Next() {
		var name string
		if err := tables.Scan(&name); err != nil {
			tables.Close()
			return fmt.Errorf("read profile erasure schema: %w", err)
		}
		names = append(names, name)
	}
	if err := tables.Close(); err != nil {
		return fmt.Errorf("close profile erasure schema inventory: %w", err)
	}
	for _, table := range names {
		columns, err := tx.QueryContext(ctx, `PRAGMA table_info(`+quoteProfileErasureIdentifier(table)+`)`)
		if err != nil {
			return fmt.Errorf("inspect profile erasure columns for %s: %w", table, err)
		}
		var relationalColumns []string
		for columns.Next() {
			var sequence, notNull, primaryKey int
			var name, kind string
			var defaultValue any
			if err := columns.Scan(&sequence, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
				columns.Close()
				return fmt.Errorf("read profile erasure column for %s: %w", table, err)
			}
			if (name == "profile_id" || strings.HasSuffix(name, "_profile_id")) && !(table == "profiles" && name == "external_profile_id") {
				relationalColumns = append(relationalColumns, name)
			}
		}
		if err := columns.Close(); err != nil {
			return fmt.Errorf("close profile erasure columns for %s: %w", table, err)
		}
		if len(relationalColumns) == 0 {
			continue
		}
		cascade := map[string]bool{}
		foreignKeys, err := tx.QueryContext(ctx, `PRAGMA foreign_key_list(`+quoteProfileErasureIdentifier(table)+`)`)
		if err != nil {
			return fmt.Errorf("inspect profile erasure foreign keys for %s: %w", table, err)
		}
		for foreignKeys.Next() {
			var id, sequence int
			var parent, from, to, onUpdate, onDelete, match string
			if err := foreignKeys.Scan(&id, &sequence, &parent, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				foreignKeys.Close()
				return fmt.Errorf("read profile erasure foreign key for %s: %w", table, err)
			}
			if parent == "profiles" && to == "id" && strings.EqualFold(onDelete, "CASCADE") {
				cascade[from] = true
			}
		}
		if err := foreignKeys.Close(); err != nil {
			return fmt.Errorf("close profile erasure foreign keys for %s: %w", table, err)
		}
		for _, column := range relationalColumns {
			if !cascade[column] && !explicit[table+"."+column] {
				return fmt.Errorf("profile erasure schema inventory is incomplete for %s.%s", table, column)
			}
		}
	}
	return nil
}

func quoteProfileErasureIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
