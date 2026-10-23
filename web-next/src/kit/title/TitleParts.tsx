import type {HTMLAttributes, ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../controls';
import {Artwork, type ArtworkSource} from '../artwork';
import styles from './title-parts.module.css';

/** Exact title/biography prose from desktop-detail and detail-phone. */
export function TitleProse({variant = 'body', className, ...props}: HTMLAttributes<HTMLParagraphElement> & {variant?: 'body' | 'hero'}) {
  return <p {...props} data-variant={variant} className={[styles.prose, className].filter(Boolean).join(' ')} />;
}
/** An episode opens inline under its row, on the page background. Never an overlay. */
export function EpisodeDetailsLayout({title, artwork, backdrop, context, facts, synopsis, actions, adjacent, credits, onClose}: {
  title: string; artwork?: ArtworkSource; backdrop?: boolean; context: ReactNode; facts: ReactNode;
  synopsis?: string; actions: ReactNode; adjacent?: ReactNode; credits?: ReactNode; onClose: () => void;
}) {
  return <section className={styles.episode} aria-label={defaultI18n.t('title.episodeDetails')}>
    <span className={styles.caret} aria-hidden="true" />
    <Button className={styles.close} variant="icon" icon="i-x" iconOnly aria-label={defaultI18n.t('action.close')} onClick={onClose} />
    <div className={styles.side}><div className={styles.still} data-backdrop={backdrop || undefined}><Artwork source={artwork} alt="" /></div>{adjacent}</div>
    <div className={styles.main}><div className={styles.context}>{context}</div><h3>{title}</h3>{facts}{synopsis && <TitleProse>{synopsis}</TitleProse>}{actions}</div>
    {credits && <div className={styles.credits}>{credits}</div>}
  </section>;
}
