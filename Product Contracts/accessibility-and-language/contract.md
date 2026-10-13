# Accessibility and Language Contract

> Current Portico product contract. October decisions supersede the September reconciliation where noted.


Contract ID: `PC-ACCESSIBILITY-LANGUAGE`  
Revision: `portico.october.1`  
Status: **approved**  
Owners: Product, Accessibility, Product Language, Client Foundations  
Imports: `PC-FOUNDATION` viewer scope and canonical semantics; `PC-GOVERNANCE` lifecycle/exception law; `PC-GLOSSARY` vocabulary  
Coordinates with: `PC-VISUAL`, `PC-MEDIA-PRESENTATION`, `PC-NAVIGATION-INPUT`, `PC-IDENTITY`, `PC-PLAYBACK`  
Applies to: Hosted and Server user-facing responses, Product Language, Web, bundled Server Web, Client Core, iOS, Android, tvOS, Android TV, Fire TV, Roku, Cast, notifications, email, and future first-party clients

## A11Y-1. Purpose and baseline

Accessibility and language are product behavior, not post-release polish. A feature is incomplete if a person cannot perceive its state, reach its actions, understand its language, or use it with the input and assistive technology expected on that platform.

This contract owns:

- semantic structure, names, roles, state, focus, and assistive announcements;
- screen-reader, VoiceOver, TalkBack, Audio Guide, keyboard, switch, and voice-control outcomes;
- text scaling, browser zoom/reflow, contrast, reduced motion, and non-color cues;
- captions, SDH/CC, forced subtitles, audio description, and accessibility preferences;
- UI locale, Product Language, message/icon IDs, interpolation, pluralization, formatting, RTL, and fallback;
- consumer vs administrative language, legal copy, errors, and privacy-safe accessible text.

Web and Web-derived surfaces MUST conform to WCAG 2.2 Level AA. Native and television clients MUST meet the equivalent platform accessibility guidance and MUST NOT provide a weaker outcome than WCAG 2.2 AA where the criterion applies meaningfully. Portico SHOULD meet WCAG AAA focus appearance and enhanced target guidance when doing so does not contradict a platform standard.

## A11Y-2. Non-goals and separations

- Pixel-identical text rasterization or focus effects across platforms are not required.
- Accessibility is not satisfied by adding labels to otherwise unreachable or unstable controls.
- UI locale, metadata language, preferred audio language, subtitle language/mode, and server library metadata language are separate settings.
- Screen-reader audio guidance, media audio description, spoken subtitles, and ordinary captions are separate features.
- Product Language does not translate user media titles, usernames, server names, file paths, or provider-supplied metadata unless an owning metadata contract supplies localized variants.
- Raw server prose is not a localization source.

There are no unresolved baseline decisions in this revision. Adding a locale is an implementation/release decision governed by the same rules, not a new product contract.

## A11Y-3. Semantic product model

1. Every interactive control MUST expose an accurate role, accessible name, current value/state, disabled/expanded/selected/checked status, and relationship to controlled content where applicable.
2. The accessible name MUST contain the visible label text. Icon-only controls use the canonical semantic icon/message label; a filename, glyph name, or implementation component name is prohibited.
3. Headings, lists, grids, rows, tables, landmarks, tabs, dialogs, alerts, progress, and form fields MUST use the platform's real semantics rather than visual imitation.
4. A clickable container MUST NOT contain competing nested click targets unless the platform semantics and focus order clearly separate them.
5. Static text MUST NOT be made focusable merely to make it readable. Assistive technology reads it through the platform's content navigation.
6. Decorative artwork, glows, separators, and duplicate icons are hidden from the accessibility tree. Informative artwork gets a concise purpose, not an exhaustive visual description, unless its content is necessary to complete the task.
7. Media titles exposed visually in truncated form retain the complete title in their accessible name.
8. Loading reservations and skeletons are not controls and are hidden from focus. The region's busy state and meaningful result are announced without reading every placeholder.
9. A disabled control remains discoverable only when knowing its existence/reason is useful. Otherwise it is omitted consistently from both visible and semantic interaction order.
10. Semantic order MUST match the logical reading and interaction order even when visual layout uses columns, grids, portals, or transformed television surfaces.

### A11Y-3.1 Forms, validation, and authentication

- Every field retains a visible localized label. Placeholder text is an example or hint, never the only label.
- Required, optional, help, unit, formatting, and validation semantics are programmatically associated with the field.
- Validation identifies the field, describes how to correct it, preserves safe entered data, and moves/announces focus only when needed to complete the task.
- Errors are not communicated by red color alone and are not cleared merely because the field loses focus.
- Password managers, paste, autofill, one-time-code APIs, and system/provider authentication remain available. Portico MUST NOT require memorization, transcription, or a cognitive puzzle where a standard authenticated path exists.
- Show/hide password is a real toggle with current state. It does not change the password value, move focus unexpectedly, or speak the secret.
- A multi-step form exposes step name/position and preserves completed non-secret fields when a recoverable interruption occurs.

## A11Y-4. Focus and navigation accessibility

`PC-NAVIGATION-INPUT` owns route and focus outcomes. This contract adds assistive behavior:

1. Keyboard/remote focus is always visible and never fully obscured by sticky chrome, overlays, safe-area clipping, or scroll containers.
2. Focus order follows task and reading order. CSS/native visual reordering MUST NOT create a different semantic order.
3. A route change announces the localized destination once after the verified destination becomes current. Background refresh, focus/blur noise, and unchanged route parameters MUST NOT repeat it.
4. Opening a modal moves focus inside and announces its localized title/purpose. A modal that makes its background inert simultaneously exposes a visible reachable Close/Cancel action, platform Back/Menu dismissal, and accessibility escape where supported. Closing normally lets the native platform restore its invoker; an explicit semantic restore is used only when required and still fenced to an eligible invoker or documented workflow fallback.
5. A new error is announced once with its next truthful action. Repeated automatic retries MUST NOT repeatedly announce the same failure.
6. Live progress announcements are throttled. Playback time, scan progress, countdowns, and download progress MUST NOT be announced every update unless the user explicitly requested continuous feedback.
7. When a focused item disappears, deterministic focus fallback is announced once. Reorder alone produces no announcement.
8. Screen-reader virtual cursor and keyboard focus MUST NOT be trapped outside an active modal or player layer.
9. Custom gestures, swipe actions, drag/reorder, long press, hover, and remote-only Options actions require a visible/semantic alternative.
10. Focus restoration and route announcements are fenced by viewer scope so a prior profile's media is never spoken after a transition.
11. Web provides a first-focusable **Skip to content** link that moves focus past persistent application chrome to the current destination's main region.
12. Television player chrome MAY transfer remote focus to one route-scoped player-stage interaction target before hiding. That target exposes **Show playback controls** to audio guidance and is not a reusable invisible focus sink. Hidden chrome descendants are not focusable or traversable. Screen-reader interaction keeps player chrome visible.

## A11Y-5. Screen readers, audio guidance, and voice control

- iOS/tvOS use VoiceOver-native labels, traits, values, actions, grouping, and screen-change notifications.
- Android/Android TV use TalkBack-native semantics, traversal, state descriptions, custom actions only when standard actions are insufficient, and pane/window announcements.
- Roku supports Audio Guide through SceneGraph semantics and must receive real hardware review before claiming conformance.
- Web uses semantic HTML first; ARIA supplements missing semantics and never replaces working native controls.
- Voice Control/speech users can name a visible action using words contained in its accessible name. Duplicate visible labels within one scope require disambiguating context.
- Television audio guidance MUST identify focused item, selected state, disabled state/reason where useful, position in collection when stable, and available primary action without exposing raw IDs.
- A temporary human activation code MAY be spoken in its visible groups with pauses between characters and groups because it is intentionally displayed. Hidden poll/device secrets, credentials, emails, tokens, and internal IDs are never spoken.
- Audio guidance MUST NOT compete with media by continuously narrating passive background changes. Player state announcements are limited to user-initiated play/pause, track/quality change, error, and other material transitions.

## A11Y-6. Target size and motor access

1. Touch controls MUST have at least a `44×44` point/CSS-pixel activation area; `48×48` is preferred for primary actions. Visual glyphs may be smaller inside that area.
2. Television focus targets MUST be at least `64` design pixels high on a 1080p reference canvas and preserve room for focus growth. Apple system-owned tvOS controls may use the platform minimum when larger geometry is unavailable.
3. Web pointer targets MUST meet WCAG 2.2 `2.5.8` minimum or use sufficient spacing. Frequently used and destructive targets SHOULD exceed the minimum.
4. Adjacent destructive and safe actions require enough spacing to reduce accidental activation.
5. Dragging, multi-finger, path-based, motion, and timed input always has a simple activation alternative.
6. Controls do not require precision hover, rapid repeated taps, or held buttons except when the action is inherently continuous; a held remote key never causes repeated destructive activation.
7. Time limits under Portico control are adjustable, extendable, or absent unless the security protocol requires expiry. Security expiry is explained and restartable without losing unrelated state.

## A11Y-7. Text scaling, zoom, and reflow

1. Product-owned text uses real text and honors the platform's text-size/accessibility setting. Arbitrary application caps that defeat the user's requested size are prohibited.
2. Web supports text zoom to `200%` without lost content/functionality and reflows at a `320 CSS px` viewport equivalent except where a genuine two-dimensional data view, such as an EPG grid, requires two-axis navigation.
3. Native layouts adapt to Dynamic Type/font scale through wrapping, vertical growth, scrolling, and alternate compact arrangements. Essential actions, errors, legal copy, and settings descriptions MUST NOT be clipped or ellipsized.
4. Media-card titles MAY use a bounded number of lines/ellipsis when complete text remains accessible and a detail surface is reachable.
5. Fixed bars, mini-player, rail, keyboard, safe areas, and captions MUST NOT obscure the currently focused control at large text sizes.
6. Television clients SHOULD provide a Portico text-size preference when the platform does not expose reliable system scaling. That preference is device-local and cannot reduce below the canonical readable baseline.
7. Layout changes at large text are not a visual contract exception. They are required platform adaptation.

## A11Y-8. Contrast, color, images, and flashing

1. Normal text meets at least `4.5:1`; large text meets `3:1`; active control boundaries, essential icons, selection indicators, and focus indicators meet `3:1` against adjacent colors.
2. Contrast is measured against the actual rendered backdrop/crop/scrim, not the nominal base token. Artwork-led text receives adaptive scrim or relocation.
3. Disabled/inactive states may be lower contrast only when they are not required to understand or complete the task. Explanatory text remains readable.
4. Color is never the only indicator for focused, selected, watched, recording, warning, error, success, live, or destructive state. Shape, icon, text, edge, or pattern accompanies it.
5. Focus remains distinct from selection and hover at required contrast.
6. Product pages use the atmospheric/backdrop rules in `PC-VISUAL`; the glow cannot lower text/control contrast.
7. Video and animated artwork MUST NOT flash more than accessibility thresholds. Portico-owned animation MUST NOT create three flashes per second or large high-contrast flashes.
8. User-generated/media artwork is not altered for aesthetic contrast except through reversible overlays/crops; foreground UI remains accessible regardless of the artwork.

## A11Y-9. Reduced motion and sensory alternatives

- Portico automatically honors the operating-system/browser Reduced Motion preference without requiring a second setting.
- Any Portico-local Reduce Motion setting may strengthen but never override an enabled system preference.
- Reduced Motion removes parallax, large translation, simulated depth travel, spin, pulsing, auto-pan, and nonessential scale. Route changes are immediate or brief opacity-only dissolves.
- Function MUST NOT depend on animation direction, color, sound, haptics, or spatial position alone.
- Autoplaying decorative/ambient movement is prohibited. Media previews that move require an explicit user preference and a pause/stop path.
- Haptics and sound MAY confirm an action but cannot be the only confirmation.
- System accessibility preferences are re-read on change where the platform supports it; relaunch is not required for ordinary preference changes.

## A11Y-10. Captions, subtitles, audio description, and media accessibility

`PC-PLAYBACK` owns discovery, selection, conversion, timing, and delivery of tracks. This contract owns their accessible identity and controls.

1. Track choices expose localized language/display name, kind (`subtitles`, `CC`, `SDH`, `forced`), commentary/descriptive purpose, default status, and selected state where known.
2. `None`/`Off`, automatic selection, forced subtitles, and manually selected tracks are distinct choices. A manual choice is respected as defined by the Playback Contract.
3. Audio-description tracks are explicitly labeled **Audio Description** (localized), not merely another language duplicate. Descriptive and commentary audio are distinct.
4. Captions and subtitles can be toggled and changed without leaving playback. Controls remain operable with screen reader, keyboard, remote, and switch access.
5. Caption appearance respects platform/user preferences where available. Portico MAY expose font size, text color, edge, and background choices when the player permits them, but MUST preserve readability and never hide essential content behind controls.
6. Burned-in/open captions are identified as non-toggleable when known. Portico does not pretend they can be turned off.
7. Subtitle/caption timing errors present recovery without exposing codec internals on consumer surfaces.
8. Audio-only lyrics are synchronized text, not a substitute for captions. Lyrics reading order and time highlighting remain accessible without forcing continuous announcements.
9. Spoken subtitles are a separate assistive feature and are never inferred from screen-reader enablement alone.
10. A media item lacking captions or audio description is described factually. Portico MUST NOT label it inaccessible in a way that blames the user or imply a missing track can be generated when it cannot.

## A11Y-11. Locale model and ownership

Portico maintains separate locale/language facts:

| Fact | Authority | Scope |
| --- | --- | --- |
| UI locale | explicit viewer/device setting, otherwise platform locale | viewer + device/platform; may be synced as a viewer preference when supported |
| Product Language catalogue | canonical Product Contract source and generated projections | build/contract revision |
| Metadata display locale | library/provider metadata contract | server/library plus viewer preference where supported |
| Audio/subtitle preference | viewer playback settings | viewer/profile |
| Server administration locale | current administrator UI locale | administrator session/device; never changes other viewers' UI |
| Logs/diagnostics | structured stable codes with operator-selected rendering | operator only |

Changing UI locale updates product copy, formatting, direction, semantic labels, and route announcements atomically. It MUST NOT change selected server, profile, audio track, subtitle track, metadata provider language, or media title silently.

## A11Y-12. Product Language authority

1. Stable message IDs and semantic icon IDs are the only ordinary cross-platform language/graphic identifiers. Client code MUST NOT branch on English copy.
2. The canonical Product Language catalogue owns action labels, navigation names, errors, empty/loading/status copy, recovery actions, tone, semantic icon ID, interpolation variables, and translator context.
3. Hosted/Server responses emit stable codes and optional message IDs plus structured safe variables. First-party clients resolve them through the catalogue. Raw `detail`, exception text, HTTP reason phrases, or transport strings are diagnostic evidence and MUST NOT become product copy.
4. Unknown message IDs/codes resolve to the safe localized generic state for the operation class. They never display an untranslated raw server message.
5. Icon IDs map to one canonical semantic meaning. A missing icon mapping fails visibly in development/test and becomes unavailable in production; it MUST NOT fall back to a plausible but false glyph.
6. Visible label and accessible label SHOULD use the same message ID. A separate accessible message is permitted only when visible brevity loses required context.
7. Messages use named variables with declared type and privacy classification. Positional interpolation, implicit stringification, and unbounded server strings are prohibited.
8. Sentences are translated as complete units. Concatenating fragments, inserting prepositions, manually appending plurals, or constructing grammar from action labels is prohibited.
9. Every catalogue revision is schema-validated, deterministically generated, and drift-checked in Client Core, Web, native clients, Roku, Hosted, and Server projections.
10. Product Language revisions that remove or change ID meaning require a coordinated current-product cutover, not silent per-client aliases.
11. Every client bundles the current `en-US` fallback and SHOULD bundle or durably cache its supported active locales. Product Language availability MUST NOT depend on a reachable server during sign-in, offline use, or server recovery.

## A11Y-13. Localization, plurals, and formatting

- Locale identifiers use canonical BCP 47 form.
- Plural and select logic uses CLDR/ICU-compatible categories (`zero`, `one`, `two`, `few`, `many`, `other`) as required by each locale. English singular/plural branching is not a portable rule.
- Dates, times, durations, relative time, numbers, percentages, byte sizes, bitrates, currencies, ordinals, and lists use locale-aware formatters. Hand-authored separators, month names, decimal symbols, and 12/24-hour assumptions are prohibited.
- Media technical values keep their standard unit identity but localize spacing, number format, and surrounding explanation.
- File extensions, codecs, model identifiers, URLs, device codes, channel call signs, and other literal technical tokens are isolated from surrounding grammar and not translated.
- Variables include translator context and examples. A variable's grammatical role is not changed without updating translations.
- Sorting/searching follows locale-aware collation where the product promises alphabetical order; stable server-defined order and media sort-title rules remain authoritative where specified.
- User-generated strings are displayed verbatim, Unicode-safe, bounded, escaped, and directionally isolated.

## A11Y-14. Fallback and missing translations

1. Locale resolution order is exact requested locale → approved parent locale → `en-US`.
2. An unsupported locale does not produce blank labels, raw IDs, broken layout, or a partially loaded application.
3. Production MAY fall back per message to `en-US`; the current locale and fallback use the same semantic ID and variable schema. A fallback is observable in diagnostics without exposing user content.
4. Development and release gates fail on missing required `en-US`, mismatched variables, invalid plural categories, orphaned IDs, missing accessible labels, or catalogue/schema drift.
5. A locale is declared supported only when critical identity, onboarding, navigation, playback, settings, error, legal, and accessibility copy is complete and reviewed in context.
6. Legal documents may have a more limited locale set. The UI clearly opens the available authoritative version and does not machine-translate legal text.

## A11Y-15. Right-to-left and bidirectional content

1. RTL locales mirror leading/trailing product layout, navigation rail, reading order, aligned text, drawers, ordinary directional transitions, and semantic Back/forward chevrons.
2. Media time remains chronological according to platform media convention; play, pause, volume, Cast, brand marks, and non-directional media icons do not mirror.
3. Previous/next, rewind/forward, skip, and chapter controls follow semantic media-time direction rather than blindly mirroring glyph assets. Their labels remain explicit.
4. URLs, email addresses, file paths, device codes, IP addresses, codecs, timestamps intended as technical tokens, and opaque IDs use directional isolation. They do not force direction on the containing sentence.
5. Unknown-direction user/provider content uses first-strong/automatic isolation at the smallest useful block.
6. Cropping and artwork focal points do not mirror. Decorative direction may mirror only when meaning is preserved.
7. Keyboard/remote physical left/right movement follows the visual layout; semantic next/previous remains attached to the labeled action.
8. Every supported RTL locale receives screenshot, keyboard, screen-reader, truncation, and mixed LTR/RTL content review.

## A11Y-16. Consumer, administrative, error, and legal language

### A11Y-16.1 Consumer language

Consumer copy is concise, direct, calm, and media-oriented. It states what happened, what remains safe, and the next truthful action. It avoids HTTP status, route, auth, codec, scanner, database, worker, lease, grant, and exception terminology unless the user opened an explicit technical surface.

### A11Y-16.2 Administrative language

Server Console and diagnostics may use technical terms necessary for informed administration, but each term has clear context and does not substitute raw logs for UI. Destructive actions name the exact scope and consequence. Progress and long-running jobs state whether playback remains prioritized.

### A11Y-16.3 Errors

- Error title identifies the failed user task, not the failing implementation.
- Body explains recoverability and retained state.
- Actions are truthful. **Try again** appears only when user action can usefully retry; automatic retry says Portico will continue automatically.
- Device-offline, server-offline, Hosted-unavailable, authorization, permission, rate-limit, unsupported-version, validation, and unknown failures remain distinct stable categories.
- Unknown errors do not blame the user's connection without evidence.
- Repeated errors are deduplicated and do not steal focus.

### A11Y-16.4 Legal

Terms of Use and Privacy Policy are the final content on sign-in and account-creation pages. Touch/Web uses real localized link labels. Television prints `getportico.tv/terms` and `getportico.tv/privacy` as non-focusable text unless the platform has a complete trustworthy browser handoff. Consent language and legal version are owned centrally and cannot be paraphrased per client.

## A11Y-17. Privacy and security

- Accessible names, announcements, notifications, emails, route labels, and telemetry MUST NOT expose passwords, PINs, contact email where username suffices, tokens, grants, private paths, hidden provider addresses, or prior-viewer data.
- Password managers, paste, autofill, and accessible authentication are supported. Cognitive-function tests or forced transcription are not used when a standard authentication option exists.
- Password visibility controls announce current state and action without speaking the password.
- Sensitive notifications use privacy-safe lock-screen copy by default.
- Screen capture/accessibility tooling is not treated as hostile, but secrets not intended for display are never placed in visual or semantic trees.
- User content inserted into localized messages is escaped, bounded, directionally isolated, and never interpreted as markup.

## A11Y-18. Platform adaptation

| Platform | Required adaptation | Prohibited weakening |
| --- | --- | --- |
| Web/Server Web | semantic HTML, browser zoom/reflow, keyboard, landmarks, live-region restraint | div-only controls, hidden focus, ARIA replacing native semantics |
| iOS/iPadOS/tvOS | VoiceOver, Dynamic Type where applicable, Reduce Motion, native traits/actions/focus | fixed text that clips, unannounced modal, focus moved by refresh |
| Android/Android TV | TalkBack, font scale, system Back, D-pad traversal, Reduce Motion | content descriptions as raw IDs, unreachable actions, duplicate spoken labels |
| Roku | SceneGraph accessibility, Audio Guide labels/state, remote-only complete path | visible but unfocusable actions, coordinate-only semantics, unverified conformance claim |
| Cast | caption/track semantics on receiver and accessible controller | receiver-only controls inaccessible from controller |
| Email/notifications | semantic structure, plain-text equivalent, localized safe copy | image-only content, secret-bearing lock-screen text |

Native controls MAY render their standard focus or announcement behavior. Platform adaptation MUST preserve the canonical message/icon meaning and user outcome.

## A11Y-19. Conformance and release evidence

`conformance.json` defines minimum platform-neutral cases. Conformance also requires:

- automated WCAG checks plus manual keyboard, zoom, contrast, screen-reader, and modal review on Web;
- VoiceOver on iOS and tvOS, TalkBack on Android and Android TV, and Audio Guide on Roku hardware;
- text-size extremes, 200% Web zoom, 320px reflow, safe-area/keyboard/caption collision checks;
- reduced-motion behavior captured at route, overlay, focus, hero, and player surfaces;
- caption, SDH/CC, forced subtitle, audio-description, lyrics, and track-label vectors;
- exact/parent/`en-US` fallback, plural categories, variable mismatch, locale formatting, and RTL mixed-content tests;
- Product Language schema/generation/drift gates and raw-error-copy scans;
- accessibility tree inspection for loading, empty, offline, denied, error, and recovery states;
- privacy review of spoken/visible text across viewer transitions.

Minimum applicability:

| Rule family | Hosted/Server | Web | Client Core | Apple | Android | Roku | Cast |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Stable messages/icons | source/projection | required | authority adapter | required | required | required | required |
| Semantic controls/focus | response semantics | required | fixtures | required | required | required | controller/receiver |
| Scaling/contrast/motion | copy-safe | required | tokens/fixtures | required | required | required | required |
| Captions/audio description | track facts | required | normalized facts | required | required | required | required |
| Localization/RTL | source facts | required | catalogue adapter | required | required | required | required |
| Assistive hardware | not applicable | browsers | outcomes | VoiceOver | TalkBack | Audio Guide | controller-dependent |

No client may claim accessibility conformance solely from unit tests or screenshots. At least one real assistive-technology interaction pass is required per release family.

## A11Y-20. Contract exceptions

An exception requires product-owner approval, the closest durable code comment, an exception-registry entry, and a test proving its boundary. The exact comment is:

```text
PORTICO-CONTRACT-EXCEPTION
Contract: PC-ACCESSIBILITY-LANGUAGE#<section-id>
Scope: <platform, surface, or capability>
Decision: <the exact deviation>
Reason: <why the canonical rule cannot or should not apply>
Approved-by: <name or recorded decision reference>
Review-when: <expiry date, capability change, or other objective trigger>
Evidence: <test or fixture that prevents accidental expansion>
```

An accessibility exception cannot be justified only by schedule, implementation difficulty, aesthetics, or an inaccessible upstream default. If a product issue affects more than one client, update this contract and align every client.

## A11Y-21. Comparative precedent and standards

Plex and Jellyfin lessons and rejected precedents are recorded in [`precedent-review.md`](../../../References/Contract%20Baseline%202026-09-05/accessibility-and-language/precedent-review.md). Normative external baselines are WCAG 2.2, platform accessibility guidance, WAI semantic patterns, and Unicode CLDR/LDML. External guidance does not override a stronger Portico guarantee.


## Reconciled architecture boundary

The applicable section of [Domain architecture and failure boundaries](../architecture-amendments.md) completes this contract. It resolves named implementation conflicts; original UX and product choices remain as recorded in [decisions.md](../decisions.md). Old schema/fixture references are historical inputs to be ported, as defined by the current registry, not a second rebuilt API authority.

## October scope and evidence

Native player caption size/background now have source implementation. Device-drawn ASS/image paths retain declared font and PiP/Cast limits. Screen-reader, large text, keyboard and TV-focus acceptance remain open; a source change does not mark the complete subtitle/accessibility contract met.

Sources: [native-playback](../../../_program/reports/native-playback.md), [web-subtitles](../../../_program/reports/web-subtitles.md). [Owner decisions](../decisions.md) govern. Each existing conformance matrix is required coverage, not proof of execution. Release parity is required for applicable first-party features; truly unsupported platform capabilities are hidden. Roku, CarPlay, Android Auto and light theme are outside this programme. No consumer push or email. No stable API or old-database compatibility is promised before release.
