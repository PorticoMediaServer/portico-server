package access

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"portico.local/server/internal/identity"
)

// Member is one account as administration sees it: its tier, its library allow
// list and the revision of its limits envelope, so a console can render the
// people page from one list.
type Member struct {
	AccountID        string   `json:"accountId"`
	Username         string   `json:"username"`
	ProfileID        string   `json:"profileId"`
	Role             string   `json:"role"`
	Disabled         bool     `json:"disabled"`
	Revision         int64    `json:"revision"`
	AllowedLibraries []string `json:"allowedLibraries"`
	LimitsRevision   int64    `json:"limitsRevision"`
	Limits           Limits   `json:"limits"`
	// HostedAccountID marks a Portico Account member: no password here; its
	// password, email and second factor are managed at the Portico Account.
	HostedAccountID string `json:"hostedAccountId,omitempty"`
}

type MemberPage struct {
	Page
	Items []Member `json:"items"`
}

// Members lists accounts newest username first by page. The caller's own tier is
// passed in so the list can mark which rows that caller may manage, which keeps
// the tier rule in identity.ManagesTier rather than in a client.
type MemberQuery struct {
	Cursor string
	Limit  string
}

func (s *Store) Members(ctx context.Context, auth Authorize, q MemberQuery) (out MemberPage, err error) {
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return out, err
	}
	_, after, err := decodeCursor(q.Cursor)
	if err != nil {
		return out, err
	}
	out.Limit, out.Items = limit, []Member{}
	err = s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, `SELECT a.id,a.username,a.profile_id,m.role,m.disabled,m.revision,m.allowed_libraries,
 COALESCE(l.revision,1),COALESCE(l.body,''),COALESCE(p.hosted_account_id,'') FROM accounts a JOIN direct_memberships m ON m.account_id=a.id
 LEFT JOIN access_limits l ON l.account_id=a.id LEFT JOIN account_portico_links p ON p.account_id=a.id WHERE a.id>? ORDER BY a.id LIMIT ?`, after, limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var m Member
			var libraries, body string
			if e = rows.Scan(&m.AccountID, &m.Username, &m.ProfileID, &m.Role, &m.Disabled, &m.Revision, &libraries, &m.LimitsRevision, &body, &m.HostedAccountID); e != nil {
				return e
			}
			if e = json.Unmarshal([]byte(libraries), &m.AllowedLibraries); e != nil {
				return e
			}
			if m.AllowedLibraries == nil {
				m.AllowedLibraries = []string{}
			}
			m.Limits = DefaultLimits()
			if body != "" {
				if e = json.Unmarshal([]byte(body), &m.Limits); e != nil {
					return e
				}
			}
			out.Items = append(out.Items, m)
		}
		return rows.Err()
	})
	if err == nil && len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = encodeCursor(0, out.Items[limit-1].AccountID)
	}
	return
}

type RoleChange struct {
	ExpectedRevision int64  `json:"expectedRevision"`
	OperationID      string `json:"operationId"`
	Role             string `json:"role"`
}

// SetRole moves an account between the admin and member tiers. Ownership is not
// reachable here: it moves only through TransferDirectOwnership, which proves
// the current owner's password.
//
// actor is the calling principal's tier. identity.ManagesTier decides both ends:
// the caller must out-rank the account's current tier and the tier it is moving
// to, so an admin can neither promote anyone to admin nor demote another admin.
// Changing a tier bumps the account epoch, which revokes its live sessions, so
// a demoted administrator loses administrative tokens immediately.
func (s *Store) SetRole(ctx context.Context, auth Authorize, actor identity.Principal, account string, c RoleChange) (out Member, err error) {
	if !validID(account) {
		return out, invalid("accountId")
	}
	if !validOperationID(c.OperationID) {
		return out, invalid("operationId")
	}
	if c.Role != identity.TierAdmin && c.Role != identity.TierMember {
		return out, invalid("role")
	}
	if account == actor.AccountID {
		return out, identity.ErrUnauthorized
	}
	err = s.transaction(ctx, auth, func(tx *sql.Tx) error {
		scope := "role:" + account
		body, want, e := receipt(tx, scope, c.OperationID, c)
		if e != nil {
			return e
		}
		if body != "" {
			return json.Unmarshal([]byte(body), &out)
		}
		var current string
		var revision int64
		if e = tx.QueryRowContext(ctx, `SELECT role,revision FROM direct_memberships WHERE account_id=?`, account).Scan(&current, &revision); e != nil {
			return e
		}
		if !identity.ManagesTier(actor.Role, current) || !identity.ManagesTier(actor.Role, c.Role) {
			return identity.ErrUnauthorized
		}
		if revision != c.ExpectedRevision {
			return &ConflictError{revision}
		}
		if _, e = tx.ExecContext(ctx, `UPDATE direct_memberships SET role=?,revision=revision+1 WHERE account_id=?`, c.Role, account); e != nil {
			return e
		}
		if e = identity.RevokeFamiliesMatchingTx(ctx, tx, identity.RevokedMembershipRemoved, `authority='local' AND account_id=?`, account); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE accounts SET epoch=epoch+1 WHERE id=?`, account); e != nil {
			return e
		}
		if out, e = readMember(ctx, tx, account); e != nil {
			return e
		}
		return saveReceipt(tx, scope, c.OperationID, want, out, s.now())
	})
	return
}

func readMember(ctx context.Context, tx *sql.Tx, account string) (Member, error) {
	var m Member
	var libraries, body string
	e := tx.QueryRowContext(ctx, `SELECT a.id,a.username,a.profile_id,m.role,m.disabled,m.revision,m.allowed_libraries,
 COALESCE(l.revision,1),COALESCE(l.body,''),COALESCE(p.hosted_account_id,'') FROM accounts a JOIN direct_memberships m ON m.account_id=a.id
 LEFT JOIN access_limits l ON l.account_id=a.id LEFT JOIN account_portico_links p ON p.account_id=a.id WHERE a.id=?`, account).Scan(&m.AccountID, &m.Username, &m.ProfileID, &m.Role, &m.Disabled, &m.Revision, &libraries, &m.LimitsRevision, &body, &m.HostedAccountID)
	if e != nil {
		return m, e
	}
	if e = json.Unmarshal([]byte(libraries), &m.AllowedLibraries); e != nil {
		return m, e
	}
	if m.AllowedLibraries == nil {
		m.AllowedLibraries = []string{}
	}
	m.Limits = DefaultLimits()
	if body != "" {
		if e = json.Unmarshal([]byte(body), &m.Limits); e != nil {
			return m, e
		}
	}
	return m, nil
}

// Role reads one account's stored tier inside an existing transaction. The HTTP
// authorization helpers use it so a token's claimed role is never the authority.
func Role(tx *sql.Tx, account string) (string, error) {
	var role string
	var disabled bool
	e := tx.QueryRow(`SELECT role,disabled FROM direct_memberships WHERE account_id=?`, account).Scan(&role, &disabled)
	if errors.Is(e, sql.ErrNoRows) {
		return "", identity.ErrUnauthorized
	}
	if e != nil {
		return "", e
	}
	if disabled {
		return "", identity.ErrUnauthorized
	}
	return role, nil
}
