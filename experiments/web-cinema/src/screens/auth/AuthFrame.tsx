import React, {useEffect} from 'react';
import {Link} from '@tanstack/react-router';
import {artKeys} from '../../data/art';
import {Art, Wordmark} from '../../ui/Media';
import {cx} from '../../ui/cx';
import {resetTint} from '../../app/tint';

const posters = [...artKeys].filter(k => k.startsWith('poster/m-') || k.startsWith('poster/s-')).slice(0, 72);

/**
 * The frame for getting in: a slow-drifting wall of the library's own posters behind a glass card,
 * so even the sign-in screen feels like the front of a cinema.
 */
export function AuthFrame({children, wide, full, title, footer, bare}: {children: React.ReactNode; wide?: boolean; full?: boolean; title?: string; footer?: React.ReactNode; bare?: boolean}) {
  useEffect(() => { document.title = title ? `${title} · Portico` : 'Portico'; resetTint(); }, [title]);
  return (
    <div className="auth">
      <div className="auth-wall" aria-hidden>
        {[0, 1, 2, 3, 4, 5].map(col => (
          <div key={col} className={cx('auth-col', col % 2 && 'is-down')} style={{animationDuration: `${90 + col * 12}s`}}>
            {[...posters.slice(col * 12, col * 12 + 12), ...posters.slice(col * 12, col * 12 + 12)].map((p, i) => <Art key={p + i} src={p} ratio="2/3" className="auth-poster" eager={i < 4} />)}
          </div>
        ))}
      </div>
      <div className="auth-scrim" aria-hidden />
      <main className={cx('auth-main', wide && 'is-wide', full && 'is-full')}>
        <Link to="/" className="auth-brand" aria-label="Portico"><Wordmark height={26} /></Link>
        {bare ? children : <div className="auth-card">{children}</div>}
        {footer ? <div className="auth-foot">{footer}</div> : null}
      </main>
    </div>
  );
}
