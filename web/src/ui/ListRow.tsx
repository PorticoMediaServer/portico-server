import React from 'react';
import {cx} from './cx';
import {Artwork, type ArtworkShape} from './Artwork';
import {Icon, type IconName} from './Icon';
import s from './ListRow.module.css';

/** A list row: leading icon/artwork/index, title + subtitle, trailing meta/actions. */
export function ListRow({title, subtitle, icon, art, artShape = 'poster', index, meta, trailing, trailingIcon, selected, onClick, className, as, children, actions}: {
  title: React.ReactNode; subtitle?: React.ReactNode; icon?: IconName; art?: {path?: string; icon?: IconName; progress?: number}; artShape?: ArtworkShape; index?: number | string; meta?: React.ReactNode; trailing?: React.ReactNode; trailingIcon?: IconName; selected?: boolean; onClick?: () => void; className?: string; as?: 'div' | 'li'; children?: React.ReactNode;
  /** Secondary controls rendered beside the row's own hit area (never nested inside it, which HTML forbids). */
  actions?: React.ReactNode;
}) {
  const Tag: React.ElementType = onClick ? 'button' : (as ?? 'div');
  const row = (
    <Tag type={onClick ? 'button' : undefined} onClick={onClick} className={cx(s.row, !onClick && s.static, selected && s.selected, actions && s.withActions, !actions && className)} aria-current={selected || undefined}>
      {art ? <Artwork path={art.path} shape={artShape} icon={art.icon} progress={art.progress} className={cx(s.art, artShape === 'square' && s.artSquare, artShape === 'landscape' && s.artLandscape)} /> : index !== undefined ? <span className={s.numberPlay}><span className={s.index}>{index}</span>{onClick ? <Icon name="play" size={14} className={s.playIcon} /> : null}</span> : icon ? <span className={s.leading}><Icon name={icon} /></span> : <span />}
      <span className={s.copy}>
        <span className={s.title}>{title}</span>
        {subtitle ? <span className={s.subtitle}>{subtitle}</span> : null}
        {children}
      </span>
      <span className={s.trailing}>
        {meta}
        {trailing}
        {trailingIcon ? <Icon name={trailingIcon} size={18} /> : null}
      </span>
    </Tag>
  );
  if (!actions) return row;
  return (
    <div className={cx(s.host, className)}>
      {row}
      <span className={s.actions}>{actions}</span>
    </div>
  );
}
