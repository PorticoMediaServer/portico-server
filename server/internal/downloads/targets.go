package downloads

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"path"
	"portico.local/server/internal/preparedmedia"
	"strconv"
	"strings"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/playback"
)

// source is everything downloads needs to know about the one file behind an
// item: where it is, how big it is, what it contains and whether the library
// can currently see it.
type source struct {
	Item, Library, Kind string
	Asset, Path         string
	Size, ModifiedNS    int64
	Container           string
	VideoCodec          string
	AudioCodec          string
	Height              int
	Duration            float64
	Available           bool
	Parts               int
}

func audioKind(kind string) bool { return kind == "song" || kind == "audiobook_file" }

// The audio recipe has one budget, and every ladder rung resolves to it for an
// audio source. Estimating from the recipe rather than from the rung keeps the
// size honest whichever rung a client names.
const audioRecipeBitrateBPS = 192_000

// canonicalAudioRung is the rung an audio item publishes. The ladder is a video
// vocabulary; an audio source has one optimized target, and this is the rung
// whose audio budget equals the recipe's, so the published estimate and the
// delivered bytes agree.
const canonicalAudioRung = "1080p"

// resolveSource reads the single source file for an item. A multi-part item has
// no single artifact to hand a client, so it is refused here with a reason code
// rather than silently downloading one part.
func resolveSource(ctx context.Context, tx *sql.Tx, item string) (source, error) {
	var s source
	var available int
	var kindInt int
	e := tx.QueryRowContext(ctx, `SELECT cl.library_id,e.kind,a.token,a.path,a.size,a.modified_ns,a.container,a.video_codec,a.audio_codec,a.height,a.duration,link.available,
 (SELECT count(*) FROM catalog_asset_links p WHERE p.entity_id=e.id)
 FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id JOIN catalog_asset_links link ON link.entity_id=e.id JOIN catalog_assets a ON a.id=link.asset_id
 WHERE e.public_id=pid_blob(?) ORDER BY link.part_index,a.token LIMIT 1`, item).Scan(&s.Library, &kindInt, &s.Asset, &s.Path, &s.Size, &s.ModifiedNS, &s.Container, &s.VideoCodec, &s.AudioCodec, &s.Height, &s.Duration, &available, &s.Parts)
	if e != nil {
		return s, e
	}
	s.Kind, _ = compactcatalog.Kind(kindInt).Name()
	s.Item, s.Available = item, available != 0
	return s, nil
}

// sourceReason turns a resolution outcome into a published reason code.
func sourceReason(s source, e error) string {
	switch {
	case errors.Is(e, sql.ErrNoRows):
		return ReasonItemDeleted
	case e != nil:
		return ReasonFailed
	case s.Parts != 1:
		return ReasonSourceMissing
	case !s.Available:
		return ReasonSourceMissing
	}
	return ""
}

// preparedProfile maps a published quality ladder rung onto the immutable
// prepared-media recipe that can satisfy it. The ladder is a delivery vocabulary
// and the recipes are an encoder vocabulary; downloads is the only place the two
// have to meet, so the mapping lives here and is published in the API document.
//
//	audio item                → audio-aac-v3
//	video rung               → portable-{height}-v3
func preparedProfile(kind string, rung playback.QualityPreset) string {
	if audioKind(kind) {
		return "audio-aac-v3"
	}
	return "portable-" + strconv.Itoa(rung.Height) + "-v3"
}

// estimateBytes is bitrate times duration, and the caller publishes it as an
// estimate. Only the original's size is exact before the artifact exists.
func estimateBytes(quality string, s source) (int64, bool) {
	if quality == QualityOriginal {
		return s.Size, false
	}
	rung, ok := playback.LadderPreset(quality)
	if !ok || s.Duration <= 0 || math.IsNaN(s.Duration) || math.IsInf(s.Duration, 0) {
		return 0, true
	}
	recipe, err := preparedmedia.ProfileByID(preparedProfile(s.Kind, rung))
	if err != nil {
		return 0, true
	}
	bits := float64((recipe.VideoKbps + recipe.AudioKbps) * 1000)
	// Container overhead is real and small; 3% keeps the estimate honest about
	// erring high rather than promising a client less than it will receive.
	estimate := int64(bits / 8 * s.Duration * 1.03)
	if estimate < 1 {
		estimate = 1
	}
	return estimate, true
}

// artifactDelivery names the bytes for the client's filesystem. The extension
// follows the container, and the file name carries the preparation id so two
// downloads of one item at different qualities never collide on disk.
func artifactDelivery(container, preparation string) (string, string) {
	kind := "application/octet-stream"
	extension := container
	switch container {
	case "mp4", "m4v":
		kind, extension = "video/mp4", "mp4"
	case "m4a", "m4b":
		kind, extension = "audio/mp4", container
	case "mkv":
		kind, extension = "video/x-matroska", "mkv"
	case "webm":
		kind, extension = "video/webm", "webm"
	case "mp3":
		kind, extension = "audio/mpeg", "mp3"
	case "flac":
		kind, extension = "audio/flac", "flac"
	case "aac":
		kind, extension = "audio/aac", "aac"
	case "ogg", "opus":
		kind, extension = "audio/ogg", container
	case "wav":
		kind, extension = "audio/wav", "wav"
	case "":
		extension = "bin"
	}
	if extension == "" {
		extension = "bin"
	}
	// path.Base, never filepath.Base: this is a download file name in a URL and
	// a header, not a path on this server's filesystem.
	return kind, path.Base(preparation) + "." + extension
}

// expandTargets resolves one request into the ordered item list it names.
// Exactly one target form is accepted; naming two is a client bug, not a merge.
// A container (show, season, album, book, playlist) is not a target here: it is
// a durable request (POST /v1/downloads/requests) resolved in keyset pages with
// no size limit (PERF-S23), so this synchronous batch never truncates one.
func expandTargets(ctx context.Context, tx *sql.Tx, r Request) ([]string, string, error) {
	forms := 0
	origin := ""
	if r.MediaID != "" {
		forms, origin = forms+1, OriginItem
	}
	if len(r.MediaIDs) > 0 {
		forms, origin = forms+1, OriginItems
	}
	if r.NextAfterMediaID != "" {
		forms, origin = forms+1, OriginNext
	}
	if forms != 1 {
		return nil, "", ErrInput
	}
	switch origin {
	case OriginItem:
		if !validID.MatchString(r.MediaID) {
			return nil, "", ErrInput
		}
		return []string{r.MediaID}, origin, nil
	case OriginItems:
		if len(r.MediaIDs) > MaxBatchTargets {
			return nil, "", ErrInput
		}
		seen := map[string]bool{}
		out := []string{}
		for _, id := range r.MediaIDs {
			if !validID.MatchString(id) {
				return nil, "", ErrInput
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
		if len(out) == 0 {
			return nil, "", ErrInput
		}
		return out, origin, nil
	default:
		if !validID.MatchString(r.NextAfterMediaID) {
			return nil, "", ErrInput
		}
		out, e := nextEpisode(ctx, tx, r.NextAfterMediaID)
		return out, origin, e
	}
}

// nextEpisode is the one episode that follows the named one: the next number in
// the same season, else the first episode of the next season of the same show.
// Absolute numbering is followed on its own axis. It answers one item, never a
// tail, so an auto-next download stays one episode ahead rather than silently
// pulling a season.
func nextEpisode(ctx context.Context, tx *sql.Tx, after string) ([]string, error) {
	var show, numbering string
	var season sql.NullString
	var number int
	e := tx.QueryRowContext(ctx, `SELECT pid(show.public_id),e.numbering,pid(season.public_id),e.number FROM catalog_entities item JOIN catalog_episodes e ON e.entity_id=item.id JOIN catalog_entities show ON show.id=e.show_id LEFT JOIN catalog_entities season ON season.id=e.season_id WHERE item.public_id=pid_blob(?)`, after).Scan(&show, &numbering, &season, &number)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	var next string
	if numbering == "absolute" {
		e = tx.QueryRowContext(ctx, `SELECT pid(item.public_id) FROM catalog_entities show JOIN catalog_episodes e ON e.show_id=show.id JOIN catalog_entities item ON item.id=e.entity_id WHERE show.public_id=pid_blob(?) AND e.numbering='absolute' AND e.number>? ORDER BY e.number LIMIT 1`, show, number).Scan(&next)
	} else {
		e = tx.QueryRowContext(ctx, `SELECT pid(item.public_id) FROM catalog_entities season CROSS JOIN catalog_episodes e INDEXED BY catalog_episodes_season ON e.season_id=season.id CROSS JOIN catalog_entities item ON item.id=e.entity_id WHERE season.public_id=pid_blob(?) AND e.numbering='seasonal' AND e.number>? ORDER BY e.number LIMIT 1`, season.String, number).Scan(&next)
		if errors.Is(e, sql.ErrNoRows) {
			e = tx.QueryRowContext(ctx, `SELECT pid(item.public_id) FROM catalog_entities show JOIN catalog_episodes e ON e.show_id=show.id JOIN catalog_seasons s ON s.entity_id=e.season_id JOIN catalog_entities item ON item.id=e.entity_id
 WHERE show.public_id=pid_blob(?) AND e.numbering='seasonal' AND s.number>(SELECT next.number FROM catalog_seasons next JOIN catalog_entities current ON current.id=next.entity_id WHERE current.public_id=pid_blob(?))
 ORDER BY s.number,e.number LIMIT 1`, show, season.String).Scan(&next)
		}
	}
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	return []string{next}, nil
}

// libraryOf is used by the routes layer's per-item authorization check.
func libraryOf(ctx context.Context, tx *sql.Tx, item string) (string, error) {
	var library string
	e := tx.QueryRowContext(ctx, `SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.public_id=pid_blob(?)`, item).Scan(&library)
	return library, e
}

// qualityLabel renders a rung for a human-facing list without a client having to
// keep its own table of ladder names.
func qualityLabel(quality string) string {
	if quality == QualityOriginal {
		return "Original"
	}
	if rung, ok := playback.LadderPreset(quality); ok {
		return rung.Label
	}
	return strings.ToUpper(quality)
}
