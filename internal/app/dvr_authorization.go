package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var errDVRLiveTVReferenceDenied = errors.New("DVR Live TV reference is invalid or unavailable to this profile")

type authorizedDVRLiveTVReference struct {
	SourceID  string
	ChannelID string
	ProgramID string
}

func (s *Server) resolveAuthorizedDVRLiveTVReference(ctx context.Context, user User, sourceID, channelID, programID string, allowSourceOnly, requirePlayableChannel bool) (authorizedDVRLiveTVReference, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ref := authorizedDVRLiveTVReference{
		SourceID: strings.TrimSpace(sourceID), ChannelID: strings.TrimSpace(channelID), ProgramID: strings.TrimSpace(programID),
	}
	if ref.ProgramID != "" {
		var programSourceID, programChannelID string
		if err := s.queryUserRow(ctx, `SELECT source_id, COALESCE(channel_id, '') FROM live_tv_programs WHERE id = ?`, ref.ProgramID).Scan(&programSourceID, &programChannelID); err != nil {
			return authorizedDVRLiveTVReference{}, errDVRLiveTVReferenceDenied
		}
		if ref.SourceID != "" && ref.SourceID != programSourceID || ref.ChannelID != "" && ref.ChannelID != programChannelID {
			return authorizedDVRLiveTVReference{}, errDVRLiveTVReferenceDenied
		}
		ref.SourceID, ref.ChannelID = programSourceID, programChannelID
	}
	if ref.SourceID == "" {
		return authorizedDVRLiveTVReference{}, errDVRLiveTVReferenceDenied
	}
	var sourceEnabled int
	if err := s.queryUserRow(ctx, `SELECT enabled FROM live_tv_sources WHERE id = ?`, ref.SourceID).Scan(&sourceEnabled); err != nil || requirePlayableChannel && sourceEnabled != 1 {
		return authorizedDVRLiveTVReference{}, errDVRLiveTVReferenceDenied
	}
	if ref.ChannelID == "" {
		if allowSourceOnly {
			return ref, nil
		}
		return authorizedDVRLiveTVReference{}, errDVRLiveTVReferenceDenied
	}
	var channelSourceID, streamURL string
	var channelEnabled int
	if err := s.queryUserRow(ctx, `SELECT source_id, enabled, stream_url FROM live_tv_channels WHERE id = ?`, ref.ChannelID).Scan(&channelSourceID, &channelEnabled, &streamURL); err != nil || channelSourceID != ref.SourceID || requirePlayableChannel && (channelEnabled != 1 || strings.TrimSpace(streamURL) == "") {
		return authorizedDVRLiveTVReference{}, errDVRLiveTVReferenceDenied
	}
	policy, err := s.userLiveTVChannelPolicyContext(ctx, accountIDForUser(user))
	if err != nil || !liveTVChannelAllowedByPolicy(ref.ChannelID, policy) || !liveTVChannelAllowedByPolicy(ref.ChannelID, user.ChannelPolicy) {
		return authorizedDVRLiveTVReference{}, errDVRLiveTVReferenceDenied
	}
	return ref, nil
}

func (s *Server) currentDVRUser(accountID, profileID string) (User, error) {
	principal, err := s.resolveRequestPrincipalContext(context.Background(), accountID, profileID)
	if err != nil {
		return User{}, err
	}
	user := User{ID: accountID, AccountID: accountID, ProfileID: profileID}
	applyRequestPrincipal(&user, principal)
	user = s.hydratePlaybackVisibilityUserContext(context.Background(), user)
	if !canScheduleDVR(user) || !canViewDVR(user) {
		return User{}, errDVRLiveTVReferenceDenied
	}
	return user, nil
}

func (s *Server) dvrRecordingChannelAllowed(user User, recording DVRRecording) bool {
	if strings.TrimSpace(recording.ChannelID) == "" {
		return false
	}
	_, err := s.resolveAuthorizedDVRLiveTVReference(context.Background(), user, recording.SourceID, recording.ChannelID, "", false, false)
	return err == nil
}

// authorizeMappedDVRMediaForUser closes the generic media-route bypass. DVR
// imports remain ordinary media for indexing and presentation, but their bytes
// retain recording ownership and current Live TV channel-policy semantics.
func (s *Server) authorizeMappedDVRMediaForUser(ctx context.Context, user User, mediaID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var recordingID, accountID, profileID, sourceID, channelID, status string
	err := s.queryUserRow(ctx, `
		SELECT r.id, r.user_id, COALESCE(NULLIF(r.profile_id, ''), r.user_id), r.source_id,
			COALESCE(r.channel_id, ''), r.status
		FROM dvr_recording_media mapping
		JOIN live_tv_recordings r ON r.id = mapping.recording_id
		WHERE mapping.media_id = ?`, strings.TrimSpace(mediaID)).Scan(
		&recordingID, &accountID, &profileID, &sourceID, &channelID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		var mediaType string
		if mediaErr := s.queryUserRow(ctx, `SELECT type FROM media_items WHERE id = ?`, strings.TrimSpace(mediaID)).Scan(&mediaType); mediaErr == nil && mediaType == "recording" {
			// A recorded-TV item without its ownership mapping is an incomplete
			// upgrade, never an ordinary library item. Fail closed until backfilled.
			return errDVRLiveTVReferenceDenied
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !canViewDVR(user) || !hasPermission(user, "playMedia") || profileID != viewerProfileID(user) || (status != "complete" && status != "completed") {
		return errDVRLiveTVReferenceDenied
	}
	if _, err := s.resolveAuthorizedDVRLiveTVReference(ctx, user, sourceID, channelID, "", false, false); err != nil {
		return errDVRLiveTVReferenceDenied
	}
	_ = recordingID
	_ = accountID
	return nil
}

func (s *Server) pruneUnauthorizedScheduledDVRRecordings(user User, ruleID string) {
	rows, err := s.queryBackgroundRead(context.Background(), `
		SELECT id, source_id, COALESCE(channel_id, ''), program_id
		FROM live_tv_recordings WHERE rule_id = ? AND status = 'scheduled'`, ruleID)
	if err != nil {
		return
	}
	type candidate struct{ id, sourceID, channelID, programID string }
	candidates := []candidate{}
	for rows.Next() {
		var next candidate
		if rows.Scan(&next.id, &next.sourceID, &next.channelID, &next.programID) == nil {
			candidates = append(candidates, next)
		}
	}
	_ = rows.Close()
	for _, candidate := range candidates {
		if _, err := s.resolveAuthorizedDVRLiveTVReference(context.Background(), user, candidate.sourceID, candidate.channelID, candidate.programID, false, true); err != nil {
			_, _ = s.execBackgroundWrite(context.Background(), `DELETE FROM live_tv_recordings WHERE id = ? AND status = 'scheduled'`, candidate.id)
		}
	}
}

func isDVRReferenceDenied(err error) bool {
	return errors.Is(err, errDVRLiveTVReferenceDenied) || errors.Is(err, sql.ErrNoRows)
}
