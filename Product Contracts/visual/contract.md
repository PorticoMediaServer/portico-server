# Portico Visual Contract

> Current Portico product contract. October decisions supersede the September reconciliation where noted.


**Contract ID:** `PC-VISUAL`
**Contract revision:** `portico.reconciled.1`
**Approval state:** Approved
**Applies to:** every first-party Portico consumer surface, including Web, bundled Server Web, iOS, Android, tvOS, Android TV, Fire TV, Roku, Cast, and future clients
**Does not apply to:** the public marketing site except where it deliberately consumes a Portico product component

This contract defines Portico's platform-agnostic visual and interaction law. It describes the product meaning that every client must preserve; it does not require every rendering engine to produce byte-identical pixels.

## 1. Authority and interpretation

The key words **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are normative.

1. Approved Product Contracts take precedence over historical architecture notes, screenshots, implementation-local tokens, framework defaults, and current code.
2. The current reviewed shared design-token projection owns primitive/semantic values. The preserved original tokens.json is a reference for roles and visual intent, not an instruction to revert the accepted Portico palette. Platform packages consume reviewed projections rather than independently redefining the design.
3. `../media-presentation/contract.md` defines which content and artwork belongs on media surfaces. This contract defines how those elements are visually expressed.
4. The Product Language catalog owns standard user-facing copy and semantic icon identifiers. A client MUST NOT change meaning to make copy fit.
5. Operating-system requirements and current provider-brand requirements MAY supersede product styling only for system- or provider-owned UI.
6. An intentional implementation exception MUST use the exception record in section 22. An undocumented exception is a defect.
7. Platform adaptation changes scale, input mechanics, information density, and native lifecycle. It MUST NOT create a different product character or semantic hierarchy.

This approved revision is the normative baseline. Product decisions are centralized here; clients do not invent temporary alternatives.

## 2. Product intent

### 2.1 Human and task

Portico serves a person choosing, watching, or listening to media, often from a couch, a phone held at arm's length, or a computer used for both viewing and administration. The interface must let that person recognize content, understand the current household/server/profile context, act confidently, and recover from interruptions without learning media-server internals.

### 2.2 Character

Portico MUST feel:

- cinematic without becoming ornamental;
- premium without becoming glossy;
- calm without becoming empty;
- technically capable without looking like a diagnostic console;
- consistent without forcing television, touch, and pointer interfaces into the same geometry.

Media artwork provides most of the color and emotional range. Product chrome provides a restrained theatre frame: deep navy-black, cool silver, screen blue, precise spacing, quiet boundaries, and direct language.

### 2.3 Domain and color world

The system draws from a home-theatre domain: projector room, screen light, projection falloff, shelves, rails, programme guide, signal state, transport controls, tuned channel, and a persistent player.

Its natural color world is projector navy, screen silver, graphite equipment, restrained cyan-blue status light, tuner amber, recording red, and the media's own artwork.

### 2.4 Signature

Portico's visual signature is **screen light in a persistent theatre frame**:

- a restrained blue atmosphere prevents product pages from collapsing into empty black;
- media backdrops become the dominant light source when they exist;
- navigation and known geometry remain stable while content tunes in;
- controls support the catalogue rather than decorating it.

This signature MUST be visible in authentication, empty Home, settings, profile selection, libraries, error states, and other pages without usable artwork. It MUST yield to the video itself in full-screen playback.

### 2.5 Defaults explicitly rejected

- Pure-black application pages are replaced by the Portico atmospheric canvas.
- Generic dark-dashboard cards are replaced by media-first layout plus quiet slate surfaces where containment has meaning.
- Framework-default controls are replaced by semantic Portico controls, except where a system-owned control is required.
- Decorative gradients, glass everywhere, and dramatic shadows are replaced by subtle surface shifts, translucent boundaries, and purposeful atmosphere.
- Spinner-first screens are replaced by stable final geometry, retained data, and route-shaped loading states.
- Tiny television links and hover-dependent television actions are replaced by large focusable targets and printed legal locations.

## 3. Visual surface taxonomy

Every rendered surface MUST declare one of these semantic classes. A platform can implement the declaration through a component, style, theme, or generated token, but it may not silently invent another class.

| Class | Examples | Background authority | Depth / containment |
| --- | --- | --- | --- |
| `product-root` | Home without a hero, Library, Search, Saved, Settings, Server Console, profiles, auth | Portico atmospheric canvas | Page content is open; containment only where it clarifies grouping |
| `media-led-root` | Home with hero, movie/show/album/book detail | Media backdrop over atmospheric canvas | Artwork leads; scrims preserve text and page continuity |
| `transient-overlay` | dialog, sheet, popover, filter menu, profile menu | parent remains visible under scrim | one elevation above invoker; focus-trapped where modal |
| `floating-product` | mini-player, selection bar, toast, compact queue | transparent or quiet slate material | floats without replacing page atmosphere |
| `player-canvas` | full-screen video | video plus necessary letterboxing only | no Portico page atmosphere or decorative surface |
| `player-overlay` | transport, scrubber, track/quality panels over video | transparent or mostly transparent, with local scrim and restrained blue glow | disappears when inactive; never becomes a second page |
| `provider-owned` | Google/Apple sign-in, OS permission, purchase/auth sheet | provider/system rules | Portico controls placement and surrounding hierarchy only |

## 4. Background atmosphere

### 4.1 Non-player product pages

1. A non-player product page MUST NOT use `#000000`, an unmodified platform black, or an effectively pure-black fill as its visible page background.
2. A non-player page without a usable media backdrop MUST render the Portico atmospheric canvas for the complete page, including empty and loading regions.
3. The canvas MUST begin with the accepted Portico base (`#071018`) or its reviewed equivalent, include a subtle raised/recess field (`#10212d`), and include a blue light source derived from Deep screen blue.
4. The glow MUST be perceptible but subordinate to text and artwork. It MUST NOT read as a decorative blue blob, a neon gradient, or a bright horizon.
5. The canonical wide-screen reference is an elliptical glow approximately `84%` of canvas width and `52%` of canvas height, centered horizontally with its origin slightly above the top edge, peaking near `rgba(27, 76, 108, 0.39)` and fading fully by roughly `74%` of its radius. This is the standard intensity across product roots; individual screens MUST NOT author weaker or stronger atmosphere merely to fill space.
6. Touch clients SHOULD use a narrower, slightly taller equivalent that follows the top content field. Television MAY move the source modestly toward the primary copy/action region. These are optical adaptations, not different palettes.
7. The atmosphere is an app-shell or viewport layer independent of the scrolling content layer. Content scrolls over it; the atmosphere does not move, repeat, stretch, end, or reveal black as the scroll position changes. Web implementations normally use a fixed non-interactive shell layer; native implementations normally place it outside the scrolling container. Equivalent platform techniques are conformant when the visible result is stationary.
8. Settings and Server Console MUST use the same standard atmosphere. Dense surfaces MAY cover more of it where containment improves readability, but administration MUST NOT weaken the root atmosphere or look like an unrelated application.

The tvOS activation screenshot supplied for this contract is a good expression: Projector foundation, broad blue light near the upper centre, no obvious gradient edge, and the activation ticket as the visual centre. The iOS Home screenshot without an active hero is a documented anti-example because most of the page resolves as visually empty black.

### 4.2 Media backdrop precedence

When an eligible backdrop exists and the viewer permits backdrop artwork:

1. The media backdrop MUST become the dominant background content in the hero or detail region.
2. The atmospheric canvas MUST remain underneath it so loading, crossfade, transparent pixels, image failure, and the transition into the page never expose pure black.
3. Backdrop display MUST use `cover` unless the artwork role explicitly declares another fit.
4. The crop MUST prioritize the Product Contract's focal point when supplied. Without a focal point, clients SHOULD protect faces and title-safe regions where their image system supports it and otherwise use a stable center crop.
5. A vertical scrim MUST preserve readable foreground content and fade the backdrop continuously into the page. A visible fixed-height cutoff is prohibited.
6. A horizontal scrim MAY protect left- or right-aligned hero copy. It MUST NOT flatten the entire image when localized protection is sufficient.
7. Text contrast MUST be correct for the actual crop, not merely for the base palette. A client MAY strengthen its scrim when artwork luminance is hostile.
8. Image failure MUST retain the atmosphere and fixed geometry. It MUST NOT collapse the hero, move rows, or show a broken-image glyph.
9. A new backdrop SHOULD crossfade only after decode. The last valid authorized backdrop remains visible during a compatible refresh.
10. Backdrops MUST NOT appear on privacy-sensitive surfaces when the viewer scope has not been verified.

Media backdrop precedence does not mean every screen needs a hero. A compact mobile Home page with no resume candidate may begin directly with rows; it still requires the atmospheric canvas behind the header and rows.

### 4.3 Full-screen video and letterboxing

1. `player-canvas` MUST contain only the video image and letterboxing/pillarboxing required to preserve the negotiated display aspect ratio.
2. Letterbox/pillarbox bars MAY be pure black. This is an explicit exception to the non-player background rule because black protects the authored image and perceived contrast.
3. No Portico blue atmosphere, backdrop artwork, page texture, logo, or decorative gradient may remain visible around the active video.
4. Audio-only full-screen Now Playing is not `player-canvas`; it is a media-led product surface and follows the audio presentation rules in the Media Presentation Contract.

### 4.4 Player controls, overlays, and floating menus

1. Player chrome MUST be transparent or mostly transparent over the video.
2. Controls MAY use localized edge scrims, blur where the platform can guarantee contrast and performance, and a restrained blue glow that connects them to Portico.
3. Player chrome MUST NOT cover the video with a persistent opaque blue-black page or large solid card.
4. Control opacity MUST be no stronger than needed for readability. The video remains visually primary while controls are shown.
5. Focused television controls require a clear halo/scale/fill response, but the focus treatment MUST remain local to the control.
6. Track, quality, queue, lyrics, chapter, and other selection-heavy player tools use a quiet smoked/translucent neutral surface only when direct-on-video controls would be unreadable. Handheld normally uses a bottom sheet, television normally uses an edge panel or full-height selection layer, and Web normally uses a compact anchored panel or edge panel. These surfaces use Projector/Recess/Slate with restrained opacity, blur where reliable, a soft boundary, silver text, and blue only for focus, selection, progress, toggles, and links. A bright blue panel or blue-filled menu row is prohibited. The panel occupies only the necessary region and leaves video legible where practical.
7. Inactive video chrome MUST auto-hide according to the Playback Contract. A static full-screen scrim is prohibited.
8. Floating menus outside the player use the same principle: the parent context remains perceptible; the menu is an elevated tool, not a replacement page.
9. Handheld video continuation uses the operating system's native Picture in Picture presentation. An in-app floating or docked video mini-player is prohibited.
10. A handheld audio mini-player is one exceptionally compact horizontal bar with small square artwork, bounded identity, play/pause, next, and close. It has no timeline or scrubber. Reproducing the full transport row inside this bar is prohibited. Its material is quiet smoked glass: translucent enough to admit controlled atmosphere or nearby artwork color, blurred/scrimmed enough to preserve text contrast, and bounded by a subtle neutral edge. It is neither fully transparent nor an opaque blue-black card. Controls are white/silver at rest; blue is reserved for selection, focus/press, and active state.
11. Television has no content-overlay or bottom-docked mini-player. Persistent audio is represented by a compact focusable rectangular now-playing indicator in the shell's top-right safe area; television video remains full-screen-only.
12. On a standard viewport, visible full-player controls sit over a transparent black shadow that rises smoothly from the bottom edge and fades into the video. This is localized contrast protection, not an opaque panel. The timeline, transport, and five immediate option boxes remain inside this shadow and over the video when minimum targets, safe areas, and text scaling permit.
13. Pause/Play is the only filled transport control and uses Portico action blue. Its focused/pressed state adds a crisp white outer outline. Other transport controls remain transparent and gain a circular white outline only while focused/pressed.
14. Transport order is Shuffle, Previous item, Back seconds, Pause/Play, Forward seconds, Next item, Repeat; handheld volume follows at the far right when exposed. The option boxes below are Audio, Subtitles, Quality, Speed, Sleep. Autoplay is absent from player chrome.
15. On a standard handheld or television viewport, timeline, transport, and the five option boxes SHOULD occupy no more than one third of the available viewport height. The localized bottom shadow MAY fade above that boundary but MUST NOT create the perception of a larger opaque control region. On compact height, large Dynamic Type, or constrained safe area, minimum targets, complete essential labels/actions, and reachability override this density target.
16. All five option choices are simultaneously visible on standard landscape/television geometry when they fit. On constrained handheld geometry, Audio, Subtitles, Quality, Speed, and Sleep remain reachable in canonical order through one bounded, clearly labelled utility/overflow sheet; they are never clipped or compressed below accessibility minimums.
17. Television MAY expose contracted discovery shelves below the base player controls. They are a separate, deliberately entered region and do not justify enlarging the base one-third control region. The authored video remains the visual anchor while the discovery region is entered or dismissed.
18. Handheld title/subtitle identity and the Cast, native Picture in Picture, and Close actions share one compact top header band. The identity block is left-aligned; actions are right-aligned in canonical order. Every element remains fully inside the platform safe area in portrait and landscape, including camera cutout, Dynamic Island/notch, rounded-corner, and rotation insets.
19. Visible top chrome uses a localized transparent black shadow that falls and fades from the top edge. It protects title and action contrast without becoming an opaque header bar and is absent when chrome hides.
20. On a standard viewport, the seven-action transport cluster is symmetric around Pause/Play, which remains mathematically and optically centered. A handheld-only trailing Volume control MUST NOT shift that center. On compact handheld geometry, timeline, Close, Pause/Play, and seek remain directly reachable; Previous/Next appear when applicable, and Shuffle/Repeat/other secondary controls may move to the bounded utility sheet. Every retained control preserves the contracted minimum target size.
21. Landscape option boxes or their bounded utility-sheet replacement MUST clear the bottom safe inset with complete border, label, and value visible. Clients MAY reduce internal padding modestly but MUST NOT cap Dynamic Type, clip actions, or place them partially off-screen.
22. An option popover is anchored to its invoking Audio, Subtitles, Quality, Speed, or Sleep box and clamped to the safe viewport. It MUST NOT appear as an unrelated centered modal. Opening/closing uses a restrained `120–160ms` decelerating opacity plus no more than `4–8` design pixels of translation or `0.98→1` scale. Reduced Motion removes spatial motion while preserving the state change and brief opacity transition. Dismissal restores the invoking box.

## 5. Color and semantic hierarchy

All product colors MUST resolve through the current reviewed shared design-token projection. Literal color values are permitted only inside generated projections, brand assets, media, provider-owned buttons, QR codes, video, and documented platform integration boundaries.

### 5.1 Foundations and surfaces

- **Projector** is the canvas foundation.
- **Recess** is an inset or quieter background field.
- **Slate** is a control, row, or card surface.
- **Raised slate** is hover/focus/open elevation.
- **Bright slate** is the highest ordinary product-owned surface.

Elevation in the dark theme is expressed primarily through same-hue lightness shifts and low-alpha borders. Dramatic drop shadows and mixed depth strategies are prohibited.

### 5.2 Text hierarchy

Every client MUST support at least four text roles:

| Role | Token | Use |
| --- | --- | --- |
| primary | Silver | titles, entered values, selected labels, essential body copy |
| secondary | Soft silver | descriptions and supporting copy |
| tertiary | Dim silver | metadata, timestamps, secondary facts |
| muted | Muted silver | disabled/status/supporting information that remains readable |

Placeholder text uses a dedicated control-placeholder token and MUST NOT substitute for a persistent label. Disabled text MUST remain distinguishable from tertiary metadata.

These four roles govern product copy on dark product surfaces. They do not override semantic control ink. Portico action buttons use the dark `actionInk` token on blue; provider buttons use provider-approved ink; destructive and other specifically contracted controls use their semantic text/icon token. A client MUST NOT recolor a proven control merely to force it into the silver body-text hierarchy.

### 5.3 Accent and state

- Screen blue communicates product action, active navigation, selection, verified focus/progress, and links.
- Strong screen blue is the brighter interaction state and high-emphasis link/focus color.
- Deep screen blue is the pressed state and atmospheric source.
- Healthy, tuner amber, and record/destructive colors communicate success/available, warning/attention, and destructive/recording state respectively.
- Semantic color MUST be paired with text, iconography, or shape. Color alone never communicates state.
- Blue MUST NOT become a decorative border around every field, card, and panel.

## 6. Typography

1. Portico Sans, the self-hosted/bundled Manrope family at weights `400`, `500`, `600`, and `700`, is the product typeface.
2. First-party clients MUST bundle or self-host the same approved font files. They MUST NOT use an unrestricted native system font stack for product-owned text merely because it renders more conveniently on one platform.
3. `Avenir Next`, `Avenir`, then platform sans-serif are failure fallbacks, not a design choice.
4. Synthetic bold/italic MUST be disabled where the platform allows it.
5. Headlines use weight, line height, and modest negative tracking for presence. Labels use medium or semibold weight. Body copy uses regular weight and comfortable line height.
6. Codes, time counters, channel numbers, and aligned technical data MAY use a platform-appropriate tabular or monospaced face. Ordinary titles MUST NOT.
7. Dynamic Type/font scaling and browser zoom MUST be supported. Layout adapts rather than truncating essential copy or preventing actions.
8. One-line media titles MAY ellipsize in cards; accessible names retain the complete title. Page titles, errors, legal copy, and settings descriptions MUST wrap.
9. Text MUST NOT be rasterized into ordinary UI assets.
10. Platform rasterization will not be identical. Conformance is judged by family, weight, scale, spacing, hierarchy, and readability—not pixel identity.

Starting scales use the current reviewed shared design-token projection; original tokens.json is historical reference for compact/touch, browser, and ten-foot contexts. A client MAY make optical adjustments for a platform's physical viewing distance or font metrics, but role relationships MUST remain intact.

## 7. Spacing, density, shape, and safe areas

1. The base spacing unit is `4`. Standard steps are `4`, `8`, `12`, `16`, `24`, and `32`; larger spacing composes those steps.
2. Unexplained one-off spacing values are prohibited. Optical corrections MUST be named and documented.
3. Consumer surfaces are artwork-led and comfortably spaced. Settings and Server Console use denser row composition while retaining the same type, controls, and minimum target sizes.
4. Touch targets MUST be at least `44×44` points/CSS pixels where the platform uses direct touch. Primary touch actions SHOULD be at least `48` high.
5. Television targets MUST be at least `64` design pixels high on a 1080p reference canvas, with clear space for focus growth.
6. Content MUST respect device safe areas, browser chrome, television overscan guidance, captions, and the persistent player/navigation reservations.
7. Controls use an `8` radius; ordinary content surfaces use `10`; overlays use `12–14`; artwork uses `8`. Pill geometry is reserved for transport, compact status, and intentionally segmented choices.
8. Padding is symmetric unless content hierarchy creates a clear reason for asymmetry.
9. Dense pages SHOULD use available width. Accidental narrow columns, large dead margins, or breakpoint-induced control piles are defects.

## 8. Depth, borders, and focus

1. Portico uses **surface-color shifts plus restrained translucent borders** as its depth strategy.
2. Standard, soft, strong, and focus boundaries are separate tokens. A boundary's intensity must match its semantic importance.
3. Ordinary cards and inputs MUST NOT carry dramatic shadows.
4. Keyboard and remote focus MUST be unmistakable and must not look like two nested highlights. A control receives one visual focus system: an external halo or an integrated high-contrast edge, plus an optional small scale/fill change on television.
5. A permanent border MUST NOT visually compete with the focus indicator. Focused controls MUST NOT stack a bright border, a second bright outline, and a shadow.
6. Pointer hover, focus, pressed, selected, disabled, and invalid are different states and MUST remain visually distinguishable.
7. Selected state is durable; focus is ephemeral. Moving focus MUST NOT change selection.

## 9. Product-owned actions

### 9.1 Action material

1. Product-owned page actions—including commit, continue, retry, save, back-as-a-visible-button, and other explicit action buttons—MUST use the Portico blue surface with Projector/dark text.
2. Default uses Screen blue, hover/focus may use Strong screen blue, and press uses Deep screen blue.
3. Hierarchy between primary and secondary page actions comes from placement, width, label weight, grouping, and disclosure—not arbitrary color families.
4. Context menus, player utility panels, filter lists, metadata-editor menus, and similar transient selection surfaces are not rows of page-action buttons. Their options use the quiet menu material: transparent or subtle Recess/Slate rows, Silver/Soft silver labels, soft separation, and Screen blue for focus/selection rather than a blue fill on every option. A quiet action MUST remain visibly interactive and MUST NOT look disabled.
5. Destructive confirmation uses Record/destructive emphasis only inside an explicit destructive context.
6. Icon-only actions require a semantic accessible name and a target meeting the platform minimum.
7. Loading actions preserve their geometry and label context; they MUST NOT become an unexplained spinner.

### 9.2 Text links and television

1. Touch and Web MAY use blue text links for low-risk navigation, legal documents, password recovery, and account creation.
2. A television surface MUST NOT present tiny hyperlink-style focus targets.
3. Television password recovery and account creation occur through Web/phone guidance unless a full television workflow has been explicitly contracted.
4. A television Back action shown on screen MUST use a normal large action target. The remote Back/Menu behavior remains authoritative where available.

## 10. Provider authentication and legal treatment

1. Google and Apple sign-in MUST be separate provider actions.
2. Provider actions MUST use the provider's current approved mark, wording, colors, contrast, and geometry. They are not recolored as Portico actions.
3. The current reference geometry is a matched pair at `48` high on touch/Web: Google on white with dark text and the Google mark; Apple on black with white text/mark and the provider-compliant white boundary where required for contrast.
4. Provider actions MUST have equal visual weight and width within one sign-in surface.
5. Product-owned email/password continuation follows the Portico blue action material.
6. Credential fields retain visible labels; placeholders provide examples, not identity.
7. Terms of Use and Privacy Policy appear as the final content on every sign-in/account-creation surface.
8. Touch/Web legal names are real, Screen-blue links. Television prints `getportico.tv/terms` and `getportico.tv/privacy` as non-focusable text.
9. Legal text MUST remain readable but visually subordinate to authentication actions.

## 11. Inputs and form controls

1. Product-owned fields use Slate fill, Standard line, `8` radius, Silver entered text, and the dedicated placeholder color.
2. Hover/focus may raise the fill and strengthen the edge. A field MUST NOT become a white pill on television focus.
3. Every field has a persistent visible label. Placeholder-only identification is prohibited because the label disappears during entry and is inaccessible in error review.
4. Validation appears near the field, identifies the problem in plain language, and preserves the entered value unless security requires clearing it.
5. Server addresses, usernames, codes, identifiers, and email fields MUST set appropriate capitalization, keyboard, spell-check, and autocorrect semantics.
6. A password field provides a clear show/hide action with an accessible state name where the platform permits it.
7. Choice controls use the semantic component best suited to the number and consequence of choices: compact segmented choice, radio/list choice, checkbox/toggle, or disclosure to a selection page.
8. Filters and sorts show their current selection. Apply/clear actions remain reachable, and hidden controls cannot retain focus.
9. Every enabled television text field is a deterministic remote-focus destination in visual order. Focus exposes the platform text-entry interaction; Continue/Submit MUST NOT be the only reachable control while required fields remain above it.

## 12. Navigation, rows, pivots, and selection

1. Navigation chrome grounds every product screen in current destination, server, and profile context where those contexts are relevant.
2. Primary navigation keeps stable order and semantic identifiers. It does not reorder itself based on recent use.
3. Selected navigation uses a stable combination of blue surface/edge, text contrast, and icon state. Selection is distinct from focus and hover.
4. Pivots/tabs represent peer views. They retain stable order, expose one selected state, and remain keyboard/remote navigable without activating on mere focus unless the platform convention explicitly requires it.
5. Rows are open page structures by default. A row receives a full card boundary only when it is a discrete actionable/settings object rather than a media shelf.
6. Settings rows align label, description, state/control, and validation predictably. A row MUST NOT become focusable if it has no available action.
7. Television focus movement uses semantic identities and deterministic neighbors. Loading, hidden, disabled, and virtualized-out elements are not invisible focus stops.
8. Back closes the top transient surface, then unwinds the in-app page/player hierarchy, before the app may exit at a root.

## 13. Media cards and shelves

1. Card geometry is established before artwork loads and follows the Product Contract artwork role.
2. An ordinary Home, discovery, recommendation, search, Saved, or library shelf uses exactly one card geometry and that geometry is either poster or square. Landscape cards are prohibited on these surfaces.
3. Landscape episode thumbnails are reserved for episode rows inside the containing television-show/anime detail workspace. This is a media-content rule and applies equally on phone, Web, and television clients.
4. Outside that show-detail workspace, an episode resolves to its season poster when available and otherwise its show poster. It retains episode title, number, progress, and destination semantics; inherited artwork standardizes geometry without pretending the episode is the parent entity.
5. Artwork fills its role container according to `cover`/`contain`. Transparent logos use `contain` on a quiet surface and never stretch.
6. Title and one or two decision-useful metadata lines appear outside the image when the surface calls for copy.
7. Card progress is drawn as a stable, high-contrast progress treatment and represents the real ratio; it MUST NOT exaggerate small non-zero values.
8. Watched, unavailable, selected, focused, downloaded, recording, and progress states remain distinct and accessible.
9. Pointer-only actions appear on deliberate hover or keyboard focus and clear when interaction leaves. Television actions cannot depend on hover.
10. Artwork fallback uses the correct aspect ratio, a restrained Portico surface, semantic icon/initial treatment, and an accessible “artwork unavailable” name. It MUST NOT substitute unrelated artwork.
11. One failed card or row does not blank the page. A failed row retains or releases its reserved geometry according to the continuity contract and shows recovery only when useful.

Detailed media-type composition is in `../media-presentation/contract.md` and `../media-presentation/presentation-matrix.json`.

## 14. Overlays and layering

1. Popovers, context menus, quick search, profile selectors, tooltips, selection bars, sheets, dialogs, and player panels use shared overlay primitives.
2. Overlays render above cards/backdrops and MUST NOT be clipped by an ancestor's overflow or transform.
3. An overlay computes available viewport/safe-area space, remains reachable at zoom, and restores focus to its semantic invoker when closed.
4. Modal surfaces trap focus. Non-modal popovers keep a deterministic dismiss path.
5. Layer semantics are navigation `50`, player `120`, selection `140`, popover `500`, dialog `600`, and floating `700` unless the canonical token artifact changes them.
6. A scrim communicates modality but MUST NOT erase the product context more than necessary.
7. Floating player surfaces remain above ordinary navigation/content but below blocking dialogs unless playback must be interrupted for security.

## 15. Loading, refreshing, empty, error, and offline states

The visual state grammar MUST preserve the user's context.

| State | Required presentation |
| --- | --- |
| initial loading | final route/chrome geometry plus restrained skeleton/reserved content; no blank black page |
| refreshing | successful prior content remains visible; optional subtle local progress only |
| empty content | atmosphere/media context remains; specific title, explanation, and action only when an action can help |
| empty filter result | retain filters and page; say that filters excluded content; expose clear/reset |
| offline with cached content | retain verified cached content with quiet connectivity state; do not sign out |
| offline without content | persistent product frame plus explicit offline state and automatic recovery where applicable |
| recoverable failure | preserve input/data and provide the contracted recovery behavior |
| terminal/security failure | clear affected protected content and show a specific safe next action |
| partial failure | successful rows/sections remain; failed region reports locally |

Rules:

1. Full-screen logo/spinner gates are reserved for genuinely pre-product or security transitions and MUST be bounded.
2. Skeletons match final geometry, do not shimmer continuously by default, and are not focusable.
3. Error red is reserved for actual errors/destructive state. Routine refreshing is not an error.
4. Raw transport/server exception text MUST NOT appear in consumer UI.
5. Automatic Hosted recovery surfaces MUST NOT offer a redundant `Try again` action when the system is already retrying.
6. A status message says what failed, what remains safe, and what will happen or what the person can do next.

## 16. Motion and continuity

1. Motion explains state change; it is not ambient decoration.
2. Micro-interactions use approximately `120ms`; browser route replacement approximately `160ms`; television full-page fade-through `180–220ms`.
3. Handheld forward/back navigation uses platform-standard directional push/pop. In-route content replacement uses fade-through.
4. Television true destinations and full-screen account/player surfaces use fade-through with at most `8–12` design pixels of lift or `1–2%` scale. Ordinary focus movement does not trigger page transition.
5. Artwork may crossfade after decode without moving geometry.
6. Motion is interruptible and uses a deceleration/ease-out curve. Bounce and ornamental spring motion are prohibited in ordinary product chrome.
7. Reduced Motion removes nonessential translation/scale and uses immediate change or brief opacity-only dissolve.
8. Refresh, realtime patch, pagination, and focus resolution MUST NOT remount stable chrome, restart playback, steal focus, or flash black.

## 17. Responsive and platform adaptation

### 17.1 Shared semantic invariants

All platforms preserve:

- surface class;
- information order and action meaning;
- typography family and hierarchy;
- palette and atmosphere;
- artwork role/aspect/fit;
- selected/focused/error meanings;
- loading/empty/offline grammar;
- Product Language and semantic icon identity.

### 17.2 Touch / handheld

- Bottom navigation and compact top actions MAY replace a television rail.
- Important actions remain thumb-reachable and at least `44×44`.
- Ordinary forward/back navigation follows native gestures.
- Pages MAY reduce hero height or omit an empty hero, but MUST retain atmosphere and hierarchy.
- Forms should fit common phone heights without mandatory scrolling when the content can be responsibly condensed; accessibility text scaling may require scrolling.
- Every core route, including empty, loading, no-access, and error routes, keeps the fixed atmosphere visible beneath its content. A route-owned opaque root MUST NOT mask the app-shell atmosphere.

### 17.3 Pointer / Web

- Hover is additive, never the only path to an action.
- Keyboard focus uses `:focus-visible`; pointer click does not leave an unexplained permanent halo.
- Wide screens use available space, while text measure remains readable.
- Browser font rendering differences are accepted; replacing Portico Sans with the OS font is not.
- At phone width, Web uses a deliberate mobile shell, not a scaled desktop layout.

### 17.4 Television / ten-foot

- Layout uses the platform safe area and readable ten-foot typography.
- Every action is a large focusable target; no tiny links.
- Focus growth has reserved space and cannot crop neighboring content.
- Remote focus and Back are deterministic and semantically restored.
- Legal destinations are printed, not fake focusable links.
- QR and activation codes are readable from distance, separated from surrounding copy, and retain stable display during background renewal.
- A mobile playback-destination sheet presents Google Cast, AirPlay, and a permanent full-size **Enter a TV Code** action as peers. The TV-code action remains operable with zero nearby results. When local TVs exist, the same action may show a restrained nearby indicator and disclosure control; its expanded nearby list is subordinate to manual entry. Empty discovery status is helper text, never a replacement for the action.
- Forms expose every enabled field and action through deterministic remote focus in reading order; an input cannot be visually present but unreachable.

### 17.5 Constrained clients

Roku, Cast, and future constrained renderers MAY reduce blur, transparency, animation, and simultaneous visible content for performance. They MUST preserve semantic hierarchy, color meaning, atmosphere, focus, artwork roles, and state grammar.

## 18. Accessibility

1. Normal text and essential icons MUST meet WCAG AA contrast against the actual approved surface or artwork crop. Large text follows the applicable large-text threshold.
2. Focus is visible, singular, and not communicated by color alone.
3. Controls have semantic roles, accessible names, states, and logical traversal.
4. Content order remains meaningful without visual position.
5. Text scaling/zoom MUST NOT hide essential actions, create horizontal page scrolling, or make dialogs unreachable.
6. Reduced Motion, increased contrast, captions/subtitles, screen readers, and television audio guidance are supported according to platform capability.
7. Artwork has contextual accessible names; decorative backdrops are hidden from accessibility APIs.
8. QR codes are accompanied by the human URL and code. The QR image alone is never the only authorization path.
9. Error announcements occur when state changes, without repeatedly announcing automatic retry loops.
10. Focus does not move because data loaded, a row refreshed, or artwork decoded.

## 19. Icons and brand assets

1. Product-owned ordinary icons are selected by canonical semantic ID from the approved Lucide-derived registry.
2. A platform adapter may alter stroke rendering for rasterization/scale but not glyph identity or meaning.
3. Product code MUST NOT import an arbitrary alternate icon because it is convenient.
4. An unknown semantic ID fails visibly in development/test and becomes unavailable in production; it MUST NOT fall back to a plausible but false symbol.
5. Portico symbols and wordmarks use approved brand masters/exports and remain separate from ordinary icons.
6. Brand marks are not rebuilt with text or approximated geometry.

## 20. Conformance model

Every client implementation MUST demonstrate:

### 20.1 Static/token conformance

- all semantic colors, type roles, spacing, radii, layers, and motion values resolve from the current reviewed shared design-token projection;
- no product page introduces pure black outside an allowlisted player/brand/media boundary;
- provider buttons and brand assets use approved resources;
- raw colors, icon paths, and product copy are linted where practical.

### 20.2 State conformance

Reference fixtures MUST cover at minimum:

- signed out, email sign-in, account creation, and direct server sign-in;
- Home with a media hero and Home with no hero/history;
- Library poster and square presentations, plus show-detail episode landscape rows;
- detail with backdrop, missing backdrop, and failed artwork;
- movie detail with complete decision region and cast/extras/related continuation;
- show detail with season/episode workspace before exploratory rows;
- handheld and television detail density using the same semantic order;
- initial loading, retained refresh, empty, filtered empty, offline, partial failure, permission denial, and security-blocked;
- focused, selected, pressed, disabled, invalid, and loading controls;
- profile selection, server selection, Settings, and Server Console;
- full-screen video with hidden and visible player chrome;
- long product pages at the top, middle, and end of scroll, proving that the atmospheric layer remains fixed and never reveals black;
- elastic/overscroll, browser resize and rotation, safe-area changes, split-screen where supported, and browser chrome expansion/collapse;
- media-led roots while backdrop artwork decodes, fails, is replaced, and disappears after a viewer/server scope change;
- player overlays during auto-hide, paused, seeking, track/quality selection, interruption, and return-to-content states;
- reduced motion, large text/browser zoom, and high-contrast/focus states.

### 20.3 Platform evidence

- Web: automated semantic/style assertions plus screenshots at representative desktop, laptop, tablet, and phone widths.
- Handheld: screenshot tests or deterministic fixture screens plus simulator/device review at smallest and common supported sizes.
- Television: 1080p reference screenshots, HD downscale/overscan validation, remote traversal, and focus restoration evidence.
- Constrained clients: packaged-asset/token validation plus hardware review where emulator rendering is not authoritative.

Pixel-perfect equality across platforms is not required. Unexplained semantic, hierarchy, palette, artwork-role, state, or interaction differences fail conformance.

## 21. Container and recording artwork decisions

1. A playlist or collection carries a server-declared `presentationFamily`. Music containers use square art; video and audiobook containers use poster art; a genuinely mixed container uses poster art. Collections normally inherit their library family. The server updates this declaration with membership revisions; clients do not inspect a sample of children and guess.
2. Recorded television follows episodic poster inheritance outside its programme/show detail context. A non-episodic one-off recording uses accepted programme poster art when available. If only landscape programme art exists, the server may generate and persist a focal-safe poster derivative; clients MUST NOT crop it independently. With no usable art, the deterministic poster fallback applies.

## 22. Explicit exception policy

An exception is appropriate only when a platform requirement or narrow product decision makes the canonical rule materially worse. General inconsistency, convenience, framework defaults, and migration cost are not reasons.

Use the current governance exception form:

```text
PORTICO-CONTRACT-EXCEPTION
Contract: PC-VISUAL#<section>
Scope: <platform, component and state>
Decision: <exact deviation>
Reason: <user or platform outcome>
Approved-by: <recorded product decision>
Review-when: <objective trigger>
Evidence: <boundary fixture>
```

The implementation's conformance allowlist MUST point to that comment. If the exception represents a generally better product rule, update this contract and all clients instead of multiplying exceptions.

## 23. Implementation enforcement sequence

Implementations enforcing this contract SHOULD:

1. generate platform token projections from the canonical artifacts;
2. classify every top-level surface;
3. replace pure-black non-player roots with atmosphere;
4. reconcile backdrop and player exceptions;
5. replace local component styling with semantic primitives;
6. add state and screenshot conformance fixtures;
7. record only genuine approved exceptions.


## Reconciled architecture boundary

The applicable section of [Domain architecture and failure boundaries](../architecture-amendments.md) completes this contract. It resolves named implementation conflicts; original UX and product choices remain as recorded in [decisions.md](../decisions.md). Old schema/fixture references are historical inputs to be ported, as defined by the current registry, not a second rebuilt API authority.

## October scope and evidence

Retain the reviewed dark Manrope/blue design. No plugins, themes, custom CSS or light theme. Browser/device/focus/large-text checks remain unverified; visual contract tables are requirements, not acceptance records.

Sources: [web-app](../../../_program/reports/web-app.md), [native-app](../../../_program/reports/native-app.md), [native-sync](../../../_program/reports/native-sync.md). [Owner decisions](../decisions.md) govern. Each existing conformance matrix is required coverage, not proof of execution. Release parity is required for applicable first-party features; truly unsupported platform capabilities are hidden. Roku, CarPlay, Android Auto and light theme are outside this programme. No consumer push or email. No stable API or old-database compatibility is promised before release.
