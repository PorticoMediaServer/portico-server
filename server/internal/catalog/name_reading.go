package catalog

import (
	"context"
	"database/sql"
	"path/filepath"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/naming"
)

// Names are read relative to the admitted source. The machine's mount path
// never contributes a title, year, edition or provider hint.
func movieFileReadingTx(ctx context.Context, tx *sql.Tx, library string, asset int64, path string) (MovieFile, error) {
	_, relative, err := assetSourceTx(ctx, tx, library, asset)
	if err != nil {
		return MovieFile{}, err
	}
	if relative == "" {
		relative = filepath.Base(path)
	}
	r := naming.Read(relative, "movie")
	title, year := r.PreferredTitle()
	f := MovieFile{Library: library, Asset: asset, Path: path, Title: title, Year: year, Provider: nameProviderIDs(r, "movie")}
	if len(r.Editions) > 0 {
		f.Edition = r.Editions[0].Value
	}
	if len(r.Parts) > 0 {
		f.Part = r.Parts[0].Value
	}
	return f, nil
}

func nameProviderIDs(r naming.Reading, kind string) []compactcatalog.ProviderIdentity {
	out := []compactcatalog.ProviderIdentity{}
	seen := map[string]bool{}
	for _, e := range r.ProviderIDs {
		if seen[e.Value.Provider] {
			continue
		}
		seen[e.Value.Provider] = true
		providerKind := kind
		if e.Value.Provider == "imdb" {
			providerKind = "title"
		}
		if e.Value.Provider == "anilist" {
			providerKind = "anime"
		}
		out = append(out, compactcatalog.ProviderIdentity{Provider: e.Value.Provider, Kind: providerKind, ID: e.Value.ID})
	}
	return out
}
