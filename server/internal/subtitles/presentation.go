package subtitles

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strconv"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/mediaartifact"
)

type selectionPreparation struct {
	replace                  bool
	oldRender                string
	position, policyRevision int64
	ordinaryBurn             bool
	resource                 *Resource
}

func presentation(scope sessionScope, session, renderID string, position int64) *Presentation {
	url := "/v1/media/" + scope.grant
	mode := scope.mode
	if renderID == "ordinary" {
		url += "/master.m3u8"
		mode = "hls"
	} else if mode == "hls" {
		url += "/master.m3u8"
	}
	if mode == "remote" {
		mode = "direct"
	}
	return &Presentation{SessionID: session, Generation: scope.generation, StreamURL: url, Mode: mode, PositionUS: strconv.FormatInt(position, 10)}
}
func (s *Service) Select(ctx context.Context, p identity.Principal, item, session string, m SelectRequest) (PlaybackPlan, error) {
	if !validID(m.OperationID) || m.Generation < 1 || m.ExpectedRevision < 1 || (m.Mode != "off" && m.Mode != "track") {
		return PlaybackPlan{}, ErrInput
	}
	offset, e := Offset(m.OffsetUS)
	if e != nil {
		return PlaybackPlan{}, e
	}
	if m.Mode == "off" && (m.ResourceID != "" || m.ResourceRevision != 0 || offset != 0) || m.Mode == "track" && (!validID(m.ResourceID) || m.ResourceRevision < 1) {
		return PlaybackPlan{}, ErrInput
	}
	gated, p, e := s.tx(ctx, p, item)
	if e != nil {
		return PlaybackPlan{}, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	scope, e := sessionTx(ctx, tx, p, item, session)
	if e != nil {
		return PlaybackPlan{}, e
	}
	digest := requestDigest([]any{session, m})
	var oldDigest, oldSession string
	e = tx.QueryRowContext(ctx, `SELECT request_digest,session_id FROM subtitle_selection_operations WHERE actor=? AND operation_id=?`, actor(p), m.OperationID).Scan(&oldDigest, &oldSession)
	if e == nil {
		if oldDigest != digest || oldSession != session {
			return PlaybackPlan{}, ErrOperation
		}
		gated.Rollback()
		return s.selectCommit(ctx, p, item, session, m, nil)
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return PlaybackPlan{}, e
	}
	if scope.generation != m.Generation {
		return PlaybackPlan{}, ErrConflict
	}
	if e = PinSessionTx(ctx, tx, session, scope.generation); e != nil {
		return PlaybackPlan{}, e
	}
	var revision int64
	var currentID string
	var currentRevision, currentOffset int64
	if e = tx.QueryRowContext(ctx, `SELECT revision,resource_id,resource_revision,offset_us FROM playback_subtitle_state WHERE session_id=? AND generation=?`, session, scope.generation).Scan(&revision, &currentID, &currentRevision, &currentOffset); e != nil {
		return PlaybackPlan{}, e
	}
	if revision != m.ExpectedRevision {
		return PlaybackPlan{}, ErrConflict
	}
	prepared := &selectionPreparation{}
	e = tx.QueryRowContext(ctx, `SELECT render_id FROM playback_subtitle_presentations WHERE session_id=?`, session).Scan(&prepared.oldRender)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return PlaybackPlan{}, e
	}
	var resource Resource
	var reader *mediaartifact.Reader
	if m.Mode == "track" {
		resource, e = scanResource(tx.QueryRowContext(ctx, `SELECT `+resourceColumns+` FROM subtitle_resources r JOIN subtitle_revisions v ON v.resource_id=r.id WHERE r.id=? AND r.item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND v.revision=?`, m.ResourceID, item, m.ResourceRevision))
		if e != nil {
			return PlaybackPlan{}, e
		}
		if !visible(resource, p) || resource.SourceID != scope.source {
			return PlaybackPlan{}, identity.ErrUnauthorized
		}
		if resource.sourceSize != scope.size || resource.sourceModified != scope.modified || !scope.available {
			return PlaybackPlan{}, ErrConflict
		}
		var latest int64
		if e = tx.QueryRowContext(ctx, `SELECT current_revision FROM subtitle_resources WHERE id=?`, resource.ID).Scan(&latest); e != nil {
			return PlaybackPlan{}, e
		}
		if (resource.deleted || latest != resource.Revision) && (currentID != resource.ID || currentRevision != resource.Revision) {
			return PlaybackPlan{}, ErrConflict
		}
		if resource.Renderer == "burn_in" {
			if s.ChangeDeliveryTx == nil {
				return PlaybackPlan{}, ErrUnavailable
			}
			var enabled bool
			if e = tx.QueryRowContext(ctx, `SELECT transcoding_enabled,revision FROM playback_owner_policy WHERE singleton=1`).Scan(&enabled, &prepared.policyRevision); e != nil {
				return PlaybackPlan{}, e
			}
			if !enabled {
				return PlaybackPlan{}, ErrUnsupported
			}
			// Acquire a physical object lease while still holding exact source/resource
			// authority. Garbage collection cannot delete this preparation's revision.
			reader, e = s.objects.Open(ctx, mediaartifact.Object{Digest: resource.digest, Size: resource.size})
			if e != nil {
				return PlaybackPlan{}, e
			}
			defer reader.Close()
		}
	}
	prepared.replace = resource.Renderer == "burn_in" || prepared.oldRender != "" || scope.mode == "hls" && currentOffset != offset
	if prepared.replace {
		position, e := strconv.ParseInt(m.PositionUS, 10, 64)
		if e != nil || position < 0 || strconv.FormatInt(position, 10) != m.PositionUS {
			return PlaybackPlan{}, ErrInput
		}
		prepared.position = position
	}
	if s.ControlSelection != nil {
		if _, e = s.ControlSelection(ctx, gated, p, session, m, false); e != nil {
			return PlaybackPlan{}, e
		}
	}
	if e = gated.Commit(); e != nil {
		return PlaybackPlan{}, e
	}
	// Remote representation evidence is distinct from .strm descriptor size/mtime.
	if resource.sourceEvidence != "" {
		if s.SourceInputs == nil {
			return PlaybackPlan{}, ErrUnavailable
		}
		input, e := s.SourceInputs.OpenSubtitleInput(ctx, item, scope.source, session)
		if e != nil {
			return PlaybackPlan{}, e
		}
		same := input.Evidence() == resource.sourceEvidence
		valid := input.Validate(ctx)
		input.Close()
		if !same || valid != nil {
			return PlaybackPlan{}, ErrConflict
		}
	}
	if reader != nil {
		raw, e := io.ReadAll(io.LimitReader(reader, MaxAssetBytes+1))
		if e != nil {
			return PlaybackPlan{}, e
		}
		if len(raw) > MaxAssetBytes {
			return PlaybackPlan{}, ErrCapacity
		}
		_, e = DecodeRenderAsset(raw)
		if e != nil {
			return PlaybackPlan{}, e
		}
		prepared.ordinaryBurn = true
		prepared.resource = &resource
	}

	out, e := s.selectCommit(ctx, p, item, session, m, prepared)
	return out, e
}
