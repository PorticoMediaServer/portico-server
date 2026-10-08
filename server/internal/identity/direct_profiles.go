package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"portico.local/server/internal/dbwork"
	"strings"
	"time"
)

type DirectProfile struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Primary          bool     `json:"primary"`
	Position         int      `json:"position"`
	Art              string   `json:"art"`
	Revision         int64    `json:"revision"`
	PINRevision      int64    `json:"pinRevision"`
	PINRequired      bool     `json:"pinRequired"`
	AllowedLibraries []string `json:"allowedLibraries"`
}
type ProfilePolicy struct {
	AllowedLibraries []string `json:"allowedLibraries"`
}
type DirectProfileEdit struct {
	ExpectedRevision int64          `json:"expectedRevision"`
	Name             *string        `json:"name,omitempty"`
	Art              *string        `json:"art,omitempty"`
	PIN              *string        `json:"pin,omitempty"`
	Policy           *ProfilePolicy `json:"policy,omitempty"`
}

func validProfileArt(v string) bool {
	switch v {
	case "blue", "violet", "mint", "coral", "gold", "slate", "rose", "sky":
		return true
	}
	return false
}
func validPIN(v string) bool {
	if len(v) != 4 {
		return false
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func (s *Service) directSnapshotTx(ctx context.Context, tx *sql.Tx, c directCaller) (DirectSnapshot, error) {
	if c.account.AllowedLibraries == nil {
		c.account.AllowedLibraries = []string{}
	}
	var hosted sql.NullString
	if e := tx.QueryRowContext(ctx, `SELECT hosted_account_id FROM account_portico_links WHERE account_id=?`, c.account.ID).Scan(&hosted); e != nil && !errors.Is(e, sql.ErrNoRows) {
		return DirectSnapshot{}, e
	}
	c.account.HostedAccountID = hosted.String
	out := DirectSnapshot{Authority: "local", ServerID: s.ID(), Account: c.account, Profiles: []DirectProfile{}, CanManage: c.manage}
	if hosted.Valid && c.account.Role == TierOwner {
		var custodian sql.NullString
		if e := tx.QueryRowContext(ctx, `SELECT custodian FROM hosted_membership_sync WHERE singleton=1`).Scan(&custodian); e != nil && !errors.Is(e, sql.ErrNoRows) {
			return out, e
		}
		out.CustodyPending = custodian.Valid && custodian.String != hosted.String
	}
	rows, e := tx.QueryContext(ctx, `SELECT id,name,is_primary,position,art,revision,pin_revision,pin_hash IS NOT NULL,allowed_libraries FROM direct_profiles WHERE account_id=? AND deleted=0 ORDER BY position,id`, c.account.ID)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var p DirectProfile
		var libraries sql.NullString
		if e = rows.Scan(&p.ID, &p.Name, &p.Primary, &p.Position, &p.Art, &p.Revision, &p.PINRevision, &p.PINRequired, &libraries); e != nil {
			return out, e
		}
		if libraries.Valid {
			if e = json.Unmarshal([]byte(libraries.String), &p.AllowedLibraries); e != nil {
				return out, e
			}
		}
		if p.AllowedLibraries == nil {
			p.AllowedLibraries = []string{}
		}
		out.Profiles = append(out.Profiles, p)
	}
	return out, rows.Err()
}
func (s *Service) CreateDirectProfile(ctx context.Context, bearer, name, art string) (DirectSnapshot, error) {
	name = strings.TrimSpace(name)
	if art == "" {
		art = "blue"
	}
	if !directName(name) || !validProfileArt(art) {
		return DirectSnapshot{}, ErrDirectInput
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return DirectSnapshot{}, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return DirectSnapshot{}, e
	}
	var count, position int
	if e = tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(max(position),-1)+1 FROM direct_profiles WHERE account_id=? AND deleted=0`, c.account.ID).Scan(&count, &position); e != nil {
		return DirectSnapshot{}, e
	}
	if count >= 8 {
		return DirectSnapshot{}, ErrProfileCapacity
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO direct_profiles(id,account_id,name,position,art) VALUES(?,?,?,?,?)`, Token(), c.account.ID, name, position, art); e != nil {
		return DirectSnapshot{}, e
	}
	out, e := s.directSnapshotTx(ctx, tx, c)
	if e != nil {
		return out, e
	}
	return out, gated.Commit()
}
func (s *Service) EditDirectProfile(ctx context.Context, bearer, id string, q DirectProfileEdit) (DirectSnapshot, error) {
	if !validFamilyID(id) || q.ExpectedRevision < 1 || q.Name == nil && q.Art == nil && q.PIN == nil && q.Policy == nil {
		return DirectSnapshot{}, ErrDirectInput
	}
	if q.Name != nil {
		v := strings.TrimSpace(*q.Name)
		q.Name = &v
		if !directName(v) {
			return DirectSnapshot{}, ErrDirectInput
		}
	}
	if q.Art != nil && !validProfileArt(*q.Art) || q.PIN != nil && *q.PIN != "" && !validPIN(*q.PIN) || q.Policy != nil && !directLibraries(q.Policy.AllowedLibraries) {
		return DirectSnapshot{}, ErrDirectInput
	}
	var hash []byte
	var e error
	if q.PIN != nil && *q.PIN != "" {
		select {
		case s.hashSlots <- struct{}{}:
			defer func() { <-s.hashSlots }()
		default:
			return DirectSnapshot{}, ErrBusy
		}
		hash, e = bcrypt.GenerateFromPassword([]byte(*q.PIN), PINCost)
		if e != nil {
			return DirectSnapshot{}, e
		}
	}
	gated2, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return DirectSnapshot{}, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return DirectSnapshot{}, e
	}
	var revision int64
	if e = tx.QueryRowContext(ctx, `SELECT revision FROM direct_profiles WHERE account_id=? AND id=? AND deleted=0`, c.account.ID, id).Scan(&revision); e != nil {
		return DirectSnapshot{}, e
	}
	if revision != q.ExpectedRevision {
		return DirectSnapshot{}, ErrProfileChanged
	}
	if q.Name != nil {
		if _, e = tx.ExecContext(ctx, `UPDATE direct_profiles SET name=? WHERE id=?`, *q.Name, id); e != nil {
			return DirectSnapshot{}, e
		}
	}
	if q.Art != nil {
		if _, e = tx.ExecContext(ctx, `UPDATE direct_profiles SET art=? WHERE id=?`, *q.Art, id); e != nil {
			return DirectSnapshot{}, e
		}
	}
	if q.PIN != nil {
		var value any
		if len(hash) > 0 {
			value = hash
		}
		if _, e = tx.ExecContext(ctx, `UPDATE direct_profiles SET pin_hash=?,pin_revision=pin_revision+1,pin_attempts=0,pin_locked_until=0 WHERE id=?`, value, id); e != nil {
			return DirectSnapshot{}, e
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM direct_profile_pin_attempts WHERE profile_id=?`, id); e != nil {
			return DirectSnapshot{}, e
		}
	}
	if q.Policy != nil {
		var value any
		if q.Policy.AllowedLibraries != nil {
			raw, _ := json.Marshal(q.Policy.AllowedLibraries)
			value = string(raw)
		}
		if _, e = tx.ExecContext(ctx, `UPDATE direct_profiles SET allowed_libraries=? WHERE id=?`, value, id); e != nil {
			return DirectSnapshot{}, e
		}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE direct_profiles SET revision=revision+1 WHERE id=?`, id); e != nil {
		return DirectSnapshot{}, e
	}
	if q.PIN != nil || q.Policy != nil {
		if _, e = tx.ExecContext(ctx, `DELETE FROM direct_profile_trust WHERE profile_id=?`, id); e != nil {
			return DirectSnapshot{}, e
		}
		if e = s.revokeDirectTx(ctx, tx, c.account.ID, id, ""); e != nil {
			return DirectSnapshot{}, e
		}
	}
	out, e := s.directSnapshotTx(ctx, tx, c)
	if e != nil {
		return out, e
	}
	return out, gated2.Commit()
}

type ProfileOrderEntry struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

func (s *Service) OrderDirectProfiles(ctx context.Context, bearer string, order []ProfileOrderEntry) (DirectSnapshot, error) {
	if len(order) < 1 || len(order) > 8 {
		return DirectSnapshot{}, ErrDirectInput
	}
	gated3, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return DirectSnapshot{}, e
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return DirectSnapshot{}, e
	}
	old, e := s.directSnapshotTx(ctx, tx, c)
	if e != nil {
		return old, e
	}
	if len(old.Profiles) != len(order) {
		return old, ErrProfileChanged
	}
	revisions := map[string]int64{}
	for _, p := range old.Profiles {
		revisions[p.ID] = p.Revision
	}
	for i, p := range order {
		if p.Revision < 1 || revisions[p.ID] != p.Revision {
			return old, ErrProfileChanged
		}
		delete(revisions, p.ID)
		if _, e = tx.ExecContext(ctx, `UPDATE direct_profiles SET position=?,revision=revision+1 WHERE account_id=? AND id=?`, i, c.account.ID, p.ID); e != nil {
			return old, e
		}
	}
	out, e := s.directSnapshotTx(ctx, tx, c)
	if e != nil {
		return out, e
	}
	return out, gated3.Commit()
}

type DirectSelection struct {
	PIN            string `json:"pin"`
	InstallationID string `json:"installationId"`
	TrustToken     string `json:"trustToken"`
	Trust          bool   `json:"trust"`
}
type DirectTrust struct {
	Authority          string `json:"authority"`
	Token              string `json:"token"`
	ExpiresAt          string `json:"expiresAt"`
	InstallationID     string `json:"installationId"`
	ServerID           string `json:"serverId"`
	AccountID          string `json:"accountId"`
	ProfileID          string `json:"profileId"`
	PINRevision        int64  `json:"pinRevision"`
	ProfileRevision    int64  `json:"profileRevision"`
	MembershipRevision int64  `json:"membershipRevision"`
}
type DirectSelectionResult struct {
	Session          Envelope     `json:"session"`
	TrustedSelection *DirectTrust `json:"trustedSelection,omitempty"`
}

func (s *Service) SelectDirectProfile(ctx context.Context, bearer, id string, q DirectSelection) (DirectSelectionResult, error) {
	return s.SelectDirectProfileFrom(ctx, bearer, id, q, true)
}
func (s *Service) SelectDirectProfileFrom(ctx context.Context, bearer, id string, q DirectSelection, private bool) (DirectSelectionResult, error) {
	out := DirectSelectionResult{}
	if !validFamilyID(id) || len(q.PIN) > 4 || len(q.TrustToken) > 128 || len(q.InstallationID) > 128 || q.Trust && !validFamilyID(q.InstallationID) {
		return out, ErrDirectInput
	}
	gated4, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return out, e
	}
	tx := gated4.Tx()
	defer gated4.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, false)
	if e != nil {
		return out, e
	}
	var hash []byte
	var revision, pinRevision int64
	var primary int
	e = tx.QueryRowContext(ctx, `SELECT pin_hash,revision,pin_revision,is_primary FROM direct_profiles WHERE account_id=? AND id=? AND deleted=0`, c.account.ID, id).Scan(&hash, &revision, &pinRevision, &primary)
	if e != nil {
		return out, ErrUnauthorized
	}
	trusted := false
	if q.TrustToken != "" && validFamilyID(q.InstallationID) {
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM direct_profile_trust WHERE token_hash=? AND account_id=? AND profile_id=? AND installation_id=? AND pin_revision=? AND membership_revision=? AND profile_revision=? AND expires_at>?)`, Digest(q.TrustToken), c.account.ID, id, q.InstallationID, pinRevision, c.account.Revision, revision, time.Now().UTC().Format(time.RFC3339)).Scan(&trusted)
		if e != nil {
			return out, e
		}
	}
	if len(hash) > 0 && !trusted {
		var attempts int
		var lastFailed, locked int64
		e = tx.QueryRowContext(ctx, `SELECT failed_attempts,last_failed_ms,locked_until_ms FROM direct_profile_pin_attempts WHERE profile_id=? AND device_id=?`, id, c.deviceID).Scan(&attempts, &lastFailed, &locked)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return out, e
		}
		nowMS := time.Now().UnixMilli()
		if nowMS-lastFailed > int64((24*time.Hour)/time.Millisecond) {
			attempts = 0
			locked = 0
		}
		if locked > nowMS {
			return out, ErrProfileLocked
		}
		select {
		case s.hashSlots <- struct{}{}:
			defer func() { <-s.hashSlots }()
		default:
			return out, ErrBusy
		}
		if !validPIN(q.PIN) || bcrypt.CompareHashAndPassword(hash, []byte(q.PIN)) != nil {
			attempts++
			delay := int64(2)
			if attempts >= 3 {
				delay = 30
			}
			if attempts >= 5 {
				delay = 300
			}
			if attempts >= 10 {
				delay = 900
			}
			if attempts >= 15 {
				delay = 3600
			}
			if _, e = tx.ExecContext(ctx, `INSERT INTO direct_profile_pin_attempts(profile_id,device_id,failed_attempts,last_failed_ms,locked_until_ms) VALUES(?,?,?,?,?) ON CONFLICT(profile_id,device_id) DO UPDATE SET failed_attempts=excluded.failed_attempts,last_failed_ms=excluded.last_failed_ms,locked_until_ms=excluded.locked_until_ms`, id, c.deviceID, attempts, nowMS, nowMS+delay*1000); e != nil {
				return out, e
			}
			if e = gated4.Commit(); e != nil {
				return out, e
			}
			return out, ErrProfilePIN
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM direct_profile_pin_attempts WHERE profile_id=? AND device_id=?`, id, c.deviceID); e != nil {
			return out, e
		}
	}
	role := c.account.Role
	if primary == 0 {
		role = "member"
	}
	if e = s.recoveryAllowedTx(ctx, tx, c.account.ID, c.familyID, private); e != nil {
		return out, e
	}
	out.Session, e = s.IssueTx(withExistingDevice(ctx, c.deviceID), tx, c.account.ID, id, "local", role, c.epoch, time.Time{})
	if e != nil {
		return out, e
	}
	if e = inheritPorticoAdmissionTx(ctx, tx, c.familyID, out.Session.SessionFamilyID); e != nil {
		return out, e
	}
	if c.viewerOnly {
		if _, e = tx.ExecContext(ctx, `INSERT INTO authorization_family_grants(family_id,grant_kind) VALUES(?,'viewer_only')`, out.Session.SessionFamilyID); e != nil {
			return out, e
		}
	}
	if q.Trust {
		proof := &DirectTrust{Authority: "local", Token: Token(), ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339), InstallationID: q.InstallationID, ServerID: s.ID(), AccountID: c.account.ID, ProfileID: id, PINRevision: pinRevision, ProfileRevision: revision, MembershipRevision: c.account.Revision}
		if _, e = tx.ExecContext(ctx, `DELETE FROM direct_profile_trust WHERE (account_id=? AND profile_id=? AND installation_id=?) OR expires_at<=?`, c.account.ID, id, q.InstallationID, time.Now().UTC().Format(time.RFC3339)); e != nil {
			return out, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO direct_profile_trust VALUES(?,?,?,?,?,?,?,?)`, Digest(proof.Token), c.account.ID, id, q.InstallationID, pinRevision, c.account.Revision, revision, proof.ExpiresAt); e != nil {
			return out, e
		}
		out.TrustedSelection = proof
	}
	return out, gated4.Commit()
}
func (s *Service) RevokeDirectTrust(ctx context.Context, bearer, profile, installation string) error {
	gated5, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return e
	}
	tx := gated5.Tx()
	defer gated5.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM direct_profile_trust WHERE account_id=? AND (?='' OR profile_id=?) AND (?='' OR installation_id=?)`, c.account.ID, profile, profile, installation, installation); e != nil {
		return e
	}
	return gated5.Commit()
}

type ProfileDeletionPreview struct {
	ProfileID      string `json:"profileId"`
	Revision       int64  `json:"revision"`
	Name           string `json:"name"`
	Playlists      int    `json:"playlists"`
	SavedResources int    `json:"savedResources"`
	Primary        bool   `json:"primary"`
}

func (s *Service) directDeletionTx(ctx context.Context, tx *sql.Tx, account, id string) (ProfileDeletionPreview, error) {
	p := ProfileDeletionPreview{ProfileID: id}
	e := tx.QueryRowContext(ctx, `SELECT name,revision,is_primary FROM direct_profiles WHERE account_id=? AND id=? AND deleted=0`, account, id).Scan(&p.Name, &p.Revision, &p.Primary)
	if e != nil {
		return p, e
	}
	e = tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM catalog_playlists WHERE owner_authority='local' AND owner_account=? AND owner_profile=? AND deleted=0),(SELECT count(*) FROM saved_resources WHERE owner_authority='local' AND owner_account=? AND owner_profile=? AND deleted=0)`, account, id, account, id).Scan(&p.Playlists, &p.SavedResources)
	return p, e
}
func (s *Service) PreviewDirectProfileDeletion(ctx context.Context, bearer, id string) (ProfileDeletionPreview, error) {
	gated6, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return ProfileDeletionPreview{}, e
	}
	tx := gated6.Tx()
	defer gated6.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return ProfileDeletionPreview{}, e
	}
	return s.directDeletionTx(ctx, tx, c.account.ID, id)
}
func (s *Service) DeleteDirectProfile(ctx context.Context, bearer, id string, expected int64, deleteSaved bool) error {
	if expected < 1 || !deleteSaved {
		return ErrDirectInput
	}
	gated7, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return e
	}
	tx := gated7.Tx()
	defer gated7.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return e
	}
	p, e := s.directDeletionTx(ctx, tx, c.account.ID, id)
	if e != nil {
		return e
	}
	if p.Primary {
		return ErrPrimaryProfile
	}
	if p.Revision != expected {
		return ErrProfileChanged
	}
	if e = s.revokeDirectTx(ctx, tx, c.account.ID, id, ""); e != nil {
		return e
	}
	if e = EraseProfileSavedDataTx(ctx, tx, Viewer{Authority: "local", AccountID: c.account.ID, ProfileID: id, ServerID: s.ID()}); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM direct_profile_trust WHERE account_id=? AND profile_id=?`, c.account.ID, id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE direct_profiles SET deleted=1,name='Deleted profile',pin_hash=NULL,art='slate',revision=revision+1,pin_revision=pin_revision+1 WHERE id=?`, id); e != nil {
		return e
	}
	return gated7.Commit()
}

// EraseProfileSavedDataTx is deliberately authority-qualified. Physical media and
// recordings are NOT deleted; existing playback revocation closes their readers.
func EraseProfileSavedDataTx(ctx context.Context, tx *sql.Tx, v Viewer) error {
	if v.Authority != "local" && v.Authority != "hosted" {
		return ErrUnauthorized
	}
	key := PersonalKey(v)
	for _, q := range []string{
		`DELETE FROM playlist_shares WHERE authority=? AND account_id=? AND profile_id=?`,
		`DELETE FROM saved_resource_shares WHERE authority=? AND account_id=? AND profile_id=?`,
		`DELETE FROM playlist_receipts WHERE authority=? AND account_id=? AND profile_id=?`,
		`DELETE FROM catalog_playlists WHERE owner_authority=? AND owner_account=? AND owner_profile=?`,
		`DELETE FROM saved_resources WHERE owner_authority=? AND owner_account=? AND owner_profile=?`,
	} {
		if _, e := tx.ExecContext(ctx, q, v.Authority, v.AccountID, v.ProfileID); e != nil {
			return e
		}
	}
	for _, table := range []string{"progress", "personal_items", "personal_receipts", "personal_history", "personal_snapshots", "personal_explicit_events", "personal_conflicts", "personal_activity_receipts", "personal_device_sequences", "personal_evidence_fences", "personal_play_counts"} {
		if _, e := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE profile_id=?`, key); e != nil {
			return e
		}
	}
	for _, table := range []string{"saved_pins", "saved_resource_receipts", "saved_operation_fences"} {
		if _, e := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE owner_key=?`, key); e != nil {
			return e
		}
	}
	return eraseProfileRuntimeDataTx(ctx, tx, v)
}

// Current local membership can be used by the share-target validator without a
// token. It is never an account or session authentication shortcut.
func DirectShareTarget(tx *sql.Tx, account, profile string) error {
	var valid bool
	e := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM direct_profiles p JOIN direct_memberships m ON m.account_id=p.account_id WHERE p.account_id=? AND p.id=? AND p.deleted=0 AND m.disabled=0)`, account, profile).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return errors.New("share target is not an active server profile")
	}
	return nil
}
