import React, {useState} from 'react';
import {uiI18n} from './i18n';
import {heroLayout} from '../app/title-layout';
import {Artwork, type ArtworkShape} from './Artwork';
import {Backdrop} from './Backdrop';
import {Button} from './Button';
import {cx} from './cx';
import type {IconName} from './Icon';
import {Skeleton} from './Feedback';
import s from './TitleHero.module.css';

export type TitleHeroAction = {label: string; icon?: IconName; onClick: () => void; loading?: boolean; disabled?: boolean; /** 0–1: a thin bar under the label when resuming. */ progress?: number};

/**
 * The shared hero for every title page (Spec — Title Pages §1). Render it
 * outside <Page> so the backdrop bleeds to the content edges; its own content
 * keeps the page's width and gutters. Movie, show
 * and anime, album, artist, audiobook, person, collection and playlist fill
 * the same slots. Its height doesn't change between types: min(70vh, 720px)
 * on desktop; on phones a 16:9 backdrop with the identity block beneath.
 *
 * - Backdrop: full bleed with a bottom and left scrim; without one, the
 *   compact hero: art beside the title over a blurred, darkened version of
 *   that art (never a tall empty band).
 * - Identity: logo art when there is one, else the title in display type.
 * - Facts: one `·`-separated line; the content rating is a chip.
 * - Tagline on a line of its own; synopsis: two lines; More expands in place.
 * - Primary action (with resume progress) and secondary actions.
 * - Artwork beside the identity on wide layouts only when there's no logo:
 *   poster, square for music, circle for artists and people.
 */
export function TitleHero({backdrop, artwork, logo, title, originalTitle, context, onContext, facts = [], badge, extra, tagline, synopsis, credits, choices, primary, secondary, onBack, loading}: {
  backdrop?: string;
  artwork?: {path?: string; shape: Extract<ArtworkShape, 'poster' | 'square' | 'circle'>; icon?: IconName; initial?: string; /** 2×2 hero mosaic (collection/playlist without custom art); wins over `path`. */ mosaic?: readonly string[]};
  logo?: string;
  title?: string;
  /** A short line above the title that places it (an episode's show and season; an album's artist link). */
  context?: React.ReactNode;
  /** Makes the context line a link (an album's artist): readable on any backdrop, underlined on hover. */
  onContext?: () => void;
  /** Text, or a node where a fact is more than text (genres that link to their filter). */
  facts?: readonly React.ReactNode[];
  badge?: string;
  /** Extra inline content at the end of the facts line (e.g. a provider score). */
  extra?: React.ReactNode;
  /** The original title, when it differs: a subtitle under the title, not a fact. */
  originalTitle?: string;
  /** The title's tagline: one line of its own above the synopsis, never run into it. */
  tagline?: string;
  synopsis?: string;
  /** A short credits line under the synopsis ("Directed by …"). */
  credits?: readonly string[];
  /** Pick before play: version, audio and subtitle controls, between the credits and the actions. */
  choices?: React.ReactNode;
  primary?: TitleHeroAction;
  secondary?: React.ReactNode;
  onBack?: () => void;
  loading?: boolean;
}) {
  const [expanded, setExpanded] = useState(false);
  const shownFacts = facts.filter(f => f !== undefined && f !== null && f !== false && f !== '');
  // M26: no backdrop means the compact hero (art beside the title over a
  // blurred, darkened version of that art), never the tall empty band. While
  // loading, the skeleton keeps the full geometry so the page doesn't jump.
  const compact = heroLayout({backdropUrl: backdrop, loading}) === 'compact';
  // No empty frames: artwork shows while loading, when there's a path, for a
  // mosaic, or for an initial (an artist without an image keeps its circle).
  const showArtwork = !!artwork && !logo && (loading || !!artwork.path || !!artwork.initial || !!artwork.mosaic?.length);
  const mosaic = showArtwork ? artwork!.mosaic?.filter((p): p is string => !!p).slice(0, 4) : undefined;
  return (
    <div className={cx(s.root, compact && s.compact)}>
      {backdrop ? <Backdrop path={backdrop} height="var(--title-hero-height)" opacity={0.8} className={s.backdrop} /> : compact && artwork?.path ? (
        <div className={s.compactBg} aria-hidden>
          <Artwork path={artwork.path} shape={artwork.shape} alt="" className={s.compactBgArt} />
        </div>
      ) : <div className={s.fallback} aria-hidden />}
      {onBack ? <div className={s.top}><Button variant="ghost" size="sm" icon="back" label={uiI18n().t('action.back')} onClick={onBack} /></div> : null}
      <div className={cx(s.hero, showArtwork && s.withArtwork)}>
        {showArtwork ? (
          loading && !artwork.path && !artwork.initial && !mosaic?.length ? <Skeleton className={cx(s.artwork, s[artwork.shape])} /> : mosaic && mosaic.length > 1 ? (
            <div className={cx(s.artwork, s.mosaic)} aria-hidden>
              {[mosaic[0], mosaic[1], mosaic[2], mosaic[3]].map((path, i) => <Artwork key={i} path={path} shape="poster" icon={artwork.icon} alt="" className={s.mosaicTile} />)}
            </div>
          ) : <Artwork path={mosaic?.[0] ?? artwork.path} shape={artwork.shape} icon={artwork.icon} initial={artwork.initial} className={cx(s.artwork, s[artwork.shape])} alt="" />
        ) : null}
        <div className={s.identity}>
          {context ? (onContext ? <button type="button" className={cx(s.context, s.contextLink)} onClick={onContext}>{context}</button> : <span className={s.context}>{context}</span>) : null}
          {logo ? <Artwork path={logo} shape="logo" alt={title ?? ''} className={s.logo} /> : title ? <h1 className={s.title}>{title}</h1> : <Skeleton height={44} width="55%" />}
          {logo && title ? <h1 className="visually-hidden">{title}</h1> : null}
          {originalTitle ? <p className={s.originalTitle}>{originalTitle}</p> : null}
          {shownFacts.length || badge || extra ? (
            <div className={s.facts}>
              {shownFacts.map((f, i) => <span key={i} className={cx(i > 0 && s.sep)}>{f}</span>)}
              {badge ? <span className={s.badge}>{badge}</span> : null}
              {extra}
            </div>
          ) : null}
          {tagline ? <p className={s.tagline}>{tagline}</p> : null}
          {synopsis ? (
            <div className={s.synopsisBlock}>
              <p className={cx(s.synopsis, !expanded && s.clamped)}>{synopsis}</p>
              {synopsis.length > 180 ? <Button variant="link" size="sm" label={uiI18n().t(expanded ? 'title.less' : 'title.more')} onClick={() => setExpanded(v => !v)} aria-expanded={expanded} /> : null}
            </div>
          ) : null}
          {credits?.length ? <div className={s.credits}>{credits.map(c => <span key={c}>{c}</span>)}</div> : null}
          {choices ? <div className={s.choices}>{choices}</div> : null}
          {primary || secondary ? (
            <div className={s.actions}>
              {primary ? (
                <span className={s.primary}>
                  <Button variant="primary" size="lg" icon={primary.icon ?? 'play'} label={primary.label} onClick={primary.onClick} loading={primary.loading} disabled={primary.disabled} />
                  {primary.progress && primary.progress > 0 && primary.progress < 1 ? <span className={s.progress} aria-hidden><i style={{width: `${Math.round(primary.progress * 100)}%`}} /></span> : null}
                </span>
              ) : null}
              {secondary ? <div className={s.secondary}>{secondary}</div> : null}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
