# Glossary — Norwegian Bokmål (nb)

Address form: informal **du** throughout (Netflix/NRK convention: "Logg inn",
"Seerhistorikken din"). No formal De anywhere.

Punctuation/typography: guillemets **«»** for quoted titles («{title}»);
ellipsis **…** (U+2026) kept as in source; **·** (U+00B7) as separator;
non-breaking space before **%** and units ("{percent} %", "10 min");
time range en-dash ("1–100"); alphabetised sort uses **A–Å** (not A–Z).
Titles/buttons use sentence case. Typographic apostrophe **’** (U+2019) only;
plain `'` never appears. Product names stay English: Portico, Portico Account,
Portico Services, Quick Connect, Dolby Vision, AirPlay, Google Cast, Fire TV,
TMDB, MusicBrainz, AniList, HDHomeRun, M3U, XMLTV, Xtream, FFmpeg, mpv.

## Core app terms (English → nb)

- Settings → Innstillinger
- Sign in / Sign out → Logg inn / Logg ut
- Library → Bibliotek
- Home → Hjem
- My List (Watchlist) → Min liste
- Favorites → Favoritter
- Continue Watching → Fortsett å se
- Continue Listening → Fortsett å lytte
- Up Next → Neste
- Subtitles → Undertekster
- Episode → Episode, Season → Sesong, Series → Serie
- Movie → Film, Movies → Filmer, TV show → TV-serie
- Downloads → Nedlastinger
- Profile → Profil
- Search → Søk
- Play → Spill av, Pause → Pause, Resume → Fortsett
- Recordings (DVR) → Opptak, Recording → Opptak / Tar opp (state)
- Live TV → Direkte-TV, Guide → Guide, Channel → Kanal
- Watch Together → Se sammen, group → gruppe
- Notifications → Varsler
- History → Historikk
- Playlists → Spillelister, Collections → Samlinger
- Saved views → Lagrede visninger
- Cast → Skuespillere, Crew → Stab
- Shuffle → Bland, Repeat → Repeter
- Trailer → Trailer, Synopsis → Synopsis
- Lyrics → Sangtekster
- Audiobook → Lydbok, Song → Sang, Album → Album, Artist → Artist
- Server console → Serverkonsollen, Dashboard → Dashbord

## Server / account terms

- Server account → Serverkonto, Portico Account → Portico Account (untranslated)
- Owner → Eier, Member → Medlem, Administrator → Administrator
- Invitation → Invitasjon, Invite → Inviter
- Remote access → Fjerntilgang, Secure connection → Sikker tilkobling
- API key → API-nøkkel, Personal access token → Personlig tilgangstoken
- Webhook → Webhook (lowercase mid-sentence: Webhooken)
- Backup → Sikkerhetskopi, Restore → Gjenopprett
- Scan → Skann, Library scan → Bibliotekskanning
- Metadata → Metadata, Match → Match (verb: matche), Matching → Matching
- Tuner → Tuner, Recording space → Opptaksplass
- Maintenance window → Vedlikeholdsvindu, Schedule → Tidsplan
- Log → Logg, Health → Helse, Support export → Støtteeksport
- PIN → PIN, Password → Passord, Two-step verification → To-trinns bekreftelse
- Recovery code → Gjenopprettingskode, Authenticator app → Autentiseringsapp

## Playback / quality terms

- Quality → Kvalitet, Automatic → Automatisk, Original → Original
- Conversion → Konvertering (transcoding), Convert → Konverter
- Buffer → Buffer, Sleep timer → Sovnetimer
- Fullscreen → Fullskjerm, Picture in Picture → Bilde-i-bilde
- Chapters → Kapitler, Trickplay/seek-bar pictures → Spolefeltbilder
- HDR → HDR, Dolby Vision → Dolby Vision (untranslated)
- Subtitle timing → Tidsinnstilling, Forced subtitles → Tvungne undertekster

## Judgement calls a reviewer should know

- "Saved" (section) → **Lagret**; "Saved couldn’t load" → "Lagret kunne ikke lastes".
- "Server claiming" (claim) → **knytte serveren til kontoen** ("Denne serveren
  kunne ikke knyttes til kontoen"), since Norwegian has no equivalent verb.
- "Watch Together" kept as the feature name **Se sammen** everywhere.
- "Show" (title kind) → **TV-serie** to distinguish from "Serie" (show grouping);
  library tab "Shows" → **Serier**.
- "Disc" → **Plate**; "Plate" ambiguous with dish but standard for optical media.
- "Play on" (cast target) → **Spill av på**.
- "Unwatched"/"Unplayed" for music → **Usett** / **Ikke spilt av** (music uses
  "spilt av", video uses "sett").
- "Marked as watched" → **Merket som sett** (past participle agrees with common
  gender default; neuter "sett" also correct for intetkjønn nouns).
- DVR "padding" (pre/post roll) → **start-/sluttid** (plain words, no jargon).
- "This server" possessive forms use dens/dens (neuter serveren? no: serveren
  is masculine: "serveren sin/dens" → used **dens** for consistency with
  masculine nouns; "tittelen" feminine/masculine → **dens**).
- "Account" alone (non-Portico) → **Konto**; "Portico Account" never translated.
- "Portico Accounts" (plural, hosted service) kept as English plural + Norwegian
  verb agreement.
- "Check in" (download licence) → **melde seg på**; "check out" avoided.
- "Slate" (channel filler) → **Plakat**; "route" (delivery fallback) left as
  **måte** ("den måten").
- "Library Channels" → **Bibliotekkanaler** (compound, no space).
- Guide "This channel/That page" demonstratives → Denne/Den siden.
- "A–Z" sort labels localised to **A–Å** for Norwegian alphabet.
- "Sleep" (player option, short) → **Sovnetimer** (matches summary string).
- Age-rating "Highest rating" → **Høyeste aldersgrense** (clarifies meaning).
- "Keep missing titles for" + duration unit rendered by unit strings (dager).
- Web "http(s)://" user-facing examples kept verbatim (192.168.1.20:32500).
- "recovery owner" → **Gjenopprettingseier**; "recovery sign-in" →
  **Gjenopprettingspålogging**.

## Known weak spots

- claim-related strings (no Norwegian verb; paraphrased as "knytte til kontoen").
- "Saved" as a noun section name (Lagret) may read adjectival; alternatives:
  "Lagrede elementer" (used only in error branch).
- Long server-console sentences (retention, secure-connection, home-network
  help) are faithful but dense; a native reviewer should smooth them.
- "DVR"/"PVR" never surfaced to users; "Opptak" used everywhere instead.
- Gender agreement in a few participles (merket/beholdt) defaults to common
  gender; check against the noun in context during review.
