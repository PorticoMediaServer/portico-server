import {useEffect, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {connectPairedServer, inspectDirectServer} from '@core/server-connections.ts';
import {knownServerAddress} from '@core/known-servers.ts';
import {passwordStrength} from '@core/password-strength.ts';
import {browserConnectionEnvironment} from '../../bridge/server-connections';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {signInFailure} from '../../app/sign-in-errors';
import {Button, Input, Loading, Notice, PasswordInput, PasswordStrength} from '../../ui';
import {AuthFrame} from './AuthFrame';
import {AccountForm} from './AccountForm';
import s from './Auth.module.css';

/** Served by a server (bundled), `/join` belongs to that server; on the hosted web app the link names it. */
const isBundled = () => !import.meta.env.VITE_HOSTED_WEB && location.hostname !== 'web.getportico.tv';
const status = (e: unknown) => (e as {status?: number} | undefined)?.status;

/** The invitation-code rules the server applies (`access.Accept`). */
export function validInvitationCode(code: string): boolean { const c = code.trim(); return c.length >= 32 && c.length <= 128 && !/\s/.test(c); }
export function validMemberUsername(username: string): boolean { const u = username.trim(); return u.length >= 3 && u.length <= 64 && !/[\s@]/.test(u); }

/** Accept outcome → what the person should do next (status from `POST /v1/access/invitations/accept`). */
export function joinErrorId(e: unknown): 'web.directJoin.error.expired' | 'web.directJoin.error.taken' | 'web.directJoin.error.invalid' | 'web.directJoin.error.tooMany' | undefined {
  switch (status(e)) {
    case 410: case 404: return 'web.directJoin.error.expired';
    case 409: return 'web.directJoin.error.taken';
    case 400: return 'web.directJoin.error.invalid';
    case 429: return 'web.directJoin.error.tooMany';
    default: return undefined;
  }
}

/**
 * ONB-08: `/join#code=<code>` (optionally `&server=<origin>` on the hosted web app) for a server
 * without Portico Accounts. The invitee sees which server invited them, chooses a username and
 * password (8+ characters, advisory strength), the server creates the member, and they are
 * signed in. The code stays in the fragment and is scrubbed by the caller.
 */
export function DirectJoin({code: linkCode, server}: {code?: string; server?: string}) {
  const i18n = useI18n();
  const navigate = useNavigate();
  const session = useSession();
  const bundled = isBundled();
  const [origin, setOrigin] = useState(() => knownServerAddress(server) ?? (bundled ? location.origin : ''));
  const [serverName, setServerName] = useState<string>();
  // Spec — Hosted at Scale: a server with Portico Accounts admits its invitees by their account.
  const [porticoServer, setPorticoServer] = useState(false);
  const [withPassword, setWithPassword] = useState(false);
  const [looking, setLooking] = useState(false);
  const [code, setCode] = useState(linkCode ?? '');
  const [name, setName] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const address = knownServerAddress(origin.trim().replace(/\/+$/, ''));

  // The preview: which server this is, from the server itself (its identity is pinned first).
  useEffect(() => {
    if (!address) { setServerName(undefined); return; }
    const controller = new AbortController();
    setLooking(true);
    (async () => {
      const env = browserConnectionEnvironment();
      const pin = await inspectDirectServer(address, env, controller.signal);
      const {api, connection} = await connectPairedServer(address, pin, env, controller.signal);
      try { return await api.system(); } finally { connection.dispose(); }
    })().then(info => { if (!controller.signal.aborted) { setServerName(info.name); setPorticoServer(!!(info.hostedAttached ?? info.hostedConfigured)); } }, () => { if (!controller.signal.aborted) setServerName(undefined); }).finally(() => { if (!controller.signal.aborted) setLooking(false); });
    return () => controller.abort();
  }, [address]);

  const mismatch = confirm.length > 0 && confirm !== password;
  const ready = !!address && validInvitationCode(code) && validMemberUsername(username) && passwordStrength(password).acceptable && password === confirm;
  const join = async () => {
    if (!ready || !address) return;
    setBusy(true); setError('');
    const env = browserConnectionEnvironment();
    const controller = new AbortController();
    try {
      const pin = await inspectDirectServer(address, env, controller.signal);
      const {api, connection} = await connectPairedServer(address, pin, env, controller.signal);
      try {
        await api.request('/v1/access/invitations/accept', 'POST', {code: code.trim(), username: username.trim(), password, name: name.trim() || username.trim()});
      } finally { connection.dispose(); }
    } catch (e) {
      const id = joinErrorId(e);
      setError(id ? i18n.t(id) : signInFailure(e, {step: 'password', server: serverName, online: navigator.onLine}, i18n).text);
      setBusy(false);
      return;
    }
    // The member exists: sign in as them on this server, then continue into Portico.
    await session.direct.signIn(username.trim(), password, address);
    setBusy(false);
    void navigate({to: '/'});
  };

  // As a Portico Account: the server checks the code and the account's identity, makes the
  // membership and signs this browser in; the server is added at once.
  const joinPortico = async () => {
    if (!address || !validInvitationCode(code)) return;
    setBusy(true); setError('');
    try {
      if (await session.portico.acceptInvitation(address, code.trim())) void navigate({to: '/'});
    } catch (e) {
      const id = joinErrorId(e);
      setError(id ? i18n.t(id) : signInFailure(e, {step: 'invitation', server: serverName, online: navigator.onLine}, i18n).text);
    } finally { setBusy(false); }
  };
  const portico = !withPassword && (!!session.hosted || porticoServer) && !bundled;
  if (portico) {
    return (
      <AuthFrame title={i18n.t('web.directJoin.title')} lede={serverName ? i18n.t('web.directJoin.bodyPortico', {server: serverName}) : i18n.t('web.directJoin.bodyPorticoUnnamed')}>
        <div className={s.stack}>
          {!knownServerAddress(server) ? <Input label={i18n.t('web.direct.serverAddress')} value={origin} onChange={e => setOrigin(e.target.value)} placeholder="http://192.168.1.20:32500" autoCapitalize="off" autoCorrect="off" spellCheck={false} /> : null}
          {!linkCode ? <Input label={i18n.t('web.directJoin.code')} value={code} onChange={e => setCode(e.target.value)} autoComplete="off" spellCheck={false} code /> : null}
          {session.hosted ? (
            <Button variant="primary" size="lg" block label={i18n.t('web.directJoin.withAccount', {account: session.hosted.account.displayName || session.hosted.account.username})} loading={busy} disabled={busy || !address || !validInvitationCode(code)} onClick={() => void joinPortico()} />
          ) : <AccountForm onSignedIn={() => session.accountSignedIn()} />}
          {error || session.error ? <Notice tone="error">{error || session.error}</Notice> : null}
          <Button variant="link" label={i18n.t('web.directJoin.withPassword')} onClick={() => setWithPassword(true)} />
          <Button variant="link" label={i18n.t('web.link.backToPortico')} to="/" />
        </div>
      </AuthFrame>
    );
  }
  return (
    <AuthFrame title={i18n.t('web.directJoin.title')} lede={serverName ? i18n.t('web.directJoin.body', {server: serverName}) : i18n.t('web.directJoin.bodyUnnamed')}>
      <form className={s.stack} onSubmit={e => { e.preventDefault(); void join(); }}>
        {!bundled && !knownServerAddress(server) ? <Input label={i18n.t('web.direct.serverAddress')} value={origin} onChange={e => setOrigin(e.target.value)} placeholder="http://192.168.1.20:32500" autoCapitalize="off" autoCorrect="off" spellCheck={false} /> : null}
        {looking ? <Loading label={i18n.t('web.directJoin.checking')} /> : null}
        {!linkCode ? <Input label={i18n.t('web.directJoin.code')} value={code} onChange={e => setCode(e.target.value)} autoComplete="off" spellCheck={false} code /> : null}
        <Input label={i18n.t('web.directJoin.name')} help={i18n.t('web.directJoin.nameHelp')} value={name} onChange={e => setName(e.target.value)} autoComplete="name" maxLength={120} optional />
        <Input label={i18n.t('web.directJoin.username')} help={i18n.t('web.directJoin.usernameHelp')} value={username} onChange={e => setUsername(e.target.value)} autoComplete="username" autoCapitalize="off" spellCheck={false} maxLength={64} required />
        <PasswordInput label={i18n.t('web.directJoin.password')} value={password} onChange={e => setPassword(e.target.value)} autoComplete="new-password" aria-describedby="join-password-strength" required />
        <PasswordStrength id="join-password-strength" password={password} />
        <PasswordInput label={i18n.t('web.directJoin.confirm')} value={confirm} onChange={e => setConfirm(e.target.value)} autoComplete="new-password" error={mismatch ? i18n.t('web.directJoin.mismatch') : undefined} required />
        {error || session.error ? <Notice tone="error">{error || session.error}</Notice> : null}
        <Button variant="primary" size="lg" type="submit" block label={i18n.t('web.directJoin.submit')} loading={busy} disabled={!ready || busy} />
        <Button variant="link" label={i18n.t('web.link.backToPortico')} to="/" />
      </form>
    </AuthFrame>
  );
}
