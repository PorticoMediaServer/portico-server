import {useEffect, useMemo, useState} from 'react';
import type {HostedSession} from '@core/index.ts';
import {parseJoinLink, type JoinLink} from '@core/join-link.ts';
import {StateView} from '../kit/states';
import {ServicesAuthStage, HostedError, ServicesNotice} from '../kit/accounts/ServicesAuth';
import {ServicesHeader, ServicesStart} from '../kit/accounts/ServicesWorkspace';
import {ProfileAvatar} from '../kit/shell/AccountSurfaces';
import {AccountPage} from './AccountPage';
import {JoinPage} from './JoinPage';
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
  const [invitation, setInvitation] = useState(''), [join, setJoin] = useState<JoinLink>();
  const [error, setError] = useState<unknown>(), [invalid, setInvalid] = useState(false);
  const inspect = (input = invitation) => {
    const parsed = parseJoinLink(input.trim());
    setInvalid(!parsed); setError(undefined);
    if (parsed) setJoin(parsed);
  };
  const run = (work: () => Promise<void>) => { void Promise.resolve().then(work).catch(setError); };
  const download = () => location.assign(distributionPage);
  if (join) return <JoinPage account={account} link={join} onBack={() => setJoin(undefined)} />;
  const name = session.account.displayName || session.account.username;
  return <ServicesStart header={<ServicesHeader notifications={false} avatar={<ProfileAvatar initial={[...name][0]?.toLocaleUpperCase() ?? ''} name={name} onClick={() => location.assign('/account')} />} />}
    invitation={invitation} onInvitationChange={value => { setInvitation(value); setInvalid(false); }} onJoin={() => inspect()}
    onInvitationPaste={value => { setInvitation(value); inspect(value); }}
    onPaste={() => run(async () => { const value = await navigator.clipboard.readText(); setInvitation(value); inspect(value); })}
    onDownload={download} onApps={download} onCopyDownload={() => run(() => navigator.clipboard.writeText(distributionPage))}
    onShareDownload={() => run(async () => { if (navigator.share) await navigator.share({url: distributionPage}); else await navigator.clipboard.writeText(distributionPage); })}
    error={invalid ? <ServicesNotice tone="error">{t('web.link.invalid')}</ServicesNotice> : <HostedError error={error} />} />;
}
