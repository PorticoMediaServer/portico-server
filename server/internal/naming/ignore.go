package naming

import (
	"path"
	"regexp"
	"strings"
)

const (
	MaxIgnoreBytes        = 65536
	MaxIgnorePatterns     = 512
	MaxIgnorePatternBytes = 1024
)

var sampleName = regexp.MustCompile(`(?i)(?:^|[._-][ ._-]*)sample(?:$|[._-])`)

var systemFolders = map[string]bool{"@eadir": true, "#recycle": true, "$recycle.bin": true, "lost+found": true, "@recycle": true, "system volume information": true}

// IgnoreName handles one entry before stat/probe. The library root itself is
// never tested. Complete titles such as "Sample (2020)" are preserved.
func IgnoreName(name string, directory bool) bool {
	lower := strings.ToLower(name)
	if strings.HasPrefix(name, ".") || systemFolders[lower] || strings.HasSuffix(lower, ".grab") {
		return true
	}
	if directory {
		return lower == "sample" || lower == "samples"
	}
	stem := strings.TrimSuffix(name, path.Ext(name))
	if strings.EqualFold(stem, "sample") {
		return true
	}
	lower = strings.ToLower(stem)
	return sampleName.MatchString(lower)
}
func IgnorePath(relative string, directory bool) bool {
	if len(relative) > MaxPathBytes || strings.IndexByte(relative, 0) >= 0 {
		return true
	}
	parts := strings.Split(slash(relative), "/")
	if len(parts) > MaxDepth {
		return true
	}
	for i, s := range parts {
		if s == "" || s == "." || s == ".." {
			continue
		}
		if IgnoreName(s, i < len(parts)-1 || directory) {
			return true
		}
	}
	return false
}

type IgnoreDiagnostic struct {
	Line   int
	Reason string
}
type ignorePattern struct {
	re       *regexp.Regexp
	basename bool
}
type IgnoreRules struct {
	// Scope is the relative directory containing .plexignore or .ignore. Root
	// controls Plex's recursive basename-pattern behaviour.
	Scope    string
	Root     bool
	patterns []ignorePattern
}

// ParseIgnore uses Plex's line-oriented syntax: blank lines and # comments,
// wildcard patterns, slash patterns relative to the containing directory.
// A malformed or oversized line is skipped, retaining other valid patterns.
// The scanner supplies bounded contents; this package never reads files.
func ParseIgnore(scope, contents string, root bool) (IgnoreRules, []IgnoreDiagnostic) {
	rules := IgnoreRules{Scope: strings.Trim(path.Clean(slash(scope)), "/"), Root: root}
	if rules.Scope == "." {
		rules.Scope = ""
	}
	var diagnostics []IgnoreDiagnostic
	if len(contents) > MaxIgnoreBytes {
		diagnostics = append(diagnostics, IgnoreDiagnostic{0, "ignore_file_budget_exceeded"})
		contents = contents[:MaxIgnoreBytes]
		if end := strings.LastIndexByte(contents, '\n'); end >= 0 {
			contents = contents[:end]
		} else {
			contents = ""
		}
	}
	for index, line := range strings.Split(strings.TrimPrefix(contents, "\ufeff"), "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if len(s) > MaxIgnorePatternBytes {
			diagnostics = append(diagnostics, IgnoreDiagnostic{index + 1, "ignore_pattern_budget_exceeded"})
			continue
		}
		if len(rules.patterns) >= MaxIgnorePatterns {
			diagnostics = append(diagnostics, IgnoreDiagnostic{index + 1, "ignore_pattern_count_exceeded"})
			break
		}
		re, err := globPattern(s)
		if err != nil {
			diagnostics = append(diagnostics, IgnoreDiagnostic{index + 1, "invalid_ignore_pattern"})
			continue
		}
		rules.patterns = append(rules.patterns, ignorePattern{re, !strings.Contains(s, "/")})
	}
	return rules, diagnostics
}
func globPattern(s string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		case '[':
			end := strings.IndexByte(s[i+1:], ']')
			if end < 0 {
				return regexp.Compile("[")
			}
			end += i + 1
			cls := s[i+1 : end]
			if strings.HasPrefix(cls, "!") {
				cls = "^" + cls[1:]
			}
			cls = strings.ReplaceAll(cls, `\`, `\\`)
			b.WriteString("[" + cls + "]")
			i = end
		default:
			b.WriteString(regexp.QuoteMeta(s[i : i+1]))
		}
	}
	if strings.HasSuffix(s, "/") {
		b.WriteString(".*")
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
func (r IgnoreRules) Match(relative string, directory bool) bool {
	if len(relative) > MaxPathBytes || len(strings.Split(slash(relative), "/")) > MaxDepth {
		return false
	}
	p := path.Clean(slash(relative))
	if r.Scope != "" {
		if !strings.HasPrefix(p, r.Scope+"/") {
			return false
		}
		p = strings.TrimPrefix(p, r.Scope+"/")
	}
	for _, pattern := range r.patterns {
		if pattern.basename {
			if r.Root || !strings.Contains(p, "/") {
				if pattern.re.MatchString(path.Base(p)) {
					return true
				}
			}
			continue
		}
		if pattern.re.MatchString(p) || directory && pattern.re.MatchString(p+"/") {
			return true
		}
		for at := strings.IndexByte(p, '/'); at >= 0; {
			if pattern.re.MatchString(p[:at]) || pattern.re.MatchString(p[:at]+"/") {
				return true
			}
			next := strings.IndexByte(p[at+1:], '/')
			if next < 0 {
				break
			}
			at += next + 1
		}
	}
	return false
}
