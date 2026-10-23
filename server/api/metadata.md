# Metadata representations

See [metadata editing](metadata-editor.md) for owner changes and
[artwork and ordering](guide/03-browsing.md#artwork-and-ordering) for published
image URLs, cache validation and sort keys.

## Global metadata settings

The owner reads and saves global choices at `GET` and `PUT
/v1/admin/metadata/settings`. Saves compare `expectedRevision`; this revision
also changes when online lookup consent changes through a library's metadata
page. Keep the current `localArtworkLimitBytes` and `includeAdult` when updating
another choice. New optional fields are unchanged when omitted.

`onlineLookups` inhibits provider matching and artwork acquisition while keeping
each library's own provider permissions. Local metadata, owner edits, existing
published details and cached pictures remain usable. Turning it on never changes
a local-only library into an online library or enables a disabled provider.

`defaultLanguage` and `defaultRegion` seed newly created screen libraries.
Existing libraries retain their values; an explicit language chosen when adding
a library takes precedence. Artwork choices are `artworkPreference` (`folders`
or `online`), `artworkLanguage` (`same`, `en` or `textless`), `artworkMaxEdge`
(1280, 1920 or 2560) and `castPhotos`. Automatic selection honors these choices;
owner-selected and locked artwork wins. Images are never enlarged. A saved
artwork choice reseeds existing automatic choices in bounded background batches.

`refreshPolicy` is `regular`, `recent` (new and airing titles) or `never` (only
when requested). Initial lookups remain immediate. `fallbackLanguage` chooses
`en`, `original` or `blank` for missing provider descriptions, on the same
provider. It does not replace local or owner-written facts. `animeTitles`
chooses `language`, `english`, `romaji` or `native` from AniList's own title
variants.

Provider credentials are write-only. `acoustidKey` follows the existing TMDB,
TVDB and OpenSubtitles overrides: omission keeps it; an empty value restores the
server's startup credential, if configured. Read responses contain presence flags
only. AcoustID still requires the library's existing fingerprint permission and
opt-in. No setting creates a connection to Portico Services.

`POST /v1/admin/metadata/refresh` takes `expectedRevision` and queues a durable
refresh sweep. `GET` on the same path reports `queueing`, `queued`,
`requestedAt` and `enqueuedAt`. These report enqueue progress, not provider
completion. The sweep preserves selected identities, edits and locks, skips
providers that are disabled, and observes the durable hourly/daily refresh
allowance. Provider failures keep the current published details available.
