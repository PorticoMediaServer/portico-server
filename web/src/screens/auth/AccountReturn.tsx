import {useEffect, useState} from 'react';
import {accountReturnHandoff} from '@core/hosted-security.ts';
import {useI18n} from '../../app/i18n';
import {Button, Notice, Text} from '../../ui';
import {AuthFrame} from './AuthFrame';

/**
 * `/app/account-return#tx=…&code=…` (SEC-01): where a Portico Account sends a provider sign-in
 * (Google, Apple on the web) that an app started. This page runs inside the app's sign-in browser,
 * so it continues straight into the app's own return, which only that browser session receives.
 * The completion code leaves the address bar before anything else happens; a return that isn't
 * well-formed goes nowhere.
 */
export function AccountReturnScreen() {
  const {t} = useI18n();
  const [handoff] = useState(() => (typeof location === 'undefined' ? undefined : accountReturnHandoff(location.href)));
  useEffect(() => {
    try { history.replaceState(history.state, '', location.pathname); } catch { /* history unavailable */ }
    if (handoff) location.replace(handoff);
  }, [handoff]);
  return (
    <AuthFrame title={t('web.accountReturn.title')}>
      {handoff ? (
        <>
          <Text as="p" tone="secondary">{t('web.accountReturn.body')}</Text>
          <Button variant="primary" label={t('web.accountReturn.open')} onClick={() => location.replace(handoff)} />
        </>
      ) : (
        <Notice tone="warning" title={t('web.accountReturn.invalidTitle')}>{t('web.accountReturn.invalid')}</Notice>
      )}
    </AuthFrame>
  );
}
