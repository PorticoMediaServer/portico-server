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
export const StillCard = React.memo(function StillCard({still, placeholder, icon = 'tv', number, title, meta, synopsis, progress, watched, selected, playLabel, openLabel, onPlay, onOpen}: {
  still?: string; /** Shown large on the neutral placeholder when there is no still (the episode code). */ placeholder?: string; icon?: IconName; number?: number | string; title: string; meta?: string; synopsis?: string; progress?: number; watched?: boolean; selected?: boolean;
  /** Accessible names for the two targets, e.g. "Play S1 E4, Pilot". */
  playLabel: string; openLabel: string;
  onPlay?: () => void; onOpen: () => void;
}) {
  return (
    <div className={s.card} data-selected={selected || undefined}>
      <div className={s.still}>
        <button type="button" className={s.stillButton} onClick={onPlay ?? onOpen} aria-label={onPlay ? playLabel : openLabel}>
          {still ? <Artwork path={still} shape="landscape" icon={icon} progress={progress} alt="" /> : <span className={s.stillPlaceholder} aria-hidden>{placeholder}</span>}
          {onPlay ? <span className={s.play} aria-hidden><Icon name="play" size={18} /></span> : null}
        </button>
        {watched ? <span className={s.watched} title={uiI18n().t('status.watched')}><Icon name="check" size={14} strokeWidth={2.2} /><span className="visually-hidden">{uiI18n().t('status.watched')}</span></span> : null}
      </div>
      <button type="button" className={s.copy} onClick={onOpen} aria-haspopup="dialog" aria-label={openLabel}>
        <span className={s.title}>{number != null ? <span className={s.number}>{number}</span> : null}{title}</span>
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
