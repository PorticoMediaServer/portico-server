package app

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

func TestMetadataContinuationRuntimeWritesUsePriorityScheduler(t *testing.T) {
	server := newScannerTestServer(t)
	store := server.newPrioritizedMetadataContinuationStore()

	releaseGate, err := server.dbWriteScheduler.acquire(t.Context(), foundationcontract.WorkClassInteractive)
	if err != nil {
		t.Fatal(err)
	}
	continuationDone := make(chan error, 1)
	go func() {
		_, err := store.Start(context.Background(), MetadataContinuationStart{
			ID: "metadata_priority_continuation", RootKind: "movie", RootID: "movie_meridian",
			Provider: "tmdb", PolicyRevision: "policy-v1", ProviderRevision: "provider-v1",
			InitialPhase: "descendants",
		})
		continuationDone <- err
	}()
	waitForSQLiteSchedulerWaiters(t, server, foundationcontract.WorkClassBackgroundMedia, 1)

	interactiveEntered := make(chan struct{})
	releaseInteractive := make(chan struct{})
	interactiveDone := make(chan error, 1)
	go func() {
		interactiveDone <- server.withUserTxTagged(context.Background(), nil, func(*sql.Tx) error {
			close(interactiveEntered)
			<-releaseInteractive
			return nil
		})
	}()
	waitForSQLiteSchedulerWaiters(t, server, foundationcontract.WorkClassInteractive, 1)
	releaseGate()

	select {
	case <-interactiveEntered:
	case <-time.After(time.Second):
		t.Fatal("interactive transaction did not enter ahead of metadata continuation")
	}
	select {
	case err := <-continuationDone:
		close(releaseInteractive)
		t.Fatalf("metadata continuation bypassed the priority scheduler: %v", err)
	default:
	}
	close(releaseInteractive)
	if err := <-interactiveDone; err != nil {
		t.Fatalf("interactive transaction failed: %v", err)
	}
	if err := <-continuationDone; err != nil {
		t.Fatalf("metadata continuation failed: %v", err)
	}
}

func TestLibraryReadModelRepairReleasesWriterBetweenBatches(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_batched_repair', 'Batched Repair', 'movie', 999, '/tmp/batched-repair', '{}', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	tx, err := server.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < libraryReadModelRepairBatchSize*2+5; index++ {
		id := fmt.Sprintf("batched_repair_movie_%03d", index)
		if _, err := tx.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, added_at)
			VALUES (?, 'lib_batched_repair', 'movie', ?, ?, '["Potato"]', ?)`, id, id, id, now); err != nil {
			_ = tx.Rollback()
			t.Fatalf("insert item %d: %v", index, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	firstBatch := make(chan struct{})
	resumeRepair := make(chan struct{})
	var batches atomic.Int32
	server.readModelRepairBatchHook = func(int) {
		if batches.Add(1) == 1 {
			close(firstBatch)
			<-resumeRepair
		}
	}
	repairDone := make(chan error, 1)
	go func() {
		repairDone <- server.rebuildLibraryCategoryFacetsContext(context.Background(), "lib_batched_repair")
	}()
	select {
	case <-firstBatch:
	case <-time.After(2 * time.Second):
		t.Fatal("repair did not commit its first bounded batch")
	}

	interactiveEntered := make(chan struct{})
	releaseInteractive := make(chan struct{})
	interactiveDone := make(chan error, 1)
	go func() {
		interactiveDone <- server.withUserTxTagged(context.Background(), nil, func(*sql.Tx) error {
			close(interactiveEntered)
			<-releaseInteractive
			return nil
		})
	}()
	select {
	case <-interactiveEntered:
	case <-time.After(time.Second):
		t.Fatal("interactive write could not enter between repair batches")
	}
	close(resumeRepair)
	select {
	case err := <-repairDone:
		close(releaseInteractive)
		t.Fatalf("repair completed while interactive writer held admission: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseInteractive)
	if err := <-interactiveDone; err != nil {
		t.Fatalf("interactive write failed: %v", err)
	}
	if err := <-repairDone; err != nil {
		t.Fatalf("repair failed: %v", err)
	}
	if got := batches.Load(); got != 3 {
		t.Fatalf("repair batches = %d, expected 3", got)
	}
	var facets int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_category_facets WHERE library_id = 'lib_batched_repair' AND facet_type = 'genre' AND value = 'Potato'`).Scan(&facets); err != nil {
		t.Fatal(err)
	}
	if facets != libraryReadModelRepairBatchSize*2+5 {
		t.Fatalf("genre facets = %d, expected %d", facets, libraryReadModelRepairBatchSize*2+5)
	}
}

func TestLibraryReadModelRepairDefersAggregateRebuildUntilFinalConvergence(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_deferred_counts', 'Deferred Counts', 'movie', 997, '/tmp/deferred-counts', '{}', ?)`, now); err != nil {
		t.Fatal(err)
	}
	tx, err := server.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < libraryReadModelRepairBatchSize+1; index++ {
		id := fmt.Sprintf("deferred_count_movie_%03d", index)
		if _, err := tx.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, added_at)
			VALUES (?, 'lib_deferred_counts', 'movie', ?, ?, '["Deferred"]', ?)`, id, id, id, now); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO library_category_counts
			(library_id, filter, count, representative_media_id, representative_image, updated_at)
		VALUES ('lib_deferred_counts', 'repair-sentinel', 99, '', '', ?)`, now); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	firstBatch := make(chan struct{})
	resumeRepair := make(chan struct{})
	var batches atomic.Int32
	server.readModelRepairBatchHook = func(int) {
		if batches.Add(1) == 1 {
			close(firstBatch)
			<-resumeRepair
		}
	}
	repairDone := make(chan error, 1)
	go func() {
		repairDone <- server.rebuildLibraryCategoryFacetsContext(context.Background(), "lib_deferred_counts")
	}()
	select {
	case <-firstBatch:
	case <-time.After(2 * time.Second):
		t.Fatal("repair did not commit its first bounded batch")
	}

	var sentinel int
	if err := server.db.QueryRow(`
		SELECT count FROM library_category_counts
		WHERE library_id = 'lib_deferred_counts' AND filter = 'repair-sentinel'`).Scan(&sentinel); err != nil {
		t.Fatalf("repair rebuilt library aggregates inside an item batch: %v", err)
	}
	if sentinel != 99 {
		t.Fatalf("sentinel count during repair = %d, want 99", sentinel)
	}
	close(resumeRepair)
	if err := <-repairDone; err != nil {
		t.Fatalf("repair failed: %v", err)
	}

	var finalCount, sentinelRows int
	if err := server.db.QueryRow(`
		SELECT count FROM library_category_counts
		WHERE library_id = 'lib_deferred_counts' AND lower(filter) = 'genre:deferred'`).Scan(&finalCount); err != nil {
		t.Fatalf("read final aggregate: %v", err)
	}
	if err := server.db.QueryRow(`
		SELECT COUNT(*) FROM library_category_counts
		WHERE library_id = 'lib_deferred_counts' AND filter = 'repair-sentinel'`).Scan(&sentinelRows); err != nil {
		t.Fatal(err)
	}
	if finalCount != libraryReadModelRepairBatchSize+1 || sentinelRows != 0 {
		t.Fatalf("final aggregate count=%d sentinelRows=%d, want %d/0", finalCount, sentinelRows, libraryReadModelRepairBatchSize+1)
	}
}

func TestLibraryReadModelRepairPreservesConcurrentUpdateAndDelete(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_repair_race', 'Repair Race', 'movie', 996, '/tmp/repair-race', '{}', ?)`, now); err != nil {
		t.Fatal(err)
	}
	tx, err := server.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < libraryReadModelRepairBatchSize+1; index++ {
		id := fmt.Sprintf("repair_race_movie_%03d", index)
		if _, err := tx.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, metadata_revision, added_at)
			VALUES (?, 'lib_repair_race', 'movie', ?, ?, '["Original"]', 0, ?)`, id, id, id, now); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	releaseGate, err := server.dbWriteScheduler.acquire(t.Context(), foundationcontract.WorkClassInteractive)
	if err != nil {
		t.Fatal(err)
	}
	repairDone := make(chan error, 1)
	go func() {
		repairDone <- server.rebuildLibraryCategoryFacetsContext(context.Background(), "lib_repair_race")
	}()
	waitForSQLiteSchedulerWaiters(t, server, foundationcontract.WorkClassBackgroundMedia, 1)

	updateTx, err := server.db.BeginTx(context.Background(), nil)
	if err != nil {
		releaseGate()
		t.Fatal(err)
	}
	if _, err := updateTx.Exec(`
		UPDATE media_items
		SET genres_json = '["Concurrent"]', metadata_revision = 1
		WHERE id = 'repair_race_movie_001'`); err != nil {
		_ = updateTx.Rollback()
		releaseGate()
		t.Fatal(err)
	}
	if err := replaceMediaCategoryFacetsTx(context.Background(), updateTx, "repair_race_movie_001", 1, "concurrent-update"); err != nil {
		_ = updateTx.Rollback()
		releaseGate()
		t.Fatal(err)
	}
	if err := updateTx.Commit(); err != nil {
		releaseGate()
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`DELETE FROM media_items WHERE id = 'repair_race_movie_002'`); err != nil {
		releaseGate()
		t.Fatal(err)
	}
	releaseGate()
	if err := <-repairDone; err != nil {
		t.Fatalf("repair failed after concurrent mutation: %v", err)
	}

	var source string
	var revision int
	if err := server.db.QueryRow(`
		SELECT source, metadata_revision
		FROM media_category_facets
		WHERE media_id = 'repair_race_movie_001' AND facet_type = 'genre' AND value = 'Concurrent'`).Scan(&source, &revision); err != nil {
		t.Fatalf("read concurrent projection: %v", err)
	}
	if source != "concurrent-update" || revision != 1 {
		t.Fatalf("concurrent projection source=%q revision=%d, want concurrent-update/1", source, revision)
	}
	var deletedFacets int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_category_facets WHERE media_id = 'repair_race_movie_002'`).Scan(&deletedFacets); err != nil {
		t.Fatal(err)
	}
	if deletedFacets != 0 {
		t.Fatalf("deleted item retained %d facets", deletedFacets)
	}
}

func TestBackgroundYieldThrottlesButDoesNotStarveBehindDirectPlay(t *testing.T) {
	server := &Server{workloadLanes: newWorkloadLanes()}
	lane := server.workloadLane(workloadLaneMediaBody)
	if !lane.tryAcquireUncounted() {
		t.Fatal("could not establish direct-play pressure")
	}
	defer lane.release()

	started := time.Now()
	done := make(chan bool, 1)
	go func() { done <- server.waitForForegroundPressureToEase(context.Background()) }()
	select {
	case <-done:
		t.Fatal("background work did not yield to active direct play")
	case <-time.After(250 * time.Millisecond):
	}
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("background yield stopped instead of advancing a bounded unit")
		}
	case <-time.After(backgroundForegroundYieldMaximum + time.Second):
		t.Fatal("persistent direct play starved background progress")
	}
	if elapsed := time.Since(started); elapsed < backgroundForegroundYieldMaximum-backgroundForegroundPollInterval {
		t.Fatalf("background resumed too early under direct-play pressure: %s", elapsed)
	}
}

func TestLibraryReadModelRepairCancellationRerunConverges(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_resumed_repair', 'Resumed Repair', 'movie', 998, '/tmp/resumed-repair', '{}', ?)`, now); err != nil {
		t.Fatal(err)
	}
	const itemCount = libraryReadModelRepairBatchSize + 5
	tx, err := server.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < itemCount; index++ {
		id := fmt.Sprintf("resumed_repair_movie_%03d", index)
		if _, err := tx.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, added_at)
			VALUES (?, 'lib_resumed_repair', 'movie', ?, ?, '["Resumable"]', ?)`, id, id, id, now); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var batches atomic.Int32
	server.readModelRepairBatchHook = func(int) {
		if batches.Add(1) == 1 {
			cancel()
		}
	}
	err = server.rebuildLibraryCategoryFacetsContext(ctx, "lib_resumed_repair")
	if err == nil || ctx.Err() == nil {
		t.Fatalf("cancelled repair error = %v", err)
	}
	var partial int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_category_facets WHERE library_id = 'lib_resumed_repair' AND facet_type = 'genre' AND value = 'Resumable'`).Scan(&partial); err != nil {
		t.Fatal(err)
	}
	if partial != libraryReadModelRepairBatchSize {
		t.Fatalf("partial repair facets = %d, expected committed first batch %d", partial, libraryReadModelRepairBatchSize)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_category_facets (media_id, library_id, facet_type, value, sort_value, updated_at)
		VALUES ('movie_meridian', 'lib_resumed_repair', 'genre', 'Stale Repair Facet', 'stale repair facet', ?)`, now); err != nil {
		t.Fatal(err)
	}

	server.readModelRepairBatchHook = nil
	if err := server.rebuildLibraryCategoryFacetsContext(context.Background(), "lib_resumed_repair"); err != nil {
		t.Fatalf("rerun repair: %v", err)
	}
	var finalFacets, staleFacets, aggregateCount int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_category_facets WHERE library_id = 'lib_resumed_repair' AND facet_type = 'genre' AND value = 'Resumable'`).Scan(&finalFacets); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_category_facets WHERE library_id = 'lib_resumed_repair' AND value = 'Stale Repair Facet'`).Scan(&staleFacets); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT count FROM library_category_counts WHERE library_id = 'lib_resumed_repair' AND lower(filter) = 'genre:resumable'`).Scan(&aggregateCount); err != nil {
		t.Fatal(err)
	}
	if finalFacets != itemCount || staleFacets != 0 || aggregateCount != itemCount {
		t.Fatalf("converged repair facets=%d stale=%d aggregate=%d, expected %d/0/%d", finalFacets, staleFacets, aggregateCount, itemCount, itemCount)
	}
}

func waitForSQLiteSchedulerWaiters(t *testing.T, server *Server, class foundationcontract.WorkClass, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for server.dbWriteScheduler.waiting(class) != want {
		if time.Now().After(deadline) {
			t.Fatalf("SQLite scheduler %s waiters = %d, expected %d", class, server.dbWriteScheduler.waiting(class), want)
		}
		time.Sleep(time.Millisecond)
	}
}
