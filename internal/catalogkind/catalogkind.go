// Package catalogkind defines the boundary between Portico's internal media
// storage types and the stable entity kinds exposed by product contracts.
package catalogkind

import "strings"

type EntityKind string
type StorageKind string

// Storage-only spellings are deliberately separate from EntityKind even when
// their underlying strings happen to match.
const (
	StorageAnime       StorageKind = "anime"
	StorageAudiobook   StorageKind = "audiobook"
	StorageLiveChannel StorageKind = "live_channel"
	StorageLiveProgram StorageKind = "live_program"
)

const (
	Movie           EntityKind = "movie"
	Show            EntityKind = "show"
	Season          EntityKind = "season"
	Episode         EntityKind = "episode"
	Special         EntityKind = "special"
	Artist          EntityKind = "artist"
	Album           EntityKind = "album"
	Track           EntityKind = "track"
	Author          EntityKind = "author"
	AudiobookSeries EntityKind = "audiobook-series"
	Book            EntityKind = "book"
	Chapter         EntityKind = "chapter"
	Recording       EntityKind = "recording"
	LiveChannel     EntityKind = "live-channel"
	LiveProgram     EntityKind = "live-program"
	Person          EntityKind = "person"
	Collection      EntityKind = "collection"
	Playlist        EntityKind = "playlist"
	Category        EntityKind = "category"
	Extra           EntityKind = "extra"
	Unsupported     EntityKind = "unsupported"
)

var publicKinds = []EntityKind{
	Movie, Show, Season, Episode, Special, Artist, Album, Track, Author,
	AudiobookSeries, Book, Chapter, Recording, LiveChannel, LiveProgram, Person,
	Collection, Playlist, Category, Extra, Unsupported,
}

var storageToPublic = map[StorageKind]EntityKind{
	"movie": Movie, "show": Show, "anime": Show, "season": Season,
	"episode": Episode, "special": Special, "artist": Artist, "album": Album,
	"track": Track, "author": Author, "audiobook_series": AudiobookSeries,
	"audiobook-series": AudiobookSeries, "audiobook": Book, "book": Book,
	"chapter": Chapter, "recording": Recording, "live_channel": LiveChannel,
	"live-channel": LiveChannel, "live_program": LiveProgram, "live-program": LiveProgram,
	"person": Person, "collection": Collection, "playlist": Playlist,
	"category": Category, "extra": Extra,
}

// Public returns the stable public entity kind for a storage type. Unknown and
// administrative values deliberately collapse to Unsupported.
func Public(storageType string) EntityKind {
	if kind, ok := storageToPublic[StorageKind(strings.ToLower(strings.TrimSpace(storageType)))]; ok {
		return kind
	}
	return Unsupported
}

func IsPublic(value string) bool {
	kind := EntityKind(strings.ToLower(strings.TrimSpace(value)))
	for _, candidate := range publicKinds {
		if candidate == kind {
			return true
		}
	}
	return false
}

func PublicKinds() []string {
	result := make([]string, len(publicKinds))
	for i, kind := range publicKinds {
		result[i] = string(kind)
	}
	return result
}

// ResultMappings returns all recognized storage/result spellings and their
// public equivalents, preserving the canonical declaration order.
func ResultMappings() map[string]string {
	result := make(map[string]string, len(storageToPublic))
	for storage, kind := range storageToPublic {
		result[string(storage)] = string(kind)
	}
	return result
}

// StorageTypes returns the internal row kinds represented by a public entity
// kind. Virtual entity kinds (for example author and audiobook-series) have no
// direct media_items row and therefore return no storage types.
func StorageTypes(publicKind string) []string {
	switch EntityKind(strings.ToLower(strings.TrimSpace(publicKind))) {
	case Movie:
		return []string{"movie"}
	case Show:
		return []string{"show", "anime"}
	case Season:
		return []string{"season"}
	case Episode, Special:
		return []string{"episode"}
	case Artist:
		return []string{"artist"}
	case Album:
		return []string{"album"}
	case Track:
		return []string{"track"}
	case Book:
		return []string{"audiobook"}
	case Recording:
		return []string{"recording"}
	case LiveChannel:
		return []string{"live_channel"}
	case LiveProgram:
		return []string{"live_program"}
	case Person:
		return []string{"person"}
	case Collection:
		return []string{"collection"}
	case Playlist:
		return []string{"playlist"}
	case Category:
		return []string{"category"}
	case Extra:
		return []string{"extra"}
	default:
		return nil
	}
}
