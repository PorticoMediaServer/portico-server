/**
 * Portico's shared design projection. Web (scripts/tokens.mjs → tokens.css)
 * and Apple (ui/theme.ts) read every colour, type role, radius and layer from
 * here, so the platforms can't drift. Contrast is pinned by
 * packages/client-core/test/design-tokens.test.ts: every text tone must stay
 * ≥ 4.5:1 on canvas, slate, raised and overlay surfaces, and control
 * boundaries ≥ 3:1 (WCAG 1.4.3 / 1.4.11, PC-VISUAL §5 and §18).
 */
export const colors = {
  /** Surfaces, darkest to brightest: canvas → slate → raised → bright. */
  ink: "#071018",
  raised: "#10212d",
  /** Hover, focus and selected surfaces (blue slate, never grey — X-07). */
  raisedHigh: "#162c3a",
  /** The highest ordinary surface (chips, badges, tooltips). */
  raisedBright: "#1d3848",
  /** Subdued containers (sunken wells, segmented tracks). */
  raisedSoft: "#0c1a25",
  /**
   * Sheets, dialogs, menus, popovers and the expanded rail. Opaque by
   * decision V-1 / X-16: text behind an overlay must never show through.
   */
  overlaySurface: "#132634",
  /**
   * The only translucent overlay option: desktop web with working
   * backdrop blur. Apple and TV always use `overlaySurface`.
   */
  overlaySurfaceTranslucent: "#132634F5",
  ambient: "#153749",
  /** Action blue: rest, hover/focus (strong) and pressed (deep). */
  accent: "#409fd4",
  accentStrong: "#5cb3e2",
  accentDeep: "#2f86bb",
  text: "#edf5fa",
  textSecondary: "#bdccd6",
  textTertiary: "#96a9b5",
  /** Same value as textTertiary: "muted" never lowers contrast (X-06). */
  muted: "#96a9b5",
  placeholder: "#8298a6",
  /** Hairlines: soft (inside surfaces), standard, strong (emphasis). */
  lineSoft: "#1c3341",
  line: "#28404f",
  lineStrong: "#3b5668",
  /** Boundary of an input or other control at rest: ≥ 3:1 on canvas and slate (X-11). */
  lineControl: "#637580",
  danger: "#f38b88",
  focus: "#d5efff",
  scrim: "#071018E8",
  material: "#10212DF2",
  amber: "#e8bc6a",
  record: "#f17979",
  healthy: "#86cfb3",
  trackRest: "#35505f",
  /** Text and glyphs on action blue (all three accent states). */
  actionInk: "#03121c",
} as const;

export type ColorToken = keyof typeof colors;

/** The 4-point spacing scale (PC-VISUAL §7.1). Legacy names kept for existing callers. */
export const space = { 0: 0, 1: 4, 2: 8, 3: 12, 4: 16, 6: 24, 8: 32, 12: 48 } as const;
export const spacing = { xs: 8, sm: 16, md: 24, lg: 32, xl: 48 } as const;
/** Values allowed for inline spacing by the web design lint (and a guide for Apple). */
export const spacingGrid = [0, 2, 4, 8, 12, 16, 20, 24, 28, 32, 40, 48, 56, 64, 80] as const;

export const typography = {
  regular: "Manrope-Regular",
  medium: "Manrope-Medium",
  semibold: "Manrope-SemiBold",
  bold: "Manrope-Bold",
  mobileBody: 16,
  tvBody: 24,
} as const;

export type TypeRole = 'display' | 'title' | 'heading' | 'subheading' | 'body' | 'bodyStrong' | 'caption' | 'captionStrong' | 'micro' | 'label';
export type TypeStyle = Readonly<{
  /** Point size (Apple) / CSS px (web). */
  size: number;
  lineHeight: number;
  weight: 400 | 500 | 600 | 700;
  /** Letter spacing in em; Apple multiplies by size. */
  tracking: number;
  uppercase?: boolean;
}>;

/**
 * One type scale, two columns (X-10). Handheld and web share the first; the
 * ten-foot column is designed at 1920×1080. Web may make the display role
 * fluid between `display.size` and its desktop size (an optical exception).
 */
export const typeScale: Readonly<Record<'handheld' | 'tenFoot', Readonly<Record<TypeRole, TypeStyle>>>> = {
  handheld: {
    display: {size: 34, lineHeight: 38, weight: 700, tracking: -0.026},
    title: {size: 26, lineHeight: 31, weight: 700, tracking: -0.023},
    heading: {size: 20, lineHeight: 25, weight: 600, tracking: -0.0175},
    subheading: {size: 17, lineHeight: 22, weight: 600, tracking: -0.012},
    body: {size: 15, lineHeight: 21, weight: 400, tracking: 0},
    bodyStrong: {size: 15, lineHeight: 21, weight: 600, tracking: 0},
    caption: {size: 13, lineHeight: 18, weight: 400, tracking: 0},
    captionStrong: {size: 13, lineHeight: 18, weight: 600, tracking: 0},
    micro: {size: 11, lineHeight: 14, weight: 500, tracking: 0.018},
    label: {size: 12, lineHeight: 16, weight: 600, tracking: 0.05, uppercase: true},
  },
  tenFoot: {
    display: {size: 62, lineHeight: 68, weight: 700, tracking: -0.029},
    title: {size: 44, lineHeight: 52, weight: 700, tracking: -0.027},
    heading: {size: 32, lineHeight: 40, weight: 600, tracking: -0.019},
    subheading: {size: 27, lineHeight: 34, weight: 600, tracking: -0.011},
    body: {size: 24, lineHeight: 33, weight: 400, tracking: 0},
    bodyStrong: {size: 24, lineHeight: 33, weight: 600, tracking: 0},
    caption: {size: 20, lineHeight: 27, weight: 400, tracking: 0},
    captionStrong: {size: 20, lineHeight: 27, weight: 600, tracking: 0},
    micro: {size: 17, lineHeight: 22, weight: 500, tracking: 0.018},
    label: {size: 18, lineHeight: 24, weight: 600, tracking: 0.055, uppercase: true},
  },
};
/** Web desktop size for the fluid display role (clamps from handheld.display.size). */
export const displayDesktopSize = 42;

export { mediaIcons, mediaFamilyIcon } from "./media-icons.ts";
export type { MediaFamily, MediaIconName, IconPrimitive, MediaIconDefinition } from "./media-icons.ts";
export { icons, iconIds, isIconId } from "./icons.ts";
export type { IconId, IconDefinition } from "./icons.ts";
export { componentSpec, focus as focusSpec, button as buttonSpec, transport as transportSpec, field as fieldSpec, toggle as toggleSpec, chip as chipSpec, segmented as segmentedSpec, row as rowSpec, overlay as overlaySpec, menu as menuSpec, feedback as feedbackSpec, confirmation as confirmationSpec, hero as heroSpec } from "./components.ts";
export type { ComponentSpec, Fill, ButtonVariant, ButtonSize, ButtonState, RowVariant, OverlayKind, ScrimStop } from "./components.ts";

/**
 * Corner radii by surface class (PC-VISUAL §7.7). `panel` is the menu /
 * popover radius; `overlay` is sheets and dialogs. Television multiplies
 * every radius except `pill` by `tvRadiusMultiplier` (an optical exception
 * for ten-foot viewing: control 8 → 14, surface 10 → 18, overlay 14 → 24).
 * Buttons use `control` at every size.
 */
export const radii = { small: 6, control: 8, artwork: 8, surface: 10, panel: 12, overlay: 14, pill: 999 } as const;
export const tvRadiusMultiplier = 1.75;
export const motion = { quick: 120, standard: 180, slow: 320 } as const;
export const layers = { base: 0, sticky: 10, navigation: 20, player: 30, menu: 40, dialog: 50, notice: 60 } as const;

/** Minimum targets (PC-VISUAL §7.4–7.5). */
export const targets = { touch: 44, pointer: 32, primaryTouch: 48, tenFoot: 64, menuRowPointer: 40 } as const;

/** Profile identity choices, shared across clients with light initials. */
export const profileArtColors = {
  blue: "#28587a",
  violet: "#685294",
  mint: "#28665d",
  coral: "#985243",
  gold: "#796120",
  slate: "#4a596a",
  rose: "#854d73",
  sky: "#376f8c",
} as const;

/** WCAG 2.x contrast ratio between two opaque `#rrggbb` colours. */
export function contrastRatio(a: string, b: string): number {
  const luminance = (hex: string): number => {
    const n = hex.replace('#', '').slice(0, 6);
    const [r, g, bl] = [0, 2, 4].map(i => parseInt(n.slice(i, i + 2), 16) / 255).map(v => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
    return 0.2126 * r + 0.7152 * g + 0.0722 * bl;
  };
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}
