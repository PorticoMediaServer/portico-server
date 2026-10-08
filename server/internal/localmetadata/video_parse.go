package localmetadata

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/metadataprovider"
	"strconv"
	"strings"
)

type nfoNode struct {
	name, text string
	attrs      map[string]string
	children   []*nfoNode
}

func (n *nfoNode) values(name string) []*nfoNode {
	out := []*nfoNode{}
	for _, c := range n.children {
		if c.name == name {
			out = append(out, c)
		}
	}
	return out
}
func (n *nfoNode) value(name string) string {
	for _, c := range n.children {
		if c.name == name {
			return strings.TrimSpace(c.text)
		}
	}
	return ""
}
func nfoNumber(s string, max int) (*int, error) {
	if s == "" {
		return nil, nil
	}
	n, e := strconv.Atoi(s)
	if e != nil || n < 0 || n > max {
		return nil, errors.New("invalid NFO number")
	}
	return &n, nil
}
func ParseVideoNFO(raw []byte, anime bool) ([]assets.VideoNFO, error) {
	if len(raw) == 0 || len(raw) > 256<<10 {
		return nil, errors.New("NFO exceeds budget")
	}
	d := xml.NewDecoder(bytes.NewReader(raw))
	d.Strict = true
	stack := []*nfoNode{}
	roots := []*nfoNode{}
	nodes := 0
	for {
		token, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, errors.New("invalid NFO XML")
		}
		switch t := token.(type) {
		case xml.Directive:
			return nil, errors.New("NFO declarations are not allowed")
		case xml.ProcInst:
			if t.Target != "xml" || nodes != 0 {
				return nil, errors.New("NFO processing instruction is not allowed")
			}
		case xml.StartElement:
			nodes++
			if nodes > 4096 || len(stack) >= 16 || len(t.Attr) > 16 {
				return nil, errors.New("NFO structure exceeds budget")
			}
			n := &nfoNode{name: strings.ToLower(t.Name.Local), attrs: map[string]string{}}
			for _, a := range t.Attr {
				n.attrs[strings.ToLower(a.Name.Local)] = a.Value
			}
			if len(stack) == 0 {
				roots = append(roots, n)
				if len(roots) > 8 {
					return nil, errors.New("too many NFO records")
				}
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, errors.New("invalid NFO nesting")
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				n := stack[len(stack)-1]
				n.text += string(t)
				if len(n.text) > 65536 {
					return nil, errors.New("NFO field exceeds budget")
				}
			} else if strings.TrimSpace(string(t)) != "" {
				return nil, errors.New("NFO contains non-XML content")
			}
		}
	}
	if len(stack) != 0 || len(roots) == 0 {
		return nil, errors.New("incomplete NFO")
	}
	out := []assets.VideoNFO{}
	sum := sha256.Sum256(raw)
	for _, n := range roots {
		kind := map[string]string{"movie": "movie", "tvshow": "show", "episodedetails": "episode"}[n.name]
		if kind == "" {
			return nil, errors.New("unsupported video NFO root")
		}
		v := assets.VideoNFO{Kind: kind, Digest: hex.EncodeToString(sum[:]), Title: n.value("title"), OriginalTitle: n.value("originaltitle"), SortTitle: n.value("sorttitle"), Overview: n.value("plot"), Tagline: n.value("tagline"), Date: n.value("premiered"), Edition: n.value("edition"), Certification: n.value("mpaa")}
		if v.Overview == "" {
			v.Overview = n.value("outline")
		}
		if v.Date == "" {
			v.Date = n.value("aired")
		}
		if v.Edition == "" {
			v.Edition = n.value("editiontitle")
		}
		year, e := nfoNumber(n.value("year"), 9999)
		if e != nil {
			return nil, e
		}
		if year != nil {
			v.Year = *year
		}
		if v.Season, e = nfoNumber(n.value("season"), 9999); e != nil {
			return nil, e
		}
		if v.Episode, e = nfoNumber(n.value("episode"), 1000000); e != nil {
			return nil, e
		}
		if v.Absolute, e = nfoNumber(n.value("absolute_number"), 1000000); e != nil {
			return nil, e
		}
		if v.AdvisoryRuntimeMinutes, e = nfoNumber(n.value("runtime"), 10080); e != nil {
			return nil, e
		}
		if len(v.Title) > 2048 || len(v.OriginalTitle) > 2048 || len(v.SortTitle) > 2048 || len(v.Edition) > 512 || len(v.Tagline) > 4096 || len(v.Certification) > 128 {
			return nil, errors.New("NFO field exceeds budget")
		}
		for _, g := range n.values("genre") {
			if len(v.Genres) >= 64 || len(g.text) > 200 {
				return nil, errors.New("NFO genres exceed budget")
			}
			v.Genres = append(v.Genres, strings.TrimSpace(g.text))
		}
		for _, g := range n.values("studio") {
			if len(v.Studios) >= 64 || len(g.text) > 200 {
				return nil, errors.New("NFO studios exceed budget")
			}
			v.Studios = append(v.Studios, strings.TrimSpace(g.text))
		}
		for _, set := range n.values("set") {
			v.Collection = set.value("name")
			if v.Collection == "" {
				v.Collection = strings.TrimSpace(set.text)
			}
			if len(v.Collection) > 2048 {
				return nil, errors.New("NFO collection exceeds budget")
			}
			break
		}
		ids := map[string]string{}
		put := func(provider, id string) error {
			provider = strings.ToLower(strings.TrimSpace(provider))
			id = strings.TrimSpace(id)
			if provider != "tmdb" && provider != "tvdb" && provider != "anilist" {
				return nil
			}
			typ := kind
			if provider == "anilist" {
				if !anime || kind == "episode" {
					return errors.New("NFO identity names wrong entity kind")
				}
				typ = "anime"
			}
			value := metadataprovider.ScreenID{Provider: provider, Type: typ, ID: id}
			if !metadataprovider.ValidScreenID(value) {
				return errors.New("invalid typed NFO provider identity")
			}
			if prior := ids[provider]; prior != "" {
				if prior != id {
					return errors.New("conflicting NFO identities")
				}
				return nil
			}
			ids[provider] = id
			v.IDs = append(v.IDs, value)
			return nil
		}
		for _, id := range n.values("uniqueid") {
			if e = put(id.attrs["type"], id.text); e != nil {
				return nil, e
			}
		}
		for _, p := range []string{"tmdb", "tvdb", "anilist"} {
			if id := n.value(p + "id"); id != "" {
				if e = put(p, id); e != nil {
					return nil, e
				}
			}
		}
		for _, actor := range n.values("actor") {
			if len(v.Credits) >= 100 {
				return nil, errors.New("NFO credits exceed budget")
			}
			name, role := actor.value("name"), actor.value("role")
			if name == "" {
				continue
			}
			if len(name) > 300 || len(role) > 500 {
				return nil, errors.New("NFO credit exceeds budget")
			}
			v.Credits = append(v.Credits, metadataprovider.ScreenCredit{Name: name, Role: role, Department: "Acting", Ordinal: len(v.Credits)})
		}
		out = append(out, v)
	}
	return out, nil
}
