# Portico component specification

The shared spec for buttons, fields, switches, chips, segmented controls, rows, sheets and dialogs, menus, feedback and confirmations. **The numbers live in `src/components.ts`** (exported from `@portico/design` as `componentSpec`, `buttonSpec`, `rowSpec`, `overlaySpec`, …); this file gives the rules those numbers serve. The web app applies them in its `src/ui`, the Apple app in its `src/ui`.

Every value has four columns: `web` (browsers), `phone` (iPhone and phone-width web), `tablet` (iPad) and `tv` (1920×1080 ten-foot design pixels). Colors are tokens from `colors`, never literals. Tests in `packages/client-core/test/design-components.test.ts` pin the grid (every padding/gap/inset on `spacingGrid`), the minimum targets, and 4.5:1 label contrast for every interactive state on page and overlay surfaces.

## Focus (all controls)
- One focus system: a 2 px `focus` ring (TV 4), outside free-standing controls (offset +2 / TV +4), inside items that sit in a container: rows, menu items, tabs, segments (offset −2 / TV −4).
- Keyboard focus never adds a fill on top of the ring. Hover may change the fill.
- Only TV scales on focus: buttons and chips 1.05, rows 1.015, cards 1.08.

## Buttons
- **Variants:** `primary` (action blue; one per context), `secondary` (blue-slate `raisedHigh`), `ghost`, `destructive` (only inside a destructive context, e.g. a confirmation), `link`.
- **Sizes:** sm 32 (web/phone/iPad) / 64 (TV); md 40 web, 44 phone/iPad, 64 TV; lg 48 web, 52 phone/iPad, 72 TV. On touch a 32 pt `sm` button extends its hit area to 44. On TV nothing focusable is below 64 (APL-SYS-01), so `sm` and `md` share a height there.
- **One height per group.** Example: the web detail row is all `lg` (Play, Watchlist and the icon buttons).
- **Radius:** `control` (8; TV 14) at every size. `round` only for transport controls and avatars.
- **Icon-only:** square, always labeled (web: tooltip + `aria-label`; Apple: `accessibilityLabel`).
- **States:** see `button.variants`. A selected secondary is an accent tint (28%) with `text` label and an accent border. (Web's current `accentStrong` label on the tint is 4.2–4.4:1 and fails.) A pressed link keeps `accent` and underlines: `accentDeep` text fails contrast on dark surfaces.
- **Loading:** keep the geometry and the label; a spinner replaces the leading icon. Never "Saving…".
- **Hierarchy:** a page's create action is the primary in the page header. A group's create action is `secondary sm` in the group header. Filter bars never use primary.

## Fields
- Height 40 web / 44 phone / 64 TV; radius 8 (TV 14); fill `raised` (hover/focus `raisedHigh`).
- Border `lineControl` at rest (≥ 3:1, X-11), `accent` while editing, `danger` when invalid.
- Keyboard focus adds the standard ring. No glow.
- Label above (`captionStrong`, `textSecondary`, 8 gap). Help or error below (`caption`). The error has an `error` glyph and replaces the help.

## Switches
- iOS/iPadOS use the system switch tinted `accent`. Web and TV draw 44×26 (TV 56×32) with a 22 (TV 24) thumb.
- A row with a switch toggles when the row is pressed, with role `switch`.
- **The accessible name never changes with state:** "Show Trending now on Home", not "Hide…" or "Show…".
- A setting that can't be turned off shows a `lock` glyph and a caption ("Always on Home") instead of a disabled switch.

## Chips
- 32 high (TV 64; touch hit 44); pill radius; label `captionStrong`; 16 px icon.
- **Rest:** `raised` fill with a `lineSoft` border and `textSecondary` label.
- **Selected:** accent tint 28% with an accent border and a `text` label.
- **Removable chips** end with a `close` glyph, and their accessible name is "Remove {label}".
- Filter bars mix chips with ghost/secondary buttons at the chip height, 8 apart (TV 12).

## Segmented control
32 high (TV 64; touch hit 44 by extending vertically), 4 padding, radius 8 with 6 on segments. `raisedSoft` track; the selected segment is `raisedHigh` with a `text` label. Use it for 2–5 peer choices only.

## Rows and grouped lists
- **One row component with variants** (`rowSpec`):

  | Variant | Trailing element |
  |---|---|
  | `navigation` | Value (optional) plus `forward` chevron; pushes a page |
  | `value` | Value plus `chevronUpDown` (Apple) or a native select (web); opens a picker |
  | `toggle` | Switch |
  | `action` | No glyph; title in `accent`, or `danger` when destructive |
  | `menu` | `more` glyph; opens a contextual menu |
  | `static` | Nothing; not focusable |
  | `media` | Art or ordinal, progress, watched |

- **Metrics:**
  - Min height: single line 48 web / 44 phone and iPad / 64 TV; with subtitle 64 / 56 / 80; media rows 64 / 96.
  - Padding: horizontal 16 web / 12 phone / 16 iPad / 20 TV; vertical 8 (TV 12).
  - Leading slot: a 24 box (TV 32) holding a 20–22 glyph (TV 30), 12 gap (TV 20) before the text.
  - Text: title `bodyStrong`; subtitle `caption` in `textTertiary`, wrapping to 3 lines in settings rows and 1 in media rows. Settings help text must wrap.
- **Groups:**
  - One surface per group (`raised`, radius 10, TV 18, `lineSoft` border). Never a border or card per row.
  - The group draws the separators, starting where the row text starts.
  - Header uses the `label` role; the footer caption wraps. Groups sit 24 apart (TV 40).

## Sheets and dialogs (one scaffold, three kinds)
- **Kinds:**

  | Kind | Close control | Footer | Dismissal |
  |---|---|---|---|
  | `form` | No ✕ | Cancel (ghost) + one primary | Esc, scrim, swipe down and TV Menu all mean Cancel. Asks "Discard changes?" when there are edits |
  | `picker` | ✕ only | None | Choosing closes it |
  | `acknowledge` | None | One primary | Can't be dismissed (recovery codes, a new API key) |

- **Material and size:**
  - Surface: opaque `overlaySurface`. Only desktop web may use `overlaySurfaceTranslucent`, behind working blur.
  - Border: `lineStrong` at 70%. Radius 14 (TV 24). Scrim `ink` at 62% (phone 72%, TV 80%).
  - Phone uses a bottom sheet with a grabber; everything else is centered.
  - Widths sm/md/lg: 440/520/640 (TV 520/640/760). Max height 88% web, 90% phone, 82% iPad/TV.
- **Alignment:**
  - One inset for header, body and footer: 20 web and iPad, 16 phone, 28 TV.
  - The body's horizontal padding is the inset minus the row padding, so row text lines up with the title. Non-row content (notices, fields, text) adds the row padding back.
- **Header:** title uses `heading`; the optional subtitle is one line of `caption` in `textSecondary` and is context, not instructions.
- **Footer:**
  - An opaque band with a `lineSoft` hairline, 12 vertical padding (TV 20).
  - The scaffold lays out the footer itself: leading ghost action (Reset to default, Clear all) at the leading edge, a spacer, then Cancel and the primary.
  - On TV the primary comes first and takes focus. On phone web the buttons stack full width, primary on top.
- **Side panel** (`side`, the episode panel): a full-height panel on the trailing edge, 440 wide (TV 640). It is opaque, has no radius on the trailing side, and slides in 24. Phones use the bottom sheet instead.
- **Half detent** (phones, `half`): opens at 50% of the screen. Dragging the grabber up expands it to full height; dragging down collapses it, then dismisses it. The keyboard expands it.
- **Results:** a result never opens a second overlay; use a toast.
- **Hand-off:** an overlay opened as another closes waits until the first has gone (iOS drops a modal presented during a dismissal).
- **Motion:** 180 ms enter, 120 ms exit, 24 translate (sheet) or 0.98 → 1 scale (centered). Reduce Motion keeps the fade only.

## Menus
- **Panel:** opaque `overlaySurface`, radius 12 (TV 20), padding 8, width 220–320.
- **Size:** at most 12 visible rows. The web card menu puts owner actions in a Manage submenu so it never scrolls.
- **Rows:** 40 pointer / 44 touch / 64 TV; padding 12, 18 glyph, 12 gap, `body` label. Highlight is an accent tint 16%.
- **Groups and markers:**
  - Section labels use the `label` role; separators are `lineSoft`, inset 4.
  - Selected shows a trailing `check` in accent; a submenu shows a trailing `forward` glyph.
  - Destructive items are `danger`, last, and in their own group.
- **Entry actions order** (shared model; CON-24): Play · Watch Together · personal (Watchlist, Favorites, watched, like/dislike) · organize (Add to playlist…, Add to collection…, Download…) · Details · Manage (Edit metadata…, Refresh metadata, Delete…).

## Feedback
- **Notice:**
  - Tones `info` / `success` / `warning` / `error`, each with its own glyph (`info`, `success`, `warning`, `error`). Apple must stop using the warning triangle for errors.
  - Tint 12% and border 30% of the tone color; padding 12 (TV 20); radius 10.
  - Action is `secondary sm`.
  - Never an error in `info` tone.
- **StateView:** an 88 (TV) / 56 circle with the glyph, `subheading` title, `body` text, and at most two actions: the primary for recovery, a ghost for the alternative. Recovery verbs are "Try again", "Refresh", or no button while Portico retries on its own.
- **Toast:**
  - Opaque `overlaySurface`, radius 12, 4 s (8 s when it carries an action such as Undo or View).
  - Position: web bottom center; iPhone above the tab bar or mini player; TV top trailing, and not focusable unless it has an action.
- **Loading:**
  - Skeletons match the final geometry.
  - Spinners are 16 (inline), 28 (region) or 44 (screen), with an accent arc on a 35% `textTertiary` track.
  - Loading copy is "Loading {thing}", with no ellipsis and never a bare "Loading".

## Confirmations (tiers)
- **Tier 0** (reversible: remove from queue or Watchlist, archive, mark read): no dialog; an Undo toast.
- **Tier 1** (bounded loss: delete profile, remove device, revoke key, end group, sign out everywhere, delete a recording):
  - A `form` dialog; title "Verb object?"; the body states the consequence.
  - Footer: Cancel and a destructive button labeled with the verb, never "OK" or "Confirm".
  - A password step-up stays inside the same dialog.
- **Tier 2** (irreversible disk or server-wide loss: delete files, empty trash, clean up a folder, remove a library): Tier 1 plus one field labeled "Type {token} to confirm". The token is a short human word, never an ID, and it's the only instruction.
- **Non-destructive checks** (verify a backup, disconnect this browser): a normal `form` with a primary. No typed field.

## Title hero (`heroSpec`; Spec — Title Pages §1)
- **Height:** it never changes between media types on one device.
  - TV: 60% of the screen. Web and iPad: min(70% of the viewport, 720).
  - Phone: a 16:9 backdrop with the identity block below it.
- **Backdrop:**
  - Requested at `w=1920` (TV, desktop, iPad) or `w=800` (phones). Opacity 0.8 on wide layouts, 1 on phones.
  - Scrims in `ink`. Wide layouts have a bottom gradient (0→35%, 28%→0, 62%→55%, 100%→100%) and a left gradient (0→82%, 45%→30%, 75%→0). Phones have a bottom gradient only (0→0, 55%→15%, 100%→100%).
  - With no backdrop: an accent glow over a `raised` → `ink` gradient, never an empty box.
- **Identity:**
  - An eyebrow in the `label` role.
  - The title is `display` (web, iPad, TV) or `title` (phone), up to 2 lines.
  - Logo art replaces the title, at most 440 wide (phone 280, TV 640).
- **Facts line:** one line in `body`, `textSecondary`, with `·` separators in `textTertiary`, 12 apart (TV 16). The content rating is a badge 24 high (TV 36) with padding 8 (TV 12), a `small` radius and a `lineControl` border, in `captionStrong`.
- **Synopsis:** `body` in `textSecondary`, 2 lines. "More" expands it inline, or opens a text sheet on TV. Maximum width 640 on web, 720 on iPad, 960 on TV.
- **Actions:**
  - One height for the group: `lg`, or `md` on phones, where the primary is full width.
  - Secondary actions are round icon buttons that show their label on focus or hover.
  - When resuming, a 3 px progress bar sits under the primary's label.
- **Artwork beside the identity:** only on wide layouts, and only without logo art. Poster 240 wide (iPad 160); square 240 (iPad 180, TV 280).

## Play on (X-18; `playOnDestinations` in client-core presentation)
- **Rows:** AirPlay (only where the system picker exists: iPhone, iPad, Safari on macOS), Google Cast (Chromecasts), then Portico devices on this network.
- **Layout:** standard rows with a leading glyph (`airplay`, `googleCast`, and `tvDevice`, `phone`, `tablet` or `browser` for Portico devices). Portico devices sit under the heading "On this network".
- **Portico devices:** signed-in Portico players found by local-network discovery. Never a cloud or remote device list, and never a code to type.
- **Empty:** nothing found shows "No Portico devices or Chromecasts found on this network."
- **TV:** Apple TV and Android TV show no Play on entry; they are receivers.
