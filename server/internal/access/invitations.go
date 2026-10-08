package access

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"portico.local/server/internal/identity"
)

// Invitation is a pending offer of an account on this direct server. The code is
// returned once, when the invitation is created, and never read back: only its
// digest is stored, so a leaked database cannot be used to accept invitations.
type Invitation struct {
	ID               string   `json:"id"`
	Email            string   `json:"email"`
	Role             string   `json:"role"`
	AllowedLibraries []string `json:"allowedLibraries"`
	State            string   `json:"state"`
	CreatedAt        string   `json:"createdAt"`
	ExpiresAt        string   `json:"expiresAt"`
	AcceptedAt       string   `json:"acceptedAt,omitempty"`
	AccountID        string   `json:"accountId,omitempty"`
	Revision         int64    `json:"revision"`
	// Code is present only on the response that creates the invitation.
	Code string `json:"code,omitempty"`
}

type InvitationPage struct {
	Page
	Items []Invitation `json:"items"`
}

type InvitationRequest struct {
	OperationID      string   `json:"operationId"`
	Email            string   `json:"email"`
	Role             string   `json:"role"`
	AllowedLibraries []string `json:"allowedLibraries"`
	ExpiresInHours   int      `json:"expiresInHours"`
}

// InvitationPreview contains only what the unauthenticated join page needs.
// It never exposes a library name or a full invitee address.
type InvitationPreview struct {
	ServerName   string `json:"serverName"`
	Email        string `json:"email"`
	ExpiresAt    string `json:"expiresAt"`
	Role         string `json:"role"`
	LibraryCount int    `json:"libraryCount"`
}

type InvitationPreviewRequest struct {
	Code string `json:"code"`
}

var ErrInvitationNotFound = errors.New("invitation not found")

func maskedInvitationEmail(email string) string {
	if email == "" {
		return ""
	}
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" || domain == "" {
		return "***"
	}
	return string([]rune(local)[0]) + "***@" + domain
}

// Preview treats an unknown, expired, revoked, or accepted code identically.
// It does not mutate or reserve the invitation.
func (s *Store) Preview(ctx context.Context, code string) (out InvitationPreview, err error) {
	code = strings.TrimSpace(code)
	if len(code) < 32 || len(code) > 128 {
		return out, ErrInvitationNotFound
	}
	err = s.snapshot(ctx, nil, func(tx *sql.Tx) error {
		var email, libraries string
		var expiry int64
		e := tx.QueryRowContext(ctx, `SELECT email,role,allowed_libraries,expires_ms FROM access_invitations WHERE code_hash=? AND state='pending' AND expires_ms>?`, digest(code), s.now()).Scan(&email, &out.Role, &libraries, &expiry)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrInvitationNotFound
		}
		if e != nil {
			return e
		}
		var ids []string
		if e = json.Unmarshal([]byte(libraries), &ids); e != nil {
			return e
		}
		out.Email = maskedInvitationEmail(email)
		out.ExpiresAt = time.UnixMilli(expiry).UTC().Format(time.RFC3339)
		out.LibraryCount = len(ids)
		return nil
	})
	return
}

// normalizeEmail lowercases and validates an address. An invitation's address is
// optional: neither this server nor Hosted mails invitations, so the owner shares
// the link and code, and an address only names who it is meant for (and keeps a
// second pending invitation to the same address from being created).
func normalizeEmail(raw string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if len(v) < 3 || len(v) > 254 || !safeText(v, 254) {
		return "", false
	}
	address, err := mail.ParseAddress(v)
	if err != nil || address.Address != v {
		return "", false
	}
	return v, true
}

func invitationRow(row interface{ Scan(...any) error }) (Invitation, error) {
	var v Invitation
	var libraries string
	var created, expires int64
	var accepted sql.NullInt64
	var account sql.NullString
	if e := row.Scan(&v.ID, &v.Email, &v.Role, &libraries, &v.State, &created, &expires, &accepted, &account, &v.Revision); e != nil {
		return v, e
	}
	if e := json.Unmarshal([]byte(libraries), &v.AllowedLibraries); e != nil {
		return v, e
	}
	if v.AllowedLibraries == nil {
		v.AllowedLibraries = []string{}
	}
	v.CreatedAt = time.UnixMilli(created).UTC().Format(time.RFC3339)
	v.ExpiresAt = time.UnixMilli(expires).UTC().Format(time.RFC3339)
	if accepted.Valid {
		v.AcceptedAt = time.UnixMilli(accepted.Int64).UTC().Format(time.RFC3339)
	}
	if account.Valid {
		v.AccountID = account.String
	}
	return v, nil
}

const invitationColumns = `id,email,role,allowed_libraries,state,created_ms,expires_ms,accepted_ms,account_id,revision`

// Invite creates a pending invitation. actor is the caller's tier: an admin may
// invite members only, because identity.ManagesTier forbids creating a peer.
func (s *Store) Invite(ctx context.Context, auth Authorize, actor identity.Principal, q InvitationRequest) (out Invitation, err error) {
	if !validOperationID(q.OperationID) {
		return out, invalid("operationId")
	}
	email := ""
	if strings.TrimSpace(q.Email) != "" {
		var ok bool
		if email, ok = normalizeEmail(q.Email); !ok {
			return out, invalid("email")
		}
	}
	if q.Role == "" {
		q.Role = identity.TierMember
	}
	if q.Role != identity.TierMember && q.Role != identity.TierAdmin {
		return out, invalid("role")
	}
	if !identity.ManagesTier(actor.Role, q.Role) {
		return out, identity.ErrUnauthorized
	}
	if q.AllowedLibraries == nil {
		q.AllowedLibraries = []string{}
	}
	if len(q.AllowedLibraries) > 256 {
		return out, invalid("allowedLibraries")
	}
	for _, id := range q.AllowedLibraries {
		if !validID(id) {
			return out, invalid("allowedLibraries")
		}
	}
	if q.ExpiresInHours == 0 {
		q.ExpiresInHours = 168
	}
	if q.ExpiresInHours < 1 || q.ExpiresInHours > 720 {
		return out, invalid("expiresInHours")
	}
	code := token()
	err = s.transaction(ctx, auth, func(tx *sql.Tx) error {
		scope := "invite"
		body, want, e := receipt(tx, scope, q.OperationID, q)
		if e != nil {
			return e
		}
		if body != "" {
			// A replay returns the stored document, which never carries the code:
			// the code exists exactly once, in the original response.
			return json.Unmarshal([]byte(body), &out)
		}
		var count int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM access_invitations WHERE state='pending'`).Scan(&count); e != nil {
			return e
		}
		if count >= 200 {
			return ErrCapacity
		}
		// Addressless invitations (link and code only) may be pending side by side.
		if email != "" {
			var taken bool
			if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM access_invitations WHERE email=? AND state='pending' AND expires_ms>?)`, email, s.now()).Scan(&taken); e != nil {
				return e
			}
			if taken {
				return ErrConflict
			}
			// An expired pending row keeps the partial unique index occupied; retire
			// it so the same address can be invited again.
			if _, e = tx.ExecContext(ctx, `UPDATE access_invitations SET state='revoked',revision=revision+1 WHERE email=? AND state='pending'`, email); e != nil {
				return e
			}
		}
		now := s.now()
		libraries, _ := json.Marshal(q.AllowedLibraries)
		id := token()[:22]
		expires := now + int64(q.ExpiresInHours)*int64(time.Hour/time.Millisecond)
		if _, e = tx.ExecContext(ctx, `INSERT INTO access_invitations(id,email,code_hash,role,allowed_libraries,state,created_ms,expires_ms,created_by,revision) VALUES(?,?,?,?,?,'pending',?,?,?,1)`,
			id, email, digest(code), q.Role, string(libraries), now, expires, actor.AccountID); e != nil {
			return e
		}
		if out, e = invitationRow(tx.QueryRowContext(ctx, `SELECT `+invitationColumns+` FROM access_invitations WHERE id=?`, id)); e != nil {
			return e
		}
		if e = saveReceipt(tx, scope, q.OperationID, want, out, now); e != nil {
			return e
		}
		out.Code = code
		return nil
	})
	return
}

type InvitationQuery struct {
	Cursor string
	Limit  string
	State  string
}

func (s *Store) Invitations(ctx context.Context, auth Authorize, q InvitationQuery) (out InvitationPage, err error) {
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return out, err
	}
	created, after, err := decodeCursor(q.Cursor)
	if err != nil {
		return out, err
	}
	switch q.State {
	case "", "pending", "accepted", "revoked":
	default:
		return out, invalid("state")
	}
	out.Limit, out.Items = limit, []Invitation{}
	err = s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		query := `SELECT ` + invitationColumns + ` FROM access_invitations WHERE (?='' OR state=?) AND (?=0 OR (created_ms,id)<(?,?)) ORDER BY created_ms DESC,id DESC LIMIT ?`
		rows, e := tx.QueryContext(ctx, query, q.State, q.State, created, created, after, limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := invitationRow(rows)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		return rows.Err()
	})
	if err == nil && len(out.Items) > limit {
		last := out.Items[limit-1]
		out.Items = out.Items[:limit]
		t, _ := time.Parse(time.RFC3339, last.CreatedAt)
		out.NextCursor = encodeCursor(t.UnixMilli(), last.ID)
	}
	return
}

type InvitationRevoke struct {
	ExpectedRevision int64  `json:"expectedRevision"`
	OperationID      string `json:"operationId"`
}

func (s *Store) RevokeInvitation(ctx context.Context, auth Authorize, id string, c InvitationRevoke) (out Invitation, err error) {
	if !validID(id) {
		return out, invalid("id")
	}
	if !validOperationID(c.OperationID) {
		return out, invalid("operationId")
	}
	err = s.transaction(ctx, auth, func(tx *sql.Tx) error {
		scope := "invite-revoke:" + id
		body, want, e := receipt(tx, scope, c.OperationID, c)
		if e != nil {
			return e
		}
		if body != "" {
			return json.Unmarshal([]byte(body), &out)
		}
		var state string
		var revision int64
		if e = tx.QueryRowContext(ctx, `SELECT state,revision FROM access_invitations WHERE id=?`, id).Scan(&state, &revision); e != nil {
			return e
		}
		if revision != c.ExpectedRevision {
			return &ConflictError{revision}
		}
		if state == "accepted" {
			// Revoking an accepted invitation would imply removing the account it
			// created, which is a separate, explicit act.
			return ErrConflict
		}
		if _, e = tx.ExecContext(ctx, `UPDATE access_invitations SET state='revoked',revision=revision+1 WHERE id=?`, id); e != nil {
			return e
		}
		if out, e = invitationRow(tx.QueryRowContext(ctx, `SELECT `+invitationColumns+` FROM access_invitations WHERE id=?`, id)); e != nil {
			return e
		}
		return saveReceipt(tx, scope, c.OperationID, want, out, s.now())
	})
	return
}

type Acceptance struct {
	Code     string `json:"code"`
	Username string `json:"username"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// Accept redeems an invitation code and creates the account it describes. It is
// unauthenticated by design — the invitee has no account yet — so the code is
// the whole authority and the caller rate-limits the route.
//
// hash is supplied by the caller so the password cost is paid outside the
// database transaction, which holds the single SQLite writer.
func (s *Store) Accept(ctx context.Context, q Acceptance, hash []byte) (out Member, err error) {
	code := strings.TrimSpace(q.Code)
	if len(code) < 32 || len(code) > 128 {
		return out, invalid("code")
	}
	username := strings.ToLower(strings.TrimSpace(q.Username))
	if len(username) < 3 || len(username) > 64 || strings.ContainsAny(username, " \t\r\n@") {
		return out, invalid("username")
	}
	name := strings.TrimSpace(q.Name)
	if name == "" {
		name = username
	}
	if !safeText(name, 120) {
		return out, invalid("name")
	}
	err = s.transaction(ctx, nil, func(tx *sql.Tx) error {
		var id, email, role, libraries string
		var expires int64
		e := tx.QueryRowContext(ctx, `SELECT id,email,role,allowed_libraries,expires_ms FROM access_invitations WHERE code_hash=? AND state='pending'`, digest(code)).Scan(&id, &email, &role, &libraries, &expires)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrExpired
		}
		if e != nil {
			return e
		}
		now := s.now()
		if expires <= now {
			return ErrExpired
		}
		var taken bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE username=?)`, username).Scan(&taken); e != nil {
			return e
		}
		if taken {
			return ErrConflict
		}
		account, profile := token(), token()
		// The direct_account_created trigger inserts the membership and primary
		// profile; the invitation's tier and library allow list are applied on top.
		if _, e = tx.ExecContext(ctx, `INSERT INTO accounts(id,username,password_hash,profile_id) VALUES(?,?,?,?)`, account, username, hash, profile); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE direct_memberships SET role=?,allowed_libraries=?,revision=revision+1 WHERE account_id=? AND role<>'owner'`, role, libraries, account); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE direct_profiles SET name=? WHERE id=?`, name, profile); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE access_invitations SET state='accepted',accepted_ms=?,account_id=?,revision=revision+1 WHERE id=?`, now, account, id); e != nil {
			return e
		}
		out, e = readMember(ctx, tx, account)
		return e
	})
	return
}

// PorticoAcceptance redeems an invitation for a Portico Account whose identity
// the caller has already verified (a Hosted identity assertion). The account it
// creates is linked to that identity and has no password; signIn runs in the
// same transaction, so the invitee comes back signed in or not at all.
type PorticoAcceptance struct {
	Code        string
	AccountID   string
	Username    string
	DisplayName string
	// Consent is the verified identity assertion, kept for Hosted.
	Consent string
}

// AcceptPortico is Accept for a Portico Account. A Hosted account that is
// already a member keeps its account; the invitation then restores a disabled
// membership and applies its tier and libraries (never to the owner).
func (s *Store) AcceptPortico(ctx context.Context, q PorticoAcceptance, signIn func(context.Context, *sql.Tx) (identity.DirectSignIn, error)) (out identity.DirectSignIn, err error) {
	code := strings.TrimSpace(q.Code)
	if len(code) < 32 || len(code) > 128 {
		return out, invalid("code")
	}
	if q.AccountID == "" || len(q.AccountID) > 128 || signIn == nil {
		return out, identity.ErrUnauthorized
	}
	username := strings.ToLower(strings.TrimSpace(q.Username))
	if len(username) < 3 || len(username) > 64 || strings.ContainsAny(username, " \t\r\n@") {
		username = "member"
	}
	name := strings.TrimSpace(q.DisplayName)
	if name == "" || !safeText(name, 120) {
		name = username
	}
	err = s.transaction(ctx, nil, func(tx *sql.Tx) error {
		var id, role, libraries string
		var expires int64
		e := tx.QueryRowContext(ctx, `SELECT id,role,allowed_libraries,expires_ms FROM access_invitations WHERE code_hash=? AND state='pending'`, digest(code)).Scan(&id, &role, &libraries, &expires)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrExpired
		}
		if e != nil {
			return e
		}
		now := s.now()
		if expires <= now {
			return ErrExpired
		}
		var account string
		e = tx.QueryRowContext(ctx, `SELECT account_id FROM account_portico_links WHERE hosted_account_id=?`, q.AccountID).Scan(&account)
		if errors.Is(e, sql.ErrNoRows) {
			account = token()
			profile := token()
			var taken bool
			if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE username=?)`, username).Scan(&taken); e != nil {
				return e
			}
			if taken {
				username += "-" + token()[:4]
			}
			// The direct_account_created trigger adds the membership and the
			// primary profile, as for any invited account; the journal trigger
			// queues the push that tells Hosted. The primary (manage) profile is
			// always this one new profile, named after the Portico Account's
			// display name: Hosted keeps no profiles for a server, so there is
			// no other to choose (INT N4). The role is the invitation's, admin
			// included, and Hosted indexes it as pushed (INT N3).
			if _, e = tx.ExecContext(ctx, `INSERT INTO accounts(id,username,password_hash,profile_id) VALUES(?,?,x'',?)`, account, strings.ToLower(username), profile); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `INSERT INTO account_portico_links(account_id,hosted_account_id,consent) VALUES(?,?,?)`, account, q.AccountID, q.Consent); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE direct_profiles SET name=? WHERE id=?`, name, profile); e != nil {
				return e
			}
		} else if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE direct_memberships SET role=?,allowed_libraries=?,disabled=0,revision=revision+1 WHERE account_id=? AND role<>'owner'`, role, libraries, account); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE access_invitations SET state='accepted',accepted_ms=?,account_id=?,revision=revision+1 WHERE id=?`, now, account, id); e != nil {
			return e
		}
		// Accepting is a fresh consent to be listed here, even for an account
		// whose standing did not change (it left at Hosted and joins again):
		// always push it, with the consent the sign-in below records.
		if _, e = tx.ExecContext(ctx, `INSERT INTO hosted_membership_journal(hosted_account_id) VALUES(?)`, q.AccountID); e != nil {
			return e
		}
		out, e = signIn(ctx, tx)
		return e
	})
	return
}

// HashPassword applies the same cost and policy the direct identity service uses
// for a member password, so an invited account is indistinguishable from one the
// owner created by hand.
func HashPassword(password string) ([]byte, error) {
	if !identity.ValidDirectPassword(password) {
		return nil, invalid("password")
	}
	return bcrypt.GenerateFromPassword([]byte(password), identity.PasswordCost)
}
