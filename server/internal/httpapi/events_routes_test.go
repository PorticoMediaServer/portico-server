package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/apievents"
	"portico.local/server/internal/eventfeed"
	"portico.local/server/internal/notify"
)

func TestEventsLongPollEnforcesProfileScope(t *testing.T) {
	d, _, owner, member := notificationFixture(t)
	d.Events = eventfeed.New(d.DB)
	mux := http.NewServeMux()
	d.foundationRoutes(mux)
	if got := notificationRequest(mux, "GET", "", "/v1/events?waitMs=0", ""); got.Code != 401 {
		t.Fatalf("anonymous: %d %s", got.Code, got.Body.String())
	}
	if got := notificationRequest(mux, "GET", member.AccessToken, "/v1/events?after=bad&waitMs=0", ""); got.Code != 400 {
		t.Fatalf("bad cursor: %d %s", got.Code, got.Body.String())
	}
	raiseNotice(t, d, notify.ProfileScope("local", "owner", "owner-profile"), "owner-only", "Owner only")
	raiseNotice(t, d, notify.ProfileScope("local", "member", "member-profile"), "member-only", "Member only")
	request := httptest.NewRequest("GET", "/v1/events?after=0&waitMs=0", nil)
	request.Header.Set("Authorization", "Bearer "+member.AccessToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, request)
	if w.Code != 200 {
		t.Fatalf("member: %d %s", w.Code, w.Body.String())
	}
	var page eventfeed.Page
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Events) != 1 || page.Events[0].ID != "2" || page.NextAfter != "2" {
		t.Fatalf("member event scope: %+v %v", page, err)
	}
	w = notificationRequest(mux, "GET", owner.AccessToken, "/v1/events?after=0&waitMs=0", "")
	if w.Code != 200 {
		t.Fatalf("owner: %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Events) != 1 {
		t.Fatalf("owner must not receive member profile events: %+v %v", page, err)
	}
}

func TestEventsLongPollReadsCanonicalDeviceRing(t *testing.T) {
	d, _, _, member := notificationFixture(t)
	d.Events = eventfeed.New(d.DB)
	mux := http.NewServeMux()
	d.foundationRoutes(mux)
	tx, err := d.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = apievents.Append(tx, apievents.DeviceAudience(member.DeviceID), "session.updated", "session", "playback-1", "2", map[string]any{"state": "playing"}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	w := notificationRequest(mux, "GET", member.AccessToken, "/v1/events?after=0&waitMs=0", "")
	if w.Code != 200 {
		t.Fatalf("canonical ring: %d %s", w.Code, w.Body.String())
	}
	var page eventfeed.Page
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Events) != 1 || page.Events[0].Type != "session.updated" || page.Events[0].Resource.ID != "playback-1" || string(page.Events[0].Data) != `{"state":"playing"}` {
		t.Fatalf("device event: %+v %v", page, err)
	}
}

func TestEventsStreamDeliversAfterCommitAndRetiresDuplicate(t *testing.T) {
	d, _, _, member := notificationFixture(t)
	d.Events = eventfeed.New(d.DB)
	mux := http.NewServeMux()
	d.foundationRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	open := func() *http.Response {
		req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/v1/events", nil)
		req.Header.Set("Authorization", "Bearer "+member.AccessToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	first := open()
	defer first.Body.Close()
	if first.StatusCode != 200 || !strings.HasPrefix(first.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("first stream: %d", first.StatusCode)
	}
	reader := bufio.NewReader(first.Body)
	if line, err := reader.ReadString('\n'); err != nil || !strings.HasPrefix(line, "retry:") {
		t.Fatalf("stream hello: %q %v", line, err)
	}
	// A stream that opens with nothing waiting says where it stands, so a
	// client has a position to resume from before the first real event.
	next := func() (event, id string) {
		t.Helper()
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(line, "id: ") {
				id = strings.TrimSpace(strings.TrimPrefix(line, "id: "))
			}
			if strings.HasPrefix(line, "event: ") {
				return strings.TrimSpace(strings.TrimPrefix(line, "event: ")), id
			}
		}
	}
	if event, id := next(); event != "stream.opened" || id == "" {
		t.Fatalf("opening position: %q %q", event, id)
	}
	raiseNotice(t, d, notify.ProfileScope("local", "member", "member-profile"), "new", "New notice")
	d.Events.Wake()
	if event, _ := next(); event != "notification.changed" {
		t.Fatalf("unexpected event: %q", event)
	}
	// A reconnect takes over: it is admitted at once and the stream it
	// replaces ends, so a device that changed network is not refused.
	second := open()
	defer second.Body.Close()
	if second.StatusCode != 200 {
		t.Fatalf("a reconnecting stream was refused: %d", second.StatusCode)
	}
	for {
		if _, err := reader.ReadString('\n'); err != nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("the replaced stream stayed open")
		}
	}
}

func TestEventsStreamStopsAfterSessionRevocation(t *testing.T) {
	d, _, _, member := notificationFixture(t)
	d.Events = eventfeed.New(d.DB)
	mux := http.NewServeMux()
	d.foundationRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/v1/events", nil)
	req.Header.Set("Authorization", "Bearer "+member.AccessToken)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("stream: %d", response.StatusCode)
	}
	if err = d.Identity.LogoutToken(context.Background(), member.AccessToken); err != nil {
		t.Fatal(err)
	}
	d.Events.Wake()
	reader := bufio.NewReader(response.Body)
	for {
		_, err = reader.ReadString('\n')
		if err != nil {
			if ctx.Err() != nil {
				t.Fatal("revoked stream stayed live until client timeout")
			}
			return
		}
	}
}
