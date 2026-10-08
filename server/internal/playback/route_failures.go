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
const routeFailureTTL = 7 * 24 * time.Hour

var routeFailureCode = regexp.MustCompile(`^[a-z0-9_]{1,48}$`)

var ErrRouteFailure = errors.New("That playback failure report is not valid.")

// RouteFailureReport is what a device sends when its engine rejects a stream.
type RouteFailureReport struct {
	SessionID string `json:"sessionId"`
	// Code is the engine's classification: decode_error, source_not_supported,
	// audio_decode_error, stalled_without_data and so on. It is recorded, not
	// interpreted: any engine failure closes the route that was in use.
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// RouteFailureResult tells the device what the report changed.
type RouteFailureResult struct {
	Route string `json:"route"`
	// Escalates is false when the failed route was already the last resort, so
	// the device knows that asking again will plan the same thing.
	Escalates bool `json:"escalates"`
}

// ReportRouteFailure records that the route of one of this device's own sessions
// failed in the engine. It never touches the session itself: the device decides
// when to ask for playback again, and the next plan reads what is recorded here.
func (s *Service) ReportRouteFailure(ctx context.Context, p identity.Principal, r RouteFailureReport) (RouteFailureResult, error) {
	return s.reportRouteFailure(ctx, p, p.Hash, r)
}

// ReportRouteFailureV1 is ReportRouteFailure for the media session that
// presents a v1 session: the caller's ownership of it was decided by the v1
// rule, and bound is the token binding that media session holds. The failure
// is still recorded for the caller's own sign-in, which the next plan reads.
func (s *Service) ReportRouteFailureV1(ctx context.Context, p identity.Principal, media, bound string, r RouteFailureReport) (RouteFailureResult, error) {
	r.SessionID = media
	return s.reportRouteFailure(ctx, p, bound, r)
}

func (s *Service) reportRouteFailure(ctx context.Context, p identity.Principal, bound string, r RouteFailureReport) (RouteFailureResult, error) {
	var out RouteFailureResult
	if !validControlID(r.SessionID) || !routeFailureCode.MatchString(r.Code) || len(r.Detail) > 512 || p.Hash == "" || bound == "" {
		return out, ErrRouteFailure
	}
	var asset, strategy string
	err := s.db.QueryRowContext(ctx, `SELECT asset_id,COALESCE((SELECT d.strategy FROM playback_delivery_plans d WHERE d.session_id=playback_sessions.id),'') FROM playback_sessions WHERE id=? AND account_id=? AND profile_id=? AND `+sessionFamilyMatch, r.SessionID, p.AccountID, p.ProfileID, bound, bound).Scan(&asset, &strategy)
	if errors.Is(err, sql.ErrNoRows) {
		// Not this device's session, ended, or unknown: hidden, and never a
		// 401 (a valid sign-in must not be sent to refresh its token).
		return out, identity.ErrNotVisible
	}
	if err != nil {
		return out, err
	}
	out.Route = strategy
	switch DeliveryStrategy(strategy) {
	case DeliveryOriginal, DeliveryCopyRemux, DeliveryAudioConversion:
		out.Escalates = true
	default:
		// A full conversion is already the most compatible thing this server can
		// produce; there is nothing above it to escalate to.
		return out, nil
	}
	detail := publicAudioText(r.Detail, 200)
	_, err = dbwork.ExecWrite(ctx, s.db, dbwork.ClassEstablishedPlayback, `INSERT INTO playback_route_failures(session_hash,asset_id,route,code,detail,reported_at_ms) VALUES(?,?,?,?,?,?) ON CONFLICT(session_hash,asset_id,route) DO UPDATE SET code=excluded.code,detail=excluded.detail,reported_at_ms=excluded.reported_at_ms`, p.Hash, asset, strategy, r.Code, detail, time.Now().UnixMilli())
	return out, err
}

// routeFailuresTx reads the routes this device has reported broken for one file.
func routeFailuresTx(ctx context.Context, tx *sql.Tx, p identity.Principal, asset string) (map[DeliveryStrategy]RouteRejection, error) {
	if p.Hash == "" {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT route,code FROM (SELECT route,code,ROW_NUMBER() OVER(PARTITION BY route ORDER BY reported_at_ms DESC) AS rank FROM playback_route_failures WHERE (session_hash=? OR session_hash IN(SELECT original.token_hash FROM authorization_family_tokens original JOIN authorization_family_tokens caller ON caller.family_id=original.family_id WHERE caller.token_hash=?)) AND asset_id=? AND reported_at_ms>?  ) WHERE rank=1 LIMIT 4`, p.Hash, p.Hash, asset, time.Now().Add(-routeFailureTTL).UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out map[DeliveryStrategy]RouteRejection
	for rows.Next() {
		var route, code string
		if err = rows.Scan(&route, &code); err != nil {
			return nil, err
		}
		if out == nil {
			out = map[DeliveryStrategy]RouteRejection{}
		}
		out[DeliveryStrategy(route)] = RouteRejection{Code: code}
	}
	return out, rows.Err()
}
