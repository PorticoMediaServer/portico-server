import {useEffect, useRef, useState} from 'react';
import {initializeServer, resumeServerSetup, type SetupSession} from '@core/setup-onboarding.ts';
import {passwordStrength} from '@core/password-strength.ts';
import type {HttpLocalApi, LocalSession, SystemInfo} from '@core/index.ts';
import {ClaimError, claimErrorMessage} from '../../app/claim-connect';
import {useSession} from '../../app/session';
import {currentI18n, useI18n} from '../../app/i18n';
import {errorText} from '../../app/errors';
import {browserSetupDraft} from '../../bridge/setup-codes';
import {Button, Icon, Input, Loading, Notice, PasswordInput, PasswordStrength, Text, cx} from '../../ui';
import {AuthFrame} from './AuthFrame';
import s from './Auth.module.css';

type Mode = 'hosted' | 'local';

/** First-run local setup. Hosted initialization is a restricted pending claim,
 * never a local owner; the old recovery-owner password flow has been retired. */
export function ServerSetup({system}: {system: SystemInfo}) {
  const {t} = useI18n();
  const session = useSession();
  const {direct} = session;
  const [name, setName] = useState(() => defaultServerName(system.name));
  const [mode, setMode] = useState<Mode>('hosted');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [resuming, setResuming] = useState(true);
  const inspecting = useRef(false);
  // The server's identity is pinned before anything secret is exchanged.
  useEffect(() => {
    if (direct.pin || inspecting.current) return;
    inspecting.current = true;
    void direct.inspect().finally(() => { inspecting.current = false; });
  }, [direct.pin, direct]);

  // A setup interrupted by a reload or a closed tab resumes where it stopped.
  useEffect(() => {
    if (!direct.pin) return;
    let live = true;
    const store = browserSetupDraft(direct.pin.serverId);
    void (async () => {
      try {
        const draft = await store.read();
        if (!draft || !live) return;
        const api = await direct.pairedApi();
        const issued = await resumeServerSetup(api, store, AbortSignal.timeout(20000));
        if (live) await acceptSetupSession(api, issued, direct.acceptIssued);
      } catch (e) {
        if (live && e instanceof ClaimError) setError(setupMessage(e));
      } finally {
        if (live) setResuming(false);
      }
    })();
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [direct.pin?.serverId]);

  const mismatch = !!confirm && confirm !== password;
  const strong = passwordStrength(password).acceptable;
  const valid = name.trim().length > 0 && (mode === 'hosted' || (username.trim().length >= 3 && strong && confirm === password));
  const submit = async () => {
    if (!valid || busy || !direct.pin) return;
    setBusy(true);
    setError('');
    try {
      const api = await direct.pairedApi();
      const {setupToken} = await api.request<{serverId: string; setupToken: string}>('/v1/setup/browser', 'POST', {});
      const issued = await initializeServer(api, browserSetupDraft(direct.pin.serverId), {setupToken, name: name.trim(), ...(mode === 'local' ? {username: username.trim(), password} : {}), authMode: mode, interactive: true}, AbortSignal.timeout(30000));
      setPassword('');
      setConfirm('');
      await acceptSetupSession(api, issued, direct.acceptIssued);
    } catch (e) {
      setError(setupMessage(e));
    } finally {
      setBusy(false);
    }
  };

  if (!direct.pin || resuming) {
    return <AuthFrame legal={false}>{session.error && !direct.pin ? <Notice tone="error" action={{label: t('action.tryAgain'), onClick: () => void direct.inspect()}}>{session.error}</Notice> : <Loading label={t('web.setup.preparing')} />}</AuthFrame>;
  }
  return (
    <AuthFrame title={t('web.setup.welcome')} lede={t('web.setup.welcomeLede')} wide>
      <form className={s.stack} onSubmit={e => { e.preventDefault(); void submit(); }}>
        <Input label={t('web.general.serverName')} value={name} onChange={e => setName(e.target.value)} maxLength={80} help={t('web.setup.serverNameHelp')} required />
        <fieldset className={s.setupChoice} role="radiogroup" aria-label={t('web.setup.howSignIn')}>
          <legend className={s.setupLegend}>{t('web.setup.howSignIn')}</legend>
          <ModeCard selected={mode === 'hosted'} onSelect={() => setMode('hosted')} icon="account" title={t('web.direct.useAccount')} badge={t('web.setup.recommended')} body={t('web.setup.hostedBodySecure')} />
          <ModeCard selected={mode === 'local'} onSelect={() => setMode('local')} icon="server" title={t('web.setup.localAccounts')} body={t('web.setup.localBody')} />
        </fieldset>
        {mode === 'local' ? <>
        <Text as="p" variant="bodyStrong">{t('web.setup.recoveryTitleLocal')}</Text>
        <Text as="p" variant="caption" tone="secondary">{t('web.setup.recoveryBodyLocal')}</Text>
        <Input label={t('web.direct.username')} value={username} onChange={e => setUsername(e.target.value)} autoComplete="username" autoCapitalize="off" spellCheck={false} minLength={3} maxLength={64} required />
        <PasswordInput label={t('web.account.password')} value={password} onChange={e => setPassword(e.target.value)} autoComplete="new-password" aria-describedby="setup-password-strength" required />
        <PasswordStrength id="setup-password-strength" password={password} />
        <PasswordInput label={t('web.directJoin.confirm')} value={confirm} onChange={e => setConfirm(e.target.value)} autoComplete="new-password" error={mismatch ? t('web.setup.mismatch') : undefined} required />
        </> : null}
        {error ? <Notice tone="error">{error}</Notice> : null}
        <Button variant="primary" type="submit" size="lg" block label={mode === 'hosted' ? t('web.setup.connectOpen') : t('web.setup.createOwner')} loading={busy} disabled={!valid} />
        {mode === 'hosted' ? <Text as="p" variant="caption" tone="tertiary" center>{t('web.setup.nextConnectSecure')}</Text> : null}
      </form>
    </AuthFrame>
  );
}

function ModeCard({selected, onSelect, icon, title, body, badge}: {selected: boolean; onSelect: () => void; icon: 'account' | 'server'; title: string; body: string; badge?: string}) {
  return (
    <button type="button" role="radio" aria-checked={selected} className={cx(s.setupCard, selected && s.setupCardSelected)} onClick={onSelect}>
      <span className={s.setupCardIcon} aria-hidden><Icon name={icon} size={20} /></span>
      <span className={s.setupCardCopy}>
        <span className={s.setupCardTitle}>{title}{badge ? <span className={s.setupBadge}>{badge}</span> : null}</span>
        <span className={s.setupCardBody}>{body}</span>
      </span>
      <span className={cx(s.setupRadio, selected && s.setupRadioOn)} aria-hidden />
    </button>
  );
}

/** Only a genuine local setup grant can enter the legacy viewer admission path.
 * Pending Hosted setup remains in protected draft storage; it needs the signed
 * setup return controller, not an owner password or a fabricated local grant. */
export async function acceptSetupSession(api: HttpLocalApi, issued: SetupSession, accept: (api: HttpLocalApi, session: LocalSession) => Promise<void>): Promise<void> {
  if ('claimRequired' in issued) throw new ClaimError('claim_unavailable');
  await accept(api, issued);
}

/** A server left named "Portico" is indistinguishable from every other one; suggest the host. */
function defaultServerName(current: string): string {
  if (current && current !== 'Portico') return current;
  const host = location.hostname;
  if (!host || /^(localhost|\d{1,3}(\.\d{1,3}){3}|\[.*\])$/.test(host)) return 'My Portico server';
  return host.replace(/\.(local|lan|home)$/i, '').replace(/[-_]+/g, ' ');
}

/** X-04: setup failures are catalogue copy by code/status, never `error.message`. */
function setupMessage(e: unknown): string {
  const claim = claimErrorMessage(e);
  if (claim) return currentI18n().t(claim);
  const status = (e as {status?: number})?.status;
  // No account exists yet, so 401/403 means the browser reached a server that
  // refuses remote setup — not "sign in again". Everything else (including a
  // weak password the server rejected) goes through the shared presenter.
  if (status === 401 || status === 403) return currentI18n().t('web.setup.notOnNetwork');
  return errorText(e, 'generic', 'action');
}
