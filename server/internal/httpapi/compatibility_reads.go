package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"portico.local/apikit/apierror"
)

func foreignMediaSources(value any) []any {
	sources := []any{}
	for _, source := range compatObjects(value) {
		streams := []any{}
		for _, stream := range compatObjects(source["streams"]) {
			s := map[string]any{}
			for native, foreign := range map[string]string{"type": "Type", "codec": "Codec", "width": "Width", "height": "Height", "bitrate": "BitRate", "bitRate": "BitRate", "channels": "Channels", "language": "Language", "index": "Index"} {
				if v, ok := stream[native]; ok {
					s[foreign] = v
				}
			}
			streams = append(streams, s)
		}
		for _, kind := range []string{"video", "audio"} {
			v := compatObject(source[kind])
			if len(v) == 0 {
				continue
			}
			stream := map[string]any{"Type": strings.ToUpper(kind[:1]) + kind[1:]}
			for native, foreign := range map[string]string{"codec": "Codec", "width": "Width", "height": "Height", "bitRate": "BitRate", "channels": "Channels", "language": "Language"} {
				if x, ok := v[native]; ok {
					stream[foreign] = x
				}
			}
			streams = append(streams, stream)
		}
		out := map[string]any{"Id": source["id"], "MediaStreams": streams, "SupportsDirectPlay": false, "SupportsDirectStream": false, "SupportsTranscoding": false}
		for native, foreign := range map[string]string{"container": "Container", "size": "Size", "path": "Path", "bitRate": "Bitrate", "width": "Width", "height": "Height"} {
			if v, ok := source[native]; ok {
				out[foreign] = v
			}
		}
		if d := compatNumber(source["durationMs"]); d > 0 {
			out["RunTimeTicks"] = int64(d * 10000)
		}
		sources = append(sources, out)
	}
	return sources
}
func (d Dependencies) compatibilityItem(r *http.Request, item map[string]any) (map[string]any, error) {
	id, err := d.compatibilityID(r, "item", compatText(item["id"]))
	if err != nil {
		return nil, err
	}
	kind := compatText(item["kind"])
	foreignKind := map[string]string{"movie": "Movie", "show": "Series", "season": "Season", "episode": "Episode", "artist": "MusicArtist", "album": "MusicAlbum", "song": "Audio", "book": "Book", "audiobook_file": "Audio", "collection": "BoxSet"}[kind]
	if foreignKind == "" {
		foreignKind = "BaseItem"
	}
	providers := map[string]any{}
	for provider, value := range compatObject(item["providerIds"]) {
		name := map[string]string{"tmdb": "Tmdb", "tvdb": "Tvdb", "imdb": "Imdb", "anilist": "AniList", "musicbrainz": "MusicBrainz"}[provider]
		if name != "" {
			providers[name] = value
		}
	}
	out := map[string]any{"Id": id, "Name": item["title"], "Type": foreignKind, "IsFolder": kind == "show" || kind == "season" || kind == "album" || kind == "artist" || kind == "book", "ProviderIds": providers, "ImageTags": map[string]any{}, "MediaSources": foreignMediaSources(item["mediaSources"])}
	if item["available"] == false {
		out["LocationType"] = "Virtual"
	} else {
		out["LocationType"] = "FileSystem"
	}
	if v := item["year"]; v != nil {
		out["ProductionYear"] = v
	}
	if v := item["addedAt"]; v != nil && v != "" {
		out["DateCreated"] = v
	}
	if duration := compatNumber(item["durationSeconds"]); duration > 0 {
		out["RunTimeTicks"] = int64(duration * 10000000)
	} else if duration := compatNumber(item["durationMs"]); duration > 0 {
		out["RunTimeTicks"] = int64(duration * 10000)
	}
	paths, _ := item["paths"].([]any)
	if len(paths) > 0 {
		out["Path"] = paths[0]
	}
	if poster := compatText(item["posterUrl"]); poster != "" {
		digest := sha256.Sum256([]byte(poster))
		out["ImageTags"] = map[string]any{"Primary": hex.EncodeToString(digest[:8])}
	}
	if library := compatText(item["libraryId"]); library != "" {
		out["ParentId"], err = d.compatibilityID(r, "library", library)
		if err != nil {
			return nil, err
		}
	}
	ep := compatObject(item["episode"])
	for native, foreign := range map[string]string{"showId": "SeriesId", "seasonId": "SeasonId"} {
		if parent := compatText(ep[native]); parent != "" {
			out[foreign], err = d.compatibilityID(r, "item", parent)
			if err != nil {
				return nil, err
			}
		}
	}
	if v, ok := ep["number"]; ok {
		out["IndexNumber"] = v
	}
	if v, ok := ep["seasonNumber"]; ok {
		out["ParentIndexNumber"] = v
	}
	if v, ok := item["number"]; ok {
		out["IndexNumber"] = v
	}
	width, height := compatNumber(item["width"]), compatNumber(item["height"])
	for _, media := range foreignMediaSources(item["mediaSources"]) {
		for _, stream := range compatObjects(compatObject(media)["MediaStreams"]) {
			if compatNumber(stream["Width"]) > width {
				width = compatNumber(stream["Width"])
			}
			if compatNumber(stream["Height"]) > height {
				height = compatNumber(stream["Height"])
			}
		}
	}
	if width > 0 {
		out["Width"] = width
	}
	if height > 0 {
		out["Height"] = height
		out["IsHD"] = height >= 720
	}
	return out, nil
}
func compatibilityInteger(q url.Values, key string, defaultValue, max int) (int, error) {
	raw := q.Get(key)
	if raw == "" {
		return defaultValue, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > max {
		return 0, &apierror.Error{Code: "invalid_request"}
	}
	return n, nil
}
func (d Dependencies) compatibilityItemsQuery(r *http.Request, q url.Values, mode string) (url.Values, error) {
	native := url.Values{"limit": {"100"}}
	if ids := q.Get("ids"); ids != "" {
		all := strings.Split(ids, ",")
		if len(all) > 100 {
			return nil, &apierror.Error{Code: "invalid_request"}
		}
		values := []string{}
		for _, id := range all {
			v, err := d.compatibilityNativeID(r, "item", id)
			if err != nil {
				return nil, &apierror.Error{Code: "not_found"}
			}
			values = append(values, v)
		}
		native.Set("ids", strings.Join(values, ","))
	}
	if parent := q.Get("parentid"); parent != "" {
		if lib, err := d.compatibilityNativeID(r, "library", parent); err == nil {
			native.Set("libraryId", lib)
		} else if item, err := d.compatibilityNativeID(r, "item", parent); err == nil {
			native.Set("parentId", item)
		} else {
			return nil, &apierror.Error{Code: "not_found"}
		}
	}
	kinds := []string{}
	for _, k := range strings.Split(q.Get("includeitemtypes"), ",") {
		if k == "" {
			continue
		}
		mapped := map[string]string{"Movie": "movie", "Series": "show", "Season": "season", "Episode": "episode", "MusicArtist": "artist", "MusicAlbum": "album", "Audio": "song", "Book": "book", "Others": ""}[k]
		if mapped != "" {
			kinds = append(kinds, mapped)
		}
	}
	if len(kinds) > 0 {
		native.Set("kind", strings.Join(kinds, ","))
	}
	if years := q.Get("years"); years != "" {
		native.Set("year", years)
	}
	if search := q.Get("searchterm"); search != "" {
		native.Set("search", search)
	}
	if provider := q.Get("anyprovideridequals"); provider != "" {
		parts := strings.Split(provider, ",")
		if len(parts) != 1 {
			return nil, &apierror.Error{Code: "invalid_request"}
		}
		pair := strings.SplitN(parts[0], ".", 2)
		if len(pair) != 2 {
			return nil, &apierror.Error{Code: "invalid_request"}
		}
		native.Set("provider", strings.ToLower(pair[0]))
		native.Set("providerId", pair[1])
	}
	if q.Get("sortby") == "DateCreated" || strings.Contains(mode, "latest") {
		native.Set("sort", "added")
		native.Set("order", "desc")
	}
	if q.Get("mindatecreated") != "" {
		native.Set("addedAfter", q.Get("mindatecreated"))
	}
	if fields := strings.ToLower(q.Get("fields")); strings.Contains(fields, "path") || strings.Contains(fields, "mediasources") {
		native.Set("includeFiles", "true")
	}
	return native, nil
}

// Foreign offsets/totals are translated over bounded native cursor pages. The
// response streams the selected rows; it never builds a whole-library array or
// persists a query. When the native page publishes a total, no count walk is
// needed; otherwise the same read-only traversal computes a truthful total.
func (d Dependencies) compatibilityItems(w http.ResponseWriter, r *http.Request, q url.Values, mode string) {
	start, err := compatibilityInteger(q, "startindex", 0, 5000000)
	if err != nil {
		compatibilityFailure(w, err)
		return
	}
	limit, err := compatibilityInteger(q, "limit", 0, 5000000)
	if err != nil {
		compatibilityFailure(w, err)
		return
	}
	if q.Get("limit") == "" && strings.Contains(mode, "latest") {
		limit = 20
	}
	native, err := d.compatibilityItemsQuery(r, q, mode)
	if err != nil {
		compatibilityFailure(w, err)
		return
	}
	page, err := d.integrationCall(r, "GET", "/v1/integrations/items", native, nil)
	if err != nil {
		compatibilityFailure(w, err)
		return
	}
	latest := strings.Contains(mode, "latest")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	if latest {
		_, err = ioWrite(w, "[")
	} else {
		_, err = ioWrite(w, `{"Items":[`)
	}
	if err != nil {
		return
	}
	count, emitted := 0, 0
	knownTotal := int(compatNumber(page["total"]))
	hasTotal := page["total"] != nil
	for {
		for _, item := range compatObjects(page["items"]) {
			if count >= start && (limit == 0 || emitted < limit) {
				translated, e := d.compatibilityItem(r, item)
				if e != nil {
					abortCompatibilityStream(w)
					return
				}
				if emitted > 0 {
					if _, e = ioWrite(w, ","); e != nil {
						return
					}
				}
				if e = json.NewEncoder(w).Encode(translated); e != nil {
					return
				}
				emitted++
			}
			count++
		}
		if hasTotal && limit > 0 && emitted >= limit {
			count = knownTotal
			break
		}
		cursor := compatText(page["nextCursor"])
		if cursor == "" {
			break
		}
		if err = r.Context().Err(); err != nil {
			abortCompatibilityStream(w)
			return
		}
		native.Set("cursor", cursor)
		page, err = d.integrationCall(r, "GET", "/v1/integrations/items", native, nil)
		if err != nil {
			abortCompatibilityStream(w)
			return
		}
	}
	if hasTotal {
		count = knownTotal
	}
	if latest {
		_, _ = ioWrite(w, "]")
	} else {
		_, _ = fmt.Fprintf(w, `],"StartIndex":%d,"TotalRecordCount":%d}`, start, count)
	}
}
func ioWrite(w http.ResponseWriter, s string) (int, error) { return w.Write([]byte(s)) }
func abortCompatibilityStream(_ http.ResponseWriter)       { panic(http.ErrAbortHandler) }
