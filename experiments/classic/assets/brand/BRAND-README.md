# Portico — Unified Play-P Brand Set

This is the current Portico identity export suite. It preserves the complete
layout, geometry, spacing, backgrounds, and export matrix of the original
Play-P set while removing the light-blue accent from the logo.

The play triangle now uses the same colour as the surrounding mark:

- Light assets: black wordmark or icon, including a black play triangle.
- Dark assets: white wordmark or icon, including a white play triangle.
- Monochrome assets: unchanged single-ink black or white artwork.

The navy backgrounds used by dark app icons, round icons, Apple TV icons, and
top-shelf artwork are intentionally preserved.

## Usage

- Use light assets on light surfaces and dark assets on dark surfaces.
- Use the transparent monochrome wordmark and symbol files when the
  surrounding surface is controlled separately.
- Do not recolour the play triangle independently from the rest of the mark.
- Do not stretch the artwork, change its spacing, or add effects.

## Contents

```text
svg/        20 vector masters — wordmark, symbol, app, square, round, and tvOS
pdf/        12 vector PDF exports of all core variants
png/
  wordmark/ 20 transparent exports, 512–8192 px wide
  symbol/   20 transparent exports, 256–4096 px tall
  appicon/  24 light and dark exports, 16–4096 px square
  square/   24 true 90°-corner exports, 16–4096 px square
  round/    16 light and dark exports, 64–4096 px square
  appletv/  10 wordmark app-icon and top-shelf exports
favicon.ico Multi-size 16/32/48 favicon from the updated navy app icon
Play P/     Preserved original blue-accent suite
```

All text is outlined in the SVG and PDF masters, so no font installation is
required. PNGs preserve transparency where appropriate.
