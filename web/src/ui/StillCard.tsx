import React from 'react';
import {uiI18n} from './i18n';
import {Artwork} from './Artwork';
import {Icon, type IconName} from './Icon';
import {Skeleton} from './Feedback';
import s from './StillCard.module.css';

/**
 * A landscape card with two targets (Spec — Title Pages §2, episode cards):
 * the still plays (a play button shows on hover or focus), the text opens
 * details. Leading number, title, one meta line, a two-line synopsis,
 * progress and a watched tick.
 */
export const StillCard = React.memo(function StillCard({still, placeholder, icon = 'tv', number, title, meta, synopsis, progress, watched, opened, landed, tag, playLabel, openLabel, moreLabel, onPlay, onOpen, onMore}: {
  still?: string; /** Shown large on the neutral placeholder when there is no still (the episode code). */ placeholder?: string; icon?: IconName; number?: number | string; title: string; meta?: string; synopsis?: string; progress?: number; watched?: boolean;
  /** Open in place: the still lifts in a soft ring and a notch points down to its details. */ opened?: boolean;
  /** The item the page was opened for: the same ring, fading out. */ landed?: boolean;
  /** A small chip after the title ("Next up"). */ tag?: string;
  /** Accessible names for the two targets, e.g. "Play S1 E4, Pilot". */
  playLabel: string; openLabel: string;
  onPlay?: () => void; onOpen: () => void;
  /** Opens the card's actions from the element given (the ⋯ button, or the card itself on a right-click). Needs `moreLabel`. */
  moreLabel?: string; onMore?: (anchor: Element) => void;
}) {
  return (
    <div className={s.card} data-opened={opened || undefined} data-landed={landed && !opened || undefined} onContextMenu={onMore ? e => { e.preventDefault(); onMore(e.currentTarget); } : undefined}>
      <div className={s.still}>
        <button type="button" className={s.stillButton} onClick={onPlay ?? onOpen} aria-label={onPlay ? playLabel : openLabel}>
          {still ? <Artwork path={still} shape="landscape" icon={icon} progress={progress} alt="" /> : <span className={s.stillPlaceholder} aria-hidden>{placeholder}</span>}
          {onPlay ? <span className={s.play} aria-hidden><Icon name="play" size={18} /></span> : null}
        </button>
        {onMore ? <button type="button" className={s.more} aria-label={moreLabel} aria-haspopup="menu" onClick={e => onMore(e.currentTarget)}><Icon name="more" size={18} /></button> : null}
        {watched ? <span className={s.watched} title={uiI18n().t('status.watched')}><Icon name="check" size={14} strokeWidth={2.2} /><span className="visually-hidden">{uiI18n().t('status.watched')}</span></span> : null}
      </div>
      <button type="button" className={s.copy} onClick={onOpen} aria-expanded={!!opened} aria-label={tag ? `${openLabel}, ${tag}` : openLabel}>
        <span className={s.titleLine}><span className={s.title}>{number != null ? <span className={s.number}>{number}</span> : null}{title}</span>{tag ? <span className={s.tag}>{tag}</span> : null}</span>
        <span className={s.meta}>{meta || ' '}</span>
        {synopsis ? <span className={s.synopsis}>{synopsis}</span> : null}
      </button>
    </div>
  );
});

export function StillCardPlaceholder() {
  return (
    <div className={s.card} aria-hidden>
      <Skeleton style={{aspectRatio: '16 / 9', width: '100%'}} />
      <div className={s.copy}><Skeleton height={16} width="70%" /><Skeleton height={12} width="30%" /></div>
    </div>
  );
}
