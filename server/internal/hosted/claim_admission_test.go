package hosted

import (
	"context"
	"testing"

	"portico.local/server/internal/identity"
)

func TestClaimAcknowledgmentImmediatelyAdmitsOwner(t *testing.T) {
	f := newClaimFixture(t)
	// No control step, status readback, topology change or wake has run.
	if !f.installed.InstallationAcknowledged {
		t.Fatal("claim not acknowledged")
	}
	who, err := verify(f, freshClaims(f, "account"))
	if err != nil {
		t.Fatal(err)
	}
	in, err := f.id.PorticoSignIn(context.Background(), who)
	if err != nil || in.Session == nil {
		t.Fatalf("acknowledged owner refused: %+v %v", in, err)
	}
	if in.Session.Viewer.AccountID != f.owner.Viewer.AccountID || in.Session.Viewer.Role != identity.TierOwner {
		t.Fatal("claim admitted the wrong owner", in.Session.Viewer)
	}
	if _, err = f.id.Authenticate(in.Session.AccessToken); err != nil {
		t.Fatal("owner session unusable", err)
	}
}
