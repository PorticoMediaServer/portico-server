import React from 'react';
import {useI18n} from '../../app/i18n';
import {cx, Wordmark} from '../../ui';
import s from './Auth.module.css';

/** The signed-out frame: atmosphere, a quiet panel, the wordmark, legal links last. */
export function AuthFrame({title, lede, children, wide, legal = true}: {title?: React.ReactNode; lede?: React.ReactNode; children: React.ReactNode; wide?: boolean; legal?: boolean}) {
  const {t} = useI18n();
  return (
    <main className={s.page}>
      <section className={cx(s.panel, wide && s.wide)} aria-labelledby="auth-title">
        <div className={s.brand}><Wordmark height={22} /></div>
        {title ? (
          <div className={s.heading}>
            <h1 id="auth-title" className={s.title}>{title}</h1>
            {lede ? <p className={s.lede}>{lede}</p> : null}
          </div>
        ) : null}
        {children}
        {legal ? (
          <footer className={s.legal}>
            <a href="https://getportico.tv/terms" target="_blank" rel="noopener noreferrer">{t('web.about.terms')}</a>
            <a href="https://getportico.tv/privacy" target="_blank" rel="noopener noreferrer">{t('web.about.privacy')}</a>
          </footer>
        ) : null}
      </section>
    </main>
  );
}
export const authStyles = s;
