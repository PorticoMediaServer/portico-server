package httpapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"portico.local/server/internal/identity"
	librarychannels "portico.local/server/internal/livechannels/library"
)

type channelResponseBarrier struct {
	http.ResponseWriter
	arrived chan<- int
	resume  <-chan struct{}
	ctx     context.Context
}

func (b *channelResponseBarrier) WriteHeader(status int) {
	b.arrived <- status
	select {
	case <-b.resume:
	case <-b.ctx.Done():
	}
	b.ResponseWriter.WriteHeader(status)
}

func TestChannelReadsAllowFourOverlappingHTTPResponses(t *testing.T) {
	for _, kind := range []string{"live", "library"} {
		t.Run(kind, func(t *testing.T) {
			var handler http.Handler
			var path, token string
			if kind == "live" {
				f := newGuideDirectoryHTTPFixture(t)
				handler, path, token = newAdmission().wrap(f.mux, f.mux), "/v1/guide/channels?limit=1", f.owner.AccessToken
			} else {
				d, _, owner, _ := notificationFixture(t)
				var err error
				d.LibraryChannels, err = librarychannels.New(d.DB)
				if err != nil {
					t.Fatal(err)
				}
				mux := http.NewServeMux()
				d.libraryChannelRoutes(mux)
				handler, path, token = newAdmission().wrap(mux, mux), "/v1/admin/library-channels/templates", owner.AccessToken
			}
			arrived, resume := make(chan int, 4), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(resume) })
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handler.ServeHTTP(&channelResponseBarrier{ResponseWriter: w, arrived: arrived, resume: resume, ctx: r.Context()}, r)
			}))
			defer server.Close()
			errors := make(chan error, 4)
			for i := 0; i < 4; i++ {
				go func() {
					r, _ := http.NewRequest("GET", server.URL+path, nil)
					r.Header.Set("Authorization", "Bearer "+token)
					client := &http.Client{Timeout: 3 * time.Second}
					response, err := client.Do(r)
					if response != nil {
						io.Copy(io.Discard, response.Body)
						response.Body.Close()
					}
					errors <- err
				}()
			}
			statuses := []int{}
			for i := 0; i < 4; i++ {
				select {
				case status := <-arrived:
					statuses = append(statuses, status)
				case <-time.After(2 * time.Second):
					once.Do(func() { close(resume) })
					t.Fatal("four authorized reads could not overlap")
				}
			}
			once.Do(func() { close(resume) })
			for _, status := range statuses {
				if status != 200 {
					t.Errorf("ordinary overlapping read refused: %v", statuses)
					break
				}
			}
			for i := 0; i < 4; i++ {
				if err := <-errors; err != nil {
					t.Error(err)
				}
			}
		})
	}
}

type channelBlockingBody struct {
	entered chan<- struct{}
	release <-chan struct{}
	source  *bytes.Reader
}

func (b *channelBlockingBody) Read(p []byte) (int, error) {
	if b.entered != nil {
		b.entered <- struct{}{}
		b.entered = nil
		<-b.release
	}
	return b.source.Read(p)
}
func (*channelBlockingBody) Close() error { return nil }

func TestChannelMutationWaitCancelsWithoutReadingBody(t *testing.T) {
	for _, kind := range []string{"live", "library"} {
		t.Run(kind, func(t *testing.T) {
			var mux *http.ServeMux
			var ownerToken, memberToken, mutationPath, readPath, readToken string
			if kind == "live" {
				f := newGuideDirectoryHTTPFixture(t)
				mux, ownerToken, memberToken = f.mux, f.owner.AccessToken, f.member.AccessToken
				mutationPath, readPath, readToken = "/v1/admin/live-sources/preview", "/v1/guide/channels?limit=1", memberToken
			} else {
				d, _, owner, member := notificationFixture(t)
				var err error
				d.LibraryChannels, err = librarychannels.New(d.DB)
				if err != nil {
					t.Fatal(err)
				}
				mux = http.NewServeMux()
				d.libraryChannelRoutes(mux)
				ownerToken, memberToken = owner.AccessToken, member.AccessToken
				mutationPath, readPath, readToken = "/v1/admin/library-channels/preview", "/v1/admin/library-channels/templates", ownerToken
			}
			entered, release := make(chan struct{}, 2), make(chan struct{})
			results := make(chan *httptest.ResponseRecorder, 2)
			var releaseOnce sync.Once
			defer func() {
				releaseOnce.Do(func() { close(release) })
				for i := 0; i < 2; i++ {
					<-results
				}
			}()
			for i := 0; i < 2; i++ {
				go func() {
					body := &channelBlockingBody{entered: entered, release: release, source: bytes.NewReader([]byte(`{"name":"invalid"}`))}
					r := httptest.NewRequest("POST", mutationPath, body)
					r.Header.Set("Authorization", "Bearer "+ownerToken)
					w := httptest.NewRecorder()
					mux.ServeHTTP(w, r)
					results <- w
				}()
			}
			for i := 0; i < 2; i++ {
				select {
				case <-entered:
				case <-time.After(time.Second):
					t.Fatal("mutations did not hold two slots")
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			r := httptest.NewRequest("POST", mutationPath, avatarUnreadBody{t}).WithContext(ctx)
			r.Header.Set("Authorization", "Bearer "+ownerToken)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != 503 || ctx.Err() == nil {
				t.Fatalf("mutation wait did not honor cancellation: %d %s", w.Code, w.Body)
			}
			// An unauthorized account is rejected before waiting for mutation capacity.
			r = httptest.NewRequest("POST", mutationPath, avatarUnreadBody{t})
			r.Header.Set("Authorization", "Bearer "+memberToken)
			w = httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != 401 && w.Code != 403 {
				t.Fatalf("member acquired owner mutation capacity: %d %s", w.Code, w.Body)
			}
			// Reads remain available even while both mutation slots are occupied.
			r = httptest.NewRequest("GET", readPath, nil)
			r.Header.Set("Authorization", "Bearer "+readToken)
			w = httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != 200 {
				t.Fatalf("mutation slots blocked a read: %d %s", w.Code, w.Body)
			}
			// fixture cleanup must not race the two admitted handlers.
			releaseOnce.Do(func() { close(release) })
		})
	}
}

func TestGuideOnlyDevicesLearnFairnessSharesAndRecheckRevocation(t *testing.T) {
	for _, role := range []string{"member", "owner"} {
		t.Run(role, func(t *testing.T) {
			f := newGuideDirectoryHTTPFixture(t)
			first := f.member
			if role == "owner" {
				first = f.owner
			}
			second, err := f.d.Identity.Issue("live-"+role, role+"-profile", "local", role, 1)
			if err != nil {
				t.Fatal(err)
			}
			gate := newAdmission()
			d := f.d
			d.admission = gate
			mux := http.NewServeMux()
			d.liveChannelRoutes(mux, f.store)
			handler := gate.wrap(mux, mux)
			requests := make([]*http.Request, 2)
			for i, token := range []string{first.AccessToken, second.AccessToken} {
				r := httptest.NewRequest("GET", "/v1/guide/channels?limit=1", nil)
				r.RemoteAddr = "198.51.100.42:12000"
				r.Header.Set("Authorization", "Bearer "+token)
				requests[i] = r
			}
			if gate.clientKey(requests[0]) != gate.clientKey(requests[1]) {
				t.Fatal("unknown guide credentials bypassed peer accounting")
			}
			for _, r := range requests {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != 200 {
					t.Fatalf("guide-only device: %d %s", w.Code, w.Body)
				}
			}
			if gate.clientKey(requests[0]) == gate.clientKey(requests[1]) {
				t.Fatal("verified guide-only devices behind one NAT share a fairness bucket")
			}
			for _, r := range requests {
				if _, known := gate.clients.lookup(presentedCredential(r), time.Now()); !known {
					t.Fatal("successful guide-only device was not remembered")
				}
			}
			if _, err := d.DB.Exec(`UPDATE authorization_session_families SET revoked=1 WHERE id=(SELECT family_id FROM authorization_family_tokens WHERE token_hash=?)`, identity.Digest(first.AccessToken)); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, requests[0])
			if w.Code != 401 {
				t.Fatalf("remembered guide token bypassed revocation: %d %s", w.Code, w.Body)
			}
			w = httptest.NewRecorder()
			handler.ServeHTTP(w, requests[1])
			if w.Code != 200 {
				t.Fatalf("revocation affected other device: %d %s", w.Code, w.Body)
			}
		})
	}
}
