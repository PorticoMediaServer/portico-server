package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"portico.local/server/internal/playback"
)

// The resolved policy a client reads must come from the server's own view of the
// request, narrowed by the viewer's preferences. A client declaring a transport
// can narrow its lane; it can never widen it, and it can never claim locality.
func TestDeliveryPolicyRouteResolvesTheViewersLane(t *testing.T) {
	f := preferenceHTTP(t)

	read := func(peer, transport string) map[string]any {
		t.Helper()
		r := httptest.NewRequest("GET", "/v1/playback/delivery-policy", strings.NewReader(""))
		r.RemoteAddr = peer
		r.Header.Set("Authorization", "Bearer "+f.token)
		r.Header.Set("X-Portico-Device-Class", "web")
		if transport != "" {
			r.Header.Set(playback.TransportClassHeader, transport)
		}
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("resolved policy was made cacheable")
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		policy, _ := out["policy"].(map[string]any)
		if policy == nil {
			t.Fatalf("no policy published: %s", w.Body.String())
		}
		if ladder, _ := out["ladder"].([]any); len(ladder) != len(playback.QualityLadder) {
			t.Fatal("ladder was not published with the policy")
		}
		return policy
	}

	// A LAN peer with no declared transport is local; the client said nothing,
	// and the address is all the server needs.
	local := read("192.168.4.10:51000", "")
	if local["networkClass"] != string(playback.NetworkLocal) || local["serverLocality"] != playback.LocalityLocal {
		t.Fatal("loopback request was not resolved as local", local)
	}
	if local["preferenceLane"] != "local" {
		t.Fatal(local["preferenceLane"])
	}

	// A declared transport narrows the lane within that locality.
	if wifi := read("192.168.4.10:51000", "wifi"); wifi["networkClass"] != string(playback.NetworkWiFi) {
		t.Fatal("declared wifi was not honoured", wifi)
	}
	cellular := read("192.168.4.10:51000", "cellular")
	if cellular["networkClass"] != string(playback.NetworkCellular) || cellular["preferenceLane"] != "cellular" {
		t.Fatal("declared cellular was not honoured", cellular)
	}
	// The cellular lane's registry defaults are conservative by design.
	if cellular["allowHDR"] != false {
		t.Fatal("cellular lane allowed HDR", cellular)
	}

	// An unrecognised declaration is not an error and is not believed.
	if nonsense := read("192.168.4.10:51000", "satellite"); nonsense["transportClass"] != "unknown" || nonsense["networkClass"] != string(playback.NetworkLocal) {
		t.Fatal("unknown transport was believed", nonsense)
	}

	// A routable peer is remote however it declares itself, unless it says
	// cellular, which only it can know and which only narrows.
	remote := read("93.184.216.34:51000", "ethernet")
	if remote["networkClass"] != string(playback.NetworkRemote) || remote["serverLocality"] != playback.LocalityRemote {
		t.Fatal("public peer was not resolved as remote", remote)
	}
	if lying := read("93.184.216.34:51000", "wifi"); lying["networkClass"] != string(playback.NetworkRemote) {
		t.Fatal("a remote client claimed the wifi lane", lying)
	}
	if roaming := read("93.184.216.34:51000", "cellular"); roaming["networkClass"] != string(playback.NetworkCellular) {
		t.Fatal("remote cellular was not honoured", roaming)
	}

	// Every published policy names its three delivery decisions.
	for _, key := range []string{"directPlay", "directStream", "transcode", "planningPolicy", "clamps"} {
		if _, ok := local[key]; !ok {
			t.Fatal("policy omitted", key)
		}
	}
}

func TestDeliveryPolicyRouteRefusesOptionsAndAnonymousReads(t *testing.T) {
	f := preferenceHTTP(t)
	r := httptest.NewRequest("GET", "/v1/playback/delivery-policy?networkClass=local", strings.NewReader(""))
	r.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("query options accepted: %d %s", w.Code, w.Body.String())
	}
	// A client cannot ask for a lane; it may only declare its transport.
	anonymous := httptest.NewRequest("GET", "/v1/playback/delivery-policy", strings.NewReader(""))
	w = httptest.NewRecorder()
	f.handler.ServeHTTP(w, anonymous)
	if w.Code == 200 {
		t.Fatal("anonymous read of a viewer's policy succeeded")
	}
}

func TestTranscodeCapacityRouteRequiresOwner(t *testing.T) {
	f := preferenceHTTP(t)
	r := httptest.NewRequest("GET", "/v1/admin/transcode/capacity", strings.NewReader(""))
	r.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	// The fixture's viewer is a member, not the owner.
	if w.Code == 200 {
		t.Fatal("member read the owner's conversion capacity")
	}
}
