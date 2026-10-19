# Polish (pl) glossary — Portico UI translation

Machine translation pass, 18 October 2026. Every message is recorded with
state `machine` and needs native-speaker review.

## Address form

Informal second person singular (`ty`), as Polish streaming apps do
(Netflix/Plex style): imperatives like `Odtwórz`, `Spróbuj ponownie`,
`Wybierz`; possessives `Twój/Twoje`. Where English is impersonal passive
(`Couldn’t be saved`), Polish uses the impersonal reflexive (`Nie udało się
zapisać`, `Nie da się…`). Mixed-voice spots to check in review are listed
under “Judgement calls”.

## Key terms (English → Polish)

| English | Polish | Notes |
|---|---|---|
| Library | Biblioteka | |
| My List / Watchlist | Moja lista | `title.watchlist`, `saved.tab.watchlist`, filters |
| Continue Watching | Oglądaj dalej | quoted as „Oglądaj dalej” inside sentences |
| Favorites | Ulubione | |
| Settings | Ustawienia | |
| Sign in / Sign out | Zaloguj się / Wyloguj się | noun: logowanie |
| Profile | Profil | |
| PIN | PIN | `Nie pamiętasz PIN-u?` |
| Episode / Season | Odcinek / Sezon | episode code `S{n} O{m}` (E→O) |
| Movie / Show (TV) | Film / Serial | `title.kind show` → Serial; search chip `shows` → Seriale |
| Song / Track | Utwór | `songs` tab → Utwory |
| Artist | Wykonawca | music context |
| Album | Album | |
| Audiobook | Audiobook | |
| Book | Książka | `book_series` → Seria książek |
| Author / Narrator | Autor / Lektor | |
| Cast / Crew | Obsada / Ekipa | `title.credits` (both) → Twórcy |
| Collection / Playlist | Kolekcja / Playlista | verb “playlisty” declined normally |
| Queue | Kolejka | |
| Playback | Odtwarzanie | verb: odtwarzać/grać (short labels use Odtwórz) |
| Downloads | Pobrane (tab/screen); Pobierz (verb); pobrane pliki (files) | `feature.downloads` → Pobrane |
| Subtitles | Napisy | |
| Quality | Jakość | Automatic → Automatyczna; Original → Oryginalna |
| Server | Serwer | `servers.fallbackName` → Serwer Portico |
| Account | Konto | product name below takes precedence |
| Portico Account | Portico Account (never translated) | declined: Twojego / Twoje / Twoim Portico Account |
| Watch Together | Wspólne oglądanie | feature name, translated descriptively |
| Live TV | TV na żywo | |
| Guide (TV) | Przewodnik | |
| Recording / Recordings (DVR) | Nagranie / Nagrania | verb: Nagrywaj |
| Tuner | Tuner | |
| Channel | Kanał | |
| Library Channels | Kanały biblioteki | feature name kept parallel to EN |
| Notifications | Powiadomienia | |
| History | Historia | |
| Search | Szukaj (verb/button); Wyszukiwanie (noun) | placeholder: Filmy, seriale, muzyka i więcej… |
| Refresh | Odśwież | |
| Cancel / Dismiss / Close / Discard | Anuluj / Zamknij / Zamknij / Odrzuć | Dismiss → Zamknij |
| Try again | Spróbuj ponownie | |
| Save / Saved | Zapisz / Zapisane (screen) | Saved views → Zapisane widoki |
| Delete / Remove | Usuń | typed confirm word `delete` kept as-is |
| Undo | Cofnij | |
| Details | Szczegóły | |
| Match / matching (metadata) | Dopasowanie / Dopasowywanie | `Match the show` → Dopasuj serial |
| Title (media) | Tytuł | |
| Metadata | Metadane | |
| Artwork | Grafika / Grafiki | |
| Rating | Ocena | content rating → Kategoria wiekowa |
| Genre | Gatunek | |
| Studio / Network (TV) | Studio / Stacja | |
| Duration / Runtime | Czas trwania / Długość (columns) | short: `{h} godz. {m} min`, `{m} min`, `{s} s` |
| Sleep timer | Timer snu | |
| Speed | Tempo | |
| Volume | Głośność | |
| Buffer | Bufor | |
| Container | Kontener | |
| Bitrate | Szybkość transmisji | unit: Mb/s, kb/s |
| HDR | HDR | |
| Dolby Vision | Dolby Vision (kept) | |
| Remote access | Dostęp zdalny | |
| Secure connection | Bezpieczne połączenie | |
| Invitation | Zaproszenie | |
| Device | Urządzenie | |
| Two-step verification | Weryfikacja dwuetapowa | |
| Recovery codes | Kody zapasowe | |
| Authenticator app | Aplikacja uwierzytelniająca | |
| API key / Token / Webhook | Klucz API / Token / Webhook | |
| Backup | Kopia zapasowa | verb: Utwórz kopię |
| Scan (library) | Skanuj / Skanowanie | |
| Folder | Folder | |
| Storage / Space | Miejsce | “free space” → wolne miejsce |
| Trash/Clean up | Posprzątaj | |
| Owner | Właściciel | |
| Member | Członek | |
| Administrator | Administrator | |
| Home (screen/row) | Strona główna | `home.*`: strona główna |
| Discover | Odkrywaj | |
| Trending / Popular on this server | Teraz na czasie / Popularne na tym serwerze | |
| Recommended for you | Polecane dla Ciebie | |
| Up Next | Następnie (row); Następny (player post-play) | |
| Trailer | Zwiastun | |
| Synopsis | Streszczenie | |
| Extras | Dodatek (`kind.extra`) | |
| Lyrics | Tekst (`player.lyrics`; full word in player) | “Lyrical lines” in errors → linijki |
| Chapters | Rozdziały | |
| AirPlay / Google Cast / Chromecast keys | kept | |
| Picture in Picture | Obraz w obrazie | |
| Fullscreen | Pełny ekran | |
| 4K / HD / SD | 4K / HD / SD (kept) | |
| MPV / FFmpeg / TMDB / M3U / XMLTV | kept | |

## Punctuation / typography

- Apostrophes: only `’` (U+2019); ASCII `'` never appears (verified by
  import on every chunk). Possessives reworded to avoid them
  (`{name}’s group` → `Grupa: {name}`).
- Quotes: Polish `„…”` for quoted titles/names inside sentences; English
  `"…"` from source converted. `«`/`»` unused.
- Ellipsis `…` kept where the source trails off (loading/working states).
- Middle dot `·` kept as the source’s separator (`{field} · {direction}`).
- `›` breadcrumbs kept; section names translated
  (`Serwer › Dostęp`, `Serwer › Biblioteki › Ustawienia`,
  `Ustawienia › Dostęp zdalny › Bezpieczne połączenie`).
- Capitalisation: sentence case for labels/buttons as in Polish apps
  (`Dodaj do playlisty`, `Oglądaj dalej`); language names capitalised
  (`Angielski`, `Japoński`) to match catalogue convention.
- Brand button labels localised as Polish apps do: `Zaloguj się przez
  Google`, `Zaloguj się przez Apple`.
- Decimals/units: `Mb/s`, `kl./s` (fps), `{n}-bitowy`; durations
  `{h} godz. {m} min`.

## Plurals

`one / few / many / other` everywhere the English has a plural, plus `=0`
branches kept (`recordings.matches`, `web.dvr.matches`,
`builder.matches`, `web.libraryChannels.ruleCount`,
`web.metadata.candidateCount`, `web.metadata.cascadeProgress`).
`other` (fractions) mirrors `few` for nominative nouns, genitive where the
construction needs it. A bare `{count}` + noun without ICU (e.g.
`mediaInfo.channels`, `card.minutesLeft`) uses a neutral form
(`Kanały: {count}`, `Zostało {count} min`).

## Judgement calls for the reviewer

1. `Saved` as a screen name: rendered `Zapisane`
   (`saved.loadFailed` → `Nie udało się wczytać Zapisanych`). Confirm.
2. `title.credits` / `web.person.credits` (cast+crew combined) → `Twórcy`;
   `noCredits` → `Brak twórców`. Alternatives: `Obsada i twórcy`.
3. `title.born` / `web.person.born` use the neutral `Data urodzenia: {date}`
   to avoid gendered `Urodzony/Urodzona`.
4. `player.opt.sleep` (short label) and `player.opt.sleepTimer` both read
   `Timer snu`; split if the UI needs distinct strings.
5. `device.platform.ios` → `iPhone lub iPad` (kept parallel to source).
6. `server.confirm.word.delete` kept as `delete` — the typed confirmation
   token; confirm with server before localising.
7. Gendered past participles default to masculine/impersonal
   (`Oznaczono…`, `Usunięto…`); second-person results use masculine
   (`Oceniłeś`, `Dołączyłeś`). Flagged for review, not blocking.
8. `web.together.defaultName` (`{name}’s group`) → `Grupa: {name}`.
9. `lib.count` / `lib.countAtLeast` nest plural+select; Polish forms use
   `#` before a lowercase noun (`# filmy`, `#+ filmów`) — check rendering.
10. `guide.onNowLeft` / `guide.minutesLeft` use `min` abbreviation forms to
    stay short on TV cards.
11. `entry.watchedCount` renders both plurals with `·` separator; feminine
    vs masculine agreement for mixed content is approximate.
12. `settings.row.stillWatching` keeps the quoted question:
    `Pytaj „Nadal oglądasz?”`.
