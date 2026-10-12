package httpapi

import (
	"net/http"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

// serveOriginal serves a presentation's original file with Range: through the
// remote-source relay when the source is remote, else from local storage. check
// re-authorizes the grant while bytes flow. decode is true only for the version 2
// audio route, whose grant may be a prepared (private) presentation's.
func (d Dependencies) serveOriginal(w http.ResponseWriter, r *http.Request, grant, aid string, decode bool, check func() error) {
	response, close, remote, remoteErr := d.Playback.OpenRemote(r.Context(), grant, r.Method, r.Header.Get("Range"), decode)
	if remoteErr != nil {
		failure(w, remoteErr)
		return
	}
	if remote {
		defer close()
		reader := &guardedReader{Reader: response.Body, check: newStreamAuthority(check).check}
		body := withRollingDeadline(w)
		defer body.release()
		_ = remotemedia.CopyResponse(body, response, reader)
		if reader.readError != nil && r.Context().Err() == nil {
			d.Playback.RemoteReadFailed(grant)
		}
		return
	}
	if d.Storage == nil {
		failure(w, storage.ErrPlaybackSource)
		return
	}
	f, e := assets.OpenIsolatedPlayback(r.Context(), d.DB, d.Storage, "", identity.Digest(grant))
	if e != nil {
		failure(w, e)
		return
	}
	defer f.Close()

	w.Header().Set("Cache-Control", "private, no-store")
	var container, video string
	_ = dbwork.QueryRow(r.Context(), d.DB, `SELECT container,video_codec FROM catalog_assets WHERE token=?`, aid).Scan(&container, &video)
	w.Header().Set("Content-Type", originalContentType(container, video))
	body := withRollingDeadline(w)
	defer body.release()
	http.ServeContent(body, r, "media."+container, f.Modified, &guardedReadSeeker{ReadSeeker: f, check: newStreamAuthority(check).check})
}

// originalContentType names an original file's media type from its container.
// Every response says nosniff, so the label is all a player that trusts it has.
// QuickTime keeps the MP4 label it has always been served with: it is the same
// box structure, and a browser that plays it refuses the QuickTime type.
func originalContentType(container, video string) string {
	if video == "" {
		return audioContentType(container)
	}
	switch container {
	case "mp4", "m4v", "mov":
		return "video/mp4"
	case "mkv":
		return "video/x-matroska"
	case "webm":
		return "video/webm"
	case "avi":
		return "video/x-msvideo"
	case "ts", "m2ts", "mts":
		return "video/mp2t"
	case "mpg", "mpeg", "vob":
		return "video/mpeg"
	}
	return "application/octet-stream"
}

// audioContentType names an audio container's media type.
func audioContentType(container string) string {
	switch container {
	case "mp3":
		return "audio/mpeg"
	case "aac", "adts":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "ogg", "opus":
		return "audio/ogg"
	case "wav":
		return "audio/wav"
	case "aiff":
		return "audio/aiff"
	}
	return "audio/mp4"
}
