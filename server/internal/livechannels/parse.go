package livechannels

import (
	"bufio"
	"encoding/xml"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var attribute = regexp.MustCompile(`([a-zA-Z0-9_-]+)="([^"]*)"`)

func validText(v string, max int) bool {
	return v != "" && len(v) <= max && utf8.ValidString(v) && !strings.ContainsFunc(v, unicode.IsControl)
}
func validLocator(v string) bool {
	if !validText(v, 8192) {
		return false
	}
	u, e := url.Parse(v)
	return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}

// Parse uploaded data only. Advertised logo URLs are recorded for the
// background importer; parsing never fetches them.
func parse(in SourceInput) (parsedSource, error) {
	var out parsedSource
	if !validText(in.Name, 120) || len(in.Playlist) == 0 || len(in.Playlist)+len(in.Guide) > MaxUploadBytes || in.TunerCount < 0 || in.TunerCount > 256 || in.ExpectedRevision < 0 {
		return out, ErrInvalid
	}
	scan := bufio.NewScanner(strings.NewReader(in.Playlist))
	scan.Buffer(make([]byte, 4096), 16384)
	first := true
	pending := false
	var ch parsedChannel
	keys := map[string]bool{}
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			if line != "#EXTM3U" && !strings.HasPrefix(line, "#EXTM3U ") {
				return out, ErrInvalid
			}
			continue
		}
		if strings.HasPrefix(line, "#EXTINF:") {
			if pending {
				return out, ErrInvalid
			}
			comma := -1
			quoted := false
			for i, r := range line {
				if r == '"' {
					quoted = !quoted
				}
				if r == ',' && !quoted {
					comma = i
					break
				}
			}
			if comma < 0 || quoted {
				return out, ErrInvalid
			}
			ch = parsedChannel{name: strings.TrimSpace(line[comma+1:])}
			for _, m := range attribute.FindAllStringSubmatch(line[:comma], -1) {
				switch m[1] {
				case "tvg-id":
					ch.key = m[2]
				case "tvg-chno":
					ch.number = m[2]
				case "group-title":
					ch.group = m[2]
				case "tvg-logo":
					if validLocator(m[2]) {
						ch.logo = m[2]
					}
				}
			}
			if !validText(ch.key, 256) || !validText(ch.name, 256) || (ch.number != "" && !validText(ch.number, 32)) || (ch.group != "" && !validText(ch.group, 120)) {
				return out, ErrInvalid
			}
			pending = true
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if !pending || !validLocator(line) {
			return out, ErrInvalid
		}
		ch.locator = line
		pending = false
		if keys[ch.key] { // Duplicate identity is collapsed only when every field agrees.
			same := false
			for _, old := range out.channels {
				if old.key == ch.key && old == ch {
					same = true
					break
				}
			}
			if !same {
				return out, ErrInvalid
			}
			continue
		}
		keys[ch.key] = true
		out.channels = append(out.channels, ch)
		if len(out.channels) > MaxChannels {
			return out, ErrInvalid
		}
	}
	if scan.Err() != nil || pending || len(out.channels) == 0 {
		return out, ErrInvalid
	}
	channelGuide := map[string]string{}
	for key := range keys {
		channelGuide[key] = key
	}
	mapped := map[string]bool{}
	for _, m := range in.Mappings {
		if !keys[m.ChannelKey] || mapped[m.ChannelKey] || !validText(m.GuideKey, 256) {
			return out, ErrInvalid
		}
		mapped[m.ChannelKey] = true
		channelGuide[m.ChannelKey] = m.GuideKey
	}
	mapping := map[string][]string{}
	for _, ch := range out.channels {
		mapping[channelGuide[ch.key]] = append(mapping[channelGuide[ch.key]], ch.key)
	}
	if strings.TrimSpace(in.Guide) == "" {
		return out, nil
	}
	if e := validateGuideXML(in.Guide); e != nil {
		return out, e
	}
	dec := xml.NewDecoder(strings.NewReader(in.Guide))
	depth := 0
	root := false
	closed := false
	seen := map[string]parsedProgramme{}
	icons := map[string]bool{}
	for {
		tok, e := dec.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return out, ErrInvalid
		}
		switch t := tok.(type) {
		case xml.Directive:
			return out, ErrInvalid
		case xml.StartElement:
			if closed {
				return out, ErrInvalid
			}
			depth++
			if depth == 1 {
				if t.Name.Local != "tv" {
					return out, ErrInvalid
				}
				root = true
				continue
			}
			if depth != 2 {
				return out, ErrInvalid
			}
			if t.Name.Local == "channel" {
				if e = dec.Skip(); e != nil {
					return out, ErrInvalid
				}
				depth--
				continue
			}
			if t.Name.Local != "programme" {
				return out, ErrInvalid
			}
			var row struct {
				Channel     string   `xml:"channel,attr"`
				Start       string   `xml:"start,attr"`
				Stop        string   `xml:"stop,attr"`
				ID          string   `xml:"id,attr"`
				Title       []string `xml:"title"`
				Description []string `xml:"desc"`
				Icons       []struct {
					Src string `xml:"src,attr"`
				} `xml:"icon"`
				Episodes []struct {
					System string `xml:"system,attr"`
					Value  string `xml:",chardata"`
				} `xml:"episode-num"`
				Series          string    `xml:"series-id"`
				New             *struct{} `xml:"new"`
				PreviouslyShown *struct{} `xml:"previously-shown"`
				SubTitle        []string  `xml:"sub-title"`
				Categories      []string  `xml:"category"`
				Date            string    `xml:"date"`
				Ratings         []struct {
					System string `xml:"system,attr"`
					Value  string `xml:"value"`
				} `xml:"rating"`
				StarRatings []struct {
					Value string `xml:"value"`
				} `xml:"star-rating"`
				Live     *struct{} `xml:"live"`
				Premiere *struct{} `xml:"premiere"`
			}
			if e = dec.DecodeElement(&row, &t); e != nil {
				return out, ErrInvalid
			}
			depth--
			channelKeys := mapping[row.Channel]
			start, e1 := time.Parse("20060102150405 -0700", row.Start)
			end, e2 := time.Parse("20060102150405 -0700", row.Stop)
			if e1 != nil || e2 != nil || !end.After(start) || end.Sub(start) > 7*24*time.Hour || len(row.Title) == 0 || !validText(row.Title[0], 512) {
				return out, ErrInvalid
			}
			// Validate the provider document before filtering its rows. An invalid
			// timestamp in an unmapped row is not a successful guide refresh.
			if !validText(row.Channel, 256) {
				return out, ErrInvalid
			}
			// The provider's programme thumbnail is the first <icon> with a
			// usable locator. An invalid icon is ignored, never a parse error.
			// At most 5000 distinct icon URLs are kept per generation; further
			// new URLs are dropped while repeats of a kept URL are retained.
			icon := ""
			for _, ic := range row.Icons {
				if len(ic.Src) <= 2048 && validLocator(ic.Src) {
					icon = ic.Src
					break
				}
			}
			if icon != "" && !icons[icon] {
				if len(icons) >= 5000 {
					icon = ""
				} else {
					icons[icon] = true
				}
			}
			for _, channelKey := range channelKeys {
				p := parsedProgramme{channel: channelKey, title: row.Title[0], start: start.UTC(), end: end.UTC(), lineage: "interval-only", newEvidence: "unknown", icon: icon}
				if row.Series != "" {
					if !validText(row.Series, 256) {
						return out, ErrInvalid
					}
					p.series = row.Series
				}
				for _, ep := range row.Episodes {
					if ep.System == "dd_progid" && validText(strings.TrimSpace(ep.Value), 256) {
						p.episode = strings.TrimSpace(ep.Value)
						if p.series == "" && len(p.episode) > 10 {
							p.series = p.episode[:10]
						}
					}
				}
				if row.New != nil && row.PreviouslyShown != nil {
					return out, ErrInvalid
				}
				if row.New != nil {
					p.newEvidence = "new"
				}
				if row.PreviouslyShown != nil {
					p.newEvidence = "repeat"
				}
				if len(row.Description) > 0 && validText(row.Description[0], 4096) {
					p.description = row.Description[0]
				}
				p.facts = xmltvFacts(row.SubTitle, row.Episodes, row.Categories, row.Date, row.Ratings, row.StarRatings, row.Live != nil, row.Premiere != nil)
				p.key = channelKey + "\x00" + p.start.Format(time.RFC3339)
				if row.ID != "" {
					if !validText(row.ID, 256) {
						return out, ErrInvalid
					}
					p.key = channelKey + "\x00id:" + row.ID
					p.lineage = "provider-id"
				}
				if old, ok := seen[p.key]; ok {
					if old != p {
						return out, ErrInvalid
					}
					continue
				}
				seen[p.key] = p
				out.programmes = append(out.programmes, p)
				if len(out.programmes) > MaxProgrammes {
					return out, ErrInvalid
				}
			}
		case xml.EndElement:
			depth--
			if depth == 0 {
				closed = true
			}
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return out, ErrInvalid
			}
		}
	}
	if !root || !closed || depth != 0 {
		return out, ErrInvalid
	}
	sort.Slice(out.programmes, func(i, j int) bool {
		a, b := out.programmes[i], out.programmes[j]
		if a.channel != b.channel {
			return a.channel < b.channel
		}
		return a.start.Before(b.start)
	})
	for i := 1; i < len(out.programmes); i++ {
		a, b := out.programmes[i-1], out.programmes[i]
		if a.channel == b.channel && a.end.After(b.start) {
			return out, ErrInvalid
		}
	}
	return out, nil
}
func PreviewSource(in SourceInput) (Preview, error) {
	data, e := parse(in)
	if e != nil {
		return Preview{}, e
	}
	p := Preview{Channels: len(data.channels), Programmes: len(data.programmes), Names: []string{}}
	for _, c := range data.channels {
		p.Names = append(p.Names, c.name)
	}
	var start, end time.Time
	for _, v := range data.programmes {
		if start.IsZero() || v.start.Before(start) {
			start = v.start
		}
		if end.IsZero() || v.end.After(end) {
			end = v.end
		}
	}
	if !start.IsZero() {
		p.AvailableStart = start.Format(time.RFC3339)
		p.AvailableEnd = end.Format(time.RFC3339)
	}
	return p, nil
}

// DecodeElement can skip unfamiliar nested tags. Preflight the entire document
// so a directive/deep tree cannot hide inside a skipped programme or channel.
func validateGuideXML(raw string) error {
	d := xml.NewDecoder(strings.NewReader(raw))
	depth, nodes, roots := 0, 0, 0
	for {
		t, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return ErrInvalid
		}
		nodes++
		if nodes > 2000000 {
			return ErrInvalid
		}
		switch v := t.(type) {
		case xml.Directive:
			return ErrInvalid
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 || v.Name.Local != "tv" {
					return ErrInvalid
				}
			}
			attributes := map[xml.Name]bool{}
			for _, a := range v.Attr {
				if attributes[a.Name] {
					return ErrInvalid
				}
				attributes[a.Name] = true
			}
			depth++
			if depth > 32 || len(v.Attr) > 64 {
				return ErrInvalid
			}
		case xml.EndElement:
			depth--
			if depth < 0 {
				return ErrInvalid
			}
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(v)) != "" {
				return ErrInvalid
			}
		}
	}
	if depth != 0 || roots != 1 {
		return ErrInvalid
	}
	return nil
}
