package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/compactcatalog"
	"regexp"
	"strconv"
	"strings"
)

type Artist struct {
	ID        string `json:"id"`
	LibraryID string `json:"libraryId"`
	Name      string `json:"name"`
}
type Album struct {
	ProviderReleaseID      string `json:"providerReleaseId,omitempty"`
	ProviderReleaseGroupID string `json:"providerReleaseGroupId,omitempty"`
	ProviderReleaseDate    string `json:"providerReleaseDate,omitempty"`
	ProviderReleaseCountry string `json:"providerReleaseCountry,omitempty"`
	ProviderObservedAt     string `json:"providerObservedAt,omitempty"`
	ID                     string `json:"id"`
	LibraryID              string `json:"libraryId"`
	Title                  string `json:"title"`
	AlbumArtistID          string `json:"albumArtistId"`
	AlbumArtist            string `json:"albumArtist"`
	Year                   int    `json:"year,omitempty"`
	ProviderMatchStatus    string `json:"providerMatchStatus"`
	PosterURL              string `json:"posterUrl,omitempty"`
}
type Book struct {
	ID                  string      `json:"id"`
	LibraryID           string      `json:"libraryId"`
	Title               string      `json:"title"`
	Author              string      `json:"author,omitempty"`
	Narrator            string      `json:"narrator,omitempty"`
	ProviderMatchStatus string      `json:"providerMatchStatus"`
	PosterURL           string      `json:"posterUrl,omitempty"`
	Resume              *BookResume `json:"resume,omitempty"`
}
type BookResume struct {
	ItemID          string  `json:"itemId"`
	PositionSeconds float64 `json:"positionSeconds"`
}
type SongInfo struct {
	ProviderTitle          string `json:"providerTitle,omitempty"`
	ProviderArtist         string `json:"providerArtist,omitempty"`
	ProviderSourceURL      string `json:"providerSourceUrl,omitempty"`
	ProviderRecordingID    string `json:"providerRecordingId,omitempty"`
	ProviderReleaseID      string `json:"providerReleaseId,omitempty"`
	ProviderReleaseGroupID string `json:"providerReleaseGroupId,omitempty"`
	ProviderTrackID        string `json:"providerTrackId,omitempty"`
	ProviderReleaseStatus  string `json:"providerReleaseStatus"`
	ProviderObservedAt     string `json:"providerObservedAt,omitempty"`
	AlbumID                string `json:"albumId"`
	AlbumTitle             string `json:"albumTitle"`
	AlbumArtist            string `json:"albumArtist"`
	Artist                 string `json:"artist"`
	DiscNumber             *int   `json:"discNumber,omitempty"`
	TrackNumber            *int   `json:"trackNumber,omitempty"`
	ProviderMatchStatus    string `json:"providerMatchStatus"`
	LocalMetadataIssue     string `json:"localMetadataIssue,omitempty"`
}
type LocalAudioPolicy struct {
	Revision  int64  `json:"revision"`
	LocalMode string `json:"localMode"`
}
type BookFileInfo struct {
	LocalPolicy        *LocalAudioPolicy  `json:"localPolicy,omitempty"`
	LocalMetadata      *LocalBookMetadata `json:"localMetadata,omitempty"`
	BookID             string             `json:"bookId"`
	BookTitle          string             `json:"bookTitle"`
	DiscNumber         *int               `json:"discNumber,omitempty"`
	PartNumber         *int               `json:"partNumber,omitempty"`
	SourceBoundary     string             `json:"sourceBoundary"`
	LocalMetadataIssue string             `json:"localMetadataIssue,omitempty"`
}
type BookChapter struct {
	ID           string  `json:"id"`
	ItemID       string  `json:"itemId"`
	Index        int     `json:"index"`
	Title        string  `json:"title"`
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds"`
}

var discFolderPattern = regexp.MustCompile(`(?i)^(disc|disk|cd)[ ._-]*([0-9]{1,3})$`)
var leadingNumber = regexp.MustCompile(`^([0-9]{1,3})[ ._-]`)

func tagNumber(s string) any {
	first, _, _ := strings.Cut(s, "/")
	n, err := strconv.Atoi(strings.TrimSpace(first))
	if err != nil || n < 1 || n > 9999 {
		return nil
	}
	return n
}
func firstTag(tags map[string]string, names ...string) string {
	for _, name := range names {
		// One line: tabs and line breaks inside a tagged name become spaces (CD-27).
		if value := strings.Join(strings.Fields(tags[name]), " "); value != "" {
			return value
		}
	}
	return ""
}
func localKey(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// albumRetireChecks are the personal, owner and provider state that keep an
// emptied album: with any of it the album is retired (it keeps its identity and
// state and leaves browse); with none it is deleted (the identity ledger still
// gives the same public id if it comes back).
var albumRetireChecks = []string{
	`SELECT EXISTS(SELECT 1 FROM catalog_songs WHERE album_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM mb_jobs WHERE kind='album' AND entity_id=? AND manual=1)`,
	`SELECT EXISTS(SELECT 1 FROM mb_album_links WHERE album_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM container_personal_state WHERE container_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM metadata_owner_fields WHERE kind='album' AND entity_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM metadata_owner_history WHERE kind='album' AND entity_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM metadata_relationship_decisions WHERE kind='album' AND entity_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM artwork_selections WHERE kind='album' AND entity_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM artwork_uploads WHERE kind='album' AND entity_id=?)`,
}

// bookRetireChecks is the same rule for an audiobook a file has left.
var bookRetireChecks = []string{
	`SELECT EXISTS(SELECT 1 FROM container_personal_state WHERE container_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM catalog_book_group_members WHERE book_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM metadata_owner_fields WHERE kind='book' AND entity_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM metadata_owner_history WHERE kind='book' AND entity_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM metadata_relationship_decisions WHERE kind='book' AND entity_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM artwork_selections WHERE kind='book' AND entity_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM artwork_uploads WHERE kind='book' AND entity_id=?)`,
}

func settleEmptiedBook(ctx context.Context, tx *sql.Tx, book int64) error {
	var files bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_book_files WHERE book_id=?)`, book).Scan(&files); err != nil || files {
		return err
	}
	return settleEmptied(ctx, tx, book, bookRetireChecks)
}

// artistRetireChecks is the same rule for the album's artist.
var artistRetireChecks = []string{
	`SELECT EXISTS(SELECT 1 FROM catalog_albums WHERE artist_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM catalog_song_artists WHERE artist_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM container_personal_state WHERE container_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM metadata_owner_fields WHERE kind='artist' AND entity_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM metadata_owner_history WHERE kind='artist' AND entity_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM metadata_relationship_decisions WHERE kind='artist' AND entity_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM artwork_selections WHERE kind='artist' AND entity_id=?)`,
	`SELECT EXISTS(SELECT 1 FROM artwork_uploads WHERE kind='artist' AND entity_id=?)`,
}

func anyPresent(ctx context.Context, tx *sql.Tx, id int64, checks []string) (bool, error) {
	for _, check := range checks {
		var present bool
		if err := tx.QueryRowContext(ctx, check, id).Scan(&present); err != nil {
			return false, err
		}
		if present {
			return true, nil
		}
	}
	return false, nil
}

// retireEmptiedAlbumTx settles the album a song just left: while songs remain
// nothing happens; an empty album with state is retired, without state it is
// deleted; its artist then gets the same treatment. Two albums are never
// merged by content: only the id passed in can go.
func retireEmptiedAlbumTx(ctx context.Context, tx *sql.Tx, album int64) error {
	var artist int64
	if err := tx.QueryRowContext(ctx, `SELECT artist_id FROM catalog_albums WHERE entity_id=?`, album).Scan(&artist); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	var songs bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_songs WHERE album_id=?)`, album).Scan(&songs); err != nil || songs {
		return err
	}
	if err := settleEmptied(ctx, tx, album, albumRetireChecks); err != nil {
		return err
	}
	var used bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_albums a JOIN catalog_entities e ON e.id=a.entity_id AND e.retired=0 WHERE a.artist_id=?1)
 OR EXISTS(SELECT 1 FROM catalog_song_artists WHERE artist_id=?1)`, artist).Scan(&used); err != nil || used {
		return err
	}
	return settleEmptied(ctx, tx, artist, artistRetireChecks)
}

func settleEmptied(ctx context.Context, tx *sql.Tx, id int64, checks []string) error {
	blocked, err := anyPresent(ctx, tx, id, checks)
	if err != nil {
		return err
	}
	if blocked {
		return compactcatalog.RetireEntityTx(ctx, tx, id)
	}
	return compactcatalog.DeleteEntityTx(ctx, tx, id)
}

// artist finds or creates a library's artist by name.
func (s *Service) artist(ctx context.Context, tx *sql.Tx, library int64, name string) (int64, error) {
	key := localKey(name)
	id, created, err := compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Artist, Key: compactcatalog.ArtistKey(key), Title: name})
	if err != nil || !created {
		return id, err
	}
	return id, compactcatalog.SetFactsTx(ctx, tx, id, map[string]any{"local_key": key})
}

func (s *Service) commitAudio(ctx context.Context, tx *sql.Tx, library, kind, root, asset, path string, f assets.Facts) error {
	if f.AudioCodec == "" && !f.InventoryOnly {
		return errors.New("audio library source has no audio stream")
	}
	if err := persistAudioEvidence(tx, library, asset, f); err != nil {
		return err
	}
	handle, err := compactcatalog.LibraryTx(ctx, tx, library)
	if err != nil {
		return err
	}
	assetID, err := compactcatalog.AssetByTokenTx(ctx, tx, asset)
	if err != nil {
		return err
	}
	var existingItem int64
	if err := tx.QueryRow(`SELECT l.entity_id FROM catalog_asset_links l INDEXED BY catalog_asset_links_asset JOIN catalog_entities i ON i.id=l.entity_id WHERE l.asset_id=? AND i.library_id=? LIMIT 1`, assetID, handle).Scan(&existingItem); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	projected, errProjection := audioProjection(tx, library, existingItem, f)
	if errProjection != nil {
		return errProjection
	}
	f = projected
	title := firstTag(f.Tags, "title")
	if title == "" {
		title, _ = FilenameTitle(path)
	}
	if len(title) > 500 {
		title = title[:500]
	}
	folder := filepath.Dir(path)
	disc := tagNumber(firstTag(f.Tags, "disc"))
	if match := discFolderPattern.FindStringSubmatch(filepath.Base(folder)); match != nil {
		if disc == nil {
			disc = tagNumber(match[2])
		}
		folder = filepath.Dir(folder)
	}
	albumTitle := firstTag(f.Tags, "album")
	if albumTitle == "" {
		albumTitle = filepath.Base(folder)
		if kind == "audiobook" && filepath.Clean(folder) == filepath.Clean(root) {
			albumTitle = title
		}
		if f.LocalMetadataIssue == "" {
			f.LocalMetadataIssue = "album_or_book_identity_from_local_path"
		}
	}
	track := tagNumber(firstTag(f.Tags, "track"))
	if track == nil {
		if match := leadingNumber.FindStringSubmatch(filepath.Base(path)); match != nil {
			track = tagNumber(match[1])
		}
	}
	itemKind := compactcatalog.Track
	if kind == "audiobook" {
		itemKind = compactcatalog.Part
	}
	var item int64
	err = tx.QueryRow(`SELECT l.entity_id FROM catalog_asset_links l INDEXED BY catalog_asset_links_asset JOIN catalog_entities i ON i.id=l.entity_id WHERE l.asset_id=? AND i.library_id=? AND i.kind=? LIMIT 1`, assetID, handle, itemKind).Scan(&item)
	if errors.Is(err, sql.ErrNoRows) {
		identityRoot, err := compactcatalog.LibraryRootTx(ctx, tx, handle)
		if err != nil {
			return err
		}
		if item, _, err = compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: handle, Kind: itemKind, Key: compactcatalog.ItemKey(identityRoot, path, 0), Title: title, Added: catalogedNow()}); err != nil {
			return err
		}
		if err = compactcatalog.LinkAssetTx(ctx, tx, item, assetID, compactcatalog.Link{}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	// An observed physical asset keeps its existing logical parent when local
	// tags still describe that parent. A folder rename is not a regrouping hint.
	if firstTag(f.Tags, "album") == "" {
		var prior string
		query := `SELECT a.title FROM catalog_songs x JOIN catalog_entities a ON a.id=x.album_id WHERE x.entity_id=?`
		if kind == "audiobook" {
			query = `SELECT b.title FROM catalog_book_files x JOIN catalog_entities b ON b.id=x.book_id WHERE x.entity_id=?`
		}
		if e := tx.QueryRow(query, item).Scan(&prior); e == nil {
			albumTitle = prior
		} else if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
	}
	fields := map[string]any{"title": title}
	if f.ArtworkKey != "" {
		fields["poster_url"] = "local:" + f.ArtworkKey
	}
	if err = compactcatalog.SetFieldsTx(ctx, tx, item, compactcatalog.Automatic, fields); err != nil {
		return err
	}
	if err = compactcatalog.ProjectAudioLocalGenresItem(ctx, tx, library, item); err != nil {
		return err
	}

	if kind == "music" {
		performer := firstTag(f.Tags, "artist")
		if performer == "" {
			performer = "Unknown artist"
		}
		albumArtist := firstTag(f.Tags, "album_artist", "albumartist")
		if albumArtist == "" {
			albumArtist = performer
			if f.Tags["compilation"] == "1" || f.Tags["compilation"] == "true" {
				albumArtist = "Various Artists"
			}
		}
		if _, err = s.artist(ctx, tx, handle, performer); err != nil {
			return err
		}
		albumArtistID, err := s.artist(ctx, tx, handle, albumArtist)
		if err != nil {
			return err
		}
		year := 0
		value := firstTag(f.Tags, "date", "year")
		if len(value) >= 4 {
			year, _ = strconv.Atoi(value[:4])
		}
		var album int64
		if e := tx.QueryRow(`SELECT x.album_id FROM catalog_songs x JOIN mb_jobs j ON j.kind='album' AND j.entity_id=x.album_id AND j.manual=1 WHERE x.entity_id=?`, item).Scan(&album); e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if album == 0 && (firstTag(f.Tags, "album") == "" || firstTag(f.Tags, "album_artist", "artist") == "" || strings.HasPrefix(f.LocalMetadataIssue, "invalid_")) {
			if e := tx.QueryRow(`SELECT album_id FROM catalog_songs WHERE entity_id=?`, item).Scan(&album); e != nil && !errors.Is(e, sql.ErrNoRows) {
				return e
			}
		}
		if album == 0 {
			e := tx.QueryRow(`SELECT x.album_id FROM catalog_songs x JOIN catalog_entities a ON a.id=x.album_id JOIN catalog_albums d ON d.entity_id=a.id
 WHERE x.entity_id=? AND a.title=? AND d.artist_id=? AND a.year=? AND (COALESCE((SELECT release_id FROM mb_album_links WHERE album_id=a.id),'')='' OR ?='' OR (SELECT release_id FROM mb_album_links WHERE album_id=a.id)=?)`, item, albumTitle, albumArtistID, year, f.Tags["musicbrainz_albumid"], f.Tags["musicbrainz_albumid"]).Scan(&album)
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return e
			}
		}
		if album != 0 {
			var pinned bool
			if err = tx.QueryRow(`SELECT manual FROM mb_jobs WHERE kind='album' AND entity_id=?`, album).Scan(&pinned); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if !pinned {
				compatible, e := localEditionCompatible(tx, album, audioEdition(f.Tags))
				if e != nil {
					return e
				}
				if !compatible {
					album = 0
				}
			}
		}
		if album == 0 {
			// Missing per-track edition tags do not split one physical album.
			// Require the same folder/local identity and exactly one compatible
			// edition; conflicting known evidence still creates a separate album.
			prefix := localKey(folder + "|" + albumTitle + "|" + albumArtist + "|" + strconv.Itoa(year) + "|")
			rows, e := tx.Query(`SELECT a.id,d.edition_key FROM catalog_albums d INDEXED BY catalog_albums_artist JOIN catalog_entities a ON a.id=d.entity_id
 WHERE d.artist_id=? AND a.library_id=? AND a.title=? AND a.year=? AND substr(d.local_key,1,length(?))=?`, albumArtistID, handle, albumTitle, year, prefix, prefix)
			if e != nil {
				return e
			}
			var candidate int64
			merged := ""
			count := 0
			for rows.Next() {
				var id int64
				var old string
				if e = rows.Scan(&id, &old); e != nil {
					rows.Close()
					return e
				}
				if edition, ok := mergeLocalEdition(old, audioEdition(f.Tags)); ok {
					candidate, merged = id, edition
					count++
				}
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			if count == 1 {
				album = candidate
				if e = compactcatalog.SetFactsTx(ctx, tx, album, map[string]any{"edition_key": merged, "local_key": prefix + merged}); e != nil {
					return e
				}
			}
		}
		if album == 0 {
			key := localKey(folder + "|" + albumTitle + "|" + albumArtist + "|" + strconv.Itoa(year) + "|" + audioEdition(f.Tags))
			var created bool
			if album, created, err = compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: handle, Kind: compactcatalog.Album, Parent: albumArtistID, Key: compactcatalog.AlbumKey(key), Title: albumTitle, Year: year}); err != nil {
				return err
			}
			if created {
				if err = compactcatalog.SetFactsTx(ctx, tx, album, map[string]any{"artist_id": albumArtistID, "local_key": key, "edition_key": audioEdition(f.Tags)}); err != nil {
					return err
				}
			}
		}
		var previous int64
		if e := tx.QueryRow(`SELECT album_id FROM catalog_songs WHERE entity_id=?`, item).Scan(&previous); e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if _, err = tx.ExecContext(ctx, `UPDATE catalog_entities SET parent_id=? WHERE id=? AND parent_id IS NOT ?`, album, item, album); err != nil {
			return err
		}
		if err = compactcatalog.SetFactsTx(ctx, tx, item, map[string]any{"album_id": album, "disc_number": disc, "track_number": track}); err != nil {
			return err
		}
		names := audioNames(f.Tags, "artists", "artist")
		if len(names) == 0 {
			names = []string{performer}
		}
		credited := make([]int64, 0, len(names))
		for _, name := range names {
			artistID, e := s.artist(ctx, tx, handle, name)
			if e != nil {
				return e
			}
			credited = append(credited, artistID)
		}
		if err = compactcatalog.SetSongArtistsTx(ctx, tx, item, credited); err != nil {
			return err
		}
		// Artist sidecars resolve to the album artist; a compilation's folder
		// art belongs to the release, never to "Various Artists".
		if carriers := artistSidecarCarriers(albumArtistID, albumArtist, f.ArtworkExtra); len(carriers) > 0 {
			if everr := commitSidecars(ctx, tx, library, carriers); everr != nil {
				return everr
			}
		}
		// The song (and its artist credits above) has left `previous` by now,
		// so an emptied album's artist is retireable in the same transaction.
		if previous != 0 && previous != album {
			if err = retireEmptiedAlbumTx(ctx, tx, previous); err != nil {
				return err
			}
		}
		return nil
	}
	author := firstTag(f.Tags, "author", "artist")
	narrator := firstTag(f.Tags, "narrator")
	var previousBook int64
	if e := tx.QueryRow(`SELECT book_id FROM catalog_book_files WHERE entity_id=?`, item).Scan(&previousBook); e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	var book int64
	if firstTag(f.Tags, "album") == "" || author == "" || strings.HasPrefix(f.LocalMetadataIssue, "invalid_") {
		if e := tx.QueryRow(`SELECT book_id FROM catalog_book_files WHERE entity_id=?`, item).Scan(&book); e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
	}
	if book == 0 {
		e := tx.QueryRow(`SELECT x.book_id FROM catalog_book_files x JOIN catalog_entities b ON b.id=x.book_id JOIN catalog_books d ON d.entity_id=b.id WHERE x.entity_id=? AND b.title=? AND d.author=? AND d.narrator=?`, item, albumTitle, author, narrator).Scan(&book)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
	}
	if book != 0 && firstTag(f.Tags, "isbn", "asin", "edition") != "" {
		var raw string
		if e := tx.QueryRow(`SELECT local_metadata_payload FROM catalog_books WHERE entity_id=?`, book).Scan(&raw); e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if raw != "" {
			var prior LocalBookMetadata
			if json.Unmarshal([]byte(raw), &prior) != nil {
				return errors.New("invalid local book metadata")
			}
			if f.Tags["edition"] != "" && prior.Edition != "" && f.Tags["edition"] != prior.Edition {
				book = 0
			}
			for _, i := range prior.Identifiers {
				if v := f.Tags[i.Scheme]; v != "" && v != i.Value {
					book = 0
				}
			}
		}
	}
	if book == 0 {
		key := localKey(folder + "|" + albumTitle + "|" + author + "|" + narrator + "|" + f.Tags["isbn"] + "|" + f.Tags["asin"] + "|" + f.Tags["edition"])
		var created bool
		if book, created, err = compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: handle, Kind: compactcatalog.Book, Key: compactcatalog.BookKey(key), Title: albumTitle}); err != nil {
			return err
		}
		if created {
			if err = compactcatalog.SetFactsTx(ctx, tx, book, map[string]any{"library_id": handle, "local_key": key, "author": author, "narrator": narrator}); err != nil {
				return err
			}
		}
	}
	if author != "" {
		// Authors are browse entities of their own, alive while a live book names them.
		if _, _, err = compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: handle, Kind: compactcatalog.Author, Key: compactcatalog.AuthorKey(author), Title: author}); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE catalog_entities SET parent_id=? WHERE id=? AND parent_id IS NOT ?`, book, item, book); err != nil {
		return err
	}
	if err = compactcatalog.SetFactsTx(ctx, tx, item, map[string]any{"book_id": book, "disc_number": disc, "part_number": track}); err != nil {
		return err
	}
	if err = saveLocalBookMetadata(tx, book, f.Tags, f.TagSources); err != nil {
		return err
	}
	// A file whose book identity changed (its tags arrived after a first scan
	// without them, or were edited) has left `previousBook`; an emptied book goes.
	if previousBook != 0 && previousBook != book {
		if err = settleEmptiedBook(ctx, tx, previousBook); err != nil {
			return err
		}
	}
	chapters := make([]compactcatalog.Chapter, 0, len(f.Chapters))
	for index, c := range f.Chapters {
		chapterTitle := c.Title
		if chapterTitle == "" {
			chapterTitle = "Chapter " + strconv.Itoa(index+1)
		}
		chapters = append(chapters, compactcatalog.Chapter{Title: chapterTitle, Start: c.Start, End: c.End})
	}
	return compactcatalog.SetBookChaptersTx(ctx, tx, item, chapters)
}
func intPointer(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	value := int(n.Int64)
	return &value
}
func (s *Service) songInfo(item string) (*SongInfo, error) {
	var info SongInfo
	var disc, track sql.NullInt64
	err := s.read().QueryRow(`SELECT pid(album.public_id),album.title,artist.title,
 COALESCE((SELECT group_concat(title,'; ') FROM (SELECT performer.title FROM catalog_song_artists sa JOIN catalog_entities performer ON performer.id=sa.artist_id WHERE sa.song_id=s.entity_id ORDER BY performer.title)),''),
 s.disc_number,s.track_number,COALESCE((SELECT status FROM mb_jobs WHERE kind='song' AND entity_id=item.id),'unmatched'),
 COALESCE((SELECT issue.issue FROM catalog_asset_links link JOIN catalog_asset_issues issue ON issue.asset_id=link.asset_id AND issue.library_id=item.library_id WHERE link.entity_id=item.id LIMIT 1),''),
 s.recording_id,s.release_id,s.release_group_id,s.track_id,s.release_status,s.provider_observed_at,s.provider_title,s.provider_artist
 FROM catalog_entities item JOIN catalog_songs s ON s.entity_id=item.id
 JOIN catalog_entities album ON album.id=s.album_id JOIN catalog_albums a ON a.entity_id=album.id
 JOIN catalog_entities artist ON artist.id=a.artist_id
 WHERE item.public_id=pid_blob(?) AND item.kind=7 AND album.retired=0
 AND EXISTS(SELECT 1 FROM catalog_song_artists sa WHERE sa.song_id=item.id)`, item).Scan(
		&info.AlbumID, &info.AlbumTitle, &info.AlbumArtist, &info.Artist, &disc, &track,
		&info.ProviderMatchStatus, &info.LocalMetadataIssue, &info.ProviderRecordingID,
		&info.ProviderReleaseID, &info.ProviderReleaseGroupID, &info.ProviderTrackID,
		&info.ProviderReleaseStatus, &info.ProviderObservedAt, &info.ProviderTitle,
		&info.ProviderArtist)
	if info.ProviderRecordingID != "" {
		info.ProviderSourceURL = "https://musicbrainz.org/recording/" + info.ProviderRecordingID
	}
	info.DiscNumber = intPointer(disc)
	info.TrackNumber = intPointer(track)
	return &info, err
}
func (s *Service) bookFileInfo(item string) (*BookFileInfo, error) {
	var info BookFileInfo
	var disc, part sql.NullInt64
	var payload, library string
	err := s.read().QueryRow(`SELECT pid(b.public_id),b.title,f.disc_number,f.part_number,
 COALESCE((SELECT issue.issue FROM catalog_asset_links link JOIN catalog_asset_issues issue ON issue.asset_id=link.asset_id AND issue.library_id=item.library_id WHERE link.entity_id=item.id LIMIT 1),''),
 l.library_id,d.local_metadata_payload
 FROM catalog_entities item JOIN catalog_book_files f ON f.entity_id=item.id
 JOIN catalog_entities b ON b.id=f.book_id JOIN catalog_books d ON d.entity_id=b.id
 JOIN catalog_libraries l ON l.id=b.library_id
 WHERE item.public_id=pid_blob(?) AND item.kind=9 AND b.retired=0`, item).Scan(&info.BookID, &info.BookTitle, &disc, &part, &info.LocalMetadataIssue, &library, &payload)
	info.DiscNumber = intPointer(disc)
	info.PartNumber = intPointer(part)
	info.SourceBoundary = "whole_source"
	if err == nil {
		var policy LocalAudioPolicy
		e := s.read().QueryRow(`SELECT revision,local_mode FROM audio_metadata_policies WHERE library_id=?`, library).Scan(&policy.Revision, &policy.LocalMode)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return &info, e
		}
		if e == nil {
			info.LocalPolicy = &policy
		}
		if e == nil && policy.LocalMode != "off" && payload != "" {
			var local LocalBookMetadata
			if e = json.Unmarshal([]byte(payload), &local); e != nil {
				return &info, e
			}
			info.LocalMetadata = &local
		}
	}
	return &info, err
}

// normalizeAudioGenres splits the bounded delimiters a tagger may use for one
// genre field, trims, and keeps the first sixteen names under sixty-four
// characters, deduplicated by normalized text. The normalized text is the
// facet identity, so "Science Fiction" and "science  fiction" are one facet.
func normalizeAudioGenres(raw string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == '/' }) {
		name := strings.TrimSpace(part)
		if name == "" || len(name) > 64 {
			continue
		}
		id := localKey(name)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, name)
		if len(out) >= 16 {
			break
		}
	}
	return out
}
