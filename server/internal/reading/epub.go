package reading

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"path"
	"sort"
	"strings"
)

type Resource struct {
	Path string
	MIME string
	Size int64
}
type Metadata struct {
	Title, Author, Language, Direction, CoverURL, ExplicitPublicationType string
	PublicationTypeField                                                  string
	PublicationTypeRaw                                                    []string
	PublicationTypeComplete                                               bool
}
type EPUB struct {
	archive    *archive
	Resources  []Resource
	Metadata   Metadata
	Package    string
	Spine      []string
	allowed    map[string]string
	navigation []string
	ncx        string
}

type containerXML struct {
	Roots []struct {
		Path string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}
type packageXML struct {
	XMLName  xml.Name
	Metadata struct {
		XMLName  xml.Name
		Title    string `xml:"title"`
		Author   string `xml:"creator"`
		Language string `xml:"language"`
		Types    []struct {
			Value string `xml:",chardata"`
			Inner string `xml:",innerxml"`
		} `xml:"http://purl.org/dc/elements/1.1/ type"`
	} `xml:"metadata"`
	Manifest []struct {
		ID         string `xml:"id,attr"`
		Href       string `xml:"href,attr"`
		MIME       string `xml:"media-type,attr"`
		Properties string `xml:"properties,attr"`
	} `xml:"manifest>item"`
	Spine struct {
		Direction string `xml:"page-progression-direction,attr"`
		TOC       string `xml:"toc,attr"`
		Items     []struct {
			ID string `xml:"idref,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

func decodeXML(b []byte, out any) error {
	// No external entity, DTD or processing-instruction execution is permitted.
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = true
	if err := d.Decode(out); err != nil {
		return ErrInvalid
	}
	for {
		t, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return ErrInvalid
		}
		if c, ok := t.(xml.CharData); !ok || len(bytes.TrimSpace(c)) != 0 {
			return ErrInvalid
		}
	}
}

func OpenEPUB(ctx context.Context, r io.ReaderAt, size int64) (*EPUB, error) {
	a, err := openArchive(r, size)
	if err != nil {
		return nil, err
	}
	return openEPUBArchive(ctx, a)
}
func OpenSequentialEPUB(ctx context.Context, source SequentialSource) (*EPUB, error) {
	a, err := openSequentialArchive(ctx, source)
	if err != nil {
		return nil, err
	}
	return openEPUBArchive(ctx, a)
}
func openEPUBArchive(ctx context.Context, a *archive) (*EPUB, error) {
	if a.files["META-INF/rights.xml"] != nil {
		return nil, ErrInvalid
	}
	mime, err := a.read(ctx, "mimetype", 64)
	if err != nil {
		return nil, err
	}
	if string(mime) != "application/epub+zip" {
		return nil, ErrInvalid
	}
	b, err := a.read(ctx, "META-INF/container.xml", 256<<10)
	if err != nil {
		return nil, err
	}
	var c containerXML
	if err = decodeXML(b, &c); err != nil || len(c.Roots) != 1 || !safePath(c.Roots[0].Path) {
		return nil, ErrInvalid
	}
	pkg := c.Roots[0].Path
	b, err = a.read(ctx, pkg, 1<<20)
	if err != nil {
		return nil, err
	}
	var p packageXML
	if err = decodeXML(b, &p); err != nil || len(p.Manifest) == 0 || len(p.Spine.Items) == 0 {
		return nil, ErrInvalid
	}
	if len(p.Manifest) > MaxEntries || len(p.Spine.Items) > MaxEntries {
		return nil, ErrLimit
	}
	e := &EPUB{archive: a, Package: pkg, Metadata: Metadata{Title: p.Metadata.Title, Author: p.Metadata.Author, Language: p.Metadata.Language, PublicationTypeField: "{http://purl.org/dc/elements/1.1/}type"}, allowed: map[string]string{"mimetype": "text/plain", "META-INF/container.xml": "application/xml", pkg: "application/oebps-package+xml"}}
	if p.XMLName.Local == "package" && p.XMLName.Space == "http://www.idpf.org/2007/opf" && p.Metadata.XMLName.Local == "metadata" && p.Metadata.XMLName.Space == p.XMLName.Space {
		values := make([]string, 0, len(p.Metadata.Types))
		scalar := true
		for _, field := range p.Metadata.Types {
			values = append(values, field.Value)
			scalar = scalar && !strings.Contains(field.Inner, "<")
		}
		e.Metadata.PublicationTypeRaw, e.Metadata.PublicationTypeComplete = boundedTypeEvidence(values)
		e.Metadata.PublicationTypeComplete = e.Metadata.PublicationTypeComplete && scalar
		if e.Metadata.PublicationTypeComplete {
			e.Metadata.ExplicitPublicationType = explicitPublicationType(values)
		}
	}
	if len(e.Metadata.Title) > 4096 || len(e.Metadata.Author) > 4096 || len(e.Metadata.Language) > 64 {
		return nil, ErrLimit
	}
	if p.Spine.Direction == "rtl" || p.Spine.Direction == "ltr" {
		e.Metadata.Direction = p.Spine.Direction
	}
	ids := map[string]string{}
	for _, item := range p.Manifest {
		name, err := resolveReference(pkg, item.Href)
		if err != nil || ids[item.ID] != "" || item.ID == "" || a.files[name] == nil {
			return nil, ErrInvalid
		}
		if !allowedMIME(item.MIME) {
			return nil, ErrInvalid
		}
		if _, exists := e.allowed[name]; exists {
			return nil, ErrInvalid
		}
		if strings.Contains(item.Properties, "scripted") || strings.Contains(item.Properties, "remote-resources") {
			return nil, ErrInvalid
		}
		ids[item.ID] = name
		e.allowed[name] = item.MIME
		for _, property := range strings.Fields(item.Properties) {
			if property == "nav" && item.MIME == "application/xhtml+xml" {
				e.navigation = append(e.navigation, name)
			}
		}
	}
	for _, item := range p.Spine.Items {
		name := ids[item.ID]
		if name == "" || e.allowed[name] != "application/xhtml+xml" {
			return nil, ErrInvalid
		}
		e.Spine = append(e.Spine, name)
	}
	if name := ids[p.Spine.TOC]; name != "" && e.allowed[name] == "application/x-dtbncx+xml" {
		e.ncx = name
	}
	if a.files["META-INF/encryption.xml"] != nil {
		b, err = a.read(ctx, "META-INF/encryption.xml", 256<<10)
		if err != nil {
			return nil, err
		}
		if err = validateFontObfuscation(b, e.allowed); err != nil {
			return nil, err
		}
		e.allowed["META-INF/encryption.xml"] = "application/xml"
	}
	for name, mime := range e.allowed {
		e.Resources = append(e.Resources, Resource{name, mime, int64(a.files[name].UncompressedSize64)})
	}
	sort.Slice(e.Resources, func(i, j int) bool { return e.Resources[i].Path < e.Resources[j].Path })
	return e, nil
}

func allowedMIME(mime string) bool {
	switch mime {
	case "application/xhtml+xml", "text/css", "image/jpeg", "image/png", "image/webp", "font/otf", "font/ttf", "font/woff", "font/woff2", "application/font-sfnt", "application/vnd.ms-opentype", "application/x-dtbncx+xml":
		return true
	}
	return false
}

func (e *EPUB) Asset(ctx context.Context, name string) ([]byte, string, error) {
	mime, ok := e.allowed[name]
	if !ok {
		return nil, "", ErrInvalid
	}
	b, err := e.archive.read(ctx, name, MaxMember)
	if err != nil {
		return nil, "", err
	}
	switch mime {
	case "application/xhtml+xml":
		err = validateXHTML(name, b, e.allowed)
	case "text/css":
		err = validateCSS(name, b, e.allowed)
	case "image/jpeg", "image/png", "image/webp":
		var info Image
		info, err = inspectImage(b)
		if err == nil && info.MIME != mime {
			err = ErrInvalid
		}
	}
	if err != nil {
		return nil, "", err
	}
	return b, mime, nil
}

var htmlTags = strings.Fields("html head title meta link body div span p br hr h1 h2 h3 h4 h5 h6 a img picture source figure figcaption section article aside header footer main nav address blockquote q cite abbr em strong i b u s small sub sup mark ruby rt rp time pre code kbd samp var ol ul li dl dt dd table caption colgroup col thead tbody tfoot tr th td style")

func validateXHTML(base string, b []byte, assets map[string]string) error {
	allowed := map[string]bool{}
	for _, tag := range htmlTags {
		allowed[tag] = true
	}
	d := xml.NewDecoder(bytes.NewReader(b))
	depth := 0
	inStyle := false
	var style []byte
	for {
		token, err := d.Token()
		if err == io.EOF {
			if depth != 0 {
				return ErrInvalid
			}
			return nil
		}
		if err != nil {
			return ErrInvalid
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			tag := strings.ToLower(t.Name.Local)
			if inStyle || depth > 128 || !allowed[tag] || (t.Name.Space != "" && t.Name.Space != "http://www.w3.org/1999/xhtml") {
				return ErrInvalid
			}
			if tag == "style" {
				inStyle = true
				style = style[:0]
			}
			for _, a := range t.Attr {
				name := strings.ToLower(a.Name.Local)
				value := strings.TrimSpace(a.Value)
				if strings.HasPrefix(name, "on") || name == "base" || name == "srcdoc" || name == "srcset" || name == "http-equiv" || name == "ping" || name == "action" || name == "formaction" || name == "background" {
					return ErrInvalid
				}
				if name == "style" {
					if err := validateCSS(base, []byte(value), assets); err != nil {
						return err
					}
				}
				if name == "href" || name == "src" || name == "poster" {
					target, err := resolveReference(base, value)
					if err != nil {
						return err
					}
					if target != base {
						if _, ok := assets[target]; !ok {
							return ErrInvalid
						}
					}
				}
			}
		case xml.EndElement:
			depth--
			if strings.EqualFold(t.Name.Local, "style") {
				if err := validateCSS(base, style, assets); err != nil {
					return err
				}
				inStyle = false
			}
		case xml.CharData:
			if inStyle {
				style = append(style, t...)
			}
		case xml.Directive:
			if strings.TrimSpace(strings.ToLower(string(t))) != "doctype html" {
				return ErrInvalid
			}
		case xml.ProcInst:
			if t.Target != "xml" {
				return ErrInvalid
			}
		}
	}
}

func extension(name string) string { return strings.ToLower(path.Ext(name)) }
