package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"portico.local/server/internal/dbwork"
	"strings"
	"time"

	"portico.local/server/internal/operations"
	"portico.local/server/internal/playback"
)

// deliveryContext is the edge's own observation of one request. Network class is
// resolved here and nowhere else: the client declares a transport, the server
// decides what that means next to the address it actually sees.
func (d Dependencies) deliveryContext(r *http.Request) playback.DeliveryContext {
	class, locality, transport := playback.NetworkClassFromRequest(r, d.trustedProxy(r))
	return playback.DeliveryContext{NetworkClass: class, ServerLocality: locality, TransportClass: transport, DeviceClass: deviceClassHeader(r)}
}

// trustedProxy reports whether the immediate peer is a configured proxy, which
// is the only case where a forwarded address may stand in for the peer's own.
func (d Dependencies) trustedProxy(r *http.Request) bool {
	if r == nil {
		return false
	}
	peer, ok := peerAddress(r)
	return ok && containsAddress(d.trustedProxyPrefixes(r.Context()), peer)
}

// DeviceClassHeader lets a client name the surface it is rendering on, which is
// the scope of its device-class preferences.
const DeviceClassHeader = "X-Portico-Device-Class"

func deviceClassHeader(r *http.Request) string {
	if r == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(r.Header.Get(DeviceClassHeader))) {
	case "web":
		return "web"
	case "mobile":
		return "mobile"
	case "television", "tv":
		return "television"
	}
	return ""
}

// withDelivery attaches the edge observation to the request context so session
// creation, offers and quality changes all resolve against the same lane.
func (d Dependencies) withDelivery(r *http.Request) *http.Request {
	return r.WithContext(playback.WithDeliveryContext(r.Context(), d.deliveryContext(r)))
}

// deliveryRoutes publishes what delivery resolved to and what this host can
// actually encode with. Both are reads; nothing here changes a session.
func (d Dependencies) deliveryRoutes(mux *http.ServeMux) {
	// GET /v1/playback/delivery-policy — one indexed row read plus the viewer's
	// preference documents. Constant cost, no paging.
	mux.HandleFunc("GET /v1/playback/delivery-policy", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, errInvalidDeliveryQuery)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		edge := d.deliveryContext(r)
		cfg := playback.DefaultDeliveryConfiguration()
		if d.Playback != nil {
			cfg = d.Playback.DeliveryConfiguration()
		}
		var values playback.DeliveryPreferenceReader
		if d.DB != nil {
			gated, txErr := dbwork.BeginSnapshot(ctx, d.DB)
			if txErr != nil {
				failure(w, txErr)
				return
			}
			tx := gated.Tx()
			read, _, _, prefErr := operations.EffectivePreferences(tx, p.Viewer, edge.DeviceClass)
			_ = gated.Rollback()
			if prefErr == nil {
				values = read
			}
		}
		policy := playback.ResolveDeliveryPolicy(values, edge.NetworkClass, edge.ServerLocality, edge.TransportClass, cfg)
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, map[string]any{"serverId": d.Identity.ID(), "policy": policy, "ladder": playback.QualityLadder, "planningPolicies": playback.PlanningPolicies, "transportClasses": playback.TransportClasses})
	})

	// PUT /v1/playback/client-profile — a device publishes what it can decode,
	// display and output. One bounded body, one indexed read, and a write only
	// when the document actually changed. The answer echoes the accepted revision
	// so a client can tell a fresh publication from a no-op.
	mux.HandleFunc("PUT /v1/playback/client-profile", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		if d.Playback == nil {
			failure(w, playback.ErrIncompatible)
			return
		}
		raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, playback.MaxClientProfileBytes+1))
		if e != nil {
			failure(w, playback.ErrClientProfile)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		profile, e := d.Playback.PublishClientProfile(ctx, p, raw)
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, map[string]any{"version": playback.ClientProfileVersion, "revision": profile.Revision, "summary": profile.Summary()})
	})

	// GET /v1/playback/client-profile — what the server holds for this sign-in,
	// or the built-in baseline it plans against when nothing has been published.
	mux.HandleFunc("GET /v1/playback/client-profile", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		if d.Playback == nil {
			failure(w, playback.ErrIncompatible)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		profile := d.Playback.ClientProfileFor(ctx, p)
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, map[string]any{"version": playback.ClientProfileVersion, "revision": profile.Revision, "summary": profile.Summary(), "profile": profile})
	})

	// POST /v1/playback/route-failures — a device says the route it was given
	// failed in its engine. Recorded against the file for this sign-in, so the
	// next plan takes the next route up. Constant cost.
	mux.HandleFunc("POST /v1/playback/route-failures", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		if d.Playback == nil {
			failure(w, playback.ErrIncompatible)
			return
		}
		var body playback.RouteFailureReport
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		// A v1 client names its v1 session; the route belongs to the media
		// session presenting it (playback_v1_presentation.go).
		presentation, v1, e := d.resolveV1Presentation(ctx, p, body.SessionID)
		if e != nil {
			failure(w, e)
			return
		}
		var out playback.RouteFailureResult
		if v1 {
			out, e = d.Playback.ReportRouteFailureV1(ctx, p, presentation.media, presentation.as(p).Hash, body)
		} else {
			out, e = d.Playback.ReportRouteFailure(ctx, p, body)
		}
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	})

	// GET /v1/admin/transcode/capacity lives in telemetry_routes.go; its report
	// takes this service's hardware probes through playbackProbes.
}

var errInvalidDeliveryQuery = errors.New("Read delivery policy without additional options.")
