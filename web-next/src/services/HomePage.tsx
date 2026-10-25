import {useEffect, useMemo, useState} from 'react';
import type {HostedSession} from '@core/index.ts';
import {parseJoinLink, type JoinLink} from '@core/join-link.ts';
import {StateView} from '../kit/states';
import {ServicesAuthStage, HostedError, ServicesNotice} from '../kit/accounts/ServicesAuth';
import {ServicesHeader, ServicesStart} from '../kit/accounts/ServicesWorkspace';
import {ServerlessAccountMenu} from '../screens/accounts/ServerlessAccountMenu';
import {AccountPage} from './AccountPage';
import {JoinPage, ShortJoinPage} from './JoinPage';
import {parseShortInvitationCode} from '@core/short-invitation-code.ts';
import {distributionPage} from './distribution';
import {servicesI18n} from './i18n';
import {accountIdentity, type ServicesAccount} from './session';
import {workspaceApi, type WorkspaceServers} from './workspace-api';

/** An empty directory is the drawn welcome, with no Server or library bootstrap. */
export function ServicesHomePage({account, session}: {account: ServicesAccount; session: Omit<HostedSession, 'refreshToken'>}) {
  const identity = accountIdentity(session);
  return <BoundHome key={identity.accountId + '/' + identity.familyId} account={account} session={session} />;
}
function BoundHome({account, session}: {account: ServicesAccount; session: Omit<HostedSession, 'refreshToken'>}) {
  const {t} = useMemo(servicesI18n, []);
  const accountId = session.account.id, familyId = session.familyId!;
  const api = useMemo(() => workspaceApi(account, {accountId, familyId}), [account, accountId, familyId]);
  const [servers, setServers] = useState<WorkspaceServers>(), [error, setError] = useState<unknown>(), [attempt, retry] = useState(0);
  useEffect(() => {
    const controller = new AbortController(); setError(undefined);
    void api.servers(controller.signal).then(value => { if (!controller.signal.aborted) setServers(value); }, value => { if (!controller.signal.aborted) setError(value); });
    return () => controller.abort();
  }, [api, attempt]);
  if (error) return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => retry(value => value + 1)}} />;
  if (!servers) return <ServicesAuthStage><></></ServicesAuthStage>;
  return servers.items.length ? <AccountPage account={account} session={session} section="servers" /> : <Start account={account} session={session} />;
}
function Start({account, session}: {account: ServicesAccount; session: Omit<HostedSession, 'refreshToken'>}) {
  const {t} = useMemo(servicesI18n, []);
  const [invitation, setInvitation] = useState(''), [join, setJoin] = useState<JoinLink>(), [shortJoin, setShortJoin] = useState<string>();
  const [error, setError] = useState<unknown>(), [invalid, setInvalid] = useState(false);
  const inspect = (input = invitation) => {
    const value = input.trim(), parsed = parseJoinLink(value), short = parseShortInvitationCode(value);
    setInvalid(!parsed && !short); setError(undefined);
    if (short) setShortJoin(value);
    if (parsed) setJoin(parsed);
  };
  const run = (work: () => Promise<void>) => { void Promise.resolve().then(work).catch(setError); };
  const download = () => location.assign(distributionPage);
  const scrollToCard = (id: 'join' | 'setup' | 'apps') => {
    const target = document.getElementById(id);
    const visible = id === 'setup' && !target?.getClientRects().length ? document.getElementById('setup-details') : target;
    visible?.scrollIntoView({block: 'start', behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'});
  };
  if (shortJoin !== undefined) return <ShortJoinPage account={account} initialCode={shortJoin} onBack={() => setShortJoin(undefined)} />;
  if (join) return <JoinPage account={account} link={join} onBack={() => setJoin(undefined)} />;
  const name = session.account.displayName || session.account.username;
  return <ServicesStart header={<ServicesHeader notifications={false} avatar={<ServerlessAccountMenu username={session.account.username} name={name} t={t}
    onAccount={() => location.assign('/account')} onJoin={() => scrollToCard('join')} onSetup={() => scrollToCard('setup')} onApps={() => scrollToCard('apps')}
    onHelp={() => window.open('https://getportico.tv/help', '_blank', 'noopener,noreferrer')}
    onSignOut={() => run(async () => {await account.service.logoutIfCurrent(accountIdentity(session));})} />} />}
    invitation={invitation} onInvitationChange={value => { setInvitation(value); setInvalid(false); }} onJoin={() => inspect()}
    onInvitationPaste={value => { setInvitation(value); inspect(value); }}
    onPaste={() => run(async () => { const value = await navigator.clipboard.readText(); setInvitation(value); inspect(value); })}
    onDownload={download} onApps={download} onCopyDownload={() => run(() => navigator.clipboard.writeText(distributionPage))}
    onShareDownload={() => run(async () => { if (navigator.share) await navigator.share({url: distributionPage}); else await navigator.clipboard.writeText(distributionPage); })}
    error={invalid ? <ServicesNotice tone="error">{t('web.link.invalid')}</ServicesNotice> : <HostedError error={error} />} />;
}
