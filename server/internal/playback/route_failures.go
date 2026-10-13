package playback

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// A capability document can be wrong: a browser says it decodes a level it then
// chokes on, a television firmware lies about a codec. When a device reports
// that the route it was given failed in the engine (not on the network), the
// server remembers it for that file on that sign-in and plans the next route up.
// A wrong claim therefore costs one retry, not the title.

// routeFailureTTL bounds how long a report keeps a route closed. It is long
// enough to cover the rest of a viewing and short enough that a fixed client,
// driver or file is tried again without anyone clearing anything.
//
// That is for a report that says what was wrong: the engine could not decode
// the picture or the sound, or could not read the source. A report that only
// says something went wrong (a generic engine error, a stall) may be a passing
// glitch, so it closes the route for an evening, not a week.
const (
	routeFailureTTL          = 7 * 24 * time.Hour
	uncertainRouteFailureTTL = 6 * time.Hour
)

// certainRouteFailures are the codes that name a fault of the stream itself.
const certainRouteFailures = `'decode_error','source_not_supported','audio_decode_error','manifest_incompatible'`

var routeFailureCode = regexp.MustCompile(`^[a-z0-9_]{1,48}$`)

var ErrRouteFailure = errors.New("That playback failure report is not valid.")

// RouteFailureResult tells the device what a failure report changed.
type RouteFailureResult struct {
	Route string `json:"route"`
	// Engine is the player engine the failed route was planned for.
	Engine string `json:"engine"`
	// Escalates is false when the failed route was already the last resort, so
	// the device knows that asking again will plan the same thing.
	Escalates bool `json:"escalates"`
}

// RecordRouteFailure records that the route of one presentation failed in the
// device's engine: code is the engine's classification (decode_error,
// source_not_supported, audio_decode_error, stalled_without_data and so on),
// recorded and not interpreted. media is the presentation's media session,
// whose ownership the caller (the v1 session layer) has already decided. The
// failure is recorded for the caller's own sign-in and for the engine the
// presentation was planned for, which is what the next plan reads. It never
// touches the presentation itself.
func (s *Service) RecordRouteFailure(ctx context.Context, p identity.Principal, media, code, detail string) (RouteFailureResult, error) {
	var out RouteFailureResult
	if !validControlID(media) || !routeFailureCode.MatchString(code) || len(detail) > 512 || p.Hash == "" {
		return out, ErrRouteFailure
	}
	var asset, strategy, engine string
	err := s.db.QueryRowContext(ctx, `SELECT asset_id,COALESCE((SELECT d.strategy FROM playback_delivery_plans d WHERE d.session_id=playback_sessions.id),''),COALESCE((SELECT json_extract(d.policy_json,'$.engine') FROM playback_delivery_plans d WHERE d.session_id=playback_sessions.id AND d.policy_json<>''),'') FROM playback_sessions WHERE id=? AND account_id=? AND profile_id=?`, media, p.AccountID, p.ProfileID).Scan(&asset, &strategy, &engine)
	if errors.Is(err, sql.ErrNoRows) {
		// Not this viewer's presentation, ended, or unknown: hidden, and never a
		// 401 (a valid sign-in must not be sent to refresh its token).
		return out, identity.ErrNotVisible
	}
	if err != nil {
		return out, err
	}
	if engine != EngineMPV {
		engine = EngineNative
	}
	out.Route, out.Engine = strategy, engine
	switch DeliveryStrategy(strategy) {
	case DeliveryOriginal, DeliveryCopyRemux, DeliveryAudioConversion:
		out.Escalates = true
	default:
		// A full conversion is already the most compatible thing this server can
		// produce; there is nothing above it to escalate to.
		return out, nil
	}
	_, err = dbwork.ExecWrite(ctx, s.db, dbwork.ClassEstablishedPlayback, `INSERT INTO playback_route_failures(session_hash,asset_id,engine,route,code,detail,reported_at_ms) VALUES(?,?,?,?,?,?,?) ON CONFLICT(session_hash,asset_id,engine,route) DO UPDATE SET code=excluded.code,detail=excluded.detail,reported_at_ms=excluded.reported_at_ms`, p.Hash, asset, engine, strategy, code, publicAudioText(detail, 200), time.Now().UnixMilli())
	return out, err
}

// routeFailuresTx reads the routes this device has reported broken for one
// file, per player engine: a route that failed in one engine says nothing
// about the other.
func routeFailuresTx(ctx context.Context, tx *sql.Tx, p identity.Principal, asset string) (map[string]map[DeliveryStrategy]RouteRejection, error) {
	if p.Hash == "" {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT engine,route,code FROM (SELECT engine,route,code,ROW_NUMBER() OVER(PARTITION BY engine,route ORDER BY reported_at_ms DESC) AS rank FROM playback_route_failures WHERE (session_hash=? OR session_hash IN(SELECT original.token_hash FROM authorization_family_tokens original JOIN authorization_family_tokens caller ON caller.family_id=original.family_id WHERE caller.token_hash=?)) AND asset_id=? AND reported_at_ms>CASE WHEN code IN(`+certainRouteFailures+`) THEN ? ELSE ? END) WHERE rank=1 LIMIT 8`, p.Hash, p.Hash, asset, time.Now().Add(-routeFailureTTL).UnixMilli(), time.Now().Add(-uncertainRouteFailureTTL).UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out map[string]map[DeliveryStrategy]RouteRejection
	for rows.Next() {
		var engine, route, code string
		if err = rows.Scan(&engine, &route, &code); err != nil {
			return nil, err
		}
		if out == nil {
			out = map[string]map[DeliveryStrategy]RouteRejection{}
		}
		if out[engine] == nil {
			out[engine] = map[DeliveryStrategy]RouteRejection{}
		}
		out[engine][DeliveryStrategy(route)] = RouteRejection{Code: code}
	}
	return out, rows.Err()
}
