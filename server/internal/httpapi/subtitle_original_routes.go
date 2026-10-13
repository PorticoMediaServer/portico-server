package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"portico.local/apikit"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/subtitles"
)

// Binary payloads use the same grant registry and byte-range semantics as media.
type SubtitleBytesContract struct {
	ContentType string `json:"contentType"`
	ByteRange   string `json:"byteRange,omitempty"`
}
type SubtitleRefreshRequest struct {
	SourceID string `json:"sourceId"`
}

func registerSubtitleRaw[Req, Resp any](registry *apikit.Registry, pattern string, handler http.HandlerFunc) error {
	method, path, _ := strings.Cut(pattern, " ")
	access := apikit.ViewerLibrary
	lane := apikit.Default
	if strings.Contains(path, "/media/") {
		access = apikit.Public
		lane = apikit.Media
	}
	body := int64(0)
	if method != "GET" {
		body = subtitles.MaxAssetBytes + 16384
	}
	identifier := strings.NewReplacer("/", "_", "{", "", "}", "", "-", "_").Replace(strings.ToLower(method) + path)
	return apikit.RegisterRaw(registry, apikit.RawRoute[Req, Resp]{Metadata: apikit.Metadata{ID: identifier, Method: method, Path: path, Summary: "Read or change authorized subtitles", Access: access, Lane: lane, Cost: apikit.Constant, BodyLimit: body, Query: []string{"segment"}, Status: 200, Errors: []string{"unauthorized", "not_permitted", "not_found", "invalid_request", "internal"}}, Serve: handler})
}
func (d Dependencies) originalSubtitleRoute(w http.ResponseWriter, r *http.Request) {
	subtitleHeaders(w)
	revision, e := strconv.ParseInt(r.PathValue("revision"), 10, 64)
	if e != nil || revision < 1 || strconv.FormatInt(revision, 10) != r.PathValue("revision") {
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
	a, e := d.Subtitles.OriginalAsset(r.Context(), p, item, session, id, revision)
	if e != nil {
		subtitleFailure(w, e)
		return
	}
	check := func() error {
		if _, _, _, e := d.Playback.ResolveGrantContext(r.Context(), r.PathValue("grant")); e != nil {
			return e
		}
		return d.Subtitles.CheckOriginal(r.Context(), p, item, session, id, revision)
	}
	if e = check(); e != nil {
		subtitleFailure(w, e)
		return
	}
	base := "/v1/media/" + r.PathValue("grant") + "/subtitles/" + id + "/" + strconv.FormatInt(revision, 10)
	if strings.HasSuffix(r.URL.Path, "/fonts") {
		write(w, 200, subtitles.FontURLs(base, a))
		return
	}
	name := r.PathValue("file")
	data := a.Data
	contentType := "application/octet-stream"
	if font := r.PathValue("font"); font != "" {
		n, e := strconv.Atoi(font)
		if e != nil || n < 0 || n >= len(a.Fonts) {
			subtitleFailure(w, subtitles.ErrInput)
			return
		}
		name = "font-" + strconv.Itoa(n)
		data = a.Fonts[n].Data
		contentType = "font/ttf"
	} else {
		switch name {
		case "original.ass", "original.ssa":
			if a.Format != "ass" && a.Format != "ssa" {
				subtitleFailure(w, subtitles.ErrUnsupported)
				return
			}
			contentType = "text/x-ssa"
		case "original.sup":
			if a.Format != "pgs" {
				subtitleFailure(w, subtitles.ErrUnsupported)
				return
			}
		case "original.idx":
			if a.Format != "vobsub" {
				subtitleFailure(w, subtitles.ErrUnsupported)
				return
			}
			contentType = "text/plain"
		case "original.sub":
			if a.Format != "vobsub" {
				subtitleFailure(w, subtitles.ErrUnsupported)
				return
			}
			data = a.Companion
		case "original.mks":
			if a.Format != "mks" && a.Format != "dvb" {
				subtitleFailure(w, subtitles.ErrUnsupported)
				return
			}
		default:
			subtitleFailure(w, subtitles.ErrInput)
			return
		}
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("ETag", `"subtitle-original-`+id+`-`+strconv.FormatInt(revision, 10)+`-`+name+`"`)
	http.ServeContent(w, r, name, time.Time{}, &authorizedSubtitleBytes{Reader: bytes.NewReader(data), check: check})
}

type authorizedSubtitleBytes struct {
	*bytes.Reader
	check func() error
}

func (r *authorizedSubtitleBytes) Read(b []byte) (int, error) {
	if e := r.check(); e != nil {
		return 0, e
	}
	return r.Reader.Read(b[:min(len(b), 32<<10)])
}

var _ io.ReadSeeker = (*authorizedSubtitleBytes)(nil)
