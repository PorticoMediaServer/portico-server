package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/server/internal/dbwork"
	"sort"
	"strings"
)

var ErrRestrictionInput = errors.New("invalid profile restriction")
var ErrRestrictionChanged = errors.New("These restrictions changed. Refresh before trying again.")

// ProfileRestrictions is the complete per-profile restriction document. It is
// owned by whoever may manage the account (the primary profile or the owner);
// the restricted profile itself may read it but never write it.
//
// MaximumAge is the resolved admission-age ceiling for MaximumAgeRating, or -1
// when the profile has no ceiling. Storing the resolved age alongside the code
// keeps catalog reads free of rating-table lookups and keeps a ceiling stable
// if a rating table entry is ever relabelled.
type ProfileRestrictions struct {
	ProfileID          string   `json:"profileId"`
	RatingSystem       string   `json:"ratingSystem"`
	MaximumAgeRating   string   `json:"maximumAgeRating"`
	MaximumAge         int      `json:"maximumAge"`
	AllowUnrated       bool     `json:"allowUnrated"`
	BlockedLabels      []string `json:"blockedLabels"`
	AllowDownloads     bool     `json:"allowDownloads"`
	AllowLiveTV        bool     `json:"allowLiveTv"`
	AllowDVR           bool     `json:"allowDvr"`
	AllowWatchTogether bool     `json:"allowWatchTogether"`
	Revision           int64    `json:"revision"`
}

// ContentRestrictions is the catalog-visible subset: the three facts that decide
// whether a viewer may see a title at all. The remaining flags gate features,
// not visibility, and are answered by the feature that owns them.
//
// Every field is chosen so the zero value restricts nothing. That is not a
// convenience: this value is embedded in catalog request structs that dozens of
// call sites build by hand, and a zero value that happened to mean "block
// everything" would turn a forgotten field into an empty library. MaximumAge is
// therefore a pointer (nil is no ceiling, and a ceiling of 0 stays expressible),
// and the unrated rule is stated as BlockUnrated rather than AllowUnrated.
type ContentRestrictions struct {
	ProfileID     string
	MaximumAge    *int
	BlockUnrated  bool
	BlockedLabels []string
	Revision      int64
	// Member limits are an independent, narrowing authority. They use the
	// access rating ladder, whose unknown values follow AllowUnrated.
	MemberMaxRating    string
	MemberAllowUnrated bool
	MemberDeniedLabels []string
	MemberRevision     int64
	// BlockRecordings is the profile's Recordings switch turned off (P8).
	// Published recordings live only in the owner's private DVR library, which
	// the library gate already withholds (recordingaccess), so this is not a
	// per-row catalogue predicate and does not make Active() true: it is
	// enforced by the single-item fence (contentaccess.VisibleItemTx) for a
	// recording reached by id, and folded into RestrictionFence.
	BlockRecordings bool
}

// Active reports whether the predicate has anything to enforce. An inactive
// restriction must compile to no SQL at all so unrestricted reads keep their
// existing plans.
func (r ContentRestrictions) Active() bool {
	return r.MaximumAge != nil || r.BlockUnrated || len(r.BlockedLabels) > 0 || r.MemberMaxRating != "" || len(r.MemberDeniedLabels) > 0
}

// Content projects the catalog-visible subset.
func (r ProfileRestrictions) Content() ContentRestrictions {
	out := ContentRestrictions{ProfileID: r.ProfileID, BlockUnrated: !r.AllowUnrated, BlockedLabels: r.BlockedLabels, Revision: r.Revision, BlockRecordings: !r.AllowDVR}
	if r.MaximumAge >= 0 {
		ceiling := r.MaximumAge
		out.MaximumAge = &ceiling
	}
	return out
}

// DefaultRestrictions is what a profile with no stored row means: nothing blocked.
func DefaultRestrictions(profile string) ProfileRestrictions {
	return ProfileRestrictions{ProfileID: profile, MaximumAge: -1, AllowUnrated: true, BlockedLabels: []string{}, AllowDownloads: true, AllowLiveTV: true, AllowDVR: true, AllowWatchTogether: true, Revision: 1}
}

// ProfileRestrictionEdit is a full replacement guarded by ExpectedRevision. A
// partial patch is deliberately not offered: a restriction document read, shown
// and re-submitted as a whole cannot lose a field a newer client added.
type ProfileRestrictionEdit struct {
	ExpectedRevision   int64    `json:"expectedRevision"`
	RatingSystem       string   `json:"ratingSystem"`
	MaximumAgeRating   string   `json:"maximumAgeRating"`
	AllowUnrated       bool     `json:"allowUnrated"`
	BlockedLabels      []string `json:"blockedLabels"`
	AllowDownloads     bool     `json:"allowDownloads"`
	AllowLiveTV        bool     `json:"allowLiveTv"`
	AllowDVR           bool     `json:"allowDvr"`
	AllowWatchTogether bool     `json:"allowWatchTogether"`
}

func normalizeLabels(raw []string) ([]string, bool) {
	if len(raw) > 64 {
		return nil, false
	}
	seen := map[string]bool{}
	out := []string{}
	for _, label := range raw {
		label = strings.TrimSpace(label)
		if label == "" || len(label) > 120 || strings.ContainsAny(label, "\r\n\t\x00") {
			return nil, false
		}
		key := strings.ToLower(label)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, label)
	}
	sort.Strings(out)
	return out, true
}

func scanRestrictions(row interface{ Scan(...any) error }, profile string) (ProfileRestrictions, error) {
	out := ProfileRestrictions{ProfileID: profile}
	var labels string
	if e := row.Scan(&out.RatingSystem, &out.MaximumAgeRating, &out.MaximumAge, &out.AllowUnrated, &labels, &out.AllowDownloads, &out.AllowLiveTV, &out.AllowDVR, &out.AllowWatchTogether, &out.Revision); e != nil {
		return out, e
	}
	if e := json.Unmarshal([]byte(labels), &out.BlockedLabels); e != nil {
		return out, e
	}
	if out.BlockedLabels == nil {
		out.BlockedLabels = []string{}
	}
	return out, nil
}

const restrictionColumns = `rating_system,maximum_age_rating,maximum_age,allow_unrated,blocked_labels,allow_downloads,allow_live_tv,allow_dvr,allow_watch_together,revision`

// RestrictionsTx reads one profile's restrictions inside a caller transaction.
// A missing row is not an error: it is the documented "nothing blocked" default.
func RestrictionsTx(ctx context.Context, tx *sql.Tx, profile string) (ProfileRestrictions, error) {
	out, e := scanRestrictions(tx.QueryRowContext(ctx, `SELECT `+restrictionColumns+` FROM profile_restrictions WHERE profile_id=?`, profile), profile)
	if errors.Is(e, sql.ErrNoRows) {
		return DefaultRestrictions(profile), nil
	}
	return out, e
}

// API keys belong to a local account and inherit that owner's profile policy.
// Hosted profile ids live in a separate authority and can collide with local
// ids. Unknown authorities are refused rather than given an open default.
func apiKeyProfileBelongsToAccount(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, viewer Viewer) error {
	var allowed bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM direct_profiles WHERE id=? AND account_id=? AND deleted=0)`, viewer.ProfileID, viewer.AccountID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrUnauthorized
	}
	return nil
}

func RestrictionsForViewerTx(ctx context.Context, tx *sql.Tx, viewer Viewer) (ProfileRestrictions, error) {
	switch viewer.Authority {
	case "local":
		return RestrictionsTx(ctx, tx, viewer.ProfileID)
	case "api-key":
		if err := apiKeyProfileBelongsToAccount(ctx, tx, viewer); err != nil {
			return ProfileRestrictions{}, err
		}
		return RestrictionsTx(ctx, tx, viewer.ProfileID)
	case "hosted":
		return DefaultRestrictions(viewer.ProfileID), nil
	default:
		return ProfileRestrictions{}, ErrUnauthorized
	}
}

// ViewerRestrictions resolves the content restrictions a viewer session carries.
// Catalog reads take this value; it never widens what a library policy allows.
func (s *Service) ViewerRestrictions(ctx context.Context, p Principal) (ContentRestrictions, error) {
	if p.Authority != "local" && p.Authority != "api-key" && p.Authority != "hosted" {
		return ContentRestrictions{}, ErrUnauthorized
	}
	if p.ProfileID == "" {
		return ContentRestrictions{}, nil
	}
	if p.Authority == "hosted" {
		return DefaultRestrictions(p.ProfileID).Content(), nil
	}
	if p.Authority == "api-key" {
		if err := apiKeyProfileBelongsToAccount(ctx, s.db, p.Viewer); err != nil {
			return ContentRestrictions{}, err
		}
	}
	out, e := scanRestrictions(s.db.QueryRowContext(ctx, `SELECT `+restrictionColumns+` FROM profile_restrictions WHERE profile_id=?`, p.ProfileID), p.ProfileID)
	if errors.Is(e, sql.ErrNoRows) {
		return DefaultRestrictions(p.ProfileID).Content(), nil
	}
	if e != nil {
		return ContentRestrictions{}, e
	}
	return out.Content(), nil
}

// RestrictionRevision is folded into the viewer fence so an open cursor or a
// cached page cannot outlive a restriction change.
func (s *Service) RestrictionRevision(ctx context.Context, profile string) (int64, error) {
	var revision int64
	e := s.db.QueryRowContext(ctx, `SELECT revision FROM profile_restrictions WHERE profile_id=?`, profile).Scan(&revision)
	if errors.Is(e, sql.ErrNoRows) {
		return 0, nil
	}
	return revision, e
}

// ProfileRestrictions reads a profile's restrictions through an account session.
// The caller must be able to manage the account, or be the profile itself.
func (s *Service) ProfileRestrictions(ctx context.Context, bearer, profile string) (ProfileRestrictions, error) {
	if !validFamilyID(profile) {
		return ProfileRestrictions{}, ErrRestrictionInput
	}
	gated, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return ProfileRestrictions{}, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, false)
	if e != nil {
		return ProfileRestrictions{}, e
	}
	if !c.manage && c.profileID != profile {
		return ProfileRestrictions{}, ErrForbidden
	}
	var deleted int
	if e = tx.QueryRowContext(ctx, `SELECT deleted FROM direct_profiles WHERE account_id=? AND id=?`, c.account.ID, profile).Scan(&deleted); e != nil || deleted != 0 {
		return ProfileRestrictions{}, ErrNotVisible
	}
	return RestrictionsTx(ctx, tx, profile)
}

// SetProfileRestrictions replaces a profile's restriction document. Account
// management authority is carried by the ordinary session; a child profile's
// viewing session cannot manage the account.
//
// Tightening or loosening both revoke the profile's viewing families, so an open
// page, cursor or playback grant cannot outlive the change.
func (s *Service) SetProfileRestrictions(ctx context.Context, bearer, profile string, edit ProfileRestrictionEdit) (ProfileRestrictions, error) {
	if !validFamilyID(profile) || edit.ExpectedRevision < 1 {
		return ProfileRestrictions{}, ErrRestrictionInput
	}
	labels, ok := normalizeLabels(edit.BlockedLabels)
	if !ok {
		return ProfileRestrictions{}, ErrRestrictionInput
	}
	system := strings.TrimSpace(edit.RatingSystem)
	code := strings.TrimSpace(edit.MaximumAgeRating)
	if code == "" {
		system = ""
	}
	ceiling, ok := RatingCeiling(system, code)
	if !ok {
		return ProfileRestrictions{}, ErrRestrictionInput
	}
	gated2, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return ProfileRestrictions{}, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return ProfileRestrictions{}, e
	}
	var deleted int
	if e = tx.QueryRowContext(ctx, `SELECT deleted FROM direct_profiles WHERE account_id=? AND id=?`, c.account.ID, profile).Scan(&deleted); e != nil || deleted != 0 {
		return ProfileRestrictions{}, ErrNotVisible
	}
	current, e := RestrictionsTx(ctx, tx, profile)
	if e != nil {
		return ProfileRestrictions{}, e
	}
	if current.Revision != edit.ExpectedRevision {
		return ProfileRestrictions{}, ErrRestrictionChanged
	}
	raw, _ := json.Marshal(labels)
	if _, e = tx.ExecContext(ctx, `INSERT INTO profile_restrictions(profile_id,rating_system,maximum_age_rating,maximum_age,allow_unrated,blocked_labels,allow_downloads,allow_live_tv,allow_dvr,allow_watch_together,revision) VALUES(?,?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(profile_id) DO UPDATE SET rating_system=excluded.rating_system,maximum_age_rating=excluded.maximum_age_rating,maximum_age=excluded.maximum_age,allow_unrated=excluded.allow_unrated,blocked_labels=excluded.blocked_labels,allow_downloads=excluded.allow_downloads,allow_live_tv=excluded.allow_live_tv,allow_dvr=excluded.allow_dvr,allow_watch_together=excluded.allow_watch_together,revision=profile_restrictions.revision+1`,
		profile, system, code, ceiling, edit.AllowUnrated, string(raw), edit.AllowDownloads, edit.AllowLiveTV, edit.AllowDVR, edit.AllowWatchTogether, current.Revision+1); e != nil {
		return ProfileRestrictions{}, e
	}
	// A restriction change ends the profile's live sessions. The family hook is the
	// same one a PIN change uses, so playback occurrences, queues and physical
	// readers retire through the v2 controller model rather than being orphaned.
	if _, e = tx.ExecContext(ctx, `DELETE FROM direct_profile_trust WHERE profile_id=?`, profile); e != nil {
		return ProfileRestrictions{}, e
	}
	if e = s.revokeDirectTx(ctx, tx, c.account.ID, profile, ""); e != nil {
		return ProfileRestrictions{}, e
	}
	out, e := RestrictionsTx(ctx, tx, profile)
	if e != nil {
		return out, e
	}
	return out, gated2.Commit()
}

// ErrContentRestricted is the single refusal a restricted viewer sees, whether the
// title was filtered from a list, opened by id, or asked to play. One code means a
// client can say the same clear thing everywhere, and a probe cannot distinguish
// "blocked for you" from "blocked for a different reason".
var ErrContentRestricted = errors.New("This title is outside the limits set for this profile.")

// WithProfileRestrictions exposes the complete current policy to route admission.
func (s *Service) WithProfileRestrictions(ctx context.Context, p Principal, check func(ProfileRestrictions) error) error {
	if p.Authority != "local" && p.Authority != "api-key" && p.Authority != "hosted" {
		return ErrUnauthorized
	}
	if p.ProfileID == "" {
		return ErrUnauthorized
	}
	if p.Authority == "hosted" {
		return check(DefaultRestrictions(p.ProfileID))
	}
	if p.Authority == "api-key" {
		if err := apiKeyProfileBelongsToAccount(ctx, s.db, p.Viewer); err != nil {
			return err
		}
	}
	out, err := scanRestrictions(s.db.QueryRowContext(ctx, `SELECT `+restrictionColumns+` FROM profile_restrictions WHERE profile_id=?`, p.ProfileID), p.ProfileID)
	if errors.Is(err, sql.ErrNoRows) {
		out = DefaultRestrictions(p.ProfileID)
		err = nil
	}
	if err != nil {
		return err
	}
	return check(out)
}
