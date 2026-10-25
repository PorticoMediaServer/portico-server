package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"portico.local/hosted/internal/directory"
)

func TestExplicitMembershipEndingIsOwnAccountFrozenAndReplayStable(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	authority, err := f.dir.CaptureServerAuthority(ctx, f.token)
	if err != nil {
		t.Fatal(err)
	}
	memberID := strings.Repeat("a", 32)
	owner := directory.MemberStanding{AccountID: f.owner.Account.ID, Role: "owner", Active: true, MembershipID: strings.Repeat("b", 32)}
	member := directory.MemberStanding{AccountID: f.member.Account.ID, Role: "member", Active: true, MembershipID: memberID}
	push := func(p directory.MemberPush) directory.MemberPushResult {
		t.Helper()
		out, err := f.dir.PushMembers(ctx, authority, p)
		if err != nil || !out.Applied {
			t.Fatal("publication refused", err)
		}
		return out
	}
	push(directory.MemberPush{Full: true, Through: 1, Members: []directory.MemberStanding{owner, member}})
	if _, err = f.store.DB.Exec(`UPDATE servers SET name='A later hidden name' WHERE id=$1`, f.server); err != nil {
		t.Fatal(err)
	}
	ended := time.Now().UTC().Truncate(time.Microsecond)
	event := &directory.MembershipEnding{Sequence: 2, Cause: "owner_removed", EndedAt: ended, ExpiresAt: ended.Add(30 * 24 * time.Hour)}
	removed := directory.MemberStanding{AccountID: member.AccountID, MembershipID: memberID, Ending: event}
	p := directory.MemberPush{After: 1, Through: 2, Members: []directory.MemberStanding{removed}}
	push(p)
	notice, err := f.store.MembershipEndingNotice(ctx, member.AccountID)
	if err != nil || notice == nil || notice.MembershipID != memberID || notice.Sequence != 2 || notice.Cause != "owner_removed" || notice.ServerName != "Local" {
		t.Fatal("missing original admitted ending", err)
	}
	if other, err := f.store.MembershipEndingNotice(ctx, f.owner.Account.ID); err != nil || other != nil {
		t.Fatal("ending crossed account scope", err)
	}
	var before, after int64
	if err = f.store.DB.QueryRow(`SELECT server_list_version FROM accounts WHERE id=$1`, member.AccountID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	push(p)
	if err = f.store.DB.QueryRow(`SELECT server_list_version FROM accounts WHERE id=$1`, member.AccountID).Scan(&after); err != nil || after != before {
		t.Fatal("exact replay republished the ending", err)
	}
	// Neither the ended incarnation nor an unbound legacy full list is a rejoin.
	for n, active := range []directory.MemberStanding{member, {AccountID: member.AccountID, Role: "member", Active: true}} {
		push(directory.MemberPush{Full: true, Through: int64(3 + n), Members: []directory.MemberStanding{owner, active}})
		var listed bool
		if err = f.store.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM server_members WHERE server_id=$1 AND account_id=$2)`, f.server, member.AccountID).Scan(&listed); err != nil || listed {
			t.Fatal("stale full snapshot resurrected an ended membership", err)
		}
	}
	consent, err := f.dir.IssueIdentity(member.AccountID, "member", "Member", "", f.server, testChallenge, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(consent)
	if err != nil {
		t.Fatal(err)
	}
	rejoined := directory.MemberStanding{AccountID: member.AccountID, Role: "member", Active: true, MembershipID: strings.Repeat("c", 32), Consent: raw}
	push(directory.MemberPush{After: 4, Through: 5, Members: []directory.MemberStanding{rejoined}})
	if current, err := f.store.MembershipEndingNotice(ctx, member.AccountID); err != nil || current != nil {
		t.Fatal("rejoin retained the ending notice", err)
	}
	// An older incarnation's tombstone cannot remove the admitted successor.
	push(directory.MemberPush{Full: true, Through: 6, Members: []directory.MemberStanding{owner, removed}})
	var listed bool
	if err = f.store.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM server_members WHERE server_id=$1 AND account_id=$2)`, f.server, member.AccountID).Scan(&listed); err != nil || !listed {
		t.Fatal("old tombstone removed a successor", err)
	}
}

func TestMembershipEndingNeverComesFromOmissionDepartureOrUnknownIncarnation(t *testing.T) {
	for _, scenario := range []string{"omission", "departure", "unknown-incarnation", "neutral-ending"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t)
			ctx := context.Background()
			authority, err := f.dir.CaptureServerAuthority(ctx, f.token)
			if err != nil {
				t.Fatal(err)
			}
			owner := directory.MemberStanding{AccountID: f.owner.Account.ID, Role: "owner", Active: true}
			member := directory.MemberStanding{AccountID: f.member.Account.ID, Role: "member", Active: true, MembershipID: strings.Repeat("a", 32)}
			if scenario != "unknown-incarnation" {
				if _, err = f.dir.PushMembers(ctx, authority, directory.MemberPush{Full: true, Through: 1, Members: []directory.MemberStanding{owner, member}}); err != nil {
					t.Fatal(err)
				}
			}
			ended := time.Now().UTC().Truncate(time.Microsecond)
			removed := directory.MemberStanding{AccountID: member.AccountID, MembershipID: member.MembershipID, Ending: &directory.MembershipEnding{Sequence: 2, Cause: "owner_removed", EndedAt: ended, ExpiresAt: ended.Add(30 * 24 * time.Hour)}}
			p := directory.MemberPush{After: 1, Through: 2, Members: []directory.MemberStanding{removed}}
			switch scenario {
			case "omission":
				p = directory.MemberPush{Full: true, Through: 2, Members: []directory.MemberStanding{owner}}
			case "departure":
				if err = f.dir.LeaveServer(ctx, member.AccountID, f.server); err != nil {
					t.Fatal(err)
				}
			case "unknown-incarnation":
				p.After = 0
			case "neutral-ending":
				p.Members[0].Ending = nil
			}
			out, err := f.dir.PushMembers(ctx, authority, p)
			if err != nil || !out.Applied {
				t.Fatal("publication refused", err)
			}
			if notice, err := f.store.MembershipEndingNotice(ctx, member.AccountID); err != nil || notice != nil {
				t.Fatal("cause was manufactured", err)
			}
			// Used or unused invitation deadlines are not an access-ending protocol.
			removed.Ending.Cause = "invitation_expired"
			if _, err = f.dir.PushMembers(ctx, authority, directory.MemberPush{After: 2, Through: 3, Members: []directory.MemberStanding{removed}}); !errors.Is(err, directory.ErrInvalid) {
				t.Fatal("invented invitation expiry cause admitted", err)
			}
		})
	}
}
