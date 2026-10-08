package metadata

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"portico.local/server/internal/worker"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/metadataprovider"
	"portico.local/server/internal/supervise"
)

const Attribution = "This product uses the TMDB API but is not endorsed or certified by TMDB."

type Movie struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	ReleaseDate   string `json:"release_date"`
	Overview      string `json:"overview"`
	PosterPath    string `json:"poster_path"`
	BackdropPath  string `json:"backdrop_path"`
}
type Service struct {
	Admission func(context.Context, *sql.Tx, string) (bool, error)
	// musicArtistCursor is where the artist enrichment walk resumes.
	musicArtistCursor atomic.Int64
	now               func() time.Time // private deterministic lease clock; production uses UTC wall time

	screenProviders map[string]ScreenProvider
	// tmdbPaced is the one door to TMDB: every request, from any worker, shares
	// its pacing clock, in-flight gate, Retry-After deferral and short cache.
	tmdbPaced *metadataprovider.TMDB
	// anilistPaced is the same one-door policy for AniList trend and anime
	// evidence; the screen matcher and the discovery worker share its transport.
	anilistPaced    *metadataprovider.AniList
	trendingSources map[string]trendingSource
	mb              MusicBrainzProvider
	tvdb            TVDBProvider
	// The artist chain's sources (Spec — Page Content §8): MusicBrainz artist
	// details plus Wikidata, Wikipedia and Commons. One transport each, so
	// each keeps the standard pacing. Tests substitute bounded fakes.
	wikidata       wikidataArtistSource
	wikipedia      wikipediaSource
	commons        commonsSource
	LocalArtwork   func(string) (*os.File, string, error)
	db             *sql.DB
	token          string
	client         *http.Client
	base           string
	cacheRoot      string
	artDirMu       sync.Mutex
	missingMu      sync.Mutex
	missingArtwork map[missingArtwork]struct{}
	missingWake    worker.Broadcast
	acoustid       AcousticProvider
	acoustidConfig string
	artClient      *http.Client
	artDirectory   *os.File
}

func New(db *sql.DB, token string) *Service {
	if token == "" {
		token = ProjectToken
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.MaxConnsPerHost = 2
	transport.ResponseHeaderTimeout = 10 * time.Second
	s := &Service{db: db, token: token, client: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, base: "https://api.themoviedb.org/3"}
	s.tvdb, _ = metadataprovider.NewTVDB(TVDBProjectKey)
	s.mb = metadataprovider.NewMusicBrainz()
	s.wikidata = metadataprovider.NewWikidata()
	s.wikipedia = metadataprovider.NewWikipedia()
	s.commons = metadataprovider.NewCommons()
	tmdb, _ := metadataprovider.NewTMDB(token)
	anilist := metadataprovider.NewAniList()
	s.tmdbPaced = tmdb
	s.anilistPaced = anilist
	s.screenProviders = map[string]ScreenProvider{"tmdb": tmdb, "anilist": anilist}
	// Trend feeds are keyed by the persisted discovery (provider, media_kind)
	// pair. Every entry goes through the same paced adapters as item matching;
	// tests substitute bounded fakes for a key, never a second client.
	s.trendingSources = map[string]trendingSource{
		"tmdb/movie":    tmdbTrendingSource{tmdb, "movie"},
		"tmdb/tv":       tmdbTrendingSource{tmdb, "tv"},
		"anilist/anime": anilist,
	}
	if tvdb, ok := s.tvdb.(ScreenProvider); ok {
		s.screenProviders["tvdb"] = tvdb
	}
	s.artClient = newArtworkClient()
	return s
}
func imageURL(path, size string) string {
	if path == "" || !strings.HasPrefix(path, "/") || strings.Contains(path, "..") || strings.ContainsAny(path, "?#\\") {
		return ""
	}
	return "https://image.tmdb.org/t/p/" + size + path
}
func (s *Service) Run(ctx context.Context) {
	ctx = dbwork.WithClass(ctx, dbwork.ClassBackgroundMedia)
	s.logProviderState(ctx)
	// Woken by any committed write rather than asking once a second whether
	// anything changed. On a server with nothing queued this runs at the safety
	// tick and no more; on a busy one it hears about work as it is enqueued,
	// which is sooner than the timer it replaces.
	wake := worker.NewSignal()
	unregister := dbwork.WakeOnTables(wake, "metadata_*", "screen_metadata_*", "mb_*", "tvdb_*", "audio_metadata_*", "audio_genre_*", "maintenance_*", "console_documents")
	defer unregister()
	progress := worker.NewProgress(dbwork.ChangingCommits)
	worker.Run(ctx, "metadata.providers", wake, func(ctx context.Context) time.Duration {
		if s.Admission != nil {
			allowed, e := s.Admission(ctx, nil, "metadata-refresh")
			if e != nil || !allowed {
				return time.Minute
			}
		}
		// Provider refresh remains a bounded background batch under load.
		progress.Reset()
		if !dbwork.Yield(ctx) {
			return 0
		}
		_ = s.ScreenStep(ctx)
		if !dbwork.Yield(ctx) {
			return 0
		}
		_ = s.screenTVDBRun(ctx)
		if !dbwork.Yield(ctx) {
			return 0
		}
		_ = s.MusicBrainzStep(ctx)
		if !dbwork.Yield(ctx) {
			return 0
		}
		// Local audio genre catch-up: projects pre-upgrade catalogs and policy
		// revisions in durable batches through the scanner's shared projection.
		// Reads nothing and writes nothing when every audio library is current.
		_ = s.AudioGenreStep(ctx)
		if !dbwork.Yield(ctx) {
			return 0
		}
		// Provider Trending Now: one bounded feed pass per wake. The step itself
		// reads a few rows and returns without writing when nothing is due, so
		// an idle server keeps its wake floor and never loops on itself.
		_ = s.DiscoveryStep(ctx)
		if progress.Moved() {
			// Something moved, so there is probably more behind it.
			return time.Second
		}
		return 0
	})
}

// Separate bounded lanes keep artwork and local repair independent of metadata
// providers that are slow or offline. Shutdown cancels I/O and releases handles.
func (s *Service) RunArtwork(ctx context.Context) {
	ctx = dbwork.WithClass(ctx, dbwork.ClassBackgroundMedia)
	var wg sync.WaitGroup
	for _, step := range []func(context.Context) error{s.RepairMissingArtworkStep, s.ArtworkDiscoveryStep, s.ArtworkStep, s.CompactArtworkStep, s.RepairCascadeStep} {
		wg.Add(1)
		supervise.Go("metadata.artwork-lane", func() {
			run := step
			defer wg.Done()
			urgent, unsubscribe := s.missingWake.Subscribe()
			defer unsubscribe()
			wake := worker.NewSignal()
			unregister := dbwork.WakeOnTables(wake, "artwork_*", "metadata_owner_fields", "metadata_relationship_decisions", "screen_metadata_publications", "metadata_publication_heads", "maintenance_*", "console_documents")
			defer unregister()
			progress := worker.NewProgress(dbwork.ChangingCommits)
			worker.RunWith(ctx, "metadata.artwork-lane", urgent, wake, func(ctx context.Context) time.Duration {
				if s.Admission != nil {
					allowed, e := s.Admission(ctx, nil, "metadata-refresh")
					if e != nil || !allowed {
						return time.Minute
					}
				}
				progress.Reset()
				if !dbwork.Yield(ctx) {
					return 0
				}
				_ = run(ctx)
				if progress.Moved() {
					return time.Second
				}
				return 0
			})
		})
	}
	wg.Wait()
	s.artDirMu.Lock()
	defer s.artDirMu.Unlock()
	if s.artDirectory != nil {
		s.artDirectory.Close()
		s.artDirectory = nil
	}
}

// tmdbGet is how the catalogue paths reach TMDB. A provider refusal comes back
// as a *metadataprovider.Error, so a 429's Retry-After reaches the work row's
// backoff and the provider cooldown instead of being flattened into a string.
func (s *Service) tmdbGet(ctx context.Context, route string) ([]byte, error) {
	if s.tmdbPaced == nil {
		return nil, errors.New("metadata provider unavailable")
	}
	base := ""
	if s.base != "https://api.themoviedb.org/3" {
		base = s.base
	}
	return s.tmdbPaced.Raw(ctx, s.client, base, route, 1<<20)
}
