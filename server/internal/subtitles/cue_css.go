package subtitles

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Deliberately finite WebVTT CSS profile. Reject unknown selectors/properties,
// URLs and escaped names; never flatten a stylesheet we cannot faithfully render.
var cueRule = regexp.MustCompile(`(?s)::cue(?:\(\.([a-zA-Z_][a-zA-Z0-9_-]{0,63})\))?\s*\{([^{}]*)\}`)
var safeFont = regexp.MustCompile(`^[a-zA-Z0-9 _-]{1,100}$`)

func parseCueCSS(raw string, existing map[string]string) (map[string]string, error) {
	if len(raw) > 65536 || strings.Contains(raw, "/*") || strings.Contains(raw, "\\") {
		return nil, ErrUnsupported
	}
	rules := cueRule.FindAllStringSubmatchIndex(raw, -1)
	if len(rules) == 0 || len(rules) > 64 {
		return nil, ErrUnsupported
	}
	pos := 0
	for _, m := range rules {
		if strings.TrimSpace(raw[pos:m[0]]) != "" {
			return nil, ErrUnsupported
		}
		pos = m[1]
		key := ""
		if m[2] >= 0 {
			key = raw[m[2]:m[3]]
		}
		body := raw[m[4]:m[5]]
		var tags strings.Builder
		for _, declaration := range strings.Split(body, ";") {
			if strings.TrimSpace(declaration) == "" {
				continue
			}
			name, value, ok := strings.Cut(declaration, ":")
			if !ok {
				return nil, ErrText
			}
			name = strings.ToLower(strings.TrimSpace(name))
			value = strings.TrimSpace(value)
			switch name {
			case "color":
				color, ok := assColor(value)
				if !ok {
					return nil, ErrUnsupported
				}
				tags.WriteString("\\c" + color)
			case "font-family":
				value = strings.Trim(value, "\"'")
				switch value {
				case "sans-serif":
					value = "Arial"
				case "serif":
					value = "Times New Roman"
				case "monospace":
					value = "Courier New"
				}
				if !safeFont.MatchString(value) {
					return nil, ErrUnsupported
				}
				tags.WriteString("\\fn" + value)
			case "font-size":
				if !strings.HasSuffix(value, "px") {
					return nil, ErrUnsupported
				}
				n, e := strconv.Atoi(strings.TrimSuffix(value, "px"))
				if e != nil || n < 8 || n > 200 {
					return nil, ErrText
				}
				fmt.Fprintf(&tags, "\\fs%d", n)
			case "font-weight":
				switch value {
				case "normal", "400":
					tags.WriteString("\\b0")
				case "bold", "700":
					tags.WriteString("\\b1")
				default:
					return nil, ErrUnsupported
				}
			case "font-style":
				switch value {
				case "normal":
					tags.WriteString("\\i0")
				case "italic":
					tags.WriteString("\\i1")
				default:
					return nil, ErrUnsupported
				}
			case "text-decoration":
				switch value {
				case "none":
					tags.WriteString("\\u0\\s0")
				case "underline":
					tags.WriteString("\\u1")
				case "line-through":
					tags.WriteString("\\s1")
				default:
					return nil, ErrUnsupported
				}
			default:
				return nil, ErrUnsupported
			}
		}
		if tags.Len() > 0 {
			existing[key] += "{" + tags.String() + "}"
		}
	}
	if strings.TrimSpace(raw[pos:]) != "" {
		return nil, ErrUnsupported
	}
	return existing, nil
}
