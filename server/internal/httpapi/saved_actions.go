package httpapi

import (
	"context"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
)

// Project only the catalogue members already admitted by this Saved read. The
// caller owns composition and the live post-read command-family fence.
func (d Dependencies) projectSavedActions(ctx context.Context, p identity.Principal, viewer catalog.Viewer, cat *catalog.Service, entries []*catalog.ContentEntry) error {
	pointers := make([]*catalog.ContentEntry, 0, len(entries))
	values := make([]catalog.ContentEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.Kind == "person" || entry.Kind == "channel" || entry.Kind == "playlist" || entry.Kind == "playlist_entry" || (entry.Hidden != nil && *entry.Hidden) {
			continue
		}
		pointers = append(pointers, entry)
		values = append(values, *entry)
	}
	rows := []catalog.HomeRow{{Entries: values}}
	if err := d.projectHomeActions(ctx, p, viewer, cat, rows, nil); err != nil {
		return err
	}
	for i, entry := range pointers {
		entry.HomeActions = rows[0].Entries[i].HomeActions
	}
	return nil
}
func savedContentEntries(out *catalog.ContentEnvelope) []*catalog.ContentEntry {
	entries := []*catalog.ContentEntry{}
	for i := range out.Sections {
		for j := range out.Sections[i].Entries {
			entry := &out.Sections[i].Entries[j]
			if entry.Kind == "playlist_entry" {
				if entry.Hidden == nil || !*entry.Hidden {
					entries = append(entries, entry.Media)
				}
			} else {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}
