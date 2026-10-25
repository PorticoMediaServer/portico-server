import {useMemo, useState, useSyncExternalStore} from 'react';
import type {JoinLink} from '@core/join-link.ts';
import {AccountInvitationScreen} from '../screens/accounts/AccountInvitation';
import {ShortInvitationEntry} from '../screens/accounts/ShortInvitationEntry';
import {HostedError} from '../kit/accounts/ServicesAuth';
import {ServicesAuthForm} from './AuthForm';
import {servicesInvitationAccount} from './invitation-account';
import {accountIdentity, type ServicesAccount} from './session';

/** The link exists only in this document. Provider navigation requires reopening it. */
export function JoinPage({account, link, onBack}: {account: ServicesAccount; link: JoinLink; onBack: () => void}) {
  const session = useSyncExternalStore(account.service.subscribe, account.service.getSnapshot).session;
  const current = useMemo(() => session ? servicesInvitationAccount(account, session) : undefined, [account, session]);
  const [error, setError] = useState<unknown>();
  const different = () => {
    if (session) void account.service.logoutIfCurrent(accountIdentity(session)).catch(setError);
  };
  return <AccountInvitationScreen link={link} account={current} onBack={onBack} onDifferentAccount={different}
    signedOutContent={<><HostedError error={error} /><ServicesAuthForm account={account} embedded invitation onSignedIn={() => {}} /></>}
    // The canonical invitation controller has retired its temporary Server grant.
    // This opens the proved origin for fresh admission; no credential crosses origins.
    onJoined={destination => location.assign(destination.origin)} />;
}

/** The typed address/code step. It carries only a verified destination out,
 * exactly as JoinPage does; no credential or long link is serialized. */
export function ShortJoinPage({account, onBack, initialCode}: {account: ServicesAccount; onBack: () => void; initialCode?: string}) {
  const session = useSyncExternalStore(account.service.subscribe, account.service.getSnapshot).session;
  const current = useMemo(() => session ? servicesInvitationAccount(account, session) : undefined, [account, session]);
  const [error, setError] = useState<unknown>();
  const different = () => {
    if (session) void account.service.logoutIfCurrent(accountIdentity(session)).catch(setError);
  };
  return <ShortInvitationEntry initialCode={initialCode} account={current} onBack={onBack} onDifferentAccount={different}
    signedOutContent={<><HostedError error={error} /><ServicesAuthForm account={account} embedded invitation onSignedIn={() => {}} /></>}
    onJoined={destination => location.assign(destination.origin)} />;
}
