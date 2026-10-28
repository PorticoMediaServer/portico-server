package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// MemberProfile is one of another person's profiles as an owner or
// administrator manages it from People: the facts Profile & account shows for
// it, and its restrictions.
type MemberProfile struct {
	DirectProfile
	Restrictions ProfileRestrictions `json:"restrictions"`
}

// MemberProfiles is every live profile of one account, in its own order.
type MemberProfiles struct {
	AccountID string          `json:"accountId"`
	Items     []MemberProfile `json:"items"`
}

// MemberProfileEdit renames or recolours one profile.
type MemberProfileEdit struct {
	ExpectedRevision int64   `json:"expectedRevision"`
	Name             *string `json:"name,omitempty"`
	Art              *string `json:"art,omitempty"`
}

// MemberProfilePINReset clears a profile's PIN, so the person can choose a new
// one. Nobody else ever chooses it for them.
type MemberProfilePINReset struct {
	ExpectedRevision int64 `json:"expectedRevision"`
}

// The functions below act on an account other than the caller's. Each runs in
// a transaction whose caller has already proved, in that same transaction,
// that its tier manages the account's (an owner manages administrators and
// members, an administrator members only). They never read a bearer.

func memberProfileTx(ctx context.Context, tx *sql.Tx, account, id string) (MemberProfile, error) {
	var out MemberProfile
	var libraries sql.NullString
	e := tx.QueryRowContext(ctx, `SELECT id,name,is_primary,position,art,revision,pin_revision,pin_hash IS NOT NULL,allowed_libraries FROM direct_profiles WHERE account_id=? AND id=? AND deleted=0`, account, id).
		Scan(&out.ID, &out.Name, &out.Primary, &out.Position, &out.Art, &out.Revision, &out.PINRevision, &out.PINRequired, &libraries)
	if errors.Is(e, sql.ErrNoRows) {
		return out, ErrNotVisible
	}
	if e != nil {
		return out, e
	}
	if libraries.Valid {
		if e = json.Unmarshal([]byte(libraries.String), &out.AllowedLibraries); e != nil {
			return out, e
		}
	}
	if out.AllowedLibraries == nil {
		out.AllowedLibraries = []string{}
	}
	out.Restrictions, e = RestrictionsTx(ctx, tx, id)
	return out, e
}

// MemberProfilesTx lists an account's live profiles.
func MemberProfilesTx(ctx context.Context, tx *sql.Tx, account string) (MemberProfiles, error) {
	out := MemberProfiles{AccountID: account, Items: []MemberProfile{}}
	rows, e := tx.QueryContext(ctx, `SELECT id FROM direct_profiles WHERE account_id=? AND deleted=0 ORDER BY position,id LIMIT 64`, account)
	if e != nil {
		return out, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return out, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, id := range ids {
		p, e := memberProfileTx(ctx, tx, account, id)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	return out, nil
}

// memberRevisionTx fences an edit on the profile's revision.
func memberRevisionTx(ctx context.Context, tx *sql.Tx, account, id string, expected int64) error {
	var revision int64
	e := tx.QueryRowContext(ctx, `SELECT revision FROM direct_profiles WHERE account_id=? AND id=? AND deleted=0`, account, id).Scan(&revision)
	if errors.Is(e, sql.ErrNoRows) {
		return ErrNotVisible
	}
	if e != nil {
		return e
	}
	if revision != expected {
		return ErrProfileChanged
	}
	return nil
}

// EditMemberProfileTx renames or recolours one of an account's profiles.
func EditMemberProfileTx(ctx context.Context, tx *sql.Tx, account, id string, q MemberProfileEdit) (MemberProfile, error) {
	if !validFamilyID(id) || q.ExpectedRevision < 1 || q.Name == nil && q.Art == nil {
		return MemberProfile{}, ErrDirectInput
	}
	if q.Name != nil {
		v := strings.TrimSpace(*q.Name)
		if !directName(v) {
			return MemberProfile{}, ErrDirectInput
		}
		q.Name = &v
	}
	if q.Art != nil && !validProfileArt(*q.Art) {
		return MemberProfile{}, ErrDirectInput
	}
	if e := memberRevisionTx(ctx, tx, account, id, q.ExpectedRevision); e != nil {
		return MemberProfile{}, e
	}
	if q.Name != nil {
		if _, e := tx.ExecContext(ctx, `UPDATE direct_profiles SET name=? WHERE id=?`, *q.Name, id); e != nil {
			return MemberProfile{}, e
		}
	}
	if q.Art != nil {
		if _, e := tx.ExecContext(ctx, `UPDATE direct_profiles SET art=? WHERE id=?`, *q.Art, id); e != nil {
			return MemberProfile{}, e
		}
	}
	if _, e := tx.ExecContext(ctx, `UPDATE direct_profiles SET revision=revision+1 WHERE id=?`, id); e != nil {
		return MemberProfile{}, e
	}
	return memberProfileTx(ctx, tx, account, id)
}

// SetMemberProfileRestrictionsTx replaces one of an account's profile
// restrictions, exactly as the person's own main profile would.
func SetMemberProfileRestrictionsTx(ctx context.Context, tx *sql.Tx, account, id string, edit ProfileRestrictionEdit) (MemberProfile, error) {
	if !validFamilyID(id) {
		return MemberProfile{}, ErrRestrictionInput
	}
	if e := edit.normalize(); e != nil {
		return MemberProfile{}, e
	}
	if _, e := writeRestrictionsTx(ctx, tx, account, id, edit); e != nil {
		return MemberProfile{}, e
	}
	return memberProfileTx(ctx, tx, account, id)
}

// ClearMemberProfilePINTx removes a profile's PIN. As with the person's own
// reset, every remembered trust and viewing session that relied on the old
// PIN ends.
func (s *Service) ClearMemberProfilePINTx(ctx context.Context, tx *sql.Tx, account, id string, q MemberProfilePINReset) (MemberProfile, error) {
	if !validFamilyID(id) || q.ExpectedRevision < 1 {
		return MemberProfile{}, ErrDirectInput
	}
	if e := memberRevisionTx(ctx, tx, account, id, q.ExpectedRevision); e != nil {
		return MemberProfile{}, e
	}
	if _, e := tx.ExecContext(ctx, `UPDATE direct_profiles SET pin_hash=NULL,pin_revision=pin_revision+1,revision=revision+1 WHERE id=?`, id); e != nil {
		return MemberProfile{}, e
	}
	if e := s.retireProfilePINGrantsTx(ctx, tx, account, id); e != nil {
		return MemberProfile{}, e
	}
	return memberProfileTx(ctx, tx, account, id)
}
