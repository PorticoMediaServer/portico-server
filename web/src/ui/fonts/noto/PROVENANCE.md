# Noto Sans web subsets

Noto Sans by Google, SIL Open Font License 1.1 (`OFL.txt`). Copied from the Fontsource
packages `@fontsource/noto-sans-{jp,sc,tc,kr,thai,devanagari}` 5.3.x, the `japanese`,
`chinese-simplified`, `chinese-traditional`, `korean`, `thai` and `devanagari` subsets, weights
400 and 700 (`<subset>-<weight>.css`, files in `files/`).

Changes from the Fontsource output, and why:

- The `.woff` fallbacks are dropped. Every browser that reads `unicode-range` reads `.woff2`.
- A `unicode-range` is added to each `@font-face` for the script the subset serves, so a
  browser never downloads a CJK file to draw Latin, Cyrillic or Greek.

Upgrading: fetch the newer package, apply the same two changes, and check that the family names
(`Noto Sans JP`, `Noto Sans SC`, `Noto Sans TC`, `Noto Sans KR`, `Noto Sans Thai`,
`Noto Sans Devanagari`) match the stacks in `web/src/ui/base.css`.
