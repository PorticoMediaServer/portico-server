package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"portico.local/server/internal/decoder"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/subtitles"
)

// AuthorizeSubtitles shares the established transactional identity/library gate.
// Exported only for startup composition; it does not introduce new authority.
func (d Dependencies) AuthorizeSubtitles(ctx context.Context, tx *sql.Tx, p identity.Principal, item string) (identity.Principal, error) {
	return d.playbackAuthorityTx(ctx, tx, p, item)
}
func subtitleHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// sessionNotVisible: the subtitle store answers "unauthorized" for a playback
// session the caller can't reach (not its own, ended, or unknown). The caller's
// credential was already accepted, so that is a hidden 404, never a 401 that
// would send a signed-in client to the sign-in screen (CD-51, NEW-28).
func sessionNotVisible(e error) error {
	if errors.Is(e, identity.ErrUnauthorized) {
		return identity.ErrNotVisible
	}
	return e
}

func subtitleFailure(w http.ResponseWriter, e error) {
	status, code, message := 503, "subtitle_unavailable", "Subtitles are unavailable. Refresh and try again."
	switch {
	// CD-51: a refused but valid session is 403 (or a hidden 404), never 401.
	case errors.Is(e, identity.ErrNotVisible):
		status, code, message = 404, "not_found", publicErrorMessage("not_found")
	case errors.Is(e, identity.ErrForbidden):
		status, code, message = 403, "forbidden", identity.ErrForbidden.Error()
	case errors.Is(e, identity.ErrUnauthorized):
		status, code, message = 401, "unauthorized", "Authentication is required."
	case errors.Is(e, sql.ErrNoRows):
		status, code, message = 404, "subtitle_not_found", "The subtitle resource was not found."
	case errors.Is(e, decoder.ErrConfinementUnavailable):
		status, code, message = 422, "subtitle_renderer_platform_unsupported", "This server platform has no configured confined decoder backend. Text subtitle delivery remains available."
	case errors.Is(e, subtitles.ErrRendererConfiguration), errors.Is(e, decoder.ErrInvalidConfiguration):
		status, code, message = 422, "subtitle_renderer_configuration", "Configure the server FFmpeg, FFprobe and confined decoder dependencies before rendering this subtitle."
	case errors.Is(e, subtitles.ErrConflict):
		status, code, message = 409, "subtitle_conflict", "Subtitles changed. Refresh before applying this edit."
	case errors.Is(e, subtitles.ErrOperation):
		status, code, message = 409, "subtitle_operation_conflict", "This operation identifier belongs to a different edit."
	case errors.Is(e, subtitles.ErrCapacity):
		status, code, message = 422, "subtitle_capacity", "The subtitle exceeds the supported size, track or cue limit."
	case errors.Is(e, subtitles.ErrUnsupported), errors.Is(e, subtitles.ErrText), errors.Is(e, subtitles.ErrTiming):
		status, code, message = 422, "subtitle_unsupported", "The subtitle format or timing cannot be rendered by this playback configuration. SRT, WebVTT, ASS/SSA, PGS and paired VobSub are supported within the documented limits."
	case errors.Is(e, subtitles.ErrInput):
		status, code, message = 400, "subtitle_invalid_input", "The subtitle request is invalid."
	}
	if status == 503 {
		w.Header().Set("Retry-After", "2")
	}
	write(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "retryable": status == 503}})
}
func subtitleDecode(w http.ResponseWriter, r *http.Request, out any) error {
	typ, params, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || typ != "application/json" || params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8") {
		return subtitles.ErrInput
	}
	r.Body = http.MaxBytesReader(w, r.Body, subtitles.MaxAssetBytes+16384)
	raw, e := io.ReadAll(r.Body)
	if e != nil {
		return subtitles.ErrCapacity
	}
	if !utf8.Valid(raw) {
		return subtitles.ErrInput
	}
	tokens := json.NewDecoder(bytes.NewReader(raw))
	if e = strictJSONValue(tokens, 0); e != nil {
		return subtitles.ErrInput
	}
	if _, e = tokens.Token(); e != io.EOF {
		return subtitles.ErrInput
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		return subtitles.ErrInput
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return subtitles.ErrInput
	}
	return nil
}
func (d Dependencies) subtitleRoutes(mux *http.ServeMux) {
	if d.Subtitles == nil {
		return
	}
	type handler func(http.ResponseWriter, *http.Request, identity.Principal) (any, error)
	register := func(pattern string, fn handler) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			subtitleHeaders(w)
			p, e := d.principal(r)
			if e != nil {
				subtitleFailure(w, e)
				return
			}
			out, e := fn(w, r, p)
			if e != nil {
				subtitleFailure(w, e)
				return
			}
			write(w, 200, out)
		})
	}
	register("GET /v1/items/{id}/subtitles", func(w http.ResponseWriter, r *http.Request, p identity.Principal) (any, error) {
		return d.Subtitles.List(r.Context(), p, r.PathValue("id"))
	})
	register("POST /v1/items/{id}/subtitles", func(w http.ResponseWriter, r *http.Request, p identity.Principal) (any, error) {
		var m subtitles.Mutation
		if e := subtitleDecode(w, r, &m); e != nil {
			return nil, e
		}
		if m.ResourceID != "" {
			return nil, subtitles.ErrInput
		}
		return d.Subtitles.Mutate(r.Context(), p, r.PathValue("id"), m)
	})
	register("PUT /v1/items/{id}/subtitles/{resource}", func(w http.ResponseWriter, r *http.Request, p identity.Principal) (any, error) {
		var m subtitles.Mutation
		if e := subtitleDecode(w, r, &m); e != nil {
			return nil, e
		}
		if m.ResourceID != "" && m.ResourceID != r.PathValue("resource") {
			return nil, subtitles.ErrInput
		}
		m.ResourceID = r.PathValue("resource")
		return d.Subtitles.Mutate(r.Context(), p, r.PathValue("id"), m)
	})
	register("DELETE /v1/items/{id}/subtitles/{resource}", func(w http.ResponseWriter, r *http.Request, p identity.Principal) (any, error) {
		var m subtitles.DeleteMutation
		if e := subtitleDecode(w, r, &m); e != nil {
			return nil, e
		}
		return d.Subtitles.Delete(r.Context(), p, r.PathValue("id"), r.PathValue("resource"), m)
	})
	register("POST /v1/items/{id}/subtitles/import", func(w http.ResponseWriter, r *http.Request, p identity.Principal) (any, error) {
		var m subtitles.ImportRequest
		if e := subtitleDecode(w, r, &m); e != nil {
			return nil, e
		}
		return d.Subtitles.Import(r.Context(), p, r.PathValue("id"), m)
	})
	register("POST /v1/items/{id}/subtitles/refresh", func(w http.ResponseWriter, r *http.Request, p identity.Principal) (any, error) {
		var m struct {
			SourceID string `json:"sourceId"`
		}
		if e := subtitleDecode(w, r, &m); e != nil {
			return nil, e
		}
		return d.Subtitles.Refresh(r.Context(), p, r.PathValue("id"), m.SourceID)
	})
	register("POST /v1/items/{id}/subtitles/remote", func(w http.ResponseWriter, r *http.Request, p identity.Principal) (any, error) {
		var m subtitles.RemoteRequest
		if e := subtitleDecode(w, r, &m); e != nil {
			return nil, e
		}
		return d.Subtitles.ImportRemote(r.Context(), p, r.PathValue("id"), m)
	})
	register("POST /v1/items/{id}/subtitles/search", func(w http.ResponseWriter, r *http.Request, p identity.Principal) (any, error) {
		var m subtitles.SearchRequest
		if e := subtitleDecode(w, r, &m); e != nil {
			return nil, e
		}
		return d.Subtitles.Search(r.Context(), p, r.PathValue("id"), m)
	})
	register("POST /v1/items/{id}/subtitles/apply", func(w http.ResponseWriter, r *http.Request, p identity.Principal) (any, error) {
		var m subtitles.ApplyRequest
		if e := subtitleDecode(w, r, &m); e != nil {
			return nil, e
		}
		return d.Subtitles.Apply(r.Context(), p, r.PathValue("id"), m)
	})
	// A v1 client names its v1 session and presentation generation; the plan
	// lives on the media session presenting it (playback_v1_presentation.go).
	register("GET /v1/items/{id}/playback/{session}/subtitles", func(w http.ResponseWriter, r *http.Request, p identity.Principal) (any, error) {
		presentation, v1, e := d.resolveV1Presentation(r.Context(), p, r.PathValue("session"))
		if e != nil {
			return nil, e
		}
		if !v1 {
			out, e := d.Subtitles.Plan(r.Context(), p, r.PathValue("id"), r.PathValue("session"))
			return out, sessionNotVisible(e)
		}
		out, e := d.Subtitles.Plan(r.Context(), presentation.as(p), r.PathValue("id"), presentation.media)
		if e == nil {
			out.SessionID, out.Generation = presentation.session, presentation.generation
		}
		return out, sessionNotVisible(e)
	})
	register("PUT /v1/items/{id}/playback/{session}/subtitles", func(w http.ResponseWriter, r *http.Request, p identity.Principal) (any, error) {
		var m subtitles.SelectRequest
		if e := subtitleDecode(w, r, &m); e != nil {
			return nil, e
		}
		m.ControlProof = r.Header.Get("X-Playback-Controller-Token")
		presentation, v1, e := d.resolveV1Presentation(r.Context(), p, r.PathValue("session"))
		if e != nil {
			return nil, e
		}
		if !v1 {
			out, e := d.Subtitles.Select(r.Context(), p, r.PathValue("id"), r.PathValue("session"), m)
			return out, sessionNotVisible(e)
		}
		// The client fences on the generation it holds, the v1 one; the media
		// session's own generation is what the selection is checked against.
		if m.Generation != presentation.generation {
			return nil, subtitles.ErrConflict
		}
		m.Generation, e = d.mediaGeneration(r.Context(), presentation.media)
		if e != nil {
			return nil, e
		}
		out, e := d.Subtitles.Select(r.Context(), presentation.as(p), r.PathValue("id"), presentation.media, m)
		if e == nil {
			out.SessionID, out.Generation = presentation.session, presentation.generation
		}
		return out, sessionNotVisible(e)
	})
	mux.HandleFunc("GET /v1/media/{grant}/subtitles/{resource}/{revision}", func(w http.ResponseWriter, r *http.Request) {
		subtitleHeaders(w)
		isVTT := strings.HasSuffix(r.PathValue("revision"), ".vtt")
		revisionText := strings.TrimSuffix(r.PathValue("revision"), ".vtt")
		revision, e := strconv.ParseInt(revisionText, 10, 64)
		if e != nil || revision < 1 || strconv.FormatInt(revision, 10) != revisionText {
			subtitleFailure(w, subtitles.ErrInput)
			return
		}
		_, p, item, e := d.Playback.ResolveGrantContext(r.Context(), r.PathValue("grant"))
		if e != nil {
			subtitleFailure(w, e)
			return
		}
		var session string
		e = d.DB.QueryRowContext(r.Context(), `SELECT id FROM playback_sessions WHERE grant_hash=?`, identity.Digest(r.PathValue("grant"))).Scan(&session)
		if e != nil {
			subtitleFailure(w, e)
			return
		}
		id := r.PathValue("resource")
		if isVTT {
			// One sealed document (at most 4 MiB), filtered to one 60s window.
			segment := -1
			if raw := r.URL.Query().Get("segment"); raw != "" {
				segment, e = strconv.Atoi(raw)
				if e != nil || segment < 0 || segment > 1440 {
					subtitleFailure(w, subtitles.ErrInput)
					return
				}
			}
			reader, offset, err := d.Subtitles.OpenManifestDocument(r.Context(), p, item, session, id, revision)
			if err != nil {
				subtitleFailure(w, err)
				return
			}
			defer reader.Close()
			data, err := subtitles.ManifestWebVTT(reader, offset, segment)
			if err != nil {
				subtitleFailure(w, err)
				return
			}
			if _, _, _, err = d.Playback.ResolveGrantContext(r.Context(), r.PathValue("grant")); err != nil {
				subtitleFailure(w, err)
				return
			}
			if err = d.Subtitles.CheckManifestDocument(r.Context(), p, item, session, id, revision); err != nil {
				subtitleFailure(w, err)
				return
			}
			w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Write(data)
			return
		}
		reader, e := d.Subtitles.OpenDocument(r.Context(), p, item, session, id, revision)
		if e != nil {
			subtitleFailure(w, e)
			return
		}
		defer reader.Close()
		check := func() error {
			_, _, _, e := d.Playback.ResolveGrantContext(r.Context(), r.PathValue("grant"))
			if e != nil {
				return e
			}
			return d.Subtitles.CheckDocument(r.Context(), p, item, session, id, revision)
		}
		serveSubtitleDocument(w, r, id, revision, reader, check)
	})
}

// subtitleETagMatches compares an If-None-Match header with this document's
// strong validator. A weak validator never matches: the comparison is strong.
func subtitleETagMatches(header, etag string) bool {
	if len(header) > 1024 {
		return false
	}
	for _, candidate := range strings.Split(header, ",") {
		if strings.TrimSpace(candidate) == etag {
			return true
		}
	}
	return false
}

// serveSubtitleDocument answers one authorised document read.
//
// Players reload this document every few seconds purely to extend their
// authorisation. check runs in full first, every time; only for a viewer who is
// still entitled to the document may the body be skipped. A stored revision is
// immutable, so resource and revision are a strong validator of its bytes. The
// response stays uncacheable: the validator travels in the header by hand, so
// nothing can ever be answered from a cache for a revoked viewer.
func serveSubtitleDocument(w http.ResponseWriter, r *http.Request, id string, revision int64, reader io.Reader, check func() error) {
	if e := check(); e != nil {
		subtitleFailure(w, e)
		return
	}
	etag := `"` + id + "." + strconv.FormatInt(revision, 10) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store")
	if match := r.Header.Get("If-None-Match"); match != "" && subtitleETagMatches(match, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method == "HEAD" {
		return
	}
	// No range cache or disk pathname escapes the grant boundary. A revocation
	// during an I/O call discards that chunk before it reaches the response.
	buffer := make([]byte, 32<<10)
	for {
		if check() != nil {
			return
		}
		n, readErr := reader.Read(buffer)
		if check() != nil {
			return
		}
		if n > 0 {
			if _, e := w.Write(buffer[:n]); e != nil {
				return
			}
		}
		if readErr != nil {
			return
		}
	}
}
