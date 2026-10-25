package reading

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf16"
)

const maxContents = 512

// ContentsEntry is publication navigation, not a reading position or authority.
// Section is always a member of this exact EPUB's spine. No timing or page
// estimate is inferred from a publisher's label or order.
type ContentsEntry struct {
	Title    string `json:"title"`
	Section  string `json:"section"`
	Fragment string `json:"fragment,omitempty"`
	Depth    int    `json:"depth"`
}

type navigationNode struct {
	name     xml.Name
	attrs    []xml.Attr
	text     string
	children []*navigationNode
}

// navigationTree consumes one bounded XML document to EOF. It never resolves
// entities, executes instructions or follows a target outside the archive.
func navigationTree(raw []byte) (*navigationNode, error) {
	d := xml.NewDecoder(bytes.NewReader(raw))
	root := &navigationNode{}
	stack := []*navigationNode{root}
	nodes := 0
	for {
		token, err := d.Token()
		if err == io.EOF {
			if len(stack) != 1 || len(root.children) != 1 {
				return nil, ErrInvalid
			}
			return root.children[0], nil
		}
		if err != nil {
			return nil, ErrInvalid
		}
		switch v := token.(type) {
		case xml.StartElement:
			nodes++
			if len(stack) >= 128 || nodes > 8192 || len(v.Attr) > 64 {
				return nil, ErrLimit
			}
			node := &navigationNode{name: v.Name, attrs: v.Attr}
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, node)
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) <= 1 {
				return nil, ErrInvalid
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 1 && len(bytes.TrimSpace(v)) > 0 {
				return nil, ErrInvalid
			}
			node := stack[len(stack)-1]
			// Keep whitespace between inline elements for the publisher's label.
			node.children = append(node.children, &navigationNode{text: string(v)})
		case xml.Directive:
			return nil, ErrInvalid
		case xml.ProcInst:
			if v.Target != "xml" {
				return nil, ErrInvalid
			}
		}
	}
}

func (n *navigationNode) attr(name, namespace string) string {
	for _, a := range n.attrs {
		if a.Name.Local == name && a.Name.Space == namespace {
			return a.Value
		}
	}
	return ""
}
func (n *navigationNode) label() string {
	var b strings.Builder
	var visit func(*navigationNode)
	visit = func(v *navigationNode) {
		b.WriteString(v.text)
		for _, c := range v.children {
			visit(c)
		}
	}
	visit(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func contentsText(value string, limit int) bool {
	if len(utf16.Encode([]rune(value))) > limit {
		return false
	}
	return strings.IndexFunc(value, unicode.IsControl) < 0
}

func (e *EPUB) contentsTarget(base, href, title string, depth int) (ContentsEntry, error) {
	if title == "" || !contentsText(title, 1024) || href == "" || len(href) > 4096 || depth < 0 || depth > 16 {
		return ContentsEntry{}, ErrInvalid
	}
	section, err := resolveReference(base, href)
	if err != nil {
		return ContentsEntry{}, err
	}
	u, err := url.Parse(href)
	if err != nil || !contentsText(u.Fragment, 1024) {
		return ContentsEntry{}, ErrInvalid
	}
	found := false
	for _, item := range e.Spine {
		if item == section {
			found = true
			break
		}
	}
	if !found {
		return ContentsEntry{}, ErrInvalid
	}
	return ContentsEntry{Title: title, Section: section, Fragment: u.Fragment, Depth: depth}, nil
}

// Contents withholds the entire optional descriptor if declared navigation is
// malformed, ambiguous or outside its budget. It never substitutes spine file
// names as chapters. Transport/cancellation/source-guard errors still propagate.
func (e *EPUB) Contents(ctx context.Context) ([]ContentsEntry, error) {
	name := e.ncx
	modern := len(e.navigation) > 0
	if modern {
		if len(e.navigation) != 1 {
			return nil, nil
		}
		name = e.navigation[0]
	}
	if name == "" {
		return nil, nil
	}
	raw, err := e.archive.read(ctx, name, 1<<20)
	if errors.Is(err, ErrInvalid) || errors.Is(err, ErrLimit) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if modern && validateXHTML(name, raw, e.allowed) != nil {
		return nil, nil
	}
	root, err := navigationTree(raw)
	if err != nil {
		return nil, nil
	}
	var out []ContentsEntry
	appendEntry := func(href, title string, depth int) error {
		if len(out) >= maxContents {
			return ErrLimit
		}
		entry, err := e.contentsTarget(name, href, title, depth)
		if err == nil {
			out = append(out, entry)
		}
		return err
	}
	if modern {
		var nav *navigationNode
		var find func(*navigationNode) error
		find = func(n *navigationNode) error {
			if n.name.Local == "nav" {
				for _, kind := range strings.Fields(n.attr("type", "http://www.idpf.org/2007/ops")) {
					if kind == "toc" {
						if nav != nil {
							return ErrInvalid
						}
						nav = n
					}
				}
			}
			for _, c := range n.children {
				if err := find(c); err != nil {
					return err
				}
			}
			return nil
		}
		if find(root) != nil || nav == nil {
			return nil, nil
		}
		var walk func(*navigationNode, int) error
		walk = func(n *navigationNode, depth int) error {
			if n.name.Local == "ol" {
				depth++
			}
			if depth > 17 {
				return ErrLimit
			}
			if n.name.Local == "a" {
				return appendEntry(n.attr("href", ""), n.label(), depth-1)
			}
			for _, c := range n.children {
				if err := walk(c, depth); err != nil {
					return err
				}
			}
			return nil
		}
		if walk(nav, 0) != nil {
			return nil, nil
		}
	} else {
		var walk func(*navigationNode, int) error
		walk = func(n *navigationNode, depth int) error {
			if n.name.Local == "navPoint" {
				var label, href string
				for _, c := range n.children {
					if c.name.Local == "navLabel" {
						label = c.label()
					}
					if c.name.Local == "content" {
						href = c.attr("src", "")
					}
				}
				if err := appendEntry(href, label, depth); err != nil {
					return err
				}
				depth++
			}
			for _, c := range n.children {
				if err := walk(c, depth); err != nil {
					return err
				}
			}
			return nil
		}
		if root.name.Local != "ncx" || walk(root, 0) != nil {
			return nil, nil
		}
	}
	return out, nil
}
