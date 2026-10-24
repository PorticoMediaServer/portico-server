import {useEffect, useRef, useState, useSyncExternalStore, type ChangeEvent, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import type {HostedRequestApi} from '@core/portico-servers.ts';
import {AuthError, AuthLinks, AuthStage} from '../../kit/accounts/AuthStage';
import {InvitationUnavailable, LocalInvitationForm, PorticoInvitation} from '../../kit/accounts/Invitation';
import {Button, TextField} from '../../kit/controls';
import {createBrowserAccountEntry, type BrowserAccountEntry} from '../../platform/account-entry';
import styles from '../../kit/accounts/short-invitation-entry.module.css';

const t = defaultI18n.t;
const CODE_PLACEHOLDER = t('web.directJoin.codePlaceholder');
export const shortInvitationCodePlaceholder = CODE_PLACEHOLDER;
const compact = (value: string): string => value.replace(/[\s-]/g, '').slice(0, 16);
const grouped = (value: string): string => compact(value).replace(/(.{4})(?=.)/g, '$1 ');
export const shortInvitationCodeValue = (value: string): string => compact(value);
export const shortInvitationCodeDisplay = (value: string): string => grouped(value);
/** The two approved generic sentences; everything else keeps the typed error presentation. */
export function shortInvitationFormErrorKind(phase: string, error: unknown, invitation: unknown, busy: boolean): 'invalid' | 'unavailable' | 'typed' | undefined {
  if (!error || busy) return undefined;
  const code = typeof error === 'object' && 'code' in error ? error.code : undefined;
  if (phase === 'address') return code === 'invitation_incomplete' ? 'invalid' : 'typed';
  if ((phase === 'password' || phase === 'portico') && !invitation) {
    const status = typeof error === 'object' && 'status' in error ? error.status : undefined;
    return typeof status === 'number' && status >= 400 && status < 500 && status !== 429 ? 'unavailable' : 'typed';
  }
  return undefined;
}

export type ShortInvitationEntryProps = Readonly<{
  onBack: () => void;
  onJoined: (destination: Readonly<{origin: string; signIn: 'local' | 'portico'}>) => void;
  account?: Readonly<{origin: string; accountId: string; username: string; request: HostedRequestApi['request']; isCurrent: () => boolean}>;
  signedOutContent?: ReactNode;
  onDifferentAccount?: () => void;
  /** Test seam only; the real entry keeps its own browser custody. */
  createEntry?: () => BrowserAccountEntry;
  initialAddress?: string;
}>;

/** The ruling-10 initial Join step. The same entry instance carries the
 * verified origin into the existing local/Portico invitation acceptance. */
export function ShortInvitationEntry(props: ShortInvitationEntryProps) {
  const {createEntry} = props;
  const [entry, setEntry] = useState<BrowserAccountEntry>();
  useEffect(() => {
    const current = (createEntry ?? (() => createBrowserAccountEntry({invitationDestination: 'server-origin'})))();
    setEntry(current);
    return () => current.dispose();
  }, [createEntry]);
  return entry ? <ShortInvitationOperation {...props} entry={entry} /> : null;
}

function ShortInvitationOperation({entry, onBack, onJoined, account, signedOutContent, onDifferentAccount, initialAddress = ''}: ShortInvitationEntryProps & {entry: BrowserAccountEntry}) {
  const state = useSyncExternalStore(entry.subscribe, entry.getSnapshot);
  const [address, setAddress] = useState(initialAddress), [code, setCode] = useState('');
  const delivered = useRef<unknown>(undefined);
  useEffect(() => {
    if (state.phase !== 'joined' || !state.joined || delivered.current === state.joined) return;
    delivered.current = state.joined;
    onJoined(state.joined);
  }, [state.phase, state.joined, onJoined]);
  const busy = state.busy || state.phase === 'checking';
  const back = () => {if (busy) return; entry.cancel(); onBack();};
  const errorKind = shortInvitationFormErrorKind(state.phase, state.error, state.invitation, busy);
  const formError = errorKind === 'invalid' ? <p className={styles.error} role="alert">{t('web.directJoin.entryInvalid')}</p>
    : errorKind === 'unavailable' ? <p className={styles.error} role="alert">{t('web.directJoin.entryUnavailable')}</p>
    : errorKind === 'typed' ? <AuthError error={state.error} /> : undefined;
  if (state.phase === 'joined') return null;
  if (state.invitation && state.server) {
    const returnBack = () => {if (state.busy) return;entry.cancel();};
    if (state.invitation.signInMethod === 'local') return <LocalInvitationForm serverName={state.server.name} inviter={state.invitation.inviterUsername}
      libraries={state.invitation.libraries.map(library => library.name)} busy={state.busy} error={state.error}
      onJoin={(username, password, name) => void entry.joinLocal(username, password, name)} onBack={returnBack} />;
    return <PorticoInvitation serverName={state.server.name} libraries={state.invitation.libraries} member={state.invitation.role === 'member'}
      account={account?.isCurrent() ? account : undefined} signedOutContent={signedOutContent} busy={state.busy} error={state.error}
      onJoin={() => {if (account?.isCurrent()) void entry.joinPortico(account);}} onDifferentAccount={() => {if (state.busy) return;entry.cancel();onDifferentAccount?.();}} />;
  }
  const codeStatus = state.error && typeof state.error === 'object' && 'code' in state.error ? state.error.code : undefined;
  if (codeStatus === 'invitation_expired' || codeStatus === 'invitation_used' || codeStatus === 'invitation_for_another_account') {
    return <InvitationUnavailable otherAccount={codeStatus === 'invitation_for_another_account'} onBack={back} onDifferentAccount={() => {if (state.busy) return;entry.cancel();onDifferentAccount?.();}} />;
  }
  const change = (apply: (value: string) => void) => (event: ChangeEvent<HTMLInputElement>) => {
    if (busy || state.error) entry.cancel();
    apply(event.currentTarget.value);
  };
  return <AuthStage phoneShape="top" className={styles.stage} contentClassName={styles.content}>
    <h1>{t('web.directJoin.title')}</h1>
    <p className={styles.lede}>{t('web.directJoin.entryBody')}</p>
    {formError}
    <form className={styles.form} onSubmit={event => {
      event.preventDefault();
      if (busy || !address.trim() || !code) return;
      void entry.inspectShortInvitation(address.trim(), code);
    }}>
      <TextField variant="auth" label={t('web.direct.serverAddress')} name="server" autoComplete="url" inputMode="url" autoCapitalize="none" spellCheck={false}
        value={address} onChange={change(setAddress)} disabled={busy} required />
      <TextField variant="auth" label={t('web.directJoin.code')} name="code" autoComplete="off" inputMode="text" autoCapitalize="characters" spellCheck={false}
        placeholder={CODE_PLACEHOLDER} maxLength={19} value={shortInvitationCodeDisplay(code)} onChange={change(value => setCode(shortInvitationCodeValue(value)))} disabled={busy}
        required />
      <Button className={styles.submit} variant="primary" type="submit" loading={busy} disabled={!address.trim() || !code}>{t('action.continue')}</Button>
      {busy && <p className={styles.checking} role="status">{t('web.directJoin.checking')}</p>}
    </form>
    <AuthLinks><button type="button" disabled={busy} onClick={back}>{t('action.back')}</button></AuthLinks>
  </AuthStage>;
}
