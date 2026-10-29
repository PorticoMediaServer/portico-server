package catalog

// A retained owner is copied privately, one current or previously retained
// Reading position per turn. Only a complete capture replaces its identity
// mappings, atomically with the ready/count flip. The caller owns selection,
// custody fences and the durable owner frontier.

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/personalstate"
)

// captureRetainedOwnerStepTx advances one selected owner. It returns true only
// when this writer publishes the fully copied header and identity mappings.
func captureRetainedOwnerStepTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation) (published bool, err error) {
	job, err := readRetainedCaptureJobTx(ctx, tx, m.Ref.ID)
	if err != nil {
		return false, err
	}
	if job.Generation != m.Ref.Generation {
		// The durable job belongs to another mutation incarnation. The caller
		// owns the reset and must not let this generation publish.
		return false, errRetainedCaptureRoot
	}
	if job.Arm == retainedCaptureArmPublished || job.Arm == retainedCaptureArmFinished {
		return false, errRetainedCaptureFinished
	}
	if !job.OwnerStarted || job.OwnerValue == "" || job.OwnerAfter != job.OwnerValue {
		return false, errRetainedCaptureState
	}

	root, err := readRetainedCaptureRootTx(ctx, tx, m)
	if err != nil {
		return false, err
	}
	if root == nil {
		return false, errRetainedCaptureRoot
	}

	if job.Arm == retainedCaptureArmReset {
		return false, resetRetainedCaptureStepTx(ctx, tx, m, job)
	}

	binding, err := readRetainedCaptureBindingTx(ctx, tx, job)
	if err != nil {
		return false, err
	}
	switch binding.state {
	case retainedCaptureBindingRotated, retainedCaptureBindingDead:
		// A rotated or destroyed binding has to be visible on the job and must
		// never fall through to a Hosted or raw-owner copy. Enter the same
		// bounded reset used for drift.
		return false, beginRetainedCaptureResetTx(ctx, tx, m, job, sql.NullInt64{}, "capture-binding")
	case retainedCaptureBindingOpaque:
		// Opaque values are copied verbatim; this component makes no Hosted
		// custody claim. The caller's actual source authority is what stands.
	case retainedCaptureBindingValid:
	}

	switch job.Arm {
	case retainedCaptureArmStart:
		return false, startRetainedCaptureTx(ctx, tx, m, job, root)
	case retainedCaptureArmHeader, retainedCaptureArmCurrent, retainedCaptureArmRetained, retainedCaptureArmPublish:
		header, err := readRetainedCaptureHeaderTx(ctx, tx, job.CurrentState.Int64)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Never reinterpret a missing header as complete. It may have been
				// removed by profile erasure (which also SET NULLs the job pointer),
				// so reset and revalidate this owner before any publication.
				return false, beginRetainedCaptureResetTx(ctx, tx, m, job, sql.NullInt64{}, "capture-missing")
			}
			return false, err
		}
		if !header.CaptureMutation.Valid || header.CaptureMutation.String != m.Ref.ID || header.Profile != job.OwnerValue {
			return false, errRetainedCaptureState
		}
		if header.Ready {
			// A ready=1 header is never a private copy.
			return false, errRetainedCaptureState
		}
		if header.CaptureGeneration != m.Ref.Generation {
			return false, beginRetainedCaptureResetTx(ctx, tx, m, job, sql.NullInt64{}, "capture-generation")
		}
		switch job.Arm {
		case retainedCaptureArmHeader:
			return false, advanceRetainedCaptureJobTx(ctx, tx, m, "capture_arm=2,reading_started=1")
		case retainedCaptureArmCurrent:
			return false, copyRetainedCurrentStepTx(ctx, tx, m, job)
		case retainedCaptureArmRetained:
			return false, copyRetainedIdentityStepTx(ctx, tx, m, job)
		case retainedCaptureArmPublish:
			return publishRetainedCaptureTx(ctx, tx, m, job)
		}
	}
	return false, errRetainedCaptureState
}

// capture_arm values. See the PROGRAM note for the full contract.
const (
	retainedCaptureArmStart     = 0
	retainedCaptureArmHeader    = 1
	retainedCaptureArmCurrent   = 2
	retainedCaptureArmRetained  = 3
	retainedCaptureArmPublish   = 4
	retainedCaptureArmPublished = 5
	retainedCaptureArmReset     = 6
	retainedCaptureArmFinished  = 7
)

var (
	// errRetainedCaptureRoot means the actual mutation root no longer matches.
	// The caller owns invalidation and the bounded private reset; this call made
	// no write.
	errRetainedCaptureRoot = errors.New("retained capture: actual mutation root/generation/entity/public no longer matches")

	// errRetainedCaptureFinished means this owner is already published or
	// finished without publication. Advance the frontier; do not recapture.
	errRetainedCaptureFinished = errors.New("retained capture: owner capture is already finished")

	// errRetainedCaptureState means the job/owner premise is not a selected
	// capture. This call made no write.
	errRetainedCaptureState = errors.New("retained capture: job/owner premise is not a selected capture")
)

type retainedCaptureJob struct {
	Generation                 int64
	OwnerStarted               bool
	OwnerAfter, OwnerValue     string
	Arm                        int
	CurrentState               sql.NullInt64
	ReadingArm                 string
	ReadingStarted             bool
	ReadingAssetAfter          int64
	ReadingStateAfter          int64
	ReadingSourceAfter         string
	IdentityAfter              int
	LocalAccount               string
	LocalProfile               string
	LocalMembershipIncarnation string
	LocalProfileLifetime       string
}

func readRetainedCaptureJobTx(ctx context.Context, tx *sql.Tx, mutation string) (retainedCaptureJob, error) {
	var j retainedCaptureJob
	err := tx.QueryRowContext(ctx, `SELECT generation,owner_started,owner_after,owner_value,capture_arm,current_state_id,
 reading_arm,reading_started,reading_asset_after,reading_state_after,reading_source_after,identity_after,
 local_account,local_profile,local_membership_incarnation,local_profile_lifetime
 FROM catalog_purge_jobs WHERE mutation_id=?`, mutation).
		Scan(&j.Generation, &j.OwnerStarted, &j.OwnerAfter, &j.OwnerValue, &j.Arm, &j.CurrentState,
			&j.ReadingArm, &j.ReadingStarted, &j.ReadingAssetAfter, &j.ReadingStateAfter, &j.ReadingSourceAfter, &j.IdentityAfter,
			&j.LocalAccount, &j.LocalProfile, &j.LocalMembershipIncarnation, &j.LocalProfileLifetime)
	if err != nil {
		return j, fmt.Errorf("retained capture job: %w", err)
	}
	return j, nil
}

// retainedCaptureRoot is the point recheck of the actual mutation and entity.
// It deliberately does not re-derive the admission source digest.
type retainedCaptureRoot struct {
	Generation  int64
	Status      string
	Entity      int64
	MutationPub string
	EntityPub   string
	Title       string
	Kind        int64
}

func readRetainedCaptureRootTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation) (*retainedCaptureRoot, error) {
	var r retainedCaptureRoot
	err := tx.QueryRowContext(ctx, `SELECT mt.generation,mt.status,mt.entity_id,mt.public_id,pid(e.public_id),e.title,e.kind
 FROM catalog_entity_mutations mt JOIN catalog_entities e ON e.id=mt.entity_id WHERE mt.mutation_id=?`, m.Ref.ID).
		Scan(&r.Generation, &r.Status, &r.Entity, &r.MutationPub, &r.EntityPub, &r.Title, &r.Kind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("retained capture root: %w", err)
	}
	if r.Generation != m.Ref.Generation || r.Status != "active" || r.Entity != m.Entity ||
		r.MutationPub != m.Public || r.EntityPub != m.Public || r.Kind != int64(m.Kind) {
		return nil, nil
	}
	return &r, nil
}

type retainedCaptureBindingState int

const (
	retainedCaptureBindingOpaque retainedCaptureBindingState = iota
	retainedCaptureBindingValid
	retainedCaptureBindingRotated
	retainedCaptureBindingDead
)

type retainedCaptureBinding struct {
	state       retainedCaptureBindingState
	incarnation string
	lifetime    string
}

// readRetainedCaptureBindingTx validates the lossless local owner tuple in the
// same writer. An empty local binding is the opaque/Hosted arm: no text-ID match
// to a local profile is made. A mismatch is never reinterpreted as opaque.
func readRetainedCaptureBindingTx(ctx context.Context, tx *sql.Tx, job retainedCaptureJob) (retainedCaptureBinding, error) {
	if job.LocalAccount == "" && job.LocalProfile == "" && job.LocalMembershipIncarnation == "" && job.LocalProfileLifetime == "" {
		// A lossless local tuple cannot become an opaque owner because a private
		// binding was cleared. Raw/Hosted legacy values remain exact opaque data;
		// no text-ID lookup or local identity activation is attempted for them.
		if encoded, ok := strings.CutPrefix(job.OwnerValue, "viewer:"); ok {
			if raw, e := base64.RawURLEncoding.DecodeString(encoded); e == nil {
				var tuple []string
				if json.Unmarshal(raw, &tuple) == nil && len(tuple) == 3 && tuple[0] == "local" {
					return retainedCaptureBinding{state: retainedCaptureBindingDead}, nil
				}
			}
		}
		return retainedCaptureBinding{state: retainedCaptureBindingOpaque}, nil
	}
	if job.LocalAccount == "" || job.LocalProfile == "" || job.LocalMembershipIncarnation == "" || job.LocalProfileLifetime == "" {
		return retainedCaptureBinding{state: retainedCaptureBindingDead}, nil
	}
	// canonicalPersonalOwnerKey: the same lossless tuple as
	// persistence.PersonalOwnerKey and identity.PersonalKey. The job stores the
	// four exact local scalars, never the token/epoch.
	if persistence.PersonalOwnerKey("local", job.LocalAccount, job.LocalProfile) != job.OwnerValue {
		return retainedCaptureBinding{state: retainedCaptureBindingDead}, nil
	}
	var lifetime string
	var deleted int
	var incarnation string
	var disabled int
	err := tx.QueryRowContext(ctx, `SELECT p.lifetime,p.deleted,m.incarnation,m.disabled
 FROM direct_profiles p JOIN direct_memberships m ON m.account_id=p.account_id
 WHERE p.account_id=? AND p.id=?`, job.LocalAccount, job.LocalProfile).
		Scan(&lifetime, &deleted, &incarnation, &disabled)
	if errors.Is(err, sql.ErrNoRows) {
		return retainedCaptureBinding{state: retainedCaptureBindingDead}, nil
	}
	if err != nil {
		return retainedCaptureBinding{}, fmt.Errorf("retained capture binding: %w", err)
	}
	if deleted != 0 || disabled != 0 {
		return retainedCaptureBinding{state: retainedCaptureBindingDead}, nil
	}
	if lifetime != job.LocalProfileLifetime || incarnation != job.LocalMembershipIncarnation {
		return retainedCaptureBinding{state: retainedCaptureBindingRotated, lifetime: lifetime, incarnation: incarnation}, nil
	}
	return retainedCaptureBinding{state: retainedCaptureBindingValid}, nil
}

type retainedCaptureHeader struct {
	ID                int64
	Profile           string
	Ready             bool
	CaptureMutation   sql.NullString
	CaptureGeneration int64
}

func readRetainedCaptureHeaderTx(ctx context.Context, tx *sql.Tx, id int64) (retainedCaptureHeader, error) {
	var h retainedCaptureHeader
	err := tx.QueryRowContext(ctx, `SELECT id,profile_id,ready,capture_mutation_id,capture_generation FROM retained_title_state WHERE id=?`, id).
		Scan(&h.ID, &h.Profile, &h.Ready, &h.CaptureMutation, &h.CaptureGeneration)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return h, sql.ErrNoRows
		}
		return h, fmt.Errorf("retained capture header: %w", err)
	}
	return h, nil
}

// findRetainedCaptureHeaderTx is the only lookup that is not by the job pointer.
// It is used for orphan cleanup after profile erasure cleared the pointer. The
// partial capture index is read raw (LIMIT 3) before any interpretation; more
// than one private header for one mutation is a refusal, never a guess.
func findRetainedCaptureHeaderTx(ctx context.Context, tx *sql.Tx, mutation string) (*retainedCaptureHeader, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,profile_id,ready,capture_mutation_id,capture_generation
 FROM retained_title_state INDEXED BY retained_title_state_capture
 WHERE capture_mutation_id IS NOT NULL AND capture_mutation_id=? ORDER BY profile_id LIMIT 3`, mutation)
	if err != nil {
		return nil, fmt.Errorf("retained capture orphan header: %w", err)
	}
	defer rows.Close()
	var found []retainedCaptureHeader
	for rows.Next() {
		var h retainedCaptureHeader
		if err = rows.Scan(&h.ID, &h.Profile, &h.Ready, &h.CaptureMutation, &h.CaptureGeneration); err != nil {
			return nil, err
		}
		found = append(found, h)
		if len(found) > 1 {
			// The one-private-header-per-mutation invariant is broken. Refuse
			// instead of choosing one.
			return nil, errRetainedCaptureState
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}

// startRetainedCaptureTx is arm 0: create the private ready=0 header from the
// exact old scalar personal-state expression and reset the <=12 mapping
// witnesses for this owner. Nothing is published and no identity mapping is
// written.
func startRetainedCaptureTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation, job retainedCaptureJob, root *retainedCaptureRoot) error {
	// An existing private header (pointer or orphan after profile erasure) has
	// cleanup priority before a fresh capture can start.
	if job.CurrentState.Valid {
		return beginRetainedCaptureResetTx(ctx, tx, m, job, sql.NullInt64{}, "capture-orphan")
	}
	orphan, err := findRetainedCaptureHeaderTx(ctx, tx, m.Ref.ID)
	if err != nil {
		return err
	}
	if orphan != nil {
		return beginRetainedCaptureResetTx(ctx, tx, m, job, sql.NullInt64{Int64: orphan.ID, Valid: true}, "capture-orphan")
	}
	if job.IdentityAfter != 0 || job.ReadingAssetAfter != 0 || job.ReadingStateAfter != 0 || job.ReadingSourceAfter != "" ||
		job.ReadingArm != "current" || job.ReadingStarted {
		// The caller's fresh-owner switch must zero the capture progress; a
		// half-written frontier is refused rather than reused.
		return errRetainedCaptureState
	}

	// Identity facts are created by the caller with fixed ordinals 1..12. Read
	// the raw bounded set once, before allocating anything.
	rows, err := tx.QueryContext(ctx, `SELECT ordinal,identity FROM catalog_purge_identities
 WHERE mutation_id=? ORDER BY ordinal LIMIT 13`, m.Ref.ID)
	if err != nil {
		return fmt.Errorf("retained capture identities: %w", err)
	}
	var identities []string
	for rows.Next() {
		var ordinal int
		var identity string
		if err = rows.Scan(&ordinal, &identity); err != nil {
			rows.Close()
			return err
		}
		if ordinal != len(identities)+1 || len(identities) >= retainedIdentityLimit {
			rows.Close()
			return errRetainedCaptureState
		}
		identities = append(identities, identity)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(identities) == 0 {
		// Old semantics keep no retained state without an identity to restore it
		// under. The owner is finished without publication.
		return finishRetainedCaptureOwnerTx(ctx, tx, m, "")
	}

	var watched, watchlisted, favorite, notInterested, completed, reading bool
	var rating sql.NullFloat64
	var lastPlayed, progressAt string
	var plays, position, unit int64
	// The scalar state is read exactly as retainStateTx reads it: one point
	// expression over the same tables, for this one owner.
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(`+personalstate.SQL("?1", "?2")+`,0),
 COALESCE((SELECT watchlisted FROM personal_items WHERE profile_id=?1 AND item_id=?2),0),COALESCE((SELECT favorite FROM personal_items WHERE profile_id=?1 AND item_id=?2),0),
 COALESCE((SELECT not_interested FROM personal_items WHERE profile_id=?1 AND item_id=?2),0),(SELECT rating FROM personal_items WHERE profile_id=?1 AND item_id=?2),
 COALESCE((SELECT last_played_at FROM personal_items WHERE profile_id=?1 AND item_id=?2),''),COALESCE((SELECT plays FROM personal_play_counts WHERE profile_id=?1 AND item_id=?2),0),
 COALESCE((SELECT position FROM progress WHERE profile_id=?1 AND item_id=?2),0),COALESCE((SELECT unit FROM progress WHERE profile_id=?1 AND item_id=?2),0),
 COALESCE((SELECT completed FROM progress WHERE profile_id=?1 AND item_id=?2),0),COALESCE((SELECT updated_at FROM progress_activity WHERE profile_id=?1 AND item_id=?2),'')`, job.OwnerValue, m.Entity).
		Scan(&watched, &watchlisted, &favorite, &notInterested, &rating, &lastPlayed, &plays, &position, &unit, &completed, &progressAt)
	if err != nil {
		return fmt.Errorf("retained capture scalar state: %w", err)
	}
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM reading_progress WHERE owner_key=? AND item_id=?)`, job.OwnerValue, m.Entity).Scan(&reading); err != nil {
		return fmt.Errorf("retained capture reading presence: %w", err)
	}
	if !watched && !watchlisted && !favorite && !notInterested && !rating.Valid && plays == 0 && position == 0 && !reading {
		return finishRetainedCaptureOwnerTx(ctx, tx, m, "")
	}

	var header int64
	err = tx.QueryRowContext(ctx, `INSERT INTO retained_title_state(profile_id,kind,title,watched,watchlisted,favorite,not_interested,rating,last_played_at,plays,position,unit,completed,progress_at,retained_ms,ready,capture_mutation_id,capture_generation)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,?,?) RETURNING id`,
		job.OwnerValue, root.Kind, root.Title, watched, watchlisted, favorite, notInterested, rating, lastPlayed, plays, position, unit, completed, progressAt, time.Now().UnixMilli(), m.Ref.ID, m.Ref.Generation).
		Scan(&header)
	if err != nil {
		return fmt.Errorf("retained capture header: %w", err)
	}
	// The <=12 witnesses belong to the private construction of one owner. Reset
	// them before any arm uses them; this is one indexed statement, and its row
	// effects are bounded by the gate's fixed ordinal CHECK.
	if _, err = tx.ExecContext(ctx, `UPDATE catalog_purge_identities
 SET mapping_captured=0,previous_state_id=NULL WHERE mutation_id=?`, m.Ref.ID); err != nil {
		return fmt.Errorf("retained capture witness reset: %w", err)
	}
	return advanceRetainedCaptureJobTx(ctx, tx, m,
		`capture_arm=1,current_state_id=?,reading_arm='current',reading_started=0,reading_asset_after=0,reading_state_after=0,reading_source_after='',identity_after=0`,
		header)
}

// copyRetainedCurrentStepTx is arm 2: one raw current Reading position per turn,
// indexed by (item_id, owner_key, asset_id). Current positions are copied first
// and win an equal observed_at.
func copyRetainedCurrentStepTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation, job retainedCaptureJob) error {
	var asset int64
	var token, sourceRevision, format string
	var cfi, section sql.NullString
	var page sql.NullInt64
	var progression float64
	var revision, observed int64
	err := tx.QueryRowContext(ctx, `SELECT p.asset_id,a.token,p.source_revision,p.format,p.cfi,p.section,p.page_index,p.progression,p.revision,p.observed_at
 FROM reading_progress p INDEXED BY reading_progress_retention_owner
 JOIN catalog_assets a ON a.id=p.asset_id
 WHERE p.item_id=? AND p.owner_key=? AND p.asset_id>? ORDER BY p.asset_id LIMIT 1`,
		m.Entity, job.OwnerValue, job.ReadingAssetAfter).
		Scan(&asset, &token, &sourceRevision, &format, &cfi, &section, &page, &progression, &revision, &observed)
	if errors.Is(err, sql.ErrNoRows) {
		return advanceRetainedCaptureJobTx(ctx, tx, m,
			`capture_arm=3,reading_arm='retained',identity_after=0,reading_state_after=0,reading_source_after=''`)
	}
	if err != nil {
		return fmt.Errorf("retained capture current reading: %w", err)
	}
	if err = upsertRetainedReadingTx(ctx, tx, job.CurrentState.Int64, token, sourceRevision, format, cfi, section, page, progression, revision, observed, false); err != nil {
		return err
	}
	return advanceRetainedCaptureJobTx(ctx, tx, m, `reading_asset_after=?`, asset)
}

// copyRetainedIdentityStepTx is arm 3: for one identity at a time, capture the
// exact prior mapping into the witness, then copy that captured header's
// Reading children one raw source id per turn. Old children replace only on a
// strictly newer observed_at, so the current arm wins an equal timestamp.
func copyRetainedIdentityStepTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation, job retainedCaptureJob) error {
	if job.IdentityAfter >= retainedIdentityLimit {
		return advanceRetainedCaptureJobTx(ctx, tx, m, `capture_arm=4,reading_arm='complete'`)
	}
	ordinal := job.IdentityAfter + 1
	var identity string
	var captured bool
	var previous sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT identity,mapping_captured,previous_state_id FROM catalog_purge_identities
 WHERE mutation_id=? AND ordinal=?`, m.Ref.ID, ordinal).Scan(&identity, &captured, &previous)
	if errors.Is(err, sql.ErrNoRows) {
		// Fixed contiguous ordinals 1..N; no row here means N identities done.
		return advanceRetainedCaptureJobTx(ctx, tx, m, `capture_arm=4,reading_arm='complete'`)
	}
	if err != nil {
		return fmt.Errorf("retained capture witness: %w", err)
	}
	if !captured {
		// The prior mapping is captured BEFORE any child of that header is read.
		var prior sql.NullInt64
		err = tx.QueryRowContext(ctx, `SELECT state_id FROM retained_title_identities WHERE identity=? AND profile_id=?`,
			identity, job.OwnerValue).Scan(&prior)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("retained capture prior mapping: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE catalog_purge_identities SET mapping_captured=1,previous_state_id=?
 WHERE mutation_id=? AND ordinal=?`, prior, m.Ref.ID, ordinal); err != nil {
			return fmt.Errorf("retained capture witness record: %w", err)
		}
		if prior.Valid {
			return advanceRetainedCaptureJobTx(ctx, tx, m, `reading_state_after=?,reading_source_after=''`, prior.Int64)
		}
		return advanceRetainedCaptureJobTx(ctx, tx, m, `reading_state_after=0,reading_source_after=''`)
	}
	if !previous.Valid {
		// The captured prior mapping was absent; this identity has no prior
		// children and the next identity starts.
		return advanceRetainedCaptureJobTx(ctx, tx, m,
			`identity_after=?,reading_state_after=0,reading_source_after=''`, ordinal)
	}
	if job.ReadingStateAfter != previous.Int64 {
		// The captured mapping is authoritative; a lost cursor restarts that
		// header's child walk from its first source id (idempotent upserts, old
		// children still replace only on a strictly newer observed_at).
		return advanceRetainedCaptureJobTx(ctx, tx, m,
			`reading_state_after=?,reading_source_after=''`, previous.Int64)
	}
	var sourceID, sourceRevision, format string
	var cfi, section sql.NullString
	var page sql.NullInt64
	var progression float64
	var revision, observed int64
	err = tx.QueryRowContext(ctx, `SELECT r.source_id,r.source_revision,r.format,r.cfi,r.section,r.page_index,r.progression,r.revision,r.observed_at
 FROM retained_reading_progress r WHERE r.state_id=? AND r.source_id>? ORDER BY r.source_id LIMIT 1`,
		previous.Int64, job.ReadingSourceAfter).
		Scan(&sourceID, &sourceRevision, &format, &cfi, &section, &page, &progression, &revision, &observed)
	if errors.Is(err, sql.ErrNoRows) {
		return advanceRetainedCaptureJobTx(ctx, tx, m,
			`identity_after=?,reading_state_after=0,reading_source_after=''`, ordinal)
	}
	if err != nil {
		return fmt.Errorf("retained capture prior reading: %w", err)
	}
	if err = upsertRetainedReadingTx(ctx, tx, job.CurrentState.Int64, sourceID, sourceRevision, format, cfi, section, page, progression, revision, observed, true); err != nil {
		return err
	}
	return advanceRetainedCaptureJobTx(ctx, tx, m, `reading_source_after=?`, sourceID)
}

// publishRetainedCaptureTx is arm 4: the final same-writer recheck and one-commit
// publication. <=12 mapping probes, the ready flip + maintained count, <=12
// mapping UPSERTs, then the job advancement: at most 15 effects.
func publishRetainedCaptureTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation, job retainedCaptureJob) (bool, error) {
	// Recheck the same root/lifetime facts in this writer. The caller's original
	// admission digest/source authority is not re-derived here.
	recheck, err := readRetainedCaptureRootTx(ctx, tx, m)
	if err != nil {
		return false, err
	}
	if recheck == nil {
		return false, errRetainedCaptureRoot
	}
	binding, err := readRetainedCaptureBindingTx(ctx, tx, job)
	if err != nil {
		return false, err
	}
	if binding.state != retainedCaptureBindingValid && binding.state != retainedCaptureBindingOpaque {
		return false, beginRetainedCaptureResetTx(ctx, tx, m, job, sql.NullInt64{}, "capture-binding")
	}

	rows, err := tx.QueryContext(ctx, `SELECT ordinal,identity,mapping_captured,previous_state_id
 FROM catalog_purge_identities WHERE mutation_id=? ORDER BY ordinal LIMIT 13`, m.Ref.ID)
	if err != nil {
		return false, fmt.Errorf("retained capture publication witnesses: %w", err)
	}
	type witness struct {
		identity string
		previous sql.NullInt64
	}
	var witnesses []witness
	for rows.Next() {
		var ordinal int
		var w witness
		var captured bool
		if err = rows.Scan(&ordinal, &w.identity, &captured, &w.previous); err != nil {
			rows.Close()
			return false, err
		}
		if ordinal != len(witnesses)+1 || len(witnesses) >= retainedIdentityLimit {
			rows.Close()
			return false, errRetainedCaptureState
		}
		if !captured {
			// Every identity arm must have captured its exact prior mapping.
			rows.Close()
			return false, errRetainedCaptureState
		}
		witnesses = append(witnesses, w)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	if len(witnesses) == 0 {
		return false, errRetainedCaptureState
	}

	// The <=12 exact current mappings, including absence, must still equal the
	// captured witnesses. Drift resets private construction; it never publishes
	// a stale or incomplete merge and never deletes the new mapping.
	for _, w := range witnesses {
		var current sql.NullInt64
		err = tx.QueryRowContext(ctx, `SELECT state_id FROM retained_title_identities WHERE identity=? AND profile_id=?`,
			w.identity, job.OwnerValue).Scan(&current)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("retained capture mapping recheck: %w", err)
		}
		if current.Valid != w.previous.Valid || (current.Valid && current.Int64 != w.previous.Int64) {
			return false, beginRetainedCaptureResetTx(ctx, tx, m, job, sql.NullInt64{}, "capture-drift")
		}
	}

	// Capture completeness falsifier for the current-Reading stream. A row that
	// appeared behind the already-copied cursor is the caller fence's case; this
	// forward probe only refuses to publish while the forward stream is non-empty.
	var more bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM reading_progress INDEXED BY reading_progress_retention_owner
 WHERE item_id=? AND owner_key=? AND asset_id>?)`, m.Entity, job.OwnerValue, job.ReadingAssetAfter).Scan(&more); err != nil {
		return false, fmt.Errorf("retained capture completeness: %w", err)
	}
	if more {
		return false, advanceRetainedCaptureJobTx(ctx, tx, m, `capture_arm=2,reading_started=1`)
	}

	// 1. ready flip + capture FK clear (one header row; count trigger adds one
	//    maintained-count effect). The transient ready header without identities
	//    is never visible between commits.
	res, err := tx.ExecContext(ctx, `UPDATE retained_title_state
 SET ready=1,capture_mutation_id=NULL,capture_generation=0
 WHERE id=? AND profile_id=? AND ready=0 AND capture_mutation_id=? AND capture_generation=?`,
		job.CurrentState.Int64, job.OwnerValue, m.Ref.ID, m.Ref.Generation)
	if err != nil {
		return false, fmt.Errorf("retained capture publish header: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		// The private header vanished or was already published. This is not a
		// completion; the caller owns invalidation/reset.
		return false, errRetainedCaptureRoot
	}
	// 2. <=12 exact identity mappings move to the complete state.
	for _, w := range witnesses {
		if _, err = tx.ExecContext(ctx, `INSERT INTO retained_title_identities(identity,profile_id,state_id) VALUES(?,?,?)
 ON CONFLICT(identity,profile_id) DO UPDATE SET state_id=excluded.state_id`,
			w.identity, job.OwnerValue, job.CurrentState.Int64); err != nil {
			return false, fmt.Errorf("retained capture publish mapping: %w", err)
		}
	}
	// 3. job advancement. The owner frontier itself is the caller's atomic write;
	//    this component never republishes the same owner (arm 5 is terminal).
	err = advanceRetainedCaptureJobTx(ctx, tx, m,
		`capture_arm=5,owner_started=0,current_state_id=NULL,reading_arm='current',reading_started=0,reading_asset_after=0,reading_state_after=0,reading_source_after='',identity_after=0`)
	if err != nil {
		return false, err
	}
	return true, nil
}

// beginRetainedCaptureResetTx marks a private copy invalid and arms the bounded
// reset. It is one job-row write. pointer is set when the caller already located
// the private header; otherwise the job's existing pointer is preserved and the
// reset step resolves it again.
func beginRetainedCaptureResetTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation, job retainedCaptureJob, pointer sql.NullInt64, reason string) error {
	var value any
	switch {
	case pointer.Valid:
		value = pointer.Int64
	case job.CurrentState.Valid:
		value = job.CurrentState.Int64
	}
	return advanceRetainedCaptureJobTx(ctx, tx, m,
		`capture_arm=6,current_state_id=?,reading_arm='current',reading_started=0,reading_asset_after=0,reading_state_after=0,reading_source_after='',identity_after=0,reason=?`,
		value, reason)
}

// resetRetainedCaptureStepTx is arm 6: drain the invalid private copy child-first
// one row per turn, delete the ready=0 header only when it has no children (the
// gate count trigger omits ready=0, so no count row is touched), then clear the
// <=12 mapping witnesses one fixed ordinal per turn and reset the job phase.
func resetRetainedCaptureStepTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation, job retainedCaptureJob) error {
	header, err := readRetainedCaptureHeaderTx(ctx, tx, job.CurrentState.Int64)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	haveHeader := err == nil
	if !haveHeader {
		// The pointer may have been SET NULL by profile erasure while the private
		// header row still exists; find and drain it before treating the owner as
		// clean.
		orphan, e := findRetainedCaptureHeaderTx(ctx, tx, m.Ref.ID)
		if e != nil {
			return e
		}
		if orphan != nil {
			header = *orphan
			haveHeader = true
			if e = advanceRetainedCaptureJobTx(ctx, tx, m, `current_state_id=?`, header.ID); e != nil {
				return e
			}
			return nil
		}
		if job.CurrentState.Valid {
			return advanceRetainedCaptureJobTx(ctx, tx, m, `current_state_id=NULL`)
		}
	}
	if haveHeader {
		if header.Ready {
			// A published header is never deleted by the private reset.
			return errRetainedCaptureState
		}
		if !header.CaptureMutation.Valid || header.CaptureMutation.String != m.Ref.ID {
			return errRetainedCaptureState
		}
		res, e := tx.ExecContext(ctx, `DELETE FROM retained_reading_progress
 WHERE state_id=? AND source_id IN(SELECT source_id FROM retained_reading_progress WHERE state_id=? ORDER BY source_id LIMIT 1)`,
			header.ID, header.ID)
		if e != nil {
			return fmt.Errorf("retained capture reset child: %w", e)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			// One RAW child per turn; the header waits until it is empty.
			return nil
		}
		res, e = tx.ExecContext(ctx, `DELETE FROM retained_title_state
 WHERE id=? AND ready=0 AND NOT EXISTS(SELECT 1 FROM retained_reading_progress WHERE state_id=?)`, header.ID, header.ID)
		if e != nil {
			return fmt.Errorf("retained capture reset header: %w", e)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return errRetainedCaptureState
		}
		return advanceRetainedCaptureJobTx(ctx, tx, m, `current_state_id=NULL`)
	}

	// Witnesses are cleared by fixed ordinal 1..12, one per bounded source turn.
	if job.IdentityAfter < retainedIdentityLimit {
		ordinal := job.IdentityAfter + 1
		res, e := tx.ExecContext(ctx, `UPDATE catalog_purge_identities SET mapping_captured=0,previous_state_id=NULL
 WHERE mutation_id=? AND ordinal=?`, m.Ref.ID, ordinal)
		if e != nil {
			return fmt.Errorf("retained capture reset witness: %w", e)
		}
		if n, _ := res.RowsAffected(); n == 1 {
			return advanceRetainedCaptureJobTx(ctx, tx, m, `identity_after=?`, ordinal)
		}
	}

	// Reset complete. Revalidate the exact local binding: a rotated lifetime may
	// adopt the current exact binding for the same non-reused logical owner and
	// start a fresh capture; a destroyed binding visibly blocks and never becomes
	// a Hosted/opaque copy or a complete claim.
	next, e := readRetainedCaptureBindingTx(ctx, tx, job)
	if e != nil {
		return e
	}
	switch next.state {
	case retainedCaptureBindingDead:
		return advanceRetainedCaptureJobTx(ctx, tx, m,
			`capture_arm=7,owner_started=0,current_state_id=NULL,reading_arm='current',reading_started=0,reading_asset_after=0,reading_state_after=0,reading_source_after='',identity_after=0,
 state='blocked',reason='binding'`)
	case retainedCaptureBindingRotated:
		return advanceRetainedCaptureJobTx(ctx, tx, m,
			`capture_arm=0,current_state_id=NULL,reading_arm='current',reading_started=0,reading_asset_after=0,reading_state_after=0,reading_source_after='',identity_after=0,
 state='pending',attempt=0,next_attempt_ms=0,reason='',
 local_membership_incarnation=?,local_profile_lifetime=?`, next.incarnation, next.lifetime)
	default:
		return advanceRetainedCaptureJobTx(ctx, tx, m,
			`capture_arm=0,current_state_id=NULL,reading_arm='current',reading_started=0,reading_asset_after=0,reading_state_after=0,reading_source_after='',identity_after=0,
 state='pending',attempt=0,next_attempt_ms=0,reason=''`)
	}
}

// finishRetainedCaptureOwnerTx ends an owner that retains nothing. It is a
// single job write; no private header exists.
func finishRetainedCaptureOwnerTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation, reason string) error {
	return advanceRetainedCaptureJobTx(ctx, tx, m,
		`capture_arm=7,owner_started=0,current_state_id=NULL,reading_arm='current',reading_started=0,reading_asset_after=0,reading_state_after=0,reading_source_after='',identity_after=0,reason=?`, reason)
}

// advanceRetainedCaptureJobTx is the single guarded job checkpoint used by every
// step. The guards make a concurrent owner switch or generation change a visible
// refusal instead of an advance on the wrong row.
func advanceRetainedCaptureJobTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation, set string, args ...any) error {
	query := `UPDATE catalog_purge_jobs SET ` + set + ` WHERE mutation_id=? AND generation=? AND owner_started=1`
	args = append(args, m.Ref.ID, m.Ref.Generation)
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("retained capture job advance: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return errRetainedCaptureState
	}
	return nil
}

// upsertRetainedReadingTx copies one position into the private header. The
// current arm uses >= (current wins equal observed_at); the prior-retained arm
// uses > (old only replaces strictly newer).
func upsertRetainedReadingTx(ctx context.Context, tx *sql.Tx, state int64, sourceID, sourceRevision, format string, cfi, section sql.NullString, page sql.NullInt64, progression float64, revision, observed int64, strictlyNewer bool) error {
	conflict := `excluded.observed_at>=retained_reading_progress.observed_at`
	if strictlyNewer {
		conflict = `excluded.observed_at>retained_reading_progress.observed_at`
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO retained_reading_progress(state_id,source_id,source_revision,format,cfi,section,page_index,progression,revision,observed_at)
 VALUES(?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(state_id,source_id) DO UPDATE SET source_revision=excluded.source_revision,format=excluded.format,cfi=excluded.cfi,section=excluded.section,page_index=excluded.page_index,progression=excluded.progression,revision=excluded.revision,observed_at=excluded.observed_at
 WHERE `+conflict,
		state, sourceID, sourceRevision, format, cfi, section, page, progression, revision, observed)
	if err != nil {
		return fmt.Errorf("retained capture reading upsert: %w", err)
	}
	return nil
}
