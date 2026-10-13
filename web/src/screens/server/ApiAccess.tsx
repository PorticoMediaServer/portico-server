import {useEffect, useMemo, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {apiScopes, createApiKey, getApiAccessOptions, listApiKeys, listMemberAccessTokens, revokeApiKey, revokeMemberAccessToken, type ApiAccessOptions, type ApiCredential, type ApiCredentialPage, type ApiScope, type IssuedApiCredential, type NewApiCredential} from '@core/api-access.ts';
import {EXPIRY_DAYS, chosenScopes, credentialFacts, credentialMember, credentialState, credentialWorks, expiryInstant, expiryLabel, scopeTicked, type ExpiryDays} from '@core/server-admin/api-words.ts';
import {QUIET_READ_EVERY_MS, useAction, useRead, type Read} from '../../admin/console';
import {useCursorList} from '../../admin/cursor-read';
import {dependsOn} from '@core/freshness-hub.ts';
import {createOperationIds} from './operation-ids';
import {ListPager} from './ListPager';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Button, Checkbox, ConfirmDialog, Dialog, Input, Notice, Select, SettingsEmpty, SettingsFooter, SettingsGroup, SettingsLoading, SettingsNote, SettingsRow, Status, Text, TextArea} from '../../ui';

// The owner turning members' tokens off, or changing the rate, is a change to the server's settings.
const ACCESS_OPTION_MOVES = [dependsOn.settings('server')];

/** What this server offers for keys and tokens: the scopes, how many there may be, and the rate each gets. */
export function useApiAccessOptions(): Read<ApiAccessOptions> {
  const {api} = useSession();
  return useRead(() => getApiAccessOptions(api), [api], {on: ACCESS_OPTION_MOVES});
}

/** The words a list uses for one kind of credential, so keys and tokens read as themselves. */
type Words = Readonly<{nameHelp: string; none: string; create: string; createTitle: string; copyTitle: string; shownOnce: string; secretLabel: string; revokeTitle: (name: string) => string; revokeBody: string; revoked: string}>;

function useWords(kind: 'key' | 'token'): Words {
  const {t} = useI18n();
  return useMemo(() => (kind === 'key'
    ? {nameHelp: t('web.access.keyNameHelp'), none: t('web.access.noApiKeys'), create: t('web.access.createKey'), createTitle: t('web.access.createKeyTitle'), copyTitle: t('web.access.copyKeyTitle'), shownOnce: t('web.access.keyShownOnce'), secretLabel: t('web.access.apiKey'), revokeTitle: name => t('web.access.revokeTitle', {name}), revokeBody: t('web.access.revokeBody'), revoked: t('api.credential.keyRevoked')}
    : {nameHelp: t('api.tokens.nameHelp'), none: t('api.tokens.none'), create: t('api.tokens.create'), createTitle: t('api.tokens.createTitle'), copyTitle: t('api.tokens.copyTitle'), shownOnce: t('api.tokens.shownOnce'), secretLabel: t('api.tokens.token'), revokeTitle: name => t('web.access.revokeTitle', {name}), revokeBody: t('api.tokens.revokeBody'), revoked: t('api.credential.tokenRevoked')}), [kind, t]);
}

/**
 * One list of keys or tokens: the working ones first, each with what it may do, when it was last
 * used and when it stops; the ones that no longer work behind a quiet "show" so the list stays
 * short. Creating shows the secret once, with Copy. The same list serves the owner's keys, a
 * member's own tokens and the owner's view of everyone's tokens.
 */
export function CredentialList({kind, title, description, read, create, revoke, options, showMember, footnote}: {
  kind: 'key' | 'token';
  title?: string;
  description?: string;
  read: ReturnType<typeof useCursorList<ApiCredential>>;
  /** Absent: this list only shows and revokes (the owner's view of members' tokens). */
  create?: (input: NewApiCredential) => Promise<IssuedApiCredential>;
  revoke: (id: string) => Promise<unknown>;
  options?: ApiAccessOptions;
  /** Name whose token each row is. */
  showMember?: boolean;
  footnote?: string;
}) {
  const i18n = useI18n();
  const {t} = i18n;
  const words = useWords(kind);
  const action = useAction();
  const [adding, setAdding] = useState(false);
  const [revoking, setRevoking] = useState<ApiCredential>();
  const [showEnded, setShowEnded] = useState(false);
  const working = useMemo(() => read.data?.filter(credentialWorks) ?? [], [read.data]);
  const ended = useMemo(() => read.data?.filter(c => !credentialWorks(c)) ?? [], [read.data]);
  const format = useMemo(() => ({relative: (iso: string) => i18n.relativeTime(iso), date: (iso: string) => i18n.date(iso)}), [i18n]);
  const row = (c: ApiCredential) => {
    const state = credentialState(t, id => i18n.has(id), c);
    return (
      <SettingsRow key={c.id} icon="lock" label={c.name} help={showMember ? `${credentialMember(c)} · ${credentialFacts(t, c, format)}` : credentialFacts(t, c, format)}
        state={state ? <Status tone={state.tone}>{state.text}</Status> : undefined}
        control={credentialWorks(c) ? <Button size="sm" variant="ghost" label={t('web.access.revoke')} onClick={() => { action.clear(); setRevoking(c); }} /> : undefined} />
    );
  };
  return (
    <SettingsGroup title={title} description={description} action={create ? <Button size="sm" variant="secondary" icon="plus" label={words.create} disabled={kind === 'token' && !!options && !options.tokensAllowed} onClick={() => { action.clear(); setAdding(true); }} /> : undefined}>
      {read.error ? <SettingsNote><Notice tone={read.data ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {action.notice ? <SettingsNote><Notice tone="success" compact>{action.notice}</Notice></SettingsNote> : null}
      {!read.data && read.loading ? <SettingsLoading thing={t(kind === 'key' ? 'server.thing.apiKeys' : 'server.thing.tokens')} /> : null}
      {working.map(row)}
      {read.data && !working.length ? <SettingsEmpty title={words.none} /> : null}
      {showEnded ? ended.map(row) : null}
      {ended.length ? <SettingsFooter><Button size="sm" variant="ghost" label={showEnded ? t('api.credential.hideEnded') : t('api.credential.showEnded', {count: ended.length})} aria-expanded={showEnded} onClick={() => setShowEnded(v => !v)} /></SettingsFooter> : null}
      {footnote ? <SettingsFooter><Text as="p" variant="caption" tone="tertiary">{footnote}</Text></SettingsFooter> : null}
      {create ? <CreateCredentialDialog open={adding} onClose={() => setAdding(false)} words={words} scopes={options?.scopes.length ? options.scopes : apiScopes} create={create} onCreated={read.reload} /> : null}
      <ConfirmDialog open={!!revoking} onOpenChange={o => !o && setRevoking(undefined)} title={words.revokeTitle(revoking?.name ?? '')} body={words.revokeBody} confirmLabel={t('web.access.revoke')} cancelLabel={t('action.cancel')} busy={action.busy} error={action.error}
        onConfirm={() => void action.run(async () => { if (!revoking) return; await revoke(revoking.id); setRevoking(undefined); read.reload(); }, words.revoked)} />
      <ListPager pages={read} />
    </SettingsGroup>
  );
}

/**
 * Create a key or token: a name, what it may do (ticked from the scopes this server offers, each
 * with one sentence), and when it stops working. The answer is the only time the secret is seen.
 */
function CreateCredentialDialog({open, onClose, words, scopes, create, onCreated}: {open: boolean; onClose: () => void; words: Words; scopes: readonly ApiScope[]; create: (input: NewApiCredential) => Promise<IssuedApiCredential>; onCreated: () => void}) {
  const {t} = useI18n();
  const action = useAction();
  // BE-API-10: one ID per logical write; a retry of the same form reuses it, so the server answers with the key it already made.
  const [opIds] = useState(createOperationIds);
  const [name, setName] = useState('');
  const [ticked, setTicked] = useState<ReadonlySet<ApiScope>>(new Set(['read']));
  const [days, setDays] = useState<ExpiryDays>(0);
  const [issued, setIssued] = useState<IssuedApiCredential>();
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!open) return;
    setName(''); setTicked(new Set(scopes.includes('read') ? ['read'] : [])); setDays(0); setIssued(undefined); setCopied(false); action.clear();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);
  const chosen = chosenScopes(ticked).filter(scope => scopes.includes(scope));
  const [sentAt] = useState(() => new Map<string, number>());
  const firstSent = (payload: unknown) => { const key = JSON.stringify(payload); if (!sentAt.has(key)) sentAt.set(key, Date.now()); return sentAt.get(key)!; };
  const submit = () => void action.run(async () => {
    // The moment the form was first sent decides the expiry, so a retry asks for the same key.
    const payload = {name: name.trim(), scopes: chosen, days};
    const out = await create({operationId: opIds.forPayload(JSON.stringify(payload)), name: payload.name, scopes: chosen, ...(days ? {expiresAt: expiryInstant(days, firstSent(payload))} : {})});
    opIds.release();
    setIssued(out);
    onCreated();
  });
  const copy = () => void navigator.clipboard?.writeText(issued?.secret ?? '').then(() => setCopied(true), () => {});
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()} dismissable={!issued?.secret} title={issued ? words.copyTitle : words.createTitle} width={500}
      actions={issued ? <Button variant="primary" label={issued.secret ? t('web.access.copiedIt') : t('action.done')} onClick={onClose} /> : <><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" label={t('web.saved.create')} disabled={!name.trim() || !chosen.length} loading={action.busy} onClick={submit} /></>}>
      {issued ? (
        <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
          {issued.secret ? (
            <>
              <Notice tone="warning" compact>{words.shownOnce}</Notice>
              <TextArea label={words.secretLabel} readOnly rows={2} value={issued.secret} onFocus={e => e.currentTarget.select()} style={{fontFamily: 'var(--font-mono)', fontSize: 13}} />
              <div style={{display: 'flex', gap: 12, alignItems: 'center'}}><Button size="sm" variant="secondary" icon="copy" label={t('web.twoStep.copy')} onClick={copy} /><span role="status">{copied ? <Text variant="caption" tone="tertiary">{t('api.credential.copied')}</Text> : null}</span></div>
            </>
          ) : <Notice tone="warning" compact>{t('api.credential.secretNotShown')}</Notice>}
        </div>
      ) : (
        <div style={{display: 'flex', flexDirection: 'column', gap: 16}}>
          {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
          <Input label={t('profile.name')} help={words.nameHelp} autoFocus maxLength={80} value={name} onChange={e => setName(e.target.value)} placeholder="Home Assistant" /> {/* lint-strings-allow: an example name, not copy */}
          <div role="group" aria-label={t('web.access.scopeLabel')} style={{display: 'flex', flexDirection: 'column', gap: 8}}>
            <Text as="span" variant="label" tone="secondary">{t('web.access.scopeLabel')}</Text>
            {scopes.map(scope => <Checkbox key={scope} checked={scopeTicked(ticked, scope)} disabled={scope !== 'admin' && ticked.has('admin')} onCheckedChange={on => setTicked(prev => { const next = new Set(prev); if (on) next.add(scope); else next.delete(scope); return next; })} label={t(`api.scope.${scope}`)} help={t(`api.scope.${scope}.help`)} />)}
            <Text as="span" variant="caption" tone="tertiary">{t('api.scopes.note')}</Text>
          </div>
          <Select label={t('api.credential.expiryLabel')} value={String(days)} options={EXPIRY_DAYS.map(d => ({value: String(d), label: expiryLabel(t, d)}))} onChange={e => setDays(Number(e.target.value) as ExpiryDays)} />
        </div>
      )}
    </Dialog>
  );
}

/** A server page of keys or tokens as the cursor list reads it. */
export const credentialPage = (list: Promise<ApiCredentialPage>) => list.then(p => ({items: [...p.items], nextCursor: p.nextCursor ?? ''}));

/** Integrations › API keys: the owner's keys for scripts and other programs. */
export function APIKeysPanel() {
  const {t} = useI18n();
  const {api} = useSession();
  const options = useApiAccessOptions();
  const read = useCursorList(cursor => credentialPage(listApiKeys(api, cursor)), [api], {every: QUIET_READ_EVERY_MS});
  return <CredentialList kind="key" title={t('web.access.apiKeys')} description={t('server.access.keyLede')} read={read} options={options.data} create={input => createApiKey(api, input)} revoke={id => revokeApiKey(api, id)} footnote={t('api.tokens.lifetime')} />;
}

/** Integrations › Members' tokens: every token a member has made for themselves, to see and to end. */
export function MemberTokensPanel() {
  const {t} = useI18n();
  const {api} = useSession();
  const read = useCursorList(cursor => credentialPage(listMemberAccessTokens(api, {cursor})), [api], {every: QUIET_READ_EVERY_MS});
  return <CredentialList kind="token" title={t('api.tokens.memberTitle')} description={t('api.tokens.memberLede')} read={read} revoke={id => revokeMemberAccessToken(api, id)} showMember />;
}


/** The way from another panel to where keys are made. */
export function useOpenApiKeys(): () => void {
  const navigate = useNavigate();
  return () => void navigate({to: '/settings/$section', params: {section: 'server-integrations'}, search: {tab: 'keys'}});
}
