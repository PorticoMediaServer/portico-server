import React from 'react';
import {Link, useRouter} from '@tanstack/react-router';
import {Icon, type IconName} from './Icon';
import {cx} from './cx';
import s from './Page.module.css';

export function Page({children, flush, className, label}: {children: React.ReactNode; flush?: boolean; className?: string; label?: string}) {
  return <div className={cx(s.page, flush && s.pageFlush, className)} aria-label={label}>{children}</div>;
}

/** Back to the previous page when there is one in this tab, else to `fallback`. */
export function BackLink({fallback = '/', label = 'Back'}: {fallback?: string; label?: string}) {
  const router = useRouter();
  return (
    <Link to={fallback as never} className={s.back} onClick={e => { if (window.history.length > 1 && (window.history.state as {idx?: number} | null)?.idx) { e.preventDefault(); router.history.back(); } }}>
      <Icon name="back" size={16} />{label}
    </Link>
  );
}

export function PageHeader({title, eyebrow, subtitle, actions, back, children}: {title: React.ReactNode; eyebrow?: React.ReactNode; subtitle?: React.ReactNode; actions?: React.ReactNode; back?: string | {to: string; label: string}; children?: React.ReactNode}) {
  return (
    <header className={s.header}>
      <div className={s.headerText}>
        {back ? typeof back === 'string' ? <BackLink fallback={back} /> : <BackLink fallback={back.to} label={back.label} /> : null}
        {eyebrow ? <div className={s.eyebrow}>{eyebrow}</div> : null}
        <h1 className={s.h1}>{title}</h1>
        {subtitle ? <p className={s.sub}>{subtitle}</p> : null}
        {children}
      </div>
      {actions ? <div className={s.actions}>{actions}</div> : null}
    </header>
  );
}

export function Stack({children, className}: {children: React.ReactNode; className?: string}) {
  return <div className={cx(s.stack, className)}>{children}</div>;
}
export function Section({title, action, children, className, id}: {title?: React.ReactNode; action?: React.ReactNode; children: React.ReactNode; className?: string; id?: string}) {
  return (
    <section className={cx(s.section, className)} aria-labelledby={title && id ? id : undefined}>
      {title || action ? <div className={s.sectionHead}>{title ? <h2 className={s.sectionTitle} id={id}>{title}</h2> : <span className={s.grow} />}{action}</div> : null}
      {children}
    </section>
  );
}
/** A row of controls above a list. Sticky toolbars get their backdrop only once they are stuck. */
export function Toolbar({children, sticky, label}: {children: React.ReactNode; sticky?: boolean; label?: string}) {
  const sentinel = React.useRef<HTMLDivElement>(null);
  const [stuck, setStuck] = React.useState(false);
  React.useEffect(() => {
    if (!sticky || !sentinel.current) return;
    const io = new IntersectionObserver(([e]) => setStuck(!e!.isIntersecting), {rootMargin: `-${window.matchMedia('(max-width: 767px)').matches ? 57 : 1}px 0px 0px 0px`});
    io.observe(sentinel.current);
    return () => io.disconnect();
  }, [sticky]);
  return (
    <>
      {sticky ? <div ref={sentinel} aria-hidden style={{height: 1, marginBottom: -1}} /> : null}
      <div className={cx(s.toolbar, sticky && s.toolbarSticky, stuck && s.stuck)} role="toolbar" aria-label={label}>{children}</div>
    </>
  );
}
export const Grow = () => <span className={s.grow} />;
export function Inset({children, narrow, className}: {children: React.ReactNode; narrow?: boolean; className?: string}) {
  return <div className={cx(s.inset, narrow && s.narrow, className)}>{children}</div>;
}

/** Grouped settings-style rows. */
export function Group({title, lede, children, className}: {title?: string; lede?: React.ReactNode; children: React.ReactNode; className?: string}) {
  return (
    <section className={className}>
      {title ? <h2 className={s.groupTitle}>{title}</h2> : null}
      {lede ? <p className={s.groupLede}>{lede}</p> : null}
      <div className={s.group}>{children}</div>
    </section>
  );
}
export function Row({icon, label, help, trailing, to, onClick, wrap, iconNode, className}: {icon?: IconName; iconNode?: React.ReactNode; label: React.ReactNode; help?: React.ReactNode; trailing?: React.ReactNode; to?: string; onClick?: () => void; wrap?: boolean; className?: string}) {
  const body = (
    <>
      {iconNode ?? (icon ? <span className={s.rowIcon}><Icon name={icon} size={19} /></span> : null)}
      <span className={s.rowText}><span className={s.rowLabel}>{label}</span>{help ? <span className={s.rowHelp}>{help}</span> : null}</span>
      {trailing !== undefined || to || onClick ? <span className={s.rowTrail}>{trailing}{to || (onClick && trailing === undefined) ? <Icon name="forward" size={16} /> : null}</span> : null}
    </>
  );
  const cls = cx(s.row, wrap && s.rowWrap, className);
  if (to) return <Link to={to as never} className={cls}>{body}</Link>;
  if (onClick) return <button type="button" className={cls} onClick={onClick}>{body}</button>;
  return <div className={cls}>{body}</div>;
}
