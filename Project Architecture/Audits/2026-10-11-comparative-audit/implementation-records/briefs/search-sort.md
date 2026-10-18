# Lane: search-sort — the details that are frozen at first release

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-search-sort` (branch `audit-implementation/search-sort`). This is your working directory and, with `/home/justin/projects/Portico/.worktrees/audit-implementation/_program`, the only place you can write.
**You cannot commit** (the sandbox keeps git metadata read-only). At each milestone, when the tree compiles, run from the worktree root:
`bash /home/justin/projects/Portico/.worktrees/audit-implementation/_program/snap.sh search-sort "<one-line commit subject in plain language>"`
Snapshots are cumulative; each becomes one commit. Take one at least every hour of work and always one at the very end, after which you must not change the tree. Go environment: `GOCACHE` and `GOFLAGS=-p=2` are already set. Your report is `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/search-sort.md`.
A rule added since the first lanes: every route on the registry must declare the key scope it needs (`Requires: apikit.ScopeRead | ScopePlayback | ScopeLibrary | ScopeAdmin | ScopeSession | ScopeOpen`; see `apikit/registry.go`), or the server will not start. After changing routes run `go run ./cmd/apigen` from `server/` to regenerate the contract files; after changing the schema run `python3 scripts/schema-sync.py` from the root.

Audit documents: 07 (B-07.6, B-07.7, F-07.5, F-07.10, F-07.15), 08 (A-08.41), 12 (F-12.10), 18 (B-18.4); verification V04, G1. Decision register D-09. Justin accepted the recommendations: these are cheap now and a rebuild or migration after release.

## What to deliver
1. **Sort keys**: natural number order (`Rocky 2` before `Rocky 10`), leading articles handled per language as today, leading punctuation and symbols skipped (`'Til Death`, `(500) Days of Summer`, `¡Three Amigos!`), diacritics folded for ordering, jump-to-letter still exact.
2. **Search scope**: match original titles and alternative titles, people (cast, directors, artists, authors, narrators), years, and collections; punctuation- and diacritic-insensitive (`spiderman` finds `Spider-Man`, `wall e` finds `WALL·E`); sensible behaviour for Chinese, Japanese and Korean text (substring matching, since there are no spaces); prefix matching as the person types; results ranked exact title first.
3. **One time and duration convention** across the API (A-08.41): pick one (the dominant one already in use), list every route that deviates, and bring them into line together with their `client-core` parsers.
4. **Rating scale**: personal ratings stored and exchanged as integers 0–10 (shown as half-stars by clients); bring any other form into line.
5. **Lists that are capped instead of paged**: the Genres and Series tabs stop at 200 (B-18.4); give them proper paging on the server so clients can window them.
6. Bump the affected derivation versions so existing data is rebuilt.

## Boundaries
- You own `sorttext`, `facettext`, the search-document and sort-key derivations' *content*, and the list routes in item 5. A **freshness-server** lane running now owns *how* a rebuild is carried out (generations, workers, counters, events): do not change that machinery, only what is computed.
- A **scan-loop** lane and a **personal-state** lane are also running; leave scanning and personal state alone.
- Pre-release: change the stored form directly; no compatibility with old data.
