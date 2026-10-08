package identity

import (
	"context"
	"errors"
	"testing"
)

// A profile restriction is the one setting in Portico whose failure mode is a
// child seeing something. Every test here is written against that consequence,
// not against the shape of the struct.

func TestZeroContentRestrictionRestrictsNothing(t *testing.T) {
	// This is load-bearing far beyond this package: ContentRestrictions is
	// embedded in catalog request structs that dozens of call sites build by
	// hand. If the zero value were active, a call site that forgot the field
	// would hand a viewer an empty library.
	if (ContentRestrictions{}).Active() {
		t.Fatal("the zero restriction is active; a forgotten field would empty every library")
	}
	if DefaultRestrictions("profile").Content().Active() {
		t.Fatal("the documented default restricts something")
	}
}

func TestRatingsNormaliseAcrossBodiesAndSpellings(t *testing.T) {
	for raw, want := range map[string]int{
		"G": 0, "PG": 8, "PG-13": 13, "R": 17, "NC-17": 18,
		"TV-Y": 0, "TV-Y7": 7, "TV-14": 14, "TV-MA": 17,
		"U": 0, "12A": 12, "15": 15, "18": 18,
		"FSK 16": 16, "fsk18": 18,
		"MA15+": 15, "R18+": 18,
		"US:PG-13": 13, "de/FSK 12": 12, "  Rated R  ": 17,
		"-12": 12, "16+": 16,
	} {
		age, ok := RatingAge(raw)
		if !ok || age != want {
			t.Errorf("%q: got (%d,%v), want (%d,true)", raw, age, ok, want)
		}
	}
	// Anything the table does not know, and every spelling of "not rated", is
	// unrated. It must never resolve to age 0, which would read as "all ages".
	for _, raw := range []string{"", "   ", "NR", "Unrated", "not rated", "UR", "N/A", "None", "Banana", "99"} {
		if age, ok := RatingAge(raw); ok {
			t.Errorf("%q resolved to age %d; an unknown rating must be unrated, never all-ages", raw, age)
		}
	}
}

func TestRatingCeilingOnlyAcceptsAPublishedValue(t *testing.T) {
	if age, ok := RatingCeiling("", ""); !ok || age != -1 {
		t.Fatalf("no ceiling: got (%d,%v)", age, ok)
	}
	if age, ok := RatingCeiling("mpaa", "PG-13"); !ok || age != 13 {
		t.Fatalf("mpaa PG-13: got (%d,%v)", age, ok)
	}
	// A code from another system, or an invented one, must be refused rather
	// than silently treated as no ceiling.
	for _, pair := range [][2]string{{"mpaa", "TV-14"}, {"mpaa", "PG13"}, {"nonsense", "PG"}, {"bbfc", "R"}} {
		if _, ok := RatingCeiling(pair[0], pair[1]); ok {
			t.Errorf("%v was accepted as a ceiling", pair)
		}
	}
}

func restrictionFixture(t *testing.T) (*Service, DirectSignIn, DirectProfile) {
	t.Helper()
	s, _, login := directFixture(t)
	snapshot, e := s.CreateDirectProfile(context.Background(), login.AccountToken, "Child", "mint")
	if e != nil {
		t.Fatal(e)
	}
	return s, login, childProfile(t, snapshot)
}

func TestRestrictionsUseTheAccountSessionWithoutStepUp(t *testing.T) {
	ctx := context.Background()
	s, login, child := restrictionFixture(t)
	current, e := s.ProfileRestrictions(ctx, login.AccountToken, child.ID)
	if e != nil {
		t.Fatal(e)
	}
	if current.MaximumAge != -1 || !current.AllowUnrated || current.Revision != 1 {
		t.Fatalf("unset profile did not read as nothing blocked: %+v", current)
	}
	edit := ProfileRestrictionEdit{ExpectedRevision: 1, RatingSystem: "mpaa", MaximumAgeRating: "PG-13", AllowUnrated: false, BlockedLabels: []string{"Horror"}}

	stored, e := s.SetProfileRestrictions(ctx, login.AccountToken, child.ID, edit)
	if e != nil {
		t.Fatal(e)
	}
	if stored.MaximumAge != 13 || stored.AllowUnrated || len(stored.BlockedLabels) != 1 || stored.Revision != 2 {
		t.Fatalf("stored %+v", stored)
	}

	// The revision fence still prevents stale writes.
	edit.ExpectedRevision = 2
	if _, e = s.SetProfileRestrictions(ctx, login.AccountToken, child.ID, edit); e != nil {
		t.Fatalf("account session should manage restrictions without a password: %v", e)
	}
}

func TestViewingProfileCannotReadAnotherProfilesRestrictions(t *testing.T) {
	ctx := context.Background()
	s, login, child := restrictionFixture(t)
	selection, err := s.SelectDirectProfile(ctx, login.AccountToken, child.ID, DirectSelection{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ProfileRestrictions(ctx, selection.Session.AccessToken, child.ID); err != nil {
		t.Fatalf("own restriction read: %v", err)
	}
	if _, err = s.ProfileRestrictions(ctx, selection.Session.AccessToken, login.Account.PrimaryProfileID); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("another profile's restrictions were exposed: %v", err)
	}
}

func TestHostedProfileCannotBorrowCollidingLocalRestrictions(t *testing.T) {
	ctx := context.Background()
	s, login, child := restrictionFixture(t)
	if _, err := s.SetProfileRestrictions(ctx, login.AccountToken, child.ID, ProfileRestrictionEdit{
		ExpectedRevision: 1, RatingSystem: "mpaa", MaximumAgeRating: "PG-13", AllowUnrated: false,
		BlockedLabels: []string{"Horror"}, AllowDownloads: false,
	}); err != nil {
		t.Fatal(err)
	}
	hosted := Viewer{Authority: "hosted", AccountID: "another-account", ProfileID: child.ID}
	content, err := s.ViewerRestrictions(ctx, Principal{Viewer: hosted})
	if err != nil || content.Active() {
		t.Fatalf("hosted viewer inherited local content policy: %+v %v", content, err)
	}
	if err := s.WithProfileRestrictions(ctx, Principal{Viewer: hosted}, func(r ProfileRestrictions) error {
		if r.MaximumAge != -1 || !r.AllowDownloads {
			t.Fatalf("hosted viewer inherited local feature policy: %+v", r)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	restrictions, err := RestrictionsForViewerTx(ctx, tx, hosted)
	if err != nil || restrictions.MaximumAge != -1 || !restrictions.AllowDownloads {
		t.Fatalf("hosted transaction inherited local policy: %+v %v", restrictions, err)
	}
}

func TestAPIKeyInheritsItsLocalOwnersProfileRestrictions(t *testing.T) {
	ctx := context.Background()
	s, login, child := restrictionFixture(t)
	if _, err := s.SetProfileRestrictions(ctx, login.AccountToken, child.ID, ProfileRestrictionEdit{
		ExpectedRevision: 1, RatingSystem: "mpaa", MaximumAgeRating: "PG-13", AllowUnrated: false,
		AllowDownloads: false,
	}); err != nil {
		t.Fatal(err)
	}
	key := Viewer{Authority: "api-key", AccountID: login.Account.ID, ProfileID: child.ID}
	content, err := s.ViewerRestrictions(ctx, Principal{Viewer: key})
	if err != nil || !content.Active() {
		t.Fatalf("key lost content policy: %+v %v", content, err)
	}
	if err = s.WithProfileRestrictions(ctx, Principal{Viewer: key}, func(r ProfileRestrictions) error {
		if r.MaximumAge != 13 || r.AllowDownloads {
			t.Fatalf("key lost profile controls: %+v", r)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	policy, err := RestrictionsForViewerTx(ctx, tx, key)
	if err != nil || policy.MaximumAge != 13 {
		t.Fatalf("key transaction bypassed policy: %+v %v", policy, err)
	}
	key.AccountID = "different-account"
	if _, err = RestrictionsForViewerTx(ctx, tx, key); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("key borrowed another account's profile: %v", err)
	}
	key.Authority = "invented"
	if _, err = RestrictionsForViewerTx(ctx, tx, key); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unknown authority failed open: %v", err)
	}
}

func TestRestrictionChangeEndsTheProfilesSessions(t *testing.T) {
	ctx := context.Background()
	s, login, child := restrictionFixture(t)
	selected, e := s.SelectDirectProfile(ctx, login.AccountToken, child.ID, DirectSelection{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(selected.Session.AccessToken); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SetProfileRestrictions(ctx, login.AccountToken, child.ID, ProfileRestrictionEdit{ExpectedRevision: 1, RatingSystem: "mpaa", MaximumAgeRating: "PG"}); e != nil {
		t.Fatal(e)
	}
	// An open page, cursor or playback grant must not outlive the change.
	if _, e = s.Authenticate(selected.Session.AccessToken); !errors.Is(e, ErrUnauthorized) {
		t.Fatalf("the profile's session survived a restriction change: %v", e)
	}
}

func TestRestrictionsRejectAStaleRevisionAndBadInput(t *testing.T) {
	ctx := context.Background()
	s, login, child := restrictionFixture(t)
	if _, e := s.SetProfileRestrictions(ctx, login.AccountToken, child.ID, ProfileRestrictionEdit{ExpectedRevision: 7, AllowUnrated: true}); !errors.Is(e, ErrRestrictionChanged) {
		t.Fatalf("a stale revision was accepted: %v", e)
	}
	for name, edit := range map[string]ProfileRestrictionEdit{
		"unknown system": {ExpectedRevision: 1, RatingSystem: "nonsense", MaximumAgeRating: "PG"},
		"foreign code":   {ExpectedRevision: 1, RatingSystem: "mpaa", MaximumAgeRating: "TV-14"},
		"blank label":    {ExpectedRevision: 1, BlockedLabels: []string{"  "}},
		"control label":  {ExpectedRevision: 1, BlockedLabels: []string{"bad\nlabel"}},
		"no revision":    {ExpectedRevision: 0},
	} {
		if _, e := s.SetProfileRestrictions(ctx, login.AccountToken, child.ID, edit); !errors.Is(e, ErrRestrictionInput) {
			t.Errorf("%s: want ErrRestrictionInput, got %v", name, e)
		}
	}
}

func TestPINRecoveryUsesAccountSessionAndRetiresTrust(t *testing.T) {
	ctx := context.Background()
	s, login, child := restrictionFixture(t)
	// Give the child a PIN and a remembered device trust.
	if _, e := s.EditDirectProfile(ctx, login.AccountToken, child.ID, DirectProfileEdit{ExpectedRevision: child.Revision, PIN: pointer("4321")}); e != nil {
		t.Fatal(e)
	}
	installation := Token()
	selected, e := s.SelectDirectProfile(ctx, login.AccountToken, child.ID, DirectSelection{PIN: "4321", Trust: true, InstallationID: installation})
	if e != nil {
		t.Fatal(e)
	}
	if selected.TrustedSelection == nil {
		t.Fatal("trust proof missing")
	}

	methods, e := s.PINRecovery(ctx, login.AccountToken)
	if e != nil {
		t.Fatal(e)
	}
	if !methods.Password || methods.Authenticator || methods.RecoveryCode {
		t.Fatalf("methods before enrolment: %+v", methods)
	}
	after, e := s.PINReset(ctx, login.AccountToken, child.ID, "")
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range after.Profiles {
		if p.ID == child.ID && p.PINRequired {
			t.Fatal("the PIN was not cleared")
		}
	}
	// The remembered trust and the live session must both be gone: a device that
	// was trusted under the old PIN must not stay trusted under no PIN.
	if _, e = s.SelectDirectProfile(ctx, login.AccountToken, child.ID, DirectSelection{TrustToken: selected.TrustedSelection.Token, InstallationID: installation}); e != nil {
		// Selecting without a PIN now succeeds because the PIN is cleared; what
		// matters is that the trust row itself did not survive.
		t.Fatal(e)
	}
	var trusted int
	if e = s.db.QueryRow(`SELECT count(*) FROM direct_profile_trust WHERE profile_id=? AND token_hash=?`, child.ID, Digest(selected.TrustedSelection.Token)).Scan(&trusted); e != nil {
		t.Fatal(e)
	}
	if trusted != 0 {
		t.Fatal("remembered device trust survived a PIN reset")
	}
}

func TestPINRecoveryOptionsAfterFactorEnrolment(t *testing.T) {
	ctx := context.Background()
	s, login, _ := restrictionFixture(t)
	enrolment, e := s.EnrolTwoFactor(ctx, login.AccountToken, "Testing1!")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.VerifyTwoFactor(ctx, login.AccountToken, "Testing1!", currentTOTP(t, enrolment.Secret)); e != nil {
		t.Fatal(e)
	}
	methods, e := s.PINRecovery(ctx, login.AccountToken)
	if e != nil {
		t.Fatal(e)
	}
	if !methods.Authenticator || !methods.RecoveryCode {
		t.Fatalf("methods after enrolment: %+v", methods)
	}
}

func pointer[T any](v T) *T { return &v }
