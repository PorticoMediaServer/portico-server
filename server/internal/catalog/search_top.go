package catalog

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
	"unicode"
	"unicode/utf16"
)

// Search has explicit restart semantics. The generic browse reader permits a
// live keyset after publication; Search may not silently mix that revision.
func (s *Service) decodeSearchCursor(raw string, scope cursorScope, rev ContentRevision) (cursorValue, error) {
	c, err := s.decodeRevisionCursor(raw, scope, rev)
	if err != nil {
		return c, err
	}
	payload, _, _ := strings.Cut(raw, ".")
	data, _ := base64.RawURLEncoding.DecodeString(payload)
	var signed cursorValue
	if json.Unmarshal(data, &signed) != nil {
		return c, ErrCursor
	}
	if signed.Expires < time.Now().Unix() {
		return c, ErrStaleContinuation
	}
	if c.CatalogRevision != rev.Catalog || c.ViewerRevision != rev.Viewer {
		return c, ErrStaleContinuation
	}
	return c, nil
}

// This is the existing presentation rule over returned preview candidates,
// never a second search, catalogue-wide guess or channel/programme selection.
func (s *Service) searchTop(r SearchRequest, out *SearchEnvelope) error {
	q := searchTopPlain(r.Q)
	if searchTopLength(q) < 2 {
		return nil
	}
	exact, starts := []SearchTopResult{}, []SearchTopResult{}
	for _, g := range out.Groups {
		if g.Status != "success" {
			continue
		}
		switch g.ID {
		case "movies", "shows", "artists", "albums", "books", "people":
		default:
			continue
		}
		for _, e := range g.Items {
			title := searchTopPlain(e.Title)
			t := SearchTopResult{Group: g.ID, Entry: e}
			if title == q {
				exact = append(exact, t)
			} else if searchTopLength(q) >= 3 && strings.HasPrefix(title, q+" ") {
				starts = append(starts, t)
			}
		}
	}
	var top *SearchTopResult
	if len(exact) == 1 {
		top = &exact[0]
	} else if len(exact) == 0 && len(starts) == 1 {
		top = &starts[0]
	}
	if top == nil {
		return nil
	}
	// One selected entity gets the banner facts. All other rows retain their
	// bounded card projection. A show reuses its real page's next-up authority.
	if top.Group != "people" {
		entries := []ContentEntry{top.Entry}
		if err := s.homeEnrich(r.Profile, entries); err != nil {
			return err
		}
		top.Entry = entries[0]
		if top.Group == "movies" {
			items, err := s.mediaPage(r.Profile, []string{top.Entry.ID}, true)
			if err != nil {
				return err
			}
			if len(items) == 1 {
				top.Entry.Overview = items[0].Overview
			}
		}
		if top.Group == "shows" {
			page, err := s.ShowWorkspace(ShowWorkspaceRequest{Viewer: r.Viewer, Profile: r.Profile, ViewerFence: r.ViewerFence, ServerID: r.ServerID, Library: top.Entry.LibraryID, ShowID: top.Entry.ID, Limit: 1, SeasonLimit: 1})
			if err != nil {
				return err
			}
			top.Entry.Overview = page.Show.Overview
			top.Entry.Genres = page.Show.Genres
			top.Entry.ContentRating = page.Show.ContentRating
			top.Entry.SeasonCount = &page.SeasonTotalCount
			top.Primary = page.NextUp
		}
		if top.Group == "artists" || top.Group == "albums" || top.Group == "books" {
			entity, err := s.listeningEntity(ContentRequest{Viewer: r.Viewer, Profile: r.Profile, Library: top.Entry.LibraryID, View: top.Entry.Kind, EntityID: top.Entry.ID}, top.Entry.Title)
			if err != nil {
				return err
			}
			if entity != nil {
				entity.Navigation = top.Entry.Navigation
				entity.Watchlisted = top.Entry.Watchlisted
				entity.MatchedOn = top.Entry.MatchedOn
				top.Entry = *entity
			}
		}
	}
	out.Top = top
	return nil
}

// Exact normalization of the existing headless Top rule: punctuation separates
// words here (unlike FTS tokens), and length uses JavaScript UTF-16 units.
func searchTopPlain(v string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(v) {
		if r >= 0x300 && r <= 0x36f {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
func searchTopLength(v string) int {
	n := 0
	for _, r := range v {
		n += utf16.RuneLen(r)
	}
	return n
}
