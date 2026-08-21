package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestProviderRichFixtureMatrix(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		mapFixture  func([]byte) metadataProviderRichProposal
		wantKinds   []string
		wantFields  []string
		unsupported []string
	}{
		{
			name:    "tmdb movie",
			fixture: `{"id":42,"original_title":"Le Portique","original_language":"fr","spoken_languages":[{"iso_639_1":"fr","name":"Français"}],"production_countries":[{"iso_3166_1":"CA","name":"Canada"}],"production_companies":[{"id":7,"name":"Northlight"}],"keywords":{"keywords":[{"id":9,"name":"harbour"}]},"belongs_to_collection":{"id":10,"name":"Drift Cycle"},"release_dates":{"results":[{"iso_3166_1":"CA","release_dates":[{"certification":"14A","type":3}]}]},"external_ids":{"imdb_id":"tt123"},"alternative_titles":{"titles":[{"iso_3166_1":"CA","title":"The Portico"}]},"status":"Released","runtime":121,"images":{"posters":[{"file_path":"/p.jpg","width":1000,"height":1500,"vote_count":3}],"logos":[{"file_path":"/l.svg"}]},"budget":500}`,
			mapFixture: func(raw []byte) metadataProviderRichProposal {
				var value tmdbSearchResult
				mustProviderFixtureJSON(t, raw, &value)
				return mapTMDBProviderRich(value)
			},
			wantKinds:   []string{"spokenLanguage", "country", "company", "keyword", "collection", "contentRating", "externalID"},
			wantFields:  []string{"originalTitle", "originalLanguage", "alternateTitle", "status", "runtimeMinutes"},
			unsupported: []string{"budget"},
		},
		{
			name:    "anilist anime",
			fixture: `{"id":100,"idMal":200,"title":{"romaji":"Kōro","english":"Passage","native":"航路"},"format":"TV","status":"FINISHED","episodes":12,"duration":24,"season":"SPRING","seasonYear":2025,"genres":["Drama"],"tags":[{"name":"Ships","rank":88}],"studios":{"nodes":[{"id":3,"name":"Blue Studio"}]},"staff":{"edges":[{"role":"Director","node":{"id":4,"name":{"full":"A. Mori"}}}]},"characters":{"edges":[{"role":"MAIN","node":{"id":5,"name":{"full":"Nami"}},"voiceActors":[{"id":6,"name":{"full":"K. Ito"}}]}]},"popularity":9999}`,
			mapFixture: func(raw []byte) metadataProviderRichProposal {
				var value aniListMedia
				mustProviderFixtureJSON(t, raw, &value)
				return mapAniListProviderRich(value)
			},
			wantKinds:   []string{"externalID", "genre", "tag", "studio", "person", "character"},
			wantFields:  []string{"title", "format", "status", "episodes", "durationMinutes", "season"},
			unsupported: []string{"popularity"},
		},
		{
			name:    "musicbrainz recording",
			fixture: `{"id":"rec-1","title":"Harbour Light","length":180000,"isrcs":["CAABC2500001"],"aliases":[{"name":"Harbor Light","locale":"en"}],"artist-credit":[{"name":"Mara","artist":{"id":"artist-1","name":"Mara","country":"CA"}}],"releases":[{"id":"release-1","title":"Tides","country":"CA","status":"Official","barcode":"123","label-info":[{"catalog-number":"NL-1","label":{"id":"label-1","name":"Northlight"}}],"media":[{"format":"Digital Media","position":1,"track-count":1,"tracks":[{"id":"track-1","title":"Harbour Light","number":"1","position":1,"length":180000}]}]}],"video":false}`,
			mapFixture: func(raw []byte) metadataProviderRichProposal {
				var value musicBrainzRecording
				mustProviderFixtureJSON(t, raw, &value)
				return mapMusicBrainzRecordingProviderRich(value)
			},
			wantKinds:   []string{"externalID", "person", "release", "label", "medium", "track"},
			wantFields:  []string{"title", "durationMilliseconds", "alias"},
			unsupported: []string{"video"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proposal := test.mapFixture([]byte(test.fixture))
			if proposal.MappingVersion != metadataProviderRichMappingVersion || len(proposal.Snapshot) == 0 || len(proposal.SnapshotHash) != 64 {
				t.Fatalf("invalid proposal envelope: %#v", proposal)
			}
			for _, kind := range test.wantKinds {
				if !richHasKind(proposal, kind) {
					t.Errorf("missing relationship kind %q", kind)
				}
			}
			for _, field := range test.wantFields {
				if !richHasField(proposal, field) {
					t.Errorf("missing value field %q", field)
				}
			}
			mapped, _ := json.Marshal(struct {
				Values        []metadataProviderValueProposal `json:"values"`
				Relationships []metadataRelationshipProposal  `json:"relationships"`
			}{proposal.Values, proposal.Relationships})
			for _, field := range test.unsupported {
				if strings.Contains(string(mapped), field) {
					t.Errorf("intentionally unsupported field %q was mapped", field)
				}
			}
		})
	}
}

func TestMusicBrainzRichMappingPreservesReleaseAndWorkSemantics(t *testing.T) {
	var recording musicBrainzRecording
	mustProviderFixtureJSON(t, []byte(`{"id":"rec","title":"Suite","genres":[{"name":"Classical","count":8}],"tags":[{"name":"piano","count":4}],"artist-credit":[{"name":"A","joinphrase":" feat. ","artist":{"id":"a","name":"Artist"}}],"relations":[{"target-type":"work","type":"performance","direction":"forward","work":{"id":"w","title":"Suite, Op. 1","type":"Song","language":"eng","languages":["eng"],"iswcs":["T-1"]}}],"releases":[{"id":"rel","title":"Album","status":"Official","country":"CA","barcode":"123","packaging":"Digipak","text-representation":{"language":"eng","script":"Latn"},"media":[{"position":1,"format":"CD","track-offset":2,"text-representation":{"language":"eng","script":"Latn"},"tracks":[{"id":"trk","number":"3","position":1,"title":"Suite","recording":{"id":"rec","title":"Suite","isrcs":["CAAAA0100001"]}}]}]}]}`), &recording)
	p := mapMusicBrainzRecordingProviderRich(recording)
	for _, kind := range []string{"genre", "tag", "work", "release", "medium", "track"} {
		if !richHasKind(p, kind) {
			t.Errorf("missing %s", kind)
		}
	}
	encoded, _ := json.Marshal(p.Relationships)
	for _, want := range []string{"joinPhrase", "packaging", "Digipak", "musicbrainz-recording", "CAAAA0100001", "iswcs"} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("missing %q in %s", want, encoded)
		}
	}
}

func TestCoverArtArchiveDiscoveryIsBoundedAndDegrades(t *testing.T) {
	serverHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"images":[{"id":1,"image":"https://img/front.jpg","front":true,"approved":true,"types":["Front"]}]}`))
	}))
	defer serverHTTP.Close()
	_, _, server := newDiscoveryTestServer(t, config.Config{CoverArtArchiveBaseURL: serverHTTP.URL})
	p, ok := server.getCoverArtArchive(context.Background(), "release", "r1")
	if !ok || len(p.Images) != 1 || p.Images[0].Kind != "poster" {
		t.Fatalf("proposal = %+v, ok=%v", p, ok)
	}
	if _, ok := server.getCoverArtArchive(context.Background(), "artist", "a1"); ok {
		t.Fatal("unsupported CAA entity was fetched")
	}
}

func TestProviderRichSnapshotIsDeterministicSanitizedAndBounded(t *testing.T) {
	value := tmdbSearchResult{ID: 7, OriginalTitle: "  title\x00  ", Overview: strings.Repeat("x", metadataProviderSnapshotMaxBytes*2)}
	one, two := mapTMDBProviderRich(value), mapTMDBProviderRich(value)
	if one.SnapshotHash != two.SnapshotHash {
		t.Fatal("snapshot hash is not deterministic")
	}
	if len(one.Snapshot) > metadataProviderSnapshotMaxBytes {
		t.Fatalf("snapshot bound = %d, cut=%v", len(one.Snapshot), one.SnapshotCut)
	}
	if strings.Contains(string(one.Snapshot), "\x00") {
		t.Fatal("snapshot retained a control character")
	}
	stored := sha256.Sum256(one.Snapshot)
	if one.SnapshotHash != hex.EncodeToString(stored[:]) || one.SourceHash != one.SnapshotHash || one.SourceBytes != len(one.Snapshot) {
		t.Fatalf("stored snapshot digest contract is inconsistent: %+v", one)
	}
	hugeValues := make([]string, 512)
	for index := range hugeValues {
		hugeValues[index] = strings.Repeat("é", 8192)
	}
	huge := newProviderRichProposal("tmdb", hugeValues)
	hugeStored := sha256.Sum256(huge.Snapshot)
	if huge.SnapshotHash != hex.EncodeToString(hugeStored[:]) || !huge.SnapshotCut || huge.SourceHash == huge.SnapshotHash || huge.SourceBytes <= len(huge.Snapshot) {
		t.Fatalf("truncated snapshot digest contract is inconsistent: %+v", huge)
	}
	unicode := mapTMDBProviderRich(tmdbSearchResult{ID: 8, Overview: strings.Repeat("é", 9000)})
	if !json.Valid(unicode.Snapshot) {
		t.Fatal("UTF-8 snapshot truncation produced invalid JSON")
	}
}

func TestTMDBReleaseDatesCollectionArtworkAndImageLanguage(t *testing.T) {
	p := mapTMDBProviderRich(tmdbSearchResult{
		ReleaseDate: "2026-08-05", FirstAirDate: "2025-11-09",
		BelongsToCollection: &tmdbCollection{ID: 7, Name: "Cycle", PosterPath: "/collection-p.jpg", BackdropPath: "/collection-b.jpg"},
		ReleaseDates:        tmdbReleaseDates{Results: []tmdbReleaseDateCountry{{ISO31661: "CA", ReleaseDates: []tmdbReleaseDate{{ReleaseDate: "2026-08-05T00:00:00.000Z", Type: 3}}}}},
	})
	for _, field := range []string{"releaseDate", "firstAirDate"} {
		if !richHasField(p, field) {
			t.Errorf("missing exact %s", field)
		}
	}
	for _, kind := range []string{"collectionPoster", "collectionBackdrop"} {
		if !richHasImageKind(p, kind) {
			t.Errorf("missing %s", kind)
		}
	}
	if !richHasKind(p, "release") {
		t.Error("missing uncertified regional release date")
	}
	if got := tmdbImageLanguage("pt-BR"); got != "pt" {
		t.Fatalf("image language = %q", got)
	}
}

func TestAniListReleaseRelevantRichnessIsBoundedAndDoesNotInventCertification(t *testing.T) {
	var media aniListMedia
	mustProviderFixtureJSON(t, []byte(`{"id":1,"idMal":2,"isAdult":true,"startDate":{"year":2025,"month":4,"day":7},"endDate":{"year":2025,"month":6,"day":23},"source":"MANGA","countryOfOrigin":"JP","synonyms":["Passage"],"coverImage":{"extraLarge":"https://img/cover.jpg","color":"#12abef"},"staff":{"pageInfo":{"total":80,"perPage":25,"currentPage":1,"lastPage":4,"hasNextPage":true},"edges":[{"role":"Director","node":{"id":3,"name":{"full":"Director"},"image":{"large":"https://img/staff.jpg"}}}]},"characters":{"pageInfo":{"total":50,"hasNextPage":true},"edges":[{"role":"MAIN","node":{"id":4,"name":{"full":"Hero"},"image":{"large":"https://img/hero.jpg"}},"voiceActors":[{"id":5,"name":{"full":"Actor"},"image":{"large":"https://img/actor.jpg"}}]}]},"relations":{"pageInfo":{"total":1},"edges":[{"relationType":"SEQUEL","node":{"id":6,"idMal":7,"type":"ANIME","format":"TV","title":{"english":"Next"}}}]}}`), &media)
	p := mapAniListProviderRich(media)
	for _, field := range []string{"startDate", "endDate", "source", "countryOfOrigin", "alternateTitle", "dominantColor"} {
		if !richHasField(p, field) {
			t.Errorf("missing %s", field)
		}
	}
	for _, kind := range []string{"relatedMedia", "providerCoverage"} {
		if !richHasKind(p, kind) {
			t.Errorf("missing %s", kind)
		}
	}
	for _, kind := range []string{"personPortrait", "characterPortrait"} {
		if !richHasImageKind(p, kind) {
			t.Errorf("missing %s", kind)
		}
	}
	update := aniListUpdateForResult(media)
	if update.ContentRating != nil {
		t.Fatalf("isAdult invented certification %q", *update.ContentRating)
	}
}

func mustProviderFixtureJSON(t *testing.T, raw []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatal(err)
	}
}
func richHasKind(p metadataProviderRichProposal, kind string) bool {
	for _, relationship := range p.Relationships {
		if relationship.Kind == kind {
			return true
		}
	}
	return false
}
func richHasField(p metadataProviderRichProposal, field string) bool {
	for _, value := range p.Values {
		if value.Field == field {
			return true
		}
	}
	return false
}
func richHasImageKind(p metadataProviderRichProposal, kind string) bool {
	for _, image := range p.Images {
		if image.Kind == kind {
			return true
		}
	}
	return false
}
