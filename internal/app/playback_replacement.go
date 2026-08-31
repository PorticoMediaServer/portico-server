package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/database"
)

var errPlaybackReplacementRequired = errors.New("active client playback requires an exact replacement envelope")

func isPlaybackActiveAuthorityConstraint(err error) bool {
	message := strings.ToLower(err.Error())
	return database.ClassifyError(err) == database.ErrorKindConstraint &&
		(strings.Contains(message, "idx_playback_sessions_one_active_client_authority") ||
			(strings.Contains(message, "playback_sessions.user_id") && strings.Contains(message, "playback_sessions.profile_id") && strings.Contains(message, "playback_sessions.client_instance_id")))
}

// playbackReplacementPlan is the only start-route bridge into terminal
// authority transfer. A target constructor may build the reserved successor,
// but it cannot expose it until commit atomically accepts the source terminal.
type playbackReplacementPlan struct {
	SourceSessionID string
	RequestID       string
	Fingerprint     string
	Terminal        PlaybackTerminalEvent
	Claim           playbackHandoffClaim
	Committed       *PlaybackResponse
	Active          bool
}

func replacementSessionID(plan *playbackReplacementPlan) string {
	if plan == nil {
		return ""
	}
	return strings.TrimSpace(plan.Claim.ReplacementSessionID)
}

func playbackReplacementTargetFingerprint(kind, targetID string, request any) string {
	encoded, _ := json.Marshal(struct {
		Kind     string `json:"kind"`
		TargetID string `json:"targetId"`
		Request  any    `json:"request"`
	}{Kind: strings.TrimSpace(kind), TargetID: strings.TrimSpace(targetID), Request: request})
	return hashToken(string(encoded))
}

func (s *Server) preparePlaybackReplacement(ctx context.Context, user User, clientInstanceID, targetKind, targetID string, targetRequest any, replacement *PlaybackReplacementRequest) (playbackReplacementPlan, *playbackStartHTTPError) {
	return s.preparePlaybackReplacementForSource(ctx, user, clientInstanceID, clientInstanceID, targetKind, targetID, targetRequest, replacement)
}

// preparePlaybackReplacementForSource permits one deliberately narrow
// cross-client transfer: an already Server-authorized controller source to a
// registered first-party receiver target. All ordinary starts pass the same
// client ID for source and target through preparePlaybackReplacement above.
func (s *Server) preparePlaybackReplacementForSource(ctx context.Context, user User, targetClientInstanceID, sourceClientInstanceID, targetKind, targetID string, targetRequest any, replacement *PlaybackReplacementRequest) (playbackReplacementPlan, *playbackStartHTTPError) {
	clientInstanceID := normalizePlaybackClientInstanceID(targetClientInstanceID)
	sourceClientInstanceID = normalizePlaybackClientInstanceID(sourceClientInstanceID)
	profileID := viewerProfileID(user)
	var activeIDs []string
	if clientInstanceID != "" {
		rows, err := s.queryUserRead(ctx, `
			SELECT id FROM playback_sessions
			WHERE user_id = ? AND profile_id = ? AND client_instance_id = ?
				AND ended_at = '' AND state NOT IN ('stopped', 'handoff_pending')
			ORDER BY started_at, id`, accountIDForUser(user), profileID, clientInstanceID)
		if err != nil {
			return playbackReplacementPlan{}, &playbackStartHTTPError{status: 500, code: "playback_replacement_failed", message: "Unable to inspect current playback authority."}
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return playbackReplacementPlan{}, &playbackStartHTTPError{status: 500, code: "playback_replacement_failed", message: "Unable to inspect current playback authority."}
			}
			activeIDs = append(activeIDs, id)
		}
		if err := rows.Err(); err != nil {
			return playbackReplacementPlan{}, &playbackStartHTTPError{status: 500, code: "playback_replacement_failed", message: "Unable to inspect current playback authority."}
		}
	}

	if replacement == nil {
		if len(activeIDs) > 0 {
			return playbackReplacementPlan{}, &playbackStartHTTPError{status: 409, code: "replacement_required", message: "This client already owns active playback. Supply its exact replacement authority envelope."}
		}
		return playbackReplacementPlan{}, nil
	}
	if clientInstanceID == "" {
		return playbackReplacementPlan{}, &playbackStartHTTPError{status: 400, code: "replacement_client_instance_required", message: "clientInstanceId is required when replacing playback."}
	}
	sourceSessionID := strings.TrimSpace(replacement.SourceSessionID)
	if sourceSessionID == "" {
		return playbackReplacementPlan{}, &playbackStartHTTPError{status: 400, code: "replacement_source_required", message: "replacement.sourceSessionId is required."}
	}
	if !validPlaybackAuthorityRequestID(replacement.RequestID) {
		return playbackReplacementPlan{}, &playbackStartHTTPError{status: 400, code: "replacement_request_id_invalid", message: "replacement.requestId must be 8 to 128 letters, numbers, periods, underscores, colons, or hyphens."}
	}
	if terminalErr := validatePlaybackTerminalEvent(replacement.PreviousTerminal); terminalErr != nil {
		return playbackReplacementPlan{}, &playbackStartHTTPError{status: terminalErr.status, code: terminalErr.code, message: "replacement.previousTerminal." + terminalErr.message}
	}
	if replacement.ExpectedQueueRevision != nil && *replacement.ExpectedQueueRevision < 0 || replacement.ExpectedPlaybackRevision != nil && *replacement.ExpectedPlaybackRevision < 0 {
		return playbackReplacementPlan{}, &playbackStartHTTPError{status: 400, code: "replacement_revision_invalid", message: "Replacement revisions must be zero or greater."}
	}
	fingerprint := playbackReplacementTargetFingerprint(targetKind, targetID, targetRequest)
	committed, claim, err := s.consumeDirectPlaybackHandoff(ctx, user, sourceSessionID, replacement.RequestID, fingerprint, replacement.ExpectedQueueRevision, replacement.ExpectedPlaybackRevision)
	if err != nil {
		var restoreRequired *playbackReplacementRestoreRequiredError
		switch {
		case errors.Is(err, errPreparedHandoffConflict):
			return playbackReplacementPlan{}, &playbackStartHTTPError{status: 409, code: "replacement_request_conflict", message: "This source already accepted a different replacement request."}
		case errors.Is(err, errPlaybackHandoffInProgress):
			return playbackReplacementPlan{}, &playbackStartHTTPError{status: 409, code: "handoff_in_progress", message: "This replacement is already committing. Retry the same request."}
		case errors.Is(err, errPlaybackReplacementRevisionConflict):
			return playbackReplacementPlan{}, &playbackStartHTTPError{status: 409, code: "handoff_source_revision_conflict", message: "Playback queue or source selection changed before replacement authority was reserved."}
		case errors.Is(err, errPlaybackReplacementSourceInactive):
			return playbackReplacementPlan{}, &playbackStartHTTPError{status: 409, code: "replacement_source_inactive", message: "The source playback authority is no longer active."}
		case errors.Is(err, errPlaybackReplacementAuthorizationChanged):
			return playbackReplacementPlan{}, &playbackStartHTTPError{status: 409, code: "playback_replacement_scope_changed", message: "Playback authorization changed. Reconcile active playback before creating a new replacement."}
		case errors.As(err, &restoreRequired):
			return playbackReplacementPlan{}, &playbackStartHTTPError{status: 409, code: "playback_replacement_committed_restore_required", message: "Playback replacement committed, but its original credentials are no longer safe to replay. Restore the active session for fresh credentials.", details: map[string]any{"replacementSessionId": restoreRequired.ReplacementSessionID, "outcome": "committed"}}
		case errors.Is(err, sql.ErrNoRows):
			return playbackReplacementPlan{}, &playbackStartHTTPError{status: 409, code: "replacement_source_inactive", message: "The source playback authority is no longer active."}
		default:
			return playbackReplacementPlan{}, &playbackStartHTTPError{status: 500, code: "playback_replacement_failed", message: "Unable to reserve playback authority transfer."}
		}
	}
	if sourceClientInstanceID == "" || normalizePlaybackClientInstanceID(claim.ClientInstanceID) != sourceClientInstanceID {
		if committed == nil {
			s.rollbackDirectPlaybackHandoff(context.Background(), sourceSessionID, replacement.RequestID, fingerprint, claim.ID)
		}
		return playbackReplacementPlan{}, &playbackStartHTTPError{status: 409, code: "replacement_source_conflict", message: "replacement.sourceSessionId is not owned by this client instance."}
	}
	return playbackReplacementPlan{
		SourceSessionID: sourceSessionID,
		RequestID:       strings.TrimSpace(replacement.RequestID), Fingerprint: fingerprint,
		Terminal: replacement.PreviousTerminal, Claim: claim, Committed: committed, Active: committed == nil,
	}, nil
}

func (s *Server) rollbackPlaybackReplacement(plan playbackReplacementPlan) {
	if !plan.Active {
		return
	}
	s.rollbackDirectPlaybackHandoff(context.Background(), plan.SourceSessionID, plan.RequestID, plan.Fingerprint, plan.Claim.ID)
}

func (s *Server) commitPlaybackReplacement(ctx context.Context, user User, plan playbackReplacementPlan, playback PlaybackResponse) error {
	if !plan.Active {
		return nil
	}
	return s.commitDirectPlaybackHandoff(ctx, user, plan.SourceSessionID, plan.Terminal, plan.RequestID, plan.Fingerprint, plan.Claim, playback)
}

func playbackReplacementCommitHTTPError(err error) *playbackStartHTTPError {
	switch {
	case errors.Is(err, errPlaybackGenerationStale):
		return &playbackStartHTTPError{status: 409, code: "playback_generation_stale", message: "Playback progress authority changed. Retry with the current generation."}
	case errors.Is(err, errPlaybackEventSequenceStale):
		return &playbackStartHTTPError{status: 409, code: "playback_event_sequence_stale", message: "The replacement terminal event is not newer than accepted playback state."}
	case errors.Is(err, errPlaybackReplacementRevisionConflict):
		return &playbackStartHTTPError{status: 409, code: "handoff_source_revision_conflict", message: "Playback queue or source selection changed before replacement committed."}
	case errors.Is(err, errPlaybackReplacementSourceInactive):
		return &playbackStartHTTPError{status: 409, code: "replacement_source_inactive", message: "The source playback authority is no longer active."}
	case errors.Is(err, errPlaybackReplacementAuthorizationChanged):
		return &playbackStartHTTPError{status: 409, code: "playback_replacement_scope_changed", message: "Playback authorization changed before replacement committed."}
	case errors.Is(err, errPlaybackTerminalDurationMismatch):
		return &playbackStartHTTPError{status: 409, code: "playback_terminal_duration_mismatch", message: "Completed playback duration does not match the server-authoritative media duration."}
	case errors.Is(err, sql.ErrNoRows):
		return &playbackStartHTTPError{status: 409, code: "replacement_source_inactive", message: "The source playback authority is no longer active."}
	default:
		return &playbackStartHTTPError{status: 500, code: "playback_replacement_failed", message: "Unable to commit playback authority transfer."}
	}
}
