package livechannels

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/remotemedia"
)

// ReadSource bounds compressed and expanded bytes independently. The exact same
// pinned transport is used later for media and every nested manifest request.
func ReadSource(ctx context.Context, locator string, policy remotemedia.Policy, limit int64) ([]byte, error) {
	if limit < 1 || limit > MaxUploadBytes {
		return nil, ErrInvalid
	}
	c, e := remotemedia.New(ctx, locator, policy)
	if e != nil {
		return nil, sourceFetchError(e)
	}
	defer c.Close()
	resp, e := c.Open(ctx, "GET", "")
	if e != nil {
		return nil, sourceFetchError(e)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.ContentLength > limit {
		return nil, ErrSourceUnavailable
	}
	compressed := &io.LimitedReader{R: resp.Body, N: limit + 1}
	var reader io.Reader = compressed
	encoding := strings.TrimSpace(strings.ToLower(resp.Header.Get("Content-Encoding")))
	switch encoding {
	case "", "identity":
	case "gzip":
		g, e := gzip.NewReader(compressed)
		if e != nil {
			return nil, ErrSourceUnavailable
		}
		defer g.Close()
		reader = g
	default:
		return nil, ErrSourceUnavailable
	}
	raw, e := io.ReadAll(io.LimitReader(reader, limit+1))
	if e != nil || int64(len(raw)) > limit || compressed.N <= 0 {
		return nil, ErrSourceUnavailable
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return raw, nil
}

func (f SourceFetcher) fetch(ctx context.Context, c remoteConfig) (SourceInput, int, error) {
	d := c.Draft
	in := SourceInput{ID: d.ID, ExpectedRevision: d.ExpectedRevision, Name: d.Name, Mappings: append([]ChannelMapping{}, d.Mappings...)}
	p := remotemedia.Policy{Approvals: c.Approvals, Resolver: f.Resolver, ReadTimeout: 15 * time.Second}
	physical := 0
	switch d.Kind {
	case "m3u":
		b, e := ReadSource(ctx, d.Locator, p, MaxUploadBytes)
		if e != nil {
			return in, 0, e
		}
		in.Playlist = string(b)
	case "xtream":
		base, e := url.Parse(d.Locator)
		if e != nil || base.RawQuery != "" || base.User != nil {
			return in, 0, ErrInvalid
		}
		api := *base
		api.Path = strings.TrimSuffix(api.Path, "/") + "/player_api.php"
		q := url.Values{"username": {d.Username}, "password": {d.Password}, "action": {"get_live_streams"}}
		api.RawQuery = q.Encode()
		b, e := ReadSource(ctx, api.String(), p, MaxUploadBytes)
		if e != nil {
			return in, 0, e
		}
		var streams []struct {
			ID       json.Number     `json:"stream_id"`
			Name     string          `json:"name"`
			Number   json.Number     `json:"num"`
			EPG      string          `json:"epg_channel_id"`
			Category json.RawMessage `json:"category_id"`
		}
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.UseNumber()
		if dec.Decode(&streams) != nil || len(streams) == 0 || len(streams) > MaxChannels {
			return in, 0, ErrInvalid
		}
		var text strings.Builder
		text.WriteString("#EXTM3U\n")
		for _, s := range streams {
			sid, e := s.ID.Int64()
			if e != nil || sid <= 0 {
				return in, 0, ErrInvalid
			}
			key := "xtream-" + strconv.FormatInt(sid, 10)
			stream := *base
			stream.RawPath = ""
			stream.Path = strings.TrimSuffix(stream.Path, "/") + "/live/" + d.Username + "/" + d.Password + "/" + strconv.FormatInt(sid, 10) + ".ts"
			// Path components must not change route structure; URL's RawPath preserves
			// reserved characters in credentials without exposing them to consumers.
			stream.RawPath = strings.TrimSuffix(base.EscapedPath(), "/") + "/live/" + url.PathEscape(d.Username) + "/" + url.PathEscape(d.Password) + "/" + strconv.FormatInt(sid, 10) + ".ts"
			if !validPlaylistText(s.Name) || !validText(s.Name, 256) {
				return in, 0, ErrInvalid
			}
			fmt.Fprintf(&text, "#EXTINF:-1 tvg-id=\"%s\" tvg-chno=\"%s\",%s\n%s\n", key, s.Number.String(), s.Name, stream.String())
			if !hasMapping(in.Mappings, key) && s.EPG != "" {
				in.Mappings = append(in.Mappings, ChannelMapping{key, s.EPG})
			}
		}
		in.Playlist = text.String()
		if d.GuideURL == "" {
			guide := *base
			guide.Path = strings.TrimSuffix(guide.Path, "/") + "/xmltv.php"
			q.Del("action")
			guide.RawQuery = q.Encode()
			d.GuideURL = guide.String()
		}
	case "hdhomerun":
		base, e := url.Parse(d.Locator)
		if e != nil || base.RawQuery != "" || base.User != nil {
			return in, 0, ErrInvalid
		}
		discover := *base
		discover.Path = strings.TrimSuffix(base.Path, "/") + "/discover.json"
		b, e := ReadSource(ctx, discover.String(), p, 64<<10)
		if e != nil {
			return in, 0, e
		}
		var info struct {
			TunerCount int `json:"TunerCount"`
		}
		if json.Unmarshal(b, &info) != nil || info.TunerCount < 0 || info.TunerCount > 256 {
			return in, 0, ErrInvalid
		}
		physical = info.TunerCount
		lineup := *base
		lineup.Path = strings.TrimSuffix(base.Path, "/") + "/lineup.json"
		b, e = ReadSource(ctx, lineup.String(), p, MaxUploadBytes)
		if e != nil {
			return in, 0, e
		}
		var rows []struct {
			Number string `json:"GuideNumber"`
			Name   string `json:"GuideName"`
			URL    string `json:"URL"`
			DRM    int    `json:"DRM"`
		}
		if json.Unmarshal(b, &rows) != nil || len(rows) == 0 || len(rows) > MaxChannels {
			return in, 0, ErrInvalid
		}
		var text strings.Builder
		text.WriteString("#EXTM3U\n")
		for _, r := range rows {
			if r.DRM != 0 {
				continue
			}
			if !validText(r.Number, 32) || !validPlaylistText(r.Number) || !validText(r.Name, 256) || !validPlaylistText(r.Name) || !validLocator(r.URL) {
				return in, 0, ErrInvalid
			}
			// A lineup URL is not trusted merely because the device returned it. Media
			// access later passes the same policy and exact confirmed LAN scope again.
			fmt.Fprintf(&text, "#EXTINF:-1 tvg-id=\"hdhr-%s\" tvg-chno=\"%s\",%s\n%s\n", r.Number, r.Number, r.Name, r.URL)
		}
		in.Playlist = text.String()
	default:
		return in, 0, ErrInvalid
	}
	if d.GuideURL != "" {
		remaining := int64(MaxUploadBytes - len(in.Playlist))
		if remaining <= 0 {
			return in, 0, ErrInvalid
		}
		b, e := ReadSource(ctx, d.GuideURL, p, remaining)
		if e != nil {
			return in, 0, e
		}
		// A configured XMLTV response must be a document, not an empty success
		// response that would silently erase the last published guide.
		if strings.TrimSpace(string(b)) == "" {
			return in, 0, ErrInvalid
		}
		in.Guide = string(b)
	}
	return in, physical, nil
}
func validPlaylistText(s string) bool { return !strings.ContainsAny(s, "\"\r\n\x00") }
func hasMapping(ms []ChannelMapping, key string) bool {
	for _, m := range ms {
		if m.ChannelKey == key {
			return true
		}
	}
	return false
}

// Input is server-private. Callers must acquire a current source allocation and
// authorize the viewer/job before requesting it. No HTTP DTO embeds this type.
type Input struct {
	Locator                         string
	Policy                          remotemedia.Policy
	SourceID, ChannelID, Generation string
	Revision                        int64
}

func (s *Store) InputTx(ctx context.Context, tx *sql.Tx, sourceID, channelID, generation string) (Input, error) {
	var in Input
	var sealed []byte
	var active string
	e := tx.QueryRowContext(ctx, `SELECT s.revision,s.active_generation,c.locator_sealed FROM live_sources s JOIN live_channel_versions c ON c.generation_id=s.active_generation WHERE s.id=? AND c.channel_id=? AND s.state='active'`, sourceID, channelID).Scan(&in.Revision, &active, &sealed)
	if e != nil || active != generation {
		return in, ErrConflict
	}
	// AAD is exactly the binding used by the existing source store.
	in.Locator, e = s.unseal(sealed, channelID+":"+active)
	if e != nil {
		return in, e
	}
	var remote []byte
	e = tx.QueryRowContext(ctx, `SELECT sealed FROM live_remote_configs WHERE source_id=?`, sourceID).Scan(&remote)
	if e == nil {
		raw, err := s.unseal(remote, "remote:"+sourceID)
		if err != nil {
			return in, err
		}
		var c remoteConfig
		if json.Unmarshal([]byte(raw), &c) != nil {
			return in, ErrUnavailable
		}
		in.Policy.Approvals = c.Approvals
	} else if e != sql.ErrNoRows {
		return in, ErrUnavailable
	}
	in.SourceID, in.ChannelID, in.Generation = sourceID, channelID, generation
	in.Policy.ReadTimeout = 15 * time.Second
	return in, nil
}
