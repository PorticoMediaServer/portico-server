import { type ReactNode } from 'react';
import { Link, useNavigate } from 'react-router';
import { ChevronLeft, Ellipsis } from 'lucide-react';
import type { CastCredit, Palette } from '../data/types';
import { IconButton, cls } from './ui';

/** The cinematic header shared by movie, show, episode and home video pages. */
export function DetailHero({
  backdrop,
  logo,
  title,
  eyebrow,
  palette,
  meta,
  badges,
  children,
  aside,
  short,
}: {
  backdrop?: string;
  logo?: string;
  title: string;
  eyebrow?: ReactNode;
  palette?: Palette;
  meta?: ReactNode;
  badges?: ReactNode;
  children?: ReactNode;
  aside?: ReactNode;
  short?: boolean;
}) {
  return (
    <section className={cls('dhero', short && 'short')} style={{ ['--tint' as string]: palette?.base, ['--glow' as string]: palette?.accent }}>
      <div className="dhero-art">{backdrop && <img src={backdrop} alt="" fetchPriority="high" />}</div>
      <div className="dhero-shade" />
      <div className="dhero-content">
        {eyebrow && <div className="dhero-eyebrow">{eyebrow}</div>}
        {logo ? <img className="dhero-logo" src={logo} alt={title} /> : <h1 className="dhero-title">{title}</h1>}
        {logo && <h1 className="sr-only">{title}</h1>}
        {meta && <div className="dhero-meta">{meta}</div>}
        {badges && <div className="dhero-badges">{badges}</div>}
        {children}
      </div>
      {aside && <div className="dhero-aside">{aside}</div>}
    </section>
  );
}

/** Phones: floating glass back button and overflow menu over the hero. */
export function FloatBar({ onMore, moreRef }: { onMore?: (e: React.MouseEvent<HTMLElement>) => void; moreRef?: React.Ref<HTMLButtonElement> }) {
  const navigate = useNavigate();
  return (
    <div className="float-bar">
      <IconButton label="Back" onClick={() => (window.history.length > 1 ? navigate(-1) : navigate('/'))}>
        <ChevronLeft />
      </IconButton>
      <div className="group-r">
        {onMore && (
          <IconButton label="More" onClick={onMore} ref={moreRef}>
            <Ellipsis />
          </IconButton>
        )}
      </div>
    </div>
  );
}

export function ActionTile({ icon, label, onClick, active, to }: { icon: ReactNode; label: string; onClick?: () => void; active?: boolean; to?: string }) {
  const inner = (
    <>
      <span className={cls('atile-icon glass', active && 'on')}>{icon}</span>
      <span className="atile-label">{label}</span>
    </>
  );
  if (to)
    return (
      <Link className="atile" to={to} viewTransition>
        {inner}
      </Link>
    );
  return (
    <button className="atile" onClick={onClick} aria-pressed={active}>
      {inner}
    </button>
  );
}

export function Section({ title, children, action, id, className }: { title?: ReactNode; children: ReactNode; action?: ReactNode; id?: string; className?: string }) {
  return (
    <section className={cls('dsec', className)} id={id}>
      {title && (
        <header className="dsec-head">
          <h2>{title}</h2>
          {action}
        </header>
      )}
      {children}
    </section>
  );
}

const initialsOf = (name: string) =>
  name
    .split(/\s+/)
    .filter((w) => /^[A-ZÀ-Ý]/.test(w))
    .map((w) => w[0])
    .slice(0, 2)
    .join('');

const hue = (s: string) => [...s].reduce((a, c) => (a * 31 + c.charCodeAt(0)) % 360, 7);

export function CastRow({ cast }: { cast: (CastCredit | { personId: string; name: string; role: string })[] }) {
  return (
    <div className="cast-row-list no-scrollbar">
      {cast.map((c) => (
        <Link key={`${c.personId}-${c.role}`} to={`/person/${c.personId}`} className="person-chip" viewTransition>
          <span className="person-face" style={{ ['--h' as string]: hue(c.name) }}>
            {initialsOf(c.name)}
          </span>
          <span className="person-name ellipsis">{c.name}</span>
          <span className="person-role ellipsis">{c.role}</span>
        </Link>
      ))}
    </div>
  );
}

export function InfoGrid({ rows }: { rows: [string, ReactNode][] }) {
  return (
    <dl className="info-grid">
      {rows
        .filter(([, v]) => v !== undefined && v !== null && v !== '')
        .map(([k, v]) => (
          <div key={k}>
            <dt>{k}</dt>
            <dd>{v}</dd>
          </div>
        ))}
    </dl>
  );
}

export function DetailSkeleton() {
  return (
    <div className="page flush" aria-busy="true">
      <section className="dhero">
        <div className="dhero-content">
          <div className="skel" style={{ width: 120, height: 12 }} />
          <div className="skel" style={{ width: 360, height: 80, marginTop: 16, borderRadius: 14 }} />
          <div className="skel" style={{ width: 300, height: 14, marginTop: 20 }} />
          <div style={{ display: 'flex', gap: 12, marginTop: 26 }}>
            <div className="skel" style={{ width: 160, height: 50, borderRadius: 25 }} />
            <div className="skel" style={{ width: 50, height: 50, borderRadius: 25 }} />
          </div>
        </div>
      </section>
      <div className="pad" style={{ display: 'grid', gap: 10, marginTop: 30 }}>
        <div className="skel" style={{ width: '60%', height: 14 }} />
        <div className="skel" style={{ width: '70%', height: 14 }} />
        <div className="skel" style={{ width: '50%', height: 14 }} />
      </div>
    </div>
  );
}
