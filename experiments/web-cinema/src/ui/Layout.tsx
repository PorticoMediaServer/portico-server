import React, {useEffect} from 'react';
import {Link, useRouter} from '@tanstack/react-router';
import {Icon, type IconName} from './Icon';
import {cx} from './cx';

/** Sets the document title for the page ("Movies · Portico"). */
export function useTitle(title: string | undefined) {
  useEffect(() => {
    document.title = title ? `${title} · Portico` : 'Portico';
  }, [title]);
}

export function Page({children, className, title, wide, flush}: {children: React.ReactNode; className?: string; title?: string; wide?: boolean; flush?: boolean}) {
  useTitle(title);
  return <div className={cx('page', wide && 'page-wide', flush && 'page-flush', className)}>{children}</div>;
}

export function BackLink({label = 'Back', to, fallback = '/'}: {label?: string; to?: string; fallback?: string}) {
  const router = useRouter();
  if (to) return <Link to={to as never} className="back-link"><Icon name="chevronLeft" size={18} />{label}</Link>;
  return (
    <button type="button" className="back-link" onClick={() => (window.history.length > 1 ? router.history.back() : void router.navigate({to: fallback as never}))}>
      <Icon name="chevronLeft" size={18} />{label}
    </button>
  );
}

export function PageHeader({title, eyebrow, lede, actions, back, children, size = 'display'}: {title: React.ReactNode; eyebrow?: React.ReactNode; lede?: React.ReactNode; actions?: React.ReactNode; back?: {label?: string; to?: string}; children?: React.ReactNode; size?: 'display' | 'title'}) {
  return (
    <header className="page-header">
      {back ? <BackLink label={back.label} to={back.to} /> : null}
      <div className="page-header-row">
        <div className="page-header-text">
          {eyebrow ? <div className="eyebrow">{eyebrow}</div> : null}
          <h1 className={cx('display', size === 'display' ? 'page-title' : 'page-title-sm')}>{title}</h1>
          {lede ? <p className="page-lede">{lede}</p> : null}
        </div>
        {actions ? <div className="page-actions">{actions}</div> : null}
      </div>
      {children}
    </header>
  );
}

export function Section({title, subtitle, actions, children, className, id}: {title?: React.ReactNode; subtitle?: React.ReactNode; actions?: React.ReactNode; children: React.ReactNode; className?: string; id?: string}) {
  return (
    <section className={cx('section', className)} id={id} aria-labelledby={title && id ? `${id}-title` : undefined}>
      {title || actions ? (
        <div className="section-head">
          <div>
            {title ? <h2 className="section-title" id={id ? `${id}-title` : undefined}>{title}</h2> : null}
            {subtitle ? <p className="section-sub">{subtitle}</p> : null}
          </div>
          {actions ? <div className="section-actions">{actions}</div> : null}
        </div>
      ) : null}
      {children}
    </section>
  );
}

/** A glass panel that groups related rows (settings, console sections). */
export function Panel({children, className, title, actions, padded = true, subtitle}: {children: React.ReactNode; className?: string; title?: React.ReactNode; actions?: React.ReactNode; padded?: boolean; subtitle?: React.ReactNode}) {
  return (
    <div className={cx('panel', padded && 'is-padded', className)}>
      {title || actions ? (
        <div className="panel-head">
          <div className="panel-titles">
            {title ? <h3 className="panel-title">{title}</h3> : null}
            {subtitle ? <p className="panel-sub">{subtitle}</p> : null}
          </div>
          {actions ? <div className="panel-actions">{actions}</div> : null}
        </div>
      ) : null}
      {children}
    </div>
  );
}

/** One setting: label and help on the left, its control on the right (stacks on phones). */
export function Row({label, help, children, icon, onClick, to, danger, trailing, className, stack}: {label: React.ReactNode; help?: React.ReactNode; children?: React.ReactNode; icon?: IconName; onClick?: () => void; to?: string; danger?: boolean; trailing?: React.ReactNode; className?: string; stack?: boolean}) {
  const inner = (
    <>
      {icon ? <span className="row-icon"><Icon name={icon} size={19} /></span> : null}
      <span className="row-text">
        <span className={cx('row-label', danger && 'is-danger')}>{label}</span>
        {help ? <span className="row-help">{help}</span> : null}
      </span>
      {children ? <span className="row-control">{children}</span> : null}
      {trailing}
      {onClick || to ? <Icon name="chevronRight" size={18} className="row-chevron" /> : null}
    </>
  );
  if (to) return <Link to={to as never} className={cx('row', 'is-link', className)}>{inner}</Link>;
  if (onClick) return <button type="button" className={cx('row', 'is-link', className)} onClick={onClick}>{inner}</button>;
  return <div className={cx('row', stack && 'is-stack', className)}>{inner}</div>;
}

export function KeyValue({items, className}: {items: [React.ReactNode, React.ReactNode][]; className?: string}) {
  return (
    <dl className={cx('kv', className)}>
      {items.map(([k, v], i) => <div key={i} className="kv-row"><dt>{k}</dt><dd>{v}</dd></div>)}
    </dl>
  );
}

export function Tabs<T extends string>({value, onChange, tabs, label, className}: {value: T; onChange: (v: T) => void; tabs: readonly {value: T; label: React.ReactNode; count?: number; hidden?: boolean}[]; label: string; className?: string}) {
  const shown = tabs.filter(t => !t.hidden);
  return (
    <div className={cx('tabs', className)} role="tablist" aria-label={label} onKeyDown={e => {
      const i = shown.findIndex(t => t.value === value);
      let j = -1;
      if (e.key === 'ArrowRight') j = (i + 1) % shown.length;
      if (e.key === 'ArrowLeft') j = (i - 1 + shown.length) % shown.length;
      if (e.key === 'Home') j = 0;
      if (e.key === 'End') j = shown.length - 1;
      if (j >= 0) { e.preventDefault(); onChange(shown[j]!.value); (e.currentTarget.children[j] as HTMLElement)?.focus(); }
    }}>
      {shown.map(t => (
        <button key={t.value} type="button" role="tab" aria-selected={t.value === value} tabIndex={t.value === value ? 0 : -1} className={cx('tab', t.value === value && 'is-on')} onClick={() => onChange(t.value)}>
          {t.label}{t.count !== undefined ? <span className="tab-count">{t.count}</span> : null}
        </button>
      ))}
    </div>
  );
}

export function Toolbar({children, className}: {children: React.ReactNode; className?: string}) {
  return <div className={cx('toolbar', className)}>{children}</div>;
}
