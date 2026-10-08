/**
 * Portico component specification.
 *
 * Exact measurements and states for the shared primitives, so web
 * (`web/src/ui`) and Apple (`portico-react-native/apps/apple/src/ui`) build the
 * same components. Numbers are CSS px on web and points on iOS/iPadOS; the
 * `tv` column is design pixels on the 1920×1080 ten-foot canvas. Colours are
 * token names from `colors` (never literals). The prose rules are in
 * `packages/design/COMPONENTS.md`.
 *
 * Platform columns: `web` (desktop/tablet browsers), `phone` (iPhone and
 * phone-width web), `tablet` (iPad), `tv` (Apple TV, Android TV).
 */
import type {ColorToken, TypeRole} from './index.ts';

type Platforms<T> = Readonly<{web: T; phone: T; tablet: T; tv: T}>;
const all = <T>(web: T, phone: T, tablet: T, tv: T): Platforms<T> => ({web, phone, tablet, tv});
const same = <T>(value: T, tv: T): Platforms<T> => all(value, value, value, tv);

export type Fill = Readonly<{background: ColorToken | 'transparent'; backgroundAlpha?: number; foreground: ColorToken; border?: ColorToken; borderAlpha?: number}>;

/** Focus: one system per control (PC-VISUAL §8.4–8.5). Web/iPad pointer: a ring; TV: ring + scale. */
export const focus = {
  ringWidth: same(2, 4),
  ringColor: 'focus' as ColorToken,
  /** Free-standing controls (buttons, chips, switches, card art). */
  offsetOuter: same(2, 4),
  /** Items inside a container (rows, menu items, tabs, segments, table rows). */
  offsetInner: same(-2, -4),
  /** Only television scales on focus. */
  scale: {button: same(1, 1.05), row: same(1, 1.015), card: same(1, 1.08), chip: same(1, 1.05)},
  /** Never add a fill as well as the ring on keyboard focus; hover may change the fill. */
  fillOnFocus: false,
} as const;

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'destructive' | 'link';
export type ButtonSize = 'sm' | 'md' | 'lg';
export type ButtonState = 'rest' | 'hover' | 'pressed' | 'selected' | 'disabled';

/** Buttons (CON-13, CON-14, §4.7). On TV nothing focusable is below 64 px (APL-SYS-01), so `sm` and `md` share a height there. */
export const button = {
  height: {sm: all(32, 32, 32, 64), md: all(40, 44, 44, 64), lg: all(48, 52, 52, 72)} as Readonly<Record<ButtonSize, Platforms<number>>>,
  /** Minimum hit area; a 32 px/pt `sm` button extends its hit area to this on touch. */
  minHit: all(32, 44, 44, 64),
  paddingX: {sm: same(12, 20), md: same(16, 28), lg: same(24, 40)} as Readonly<Record<ButtonSize, Platforms<number>>>,
  gap: same(8, 12),
  icon: {sm: same(16, 26), md: same(18, 30), lg: same(20, 32)} as Readonly<Record<ButtonSize, Platforms<number>>>,
  label: {sm: 'captionStrong', md: 'bodyStrong', lg: 'bodyStrong'} as Readonly<Record<ButtonSize, TypeRole>>,
  /** Every size uses the control radius; TV multiplies it. `round` is only for transport controls and avatars. */
  radius: same(8, 14),
  /** Icon-only buttons are square (width = height) and always carry a label (tooltip on web, accessibilityLabel on Apple). */
  iconOnlySquare: true,
  variants: {
    primary: {rest: {background: 'accent', foreground: 'actionInk'}, hover: {background: 'accentStrong', foreground: 'actionInk'}, pressed: {background: 'accentDeep', foreground: 'actionInk'}, selected: {background: 'accent', foreground: 'actionInk'}, disabled: {background: 'accent', foreground: 'actionInk'}},
    secondary: {rest: {background: 'raisedHigh', foreground: 'text', border: 'lineSoft'}, hover: {background: 'raisedBright', foreground: 'text', border: 'lineSoft'}, pressed: {background: 'raised', foreground: 'text', border: 'lineSoft'}, selected: {background: 'accent', backgroundAlpha: 0.28, foreground: 'text', border: 'accent', borderAlpha: 0.45}, disabled: {background: 'raisedHigh', foreground: 'text', border: 'lineSoft'}},
    ghost: {rest: {background: 'transparent', foreground: 'textSecondary'}, hover: {background: 'raisedHigh', foreground: 'text'}, pressed: {background: 'raised', foreground: 'text'}, selected: {background: 'accent', backgroundAlpha: 0.16, foreground: 'text'}, disabled: {background: 'transparent', foreground: 'textSecondary'}},
    destructive: {rest: {background: 'danger', backgroundAlpha: 0.12, foreground: 'danger', border: 'danger', borderAlpha: 0.3}, hover: {background: 'danger', backgroundAlpha: 0.16, foreground: 'danger', border: 'danger', borderAlpha: 0.45}, pressed: {background: 'danger', backgroundAlpha: 0.18, foreground: 'danger', border: 'danger', borderAlpha: 0.45}, selected: {background: 'danger', backgroundAlpha: 0.16, foreground: 'danger'}, disabled: {background: 'danger', backgroundAlpha: 0.12, foreground: 'danger'}},
    link: {rest: {background: 'transparent', foreground: 'accent'}, hover: {background: 'transparent', foreground: 'accentStrong'}, pressed: {background: 'transparent', foreground: 'accent'}, selected: {background: 'transparent', foreground: 'accentStrong'}, disabled: {background: 'transparent', foreground: 'accent'}},
  } as Readonly<Record<ButtonVariant, Readonly<Record<ButtonState, Fill>>>>,
  disabledOpacity: 0.45,
  /** Link press is shown by an underline, not a darker blue (accentDeep text fails contrast on dark surfaces). */
  /** Press feedback: web/phone scale 0.985; TV uses focus scale instead. */
  pressedScale: same(0.985, 1),
  /** Loading keeps the geometry and the label; a spinner the size of the icon replaces the leading icon. Labels never change to "Saving…". */
  loading: {keepsLabel: true, spinnerSize: 'icon'},
} as const;

/**
 * Player transport controls (TV-PLAYER-02): round, icon-only. `primary` is Play/Pause; `control`
 * is everything beside it (skip, previous/next, captions). TV: 72 px with 40 px icons, 88 px Play/Pause.
 */
export const transport = {
  control: {size: all(44, 44, 48, 72), icon: all(24, 24, 26, 40)},
  primary: {size: all(64, 64, 72, 88), icon: all(32, 32, 36, 48)},
  gap: all(16, 16, 20, 32),
} as const;

/** Text fields (X-11, PC-VISUAL §11). */
export const field = {
  height: all(40, 44, 44, 64),
  paddingX: same(12, 20),
  radius: same(8, 14),
  fill: {rest: 'raised', hover: 'raisedHigh', focus: 'raisedHigh', disabled: 'raised'} as Readonly<Record<string, ColorToken>>,
  border: {rest: 'lineControl', hover: 'lineControl', focus: 'accent', invalid: 'danger'} as Readonly<Record<string, ColorToken>>,
  borderWidth: same(1, 2),
  text: 'body' as TypeRole,
  placeholder: 'placeholder' as ColorToken,
  label: {role: 'captionStrong' as TypeRole, color: 'textSecondary' as ColorToken, gap: same(8, 8)},
  help: {role: 'caption' as TypeRole, color: 'textTertiary' as ColorToken},
  error: {role: 'caption' as TypeRole, color: 'danger' as ColorToken, icon: 'error'},
  /** Keyboard focus shows the focus ring (focus.offsetOuter) on top of the accent border. No glow. */
  focusGlow: false,
} as const;

/** Switch (CON-03). iOS/iPadOS use the system switch tinted with `accent`; web and TV draw this one. */
export const toggle = {
  track: {width: all(44, 51, 51, 56), height: all(26, 31, 31, 32)},
  thumb: all(22, 27, 27, 24),
  inset: same(2, 4),
  on: {track: 'accent', thumb: 'text'} as Readonly<Record<string, ColorToken>>,
  off: {track: 'textTertiary', trackAlpha: 0.35, thumb: 'text'},
  disabledOpacity: 0.45,
  /** A row with a switch toggles when the row is pressed; role = switch; the accessible name never changes with state ("Show X on Home"). */
  wholeRowToggles: true,
  motionMs: 120,
} as const;

/** Chips: quick filters and removable filter tokens (CON-13 browse bar). */
export const chip = {
  height: all(32, 32, 32, 64),
  minHit: all(32, 44, 44, 64),
  paddingX: same(12, 20),
  gap: same(8, 12),
  icon: same(16, 24),
  radius: 'pill',
  label: 'captionStrong' as TypeRole,
  rest: {background: 'raised', foreground: 'textSecondary', border: 'lineSoft'} as Fill,
  hover: {background: 'raisedHigh', foreground: 'text', border: 'lineStrong'} as Fill,
  selected: {background: 'accent', backgroundAlpha: 0.28, foreground: 'text', border: 'accent', borderAlpha: 0.45} as Fill,
  /** A removable chip ends with a 14 px close glyph and its accessible name is "Remove {label}". */
  removableIcon: 'close',
  /** Filter bars use chips and ghost/secondary buttons at the chip height; never a primary button (PC-VISUAL §9.4). */
  barGap: same(8, 12),
} as const;

/** Segmented control (peer choices, 2–5 options). */
export const segmented = {
  height: all(32, 32, 32, 64),
  /** Touch: the 32 pt control extends its hit area vertically to 44. */
  minHit: all(32, 44, 44, 64),
  padding: same(4, 4),
  radius: same(8, 14),
  segmentRadius: same(6, 10),
  track: 'raisedSoft' as ColorToken,
  selected: {background: 'raisedHigh', foreground: 'text'} as Fill,
  rest: {background: 'transparent', foreground: 'textTertiary'} as Fill,
  label: 'captionStrong' as TypeRole,
} as const;

export type RowVariant = 'navigation' | 'value' | 'toggle' | 'action' | 'menu' | 'static' | 'media';

/** Rows and grouped lists (CON-04/05/08/09/10/11, §4.2). */
export const row = {
  minHeight: {single: all(48, 44, 44, 64), withSubtitle: all(64, 56, 56, 80), media: all(64, 64, 64, 96)},
  paddingX: all(16, 12, 16, 20),
  paddingY: same(8, 12),
  leadingBox: same(24, 32),
  leadingIcon: all(20, 22, 22, 30),
  leadingGap: same(12, 20),
  /** Row artwork width. Phone/iPad/TV columns are the Apple sizes tuned on 17 Sep (episode stills 116 / 220). */
  artwork: {poster: all(40, 52, 52, 96), square: all(48, 52, 52, 96), landscape: all(96, 116, 116, 220)},
  title: 'bodyStrong' as TypeRole,
  subtitle: {role: 'caption' as TypeRole, color: 'textTertiary' as ColorToken, lines: {settings: 3, media: 1}},
  trailing: {
    navigation: {glyph: 'forward', size: same(16, 24), color: 'textTertiary'},
    value: {text: 'textSecondary', glyph: 'chevronUpDown', size: same(16, 24)},
    toggle: {control: 'toggle'},
    action: {glyph: null, title: 'accent'},
    destructive: {glyph: null, title: 'danger'},
    menu: {glyph: 'more', size: same(18, 26), color: 'textTertiary'},
    static: {glyph: null, focusable: false},
  },
  hover: {background: 'raisedHigh' as ColorToken},
  selected: {background: 'accent' as ColorToken, backgroundAlpha: 0.16},
  /** One group = one surface; rows are never individually bordered (PC-VISUAL §12.5). */
  group: {
    radius: same(10, 18),
    background: 'raised' as ColorToken,
    border: 'lineSoft' as ColorToken,
    separator: 'lineSoft' as ColorToken,
    /** The separator starts where the row's text starts (paddingX + leadingBox + leadingGap when there is a leading element). */
    separatorInsetToText: true,
    header: {role: 'label' as TypeRole, color: 'textTertiary' as ColorToken, gapBelow: same(8, 12)},
    footer: {role: 'caption' as TypeRole, color: 'textTertiary' as ColorToken, gapAbove: same(8, 12)},
    gapBetweenGroups: same(24, 40),
  },
} as const;

export type OverlayKind = 'form' | 'picker' | 'acknowledge';

/** Sheets and dialogs (CON-01, CON-02, CON-29, §4.1). */
export const overlay = {
  surface: 'overlaySurface' as ColorToken,
  /** Only desktop web with working backdrop blur may use `overlaySurfaceTranslucent`. */
  translucentAllowed: all(true, false, false, false),
  border: {color: 'lineStrong' as ColorToken, alpha: 0.7, width: 1},
  radius: same(14, 24),
  scrim: {color: 'ink' as ColorToken, alpha: all(0.62, 0.72, 0.62, 0.8)},
  inset: all(20, 16, 20, 28),
  width: {sm: all(440, 0, 440, 520), md: all(520, 0, 520, 640), lg: all(640, 0, 640, 760)},
  /** `side` placement (the episode panel): a full-height panel on the trailing edge. Phones use the bottom sheet instead. */
  sideWidth: all(440, 0, 440, 640),
  /** Phones: a `half` detent opens the sheet at this fraction of the screen; dragging up expands it to maxHeight. */
  halfDetent: 0.5,
  /** Phone: bottom sheet, full width (0 = full width), with a grabber. Others: centred. */
  presentation: all('centered', 'bottomSheet', 'centered', 'centered'),
  maxHeight: all(0.88, 0.9, 0.82, 0.82),
  grabber: {width: 36, height: 4, color: 'textTertiary' as ColorToken, alpha: 0.5, top: 8},
  title: 'heading' as TypeRole,
  subtitle: {role: 'caption' as TypeRole, color: 'textSecondary' as ColorToken},
  headerPaddingTop: all(20, 16, 20, 28),
  headerGap: same(4, 8),
  /** ✕ appears only on pickers (and never on TV): 44 × 44 (web 40), top-trailing, label "Close". */
  closeButton: {size: all(40, 44, 44, 0), kinds: ['picker'] as readonly OverlayKind[]},
  /** Body horizontal padding = inset − row.paddingX, so row text aligns with the title; non-row content adds row.paddingX back. */
  bodyAlignsRowsToTitle: true,
  footer: {
    kinds: ['form', 'acknowledge'] as readonly OverlayKind[],
    background: 'overlaySurface' as ColorToken,
    hairline: 'lineSoft' as ColorToken,
    paddingY: same(12, 20),
    gap: same(8, 16),
    /** Order: leading ghost action (Reset, Clear all) at the leading edge · spacer · Cancel (ghost) · primary. TV: primary first and focused. Phone web: stacked full width, primary on top. */
    order: ['leading', 'spacer', 'cancel', 'primary'] as const,
    buttonSize: all('md', 'lg', 'md', 'md') as Platforms<ButtonSize>,
  },
  motion: {enterMs: 180, exitMs: 120, translate: 24, scaleFrom: 0.98},
} as const;

/** Menus (card menu, More, sort, pickers on web; anchored panels on iPad). CON-05. */
export const menu = {
  surface: 'overlaySurface' as ColorToken,
  radius: same(12, 20),
  padding: same(8, 12),
  width: {min: same(220, 360), max: same(320, 520)},
  maxVisibleRows: 12,
  row: {minHeight: all(40, 44, 44, 64), paddingX: same(12, 20), icon: same(18, 28), gap: same(12, 16), label: 'body' as TypeRole, radius: same(8, 14)},
  highlight: {background: 'accent' as ColorToken, backgroundAlpha: 0.16},
  sectionLabel: {role: 'label' as TypeRole, color: 'textTertiary' as ColorToken, paddingTop: same(8, 12), paddingBottom: same(4, 8)},
  separator: {color: 'lineSoft' as ColorToken, insetX: same(4, 8), marginY: same(4, 8)},
  destructive: {foreground: 'danger' as ColorToken, groupLast: true},
  selected: {glyph: 'check', color: 'accent' as ColorToken},
  submenu: {glyph: 'forward'},
  /** Section order for entry actions (shared model, CON-24). */
  entryGroups: ['play', 'together', 'personal', 'organize', 'open', 'manage'] as const,
} as const;

/** Inline notices, state views and toasts (CON-07, CON-17, CON-18). */
export const feedback = {
  notice: {
    padding: same(12, 20), gap: same(12, 16), radius: same(10, 18), icon: same(18, 28), text: 'caption' as TypeRole, title: 'bodyStrong' as TypeRole,
    tones: {
      info: {icon: 'info', color: 'accent', tintAlpha: 0.12, borderAlpha: 0.3},
      success: {icon: 'success', color: 'healthy', tintAlpha: 0.12, borderAlpha: 0.3},
      warning: {icon: 'warning', color: 'amber', tintAlpha: 0.12, borderAlpha: 0.3},
      error: {icon: 'error', color: 'danger', tintAlpha: 0.12, borderAlpha: 0.3},
    },
    action: {variant: 'secondary' as ButtonVariant, size: 'sm' as ButtonSize},
  },
  stateView: {
    iconCircle: same(56, 88), icon: same(24, 38), gap: same(12, 20), maxWidth: same(440, 760), title: 'subheading' as TypeRole, body: 'body' as TypeRole,
    action: {variant: 'primary' as ButtonVariant, size: 'md' as ButtonSize}, secondaryAction: {variant: 'ghost' as ButtonVariant},
  },
  toast: {
    surface: 'overlaySurface' as ColorToken, radius: same(12, 20), paddingX: same(16, 28), paddingY: same(12, 20), maxWidth: all(560, 0, 560, 900),
    position: all('bottomCenter', 'aboveTabBar', 'bottomCenter', 'topTrailing'), durationMs: 4000, durationWithActionMs: 8000,
    action: {variant: 'link' as ButtonVariant},
  },
  /** Loading label copy is "Loading {thing}" without an ellipsis; skeletons match final geometry and don't pulse under Reduce Motion. */
  spinner: {size: {inline: same(16, 26), region: same(28, 44), screen: same(44, 64)}, stroke: same(2, 3), track: 'textTertiary' as ColorToken, trackAlpha: 0.35, arc: 'accent' as ColorToken, periodMs: 1000},
} as const;

/** Confirmation tiers (CON-06, §4.5). */
export const confirmation = {
  tiers: {
    0: {dialog: false, undoToast: true, examples: ['Remove from queue', 'Remove from Watchlist', 'Archive', 'Mark as read']},
    1: {dialog: true, confirmVariant: 'destructive' as ButtonVariant, typed: false, examples: ['Delete profile', 'Remove device', 'Revoke key', 'End group', 'Sign out everywhere']},
    2: {dialog: true, confirmVariant: 'destructive' as ButtonVariant, typed: true, examples: ['Delete files from disk', 'Empty trash', 'Clean up a folder', 'Remove a library']},
  },
  /** Non-destructive checks (verify a backup, disconnect) use a normal form with a primary and no typed field. */
  typedTokenIsHumanWord: true,
  /** Title "Verb object?"; body = the consequence; confirm label = the verb ("Delete profile"), never "OK". */
  copy: {title: 'verbObjectQuestion', confirmLabel: 'verb'},
} as const;

/** A scrim stop: `at` is 0–1 along the gradient, `alpha` the ink opacity there. */
export type ScrimStop = Readonly<{at: number; alpha: number}>;

/**
 * TitleHero (Spec — Title Pages §1): the one fixture every title page opens with. Its height never
 * changes between media types on the same device.
 */
export const hero = {
  /** TV: 60 % of the screen. Web and iPad: min(70 % of the viewport height, 720). Phone: a 16:9 backdrop, then the identity block below it. */
  height: {
    web: {viewportFraction: 0.7, max: 720},
    tablet: {viewportFraction: 0.7, max: 720},
    tv: {viewportFraction: 0.6},
    phone: {backdropAspect: 16 / 9, identityBelow: true},
  },
  /** Artwork request width (`?w=`): 1920 on TV, desktop and iPad; 800 on phones. */
  backdropWidth: all(1920, 800, 1920, 1920),
  backdropOpacity: all(0.8, 1, 0.8, 0.8),
  /** Scrims in `ink`. Wide layouts: a bottom and a left gradient; phones: bottom only (the identity sits below the image). */
  scrim: {
    color: 'ink' as ColorToken,
    bottom: [{at: 0, alpha: 0.35}, {at: 0.28, alpha: 0}, {at: 0.62, alpha: 0.55}, {at: 1, alpha: 1}] as readonly ScrimStop[],
    left: [{at: 0, alpha: 0.82}, {at: 0.45, alpha: 0.3}, {at: 0.75, alpha: 0}] as readonly ScrimStop[],
    phoneBottom: [{at: 0, alpha: 0}, {at: 0.55, alpha: 0.15}, {at: 1, alpha: 1}] as readonly ScrimStop[],
  },
  /** Without a backdrop: a tinted gradient (accent glow over raised → ink), never an empty box. */
  fallback: {glow: 'accent' as ColorToken, glowAlpha: 0.18, from: 'raised' as ColorToken, to: 'ink' as ColorToken},
  paddingX: all(32, 16, 32, 80),
  paddingBottom: all(8, 16, 16, 24),
  /** Gap between poster art and the identity column on wide layouts. */
  gap: all(32, 16, 24, 40),
  identity: {
    eyebrow: 'label' as TypeRole,
    /** Web desktop, iPad and TV use display type; phones the title role. Two lines, then ellipsis. */
    title: all('display', 'title', 'display', 'display') as Platforms<TypeRole>,
    titleLines: 2,
    /** Logo art replaces the title: at most this wide (and never over 70 % of the column). */
    logoMaxWidth: all(440, 280, 440, 640),
    gap: same(8, 12),
  },
  /** One line, `·`-separated, type-specific; the content rating is a badge at the end. */
  facts: {
    role: 'body' as TypeRole,
    color: 'textSecondary' as ColorToken,
    separator: '·',
    separatorColor: 'textTertiary' as ColorToken,
    gap: same(12, 16),
    badge: {height: same(24, 36), paddingX: same(8, 12), radius: 'small', border: 'lineControl' as ColorToken, role: 'captionStrong' as TypeRole, color: 'text' as ColorToken},
  },
  /** Two lines; "More" expands inline (web, phone, iPad) or opens a text sheet (TV). */
  synopsis: {role: 'body' as TypeRole, color: 'textSecondary' as ColorToken, lines: 2, maxWidth: all(640, 0, 720, 960)},
  actions: {
    gap: same(12, 16),
    /** One height for the whole group (primary and the round secondary icon buttons). Phones: the primary is full width. */
    size: all('lg', 'md', 'lg', 'lg') as Platforms<ButtonSize>,
    phonePrimaryFullWidth: true,
    /** Resuming: a thin bar under the primary's label. */
    progress: {height: 3, insetX: 16, track: 'actionInk' as ColorToken, trackAlpha: 0.3, fill: 'actionInk' as ColorToken},
    /** Secondary icon buttons show their label on focus (TV) or hover (web). */
    labelOnFocus: true,
  },
  /** Poster (2:3) beside the identity on wide layouts only when there's no logo; square for music; circle for people and artists. */
  artwork: {poster: all(240, 0, 160, 240), square: all(240, 0, 180, 280), posterMin: 140},
} as const;

export const componentSpec = {focus, button, transport, field, toggle, chip, segmented, row, overlay, menu, feedback, confirmation, hero} as const;
export type ComponentSpec = typeof componentSpec;
