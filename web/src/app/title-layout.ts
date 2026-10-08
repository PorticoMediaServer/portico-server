import type React from 'react';
import type {ContentEntry} from '@core/library-content.ts';
import {titleTintHue} from '@core/presentation/index.ts';

/**
 * M26 layout rules (Spec — Page Content by Media Type §0.1–§0.2): the pure
 * decisions behind every title page, kept here so they can be unit-tested
 * without React. Screens and `TitleHero` only render what these return.
 */

/** The hero geometry for a title page: full-height with a backdrop, or compact. */
export type HeroLayout = 'loading' | 'full' | 'compact';

/**
 * A hero without a backdrop is the compact hero (art beside the title, a
 * background derived from the art) — never the tall empty band. While the
 * title is still loading and no backdrop has arrived, the loading skeleton
 * keeps its geometry so the page doesn't jump when data lands.
 */
export function heroLayout(input: {backdropUrl?: string; loading?: boolean}): HeroLayout {
  if (input.loading && !input.backdropUrl) return 'loading';
  return input.backdropUrl ? 'full' : 'compact';
}

export type EpisodeArt = {kind: 'still'; path: string} | {kind: 'placeholder'};

/**
 * An episode shows its own still (`stillUrl`, which the server never fills
 * from the season or show), else the styled placeholder (the episode
 * code on a neutral surface) — never the show backdrop repeated as every
 * episode's art.
 */
export function episodeArt(entry: Pick<ContentEntry, 'stillUrl'>): EpisodeArt {
  const path = entry.stillUrl;
  return path ? {kind: 'still', path} : {kind: 'placeholder'};
}

/**
 * Spec — Page Content §0.2: a missing still takes the show's colour. Returns the custom property
 * the placeholder classes read (`--placeholder-bg`), from the shared tint for this show; set it
 * on the container that holds the episodes.
 */
export function placeholderTint(seed: string | undefined): React.CSSProperties | undefined {
  if (!seed) return undefined;
  const hue = titleTintHue(seed);
  // lint-design-allow: the title's own tint, computed from its name
  return {'--placeholder-bg': `linear-gradient(160deg, hsl(${hue} 30% 27%), hsl(${hue} 36% 13%))`} as React.CSSProperties; // lint-design-allow
}

export type MosaicItem = Pick<ContentEntry, 'posterUrl' | 'backdropUrl'>;

/**
 * The collection/playlist hero mosaic: `undefined` when the entity has its
 * own art (use it), otherwise the first 4 item posters or covers (fewer when
 * the collection is smaller; empty when nothing has art, so the hero renders
 * nothing rather than empty tiles). `artworkPaths` wins when the server sent
 * it; otherwise the loaded items are read in order.
 */
export function mosaicPaths(
  entity: Pick<ContentEntry, 'posterUrl' | 'backdropUrl' | 'artworkPaths'>,
  items?: readonly MosaicItem[],
): readonly string[] | undefined {
  if (entity.posterUrl ?? entity.backdropUrl) return undefined;
  const sent = (entity.artworkPaths ?? []).filter((p): p is string => !!p).slice(0, 4);
  if (sent.length) return sent;
  return (items ?? []).map(e => e.posterUrl ?? e.backdropUrl).filter((p): p is string => !!p).slice(0, 4);
}
