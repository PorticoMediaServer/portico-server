import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {ServicesAuthStage, ServicesHeading, ServicesLead, ServicesActions, ServicesBackLink, ServicesSignedInAccount, HostedError} from '../kit/accounts/ServicesAuth';
import {Button} from '../kit/controls';
import {ServicesAuthForm} from './AuthForm';
import {ServicesInvalidLink} from './ServicesApp';
import {accountIdentity, type ServicesAccount} from './session';
import {continueHostedSignIn, hostedSignInStore, type HostedSignIn} from './sign-in';
import {servicesI18n} from './i18n';

/** Auth5 + connect19b, composed by ruling29. Only Continue issues an answer. */
export function RegisteredSignInPage({account, pending}: {account: ServicesAccount; pending: HostedSignIn}) {
  const {t} = servicesI18n(), session = useSyncExternalStore(account.service.subscribe, account.service.getSnapshot).session;
  const [authenticating, setAuthenticating] = useState(() => !session), [username, setUsername] = useState(session?.account.username ?? '');
  const [busy, setBusy] = useState(false), [error, setError] = useState<unknown>(), [expired, setExpired] = useState(false);
  const active = useRef<AbortController | undefined>(undefined), lastUsername = useRef(session?.account.username ?? '');
  useEffect(() => {
    if (session) lastUsername.current = session.account.username;
    else {setUsername(lastUsername.current); setAuthenticating(true);}
  }, [session]);
  useEffect(() => {
    const timeout = setTimeout(() => {active.current?.abort(); setExpired(true);}, Math.max(0, Date.parse(pending.handoff.expiresAt) - Date.now()));
    return () => {clearTimeout(timeout); active.current?.abort();};
  }, [pending]);
  if (expired || Date.parse(pending.handoff.expiresAt) <= Date.now()) return <ServicesInvalidLink />;
  const back = <ServicesBackLink href={pending.handoff.registration.origin}>{t('services.return.back', {server: pending.serverName})}</ServicesBackLink>;
  const lead = t('services.return.lead', {server: pending.serverName});
  if (!session || authenticating) return <ServicesAuthForm account={account} heading={t('auth.welcomeHome')} lead={lead}
    initialUsername={username} after={back} onSignedIn={() => setAuthenticating(false)} />;
  const act = (work: (signal: AbortSignal) => Promise<void>) => {
    if (busy) return;
    const controller = new AbortController(); active.current = controller; setBusy(true); setError(undefined);
    void work(controller.signal).catch(value => {if (!controller.signal.aborted) setError(value);}).finally(() => {if (!controller.signal.aborted) setBusy(false);});
  };
  return <ServicesAuthStage><ServicesHeading>{t('auth.welcomeHome')}</ServicesHeading><ServicesLead>{lead}</ServicesLead>
    <HostedError error={error} /><ServicesSignedInAccount username={session.account.username} disabled={busy}
      onDifferent={() => act(async () => {await account.service.logoutIfCurrent(accountIdentity(session));})} />
    <ServicesActions><Button variant="primary" loading={busy} disabled={busy} onClick={() => act(signal =>
      continueHostedSignIn(account, pending, accountIdentity(session), hostedSignInStore(), signal, value => location.assign(value)))}>{t('action.continue')}</Button></ServicesActions>{back}
  </ServicesAuthStage>;
}
