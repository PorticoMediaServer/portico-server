package playbackv1

import (
	"context"
	"database/sql"
	"fmt"
	"portico.local/server/internal/catalog"
)

// discVersion projects one checked title into its existing physical version.
// It does not manufacture another asset/version or borrow the main's streams.
func (s *Service) discVersion(ctx context.Context, tx *sql.Tx, v *Version, ref catalog.DiscTitleRef) ([]Chapter, error) {
	if len(v.Parts) != 1 {
		return nil, &FieldError{Path: "discTitleId"}
	}
	d, err := catalog.ReadDiscSelection(ctx, tx, v.ID)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, ErrNotFound
	}
	title, f, _, err := catalog.ResolveDiscTitleTx(ctx, tx, d, ref)
	if err != nil {
		return nil, err
	}
	if v.DiscTitles != nil {
		for _, option := range v.DiscTitles.Titles {
			if option.ID == ref.ID {
				v.Disc = &catalog.DiscMediaFacts{Kind: catalog.FileInfoDiscKind(d.Structure.Kind), Packaging: catalog.DiscPackaging(d.Structure.Packaging), TitleNumber: option.Number, ChapterCount: len(title.Chapters)}
				break
			}
		}
	}
	v.DiscTitle = &ref
	v.Container = f.Container
	v.DurationMs = title.DurationMs
	v.Parts[0].DurationMs = title.DurationMs
	v.BitrateKbps = 0
	v.Video = []VideoStream{}
	v.Audio = []AudioStream{}
	v.Subtitles = []SubStream{}
	known, rate := true, int64(0)
	for _, st := range f.Streams {
		s.appendStream(v, st)
		if st.Type == "video" || st.Type == "audio" {
			known = known && st.Detail.BitRate > 0
			rate += st.Detail.BitRate
		}
	}
	// Embedded subtitle resources currently belong to the accepted main title.
	// Until their per-title producer exists this preview truthfully offers none.
	v.Subtitles = []SubStream{}
	if known {
		v.BitrateKbps = int(rate / 1000)
	}
	out := make([]Chapter, 0, len(title.Chapters))
	for i, c := range title.Chapters {
		out = append(out, Chapter{Title: fmt.Sprintf("Chapter %d", i+1), StartMs: c.StartMs})
	}
	return out, nil
}
