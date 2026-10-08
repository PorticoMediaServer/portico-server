package catalog

import (
	"context"
	"database/sql"
	"path/filepath"
	"portico.local/server/internal/compactcatalog"
	"sort"
	"strings"
)

// Sidecar extras are recognised from the owning media folder, never from
// provider data. Classification is a pure function of the path so inventory can
// fence one small additional publication without a second filesystem pass.

type ExtraClassification struct {
	Kind       string
	Title      string
	ParentPath string
}

var extraFolders = map[string]string{
	"extras":             "other",
	"featurettes":        "featurette",
	"trailers":           "trailer",
	"deleted scenes":     "deleted_scene",
	"behind the scenes":  "behind_the_scenes",
	"behind-the-scenes":  "behind_the_scenes",
	"interviews":         "interview",
	"scenes":             "scene",
	"shorts":             "short",
	"other":              "other",
	"specials":           "other",
	"bonus":              "other",
	"bonus disc":         "other",
	"deleted":            "deleted_scene",
	"making of":          "behind_the_scenes",
	"behind the scene":   "behind_the_scenes",
	"featurette":         "featurette",
	"trailer":            "trailer",
	"interview":          "interview",
	"short":              "short",
	"deleted scene":      "deleted_scene",
	"behind_the_scenes":  "behind_the_scenes",
	"extra":              "other",
	"video extras":       "other",
	"bonus features":     "other",
	"supplemental":       "other",
	"supplementals":      "other",
	"extras and bonuses": "other",
}

var extraSuffixes = []struct{ suffix, kind string }{
	{"-trailer", "trailer"},
	{"-featurette", "featurette"},
	{"-deleted", "deleted_scene"},
	{"-behindthescenes", "behind_the_scenes"},
	{"-interview", "interview"},
	{"-scene", "scene"},
	{"-short", "short"},
	{"-other", "other"},
}

func ExtraKinds() []string {
	return []string{"trailer", "featurette", "deleted_scene", "behind_the_scenes", "interview", "scene", "short", "other"}
}

func extraKindLabel(kind string) string {
	switch kind {
	case "trailer":
		return "Trailers"
	case "featurette":
		return "Featurettes"
	case "deleted_scene":
		return "Deleted Scenes"
	case "behind_the_scenes":
		return "Behind the Scenes"
	case "interview":
		return "Interviews"
	case "scene":
		return "Scenes"
	case "short":
		return "Shorts"
	}
	return "Extras"
}

func extraTitle(name string) string {
	name = strings.TrimSuffix(name, filepath.Ext(name))
	lowered := strings.ToLower(name)
	for _, entry := range extraSuffixes {
		if strings.HasSuffix(lowered, entry.suffix) {
			name = name[:len(name)-len(entry.suffix)]
			break
		}
	}
	name = strings.TrimSpace(strings.NewReplacer("_", " ", ".", " ").Replace(name))
	if name == "" {
		return "Extra"
	}
	return name
}

// ClassifyExtra reports whether a media file under root is a sidecar extra, the
// extra kind, a display title and the media folder that owns it. Folder
// evidence outranks a filename suffix; the most specific recognised folder wins.
func ClassifyExtra(root, path string) (ExtraClassification, bool) {
	return classifyExtra(root, path, false)
}

// ClassifyExtraFor is ClassifyExtra for a library of this kind. In a TV or
// anime library a "Specials" folder is Season 0, as Plex and Kodi read it,
// not an extras folder; everywhere else it stays an extras folder.
func ClassifyExtraFor(kind, root, path string) (ExtraClassification, bool) {
	return classifyExtra(root, path, kind == "tv" || kind == "anime")
}

func classifyExtra(root, path string, episodic bool) (ExtraClassification, bool) {
	relative, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		return ExtraClassification{}, false
	}
	segments := strings.Split(filepath.ToSlash(relative), "/")
	if len(segments) == 0 {
		return ExtraClassification{}, false
	}
	name := segments[len(segments)-1]
	directories := segments[:len(segments)-1]
	kind, firstIndex := "", -1
	for index, segment := range directories {
		folder := strings.ToLower(strings.TrimSpace(segment))
		if episodic && folder == "specials" {
			continue
		}
		if mapped, ok := extraFolders[folder]; ok {
			if firstIndex < 0 {
				firstIndex = index
			}
			if mapped != "other" || kind == "" {
				kind = mapped
			}
		}
	}
	if firstIndex >= 0 {
		parent := filepath.Join(append([]string{root}, directories[:firstIndex]...)...)
		return ExtraClassification{Kind: kind, Title: extraTitle(name), ParentPath: parent}, true
	}
	lowered := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	for _, entry := range extraSuffixes {
		if strings.HasSuffix(lowered, entry.suffix) {
			parent := filepath.Join(append([]string{root}, directories...)...)
			return ExtraClassification{Kind: entry.kind, Title: extraTitle(name), ParentPath: parent}, true
		}
	}
	return ExtraClassification{}, false
}

// publishExtraTx records an extra as an ordinary item so it plays through the
// normal item path, plus one relationship row naming its owning media folder.
func (s *Service) publishExtraTx(ctx context.Context, tx *sql.Tx, source LibrarySource, asset, path string, extra ExtraClassification) error {
	relative, err := filepath.Rel(source.ResolvedPath, path)
	if err != nil {
		return err
	}
	handle, err := compactcatalog.LibraryTx(ctx, tx, source.LibraryID)
	if err != nil {
		return err
	}
	assetID, err := compactcatalog.AssetByTokenTx(ctx, tx, asset)
	if err != nil {
		return err
	}
	root, err := compactcatalog.LibraryRootTx(ctx, tx, handle)
	if err != nil {
		return err
	}
	item, _, err := compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: handle, Kind: compactcatalog.Extra, Key: compactcatalog.ExtraKey(root, path), Title: extra.Title, Added: catalogedNow()})
	if err != nil {
		return err
	}
	if err = compactcatalog.LinkAssetTx(ctx, tx, item, assetID, compactcatalog.Link{}); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO item_extras(item_id,library_id,parent_path,kind,title,asset_id,relative_path) VALUES(?,?,?,?,?,?,?)
 ON CONFLICT(item_id) DO UPDATE SET library_id=excluded.library_id,parent_path=excluded.parent_path,kind=excluded.kind,title=excluded.title,asset_id=excluded.asset_id,relative_path=excluded.relative_path`,
		item, source.LibraryID, extra.ParentPath, extra.Kind, extra.Title, asset, filepath.ToSlash(relative))
	return err
}

type DetailExtra struct {
	Type  string         `json:"type"`
	Label string         `json:"label"`
	Items []ContentEntry `json:"items"`
}

// itemExtras resolves the extras that sit beside an item's own media — all of
// them: they are the title's own. The owning folder is the join key, so extras
// survive the parent being re-identified.
func (s *Service) itemExtras(profile string, item Item) ([]DetailExtra, error) {
	out := []DetailExtra{}
	if item.Kind == "extra" {
		return out, nil
	}
	rows, e := s.read().Query(`SELECT DISTINCT a.path FROM catalog_entities i JOIN catalog_asset_links link ON link.entity_id=i.id JOIN catalog_assets a ON a.id=link.asset_id WHERE i.public_id=pid_blob(?)`, item.ID)
	if e != nil {
		return nil, e
	}
	folders := map[string]bool{}
	for rows.Next() {
		var path string
		if e = rows.Scan(&path); e != nil {
			rows.Close()
			return nil, e
		}
		folders[filepath.Dir(path)] = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil || len(folders) == 0 {
		return out, e
	}
	ordered := []string{}
	for folder := range folders {
		ordered = append(ordered, folder)
	}
	sort.Strings(ordered)
	grouped := map[string][]string{}
	for _, folder := range ordered {
		rows, e := s.read().Query(`SELECT pid(i.public_id),e.kind FROM item_extras e JOIN catalog_entities i ON i.id=e.item_id JOIN catalog_libraries l ON l.id=i.library_id AND l.library_id=e.library_id WHERE e.library_id=? AND e.parent_path=? AND `+homeVisible("i.id")+` ORDER BY e.kind,e.title COLLATE NOCASE,e.item_id`, item.LibraryID, folder)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var id, kind string
			if e = rows.Scan(&id, &kind); e != nil {
				rows.Close()
				return nil, e
			}
			grouped[kind] = append(grouped[kind], id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	for _, kind := range ExtraKinds() {
		ids := grouped[kind]
		if len(ids) == 0 {
			continue
		}
		entries, e := s.homeEntries(profile, ids)
		if e != nil {
			return nil, e
		}
		out = append(out, DetailExtra{Type: kind, Label: extraKindLabel(kind), Items: entries})
	}
	return out, nil
}
