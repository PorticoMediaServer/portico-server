export {generatedCoverTint, type GeneratedCoverTint} from './generated-cover.ts';

/**
 * Portico's shared design projection. Web (packages/design/scripts/tokens.mjs → web-next/src/kit/tokens.css)
 * and native (src-next/kit/theme.ts) read every colour, type role, radius and layer from
 * here, so the platforms can't drift. Contrast is pinned by
 * packages/client-core/test/design-tokens.test.ts: every text tone must stay
 * ≥ 4.5:1 on canvas, slate, raised and overlay surfaces, and control
 * boundaries ≥ 3:1 (WCAG 1.4.3 / 1.4.11, PC-VISUAL §5 and §18).
 */
export const colors = {
  /** Surfaces, darkest to brightest: canvas → slate → raised → bright. */
  ink: "#071018",
  white: "#ffffff",
  shadow: "#000000",
  collection: "#0a2233",
  generatedGreen: "#1e5b52",
  dim: "#03080c",
  raised: "#10212d",
  /** Hover, focus and selected surfaces (blue slate, never grey — X-07). */
  raisedHigh: "#162c3a",
  /** The highest ordinary surface (chips, badges, tooltips). */
  raisedBright: "#1d3848",
  /** Subdued containers (sunken wells, segmented tracks). */
  raisedSoft: "#0c1a25",
  /** Player notices; the approved Undo and Up next contexts apply their own alpha. */
  playerNotice: "#0c1923",
  /** Live-player top and bottom scrim colour, with per-stop alpha supplied by chrome. */
  livePlayerScrim: "#04090d",
  /** owner-server message log inset well. */
  messageLog: "#050d14",
