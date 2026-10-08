import React, {useEffect, useState} from 'react';
import QRCode from 'qrcode';
import {cx} from '@/ui/cx';
import s from './Auth.module.css';

/** Brand imagery for the signed-out pages: public-domain film posters, dimmed behind the card. */
const WALL = ['metropolis-1927', 'nosferatu-1922', 'the-general-1926', 'sherlock-jr-1924', 'his-girl-friday-1940', 'charade-1963', 'detour-1945', 'safety-last-1923', 'a-trip-to-the-moon-1902', 'the-kid-1921', 'my-man-godfrey-1936', 'night-of-the-living-dead-1968', 'the-gold-rush-1925', 'royal-wedding-1951', 'carnival-of-souls-1962', 'meet-john-doe-1941', 'the-phantom-of-the-opera-1925', 'faust-1926', 'scarlet-street-1945', 'nothing-sacred-1937', 'the-lost-world-1925', 'd-o-a-1950', 'the-thief-of-bagdad-1924', 'the-man-with-the-golden-arm-1955', 'battleship-potemkin-1925', 'sunrise-a-song-of-two-humans-1927', 'house-on-haunted-hill-1959', 'the-cabinet-of-dr-caligari-1920', 'steamboat-bill-jr-1928', 'the-navigator-1924'];

export function AuthFrame({title, lede, children, wide, foot}: {title: React.ReactNode; lede?: React.ReactNode; children?: React.ReactNode; wide?: boolean; foot?: React.ReactNode}) {
  return (
    <main className={s.frame}>
      <div className={s.wall} aria-hidden>{[...WALL, ...WALL].map((id, i) => <img key={i} src={`/art/posters/m-${id}.webp`} alt="" loading={i < 20 ? 'eager' : 'lazy'} />)}</div>
      <div className={s.veil} aria-hidden />
      <img src="/brand/portico-wordmark-dark.svg" alt="Portico" className={s.brand} />
      <section className={cx(s.card, wide && s.cardWide)} aria-labelledby="auth-title">
        <h1 id="auth-title" className={s.title}>{title}</h1>
        {lede ? <p className={s.lede}>{lede}</p> : null}
        {children ? <div className={s.body}>{children}</div> : null}
      </section>
      {foot ? <p className={s.foot}>{foot}</p> : null}
    </main>
  );
}

export function Qr({value, label}: {value: string; label: string}) {
  const [svg, setSvg] = useState('');
  useEffect(() => { void QRCode.toString(value, {type: 'svg', margin: 0, color: {dark: '#071018', light: '#ffffff'}}).then(setSvg); }, [value]);
  return <div className={s.qr} role="img" aria-label={label} dangerouslySetInnerHTML={{__html: svg}} />;
}
export {s as authStyles};

/** Provider marks for the Sign in with Apple and Google buttons, drawn to each provider's guidelines. */
export function AppleMark() {
  return <svg width="16" height="18" viewBox="0 0 814 1000" aria-hidden><path fill="currentColor" d="M788 341c-6 4-108 62-108 190 0 148 130 200 134 202-1 3-21 72-69 142-43 62-88 124-156 124s-86-40-164-40c-77 0-104 41-167 41s-106-57-156-128C44 790 0 669 0 555c0-184 119-281 237-281 62 0 115 41 154 41 37 0 96-43 167-43 27 0 124 2 188 94zM554 167c29-35 50-83 50-131 0-7-1-14-2-19-48 2-104 32-138 71-27 30-52 78-52 127 0 7 1 15 2 17 3 1 8 1 13 1 43 0 97-29 127-66z" /></svg>;
}
export function GoogleMark() {
  return (
    <svg width="18" height="18" viewBox="0 0 48 48" aria-hidden>
      <path fill="#EA4335" d="M24 9.5c3.5 0 6.6 1.2 9.1 3.6l6.8-6.8C35.8 2.4 30.3 0 24 0 14.6 0 6.6 5.4 2.7 13.3l7.9 6.1C12.5 13.6 17.8 9.5 24 9.5z" />
      <path fill="#4285F4" d="M46.1 24.5c0-1.6-.1-3.1-.4-4.5H24v9h12.4c-.5 2.9-2.2 5.3-4.6 6.9l7.4 5.7c4.3-4 6.9-9.9 6.9-17.1z" />
      <path fill="#FBBC05" d="M10.5 28.6c-.5-1.4-.8-3-.8-4.6s.3-3.2.8-4.6l-7.9-6.1C1 16.6 0 20.2 0 24s1 7.4 2.7 10.7l7.8-6.1z" />
      <path fill="#34A853" d="M24 48c6.5 0 11.9-2.1 15.9-5.8l-7.4-5.7c-2.1 1.4-4.8 2.3-8.5 2.3-6.2 0-11.5-4.2-13.4-9.9l-7.9 6.1C6.6 42.6 14.6 48 24 48z" />
    </svg>
  );
}
