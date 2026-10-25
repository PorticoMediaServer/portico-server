import {useLayoutEffect, useCallback, useMemo, useRef, useState} from 'react';
import type {HostedSession} from '@core/index.ts';
import {parseJoinLink, type JoinLink} from '@core/join-link.ts';
import {StateView} from '../kit/states';
import {ServicesAuthStage, HostedError, ServicesNotice} from '../kit/accounts/ServicesAuth';
import {ServicesHeader, ServicesStart} from '../kit/accounts/ServicesWorkspace';
import {ServerlessAccountMenu} from '../screens/accounts/ServerlessAccountMenu';
import {AccountPage} from './AccountPage';
import {ServicesInvitationDialog, ShortJoinPage} from './JoinPage';
import {parseShortInvitationCode} from '@core/short-invitation-code.ts';
import {distributionPage} from './distribution';
import {servicesI18n} from './i18n';
import {useMembershipEnding} from './use-membership-ending';
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
  type Read = {api: typeof api; attempt: number; active: boolean; controller: AbortController; value?: WorkspaceServers};
  const [loaded, setLoaded] = useState<Read>(), [error, setError] = useState<unknown>(), [attempt, retry] = useState(0);
  const live = useRef<Read | null>(null);
  useLayoutEffect(() => {
    const read: Read = {api, attempt, active: true, controller: new AbortController()}; live.current = read; setError(undefined);
    void api.servers(read.controller.signal).then(value => {
      if (read.active && live.current === read && !read.controller.signal.aborted) {read.value = value; setLoaded(read);}
    }, value => {if (read.active && live.current === read && !read.controller.signal.aborted) setError(value);});
    return () => {read.active = false; if (live.current === read) live.current = null; read.controller.abort();};
  }, [api, attempt]);
  const sourceCurrent = useCallback(() => Boolean(loaded && loaded.api === api && loaded.attempt === attempt && loaded.active && live.current === loaded && !loaded.controller.signal.aborted), [loaded, api, attempt]);
  const servers = sourceCurrent() ? loaded?.value : undefined;
  if (error) return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => retry(value => value + 1)}} />;
  if (!servers) return <ServicesAuthStage><></></ServicesAuthStage>;
  return servers.items.length ? <AccountPage account={account} session={session} section="servers" /> : <Start account={account} session={session} servers={servers} sourceCurrent={sourceCurrent} />;
}
function Start({account, session, servers, sourceCurrent}: {account: ServicesAccount; session: Omit<HostedSession, 'refreshToken'>; servers: WorkspaceServers; sourceCurrent: () => boolean}) {
  const {t} = useMemo(servicesI18n, []);
  const showEnding = useMembershipEnding(account, accountIdentity(session), servers.membershipEnding, sourceCurrent);
  const [invitation, setInvitation] = useState(''), [join, setJoin] = useState<JoinLink>(), [shortJoin, setShortJoin] = useState<string>();
  const [error, setError] = useState<unknown>(), [invalid, setInvalid] = useState(false);
  const inspect = (input = invitation) => {
    const parsed = parseJoinLink(input.trim()), short = parseShortInvitationCode(input);
    setInvalid(!parsed && !short); setError(undefined);
    setShortJoin(short ? input : undefined);
    setJoin(parsed ?? undefined);
  };
  const run = (work: () => Promise<void>) => { void Promise.resolve().then(work).catch(setError); };
  const download = () => location.assign(distributionPage);
  const scrollToCard = (id: 'join' | 'setup' | 'apps') => {
    const target = document.getElementById(id);
    const visible = id === 'setup' && !target?.getClientRects().length ? document.getElementById('setup-details') : target;
    visible?.scrollIntoView({block: 'start', behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'});
  };
  if (shortJoin !== undefined) return <ShortJoinPage account={account} initialCode={shortJoin} onBack={() => setShortJoin(undefined)} />;
  const name = session.account.displayName || session.account.username;
  const ending = servers.membershipEnding;
  return <><ServicesStart returning={Boolean(ending)} notice={showEnding.visible && ending ? <div ref={showEnding.mount}><ServicesNotice tone="info"><b>{t('services.start.endedTitle', {server: ending.serverName ?? t('services.start.thisServer')})}</b> {t('web.chooser.refusedBody')}</ServicesNotice></div> : undefined} header={<ServicesHeader notifications={false} avatar={<ServerlessAccountMenu username={session.account.username} name={name} t={t}
    onAccount={() => location.assign('/account')} onJoin={() => scrollToCard('join')} onSetup={() => scrollToCard('setup')} onApps={() => scrollToCard('apps')}
    onHelp={() => window.open('https://getportico.tv/docs/', '_blank', 'noopener,noreferrer')}
    onSignOut={() => run(async () => {await account.service.logoutIfCurrent(accountIdentity(session));})} />} />}
    invitation={invitation} onInvitationChange={value => { setInvitation(value); setInvalid(false); }} onJoin={() => inspect()}
    onInvitationPaste={value => { setInvitation(value); inspect(value); }}
    onPaste={() => run(async () => { const value = await navigator.clipboard.readText(); setInvitation(value); inspect(value); })}
    onDownload={download} onApps={download} onCopyDownload={() => run(() => navigator.clipboard.writeText(distributionPage))}
    onShareDownload={() => run(async () => { if (navigator.share) await navigator.share({url: distributionPage}); else await navigator.clipboard.writeText(distributionPage); })}
    error={invalid ? <ServicesNotice tone="error">{t('web.link.invalid')}</ServicesNotice> : <HostedError error={error} />} />
    {join ? <ServicesInvitationDialog account={account} link={join} pasted={invitation.trim()} onBack={() => setJoin(undefined)} /> : null}</>;
}
