package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"portico.local/server/internal/operations"
	"testing"
)

func TestConnectivitySaveAppliesRuntimeSettings(t *testing.T) {
	f := administrationFixtureNew(t)
	d := f.deps
	calls := 0
	d.SettingsApplied = func(context.Context) { calls++ }
	h := New(d)
	doc, e := d.Console.Settings(context.Background(), operations.AllowServerScope)
	if e != nil {
		t.Fatal(e)
	}
	b := fmt.Sprintf(`{"expectedRevision":%d,"operationId":"network-runtime-save","remoteBitrateLimitKbps":6000}`, doc.Revision)
	r := httptest.NewRequest("PATCH", "http://media.example/v1/admin/connectivity/policy", bytes.NewBufferString(b))
	r.RemoteAddr = "127.0.0.1:9"
	r.Header.Set("Authorization", "Bearer "+f.owner)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("save %d %s", w.Code, w.Body)
	}
	if calls != 1 {
		t.Fatalf("connectivity settings saved but SettingsApplied callback called %d times", calls)
	}
}
func TestConsoleSaveStopsDiscovery(t *testing.T) {
	f := administrationFixtureNew(t)
	a := f.deps.Access.Advertiser
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.Bind(ctx, func(ctx context.Context) { <-ctx.Done() })
	a.Apply(true)
	defer a.Close()
	doc, e := f.deps.Console.Settings(ctx, operations.AllowServerScope)
	if e != nil {
		t.Fatal(e)
	}
	v := doc.Effective
	v.LANDiscoveryEnabled = false
	b, _ := json.Marshal(operations.SettingsChange{ExpectedRevision: doc.Revision, IdempotencyKey: "discovery-runtime-save", Values: v})
	w := f.call("PATCH", "/v1/admin/console/settings", f.owner, string(b))
	if w.Code != 200 {
		t.Fatalf("save %d %s", w.Code, w.Body)
	}
	if s := a.Status(false); s.State != "stopped" {
		t.Fatalf("saved discovery=false but runtime=%+v", s)
	}
}
