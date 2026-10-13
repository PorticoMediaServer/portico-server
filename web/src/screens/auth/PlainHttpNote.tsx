import {useEffect, useState} from 'react';
import {HttpLocalApi} from '@core/index.ts';
import {SECURE_CONNECTION_HELP_URL, plainHttpOutsideHome} from '@core/connection-note.ts';
import {useI18n} from '../../app/i18n';
import s from './Auth.module.css';

/** Asked once per page load, and only when the page itself came over plain HTTP from a server. */
let asked: Promise<boolean> | undefined;
function ask(): Promise<boolean> {
  if (typeof location === 'undefined' || location.protocol !== 'http:' || import.meta.env.VITE_HOSTED_WEB) return Promise.resolve(false);
  asked ??= (() => {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 5000);
    return plainHttpOutsideHome(new HttpLocalApi(location.origin), controller.signal).finally(() => clearTimeout(timer));
  })();
  return asked;
}

/**
 * One quiet line on the signed-out screens when this page was loaded over plain HTTP from outside
 * the owner's home networks (the server says so; the browser does not guess). It never blocks or
 * interrupts: signing in works exactly the same with or without it.
 */
export function PlainHttpNote() {
  const {t} = useI18n();
  const [show, setShow] = useState(false);
  useEffect(() => {
    let active = true;
    void ask().then(yes => { if (active && yes) setShow(true); });
    return () => { active = false; };
  }, []);
  if (!show) return null;
  return (
    <p className={s.plainHttp}>
      {t('signIn.plainHttp.note')}{' '}
      <a href={SECURE_CONNECTION_HELP_URL} target="_blank" rel="noopener noreferrer">{t('signIn.plainHttp.link')}</a>
    </p>
  );
}
