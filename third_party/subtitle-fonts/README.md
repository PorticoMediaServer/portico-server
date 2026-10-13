# Fallback fonts for styled subtitles

A styled subtitle names its own fonts and usually carries them. When it does not, or when a line uses a script its font lacks, the browser renderer needs a font that has those letters. The default is Liberation Sans (Latin, Greek, Cyrillic), which comes with the libass build in `third_party/libass-wasm`. The fonts here cover other scripts and are fetched only when a subtitle uses them (`web/src/player/subtitles/fallback-fonts.ts`).

| Script | Font | Licence |
| --- | --- | --- |
| Arabic | Noto Sans Arabic | SIL Open Font License 1.1 |
| Hebrew | Noto Sans Hebrew | SIL Open Font License 1.1 |
| Thai | Noto Sans Thai | SIL Open Font License 1.1 |
| Devanagari | Noto Sans Devanagari | SIL Open Font License 1.1 |
| Bengali | Noto Sans Bengali | SIL Open Font License 1.1 |
| Tamil | Noto Sans Tamil | SIL Open Font License 1.1 |
| Chinese and Japanese | Droid Sans Fallback | Apache License 2.0 |

The files are unmodified copies from the Ubuntu packages `fonts-noto-core` and `fonts-droid-fallback`; `provenance.json` records each package version and digest. Licence texts: `OFL-1.1.txt`, `Apache-2.0.txt`.

Korean is not covered: Droid Sans Fallback has no Hangul syllables and the Noto CJK font that has them is about 19 MB. A Korean styled subtitle without its own fonts shows empty boxes until a font is added here or the server offers its own fonts to the browser.
