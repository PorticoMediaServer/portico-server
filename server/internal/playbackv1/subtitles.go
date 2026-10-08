package playbackv1

import (
	"context"
	"errors"
	"strconv"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/subtitles"
)

// subtitleResult is what a subtitle selection did to a presentation: the
// sidecar files the client loads, and, when the selection changed the bytes
// (burn-in on, or off again), the presentation's new stream (a new grant and
// media generation on the same playback session).
type subtitleResult struct {
	files    []SubtitleFile
	replaced *subtitles.Presentation
}

// applySubtitles selects subtitles on one presentation through the subtitle
// service (spec §5.5) at positionMs.
//
//   - nil request: the automatic choice the presentation was created with
//     (profile language, forced tracks) stands.
//   - trackId null: off (a burned presentation goes back to its plain stream).
//   - a text track: a WebVTT sidecar URL under the presentation's grant;
//     delivery "burn" for a text track is refused (only image and styled
//     tracks burn).
//   - an image or styled track: burned into the video (a transcode, and a new
//     presentation), unless the device renders it itself (delivery
//     embeddedClient), when nothing is selected on the server; delivery
//     "sidecar" for such a track is refused. Burn-in needs converting allowed.
func (s *Service) applySubtitles(ctx context.Context, p identity.Principal, item, media string, req *SubtitleRequest, offsetMs, positionMs int64, operation string) (subtitleResult, error) {
	out := subtitleResult{files: []SubtitleFile{}}
	if s.Subtitles == nil || media == "" {
		return out, nil
	}
	plan, err := s.Subtitles.Plan(ctx, p, item, media)
	if err != nil {
		return out, nil // A title without a subtitle catalog plays without subtitles.
	}
	if req != nil {
		m := subtitles.SelectRequest{OperationID: operation, Generation: plan.Generation, ExpectedRevision: plan.Revision, Mode: "off", OffsetUS: "0", PositionUS: strconv.FormatInt(max(positionMs, 0)*1000, 10)}
		if req.TrackID != TrackOff && req.TrackID != "" {
			var chosen *subtitles.Resource
			for i := range plan.Resources {
				if plan.Resources[i].ID == string(req.TrackID) {
					chosen = &plan.Resources[i]
				}
			}
			if chosen == nil || !chosen.Enabled {
				return out, &FieldError{Path: "subtitles.trackId"}
			}
			burns := chosen.Renderer == "burn_in"
			switch {
			case req.Delivery == "burn" && !burns, req.Delivery == "sidecar" && burns:
				return out, &FieldError{Path: "subtitles.delivery"}
			case burns && req.Delivery == "embeddedClient":
				// The device draws the track from the stream itself.
				return out, nil
			}
			m.Mode, m.ResourceID, m.ResourceRevision = "track", chosen.ID, chosen.Revision
			m.OffsetUS = strconv.FormatInt(offsetMs*1000, 10)
		}
		if plan, err = s.Subtitles.Select(ctx, p, item, media, m); err != nil {
			if errors.Is(err, subtitles.ErrUnsupported) {
				return out, playback.ErrTranscodingDisabled
			}
			return out, &FieldError{Path: "subtitles.trackId"}
		}
		out.replaced = plan.Presentation
	}
	if plan.Selected != nil && plan.DocumentURL != "" && plan.Renderer == "external_text" {
		out.files = append(out.files, SubtitleFile{TrackID: plan.Selected.ID, Format: "webvtt", URL: plan.DocumentURL})
	}
	return out, nil
}
