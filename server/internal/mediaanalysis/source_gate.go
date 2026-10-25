package mediaanalysis

import (
	"context"
	"database/sql"
	"errors"
)

// sourceStageAllowedTx answers whether the saved remote source that contains
// this library folder allows a whole-file analysis stage. A local folder, or a
// source with no saved page, allows everything; only the stages the page names
// are restricted, and a separate checksum stage never implies fingerprinting.
func sourceStageAllowedTx(ctx context.Context, tx *sql.Tx, librarySource, operation string) (bool, error) {
	switch operation {
	case "trickplay", "chapter_images", "waveform", "loudness", "checksum":
	default:
		return true, nil
	}
	var seekPreviews, chapterImages, waveforms, loudness, checksums bool
	err := tx.QueryRowContext(ctx, `SELECT st.seek_previews,st.chapter_images,st.waveforms,st.loudness,st.checksums FROM library_sources s JOIN remote_source_settings st ON st.source_id=s.remote_source_id WHERE s.id=?`, librarySource).
		Scan(&seekPreviews, &chapterImages, &waveforms, &loudness, &checksums)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	switch operation {
	case "trickplay":
		return seekPreviews, nil
	case "chapter_images":
		return chapterImages, nil
	case "waveform":
		return waveforms, nil
	case "loudness":
		return loudness, nil
	case "checksum":
		return checksums, nil
	}
	return true, nil
}
