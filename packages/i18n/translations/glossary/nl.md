# Dutch (nl) glossary — machine translation, needs native review

Address form: informal second person throughout — "je/jij/jouw", imperative
without "u" (matching Netflix/NL streaming usage: "Kijk je nog?", "Log opnieuw in").
"uw" is never used.

## Core UI terms (English → Dutch)

- Settings → Instellingen
- Sign in → Inloggen · Sign out → Uitloggen · Sign-in (noun) → login
- Library → Bibliotheek · Libraries → Bibliotheken
- My List → Mijn lijst (never "Mijn Lijst")
- Continue Watching → Verder kijken
- Up Next → Hierna (home row) / Volgende (player queue)
- Subtitles → Ondertiteling · captions → ondertiteling
- Episode → Aflevering · Season → Seizoen · Series (show) → Serie
- Movie → Film · Movies → Films · TV shows → Series
- Downloads → Downloads · Download (verb) → Downloaden
- Profile → Profiel · Account → account
- Home → Home · Search → Zoeken · Saved → Bewaard
- Play → Afspelen · Pause → Pauzeren · Resume → Hervatten
- Stop → Stoppen · Shuffle → Shuffle · Repeat → Herhalen
- Delete → Verwijderen · Remove → Verwijderen · Save → Opslaan/Bewaren
- Cancel → Annuleren · Close → Sluiten · Done → Gereed
- Back → Terug · Next → Volgende · Previous → Vorige
- Retry → Opnieuw proberen · Try again → Probeer het opnieuw
- Skip → Overslaan · Refresh → Vernieuwen · Undo → Ongedaan maken
- Watch Together → Samen kijken · host → host
- Live TV → Live-tv (also "live-tv" mid-sentence) · Guide → Gids
- Record/Recordings → Opnemen/Opnamen · tuner → tuner
- Channels → Kanalen · Library Channels → Bibliotheekkanalen
- Favorites → Favorieten · Playlists → Afspeellijsten · Collections → Verzamelingen
- Notifications → Meldingen · History → Geschiedenis · Details → Details
- Quality → Kwaliteit · Original → Origineel · Automatic → Automatisch
- Audio → Audio · Video → Video · HDR → HDR
- Server → Server · Owner → Eigenaar · Member → Lid
- Devices → Apparaten · This device → Dit apparaat · TV → tv (lowercase mid-sentence)
- PIN → pincode (lowercase mid-sentence) · Password → Wachtwoord
- Two-step verification → Verificatie in twee stappen · Recovery codes → Herstelcodes
- Invitation → Uitnodiging · Invite → Uitnodigen
- Metadata → Metadata · Match (verb) → Matchen · Scan → Scannen
- Artwork → Artwork · Trailer → Trailer · Cast → Cast · Crew → Crew
- Rating → Score (user-facing) / Classificatie (parental-rating context)
- Unwatched → Niet bekeken · Unplayed → Niet afgespeeld · Watched → Bekeken
- Remote access → Toegang op afstand · Secure connection → Beveiligde verbinding
- Backup → Back-up · Storage → Opslag · Folder → Map · File → Bestand
- Report a problem → Een probleem melden · Feedback → Feedback
- Support → Ondersteuning · Help → Hulp · Troubleshooting → Probleemoplossing
- Song → Nummer · Track → Track · Album → Album · Artist → Artiest
- Audiobook → Luisterboek · Book → Boek · Author → Auteur · Narrator → Verteller
- Lyrics → Songtekst (singular preferred)
- Sleep timer → Slaaptimer · Picture in Picture → Picture in Picture
- Go to live → Naar live gaan · LIVE badge → LIVE
- AirPlay/Google Cast/Chromecast → unchanged product names
- Quick Connect → Snel verbinden · Deep links (Server › Libraries › Settings) keep English "›" separators with Dutch page names

## Product names (never translated)

Portico, Portico Account (always with capital A, never "Portico-account"),
Portico Services, TMDB, TheTVDB, MusicBrainz, AniList, IMDb, OpenSubtitles,
FFmpeg, mpv, HDHomeRun, Xtream, M3U, XMLTV, rclone, WebDAV, Dolby Vision,
Google, Apple, MCP. URLs (getportico.tv/…) and brand strings stay as-is.

## Punctuation and typography

- Quotation: “…” (U+201C/U+201D) for quoted titles/names inside strings;
  never "...".
- Apostrophe: ’ (U+2019) everywhere, including Dutch possessives
  (“server’s account” → "account van de server" where possible, but ’s kept
  where the English structure is mirrored, e.g. "Portico’s").
- Ellipsis: single character … (never three dots).
- No space before ! ? : ; (unlike French).
- Decimal/thousands follow Dutch locale formatting handled by Intl at runtime;
  source placeholders like {count} are kept verbatim.

## Grammar and style choices

- Informal "je" everywhere, including server-console strings aimed at owners.
- Past-tense error pattern: "kon niet worden …" (e.g. "kon niet worden geladen");
  user-action failures: "Probeer het opnieuw."
- "Unavailable" → "Niet beschikbaar" (adjective) / "Niet-beschikbaar item" (noun phrase).
- Plural rules: Dutch needs one/other; every plural got both branches, even where
  English only has other (added "one" where required by the validator).
- select keys (movie/show/…, home/library/…, on/off…) kept verbatim; only the
  message text was translated.
- Compound nouns written closed where idiomatic (inlogpogingen, servereigenaar,
  kijkgeschiedenis, afspeellijst) to stay close to English label length.
- "Keer afgespeeld" used for play counts; server statistics header uses "Plays"
  → "Keer afgespeeld".

## Known weak spots for the native reviewer

- Long server-console sentences (transcoding, secure connection, storage) may
  read stiff; check flow rather than terminology.
- "Verder kijken" vs "Doorgaan" for Resume/Continue: Resume → Hervatten,
  persistent row → Verder kijken.
- "caption" vs "subtitle": both → ondertiteling; captionsOn/Off use
  "Ondertiteling aanzetten/uitzetten".
- builder.* channel-template strings are dense; verify against the UI.
- web.setup.welcomeLede and a few marketing-style ledes kept the English
  contraction structure ("Laten we …"); fine but flag if too formal.
- duration.hoursMinutes uses "{hours}u {minutes}m" (Dutch abbreviation is "u").
