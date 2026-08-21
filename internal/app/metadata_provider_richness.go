package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	metadataProviderRichMappingVersion = "provider-rich/v1"
	metadataProviderSnapshotMaxBytes   = 128 << 10
)

// metadataProviderRichProposal is a side-effect-free, canonical provider mapping.
// It is deliberately persistence-agnostic so the apply layer can commit its values,
// identities, relationships, credits, and images in one transaction.
type metadataProviderRichProposal struct {
	Provider       string `json:"provider"`
	MappingVersion string `json:"mappingVersion"`
	// PrimaryExternalType/ID identify supplemental evidence whose provider does
	// not own the media match itself (for example, Cover Art Archive evidence
	// attached to a MusicBrainz release). They are apply-only provenance and are
	// deliberately excluded from public/provider payload serialization.
	PrimaryExternalType string                          `json:"-"`
	PrimaryExternalID   string                          `json:"-"`
	Values              []metadataProviderValueProposal `json:"values,omitempty"`
	Relationships       []metadataRelationshipProposal  `json:"relationships,omitempty"`
	Images              []metadataProviderImageProposal `json:"images,omitempty"`
	Snapshot            json.RawMessage                 `json:"snapshot,omitempty"`
	SnapshotHash        string                          `json:"snapshotHash"`
	SourceHash          string                          `json:"sourceHash"`
	SourceBytes         int                             `json:"sourceBytes"`
	SnapshotCut         bool                            `json:"snapshotTruncated,omitempty"`
	// Supplements retain independent provider snapshots and provenance while
	// participating in the same atomic metadata apply transaction.
	Supplements []metadataProviderRichProposal `json:"-"`
}

type metadataProviderValueProposal struct {
	Field  string `json:"field"`
	Value  string `json:"value"`
	Locale string `json:"locale,omitempty"`
}

type metadataRelationshipProposal struct {
	Kind        string            `json:"kind"`
	Name        string            `json:"name,omitempty"`
	Role        string            `json:"role,omitempty"`
	Order       int               `json:"order,omitempty"`
	ExternalIDs map[string]string `json:"externalIds,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

type metadataProviderImageProposal struct {
	Kind        string  `json:"kind"`
	Path        string  `json:"path"`
	Locale      string  `json:"locale,omitempty"`
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	AspectRatio float64 `json:"aspectRatio,omitempty"`
	VoteAverage float64 `json:"voteAverage,omitempty"`
	VoteCount   int     `json:"voteCount,omitempty"`
	LocalPath   string  `json:"-"`
}

func mapTMDBProviderRich(result tmdbSearchResult) metadataProviderRichProposal {
	p := newProviderRichProposal("tmdb", result)
	p.addValue("releaseDate", result.ReleaseDate, "")
	p.addValue("firstAirDate", result.FirstAirDate, "")
	if path := safeTMDBPath(result.PosterPath); path != "" {
		p.Images = append(p.Images, metadataProviderImageProposal{Kind: "poster", Path: path})
	}
	if path := safeTMDBPath(result.BackdropPath); path != "" {
		p.Images = append(p.Images, metadataProviderImageProposal{Kind: "backdrop", Path: path})
	}
	p.addValue("originalTitle", firstNonEmpty(result.OriginalTitle, result.OriginalName), "")
	p.addValue("originalLanguage", result.OriginalLanguage, "")
	p.addValue("status", result.Status, "")
	if result.Runtime > 0 {
		p.addValue("runtimeMinutes", strconv.Itoa(result.Runtime), "")
	}
	for _, runtime := range result.EpisodeRunTime {
		if runtime > 0 {
			p.addValue("episodeRuntimeMinutes", strconv.Itoa(runtime), "")
		}
	}
	for _, title := range append(result.AlternativeTitles.Titles, result.AlternativeTitles.Results...) {
		p.addValue("alternateTitle", title.Title, title.ISO31661)
	}
	for _, lang := range result.SpokenLanguages {
		p.addRelationship("spokenLanguage", firstNonEmpty(lang.Name, lang.EnglishName), "", 0, map[string]string{"iso639-1": lang.ISO6391}, nil)
	}
	for _, country := range result.ProductionCountries {
		p.addRelationship("country", country.Name, "production", 0, map[string]string{"iso3166-1": country.ISO31661}, nil)
	}
	for _, code := range result.OriginCountry {
		p.addRelationship("country", "", "origin", 0, map[string]string{"iso3166-1": code}, nil)
	}
	for _, company := range result.ProductionCompanies {
		p.addTMDBCompany("company", company)
	}
	for _, network := range result.Networks {
		p.addTMDBCompany("network", network)
	}
	for i, creator := range result.CreatedBy {
		p.addRelationship("person", creator.Name, "Creator", i, intExternalID("tmdb", creator.ID), nil)
	}
	keywords := append(append([]tmdbKeyword{}, result.Keywords.Keywords...), result.Keywords.Results...)
	for _, keyword := range keywords {
		p.addRelationship("keyword", keyword.Name, "", 0, intExternalID("tmdb", keyword.ID), nil)
	}
	if c := result.BelongsToCollection; c != nil {
		p.addRelationship("collection", c.Name, "", 0, intExternalID("tmdb", c.ID), nil)
		if path := safeTMDBPath(c.PosterPath); path != "" {
			p.Images = append(p.Images, metadataProviderImageProposal{Kind: "collectionPoster", Path: path})
		}
		if path := safeTMDBPath(c.BackdropPath); path != "" {
			p.Images = append(p.Images, metadataProviderImageProposal{Kind: "collectionBackdrop", Path: path})
		}
	}
	for _, country := range result.ReleaseDates.Results {
		for _, rating := range country.ReleaseDates {
			p.addRelationship("release", "", strconv.Itoa(rating.Type), rating.Type, nil, map[string]string{"country": country.ISO31661, "language": rating.ISO6391, "releaseDate": rating.ReleaseDate})
			if strings.TrimSpace(rating.Certification) != "" {
				p.addRelationship("contentRating", rating.Certification, "", rating.Type, nil, map[string]string{"country": country.ISO31661, "releaseDate": rating.ReleaseDate})
			}
		}
	}
	for _, rating := range result.ContentRatings.Results {
		p.addRelationship("contentRating", rating.Rating, "", 0, nil, map[string]string{"country": rating.ISO31661})
	}
	for provider, raw := range result.ExternalIDs {
		if id := scalarProviderID(raw); id != "" {
			p.addRelationship("externalID", "", "", 0, map[string]string{strings.TrimSuffix(provider, "_id"): id}, nil)
		}
	}
	p.addTMDBImages("poster", result.Images.Posters)
	p.addTMDBImages("backdrop", result.Images.Backdrops)
	p.addTMDBImages("logo", result.Images.Logos)
	p.addTMDBImages("still", result.Images.Stills)
	p.normalize()
	return p
}

func mapAniListProviderRich(result aniListMedia) metadataProviderRichProposal {
	p := newProviderRichProposal("anilist", result)
	if path := strings.TrimSpace(firstNonEmpty(result.CoverImage.ExtraLarge, result.CoverImage.Large, result.CoverImage.Medium)); path != "" {
		p.Images = append(p.Images, metadataProviderImageProposal{Kind: "poster", Path: path})
	}
	if path := strings.TrimSpace(result.BannerImage); path != "" {
		p.Images = append(p.Images, metadataProviderImageProposal{Kind: "backdrop", Path: path})
	}
	p.addValue("startDate", formatAniListDate(result.StartDate), "")
	p.addValue("endDate", formatAniListDate(result.EndDate), "")
	p.addValue("source", result.Source, "")
	p.addValue("countryOfOrigin", result.Country, "")
	p.addValue("dominantColor", result.CoverImage.Color, "")
	for _, synonym := range result.Synonyms {
		p.addValue("alternateTitle", synonym, "")
	}
	p.addValue("title", result.Title.English, "en")
	p.addValue("title", result.Title.Romaji, "x-romaji")
	p.addValue("title", result.Title.Native, "und")
	p.addValue("status", result.Status, "")
	p.addValue("format", result.Format, "")
	p.addValue("season", result.Season, "")
	if result.SeasonYear > 0 {
		p.addValue("seasonYear", strconv.Itoa(result.SeasonYear), "")
	}
	if result.Episodes > 0 {
		p.addValue("episodes", strconv.Itoa(result.Episodes), "")
	}
	if result.Duration > 0 {
		p.addValue("durationMinutes", strconv.Itoa(result.Duration), "")
	}
	p.addRelationship("externalID", "", "", 0, intExternalID("anilist", result.ID), nil)
	p.addRelationship("externalID", "", "", 0, intExternalID("mal", result.IDMal), nil)
	for _, genre := range result.Genres {
		p.addRelationship("genre", genre, "", 0, nil, nil)
	}
	for _, tag := range result.Tags {
		p.addRelationship("tag", tag.Name, "", 0, nil, map[string]string{"rank": strconv.Itoa(tag.Rank)})
	}
	for _, studio := range result.Studios.Nodes {
		p.addRelationship("studio", studio.Name, "", 0, intExternalID("anilist", studio.ID), nil)
	}
	for i, edge := range result.Staff.Edges {
		portrait := firstNonEmpty(edge.Node.Image.Large, edge.Node.Image.Medium)
		p.addRelationship("person", firstNonEmpty(edge.Node.Name.Full, edge.Node.Name.Native), edge.Role, i, intExternalID("anilist", edge.Node.ID), map[string]string{"portrait": portrait})
		if portrait != "" {
			p.Images = append(p.Images, metadataProviderImageProposal{Kind: "personPortrait", Path: portrait})
		}
	}
	for i, edge := range result.Characters.Edges {
		portrait := firstNonEmpty(edge.Node.Image.Large, edge.Node.Image.Medium)
		attrs := map[string]string{"nativeName": edge.Node.Name.Native, "portrait": portrait}
		p.addRelationship("character", firstNonEmpty(edge.Node.Name.Full, edge.Node.Name.Native), edge.Role, i, intExternalID("anilist", edge.Node.ID), attrs)
		if portrait != "" {
			p.Images = append(p.Images, metadataProviderImageProposal{Kind: "characterPortrait", Path: portrait})
		}
		for j, actor := range edge.VoiceActors {
			actorPortrait := firstNonEmpty(actor.Image.Large, actor.Image.Medium)
			p.addRelationship("person", firstNonEmpty(actor.Name.Full, actor.Name.Native), "Voice", j, intExternalID("anilist", actor.ID), map[string]string{"character": firstNonEmpty(edge.Node.Name.Full, edge.Node.Name.Native), "portrait": actorPortrait})
			if actorPortrait != "" {
				p.Images = append(p.Images, metadataProviderImageProposal{Kind: "personPortrait", Path: actorPortrait})
			}
		}
	}
	for i, edge := range result.Relations.Edges {
		ids := intExternalID("anilist", edge.Node.ID)
		if edge.Node.IDMal > 0 {
			if ids == nil {
				ids = map[string]string{}
			}
			ids["mal"] = strconv.Itoa(edge.Node.IDMal)
		}
		p.addRelationship("relatedMedia", aniListDisplayTitle(edge.Node.Title), edge.RelationType, i, ids, map[string]string{"type": edge.Node.Type, "format": edge.Node.Format})
	}
	p.addAniListCoverage("staff", result.Staff.PageInfo, len(result.Staff.Edges), 75)
	p.addAniListCoverage("characters", result.Characters.PageInfo, len(result.Characters.Edges), 75)
	p.addAniListCoverage("relations", result.Relations.PageInfo, len(result.Relations.Edges), 25)
	p.normalize()
	return p
}

func formatAniListDate(date aniListFuzzyDate) string {
	if date.Year <= 0 {
		return ""
	}
	if date.Month <= 0 {
		return fmt.Sprintf("%04d", date.Year)
	}
	if date.Day <= 0 {
		return fmt.Sprintf("%04d-%02d", date.Year, date.Month)
	}
	return fmt.Sprintf("%04d-%02d-%02d", date.Year, date.Month, date.Day)
}

func (p *metadataProviderRichProposal) addAniListCoverage(kind string, info aniListPageInfo, returned, bound int) {
	if info.Total == 0 && returned == 0 {
		return
	}
	truncated := info.HasNextPage || info.Total > returned
	p.addRelationship("providerCoverage", kind, "bounded", 0, nil, map[string]string{
		"returned": strconv.Itoa(returned), "total": strconv.Itoa(info.Total), "bound": strconv.Itoa(bound), "truncated": strconv.FormatBool(truncated),
	})
}

func mapMusicBrainzRecordingProviderRich(recording musicBrainzRecording) metadataProviderRichProposal {
	p := newProviderRichProposal("musicbrainz", recording)
	p.addValue("title", recording.Title, "")
	p.addValue("disambiguation", recording.Disambiguation, "")
	if recording.Length > 0 {
		p.addValue("durationMilliseconds", strconv.Itoa(recording.Length), "")
	}
	p.addRelationship("externalID", "", "", 0, map[string]string{"musicbrainz": recording.ID}, nil)
	for _, isrc := range recording.ISRCs {
		p.addRelationship("externalID", "", "", 0, map[string]string{"isrc": isrc}, nil)
	}
	for _, alias := range recording.Aliases {
		p.addValue("alias", alias.Name, alias.Locale)
		p.addRelationship("alias", alias.Name, alias.Type, 0, nil, map[string]string{"locale": alias.Locale, "sortName": alias.SortName, "primary": strconv.FormatBool(alias.Primary)})
	}
	for _, genre := range recording.Genres {
		p.addRelationship("genre", genre.Name, "", 0, nil, map[string]string{"count": strconv.Itoa(genre.Count)})
	}
	for _, tag := range recording.Tags {
		p.addRelationship("tag", tag.Name, "", 0, nil, map[string]string{"count": strconv.Itoa(tag.Count)})
	}
	for i, credit := range recording.ArtistCredit {
		p.addMusicBrainzArtistCredit(credit, i)
	}
	for i, release := range recording.Releases {
		p.addMusicBrainzRelease(release, i)
	}
	p.addMusicBrainzRelations(recording.Relations)
	p.normalize()
	return p
}

func mapMusicBrainzReleaseGroupProviderRich(group musicBrainzReleaseGroup) metadataProviderRichProposal {
	p := newProviderRichProposal("musicbrainz", group)
	p.addValue("title", group.Title, "")
	p.addValue("primaryType", group.PrimaryType, "")
	p.addValue("firstReleaseDate", group.FirstReleaseDate, "")
	p.addValue("disambiguation", group.Disambiguation, "")
	for _, kind := range group.SecondaryTypes {
		p.addValue("secondaryType", kind, "")
	}
	for _, alias := range group.Aliases {
		p.addRelationship("alias", alias.Name, alias.Type, 0, nil, map[string]string{"locale": alias.Locale, "sortName": alias.SortName, "primary": strconv.FormatBool(alias.Primary)})
	}
	for _, genre := range group.Genres {
		p.addRelationship("genre", genre.Name, "", 0, nil, map[string]string{"count": strconv.Itoa(genre.Count)})
	}
	for _, tag := range group.Tags {
		p.addRelationship("tag", tag.Name, "", 0, nil, map[string]string{"count": strconv.Itoa(tag.Count)})
	}
	p.addMusicBrainzRelations(group.Relations)
	p.addRelationship("externalID", "", "", 0, map[string]string{"musicbrainz": group.ID}, nil)
	for i, credit := range group.ArtistCredit {
		p.addMusicBrainzArtistCredit(credit, i)
	}
	for i, release := range group.Releases {
		p.addMusicBrainzRelease(release, i)
	}
	p.normalize()
	return p
}

func mapMusicBrainzArtistProviderRich(artist musicBrainzArtist) metadataProviderRichProposal {
	p := newProviderRichProposal("musicbrainz", artist)
	p.addValue("name", artist.Name, "")
	p.addValue("sortName", artist.SortName, "")
	p.addValue("country", artist.Country, "")
	p.addValue("disambiguation", artist.Disambiguation, "")
	for _, alias := range artist.Aliases {
		p.addValue("alias", alias.Name, alias.Locale)
		p.addRelationship("alias", alias.Name, alias.Type, 0, nil, map[string]string{"locale": alias.Locale, "sortName": alias.SortName, "primary": strconv.FormatBool(alias.Primary)})
	}
	p.addRelationship("externalID", "", "", 0, map[string]string{"musicbrainz": artist.ID}, nil)
	for _, isni := range artist.ISNIs {
		p.addRelationship("externalID", "", "", 0, map[string]string{"isni": isni}, nil)
	}
	for _, ipi := range artist.IPIs {
		p.addRelationship("externalID", "", "", 0, map[string]string{"ipi": ipi}, nil)
	}
	p.normalize()
	return p
}

func (p *metadataProviderRichProposal) addMusicBrainzArtistCredit(credit musicBrainzArtistCredit, order int) {
	p.addRelationship("person", firstNonEmpty(credit.Name, credit.Artist.Name), "Artist", order, map[string]string{"musicbrainz": credit.Artist.ID}, map[string]string{"sortName": credit.Artist.SortName, "country": credit.Artist.Country, "joinPhrase": credit.JoinPhrase})
}
func (p *metadataProviderRichProposal) addMusicBrainzRelease(release musicBrainzRelease, order int) {
	p.addRelationship("release", release.Title, release.Status, order, map[string]string{"musicbrainz": release.ID, "barcode": release.Barcode, "asin": release.ASIN}, map[string]string{"date": release.Date, "country": release.Country, "releaseGroup": release.ReleaseGroup.ID, "packaging": release.Packaging, "language": release.TextRepresentation.Language, "script": release.TextRepresentation.Script})
	for _, info := range release.LabelInfo {
		p.addRelationship("label", info.Label.Name, "", 0, map[string]string{"musicbrainz": info.Label.ID}, map[string]string{"catalogNumber": info.CatalogNumber, "country": info.Label.Country})
	}
	for _, medium := range release.Media {
		p.addRelationship("medium", medium.Title, medium.Format, medium.Position, nil, map[string]string{"trackCount": strconv.Itoa(medium.TrackCount), "trackOffset": strconv.Itoa(medium.TrackOffset), "release": release.ID, "language": medium.TextRepresentation.Language, "script": medium.TextRepresentation.Script})
	}
	for _, medium := range release.Media {
		for _, track := range medium.Tracks {
			ids := map[string]string{"musicbrainz": track.ID}
			attrs := map[string]string{"mediumPosition": strconv.Itoa(medium.Position), "lengthMilliseconds": strconv.Itoa(track.Length)}
			if track.Recording != nil {
				attrs["recordingTitle"] = track.Recording.Title
				ids["musicbrainz-recording"] = track.Recording.ID
				if len(track.Recording.ISRCs) > 0 {
					attrs["isrcs"] = strings.Join(track.Recording.ISRCs, ",")
				}
			}
			p.addRelationship("track", track.Title, track.Number, track.Position, ids, attrs)
		}
	}
}

func (p *metadataProviderRichProposal) addMusicBrainzRelations(relations []musicBrainzRelation) {
	for i, relation := range relations {
		attrs := map[string]string{"direction": relation.Direction, "begin": relation.Begin, "end": relation.End, "ended": strconv.FormatBool(relation.Ended), "attributes": strings.Join(relation.Attributes, ",")}
		switch {
		case relation.Work != nil:
			attrs["workType"] = relation.Work.Type
			attrs["language"] = relation.Work.Language
			attrs["languages"] = strings.Join(relation.Work.Languages, ",")
			attrs["iswcs"] = strings.Join(relation.Work.ISWCs, ",")
			p.addRelationship("work", relation.Work.Title, relation.Type, i, map[string]string{"musicbrainz": relation.Work.ID}, attrs)
		case relation.Artist != nil:
			p.addRelationship("person", relation.Artist.Name, relation.Type, i, map[string]string{"musicbrainz": relation.Artist.ID}, attrs)
		case relation.Recording != nil:
			p.addRelationship("recording", relation.Recording.Title, relation.Type, i, map[string]string{"musicbrainz": relation.Recording.ID}, attrs)
		}
	}
}

func mapCoverArtArchiveProviderRich(entityType, entityID string, payload coverArtArchiveResponse) metadataProviderRichProposal {
	p := newProviderRichProposal("coverartarchive", payload)
	p.PrimaryExternalType = strings.TrimSpace(entityType)
	p.PrimaryExternalID = strings.TrimSpace(entityID)
	for _, artwork := range payload.Images {
		kind := "artwork"
		if artwork.Front {
			kind = "poster"
		} else if artwork.Back {
			kind = "back"
		} else if len(artwork.Types) > 0 {
			kind = strings.ToLower(artwork.Types[0])
		}
		path := strings.TrimSpace(artwork.Image)
		if path == "" {
			path = strings.TrimSpace(artwork.Thumbnails["large"])
		}
		if path == "" {
			continue
		}
		p.Images = append(p.Images, metadataProviderImageProposal{Kind: kind, Path: path})
		p.addRelationship("artwork", artwork.Comment, kind, 0, map[string]string{"coverartarchive": strconv.FormatInt(artwork.ID, 10)}, map[string]string{"approved": strconv.FormatBool(artwork.Approved), "types": strings.Join(artwork.Types, ","), "entityType": entityType, "entityID": entityID})
	}
	p.normalize()
	return p
}

func newProviderRichProposal(provider string, payload any) metadataProviderRichProposal {
	raw, _ := json.Marshal(payload)
	var sanitized any
	_ = json.Unmarshal(raw, &sanitized)
	sanitizeProviderSnapshot(&sanitized)
	raw, _ = json.Marshal(sanitized)
	sourceSum := sha256.Sum256(raw)
	p := metadataProviderRichProposal{Provider: provider, MappingVersion: metadataProviderRichMappingVersion, SourceHash: hex.EncodeToString(sourceSum[:]), SourceBytes: len(raw)}
	if len(raw) <= metadataProviderSnapshotMaxBytes {
		p.Snapshot = raw
	} else {
		p.SnapshotCut = true
		p.Snapshot, _ = json.Marshal(map[string]any{"truncated": true, "sourceSha256": p.SourceHash, "sourceBytes": len(raw)})
	}
	storedSum := sha256.Sum256(p.Snapshot)
	p.SnapshotHash = hex.EncodeToString(storedSum[:])
	return p
}

func sanitizeProviderSnapshot(value *any) {
	switch current := (*value).(type) {
	case string:
		current = strings.TrimSpace(strings.Map(func(r rune) rune {
			if r < 0x20 && r != '\n' && r != '\t' {
				return -1
			}
			return r
		}, current))
		if runes := []rune(current); len(runes) > 8192 {
			current = string(runes[:8192])
		}
		*value = current
	case []any:
		if len(current) > 512 {
			current = current[:512]
		}
		for i := range current {
			sanitizeProviderSnapshot(&current[i])
		}
		*value = current
	case map[string]any:
		for key, child := range current {
			clean := any(child)
			sanitizeProviderSnapshot(&clean)
			current[key] = clean
		}
	}
}

func (p *metadataProviderRichProposal) addValue(field, value, locale string) {
	if value = cleanProviderText(value); field != "" && value != "" {
		p.Values = append(p.Values, metadataProviderValueProposal{Field: field, Value: value, Locale: strings.TrimSpace(locale)})
	}
}
func (p *metadataProviderRichProposal) addRelationship(kind, name, role string, order int, ids, attrs map[string]string) {
	ids = cleanStringMap(ids)
	attrs = cleanStringMap(attrs)
	name = cleanProviderText(name)
	role = cleanProviderText(role)
	if name == "" && len(ids) == 0 && len(attrs) == 0 {
		return
	}
	p.Relationships = append(p.Relationships, metadataRelationshipProposal{Kind: strings.TrimSpace(kind), Name: name, Role: role, Order: order, ExternalIDs: ids, Attributes: attrs})
}

func cleanProviderText(value string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, value))
}
func (p *metadataProviderRichProposal) addTMDBCompany(kind string, company tmdbCompany) {
	p.addRelationship(kind, company.Name, "", 0, intExternalID("tmdb", company.ID), map[string]string{"country": company.OriginCountry, "logoPath": safeTMDBPath(company.LogoPath)})
}
func (p *metadataProviderRichProposal) addTMDBImages(kind string, images []tmdbImage) {
	for _, image := range images {
		if path := safeTMDBPath(image.FilePath); path != "" {
			p.Images = append(p.Images, metadataProviderImageProposal{Kind: kind, Path: path, Locale: image.ISO6391, Width: image.Width, Height: image.Height, AspectRatio: image.AspectRatio, VoteAverage: image.VoteAverage, VoteCount: image.VoteCount})
		}
	}
}
func intExternalID(provider string, id int) map[string]string {
	if id <= 0 {
		return nil
	}
	return map[string]string{provider: strconv.Itoa(id)}
}
func scalarProviderID(raw any) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case float64:
		if value == float64(int64(value)) {
			return strconv.FormatInt(int64(value), 10)
		}
	}
	return ""
}
func safeTMDBPath(path string) string {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		return ""
	}
	return path
}
func cleanStringMap(values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		if key = strings.TrimSpace(key); key != "" {
			if value = cleanProviderText(value); value != "" {
				out[key] = value
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
func (p *metadataProviderRichProposal) normalize() {
	sort.Slice(p.Values, func(i, j int) bool { return fmt.Sprint(p.Values[i]) < fmt.Sprint(p.Values[j]) })
	p.Values = dedupeProviderValues(p.Values)
	sort.Slice(p.Relationships, func(i, j int) bool {
		a, _ := json.Marshal(p.Relationships[i])
		b, _ := json.Marshal(p.Relationships[j])
		return string(a) < string(b)
	})
	p.Relationships = dedupeProviderRelationships(p.Relationships)
	sort.Slice(p.Images, func(i, j int) bool {
		if p.Images[i].Kind != p.Images[j].Kind {
			return p.Images[i].Kind < p.Images[j].Kind
		}
		return p.Images[i].Path < p.Images[j].Path
	})
	p.Images = dedupeProviderImages(p.Images)
}
func dedupeProviderValues(values []metadataProviderValueProposal) []metadataProviderValueProposal {
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func dedupeProviderRelationships(values []metadataRelationshipProposal) []metadataRelationshipProposal {
	out := values[:0]
	previous := ""
	for _, value := range values {
		encoded, _ := json.Marshal(value)
		key := string(encoded)
		if len(out) == 0 || key != previous {
			out = append(out, value)
			previous = key
		}
	}
	return out
}

func dedupeProviderImages(values []metadataProviderImageProposal) []metadataProviderImageProposal {
	out := values[:0]
	seen := map[string]struct{}{}
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value.Kind)) + "\x00" + strings.TrimSpace(value.Path)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
