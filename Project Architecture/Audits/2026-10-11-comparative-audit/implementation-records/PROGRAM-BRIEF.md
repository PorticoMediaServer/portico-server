# Portico implementation programme — shared brief for every lane

You are one lane in a large, parallel implementation programme on Portico, a pre-release self-hosted media server (Go + SQLite server, React/TypeScript web app, shared TypeScript packages, React Native apps for iPhone, iPad, Apple TV, Android, Android TV and Fire TV, a Vega app, and a small Hosted service). The owner is Justin Ehler. He has read a comparative audit of Portico against Jellyfin and Plezy and made the decisions below. He is not available for questions. **Never ask a question and never stop to wait for an answer: decide, record the decision in your report, and carry on.**

Read this file fully, then your lane brief, before touching code.

## 1. Justin's decisions (authoritative)

**Product positions**
- Plex is the gold standard for library scanning and matching tolerance and for "it just plays". Jellyfin's lesson is openness, delivered through the API.
- **Never** plugins, themes or custom CSS. Curation is a feature. Openness is: every action available through the API, the first-party clients using the same API a third party would, easily generated API keys, outbound webhooks and an MCP server, all on the local server with no Hosted involvement.
- Users may make unsafe choices. Do not block or detect them; at most a quiet note. Push people to TLS where it makes sense, never force it.
- Good defaults the owner can override, but no wall of switches. Prefer one drop-down with named profiles where that fits, plain numbers where an owner knows the number. Settings that exist today and do nothing get **wired**, not removed (remove only what will never be used).
- Responsiveness wins: the web UI and owner console must stay reachable under any load.
- No push notifications, no email. Own code over new dependencies (agreed exceptions: mpv, libass, and a WebAssembly libass build on the web; FFmpeg is already used).
- A feature is done only when every first-party client has it. Where a platform truly cannot do it, hide it there.
- The server decides what a screen contains; clients render. Keep this. A bounded, safe query API for third parties is allowed as an addition, not the default.
- **Pre-release rule: there is no stable API and no compatibility to preserve.** If something needs to change, change it and change every consumer now. Delete legacy routes, duplicate documents and old paths; do not patch them or keep them alive. The API that exists on release day becomes v1; versioning starts then, not now.

**Library, identity and files**
- A title and its files are separate things (the Plex model). Files and versions come and go; the title and the history attached to it stay. A title goes only when all its copies are gone and its trash period has passed.
- "Trash" is not a folder. Deleting in Portico deletes the file on disk. A file that disappears from disk leaves its title in place, marked missing, so a replacement re-attaches to the same title. The owner chooses how often missing titles are emptied (immediately, on a schedule, never), as in Plex. When a whole library root is unreachable, nothing is marked missing. Watch state is kept by provider identity even after a title is removed, so re-adding it later restores it.
- Naming: both the folder and the file contribute. Where they conflict, the file (its name and embedded metadata) wins: `S02E01` inside `Season 1` is season 2. Season folders or none; `E01` or `01` inside `Season 2`; loose files. Minor misspellings and missing years still match when confidence is decent. Leave a title unmatched only when there is no confidence.
- Be tolerant of bad data: play, match and show what can be used. Reject the smallest unit (a row, a line, a file, a sub-folder), keep the rest, and tell the owner what was skipped. Keep every size, depth and time bound that protects against hostile input.
- Metadata provider is the owner's choice per library, TMDB the default for video libraries, with a local-metadata-only option.
- No import or export of watch state for now.
- **Owner administration on devices** (Justin, 14 Oct): phones and tablets get the owner administration panels (integrations, people, metadata and library tools, diagnostics, Live TV sources). Televisions do not, but a TV may show a read-only owner Dashboard: server status, active sessions and the like.
- **Vega redirects** (Justin, 14 Oct): the stated residual risk is accepted (Vega's SDK cannot refuse a redirect before following it; the app binds credentials to the exact origin, rejects redirected responses, signs out and attempts revocation).
- **Restore and API keys** (Justin, 14 Oct): a restore does not end API keys. Keys that did not exist at the backup's point in time simply do not exist afterwards.
- **Large totals** (Justin, 14 Oct): browse totals are exact up to 10,000 and shown as "10,000+" beyond that, so counting never grows with a huge matching set.
- **Sorting and filtering are the same everywhere** (Justin, 14 Oct): whichever part or sub-group of a library is being viewed (a genre, a series, a category, a collection, a person's titles, a tab), offer the same wide set of sort and filter options as the main library view. No grouping may be limited to title order.

**Playback**
- Platform players stay the primary engine. mpv is added as a second engine on iPhone, iPad, Apple TV, Android, Android TV and Fire TV. The player setting is Automatic (default), Native, or mpv. Automatic uses the native player when it can play the file directly or with a plain remux, and uses mpv only when the alternative is a video conversion, lossy audio, or burned-in subtitles.
- Conversions must be able to keep HEVC, HDR and multichannel audio, with tone mapping where the device has no HDR. Deep, accurate capability profiles for every player.
- One capability document. No second translation.
- Quality: the default is **Original**. Fixed lower qualities exist. **Automatic** is an option the viewer can pick, never the default; it starts at Original and moves down (and back up) as the connection requires. Quality can be set permanently in client settings and per session in the player.
- Subtitles: as many formats as possible without converting video. Styled (ASS) and image subtitles are rendered on the device wherever possible, otherwise burned in at high quality. The experience must be consistent across devices.

**API and freshness**
- Open screens stay current: the server pushes changes (within about 30 seconds), screens refresh quietly, the current screen is not blanked, and failures are not shown immediately.
- Browse and search must **always** answer. Never "being prepared". While background work runs, serve the previous content; work is chunked into frequent checkpoints so the delay is small.
- A person never sees a revision conflict unless the thing they acted on really changed. Split catch-all revision counters; clients re-read and retry silently.

**Running the server**
- Supported at release: Windows, macOS, Linux, Docker; reverse proxy on its own hostname; Tailscale and other overlays; IPv4 and IPv6. Carrier-grade NAT is not blocked. Sub-path hosting (`example.com/portico`) is out and will not be built. A relay is future work.
- Plain HTTP is accepted from any address. The owner's "home networks" setting is the single definition of local versus remote. No detection of Tailscale or proxies. Owner-supplied certificates are supported. Hosted-advertised routes stay on TLS.

**Scope**
- Release needs film, TV, anime, music, audiobooks, Live TV, DVR, downloads, personal state and discovery. Watch Together and casting: fix defects, no redesign. CarPlay and Android Auto are out. Roku is out of this programme. Light theme is out.
- Live TV: one tuner input per channel however many people watch; each viewer may have their own conversion.
- DVR stays private per profile, but a programme two people record is stored once.
- Downloads may outlive server tidying; everything they need (titles, artwork, subtitles) travels with them.
- The deliberate security trade-offs stand: light password rules, no lockout, nothing encrypted at rest, a TV code confers the approver's role. Genuine defects get fixed.
- **Tuner pre-emption (15 Oct):** an owner server setting. Default: a recording that needs a tuner someone is watching does not start (it fails with a clear reason); live viewing is never interrupted. The other option lets the recording take the tuner, with a countdown shown on every player.
- **Next episode during the credits (15 Oct):** a viewer playback preference; default off (nothing starts before the title ends). On: early start only with autoplay on and a high-confidence credits marker.
- **Player choices rule (15 Oct):** settings in the Settings menu apply everywhere; changes made inside the player apply to that playback session only (carried through autoplay/queue continuation, reset on a fresh start). No per-title memory of in-player choices, no "forget last choice" control.
- **PiP subtitles (15 Oct):** not supported. Burned-in subtitles stay; drawn ones are not shown in PiP; PiP never triggers conversion.
- **Exact resume in MP3/ADTS/Ogg (15 Oct):** no conversion; a per-file seek index and direct range reads.
- **Collections (15 Oct):** personal collections have a manual order (default sort, playback follows it). Administrator scope as built is confirmed. "Progress" sort on My List/Favorites dropped.
- **15 Oct evening answers:** owners' own (self-signed/private) certificates are trusted by the apps; Apple allows plain HTTP only on the local network (App Store rule), HTTPS for remote; any browser reaching the local web UI works with no setup; building a library from scratch must be quick; volume/mute is the system's on every platform except web; missing titles kept for a server-configured time with "never"; secure connection is the default — with a Portico Account a new server is claimed and gets its certificate as soon as it is created and named, explained simply in Server Settings (no external help page); dropped from feature lists: spoiler controls, NFO/artwork writing, country rating tables, unrated/kids allow-list, unified label rules; Hosted redeploy and release keys later; source published before release; licence info in every app's About page.
- **API keys (15 Oct):** super flexible, Plex-like: a key can do nearly everything its member can (within its scope), from any origin, and in a query parameter for GET URLs; only sign-in/password/2FA, minting keys/tokens, ownership and claim flows stay session-only.

## 2. Rules of work

1. **Work only inside the worktree(s) named in your lane brief.** Never touch `/home/justin/projects/Portico/apps/*` or `/home/justin/projects/Portico/portico-internal` directly. Never push. Never deploy anything.
2. **Commits** (lanes that can commit): small, logical commits as you go, with Justin's existing git identity. Plain-language subjects describing the behaviour change. No AI or tool names, no "audit", no bug IDs, no session references in commit messages, code or comments. Lanes that cannot commit follow the snapshot instructions in their brief.
3. **Do not run full builds or full test suites.** Justin's instruction: deliver the whole scope first, with limited testing. Allowed: `go build ./...` (prefix Go commands with `GOFLAGS=-p=2`; the machine is shared by 16 lanes), `go vet` and `go test` on the packages you touched with `-run` where sensible, `npx tsc --noEmit`, a single test file. Not allowed: `go test ./...`, whole-repo `npm test`/`npm run check`, Gradle, Xcode, Docker builds, anything that takes more than a few minutes. Write the tests that belong with your change even when you only run some of them.
4. **Not being able to test here is never a reason to skip.** Windows, macOS, Apple, Android, GPU, tuner and device work is all written here. Cross-compile where possible (`GOOS=windows`, `GOOS=darwin`). Leave a test note someone else can follow cold: environment, steps, what pass and fail look like.
5. **Schema.** Portico is pre-release: edit `server/internal/persistence/migrations/0001_baseline.sql` directly (read `server/internal/persistence/README.md` first), then run `python3 scripts/schema-sync.py` from the repository root. Do not add compatibility code for old databases. Another lane may edit the same file; keep your edits localised to your own tables.
6. **API.** Every route you add or change is on the `apikit` registry with a typed contract. If you change the shape of a route, update its non-UI consumers in `packages/client-core` (parsers, services, types) in the same lane. Leave screen work to the client lanes unless your brief says otherwise, and list it in your report.
7. **Stay in your area.** Shared wiring files (`server/cmd/server/main.go`, route registration, the settings document, the string catalogue) may be edited minimally. For anything larger outside your area, do not do it; record it under "Needed from others".
8. **Security.** Do not weaken any validation, authentication, authorisation, sandboxing, signature check or bound, except exactly where section 1 says so.
9. **Text.** User-facing strings go through the string catalogue (`packages/i18n`) and read as plain sentences an ordinary person understands. No jargon, no codes.
10. **Bugs.** Your lane brief lists audit documents. Read the bug entries (`B-…`) in them and in `BUG-REGISTER.md`. Fix every one that is still present and is not contradicted by section 1. `FIX-LOG-2026-10-12.md` lists what is already fixed on your base branch; do not redo those. The audit was static reading, so confirm each bug in the current code first.
11. **When in doubt** choose the option closest to Plex's behaviour for the experience, the smallest sound design for the code, and write the choice down.
12. **Do not spawn subagents, helpers or parallel agents of any kind** (no Agent or Task tool, no extra model sessions). The owner's usage budget allows exactly one agent per lane. Do all the work yourself, in sequence.
    **The one exception is the cheap helper model**, which costs almost nothing and may be used as much as is useful: from inside your own worktree run `bash /home/justin/projects/Portico/.worktrees/audit-implementation/_program/helper.sh "<task>"` (Deepseek V4.1 Flash through Opencode; it can read and edit files under the current directory, and several can run at once). It is roughly a mid-tier model: good for bounded, well-specified jobs such as searching and summarising code, enumerating call sites, mechanical edits across many files, drafting table-driven test cases, moving strings into the catalogue, or a first draft of a small self-contained function. Give it a complete, exact task with paths and the expected result. **It needs oversight: never accept its output blindly.** Read every diff it makes (`git diff`), verify every claim it reports against the code, and keep design, security-sensitive code and anything subtle for yourself. If the command fails in your environment, carry on without it.
13. Do not create planning or design documents in the repository beyond what your brief asks for. Keep notes in your report file.

## 3. Reference material (read-only)

- Audit: `/home/justin/projects/Portico/portico-internal/Project Architecture/Audits/2026-10-11-comparative-audit/`
  - `00-FINAL-REPORT.md` (bets, decision register), `BUG-REGISTER.md`, domain documents `01`–`25`, `verification/V01`–`V11` (second review with fix directions), `verification/G1`–`G4` (which "missing" features partly exist, and where), `FIX-LOG-2026-10-12.md`.
- Architecture: `/home/justin/projects/Portico/portico-internal/Project Architecture/01`–`08 *.md`, `Backlog.md`, `Specs/`. Product contracts: `/home/justin/projects/Portico/portico-internal/Product Contracts/`. Where these disagree with section 1, section 1 wins; note the document that needs correcting in your report.
- Other products' source, for comparison only (do not copy code; several are GPL or AGPL with different terms): `/home/justin/projects/Portico/.worktrees/audit-implementation/_reference/` (jellyfin, jellyfin-web, jellyfin-android, jellyfin-androidtv, swiftfin, plezy, streamyfin, wholphin, finamp, navidrome, audiobookshelf, intro-skipper, jellyfin-docs).

## 4. Your report

Write `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/<lane>.md` (if the Write tool refuses, write it with a shell heredoc). Update it as you go so a crash loses little. Sections, in this order:

1. **Outcome** — what now works, in plain sentences.
2. **Decisions I made** — each choice a product owner might want to know about.
3. **Bugs** — one line per bug ID in your documents: fixed, already fixed, not present, or not fixed and why.
4. **API changes** — route, old shape, new shape, removed routes.
5. **Schema changes.**
6. **Owner or viewer settings added or wired** — name, type, default, where it takes effect, and whether a screen exists for it.
7. **Needed from others** — client screens, server pieces, strings, documents.
8. **Test notes for other environments.**
9. **Not finished** — be exact. An honest gap is far better than a claim that turns out false.

Your final message must be under 250 words: the outcome, what is not finished, and the path of your report. Everything else belongs in the report.
