package catalog

// P2-59 / O-022 retained-title walk: the bounded retention quantum for one
// title removal. It keeps the caller-selected owner capture component
// (captureRetainedOwnerStepTx) as the only copier, and adds the durable owner
// iterator and the per-owner input drain that component deliberately leaves to
// its caller.
//
// The durable frontier is only the existing catalog_purge_jobs scalars:
// owner_after/owner_value, the four exact local binding scalars and the shared
// phasedpurge cursor. plan_digest binds that cursor to this file's literal SQL
// and engine limits; a mismatch is refused, never restarted.
//
// This file does not delete the title, its catalogue rows or a published
// retained header. It does not create or drop catalog_purge_identities outside
// the initial bounded oracle, and it never calls EntityMutationSourceDigestTx.
// The parent owns admission source authority, late-writer fences, physical
// child cleanup and proof records.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/phasedpurge"
)

const (
	// Initial root (2), job (1), repeated Before root (2), exact local
	// profile/membership binding (2), two meter reads and Save's job point (1).
	retainedTitleOwnerReadBound = 10
	retainedTitleWalkVersion    = "p2-59-o022-title-walk-v4"
	retainedTitleOwnerDrained   = "owner_drained"
)

var (
	// errRetainedTitlePlan means the stored plan_digest is not this compiled
	// walk. The cursor is never reset or reinterpreted; the caller records a
	// visible block.
	errRetainedTitlePlan = errors.New("retained title walk: stored plan digest does not belong to this compiled walk")

	// errRetainedTitleCursor means the stored shared-engine cursor is malformed
	// or does not belong to the compiled drain plan. No write was made.
	errRetainedTitleCursor = errors.New("retained title walk: drain cursor does not belong to this compiled plan")

	// errRetainedTitleState means the durable frontier or capture premise is
	// not the aligned owner this step requires. No write was made.
	errRetainedTitleState = errors.New("retained title walk: job/owner frontier is not aligned")

	// errRetainedTitleRoot means the actual mutation root/generation/entity/
	// public no longer matches. No write was made; the caller owns invalidation.
	errRetainedTitleRoot = errors.New("retained title walk: actual mutation root/generation/entity/public no longer matches")

	// errRetainedTitleBinding means a local owner rotated after capture and
	// before its input drain. A new lifetime's rows are not this job's to delete;
	// the caller records the visible block.
	errRetainedTitleBinding = errors.New("retained title walk: owner binding rotated before its input drain")
)

// retainedTitleJob is the durable row this walk reads. It deliberately mirrors
// the capture component's fields instead of inventing a second state store.
type retainedTitleJob struct {
	generation int64
	plan       string
	cursor     string
	state      string
	reason     string

	ownerStarted bool
	ownerAfter   string
	ownerValue   string

	arm          int
	current      sql.NullInt64
	readingArm   string
	readingStart bool
	readingAsset int64
	readingState int64
	readingSrc   string
	identity     int

	localAccount, localProfile, localMembership, localLifetime string
}

func readRetainedTitleJobTx(ctx context.Context, tx *sql.Tx, mutation string) (*retainedTitleJob, error) {
	var j retainedTitleJob
	err := tx.QueryRowContext(ctx, `SELECT generation,plan_digest,cursor,state,reason,
 owner_started,owner_after,owner_value,capture_arm,current_state_id,
 reading_arm,reading_started,reading_asset_after,reading_state_after,reading_source_after,identity_after,
 local_account,local_profile,local_membership_incarnation,local_profile_lifetime
 FROM catalog_purge_jobs WHERE mutation_id=?`, mutation).
		Scan(&j.generation, &j.plan, &j.cursor, &j.state, &j.reason,
			&j.ownerStarted, &j.ownerAfter, &j.ownerValue, &j.arm, &j.current,
			&j.readingArm, &j.readingStart, &j.readingAsset, &j.readingState, &j.readingSrc, &j.identity,
			&j.localAccount, &j.localProfile, &j.localMembership, &j.localLifetime)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("retained title walk job: %w", err)
	}
	return &j, nil
}

func (j *retainedTitleJob) captureJob() retainedCaptureJob {
	return retainedCaptureJob{
		Generation:                 j.generation,
		OwnerStarted:               j.ownerStarted,
		OwnerAfter:                 j.ownerAfter,
		OwnerValue:                 j.ownerValue,
		Arm:                        j.arm,
		CurrentState:               j.current,
		ReadingArm:                 j.readingArm,
		ReadingStarted:             j.readingStart,
		ReadingAssetAfter:          j.readingAsset,
		ReadingStateAfter:          j.readingState,
		ReadingSourceAfter:         j.readingSrc,
		IdentityAfter:              j.identity,
		LocalAccount:               j.localAccount,
		LocalProfile:               j.localProfile,
		LocalMembershipIncarnation: j.localMembership,
		LocalProfileLifetime:       j.localLifetime,
	}
}

// captureRetainedTitleStepTx advances ONE bounded retention quantum for the
// title named by m. It returns done=true only when no retention owner or
// per-owner input remains. It never performs the title's physical removal.
func captureRetainedTitleStepTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation) (done bool, err error) {
	if tx == nil {
		return false, errRetainedTitleState
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// The parent checked attachment/source at admission. This is the required
	// point recheck of the actual mutation root as it exists in this writer.
	root, err := readRetainedCaptureRootTx(ctx, tx, m)
	if err != nil {
		return false, err
	}
	if root == nil {
		return false, errRetainedTitleRoot
	}

	job, err := readRetainedTitleJobTx(ctx, tx, m.Ref.ID)
	if err != nil {
		return false, err
	}
	if job == nil {
		return initializeRetainedTitleTx(ctx, tx, m)
	}
	if job.generation != m.Ref.Generation {
		return false, errRetainedTitleRoot
	}
	if job.plan == "" {
		return adoptRetainedTitleTx(ctx, tx, m, job)
	}
	if job.plan != retainedTitleDrainPlanDigest {
		return false, errRetainedTitlePlan
	}

	// No selected owner: this is the first selection, or a profile-erasure or
	// plan reset returned the durable frontier to its head.
	if job.ownerValue == "" {
		if job.ownerStarted || job.arm != retainedCaptureArmStart || job.current.Valid {
			return false, errRetainedTitleState
		}
		return selectRetainedTitleOwnerTx(ctx, tx, m, job)
	}
	if job.ownerAfter != job.ownerValue {
		return false, errRetainedTitleState
	}

	switch {
	case job.arm == retainedCaptureArmPublished || job.arm == retainedCaptureArmFinished:
		if job.ownerStarted {
			return false, errRetainedTitleState
		}
		return drainRetainedTitleOwnerTx(ctx, tx, m, job)
	case job.arm == retainedCaptureArmReset ||
		job.arm == retainedCaptureArmStart || job.arm == retainedCaptureArmHeader ||
		job.arm == retainedCaptureArmCurrent || job.arm == retainedCaptureArmRetained ||
		job.arm == retainedCaptureArmPublish:
		if !job.ownerStarted {
			return false, errRetainedTitleState
		}
		if _, err = captureRetainedOwnerStepTx(ctx, tx, m); err != nil {
			return false, err
		}
		return false, nil
	default:
		return false, errRetainedTitleState
	}
}

// initializeRetainedTitleTx creates the actual job and its <=12 restoration
// names in one bounded write. The names are read before the job/identity
// allocation so a stale title cannot leave a frontier without its oracle.
func initializeRetainedTitleTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation) (bool, error) {
	identities, _, _, err := retainedTitleIdentitiesTx(ctx, tx, m.Entity)
	if err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_purge_jobs(
 mutation_id,generation,plan_digest,state,owner_started,capture_arm,cursor)
 VALUES(?,?,?,'running',0,0,'')`, m.Ref.ID, m.Ref.Generation, retainedTitleDrainPlanDigest); err != nil {
		return false, fmt.Errorf("retained title walk job: %w", err)
	}
	for i, identity := range identities {
		if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_purge_identities(mutation_id,ordinal,identity) VALUES(?,?,?)`,
			m.Ref.ID, i+1, identity); err != nil {
			return false, fmt.Errorf("retained title walk identity: %w", err)
		}
	}
	return false, prepareRetainedTitleTombstoneTx(ctx, tx, m.Entity)
}

// adoptRetainedTitleTx completes a bare caller-created job. It refuses any
// half-written frontier and only adopts names that equal this file's bounded
// oracle exactly.
func adoptRetainedTitleTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation, job *retainedTitleJob) (bool, error) {
	if job.ownerStarted || job.ownerAfter != "" || job.ownerValue != "" || job.arm != retainedCaptureArmStart ||
		job.current.Valid || job.identity != 0 || job.readingStart || job.readingAsset != 0 ||
		job.readingState != 0 || job.readingSrc != "" || job.readingArm != "current" || job.cursor != "" {
		return false, errRetainedTitleState
	}
	oracle, _, _, err := retainedTitleIdentitiesTx(ctx, tx, m.Entity)
	if err != nil {
		return false, err
	}
	existing, err := readRetainedTitleIdentitiesTx(ctx, tx, m.Ref.ID)
	if err != nil {
		return false, err
	}
	if len(existing) == 0 {
		for i, identity := range oracle {
			if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_purge_identities(mutation_id,ordinal,identity) VALUES(?,?,?)`,
				m.Ref.ID, i+1, identity); err != nil {
				return false, fmt.Errorf("retained title walk identity: %w", err)
			}
		}
	} else if !sameRetainedTitleIdentities(existing, oracle) {
		return false, errRetainedTitleState
	}
	if _, err = tx.ExecContext(ctx, `UPDATE catalog_purge_jobs SET plan_digest=?,state='running',reason=''
 WHERE mutation_id=? AND generation=? AND plan_digest=''`, retainedTitleDrainPlanDigest, m.Ref.ID, m.Ref.Generation); err != nil {
		return false, fmt.Errorf("retained title walk adopt: %w", err)
	}
	return false, prepareRetainedTitleTombstoneTx(ctx, tx, m.Entity)
}

func readRetainedTitleIdentitiesTx(ctx context.Context, tx *sql.Tx, mutation string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT ordinal,identity FROM catalog_purge_identities
 WHERE mutation_id=? ORDER BY ordinal LIMIT 13`, mutation)
	if err != nil {
		return nil, fmt.Errorf("retained title walk identities: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ordinal int
		var identity string
		if err = rows.Scan(&ordinal, &identity); err != nil {
			return nil, err
		}
		if ordinal != len(out)+1 || len(out) >= retainedIdentityLimit {
			return nil, errRetainedTitleState
		}
		out = append(out, identity)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func sameRetainedTitleIdentities(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// retainedTitleIdentitiesTx is the bounded form of retentionIdentitiesTx. It
// preserves the old oracle's provider-first order and its first 12 local names,
// but never calls the unbounded IdentitiesTx. Duplicates inside that raw window
// are collapsed because catalog_purge_identities is UNIQUE(mutation_id,identity).
func retainedTitleIdentitiesTx(ctx context.Context, tx *sql.Tx, entity int64) ([]string, compactcatalog.Kind, string, error) {
	var kind compactcatalog.Kind
	var title, root string
	if err := tx.QueryRowContext(ctx, `SELECT e.kind,e.title,l.root
 FROM catalog_entities e JOIN catalog_libraries l ON l.id=e.library_id WHERE e.id=?`, entity).
		Scan(&kind, &title, &root); err != nil {
		return nil, 0, "", err
	}
	out := make([]string, 0, retainedIdentityLimit)
	seen := make(map[string]struct{}, retainedIdentityLimit)
	add := func(key string) {
		if key == "" || len(out) >= retainedIdentityLimit {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	addProviders := func(source int64, place string) error {
		rows, err := tx.QueryContext(ctx, retainedTitleProviderSQL, source)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var provider, providerKind, id string
			if err = rows.Scan(&provider, &providerKind, &id); err != nil {
				return err
			}
			add(compactcatalog.ProviderKey(provider, providerKind, id) + place)
		}
		return rows.Err()
	}
	switch kind {
	case compactcatalog.Episode:
		var show int64
		var numbering string
		var number, seasonNumber int
		var season sql.NullInt64
		err := tx.QueryRowContext(ctx, `SELECT ep.show_id,ep.numbering,ep.number,(SELECT number FROM catalog_seasons WHERE entity_id=ep.season_id)
 FROM catalog_episodes ep WHERE ep.entity_id=?`, entity).Scan(&show, &numbering, &number, &season)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, 0, "", err
		}
		if err == nil {
			seasonNumber = int(nullSeason(season))
			if err = addProviders(show, fmt.Sprintf("#%s/%d/%d", numbering, seasonNumber, number)); err != nil {
				return nil, 0, "", err
			}
		}
	case compactcatalog.Season:
		var show int64
		var number int
		err := tx.QueryRowContext(ctx, `SELECT show_id,number FROM catalog_seasons WHERE entity_id=?`, entity).Scan(&show, &number)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, 0, "", err
		}
		if err == nil {
			if err = addProviders(show, fmt.Sprintf("#season/%d", number)); err != nil {
				return nil, 0, "", err
			}
		}
	case compactcatalog.Track:
		var recording string
		if err := tx.QueryRowContext(ctx, `SELECT recording_id FROM catalog_songs WHERE entity_id=?`, entity).Scan(&recording); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, 0, "", err
		}
		if recording != "" {
			add(compactcatalog.ProviderKey("musicbrainz", "recording", recording))
		}
	default:
		if err := addProviders(entity, ""); err != nil {
			return nil, 0, "", err
		}
	}
	remaining := retainedIdentityLimit - len(out)
	if remaining > 0 {
		rows, err := tx.QueryContext(ctx, retainedTitleLocalSQL, entity, remaining)
		if err != nil {
			return nil, 0, "", err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err = rows.Scan(&key); err != nil {
				return nil, 0, "", err
			}
			add("local:" + root + "|" + key)
		}
		if err = rows.Err(); err != nil {
			return nil, 0, "", err
		}
	}
	return out, kind, title, nil
}

// selectRetainedTitleOwnerTx advances the durable owner frontier to the next
// raw candidate. Each of the five per-title sources and the four container
// defaults contributes at most one raw key through an index prefix; the merge
// keeps the smallest key greater than owner_after. Semantic retainability is
// deliberately not a SQL filter here: captureRetainedOwnerStepTx owns it.
func selectRetainedTitleOwnerTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation, job *retainedTitleJob) (bool, error) {
	var next string
	for _, q := range retainedTitleOwnerSQL {
		var candidate string
		err := tx.QueryRowContext(ctx, q, m.Entity, job.ownerAfter).Scan(&candidate)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("retained title walk owner candidate: %w", err)
		}
		if candidate != "" && (next == "" || candidate < next) {
			next = candidate
		}
	}
	if next == "" {
		return true, nil
	}
	account, profile, membership, lifetime, err := retainedTitleBindingTx(ctx, tx, next)
	if err != nil {
		return false, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE catalog_purge_jobs SET
 owner_after=?,owner_value=?,owner_started=1,
 local_account=?,local_profile=?,local_membership_incarnation=?,local_profile_lifetime=?,
 capture_arm=0,current_state_id=NULL,reading_arm='current',reading_started=0,
 reading_asset_after=0,reading_state_after=0,reading_source_after='',identity_after=0,
 cursor='',state='running',attempt=0,next_attempt_ms=0,reason=''
 WHERE mutation_id=? AND generation=? AND plan_digest=? AND owner_started=0 AND owner_after=? AND owner_value=?`,
		next, next, account, profile, membership, lifetime,
		m.Ref.ID, m.Ref.Generation, retainedTitleDrainPlanDigest, job.ownerAfter, job.ownerValue)
	if err != nil {
		return false, fmt.Errorf("retained title walk select owner: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return false, errRetainedTitleState
	}
	return false, nil
}

// retainedTitleBindingTx returns the four exact local scalars only for a
// canonical local owner tuple. A truly Hosted/legacy value stays opaque with
// empty scalars. A local tuple whose direct profile is gone keeps the local
// shape (empty scalars), so the capture component's binding reader reports Dead
// rather than silently retagging it as Hosted.
func retainedTitleBindingTx(ctx context.Context, tx *sql.Tx, owner string) (account, profile, membership, lifetime string, err error) {
	encoded, ok := strings.CutPrefix(owner, "viewer:")
	if !ok {
		return "", "", "", "", nil
	}
	raw, decodeErr := base64.RawURLEncoding.DecodeString(encoded)
	if decodeErr != nil {
		return "", "", "", "", nil
	}
	var tuple []string
	if json.Unmarshal(raw, &tuple) != nil || len(tuple) != 3 || tuple[0] != "local" {
		return "", "", "", "", nil
	}
	account, profile = tuple[1], tuple[2]
	if account == "" || profile == "" {
		return "", "", "", "", nil
	}
	var incarnation string
	err = tx.QueryRowContext(ctx, `SELECT p.lifetime,m.incarnation
 FROM direct_profiles p JOIN direct_memberships m ON m.account_id=p.account_id
 WHERE p.account_id=? AND p.id=?`, account, profile).Scan(&lifetime, &incarnation)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", "", nil
	}
	if err != nil {
		return "", "", "", "", fmt.Errorf("retained title walk binding: %w", err)
	}
	return account, profile, incarnation, lifetime, nil
}

// drainRetainedTitleOwnerTx runs one raw PK child deletion for the current
// owner through the shared phasedpurge executor, or advances to the next owner
// when the durable cursor says this owner's inputs are exhausted.
func drainRetainedTitleOwnerTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation, job *retainedTitleJob) (bool, error) {
	allowViewer := true
	plan := retainedTitleDrainPlan(job.ownerValue, m.Entity, &allowViewer)
	cursor := phasedpurge.Cursor{}
	if job.cursor != "" {
		decoded, err := phasedpurge.DecodeCursor(job.cursor, plan)
		if err != nil {
			return false, fmt.Errorf("%w: %v", errRetainedTitleCursor, err)
		}
		cursor = decoded
	}
	if job.reason == retainedTitleOwnerDrained {
		if cursor.Unit != len(plan) || cursor.Verify != 0 || len(cursor.Key) != 0 {
			return false, errRetainedTitleCursor
		}
		// Only Finalize writes this marker, after all Empty probes pass. A
		// Unit==len(plan) cursor alone merely starts the verification phase.
		return selectRetainedTitleOwnerTx(ctx, tx, m, job)
	}

	guard := retainedTitleDrainGuard{
		mutation:   m.Ref.ID,
		generation: m.Ref.Generation,
		plan:       retainedTitleDrainPlanDigest,
		ownerAfter: job.ownerAfter,
		ownerValue: job.ownerValue,
	}
	owner := phasedpurge.Owner{
		ReadBound: retainedTitleOwnerReadBound,
		Before: func(bctx context.Context, btx *sql.Tx) error {
			recheck, err := readRetainedCaptureRootTx(bctx, btx, m)
			if err != nil {
				return err
			}
			if recheck == nil {
				return errRetainedTitleRoot
			}
			binding, err := readRetainedCaptureBindingTx(bctx, btx, job.captureJob())
			if err != nil {
				return err
			}
			switch binding.state {
			case retainedCaptureBindingValid, retainedCaptureBindingOpaque, retainedCaptureBindingDead:
				// Dead is a destroyed/cleared local profile: capture already
				// refused to copy it, and deleting its removed-title inputs is
				// safe. A rotated lifetime is refused above, never drained.
				allowViewer = binding.state != retainedCaptureBindingDead
				return nil
			default:
				return errRetainedTitleBinding
			}
		},
		Save: func(sctx context.Context, stx *sql.Tx, c phasedpurge.Cursor) error {
			return guard.saveTx(sctx, stx, c)
		},
		Finalize: func(fctx context.Context, ftx *sql.Tx) error {
			return guard.saveTx(fctx, ftx, phasedpurge.Cursor{Unit: len(plan)}, true)
		},
	}
	if _, err := phasedpurge.Turn(ctx, tx, plan, cursor, owner); err != nil {
		switch {
		case errors.Is(err, phasedpurge.ErrCursor):
			return false, fmt.Errorf("%w: %v", errRetainedTitleCursor, err)
		case errors.Is(err, phasedpurge.ErrPlan), errors.Is(err, phasedpurge.ErrEffects),
			errors.Is(err, phasedpurge.ErrReads), errors.Is(err, phasedpurge.ErrNoProgress):
			return false, fmt.Errorf("%w: %v", errRetainedTitlePlan, err)
		default:
			return false, err
		}
	}
	return false, nil
}

type retainedTitleDrainGuard struct {
	mutation, plan, ownerAfter, ownerValue string
	generation                             int64
}

const retainedTitleSaveCursorSQL = `UPDATE catalog_purge_jobs SET cursor=?,reason=CASE WHEN ? THEN ? ELSE reason END
 WHERE mutation_id=? AND generation=? AND plan_digest=? AND owner_started=0
 AND owner_after=? AND owner_value=? AND capture_arm IN(?,?)`

func (g retainedTitleDrainGuard) saveTx(ctx context.Context, tx *sql.Tx, c phasedpurge.Cursor, final ...bool) error {
	raw, err := phasedpurge.EncodeCursor(c)
	if err != nil {
		return fmt.Errorf("%w: %v", errRetainedTitleCursor, err)
	}
	complete := len(final) != 0 && final[0]
	res, err := tx.ExecContext(ctx, retainedTitleSaveCursorSQL,
		raw, complete, retainedTitleOwnerDrained, g.mutation, g.generation, g.plan, g.ownerAfter, g.ownerValue,
		retainedCaptureArmPublished, retainedCaptureArmFinished)
	if err != nil {
		return fmt.Errorf("retained title walk cursor: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return errRetainedTitleState
	}
	return nil
}

// retainedTitleDrainUnit is one fixed literal unit. The drain cursor only ever
// carries the Reading asset key; the owner and title are exact predicates.
type retainedTitleDrainUnit struct {
	phase     int
	name      string
	readBound int
	keyKinds  []phasedpurge.Kind
	pick      string
	delete    string
	probe     string
}

const (
	retainedTitleNextPersonalItemsSQL = `SELECT profile_id FROM personal_items INDEXED BY personal_items_retention_owner
 WHERE item_id=? AND profile_id>? ORDER BY profile_id LIMIT 1`
	retainedTitleNextProgressSQL = `SELECT profile_id FROM progress INDEXED BY progress_item_state
 WHERE item_id=? AND profile_id>? ORDER BY profile_id LIMIT 1`
	retainedTitleNextReadingSQL = `SELECT owner_key FROM reading_progress INDEXED BY reading_progress_retention_owner
 WHERE item_id=? AND owner_key>? ORDER BY owner_key LIMIT 1`
	retainedTitleNextWatchedSQL = `SELECT profile_id FROM personal_watched_intents INDEXED BY personal_watched_intents_retention_owner
 WHERE item_id=? AND profile_id>? ORDER BY profile_id LIMIT 1`
	retainedTitleNextPlaysSQL = `SELECT profile_id FROM personal_play_counts INDEXED BY personal_play_counts_item
 WHERE item_id=? AND profile_id>? ORDER BY profile_id LIMIT 1`
	retainedTitleNextHistorySQL = `SELECT profile_id FROM personal_history INDEXED BY personal_history_retention_owner
 WHERE item_id=? AND profile_id>? ORDER BY profile_id LIMIT 1`

	retainedTitleNextShowDefaultSQL = `SELECT s.profile_id FROM container_personal_state s INDEXED BY container_personal_state_retention_owner
 WHERE s.container_id=(SELECT show_id FROM catalog_episodes WHERE entity_id=?) AND s.profile_id>?
 ORDER BY s.profile_id LIMIT 1`
	retainedTitleNextSeasonDefaultSQL = `SELECT s.profile_id FROM container_personal_state s INDEXED BY container_personal_state_retention_owner
 WHERE s.container_id=(SELECT season_id FROM catalog_episodes WHERE entity_id=?) AND s.profile_id>?
 ORDER BY s.profile_id LIMIT 1`
	retainedTitleNextAlbumDefaultSQL = `SELECT s.profile_id FROM container_personal_state s INDEXED BY container_personal_state_retention_owner
 WHERE s.container_id=(SELECT album_id FROM catalog_songs WHERE entity_id=?) AND s.profile_id>?
 ORDER BY s.profile_id LIMIT 1`
	retainedTitleNextBookDefaultSQL = `SELECT s.profile_id FROM container_personal_state s INDEXED BY container_personal_state_retention_owner
 WHERE s.container_id=(SELECT book_id FROM catalog_book_files WHERE entity_id=?) AND s.profile_id>?
 ORDER BY s.profile_id LIMIT 1`

	retainedTitleProviderSQL = `SELECT provider,provider_kind,provider_id FROM catalog_external_ids
 WHERE entity_id=? ORDER BY provider,provider_kind,provider_id LIMIT 6`
	retainedTitleLocalSQL = `SELECT i.source_key FROM catalog_entities e
 JOIN catalog_identities i INDEXED BY catalog_identities_retention ON i.public_id=e.public_id
 WHERE e.id=? ORDER BY i.source_key LIMIT ?`

	retainedTitleDeletePersonalItemsSQL = `DELETE FROM personal_items WHERE profile_id=? AND item_id=?`
	retainedTitleDeleteProgressSQL      = `DELETE FROM progress WHERE profile_id=? AND item_id=?`
	retainedTitleDeleteWatchedSQL       = `DELETE FROM personal_watched_intents WHERE profile_id=? AND item_id=?`
	retainedTitleDeletePlaysSQL         = `DELETE FROM personal_play_counts WHERE profile_id=? AND item_id=?`
	retainedTitleDeleteHistorySQL       = `DELETE FROM personal_history WHERE rowid=(SELECT rowid FROM personal_history INDEXED BY personal_history_retention_owner WHERE profile_id=? AND item_id=? LIMIT 1)`
	retainedTitlePickReadingSQL         = `SELECT asset_id FROM reading_progress INDEXED BY reading_progress_retention_owner
 WHERE item_id=? AND owner_key=?`
	retainedTitlePickReadingKeysetSQL = `SELECT asset_id FROM reading_progress INDEXED BY reading_progress_retention_owner
 WHERE item_id=? AND owner_key=? AND asset_id>? ORDER BY asset_id LIMIT 1`
	retainedTitleDeleteReadingSQL = `DELETE FROM reading_progress WHERE owner_key=? AND item_id=? AND asset_id=?`

	retainedTitleHasPersonalItemsSQL = `SELECT EXISTS(SELECT 1 FROM personal_items WHERE profile_id=? AND item_id=?)`
	retainedTitleHasProgressSQL      = `SELECT EXISTS(SELECT 1 FROM progress WHERE profile_id=? AND item_id=?)`
	retainedTitleHasWatchedSQL       = `SELECT EXISTS(SELECT 1 FROM personal_watched_intents WHERE profile_id=? AND item_id=?)`
	retainedTitleHasPlaysSQL         = `SELECT EXISTS(SELECT 1 FROM personal_play_counts WHERE profile_id=? AND item_id=?)`
	retainedTitleHasHistorySQL       = `SELECT EXISTS(SELECT 1 FROM personal_history INDEXED BY personal_history_retention_owner WHERE profile_id=? AND item_id=?)`
	retainedTitleHasReadingSQL       = `SELECT EXISTS(SELECT 1 FROM reading_progress WHERE owner_key=? AND item_id=?)`
)

var retainedTitleOwnerSQL = [...]string{
	retainedTitleNextPersonalItemsSQL,
	retainedTitleNextProgressSQL,
	retainedTitleNextReadingSQL,
	retainedTitleNextWatchedSQL,
	retainedTitleNextPlaysSQL,
	retainedTitleNextHistorySQL,
	retainedTitleNextShowDefaultSQL,
	retainedTitleNextSeasonDefaultSQL,
	retainedTitleNextAlbumDefaultSQL,
	retainedTitleNextBookDefaultSQL,
}

var retainedTitleDrainUnits = [...]retainedTitleDrainUnit{
	{phase: 0, name: "tombstone-progress", readBound: 5,
		delete: retainedTitleTombstoneProgressSQL, probe: retainedTitleTombstoneProgressMissingSQL},
	{phase: 0, name: "tombstone-history", readBound: 4,
		delete: retainedTitleTombstoneHistorySQL, probe: retainedTitleTombstoneHistoryMissingSQL},
	{phase: 0, name: "tombstone-favourite", readBound: 4,
		delete: retainedTitleTombstoneFavouriteSQL, probe: retainedTitleTombstoneFavouriteMissingSQL},
	// The point-delete closure includes recommendation work/epoch, bounded
	// viewer/history revision counters and API hints. Progress additionally
	// cascades to at most one composite-PK activity row, whose own partial row
	// and viewer hint are point writes. The current widest literal closure has
	// fewer than24 point probes; reserve24 for every source delete. The pinned
	// trigger/FK census test requires re-derivation when that closure changes.
	{phase: 0, name: "personal-items", readBound: 24,
		delete: retainedTitleDeletePersonalItemsSQL, probe: retainedTitleHasPersonalItemsSQL},
	{phase: 1, name: "progress", readBound: 24,
		delete: retainedTitleDeleteProgressSQL, probe: retainedTitleHasProgressSQL},
	{phase: 2, name: "watched-intents", readBound: 24,
		delete: retainedTitleDeleteWatchedSQL, probe: retainedTitleHasWatchedSQL},
	{phase: 3, name: "play-counts", readBound: 24,
		delete: retainedTitleDeletePlaysSQL, probe: retainedTitleHasPlaysSQL},
	{phase: 4, name: "reading-progress", readBound: 24,
		keyKinds: []phasedpurge.Kind{phasedpurge.Integer},
		pick:     retainedTitlePickReadingSQL, delete: retainedTitleDeleteReadingSQL, probe: retainedTitleHasReadingSQL},
	{phase: 5, name: "personal-history", readBound: 24,
		delete: retainedTitleDeleteHistorySQL, probe: retainedTitleHasHistorySQL},
}

// retainedTitleDrainPlanDigest is the deterministic identity of the complete
// walk: every fixed literal, unit phase/bound/keys, the exact owner and binding
// probes, the identity oracle and the shared engine limits. A restart of this
// binary accepts its own cursor; any other plan is refused, never restarted.
var retainedTitleDrainPlanDigest = func() string {
	h := sha256.New()
	write := func(parts ...string) {
		for _, part := range parts {
			fmt.Fprintf(h, "%d:%s|", len(part), part)
		}
		fmt.Fprint(h, "\n")
	}
	for i := range retainedTitleDrainUnits {
		u := &retainedTitleDrainUnits[i]
		kinds := make([]string, len(u.keyKinds))
		for k, kind := range u.keyKinds {
			kinds[k] = string(kind)
		}
		write("unit", u.name, strconv.Itoa(u.phase), strconv.Itoa(u.readBound),
			strings.Join(kinds, ","), u.pick, u.delete, u.probe)
		if u.pick != "" {
			write("pick", u.pick+" ORDER BY asset_id LIMIT 1")
			write("keyset", u.pick+" AND asset_id>? ORDER BY asset_id LIMIT 1")
		}
	}
	for _, q := range retainedTitleOwnerSQL {
		write("owner", q)
	}
	write("identity-provider", retainedTitleProviderSQL)
	write("identity-local", retainedTitleLocalSQL)
	write("owner-save", retainedTitleSaveCursorSQL)
	write("owner-bound", strconv.Itoa(retainedTitleOwnerReadBound))
	write("engine", strconv.Itoa(phasedpurge.EffectLimit), strconv.Itoa(phasedpurge.ReadLimit))
	write("version", retainedTitleWalkVersion)
	return hex.EncodeToString(h.Sum(nil))
}()

// RetainedTitleWalkPlanDigest exposes the compiled walk identity to the parent
// that owns job admission and visible blocking.
func RetainedTitleWalkPlanDigest() string { return retainedTitleDrainPlanDigest }

func retainedTitleDrainPlan(owner string, item int64, viewerPermit ...*bool) []phasedpurge.Unit {
	allowed := func() bool { return len(viewerPermit) == 0 || *viewerPermit[0] }
	plan := make([]phasedpurge.Unit, len(retainedTitleDrainUnits))
	for i := range retainedTitleDrainUnits {
		u := &retainedTitleDrainUnits[i]
		plan[i] = phasedpurge.Unit{
			Phase:     u.phase,
			Name:      u.name,
			KeyKinds:  u.keyKinds,
			ReadBound: u.readBound,
			Run: func(ctx context.Context, tx *sql.Tx, key []phasedpurge.Key) (phasedpurge.Outcome, error) {
				if strings.HasPrefix(u.name, "tombstone-") && !allowed() {
					return phasedpurge.Outcome{Drained: true}, nil
				}
				if u.name != "reading-progress" {
					res, err := tx.ExecContext(ctx, u.delete, owner, item)
					if err != nil {
						return phasedpurge.Outcome{}, fmt.Errorf("retained title drain %s: %w", u.name, err)
					}
					if n, _ := res.RowsAffected(); n == 0 {
						return phasedpurge.Outcome{Drained: true}, nil
					}
					if u.name == "personal-history" {
						return phasedpurge.Outcome{Changed: true}, nil
					}
					return phasedpurge.Outcome{Drained: true, Changed: true}, nil
				}
				args := []any{item, owner}
				query := u.pick + " ORDER BY asset_id LIMIT 1"
				if len(key) > 0 {
					values, err := phasedpurge.SQLKeys(key)
					if err != nil {
						return phasedpurge.Outcome{}, fmt.Errorf("%w: %v", errRetainedTitleCursor, err)
					}
					query = retainedTitlePickReadingKeysetSQL
					args = append(args, values[0])
				}
				var asset int64
				err := tx.QueryRowContext(ctx, query, args...).Scan(&asset)
				if errors.Is(err, sql.ErrNoRows) {
					return phasedpurge.Outcome{Drained: true}, nil
				}
				if err != nil {
					return phasedpurge.Outcome{}, fmt.Errorf("retained title drain reading: %w", err)
				}
				res, err := tx.ExecContext(ctx, u.delete, owner, item, asset)
				if err != nil {
					return phasedpurge.Outcome{}, fmt.Errorf("retained title drain reading: %w", err)
				}
				if n, _ := res.RowsAffected(); n != 1 {
					return phasedpurge.Outcome{}, errRetainedTitleState
				}
				next, err := phasedpurge.Keys(asset)
				if err != nil {
					return phasedpurge.Outcome{}, fmt.Errorf("%w: %v", errRetainedTitleCursor, err)
				}
				return phasedpurge.Outcome{Changed: true, Next: next}, nil
			},
			Empty: func(ctx context.Context, tx *sql.Tx) (bool, error) {
				if strings.HasPrefix(u.name, "tombstone-") && !allowed() {
					return true, nil
				}
				var found bool
				if err := tx.QueryRowContext(ctx, u.probe, owner, item).Scan(&found); err != nil {
					return false, fmt.Errorf("retained title probe %s: %w", u.name, err)
				}
				return !found, nil
			},
		}
	}
	return plan
}
