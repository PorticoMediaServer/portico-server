package subtitlevideo

import (
	"context"
	"encoding/binary"
	"path/filepath"
	"portico.local/server/internal/decoder"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/subtitles"
	"strings"
)

func (r *Runtime) extractFonts(ctx context.Context, input subtitles.RenderInput) ([]subtitles.FontAttachment, error) {
	_, streams, e := r.probe(ctx, input, &extentLedger{})
	if e != nil {
		return nil, e
	}
	var out []subtitles.FontAttachment
	total := 0
	for _, s := range streams {
		name := filepath.Base(s.Tags["filename"])
		ext := strings.ToLower(filepath.Ext(name))
		if s.Type != "attachment" || (ext != ".ttf" && ext != ".otf") {
			continue
		}
		if len(out) >= 16 {
			break
		}
		bridge, e := openBridge(ctx, input, &extentLedger{})
		if e != nil {
			return nil, e
		}
		data, e := decoder.RunSubtitleFontExtract(ctx, r.storage.Supervisor, identity.Token(), r.ffmpeg, bridge.url, bridge.reservation, s.Index, r.libraries...)
		bridge.Close()
		if e != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		if len(data) < 12 || !(binary.BigEndian.Uint32(data[:4]) == 0x00010000 || string(data[:4]) == "OTTO" || string(data[:4]) == "ttcf" || string(data[:4]) == "true") {
			continue
		}
		total += len(data)
		if total > 16<<20 {
			break
		}
		if len(name) > 160 {
			name = "font" + ext
		}
		out = append(out, subtitles.FontAttachment{Name: name, Data: data})
	}
	if e = input.Validate(ctx); e != nil {
		return nil, e
	}
	return out, nil
}
