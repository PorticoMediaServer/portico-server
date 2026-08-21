package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/webp"
)

const (
	libraryChannelLogoUploadLimit = 2 << 20
	libraryChannelLogoMaxEdge     = 4096
	libraryChannelLogoMaxPixels   = 16_000_000
)

var libraryChannelAssetIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
var libraryChannelSVGFragmentURLPattern = regexp.MustCompile(`(?i)^url\(#[A-Za-z_][A-Za-z0-9_.:-]*\)$`)

var libraryChannelSVGElements = map[string]bool{
	"svg": true, "g": true, "path": true, "rect": true, "circle": true,
	"ellipse": true, "line": true, "polyline": true, "polygon": true,
	"defs": true, "lineargradient": true, "radialgradient": true,
	"stop": true, "clippath": true,
}

var libraryChannelSVGAttributes = map[string]bool{
	"xmlns": true, "viewbox": true, "width": true, "height": true,
	"preserveaspectratio": true, "id": true, "transform": true,
	"d": true, "x": true, "y": true, "x1": true, "y1": true,
	"x2": true, "y2": true, "cx": true, "cy": true, "r": true,
	"rx": true, "ry": true, "points": true, "fill": true,
	"fill-opacity": true, "fill-rule": true, "stroke": true,
	"stroke-width": true, "stroke-opacity": true, "stroke-linecap": true,
	"stroke-linejoin": true, "stroke-miterlimit": true,
	"stroke-dasharray": true, "stroke-dashoffset": true, "opacity": true,
	"offset": true, "stop-color": true, "stop-opacity": true,
	"gradientunits": true, "gradienttransform": true, "spreadmethod": true,
	"fx": true, "fy": true, "clip-path": true, "clip-rule": true,
}

type normalizedLibraryChannelLogo struct {
	MIMEType string
	Ext      string
	Data     []byte
	Width    int
	Height   int
}

type libraryChannelLogoAssetDocument struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	MIMEType string `json:"mimeType"`
	URL      string `json:"url"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Bytes    int    `json:"bytes"`
	SHA256   string `json:"sha256"`
}

type builtInLibraryChannelLogoStyle struct {
	Label  string
	Accent string
}

var builtInLibraryChannelLogoStyles = map[string]builtInLibraryChannelLogoStyle{
	"movie-time": {"MOVIE TIME", "#39BDF8"}, "classic-cinema": {"CLASSIC", "#EABF78"},
	"movies-1970s": {"THE 70s", "#F59E6B"}, "movies-1980s": {"THE 80s", "#F472B6"},
	"movies-1990s": {"THE 90s", "#A78BFA"}, "movies-2000s": {"2000s", "#60A5FA"},
	"action-movies": {"ACTION", "#F87171"}, "adventure-movies": {"ADVENTURE", "#FBBF24"},
	"comedy-movies": {"COMEDY", "#FACC15"}, "drama-movies": {"DRAMA", "#C084FC"},
	"science-fiction-movies": {"SCI-FI", "#22D3EE"}, "horror-after-dark": {"AFTER DARK", "#EF4444"},
	"thriller-movies": {"THRILLERS", "#FB7185"}, "family-movie-night": {"FAMILY", "#34D399"},
	"documentary-films": {"DOCUMENTARY", "#2DD4BF"}, "animated-films": {"ANIMATION", "#F472B6"},
	"all-television": {"TELEVISION", "#38BDF8"}, "sitcoms": {"SITCOMS", "#FDE047"},
	"television-drama": {"TV DRAMA", "#A78BFA"}, "crime-and-mystery": {"CRIME + MYSTERY", "#94A3B8"},
	"science-fiction-and-fantasy": {"SCI-FI + FANTASY", "#22D3EE"}, "reality-and-competition": {"REALITY", "#FB923C"},
	"kids-and-family": {"KIDS + FAMILY", "#4ADE80"}, "saturday-morning-cartoons": {"SATURDAY", "#FACC15"},
	"anime": {"ANIME", "#F472B6"}, "recently-added": {"NEW THIS WEEK", "#38BDF8"},
	"marathon-tv": {"MARATHON", "#F97316"},
}

func builtInLibraryChannelLogoRef(templateKey string) string {
	if _, exists := builtInLibraryChannelLogoStyles[templateKey]; !exists {
		return ""
	}
	return "default." + templateKey
}

func builtInLibraryChannelLogo(ref string) ([]byte, bool) {
	key := strings.TrimPrefix(ref, "default.")
	style, exists := builtInLibraryChannelLogoStyles[key]
	if !exists || ref != "default."+key {
		return nil, false
	}
	// Product-owned, deterministic SVG. Text is drawn as escaped XML and the
	// asset contains no script, external reference, filter, font, or animation.
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(style.Label))
	body := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="960" height="540" viewBox="0 0 960 540" role="img" aria-label="%s"><rect width="960" height="540" rx="72" fill="#071019"/><path d="M112 180h48v180h-48zM184 180h48v180h-48zM256 180h48v180h-48z" fill="%s"/><circle cx="208" cy="270" r="118" fill="none" stroke="%s" stroke-width="14" opacity=".34"/><text x="368" y="292" fill="#F5F8FA" font-family="system-ui,sans-serif" font-size="54" font-weight="750" letter-spacing="2">%s</text></svg>`, escaped.String(), style.Accent, style.Accent, escaped.String())
	return []byte(body), true
}

func builtInLibraryChannelLogoPNG(ref string) ([]byte, bool) {
	key := strings.TrimPrefix(ref, "default.")
	style, exists := builtInLibraryChannelLogoStyles[key]
	if !exists || ref != "default."+key {
		return nil, false
	}
	accent := color.RGBA{57, 189, 248, 255}
	if parsed, err := strconv.ParseUint(strings.TrimPrefix(style.Accent, "#"), 16, 32); err == nil {
		accent = color.RGBA{uint8(parsed >> 16), uint8(parsed >> 8), uint8(parsed), 255}
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 320, 112))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{7, 16, 25, 224}}, image.Point{}, draw.Src)
	for index := 0; index < 3; index++ {
		x := 22 + index*15
		draw.Draw(canvas, image.Rect(x, 27, x+9, 85), &image.Uniform{C: accent}, image.Point{}, draw.Src)
	}
	// The on-video asset deliberately avoids runtime font dependencies. The
	// packaged full SVG retains the template label; this compact bug uses the
	// same accent and a deterministic broadcast-mark motif.
	for index, value := range []byte(style.Label) {
		if index >= 18 {
			break
		}
		height := 18 + int(value%34)
		x := 86 + index*11
		draw.Draw(canvas, image.Rect(x, 56-height/2, x+7, 56+height/2), &image.Uniform{C: color.RGBA{245, 248, 250, 235}}, image.Point{}, draw.Src)
	}
	var output bytes.Buffer
	if png.Encode(&output, canvas) != nil {
		return nil, false
	}
	return output.Bytes(), true
}

func (s *Server) handleLibraryChannelLogoRoute(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	if !canViewLiveTV(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "This profile cannot view Library Channels.")
		return
	}
	ref := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/library-channels/logos/"), "/")
	if !validLibraryChannelAssetID(ref) {
		writeProductError(w, http.StatusNotFound, "library_channel_logo_not_found", "Library Channel logo was not found.")
		return
	}
	if data, ok := builtInLibraryChannelLogo(ref); ok {
		serveLibraryChannelLogo(w, r, ref, "image/svg+xml", data)
		return
	}
	path, mimeType, err := s.libraryChannelCustomLogoPath(ref)
	if err != nil {
		writeProductError(w, http.StatusNotFound, "library_channel_logo_not_found", "Library Channel logo was not found.")
		return
	}
	file, err := os.Open(path)
	if err != nil {
		writeProductError(w, http.StatusNotFound, "library_channel_logo_not_found", "Library Channel logo was not found.")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() > libraryChannelLogoUploadLimit {
		writeProductError(w, http.StatusNotFound, "library_channel_logo_not_found", "Library Channel logo was not found.")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, libraryChannelLogoUploadLimit+1))
	if err != nil || int64(len(data)) != info.Size() {
		writeProductError(w, http.StatusNotFound, "library_channel_logo_not_found", "Library Channel logo was not found.")
		return
	}
	serveLibraryChannelLogo(w, r, ref, mimeType, data)
}

func serveLibraryChannelLogo(w http.ResponseWriter, r *http.Request, ref, mimeType string, data []byte) {
	digest := sha256.Sum256(data)
	etag := `"` + hex.EncodeToString(digest[:]) + `"`
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'none'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Disposition", `inline; filename="`+ref+mimeExtension(mimeType)+`"`)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleAdminLibraryChannelLogoUpload(w http.ResponseWriter, r *http.Request, user User) {
	if !isLibraryChannelOwner(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "Only the server owner can administer Library Channels.")
		return
	}
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, libraryChannelLogoUploadLimit+(64<<10))
	if err := r.ParseMultipartForm(libraryChannelLogoUploadLimit); err != nil {
		writeProductError(w, http.StatusBadRequest, "library_channel_logo_invalid", "Logo upload must be a PNG or WebP file under 2 MB.")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeProductError(w, http.StatusBadRequest, "library_channel_logo_invalid", "Logo file is required.")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, libraryChannelLogoUploadLimit+1))
	if err != nil || len(data) == 0 || len(data) > libraryChannelLogoUploadLimit {
		writeProductError(w, http.StatusBadRequest, "library_channel_logo_invalid", "Logo upload must be under 2 MB.")
		return
	}
	normalized, err := normalizeLibraryChannelLogo(data, header.Filename)
	if err != nil {
		writeProductError(w, http.StatusBadRequest, "library_channel_logo_invalid", err.Error())
		return
	}
	assetID := randomID("lca")
	s.libraryChannelAssetMu.Lock()
	err = s.writeLibraryChannelCustomLogo(assetID, normalized)
	s.libraryChannelAssetMu.Unlock()
	if err != nil {
		writeProductError(w, http.StatusInternalServerError, "library_channel_logo_store_failed", "Portico could not store the Library Channel logo.")
		return
	}
	digest := sha256.Sum256(normalized.Data)
	writeJSON(w, http.StatusCreated, libraryChannelLogoAssetDocument{ID: assetID, Source: "custom", MIMEType: normalized.MIMEType, URL: "/api/library-channels/logos/" + assetID, Width: normalized.Width, Height: normalized.Height, Bytes: len(normalized.Data), SHA256: hex.EncodeToString(digest[:])})
}

func (s *Server) handleAdminLibraryChannelLogoDelete(w http.ResponseWriter, r *http.Request, user User, ref string) {
	if r.Method != http.MethodDelete {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use DELETE for this endpoint.")
		return
	}
	if !validLibraryChannelAssetID(ref) || !strings.HasPrefix(ref, "lca_") {
		writeProductError(w, http.StatusNotFound, "library_channel_logo_not_found", "Uploaded Library Channel logo was not found.")
		return
	}
	s.libraryChannelAssetMu.Lock()
	defer s.libraryChannelAssetMu.Unlock()
	var uses int
	if err := s.queryUserRow(r.Context(), `SELECT COUNT(*) FROM library_channels WHERE logo_source='custom' AND logo_ref=?`, ref).Scan(&uses); err != nil {
		writeProductError(w, http.StatusInternalServerError, "library_channel_logo_delete_failed", "Portico could not inspect the Library Channel logo.")
		return
	}
	if uses > 0 {
		writeProductError(w, http.StatusConflict, "library_channel_logo_in_use", "Remove this logo from every Library Channel before deleting it.")
		return
	}
	path, _, err := s.libraryChannelCustomLogoPath(ref)
	if err != nil {
		writeProductError(w, http.StatusNotFound, "library_channel_logo_not_found", "Uploaded Library Channel logo was not found.")
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		writeProductError(w, http.StatusInternalServerError, "library_channel_logo_delete_failed", "Portico could not delete the Library Channel logo.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func normalizeLibraryChannelLogo(data []byte, filename string) (normalizedLibraryChannelLogo, error) {
	if len(data) == 0 || len(data) > libraryChannelLogoUploadLimit {
		return normalizedLibraryChannelLogo{}, errors.New("Logo upload must be under 2 MB.")
	}
	contentType := http.DetectContentType(data)
	extension := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("<")) || contentType == "image/svg+xml" || extension == "svg" {
		return rasterizeLibraryChannelSVG(data)
	}
	var config image.Config
	var decoded image.Image
	var err error
	switch contentType {
	case "image/png":
		config, err = png.DecodeConfig(bytes.NewReader(data))
		if err == nil {
			err = validateLibraryChannelLogoDimensions(config.Width, config.Height)
		}
		if err != nil {
			return normalizedLibraryChannelLogo{}, errors.New("Logo image dimensions could not be decoded safely.")
		}
		decoded, err = png.Decode(bytes.NewReader(data))
	case "image/webp":
		config, err = webp.DecodeConfig(bytes.NewReader(data))
		if err == nil {
			err = validateLibraryChannelLogoDimensions(config.Width, config.Height)
		}
		if err != nil {
			return normalizedLibraryChannelLogo{}, errors.New("Logo image dimensions could not be decoded safely.")
		}
		decoded, err = webp.Decode(bytes.NewReader(data))
	default:
		return normalizedLibraryChannelLogo{}, errors.New("Only PNG and WebP Library Channel logos are supported.")
	}
	if err != nil {
		return normalizedLibraryChannelLogo{}, errors.New("Logo image could not be decoded safely.")
	}
	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width != config.Width || height != config.Height {
		return normalizedLibraryChannelLogo{}, errors.New("Logo dimensions changed while decoding.")
	}
	if err := validateLibraryChannelLogoDimensions(width, height); err != nil {
		return normalizedLibraryChannelLogo{}, err
	}
	var output bytes.Buffer
	if err := png.Encode(&output, decoded); err != nil || output.Len() > libraryChannelLogoUploadLimit {
		return normalizedLibraryChannelLogo{}, errors.New("Normalized logo image is too large.")
	}
	return normalizedLibraryChannelLogo{MIMEType: "image/png", Ext: "png", Data: output.Bytes(), Width: width, Height: height}, nil
}

// rasterizeLibraryChannelSVG accepts a deliberately small, non-networked SVG
// subset and immediately converts it to PNG. The original XML is never stored
// or served. Script, text/font, external resource, animation, CSS, event,
// entity, and processing-instruction surfaces are rejected before the SVG
// parser sees the document.
func rasterizeLibraryChannelSVG(data []byte) (normalizedLibraryChannelLogo, error) {
	if err := validateLibraryChannelSVG(data); err != nil {
		return normalizedLibraryChannelLogo{}, err
	}
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data), oksvg.StrictErrorMode)
	if err != nil || icon == nil {
		return normalizedLibraryChannelLogo{}, errors.New("SVG logo could not be rasterized safely.")
	}
	width, height := icon.ViewBox.W, icon.ViewBox.H
	if math.IsNaN(width) || math.IsNaN(height) || math.IsInf(width, 0) || math.IsInf(height, 0) || width <= 0 || height <= 0 {
		return normalizedLibraryChannelLogo{}, errors.New("SVG logo must declare a positive viewBox or width and height.")
	}
	pixelWidth, pixelHeight := int(math.Ceil(width)), int(math.Ceil(height))
	if err := validateLibraryChannelLogoDimensions(pixelWidth, pixelHeight); err != nil {
		return normalizedLibraryChannelLogo{}, err
	}
	canvas := image.NewRGBA(image.Rect(0, 0, pixelWidth, pixelHeight))
	icon.SetTarget(0, 0, float64(pixelWidth), float64(pixelHeight))
	icon.Draw(rasterx.NewDasher(pixelWidth, pixelHeight, rasterx.NewScannerGV(pixelWidth, pixelHeight, canvas, canvas.Bounds())), 1)
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil || output.Len() > libraryChannelLogoUploadLimit {
		return normalizedLibraryChannelLogo{}, errors.New("Normalized logo image is too large.")
	}
	return normalizedLibraryChannelLogo{MIMEType: "image/png", Ext: "png", Data: output.Bytes(), Width: pixelWidth, Height: pixelHeight}, nil
}

func validateLibraryChannelSVG(data []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	depth, tokens := 0, 0
	rootSeen := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.New("SVG logo is not well-formed XML.")
		}
		tokens++
		if tokens > 20_000 {
			return errors.New("SVG logo is too complex.")
		}
		switch value := token.(type) {
		case xml.StartElement:
			depth++
			if depth > 64 {
				return errors.New("SVG logo nesting is too deep.")
			}
			name := strings.ToLower(value.Name.Local)
			if !libraryChannelSVGElements[name] || (!rootSeen && name != "svg") {
				return errors.New("SVG logo contains an unsupported element.")
			}
			rootSeen = true
			if len(value.Attr) > 128 {
				return errors.New("SVG logo contains too many attributes.")
			}
			for _, attribute := range value.Attr {
				attributeName := strings.ToLower(attribute.Name.Local)
				if strings.HasPrefix(attributeName, "on") || attributeName == "href" || attributeName == "style" || !libraryChannelSVGAttributes[attributeName] {
					return errors.New("SVG logo contains an unsupported attribute.")
				}
				if attribute.Name.Space != "" && attribute.Name.Space != "xmlns" && attribute.Name.Space != "http://www.w3.org/2000/xmlns/" {
					return errors.New("SVG logo contains an unsupported namespace.")
				}
				lowerValue := strings.ToLower(strings.TrimSpace(attribute.Value))
				if attributeName == "xmlns" {
					if lowerValue != "http://www.w3.org/2000/svg" {
						return errors.New("SVG logo contains an unsupported namespace.")
					}
					continue
				}
				for _, forbidden := range []string{"javascript:", "data:", "http:", "https:", "file:", "@import", "expression(", "var("} {
					if strings.Contains(lowerValue, forbidden) {
						return errors.New("SVG logo contains an external or executable reference.")
					}
				}
				if strings.Contains(lowerValue, "url(") && !libraryChannelSVGFragmentURLPattern.MatchString(lowerValue) {
					return errors.New("SVG logo contains an external resource reference.")
				}
			}
		case xml.EndElement:
			depth--
			if depth < 0 {
				return errors.New("SVG logo has invalid nesting.")
			}
		case xml.CharData:
			if strings.TrimSpace(string(value)) != "" {
				return errors.New("SVG logo text and embedded font content are not supported.")
			}
		case xml.Directive, xml.ProcInst:
			return errors.New("SVG logo contains an unsupported XML instruction.")
		}
	}
	if !rootSeen || depth != 0 {
		return errors.New("SVG logo must contain one complete SVG document.")
	}
	return nil
}

func validateLibraryChannelLogoDimensions(width, height int) error {
	if width < 1 || height < 1 || width > libraryChannelLogoMaxEdge || height > libraryChannelLogoMaxEdge || int64(width)*int64(height) > libraryChannelLogoMaxPixels {
		return errors.New("Logo dimensions must not exceed 4096 pixels per edge or 16 megapixels.")
	}
	return nil
}

func validLibraryChannelAssetID(value string) bool {
	return libraryChannelAssetIDPattern.MatchString(value) && !strings.Contains(value, "..") && !strings.ContainsAny(value, `/\\`)
}

func (s *Server) writeLibraryChannelCustomLogo(ref string, logo normalizedLibraryChannelLogo) error {
	if !validLibraryChannelAssetID(ref) || !strings.HasPrefix(ref, "lca_") {
		return errors.New("invalid Library Channel asset identifier")
	}
	root := filepath.Join(s.cfg.AppDataDir, "library-channel-logos")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	target := filepath.Join(root, ref+"."+logo.Ext)
	if !pathInsideRoot(target, root) {
		return errors.New("invalid Library Channel logo path")
	}
	temporary, err := os.CreateTemp(root, ".logo-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(logo.Data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, target)
}

func (s *Server) libraryChannelCustomLogoPath(ref string) (string, string, error) {
	if !validLibraryChannelAssetID(ref) || !strings.HasPrefix(ref, "lca_") {
		return "", "", os.ErrNotExist
	}
	root := filepath.Join(s.cfg.AppDataDir, "library-channel-logos")
	for _, candidate := range []struct{ ext, mime string }{{"png", "image/png"}} {
		path := filepath.Join(root, ref+"."+candidate.ext)
		if !pathInsideRoot(path, root) {
			return "", "", os.ErrNotExist
		}
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, candidate.mime, nil
		}
	}
	return "", "", os.ErrNotExist
}

// validateLibraryChannelLogoReference verifies that an aggregate never stores a
// dangling or mismatched asset reference. Callers that persist an aggregate
// hold libraryChannelAssetMu across this check and the database write so a
// concurrent delete cannot create a broken reference.
func (s *Server) validateLibraryChannelLogoReference(logo libraryChannelLogoDocument) error {
	switch logo.Source {
	case "none":
		if logo.Ref != "" || logo.MIMEType != "" || logo.BugEnabled {
			return errors.New("A Library Channel without a logo cannot include a reference or an on-video logo.")
		}
		return nil
	case "built_in":
		if _, ok := builtInLibraryChannelLogo(logo.Ref); !ok || logo.MIMEType != "image/svg+xml" {
			return errors.New("Built-in Library Channel logo reference is invalid.")
		}
	case "custom":
		_, mimeType, err := s.libraryChannelCustomLogoPath(logo.Ref)
		if err != nil || mimeType != logo.MIMEType {
			return errors.New("Uploaded Library Channel logo is unavailable or its media type does not match.")
		}
	default:
		return errors.New("Library Channel logo source is invalid.")
	}
	if logo.BugEnabled && !logo.BugOverheadAccepted {
		return errors.New("Burning a Library Channel logo into video requires accepting the transcoding overhead.")
	}
	return nil
}

func mimeExtension(mimeType string) string {
	if mimeType == "image/svg+xml" {
		return ".svg"
	}
	if mimeType == "image/png" {
		return ".png"
	}
	if extensions, _ := mime.ExtensionsByType(mimeType); len(extensions) > 0 {
		return extensions[0]
	}
	return ""
}
