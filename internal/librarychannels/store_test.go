package librarychannels

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	porticodatabase "github.com/PorticoMediaServer/portico-server/internal/database"
)

func TestStorePersistsRevisionedAggregateAndActivatesAtomically(t *testing.T) {
	db := openTestDatabase(t)
	store := NewStore(db)
	ctx := context.Background()
	aggregate := testAggregate("America/Halifax")
	created, err := store.SaveAggregate(ctx, aggregate, 0)
	if err != nil {
		t.Fatalf("create aggregate: %v", err)
	}
	loaded, err := store.GetAggregate(ctx, aggregate.Channel.ID)
	if err != nil || loaded.Channel.ConfigRevision != 1 || len(loaded.Rules) != 1 {
		t.Fatalf("unexpected aggregate (%+v, %v)", loaded, err)
	}
	result, err := Generate(ctx, testGenerateRequest(created, time.Now().UTC().Truncate(time.Hour)))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	insertTestMedia(t, db, result.Entries)
	lease, err := store.AcquireGeneration(ctx, result.Generation, 5*time.Minute)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := store.CommitGeneration(ctx, result.Generation, result.Entries, lease.Token); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if queued, err := store.ListDueGenerationRequests(ctx, 8); err != nil || len(queued) != 0 {
		t.Fatalf("committed generation did not clear satisfied work: %+v, %v", queued, err)
	}
	active, err := store.LoadActiveSchedule(ctx, aggregate.Channel.ID, result.Generation.HorizonStart, result.Generation.HorizonEnd)
	if err != nil || len(active) != len(result.Entries) {
		t.Fatalf("active schedule = %d, %v", len(active), err)
	}
	activeGeneration, err := store.GetActiveGeneration(ctx, aggregate.Channel.ID)
	if err != nil || activeGeneration.ID != result.Generation.ID || len(activeGeneration.Cursors) == 0 {
		t.Fatalf("active generation = %+v, %v", activeGeneration, err)
	}

	if _, err := db.Exec(`DELETE FROM media_items WHERE id='media-a'`); err != nil {
		t.Fatal(err)
	}
	afterDelete, err := store.LoadActiveSchedule(ctx, aggregate.Channel.ID, result.Generation.HorizonStart, result.Generation.HorizonEnd)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range afterDelete {
		if entry.Kind == EntryUnavailable && entry.ReasonCode == ReasonMediaUnavailable {
			found = true
			if entry.MediaID != "" || entry.Title != "" || entry.Summary != "" || string(entry.Artwork) != "{}" || entry.PlayoutSource.Kind != PlayoutUnavailable {
				t.Fatalf("deleted media leaked metadata: %+v", entry)
			}
		}
	}
	if !found {
		t.Fatal("expected redacted unavailable timeline entry")
	}
	afterGeneration, err := store.GetActiveGeneration(ctx, aggregate.Channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	encodedCursor, _ := json.Marshal(afterGeneration.Cursors)
	if strings.Contains(string(encodedCursor), "media-a") {
		t.Fatalf("deleted media identifier remained in generation cursor: %s", encodedCursor)
	}
	base, err := store.LoadExtensionBase(ctx, aggregate.Channel.ID, result.Generation.HorizonStart, result.Generation.HorizonEnd)
	if err != nil || len(base.Cursors) == 0 {
		t.Fatalf("rolling extension after deletion = %+v, %v", base, err)
	}
	updated, err := store.GetAggregate(ctx, aggregate.Channel.ID)
	if err != nil || updated.Channel.HealthState != "pending" || updated.Channel.HealthMessage != MessageRegenerationRequired {
		t.Fatalf("post-deletion health = %+v, %v", updated.Channel, err)
	}
}

func TestStoreRejectsLostConfigurationUpdate(t *testing.T) {
	store := NewStore(openTestDatabase(t))
	aggregate := testAggregate("UTC")
	if _, err := store.SaveAggregate(context.Background(), aggregate, 0); err != nil {
		t.Fatal(err)
	}
	aggregate.Channel.Name = "Changed"
	if _, err := store.SaveAggregate(context.Background(), aggregate, 1); err != nil {
		t.Fatal(err)
	}
	aggregate.Channel.Name = "Lost"
	if _, err := store.SaveAggregate(context.Background(), aggregate, 1); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("got %v", err)
	}
}

func TestConfigurationWritesDurablyCoalesceGenerationWork(t *testing.T) {
	db := openTestDatabase(t)
	store := NewStore(db)
	ctx := context.Background()
	aggregate := testAggregate("UTC")
	created, err := store.SaveAggregate(ctx, aggregate, 0)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := store.ListDueGenerationRequests(ctx, 8)
	if err != nil || len(requests) != 1 || requests[0].ChannelID != created.Channel.ID || requests[0].RequestedRevision != 1 {
		t.Fatalf("created generation request = %+v, %v", requests, err)
	}
	created.Channel.Name = "Edited without reshuffling"
	updated, err := store.SaveAggregate(ctx, created, 1)
	if err != nil {
		t.Fatal(err)
	}
	requests, err = store.ListDueGenerationRequests(ctx, 8)
	if err != nil || len(requests) != 1 || requests[0].RequestedRevision != 2 {
		t.Fatalf("coalesced generation request = %+v, %v", requests, err)
	}
	updated.Channel.Enabled = false
	if _, err := store.SaveAggregate(ctx, updated, 2); err != nil {
		t.Fatal(err)
	}
	requests, err = store.ListDueGenerationRequests(ctx, 8)
	if err != nil || len(requests) != 0 {
		t.Fatalf("disabled generation queue = %+v, %v", requests, err)
	}
}

func TestExpiringScheduleRenewalEnqueuesOnceAndDefersWithBackoff(t *testing.T) {
	db := openTestDatabase(t)
	store := NewStore(db)
	ctx := context.Background()
	created, err := store.SaveAggregate(ctx, testAggregate("UTC"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM library_channel_generation_queue`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnqueueExpiringChannels(ctx, time.Now().UTC().Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnqueueExpiringChannels(ctx, time.Now().UTC().Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	requests, err := store.ListDueGenerationRequests(ctx, 8)
	if err != nil || len(requests) != 1 || requests[0].ChannelID != created.Channel.ID {
		t.Fatalf("renewal requests = %+v, %v", requests, err)
	}
	if err := store.DeferGenerationRequest(ctx, created.Channel.ID, 1, time.Minute, MessageGenerationFailed); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnqueueExpiringChannels(ctx, time.Now().UTC().Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	requests, err = store.ListDueGenerationRequests(ctx, 8)
	if err != nil || len(requests) != 0 {
		t.Fatalf("deferred request was still due: %+v, %v", requests, err)
	}
	health, err := store.GetAggregate(ctx, created.Channel.ID)
	if err != nil || health.Channel.HealthState != "error" || health.Channel.HealthMessage != MessageGenerationFailed {
		t.Fatalf("deferred generation health = %+v, %v", health.Channel, err)
	}
}

func TestBuiltInTemplateKeyIsUniqueAcrossConcurrentRestores(t *testing.T) {
	store := NewStore(openConcurrentTestDatabase(t))
	ctx := context.Background()
	start := make(chan struct{})
	errorsByWorker := make(chan error, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			aggregate := testAggregate("UTC")
			aggregate.Channel.ID = fmt.Sprintf("restore-channel-%d", index)
			aggregate.Channel.TemplateKey = "movie-time"
			aggregate.Channel.DefaultRuleID = fmt.Sprintf("restore-rule-%d", index)
			aggregate.Rules[0].ID = aggregate.Channel.DefaultRuleID
			aggregate.Rules[0].ChannelID = aggregate.Channel.ID
			<-start
			_, err := store.SaveAggregate(ctx, aggregate, 0)
			errorsByWorker <- err
		}(index)
	}
	close(start)
	wait.Wait()
	close(errorsByWorker)
	successes, alreadyRestored := 0, 0
	for err := range errorsByWorker {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrTemplateExists):
			alreadyRestored++
		default:
			t.Fatalf("unexpected concurrent restore error: %v", err)
		}
	}
	if successes != 1 || alreadyRestored != 1 {
		t.Fatalf("successes=%d alreadyRestored=%d", successes, alreadyRestored)
	}
}

func TestStoreReordersCompleteChannelSetAtomically(t *testing.T) {
	db := openTestDatabase(t)
	store := NewStore(db)
	ctx := context.Background()
	first := testAggregate("UTC")
	first.Channel.ID = "channel-first"
	first.Channel.SortOrder = 0
	first.Rules[0].ID = "rule-first"
	first.Rules[0].ChannelID = first.Channel.ID
	first.Channel.DefaultRuleID = first.Rules[0].ID
	second := testAggregate("UTC")
	second.Channel.ID = "channel-second"
	second.Channel.SortOrder = 1
	second.Rules[0].ID = "rule-second"
	second.Rules[0].ChannelID = second.Channel.ID
	second.Channel.DefaultRuleID = second.Rules[0].ID
	if _, err := store.SaveAggregate(ctx, first, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveAggregate(ctx, second, 0); err != nil {
		t.Fatal(err)
	}

	reordered, err := store.ReorderChannels(ctx, []ChannelOrder{
		{ID: second.Channel.ID, ExpectedRevision: 1, SortOrder: 0},
		{ID: first.Channel.ID, ExpectedRevision: 1, SortOrder: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(reordered) != 2 || reordered[0].ID != second.Channel.ID || reordered[1].ID != first.Channel.ID || reordered[0].ConfigRevision != 2 || reordered[1].ConfigRevision != 2 {
		t.Fatalf("unexpected reordered channels: %+v", reordered)
	}

	if _, err := store.ReorderChannels(ctx, []ChannelOrder{
		{ID: first.Channel.ID, ExpectedRevision: 1, SortOrder: 0},
		{ID: second.Channel.ID, ExpectedRevision: 2, SortOrder: 1},
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale reorder = %v", err)
	}
	unchanged, err := store.ListChannels(ctx, true)
	if err != nil || unchanged[0].ID != second.Channel.ID || unchanged[1].ID != first.Channel.ID {
		t.Fatalf("failed reorder was not atomic: %+v, %v", unchanged, err)
	}
}

func TestGenerationLeaseIsOpaqueDBTimedAndBindsDefinition(t *testing.T) {
	db := openTestDatabase(t)
	store := NewStore(db)
	ctx := context.Background()
	created, err := store.SaveAggregate(ctx, testAggregate("UTC"), 0)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Generate(ctx, testGenerateRequest(created, time.Now().UTC().Truncate(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	insertTestMedia(t, db, result.Entries)
	lease, err := store.AcquireGeneration(ctx, result.Generation, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.QueryRow(`SELECT lease_token_hash FROM library_channel_schedule_generations WHERE id=?`, result.Generation.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == lease.Token || len(stored) != 64 {
		t.Fatal("lease token was not stored as a digest")
	}
	if _, err := store.RenewGenerationLease(ctx, result.Generation.ID, "stolen", time.Minute); !errors.Is(err, ErrGenerationStale) {
		t.Fatalf("stolen renewal = %v", err)
	}
	tampered := result.Generation
	tampered.CandidateHash = stableHash("tampered")
	if err := store.CommitGeneration(ctx, tampered, result.Entries, lease.Token); !errors.Is(err, ErrGenerationStale) {
		t.Fatalf("tampered commit = %v", err)
	}
	if err := store.CommitGeneration(ctx, result.Generation, result.Entries, "wrong"); !errors.Is(err, ErrGenerationStale) {
		t.Fatalf("wrong token commit = %v", err)
	}
	if err := store.CommitGeneration(ctx, result.Generation, result.Entries, lease.Token); err != nil {
		t.Fatalf("valid commit: %v", err)
	}
}

func TestGenerationLeaseRenewalExtendsBeyondOriginalExpiry(t *testing.T) {
	db := openTestDatabase(t)
	store := NewStore(db)
	ctx := context.Background()
	created, err := store.SaveAggregate(ctx, testAggregate("UTC"), 0)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Generate(ctx, testGenerateRequest(created, time.Now().UTC().Truncate(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	insertTestMedia(t, db, result.Entries)
	lease, err := store.AcquireGeneration(ctx, result.Generation, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	renewed, err := store.RenewGenerationLease(ctx, result.Generation.ID, lease.Token, 10*time.Second)
	if err != nil || !renewed.ExpiresAt.After(lease.ExpiresAt) {
		t.Fatalf("renewed lease = %+v, %v", renewed, err)
	}
	if _, err := db.Exec(`UPDATE library_channel_schedule_generations SET lease_expires_at=unixepoch()+5 WHERE id=?`, result.Generation.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitGeneration(ctx, result.Generation, result.Entries, lease.Token); err != nil {
		t.Fatalf("renewed worker could not commit: %v", err)
	}
}

func TestExpiredWorkerCannotCommitAndRecoveryUpdatesHealth(t *testing.T) {
	db := openTestDatabase(t)
	store := NewStore(db)
	ctx := context.Background()
	created, err := store.SaveAggregate(ctx, testAggregate("UTC"), 0)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Generate(ctx, testGenerateRequest(created, time.Now().UTC().Truncate(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	insertTestMedia(t, db, result.Entries)
	lease, err := store.AcquireGeneration(ctx, result.Generation, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE library_channel_schedule_generations SET lease_expires_at=unixepoch()-1 WHERE id=?`, result.Generation.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitGeneration(ctx, result.Generation, result.Entries, lease.Token); !errors.Is(err, ErrGenerationStale) {
		t.Fatalf("expired commit = %v", err)
	}
	count, err := store.RecoverExpiredGenerations(ctx)
	if err != nil || count != 1 {
		t.Fatalf("recover = %d, %v", count, err)
	}
	channel, err := store.GetAggregate(ctx, created.Channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if channel.Channel.HealthState != "error" || channel.Channel.HealthMessage != MessageGenerationLeaseExpired {
		t.Fatalf("health = %+v", channel.Channel)
	}
}

func TestGenerationLeaseSerializesBuilders(t *testing.T) {
	store := NewStore(openTestDatabase(t))
	ctx := context.Background()
	created, err := store.SaveAggregate(ctx, testAggregate("UTC"), 0)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Generate(ctx, testGenerateRequest(created, time.Now().UTC().Truncate(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireGeneration(ctx, first.Generation, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	second := first.Generation
	second.ID = "builder-two"
	if _, err := store.AcquireGeneration(ctx, second, time.Minute); !errors.Is(err, ErrGenerationInProgress) {
		t.Fatalf("second builder = %v", err)
	}
	if _, err := store.RenewGenerationLease(ctx, first.Generation.ID, lease.Token, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
}

func TestGenerationLeaseSerializesConcurrentBuildersAcrossConnections(t *testing.T) {
	store := NewStore(openConcurrentTestDatabase(t))
	ctx := context.Background()
	created, err := store.SaveAggregate(ctx, testAggregate("UTC"), 0)
	if err != nil {
		t.Fatal(err)
	}
	base, err := Generate(ctx, testGenerateRequest(created, time.Now().UTC().Truncate(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errorsByWorker := make(chan error, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			generation := base.Generation
			generation.ID = fmt.Sprintf("concurrent-builder-%d", index)
			<-start
			_, err := store.AcquireGeneration(ctx, generation, time.Minute)
			errorsByWorker <- err
		}(index)
	}
	close(start)
	wait.Wait()
	close(errorsByWorker)
	successes, conflicts := 0, 0
	for err := range errorsByWorker {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrGenerationInProgress):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent acquire error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestNoPlayableGenerationFailsHonestlyWithoutReplacingSchedule(t *testing.T) {
	db := openTestDatabase(t)
	store := NewStore(db)
	ctx := context.Background()
	created, err := store.SaveAggregate(ctx, testAggregate("UTC"), 0)
	if err != nil {
		t.Fatal(err)
	}
	request := testGenerateRequest(created, time.Now().UTC().Truncate(time.Hour))
	request.Candidates["rule-default"] = nil
	result, err := Generate(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireGeneration(ctx, result.Generation, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CommitGeneration(ctx, result.Generation, result.Entries, lease.Token); !errors.Is(err, ErrNoPlayableSchedule) {
		t.Fatalf("commit = %v", err)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM library_channel_schedule_generations WHERE id=?`, result.Generation.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("status = %s", status)
	}
	channel, err := store.GetAggregate(ctx, created.Channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if channel.Channel.HealthState != "error" || channel.Channel.HealthMessage != MessageNoPlayableSchedule {
		t.Fatalf("health = %+v", channel.Channel)
	}
}

func TestCommitCannotReplaceProgramCurrentlyInProgress(t *testing.T) {
	db := openTestDatabase(t)
	store := NewStore(db)
	ctx := context.Background()
	created, err := store.SaveAggregate(ctx, testAggregate("UTC"), 0)
	if err != nil {
		t.Fatal(err)
	}
	request := testGenerateRequest(created, time.Now().UTC().Truncate(time.Hour))
	first, err := Generate(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	insertTestMedia(t, db, first.Entries)
	lease, err := store.AcquireGeneration(ctx, first.Generation, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CommitGeneration(ctx, first.Generation, first.Entries, lease.Token); err != nil {
		t.Fatal(err)
	}
	second := first.Generation
	second.ID = "generation-replacement"
	second.CreatedAt = time.Time{}
	secondEntries := append([]ScheduleEntry(nil), first.Entries...)
	for i := range secondEntries {
		secondEntries[i].GenerationID = second.ID
	}
	for i := range secondEntries {
		if secondEntries[i].StartsAt.Before(time.Now()) && secondEntries[i].EndsAt.After(time.Now()) {
			secondEntries[i].MediaID = "replacement"
			secondEntries[i].PlayoutSource.MediaID = "replacement"
			break
		}
	}
	if _, err := db.Exec(`INSERT INTO media_items(id,library_id,type,title,sort_title,added_at) VALUES('replacement','library-one','movie','Replacement','Replacement',?)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	lease2, err := store.AcquireGeneration(ctx, second, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CommitGeneration(ctx, second, secondEntries, lease2.Token); err == nil {
		t.Fatal("current program replacement was accepted")
	}
}

func TestCommitCannotRewritePastPrograms(t *testing.T) {
	db := openTestDatabase(t)
	store := NewStore(db)
	ctx := context.Background()
	created, err := store.SaveAggregate(ctx, testAggregate("UTC"), 0)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Truncate(time.Hour).Add(-time.Hour)
	first, err := Generate(ctx, testGenerateRequest(created, start))
	if err != nil {
		t.Fatal(err)
	}
	insertTestMedia(t, db, first.Entries)
	lease, err := store.AcquireGeneration(ctx, first.Generation, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CommitGeneration(ctx, first.Generation, first.Entries, lease.Token); err != nil {
		t.Fatal(err)
	}

	second := first.Generation
	second.ID = "generation-two"
	secondEntries := append([]ScheduleEntry(nil), first.Entries...)
	for index := range secondEntries {
		secondEntries[index].GenerationID = second.ID
	}
	secondEntries[0].MediaID = "replacement-media"
	secondEntries[0].Title = "Replacement"
	secondEntries[0].PlayoutSource.MediaID = "replacement-media"
	if _, err := db.Exec(`INSERT INTO media_items(id,library_id,type,title,sort_title,added_at) VALUES('replacement-media','library-one','movie','Replacement','Replacement',?)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	lease2, err := store.AcquireGeneration(ctx, second, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CommitGeneration(ctx, second, secondEntries, lease2.Token); err == nil {
		t.Fatal("past schedule rewrite was accepted")
	}
}

func TestDisabledChannelCannotBeReadByKnownIdentifier(t *testing.T) {
	db := openTestDatabase(t)
	store := NewStore(db)
	ctx := context.Background()
	created, err := store.SaveAggregate(ctx, testAggregate("UTC"), 0)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Generate(ctx, testGenerateRequest(created, time.Now().UTC().Truncate(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	insertTestMedia(t, db, result.Entries)
	lease, err := store.AcquireGeneration(ctx, result.Generation, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CommitGeneration(ctx, result.Generation, result.Entries, lease.Token); err != nil {
		t.Fatal(err)
	}
	created.Channel.Enabled = false
	if _, err := store.SaveAggregate(ctx, created, 1); err != nil {
		t.Fatal(err)
	}
	entries, err := store.LoadActiveScheduleForProfile(ctx, created.Channel.ID, result.Generation.HorizonStart, result.Generation.HorizonEnd, func(string) AccessDecision { return AccessAllowed })
	if err != nil || len(entries) != 0 {
		t.Fatalf("disabled schedule = %d entries, %v", len(entries), err)
	}
	if _, err := store.GetActiveGeneration(ctx, created.Channel.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled active generation = %v", err)
	}
}

func TestDatabaseRejectsCrossChannelActiveGeneration(t *testing.T) {
	db := openTestDatabase(t)
	store := NewStore(db)
	ctx := context.Background()
	first, err := store.SaveAggregate(ctx, testAggregate("UTC"), 0)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Generate(ctx, testGenerateRequest(first, time.Now().UTC().Truncate(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	insertTestMedia(t, db, result.Entries)
	lease, err := store.AcquireGeneration(ctx, result.Generation, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CommitGeneration(ctx, result.Generation, result.Entries, lease.Token); err != nil {
		t.Fatal(err)
	}
	second := testAggregate("UTC")
	second.Channel.ID = "channel-two"
	second.Channel.DefaultRuleID = "rule-two"
	second.Rules[0].ID = "rule-two"
	second.Rules[0].ChannelID = second.Channel.ID
	if _, err := store.SaveAggregate(ctx, second, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE library_channels SET active_generation_id=? WHERE id=?`, result.Generation.ID, second.Channel.ID); err == nil {
		t.Fatal("cross-channel active generation pointer was accepted")
	}
}

func TestProfileProjectionPreservesTimelineAndRedactsMetadata(t *testing.T) {
	entry := ScheduleEntry{ID: "one", GenerationID: "g", ChannelID: "c", MediaID: "secret", Kind: EntryMedia, StartsAt: time.Unix(10, 0), EndsAt: time.Unix(20, 0), Title: "Secret", Summary: "Private", Artwork: []byte(`{"url":"secret"}`), SelectionMetadata: []byte(`{"seriesId":"secret"}`), Availability: "available", PlayoutSource: PlayoutSource{Kind: PlayoutMedia, MediaID: "secret", DurationSeconds: 10}}
	projected := ProjectSchedule([]ScheduleEntry{entry}, func(string) AccessDecision { return AccessRestricted })
	if len(projected) != 1 || !projected[0].StartsAt.Equal(entry.StartsAt) || !projected[0].EndsAt.Equal(entry.EndsAt) || projected[0].MediaID != "" || projected[0].Title != "" || projected[0].ReasonCode != ReasonMediaRestricted {
		t.Fatalf("projection = %+v", projected)
	}
	if entry.Title != "Secret" {
		t.Fatal("projection mutated source")
	}
}

func openTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	return openReleaseTestDatabase(t, 1)
}

func openConcurrentTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "library-channels.db")
	db, err := porticodatabase.Open(config.Config{AppDataDir: filepath.Dir(path), DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	t.Cleanup(func() { _ = db.Close() })
	insertReleaseTestLibrary(t, db)
	return db
}

func openReleaseTestDatabase(t *testing.T, maxConnections int) *sql.DB {
	t.Helper()
	appDataDir := t.TempDir()
	db, err := porticodatabase.Open(config.Config{AppDataDir: appDataDir, DatabasePath: filepath.Join(appDataDir, "portico.db")})
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(maxConnections)
	db.SetMaxIdleConns(maxConnections)
	t.Cleanup(func() { _ = db.Close() })
	insertReleaseTestLibrary(t, db)
	return db
}

func insertReleaseTestLibrary(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO libraries(id,name,type,created_at) VALUES('library-one','Library One','movies',?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
}

func insertTestMedia(t *testing.T, db *sql.DB, entries []ScheduleEntry) {
	t.Helper()
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if entry.MediaID == "" {
			continue
		}
		if _, ok := seen[entry.MediaID]; ok {
			continue
		}
		seen[entry.MediaID] = struct{}{}
		if _, err := db.Exec(`INSERT INTO media_items(id,library_id,type,title,sort_title,added_at) VALUES(?,'library-one','movie',?,?,?)`, entry.MediaID, entry.MediaID, entry.MediaID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
}
