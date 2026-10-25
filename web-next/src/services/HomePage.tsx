import {useLayoutEffect, useCallback, useMemo, useRef, useState} from 'react';
import type {HostedSession} from '@core/index.ts';
import {parseJoinLink, type JoinLink} from '@core/join-link.ts';
import {StateView} from '../kit/states';
import {ServicesAuthStage, HostedError, ServicesNotice} from '../kit/accounts/ServicesAuth';
import {ServicesHeader, ServicesStart} from '../kit/accounts/ServicesWorkspace';
import {ServerlessAccountMenu} from '../screens/accounts/ServerlessAccountMenu';
import {ServicesDirectoryPage, ServicesLocalServerPage} from './DirectoryPage';
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
  const [intent,setIntent]=useState<'join'|'setup'|'local'>();
  const reload=useCallback(()=>{setIntent(undefined);retry(value=>value+1);},[]);
  useLayoutEffect(() => {
    const read: Read = {api, attempt, active: true, controller: new AbortController()}; live.current = read; setError(undefined);
    void api.servers(read.controller.signal).then(value => {
      if (read.active && live.current === read && !read.controller.signal.aborted) {read.value = value; setLoaded(read);}
    }, value => {if (read.active && live.current === read && !read.controller.signal.aborted) setError(value);});
    return () => {read.active = false; if (live.current === read) live.current = null; read.controller.abort();};
  }, [api, attempt]);
  const sourceCurrent = useCallback(() => Boolean(loaded && loaded.api === api && loaded.attempt === attempt && loaded.active && live.current === loaded && !loaded.controller.signal.aborted), [loaded, api, attempt]);
  const servers = sourceCurrent() ? loaded?.value : undefined;
  const localCurrent=useCallback(()=>{const current=account.service.getSnapshot().session;return Boolean(sourceCurrent()&&current?.familyId&&current.account.id===accountId&&current.familyId===familyId);},[account,accountId,familyId,sourceCurrent]);
  if (error) return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => retry(value => value + 1)}} />;
  if (!servers) return <ServicesAuthStage><></></ServicesAuthStage>;
  if(intent==='local')return <ServicesLocalServerPage current={localCurrent} onBack={()=>setIntent(undefined)} />;
  if(!servers.items.length||intent==='join'||intent==='setup')return <Start account={account} session={session} servers={servers} sourceCurrent={sourceCurrent} initialSection={intent} />;
  const name=session.account.displayName||session.account.username;
  const header=<ServicesHeader notifications={false} avatar={<ServerlessAccountMenu username={session.account.username} name={name} t={t}
    onAccount={()=>location.assign('/account')} onJoin={()=>setIntent('join')} onSetup={()=>setIntent('setup')} onApps={()=>location.assign(distributionPage)}
    onHelp={()=>window.open('https://getportico.tv/docs/','_blank','noopener,noreferrer')}
    onSignOut={()=>{void account.service.logoutIfCurrent(accountIdentity(session)).catch(setError);}} />} />;
  return <ServicesDirectoryPage account={account} session={session} api={api} initial={servers} sourceCurrent={sourceCurrent} header={header}
    onEmpty={reload} onJoin={()=>setIntent('join')} onSetup={()=>setIntent('setup')} onLocal={()=>setIntent('local')} />;
}
function Start({account, session, servers, sourceCurrent, initialSection}: {account: ServicesAccount; session: Omit<HostedSession, 'refreshToken'>; servers: WorkspaceServers; sourceCurrent: () => boolean; initialSection?: 'join'|'setup'}) {
  const {t} = useMemo(servicesI18n, []);
  const showEnding = useMembershipEnding(account, accountIdentity(session), servers.membershipEnding, sourceCurrent);
  type InvitationOpening = Readonly<{link: JoinLink; pasted: string}>;
  const [invitation, setInvitation] = useState(''), [joinState, setJoinState] = useState<{opening?: InvitationOpening; expired?: boolean}>({}), [shortJoin, setShortJoin] = useState<string>();
  const opening = joinState.opening;
  const [error, setError] = useState<unknown>(), [invalid, setInvalid] = useState(false);
  const inspect = (input = invitation) => {
    const parsed = parseJoinLink(input.trim()), short = parseShortInvitationCode(input);
    setInvalid(!parsed && !short); setError(undefined);
    setShortJoin(short ? input : undefined);
    setJoinState(parsed ? {opening: Object.freeze({link: parsed, pasted: input.trim()})} : {});
  };
  const run = (work: () => Promise<void>) => { void Promise.resolve().then(work).catch(setError); };
  const download = () => location.assign(distributionPage);
  const scrollToCard = (id: 'join' | 'setup' | 'apps') => {
    const target = document.getElementById(id);
    const visible = id === 'setup' && !target?.getClientRects().length ? document.getElementById('setup-details') : target;
    visible?.scrollIntoView({block: 'start', behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'});
  };
  useLayoutEffect(()=>{if(initialSection)scrollToCard(initialSection);},[initialSection]);
  if (shortJoin !== undefined) return <ShortJoinPage account={account} initialCode={shortJoin} onBack={() => setShortJoin(undefined)} />;
  const name = session.account.displayName || session.account.username;
  const ending = servers.membershipEnding;
  return <><ServicesStart returning={Boolean(ending)||Boolean(initialSection)} notice={showEnding.visible && ending ? <div ref={showEnding.mount}><ServicesNotice tone="info"><b>{t('services.start.endedTitle', {server: ending.serverName ?? t('services.start.thisServer')})}</b> {t('web.chooser.refusedBody')}</ServicesNotice></div> : undefined} header={<ServicesHeader notifications={false} avatar={<ServerlessAccountMenu username={session.account.username} name={name} t={t}
    onAccount={() => location.assign('/account')} onJoin={() => scrollToCard('join')} onSetup={() => scrollToCard('setup')} onApps={() => scrollToCard('apps')}
    onHelp={() => window.open('https://getportico.tv/docs/', '_blank', 'noopener,noreferrer')}
    onSignOut={() => run(async () => {await account.service.logoutIfCurrent(accountIdentity(session));})} />} />}
    invitation={invitation} onInvitationChange={value => { setInvitation(value); setInvalid(false); setJoinState(previous => previous.expired ? {} : previous); }} onJoin={() => inspect()}
    onInvitationPaste={value => { setInvitation(value); inspect(value); }}
    onPaste={() => run(async () => { const value = await navigator.clipboard.readText(); setInvitation(value); inspect(value); })}
    onDownload={download} onApps={download} onCopyDownload={() => run(() => navigator.clipboard.writeText(distributionPage))}
    onShareDownload={() => run(async () => { if (navigator.share) await navigator.share({url: distributionPage}); else await navigator.clipboard.writeText(distributionPage); })}
    error={invalid || joinState.expired ? <ServicesNotice tone="error">{t(joinState.expired ? 'accounts.invitation.expiredBody' : 'web.link.invalid')}</ServicesNotice> : <HostedError error={error} />} />
    {opening ? <ServicesInvitationDialog account={account} link={opening.link} pasted={opening.pasted}
      onBack={() => setJoinState(previous => previous.opening === opening ? {} : previous)}
      onUnavailable={() => {
        const current = account.service.getSnapshot().session;
        if (!sourceCurrent() || !current || current.account.id !== session.account.id || current.familyId !== session.familyId) return;
        setJoinState(previous => previous.opening === opening ? {expired: true} : previous);
      }} /> : null}</>;
}
