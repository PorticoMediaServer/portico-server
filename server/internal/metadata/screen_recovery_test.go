package metadata

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"portico.local/server/internal/metadataprovider"
)

// outageScreenProvider fails every request with the given error while it is
// set, and answers like the fixture provider otherwise.
type outageScreenProvider struct {
	integrationScreenProvider
	problem error
}

func (p *outageScreenProvider) SearchScreen(ctx context.Context, kind, title string, year int, locale, region string) ([]metadataprovider.ScreenRecord, error) {
	if p.problem != nil {
		p.calls++
		return nil, p.problem
	}
	return p.integrationScreenProvider.SearchScreen(ctx, kind, title, year, locale, region)
}
func (p *outageScreenProvider) ScreenDetails(ctx context.Context, kind, id, locale, region string) (metadataprovider.ScreenRecord, error) {
	if p.problem != nil {
		p.calls++
		return metadataprovider.ScreenRecord{}, p.problem
	}
	return p.integrationScreenProvider.ScreenDetails(ctx, kind, id, locale, region)
}

func outageFixture(t *testing.T) (*Service, *sql.DB, *outageScreenProvider, func(time.Duration), func() (string, int, string)) {
	t.Helper()
	s, db, target, base := integrationScreen(t)
	p := &outageScreenProvider{integrationScreenProvider: *base}
	s.screenProviders = map[string]ScreenProvider{"tmdb": p}
	clock := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	entity := resolveArtworkTestEntity(t, db, target)
	row := func() (status string, attempts int, next string) {
		if err := db.QueryRow(`SELECT status,attempts,next_attempt FROM screen_metadata_work WHERE target_kind='item' AND target_id=?`, entity).Scan(&status, &attempts, &next); err != nil {
			t.Fatal(err)
		}
		return
	}
	return s, db, p, func(d time.Duration) { clock = clock.Add(d) }, row
}

// An outage long enough to use up a title's quick retries parks it, and the
// parked title is tried again after a cool-off instead of staying parked until
// somebody notices.
func TestTitleParkedByAnOutageIsRetriedAfterACoolOff(t *testing.T) {
	s, _, p, advance, row := outageFixture(t)
	ctx := context.Background()
	p.problem = &metadataprovider.Error{Provider: "tmdb", Code: "unavailable"}
	for n := 0; n < 6; n++ {
		if err := s.ScreenStep(ctx); err != nil {
			t.Fatal(err)
		}
		advance(20 * time.Minute)
	}
	status, attempts, next := row()
	if status != "unavailable" || attempts != 6 || next == "" {
		t.Fatalf("outage did not park the title with a retry time: %s %d %q", status, attempts, next)
	}
	// Parked means parked: nothing is asked again until the cool-off has passed.
	calls := p.calls
	if err := s.ScreenStep(ctx); err != nil || p.calls != calls {
		t.Fatalf("parked title retried before its cool-off: %v, %d extra requests", err, p.calls-calls)
	}
	due, err := time.Parse(time.RFC3339, next)
	if err != nil || due.Sub(s.publicationTime()) < 30*time.Minute {
		t.Fatalf("cool-off is too short to protect the provider: %q %v", next, err)
	}
	advance(due.Sub(s.publicationTime()) + time.Second)
	p.problem = nil
	if err := s.ScreenStep(ctx); err != nil {
		t.Fatal(err)
	}
	if status, attempts, _ = row(); status != "matched" || attempts != 0 {
		t.Fatalf("title did not recover when the provider came back: %s %d", status, attempts)
	}
}

// A parked title that fails again stays parked and waits longer each time.
func TestParkedTitleBacksOffFurtherWhileTheOutageLasts(t *testing.T) {
	s, _, p, advance, row := outageFixture(t)
	ctx := context.Background()
	p.problem = &metadataprovider.Error{Provider: "tmdb", Status: 503, Code: "request_rejected"}
	var waits []time.Duration
	for n := 0; n < 10; n++ {
		if err := s.ScreenStep(ctx); err != nil {
			t.Fatal(err)
		}
		status, _, next := row()
		due, err := time.Parse(time.RFC3339, next)
		if err != nil {
			t.Fatalf("attempt %d left no retry time: %s %q", n, status, next)
		}
		if n >= 5 {
			if status != "unavailable" {
				t.Fatalf("attempt %d: %s", n, status)
			}
			waits = append(waits, due.Sub(s.publicationTime()))
		}
		advance(due.Sub(s.publicationTime()) + time.Second)
	}
	for i, w := range waits {
		if w < time.Hour || w > 24*time.Hour || i > 0 && w < waits[i-1] {
			t.Fatalf("recovery waits are not a sparse, non-decreasing schedule: %v", waits)
		}
	}
}

// A failure that waiting cannot fix is still final: it is parked with no
// retry time and never claimed again on its own.
func TestTitleParkedByAFinalRefusalStaysParked(t *testing.T) {
	s, _, p, advance, row := outageFixture(t)
	ctx := context.Background()
	p.problem = &metadataprovider.Error{Provider: "tmdb", Status: 404, Code: "request_rejected"}
	if err := s.ScreenStep(ctx); err != nil {
		t.Fatal(err)
	}
	if status, _, next := row(); status != "unavailable" || next != "" {
		t.Fatalf("final refusal: %s %q", status, next)
	}
	calls := p.calls
	advance(72 * time.Hour)
	if err := s.ScreenStep(ctx); err != nil || p.calls != calls {
		t.Fatalf("final refusal was retried: %v, %d extra requests", err, p.calls-calls)
	}
}

// Waiting for a provider's own cooldown is not a failure of the title: it is
// put off until the cooldown ends and uses up none of its retries.
func TestWaitingOutAProviderCooldownCostsNoAttempts(t *testing.T) {
	s, db, p, advance, row := outageFixture(t)
	ctx := context.Background()
	for n := 0; n < 8; n++ {
		until := s.publicationTime().Add(10 * time.Minute)
		integrationExec(t, db, `INSERT INTO metadata_provider_cooldowns(provider,next_attempt) VALUES('tmdb',?) ON CONFLICT(provider) DO UPDATE SET next_attempt=excluded.next_attempt`, tvdbStamp(until))
		if err := s.ScreenStep(ctx); err != nil {
			t.Fatal(err)
		}
		status, attempts, next := row()
		if status != "pending" || attempts != 0 || next != tvdbStamp(until) {
			t.Fatalf("cooldown wait %d: %s, %d attempts, next %q (cooldown ends %q)", n, status, attempts, next, tvdbStamp(until))
		}
		advance(10*time.Minute + time.Second)
	}
	if p.calls != 0 {
		t.Fatalf("provider was asked %d times during its cooldown", p.calls)
	}
	integrationExec(t, db, `DELETE FROM metadata_provider_cooldowns`)
	if err := s.ScreenStep(ctx); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := row(); status != "matched" {
		t.Fatalf("title did not match once the cooldown was over: %s", status)
	}
}
