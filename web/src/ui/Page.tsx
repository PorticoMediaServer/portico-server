import React from 'react';
import {cx} from './cx';
import {Icon} from './Icon';
import s from './Page.module.css';

export function Page({children, className, style}: {children: React.ReactNode; className?: string; style?: React.CSSProperties}) {
  return <div className={cx(s.page, className)} style={style}>{children}</div>;
}

/** Page title block. The title is the h1; eyebrow gives context; actions sit trailing and wrap on phones. */
export function PageHeader({title, eyebrow, subtitle, actions, onBack, backLabel = 'Back', titleAdornment, className}: {title: React.ReactNode; eyebrow?: React.ReactNode; subtitle?: React.ReactNode; actions?: React.ReactNode; onBack?: () => void; backLabel?: string; titleAdornment?: React.ReactNode; className?: string}) {
  return (
    <header className={cx(s.header, className)}>
      <div className={s.headerCopy}>
        {/* The line above the title is always there (the way back, the eyebrow, or nothing), so titles sit at one height on every page. */}
        <div className={s.lead}>
          {onBack ? <button type="button" className={s.backLink} onClick={onBack}><Icon name="back" size={14} />{backLabel}</button> : null}
          {eyebrow ? <span className={s.eyebrow}>{eyebrow}</span> : null}
        </div>
        <div className={s.titleRow}>
          <h1 className={s.title}>{title}</h1>
          {titleAdornment}
        </div>
        {subtitle ? <p className={s.subtitle}>{subtitle}</p> : null}
      </div>
      {actions ? <div className={s.actions}>{actions}</div> : null}
    </header>
  );
}

export function Section({title, subtitle, action, children, className, id}: {title?: React.ReactNode; subtitle?: React.ReactNode; action?: React.ReactNode; children: React.ReactNode; className?: string; id?: string}) {
  return (
    <section className={cx(s.section, className)} id={id}>
      {title ? (
        <header className={s.sectionHead}>
          <div>
            <h2 className={s.sectionTitle}>{title}</h2>
            {subtitle ? <p className={s.sectionSub}>{subtitle}</p> : null}
          </div>
          {action}
        </header>
      ) : null}
      {children}
    </section>
  );
}

export function Inset({children, className, style}: {children: React.ReactNode; className?: string; style?: React.CSSProperties}) {
  return <div className={cx(s.inset, className)} style={style}>{children}</div>;
}
export function Stack({children, className, gap}: {children: React.ReactNode; className?: string; gap?: number}) {
  return <div className={cx(s.stack, className)} style={gap != null ? {gap} : undefined}>{children}</div>;
}
export function Row({children, className, style}: {children: React.ReactNode; className?: string; style?: React.CSSProperties}) {
  return <div className={cx(s.row, className)} style={style}>{children}</div>;
}
export function Surface({children, tone, padless, className, style}: {children: React.ReactNode; tone?: 'soft'; padless?: boolean; className?: string; style?: React.CSSProperties}) {
  return <div className={cx(tone === 'soft' ? s.surfaceSoft : s.surface, padless && s.surfacePadless, className)} style={style}>{children}</div>;
}
export function Divider({className}: {className?: string}) {
  return <hr className={cx(s.divider, className)} />;
}
