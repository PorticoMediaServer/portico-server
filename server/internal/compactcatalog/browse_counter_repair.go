package compactcatalog

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Browse counter repair (P2-64 B/B1). The maintained counters are
// catalog_browse_item_metrics.available_links/unavailable_links over an item's
// catalog_asset_links rows, and catalog_browse_member_links.available/
// unavailable/members over a container's applied member metrics and
// memberships. A startup sample reads only a bounded prefix of each holder's
// raw rows: eight rows can be counted exactly, a ninth only proves there are
// more, so a large holder is checked for a negative counter or one below the
// observed lower bound. Detected drift queues the ordinary repair (Domain25 for
// an item) or a bounded counter repair row (Domain35 for a container).
const (
	// browseCounterSample is the candidates of each kind one startup sample
	// checks: the sample is independent of the catalogue's size.
	browseCounterSample = 8
	// browseCounterProbe is the raw rows one probe reads. Eight rows give an
	// exact count; the ninth says only "more than eight".
	browseCounterProbe = 9
	// browseRepairPage is the raw rows one repair call applies.
	browseRepairPage = 8
)

// SampleBrowseCounterIntegrityTx samples at most eight item holders and eight
// container holders by primary key, starting from a bounded timestamp seed so
// restarts visit different rows. A healthy holder is only read; drift queues
// Domain25 for an item or a catalog_browse_counter_repairs row plus Domain35
// for a container.
func SampleBrowseCounterIntegrityTx(ctx context.Context, tx *sql.Tx) error {
	seed := time.Now().Unix()
	if err := sampleBrowseItemCountersTx(ctx, tx, seed); err != nil {
		return err
	}
	if err := sampleBrowseContainerCountersTx(ctx, tx, seed); err != nil {
		return err
	}
	return sampleBrowseRepairCleanupTx(ctx, tx, seed)
}

// sampleBrowseRepairCleanupTx drops at most eight repair rows whose container
// was deleted: the row has no foreign key, and its dirty key may be gone too.
func sampleBrowseRepairCleanupTx(ctx context.Context, tx *sql.Tx, seed int64) error {
	repairs, err := sampleBrowseIDs(ctx, tx, seed,
		`SELECT COALESCE(max(entity_id),0) FROM catalog_browse_counter_repairs`,
		`SELECT entity_id FROM catalog_browse_counter_repairs WHERE entity_id>=? ORDER BY entity_id LIMIT ?`,
		`SELECT entity_id FROM catalog_browse_counter_repairs WHERE entity_id<? ORDER BY entity_id DESC LIMIT ?`)
	if err != nil {
		return err
	}
	for _, id := range repairs {
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_entities WHERE id=?)`, id).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_browse_counter_repairs WHERE entity_id=?`, id); err != nil {
			return err
		}
	}
	return nil
}

func sampleBrowseItemCountersTx(ctx context.Context, tx *sql.Tx, seed int64) error {
	items, err := sampleBrowseIDs(ctx, tx, seed,
		`SELECT COALESCE(max(item_id),0) FROM catalog_browse_item_metrics`,
		`SELECT item_id FROM catalog_browse_item_metrics WHERE item_id>=? ORDER BY item_id LIMIT ?`,
		`SELECT item_id FROM catalog_browse_item_metrics WHERE item_id<? ORDER BY item_id DESC LIMIT ?`)
	if err != nil {
		return err
	}
	for _, item := range items {
		drift, err := browseItemCounterDriftedTx(ctx, tx, item)
		if err != nil {
			return err
		}
		if drift {
			if err = TouchTx(ctx, tx, DomainItemMetrics, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func sampleBrowseContainerCountersTx(ctx context.Context, tx *sql.Tx, seed int64) error {
	containers, err := sampleBrowseIDs(ctx, tx, seed,
		`SELECT COALESCE(max(entity_id),0) FROM catalog_browse_member_links`,
		`SELECT entity_id FROM catalog_browse_member_links WHERE entity_id>=? ORDER BY entity_id LIMIT ?`,
		`SELECT entity_id FROM catalog_browse_member_links WHERE entity_id<? ORDER BY entity_id DESC LIMIT ?`)
	if err != nil {
		return err
	}
	for _, container := range containers {
		drift, err := browseContainerCounterDriftedTx(ctx, tx, container)
		if err != nil {
			return err
		}
		if !drift {
			continue
		}
		// Keep an interrupted repair's progress; its revision check restarts
		// it if the container changed again.
		if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_browse_counter_repairs(entity_id) VALUES(?) ON CONFLICT(entity_id) DO NOTHING`, container); err != nil {
			return err
		}
		if err = TouchTx(ctx, tx, DomainContainerRefresh, container); err != nil {
			return err
		}
	}
	return nil
}

// sampleBrowseIDs reads at most browseCounterSample primary keys around a
// seeded start, wrapping once, with two bounded index ranges and no count.
func sampleBrowseIDs(ctx context.Context, tx *sql.Tx, seed int64, maxSQL, afterSQL, beforeSQL string) ([]int64, error) {
	var max int64
	if err := tx.QueryRowContext(ctx, maxSQL).Scan(&max); err != nil {
		return nil, err
	}
	if max <= 0 {
		return nil, nil
	}
	start := seed % (max + 1)
	ids, err := scanIDs(ctx, tx, afterSQL, start, browseCounterSample)
	if err != nil {
		return nil, err
	}
	if len(ids) < browseCounterSample && start > 0 {
		before, err := scanIDs(ctx, tx, beforeSQL, start, browseCounterSample-len(ids))
		if err != nil {
			return nil, err
		}
		for i, j := 0, len(before)-1; i < j; i, j = i+1, j-1 {
			before[i], before[j] = before[j], before[i]
		}
		ids = append(ids, before...)
	}
	return ids, nil
}

// browseItemCounterDriftedTx compares one item's counters with at most nine of
// its raw links. A negative counter is drift. With fewer than nine links the
// comparison is exact; with nine it is a lower-bound test.
func browseItemCounterDriftedTx(ctx context.Context, tx *sql.Tx, item int64) (bool, error) {
	var available, unavailable int64
	err := tx.QueryRowContext(ctx, `SELECT available_links,unavailable_links FROM catalog_browse_item_metrics WHERE item_id=?`, item).Scan(&available, &unavailable)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if available < 0 || unavailable < 0 {
		return true, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT available FROM catalog_asset_links WHERE entity_id=? ORDER BY asset_id LIMIT ?`, item, browseCounterProbe)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	var availableNow, unavailableNow int64
	probed := 0
	for rows.Next() {
		var value int64
		if err = rows.Scan(&value); err != nil {
			return false, err
		}
		probed++
		if value == 1 {
			availableNow++
		} else {
			unavailableNow++
		}
	}
	if err = rows.Err(); err != nil {
		return false, err
	}
	if probed < browseCounterProbe {
		return available != availableNow || unavailable != unavailableNow, nil
	}
	return available < availableNow || unavailable < unavailableNow, nil
}

// browseContainerCounterDriftedTx compares one container's counters with at
// most nine applied member metrics and at most nine memberships. The same exact
// or lower-bound rule applies to each side.
func browseContainerCounterDriftedTx(ctx context.Context, tx *sql.Tx, container int64) (bool, error) {
	var available, unavailable, members int64
	err := tx.QueryRowContext(ctx, `SELECT available,unavailable,members FROM catalog_browse_member_links WHERE entity_id=?`, container).Scan(&available, &unavailable, &members)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if available < 0 || unavailable < 0 || members < 0 {
		return true, nil
	}
	metrics, err := tx.QueryContext(ctx, `SELECT available_links,unavailable_links FROM catalog_browse_member_metrics WHERE entity_id=? ORDER BY item_id,source LIMIT ?`, container, browseCounterProbe)
	if err != nil {
		return false, err
	}
	var availableNow, unavailableNow int64
	probed := 0
	for metrics.Next() {
		var a, u int64
		if err = metrics.Scan(&a, &u); err != nil {
			metrics.Close()
			return false, err
		}
		probed++
		availableNow += a
		unavailableNow += u
	}
	err = metrics.Err()
	metrics.Close()
	if err != nil {
		return false, err
	}
	if probed < browseCounterProbe {
		if available != availableNow || unavailable != unavailableNow {
			return true, nil
		}
	} else if available < availableNow || unavailable < unavailableNow {
		return true, nil
	}
	seen, err := tx.QueryContext(ctx, `SELECT item_id FROM catalog_browse_memberships WHERE entity_id=? ORDER BY item_id,source LIMIT ?`, container, browseCounterProbe)
	if err != nil {
		return false, err
	}
	membersNow := 0
	for seen.Next() {
		var item int64
		if err = seen.Scan(&item); err != nil {
			seen.Close()
			return false, err
		}
		membersNow++
	}
	err = seen.Err()
	seen.Close()
	if err != nil {
		return false, err
	}
	if membersNow < browseCounterProbe {
		return members != int64(membersNow), nil
	}
	return members < int64(membersNow), nil
}

// browseCounterRepair is one container's resumable repair cursor.
type browseCounterRepair struct {
	revision    int64
	phase       int
	afterItem   int64
	afterSource int
	available   int64
	unavailable int64
	members     int64
}

const (
	browseRepairMetrics = 0
	browseRepairMembers = 1
	browseRepairFinal   = 2
)

// RepairBrowseCountersTx advances one container's pending repair by at most
// limit (capped at eight) raw rows and reports whether it is still pending. A
// revision change restarts the cursor because the input changed. The final
// call replaces the container counters atomically and removes the repair row;
// the caller then runs the ordinary Domain35 derivation in the same
// transaction. A repair for a deleted container is removed without a write.
func RepairBrowseCountersTx(ctx context.Context, tx *sql.Tx, k Key, limit int) (bool, error) {
	repair, err := readBrowseCounterRepairTx(ctx, tx, k.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_entities WHERE id=?)`, k.ID).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		_, err = tx.ExecContext(ctx, `DELETE FROM catalog_browse_counter_repairs WHERE entity_id=?`, k.ID)
		return false, err
	}
	if repair.revision != k.Revision {
		repair = browseCounterRepair{revision: k.Revision, afterSource: -1}
	}
	if limit <= 0 || limit > browseRepairPage {
		limit = browseRepairPage
	}
	switch repair.phase {
	case browseRepairMetrics:
		full, err := repairBrowseCounterMetricsTx(ctx, tx, k.ID, &repair, limit)
		if err != nil {
			return false, err
		}
		if full {
			return true, saveBrowseCounterRepairTx(ctx, tx, k.ID, repair)
		}
		// One phase per call: the next call reads the memberships.
		repair.phase = browseRepairMembers
		repair.afterItem, repair.afterSource, repair.members = 0, -1, 0
		return true, saveBrowseCounterRepairTx(ctx, tx, k.ID, repair)
	case browseRepairMembers:
		full, err := repairBrowseCounterMembersTx(ctx, tx, k.ID, &repair, limit)
		if err != nil {
			return false, err
		}
		if full {
			return true, saveBrowseCounterRepairTx(ctx, tx, k.ID, repair)
		}
	}
	if err = finishBrowseCounterRepairTx(ctx, tx, k.ID, repair); err != nil {
		return false, err
	}
	return false, nil
}

func readBrowseCounterRepairTx(ctx context.Context, tx *sql.Tx, id int64) (browseCounterRepair, error) {
	var repair browseCounterRepair
	err := tx.QueryRowContext(ctx, `SELECT revision,phase,after_item,after_source,available,unavailable,members FROM catalog_browse_counter_repairs WHERE entity_id=?`, id).
		Scan(&repair.revision, &repair.phase, &repair.afterItem, &repair.afterSource, &repair.available, &repair.unavailable, &repair.members)
	return repair, err
}

func saveBrowseCounterRepairTx(ctx context.Context, tx *sql.Tx, id int64, repair browseCounterRepair) error {
	_, err := tx.ExecContext(ctx, `UPDATE catalog_browse_counter_repairs SET revision=?,phase=?,after_item=?,after_source=?,available=?,unavailable=?,members=? WHERE entity_id=?`,
		repair.revision, repair.phase, repair.afterItem, repair.afterSource, repair.available, repair.unavailable, repair.members, id)
	return err
}

// repairBrowseCounterMetricsTx sums at most limit applied member-metric
// contributions after the full (item_id,source) cursor.
func repairBrowseCounterMetricsTx(ctx context.Context, tx *sql.Tx, id int64, repair *browseCounterRepair, limit int) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT item_id,source,available_links,unavailable_links FROM catalog_browse_member_metrics WHERE entity_id=? AND (item_id,source)>(?,?) ORDER BY item_id,source LIMIT ?`,
		id, repair.afterItem, repair.afterSource, limit)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	read := 0
	var lastItem int64
	var lastSource int
	for rows.Next() {
		var item, available, unavailable int64
		var source int
		if err = rows.Scan(&item, &source, &available, &unavailable); err != nil {
			return false, err
		}
		read++
		lastItem, lastSource = item, source
		repair.available += available
		repair.unavailable += unavailable
	}
	if err = rows.Err(); err != nil {
		return false, err
	}
	if read < limit {
		return false, nil
	}
	repair.afterItem, repair.afterSource = lastItem, lastSource
	return true, nil
}

// repairBrowseCounterMembersTx counts at most limit memberships after the full
// (item_id,source) cursor.
func repairBrowseCounterMembersTx(ctx context.Context, tx *sql.Tx, id int64, repair *browseCounterRepair, limit int) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT item_id,source FROM catalog_browse_memberships WHERE entity_id=? AND (item_id,source)>(?,?) ORDER BY item_id,source LIMIT ?`,
		id, repair.afterItem, repair.afterSource, limit)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	read := 0
	var lastItem int64
	var lastSource int
	for rows.Next() {
		var item int64
		var source int
		if err = rows.Scan(&item, &source); err != nil {
			return false, err
		}
		read++
		lastItem, lastSource = item, source
		repair.members++
	}
	if err = rows.Err(); err != nil {
		return false, err
	}
	if read < limit {
		return false, nil
	}
	repair.afterItem, repair.afterSource = lastItem, lastSource
	return true, nil
}

// finishBrowseCounterRepairTx replaces the container's counters with the
// accumulated values and drops the repair row in the caller's transaction.
func finishBrowseCounterRepairTx(ctx context.Context, tx *sql.Tx, id int64, repair browseCounterRepair) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_browse_member_links(entity_id,available,unavailable,members) VALUES(?,?,?,?)
 ON CONFLICT(entity_id) DO UPDATE SET available=excluded.available,unavailable=excluded.unavailable,members=excluded.members`,
		id, repair.available, repair.unavailable, repair.members); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM catalog_browse_counter_repairs WHERE entity_id=?`, id)
	return err
}
