# Hungarian (hu) glossary — Portico UI translation

Machine-translated pass, 18 October 2026. Every message is recorded as `machine` and needs native-speaker review.

## Address form

**Informal second person (tegezés)**, consistent with Hungarian streaming apps (Netflix HU, HBO Max HU):
imperatives as *Válaszd, Nyisd meg, Próbáld újra*; possessives as *a szervered, a fiókod, a profilod*.
Never magázás (Ön). First-person-plural system voice where the English uses "we"
(*Megtartottuk, Nem fogadtuk el*) is kept as *we* (mi), matching the source register.

## Placeholder suffixes (important Hungarian rule)

Hungarian glues case endings onto words, but a `{placeholder}` hides unknown text
(names, titles, servers), so **no suffix may ever depend on an argument's text**.
Techniques used throughout:

- Put the placeholder in nominative subject/topic position:
  `{title} már nincs meg ezen a szerveren.`, `{name} kezelheti majd…`
- Nominalise the verb instead of suffixing the object:
  `{title} lejátszása`, `{label} eltávolítása`, `{title} betöltése`
- Use a colon or dash instead of a suffixed noun:
  `Csatlakozás ide: {host}…`, `Rendező: {name}`, `Hasonló ehhez: {title}`
- Quote titles (`“{title}” nem volt áthelyezhető.`) — the quotes insulate the name.
- Generic noun carries the suffix, brand/placeholder stays bare:
  `a Portico alkalmazást`, `a Portico rendszerében`, `ehhez: {name}`

## Product names

Never translated or suffixed directly: **Portico**, **Portico Account**, **Portico Services**.
Hungarian suffixes go on a following generic noun:
*a Portico alkalmazást / alkalmazásból / alkalmazáshoz*,
*a Portico rendszerében / rendszerének / rendszerétől*,
*a Portico idejétől, a Portico appot*.
Possessives of "Portico Account" stay inside (*a Portico Accountoddal* is fine —
the validator matches the substring, but reviewers should prefer
*a Portico Accountoddal* only where natural).

## Typography

- Apostrophe: **’** (U+2019) only; ASCII `'` never appears (ICU quoting).
  Hungarian rarely needs it; most contractions were rendered uncontracted.
- Quotes: source-style **“…”** kept for quoted titles and UI strings
  (`“{title}” eltávolítva…`, `Válaszd a “Tunerek keresése a hálózaton” lehetőséget`).
- Ellipsis **…** (U+2026) as in source; **·** separators kept (`S{season} · E{episode}`).
- Units localised: durations `{hours} ó {minutes} p`, `{seconds} mp`;
  long forms *óra / perc / másodperc* (invariant after numbers, so one/other share text);
  relative time *# perce / # órája / # napja*; bitrates stay `Mbps`/`kbps`.
- Sentence case for headings and buttons, as in English.

## Core terms (English → Hungarian)

| English | Hungarian | Note |
|---|---|---|
| Home | Kezdőlap | |
| Library / Libraries | Könyvtár / Könyvtárak | media library, Plex-HU convention |
| Discover | Felfedezés | |
| Search | Keresés | |
| Settings | Beállítások | app/server config |
| Preferences | Preferenciák | personal prefs (distinct from Settings) |
| Play | Lejátszás | noun/verb nominalised |
| Resume | Folytatás | |
| Continue Watching | Folytatás | row name; Netflix-HU "Folytatás" |
| Continue Listening | Hallgatás folytatása | |
| Up Next | Következők | |
| My List | Saját listám | Netflix-HU "Saját listám" |
| Favorites | Kedvencek | |
| Watchlist term (generic) | Saját lista | where "My List" is referenced generically |
| Watch Together | Közös nézés | translated descriptively (cf. fr/de) |
| Queue | Várólista | YouTube-HU convention |
| Playlist | Lejátszási lista | |
| Collection | Gyűjtemény | |
| Channels | Csatornák | |
| Library Channels | Könyvtári csatornák | feature name, single line |
| Guide | Műsorújság | TV guide |
| Live TV | Élő tévé | |
| Record(ing) | Felvétel (noun) / Felvesz (verb imp.) | DVR |
| Tuner | Tuner | loanword |
| Downloads | Letöltések | |
| Sign in / Sign out | Bejelentkezés / Kijelentkezés | |
| Profile | Profil | |
| Account | Fiók | |
| Server | Szerver | |
| Owner | Tulajdonos | |
| Member | Tag | |
| Invitation | Meghívó | verb Meghívás avoided; *Meghívó* |
| PIN | PIN-kód | hyphenated compounds |
| Password | Jelszó | |
| Two-step verification | Kétlépcsős azonosítás | authenticator app → hitelesítőapp |
| Recovery code | Helyreállítási kód | |
| Movie / Movies | Film / Filmek | count noun *film* |
| Show (TV) | Sorozat | *Tévésorozat* in select options |
| Episode | Epizód | S/E codes kept: `S{season} · E{episode}` |
| Season | Évad | |
| Song / Album / Artist | Dal / Album / Előadó | |
| Audiobook | Hangoskönyv | |
| Cast / Crew | Szereplők / Stáb | casting-to-TV is **Átküldés**, never Szereposztás |
| Subtitles | Feliratok | *Felirat* as filter-label term |
| Audio (track) | Hang / Hangsáv | |
| Quality | Minőség | |
| Automatic | Automatikus | |
| Original (quality/file) | Eredeti | |
| Convert / Conversion | Konvertálás | server-side transcode |
| Direct play/stream | Közvetlen lejátszás / Közvetlen stream | |
| Stream | Stream | loanword kept for live/concurrent streams |
| Refresh | Frissítés | also Update → Frissítés (both conflate, standard HU) |
| Save | Mentés | |
| Cancel | Mégse | |
| Close | Bezárás | |
| Done | Kész | |
| Delete | Törlés | Remove → Eltávolítás |
| Undo | Visszavonás | |
| Retry / Try again | Újra / Próbáld újra | short vs sentence form |
| Show more / See all | Továbbiak / Összes | |
| Details | Részletek | |
| Rating | Értékelés / Besorolás | user stars → Értékelés; content rating → Besorolás |
| Watched / Unwatched | Megnézett / Megnézetlen | Played/Unplayed → Lejátszott/Lejátszatlan (music) |
| Mark as watched | Megjelölés megnézettként | nominalised (suffix-safe) |
| Notifications | Értesítések | |
| History | Előzmények | |
| Devices | Eszközök | |
| Remote access | Távoli elérés | |
| Secure connection | Biztonságos kapcsolat | |
| Backup | Mentés (noun adjunct: *biztonsági mentés* where ambiguous with Save) | record key named it *Mentés*; reviewer check |
| Scan (library) | Vizsgálat | *vizsgálja a mappát*; covers scan/rescan |
| Match (metadata) | Egyeztetés | matching → egyeztetés |
| Metadata | Metaadatok | |
| Artwork | Grafika | |
| Storage | Tárhely / Tárhely… | disk: *tárhely*; Storage&backups page: *Tárhely és mentések* |
| Maintenance | Karbantartás | |
| Logs | Naplók | |
| Tuners busy, tuner conflict | Tunerütközés | |
| Sleep timer | Elalvásidőzítő | |
| Shuffle | Keverés | |
| Repeat | Ismétlés | |
| Speed | Sebesség | |
| Buffer | Puffer | verb Pufferelés |
| Offline | Offline | loanword |
| Wi-Fi / mobile data | Wi-Fi / mobilnet | |
| HDR | HDR | untouched |
| Dolby Vision | Dolby Vision | untouched |
| Webhook / API key / Token | Webhook / API-kulcs / Token | |
| Health / Status / State | Állapot | |
| Warning / Error / Information | Figyelmeztetés / Hiba / Információ | |
| Expired | Lejárt | |
| Unavailable | Nem érhető el | Elérhetetlen only as attributive (*Elérhetetlen elem*) |
| Missing (title/file) | Hiányzik / Hiányzó | |
| Owner recovery | Helyreállítási tulajdonos | |
| Viewing hours | Megtekintési órák | |
| Storage limit | Tárhelykorlát | |
| File size / Resolution | Fájlméret / Felbontás | |
| Bitrate | Bitráta | |
| Container (media) | Tároló | tech term |
| Transcode capacity | (Konvertálási) kapacitás | |
| Simultaneous streams | Egyidejű streamek | |
| Away from home | Otthontól távol | |
| Home network | Otthoni hálózat | |
| Port forwarding/mapping | Portleképezés / Port megnyitása | |
| Certificate | Tanúsítvány | fingerprint → Ujjlenyomat |
| Reverse proxy | Fordított proxy | |
| Import limits | Importkorlátok | |
| Retention | Megőrzés | pruning → takarítás |

## Judgement calls for the reviewer

1. **Könyvtár** for Library follows Plex-HU; alternatives (*Médiatár, Tár*)
   would also work — flag if the team prefers one.
2. **Folytatás** covers Resume, Continue, Continue Watching and the
   *Resume {x} left* family; the row reference (*Eltávolítás a Folytatásból*)
   assumes the row keeps this exact name.
3. **Közös nézés** for Watch Together is descriptive; if the feature ships
   as an untranslated brand, revert to *Watch Together*.
4. **Stream** kept as loanword (streamkorlát, streamet); purist alternative
   *adatfolyam* was rejected as unidiomatic in player UI.
5. **Update and Refresh both → Frissítés.** Contexts differ
   (software update vs data reload); Hungarian apps conflate them too,
   but a reviewer may want *Aktualizálás* for Update.
6. **Backup → Mentés** collides with Save → Mentés. Where ambiguous,
   *biztonsági mentés* was used; a reviewer should check the
   backup/restore screens read clearly.
7. **Scan → Vizsgálat** (not *Szkennelés*): reads formal-technical;
   consistent across library scan, rescan interval, periodic scans.
8. **Match → Egyeztetés** for metadata matching (not *Párosítás*),
   since it covers fuzzy provider matching, not just pairing.
9. **Tegezés everywhere**, including server-console strings where some
   Hungarian enterprise software uses magázás — deliberate, matches
   streaming-app register.
10. **Plurals**: Hungarian needs only one/other; many one/other branches
    share identical text (`# cím`, `# perc`) since Hungarian nouns don't
    inflect after numbers. This is correct, not laziness.
11. **lib.count / lib.countAtLeast** pivot nouns are lowercase bare nouns
    (*# film, # sorozat*) because the count formats (`10,000+ movies`)
    need invariant forms.
12. **“{title}” stays** (not „…”) inside translations to match source
    punctuation; the one exception is builder/example prose using „…”.
13. **Server-startup and console strings** use *szolgáltatás*
    (Windows-service sense) for "service" and *Portico szolgáltatás*
    where the brand needed a suffix carrier.
14. Weak spots worth a second look: `web.metadata.*` bulk-edit strings,
    `server.routes.*` (playback-problem memory), `builder.*` channel
    conditions, and `api.mcp.*` (AI-assistant wording is new-domain HU).
