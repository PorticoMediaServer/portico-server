# Czech (cs) glossary

Address: informal tykání (2nd person singular: ty, tvůj, imperative přihlas se,
zkus to znovu), the way Czech Netflix, Voyo and Disney+ address viewers.
Gender is avoided wherever Czech grammar allows it: impersonal and passive
constructions ("Nepodařilo se načíst", "Hlášení bylo odesláno"), neuter past
participles ("Odebráno", "Uloženo"), and rephrased possessives. Where Czech
grammar forces gender (nouns like "vlastník", "divák", participles like
"připojil ses"), the generic masculine is used, as is standard in Czech UIs.
Known spots: web.together.reconnecting, membership.transfer*,
web.directJoin.forAccountHere, notifications about other people.

## Core UI terms (English → Czech)

- Search → Hledat (verb) / Vyhledávání (noun section)
- Settings → Nastavení
- Home → Domů (nav); domovská stránka (in sentences)
- Library → Knihovna; Libraries → Knihovny
- My List → Můj seznam ("V mém seznamu" for "In My List")
- Continue Watching → Pokračovat ve sledování
- Favorites → Oblíbené
- Saved → Uložené
- Movies → Filmy; Movie → Film
- Shows / TV shows → Seriály; Show → Seriál
- Season → Řada (Czech TV convention, not "sezóna")
- Episode → Epizoda; Track/Song → Skladba; Artist → Interpret; Album → Album
- Book series → Knižní série; Books → Knihy; Audiobooks → Audioknihy
- Subtitles → Titulky; Audio → Zvuk; Video → Video
- Downloads → Stahování (feature); Downloads (screen/files) → Stahování / Stažené soubory in prose
- Play → Přehrát; Pause → Pozastavit; Resume → Pokračovat; Stop → Zastavit
- Sign in → Přihlásit se; Sign out → Odhlásit se; Sign up / Create account → Vytvořit účet
- Profile → Profil; Account → Účet
- Server → Server; Server console → konzole serveru; Remote access → Vzdálený přístup
- Secure connection → Zabezpečené připojení
- Live TV → Živé vysílání; Guide → Program (TV guide); Channel → Kanál; Tuner → Tuner
- Recordings → Nahrávky; Record (verb) → Nahrát; Recording → Nahrávání; DVR → DVR
- Watch Together → Společné sledování; Host → Hostitel
- Cast → Obsazení; Crew → Štáb; Director → Režisér/Režie; Starring → V hlavní roli / V hlavních rolích
- Collection → Sbírka; Playlist (kept as loanword, like Spotify CZ) → Playlist
- Watchlist action → Přidat do mého seznamu
- Mark as watched/unwatched → Označit jako zhlédnuté/nezhlédnuté
- Played (music) → Přehráno; Played (history) → Přehrání
- Notifications → Oznámení; History → Historie
- Appearance → Vzhled; Language → Jazyk; Preferences → Předvolby
- Quality → Kvalita; Automatic (quality) → Automatická; Original → Původní/Původní kvalita
- Converting / Transcoding / Conversion → Převádění / Převod (noun)
- Direct play → Přímé přehrávání; Direct stream → Přímý stream
- Buffer → Vyrovnávací paměť; Buffering badge → Načítání
- Subtitles burned in → vypálené titulky
- Skip intro/recap/credits → Přeskočit úvod/shrnutí/titulky
- Storage → Úložiště; Server storage → server storage; Backup → Záloha; Restore → Obnovit
- Scan → Skenovat; Refresh → Obnovit; Match → Spárovat (metadata matching) / Shoda (filter)
- Delete → Smazat; Remove → Odebrat/Odebrat (files from disk: "Odebrat i soubory z disku")
- Devices → Zařízení; This device → Toto zařízení; TV → TV (kept, as in Czech UIs)
- PIN → PIN; Password → Heslo; Two-step verification → Dvoufázové ověření
- Owner → Vlastník; Member → Člen; Administrator → Administrátor
- Limits → Limity; Parental/rating → Věkové hodnocení (content rating); Rating (score) → Hodnocení
- API keys → API klíče; Personal access tokens → Osobní přístupové tokeny; Webhooks → Webhooky
- MCP Assistants → Asistenti (MCP); Logs → Protokoly (server logs); Support export → Export pro podporu
- Quick Connect → Rychlé připojení (descriptive, not kept in English)
- Watch state words: Unwatched → Nezhlédnuté, Unplayed → Nepřehrané, Watched → Zhlédnuto
- Libraries feature: Library Channels → Knihovní kanály; Channels (custom) → Knihovní kanály
- Trickplay → Náhledové miniatury ("Preview thumbnails" wording used instead)
- Still watching → Stále sleduješ?
- Genre → Žánr; Year → Rok; Decade → Dekáda; Duration/Runtime → Délka/Stopáž
- Portrait/Landscape artwork roles → Portrét/Pozadí etc. (see title.* artRole)
- Sort by → Řadit podle; Filters → Filtry; Recently added → Nedávno přidané
- Age units: {n} minut/minuty/minut; hodin/hodiny/hodin; sekund/sekundy/sekund; dní/dny/den
- % sign → non-breaking space before % ("50 %", "{value} %", "Zbývá {percent} %")
- Mbps → Mb/s; kbps → kb/s (Czech convention)
- Port (network) → Port; Address → Adresa; Certificate → Certifikát
- Hours (viewing hours) → Hodiny sledování; Viewing hours over → Hodiny sledování skončily

## Plurals

Czech one/few/other throughout (Intl.PluralRules: one=1, few=2–4, other=0,5+).
Standard noun triples used, e.g.:
- titul/tituly/titulů; epizoda/epizody/epizod; řada/řady/řad; položka/položky/položek
- sekunda/sekundy/sekund; minuta/minuty/minut; hodina/hodiny/hodin; den/dny/dní
- nahrávka/nahrávky/nahrávek; soubor/soubory/souborů; kanál/kanály/kanálů
- stream/streamy/streamů; tuner/tunery/tunerů; hlas/hlasy/hlasů; kód — invariant
- Bare English counts upgraded to plurals where Czech needs them
  (mediaInfo.channels, web.saved.days, filter.preset.lastDays, card.episodesAired...).
- Seconds in "Back/Forward {n} seconds" and durations use accusative
  (sekundu/sekundy/sekund); "Try again in {n}s" likewise.
- lib.count / lib.countAtLeast nest select pivots inside one/few/other with full
  Czech case triples (film/filmy/filmů, seriál/seriály/seriálů, ...).
- Zero-forms kept where English has =0 (recordings.matches, builder.matches,
  web.metadata.candidateCount).

## Punctuation and typography

- Czech quotes „…" (not English "…"); source curly quotes converted.
- Ellipsis … (U+2026) kept as in source for progress/loading strings.
- En dash ranges kept ({min}–{max}, {from}–{to}, {start} – {end}).
- ’ (U+2019) used only where unavoidable; no ASCII apostrophe anywhere
  (validated by tooling). English possessives rephrased ("{server}’s clock" →
  "Hodiny {server}").
- "Server › Section" breadcrumb separator kept.
- Sentence case for buttons and titles ("Přidat do sbírky", "O Portico"),
  matching Czech UI convention rather than English title case.
- Product names never declined: always "Portico", "Portico Account",
  "Portico Services" verbatim (tooling rejects inflected forms like "Portica").
  "Portico Server" and "Server Portico" kept where source has them.
- Proper names kept: TMDB, TheTVDB, MusicBrainz, AniList, IMDb, HDHomeRun,
  Dolby Vision, Google Cast, AirPlay, mpv, FFmpeg, rclone, OpenSubtitles, M3U,
  XMLTV, Xtream, UPnP/NAT-PMP/PCP, WebDAV, IPv6, PEM, SQLite, JSON, GB/MB/MiB/GiB.

## Judgement calls a reviewer should check

- title.credits → "Filmografie": standard for actor pages, approximate for
  musicians/authors. title.knownFor → "Známé tituly".
- title.kind book/audiobook_file both → "Audiokniha"; book_series → "Knižní série".
- entry.watchedCount uses "Zhlédnuto: # / Nezhlédnuto: #" (gender-free counts).
- player.nowPlayingTitle/player.stageLabel/playback "Now playing": "Právě hraje"
  vs "Přehrává se" — both used by context (title-bearing vs bare).
- "Up Next" → "Dále"; player.queue ("Up next" queue) → "Ve frontě" to differ.
- "Saved" as a section → "Uložené"; "Saved views" → "Uložená zobrazení".
- auth.agreeToTerms/terms/privacyPolicy composes as
  "Pokračováním souhlasíš s Podmínkami Zásadami ochrany osobních údajů"
  (app joins the two links; order/words chosen to compose).
- signIn.error.* and web.signIn.error.* ("INT gate 3" one-sentence copies)
  translated 1:1, mirroring the English sentence-per-cause structure.
- language.* names translated into Czech lowercase endonym-style nouns
  (angličtina, čeština, …), matching what fr/de/es machine translations did
  (they used target-language names, not English endonyms).
- Typed confirmation word server.confirm.word.delete left as "delete"
  (typed token, must match server expectation).
- web.serverless.direct, web.directJoin.*: "Join" → "Připojit se" (server
  membership), while together.join → "Připojit se" (group session) — same verb,
  context differs.
- rclone/WebDAV/MCP/FFmpeg technical strings kept close to English terms
  (binárka rclone, sandbox, fingerprint → otisk, webhook → webhook).
- guide.keyboardHint keeps raw key names (W, R, N, Shift+N) untranslated.
- web.player.keys.jump "0–90 %" kept numeric; starRating "★ {value}" kept.
- recordings/live "Record" button → "Nahrát"; "Record series" → "Nahrávat seriál".
- "Record" in noSourcesOwnerBody etc. consistent with "Nahrát".
