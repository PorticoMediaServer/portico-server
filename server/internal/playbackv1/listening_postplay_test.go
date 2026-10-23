package playbackv1

import (
	"context"
	"database/sql"
	"portico.local/server/internal/identity"
	"testing"
)

func TestListeningQueueIgnoresVideoEpisodePreferences(t *testing.T) {
	for _, audio := range []bool{false, true} {
		f := newCarryFixture(t, audio)
		f.service.PostPlay = func(context.Context, *sql.Tx, identity.Principal) (PostPlayPolicy, error) {
			return PostPlayPolicy{Autoplay: false, CountdownMs: 15000, StartDuringCredits: true, Protection: true, Episodes: 2}, nil
		}
		q := f.queue()
		if q.Queue.PostPlay == nil {
			t.Fatal("queue omitted its actual post-play policy")
		}
		policy := q.Queue.PostPlay
		if audio && (!policy.Autoplay || policy.CountdownMs != 0 || policy.StartDuringCredits || policy.PassoutCheckDue) {
			t.Fatal("video preferences reached a listening queue", policy)
		}
		if !audio && (policy.Autoplay || policy.CountdownMs != 15000 || !policy.StartDuringCredits) {
			t.Fatal("video episode preferences were changed", policy)
		}
	}
}
