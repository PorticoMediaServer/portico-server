import {useMemo, useState} from 'react';
import {contentRatings, keyScopes, parseAccessAPIKey, parseAccessDevicePage, parseInvitation, type AccessAPIKey, type AccessDevice, type AccessTier, type Invitation, type KeyScope, type MemberLimits} from '@core/server-administration.ts';
import {accessListPath, scopeHelp, scopeLabels as scopeLabel, tierLabels as tierLabel} from '@core/server-admin/panel-logic.ts';
export {accessListPath, accessLists, tierLabels} from '@core/server-admin/panel-logic.ts';
import {useAction} from '../../admin/console';
import {createOperationIds} from './operation-ids';
import {nextCursorOf, useCursorList} from '../../admin/cursor-read';
import {ListPager} from './ListPager';
import {useSession} from '../../app/session';
import {currentI18n} from '../../app/i18n';
import {Badge, Button, ConfirmDialog, Dialog, Input, Notice, Select, SettingsGroup, SettingsRow, Status, Switch, Text, TextArea, relative} from '../../ui';

const since = (iso?: string) => (iso ? relative(Date.now() - Date.parse(iso)) : 'never');
const items = <T,>(raw: unknown, each: (x: unknown) => T): T[] => {
  const list = (raw as {items?: unknown})?.items;
  if (!Array.isArray(list)) throw new Error('The administration response does not match this server. Refresh before continuing.');
  return list.map(each);
};
/** A person's limits as rows: streams, quality away from home, and what they may see. Zero or empty means no limit. */
export function LimitsFields({limits, onChange}: {limits: MemberLimits; onChange: (next: MemberLimits) => void}) {
  const t = currentI18n().t;
  return (
    <>
      <SettingsGroup title={t('web.access.limits')} description={t('web.access.zeroNoLimit')}>
        <SettingsRow label={t('web.access.maxStreams')} control={<Input hideLabel label={t('web.access.maxStreams')} type="number" min={0} max={1000} value={limits.maxStreams} onChange={e => onChange({...limits, maxStreams: Math.max(0, Number(e.target.value) || 0)})} style={{width: 100}} />} />
        <SettingsRow label={t('web.access.remoteQuality')} help={t('web.access.remoteQualityHelp')} control={<Input hideLabel label={t('web.access.remoteQuality')} type="number" min={0} max={200} value={Math.round(limits.remoteBitrateKbps / 1000)} onChange={e => onChange({...limits, remoteBitrateKbps: Math.max(0, Number(e.target.value) || 0) * 1000})} style={{width: 100}} />} />
        <SettingsRow label={t('web.access.highestRating')} control={<Select hideLabel label={t('web.access.highestRating')} value={limits.maxContentRating} onChange={e => onChange({...limits, maxContentRating: e.target.value})} options={[{value: '', label: t('limits.noLimit')}, ...contentRatings.map(r => ({value: r, label: r}))]} />} />
        <SettingsRow label={t('web.limits.unrated')} control={<Switch checked={limits.allowUnrated} onCheckedChange={v => onChange({...limits, allowUnrated: v})} label={t('limits.allowUnrated')} />} />
        <SettingsRow label={t('web.limits.hiddenLabels')} help={t('web.access.hiddenLabelsHelp')} stack control={<Input hideLabel label={t('web.limits.hiddenLabels')} value={limits.tagPolicy.deniedLabels.join(', ')} onChange={e => onChange({...limits, tagPolicy: {deniedLabels: e.target.value.split(',').map(x => x.trim()).filter(Boolean)}})} style={{width: '100%'}} />} />
      </SettingsGroup>
      {limits.schedule.windows.length ? <Text variant="caption" tone="tertiary">{t('server.people.scheduleKept', {count: limits.schedule.windows.length, zone: limits.schedule.timezone || t('server.people.serverTime')})}</Text> : null}
    </>
  );
}

/** ONB-08: the join page on this server, or on the hosted web app with the server named. The code stays in the fragment. */
function joinLink(serverBase: string, code: string): string {
  const server = new URL(serverBase).origin;
  const hash = 'code=' + encodeURIComponent(code);
  return server === location.origin ? `${server}/join#${hash}` : `${location.origin}/join#${hash}&server=${encodeURIComponent(server)}`;
}

/** People › Invitations: a link and a code for someone to join with. */
export function InvitationsPanel() {
  const {api} = useSession();
  const t = currentI18n().t;
  const read = useCursorList(async cursor => { const raw = await api.request(accessListPath('invitations', cursor)); return {items: items(raw, parseInvitation), nextCursor: nextCursorOf(raw)}; }, [api]);
  const action = useAction();
  // BE-API-10: one ID per logical write; retries reuse it.
  const [opIds] = useState(createOperationIds);
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState('');
  const [role, setRole] = useState<AccessTier>('member');
  const [issued, setIssued] = useState<Invitation>();
  const create = () => void action.run(async () => { const payload = {email: email.trim(), role, allowedLibraries: [], expiresInHours: 168}; const key = JSON.stringify(payload); const out = parseInvitation(await api.request('/v1/admin/access/invitations', 'POST', {operationId: opIds.forPayload(key), ...payload})); opIds.release(); setIssued(out); setEmail(''); read.reload(); });
  const pending = read.data?.filter(i => i.state === 'pending') ?? [];
  return (
    <SettingsGroup title={t('web.access.invitations')} description={t('web.access.invitationsLede')}>
      {read.error ? <div style={{padding: 12}}><Notice tone={read.data ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {pending.map(i => (
        <SettingsRow key={i.id} icon="mail" label={i.email || currentI18n().t('web.access.invitationUnlabelled')} help={t('web.access.inviteMeta', {role: tierLabel[i.role], date: new Date(i.expiresAt).toLocaleDateString()})}
          control={<Button size="sm" variant="ghost" label={t('web.access.revoke')} disabled={action.busy} onClick={() => void action.run(async () => { const key = JSON.stringify({id: i.id, revision: i.revision}); await api.request(`/v1/admin/access/invitations/${encodeURIComponent(i.id)}/revoke`, 'POST', {expectedRevision: i.revision, operationId: opIds.forPayload(key)}); opIds.release(); read.reload(); }, 'Invitation revoked.')} />} />
      ))}
      {read.data && !pending.length ? <div style={{padding: 16}}><Text variant="caption" tone="tertiary">{t('web.access.noInvitations')}</Text></div> : null}
      <div style={{padding: '8px 16px 16px'}}><Button size="sm" variant="secondary" icon="plus" label={t('web.access.inviteSomeone')} onClick={() => { setOpen(true); setIssued(undefined); action.clear(); }} /></div>
      <Dialog open={open} onOpenChange={setOpen} title={issued ? t('web.access.invitationReady') : t('web.access.inviteSomeone')} width={460}
        actions={issued ? <Button variant="primary" label={t('action.done')} onClick={() => setOpen(false)} /> : <><Button variant="ghost" label={t('action.cancel')} onClick={() => setOpen(false)} /><Button variant="primary" label={t('web.access.createInvitation')} disabled={!!email.trim() && !/^\S+@\S+\.\S+$/.test(email.trim())} loading={action.busy} onClick={create} /></>}>
        {issued ? (
          <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
            <Text as="p" variant="body" tone="secondary">{issued.email ? currentI18n().t('web.access.sendTo', {email: issued.email}) : currentI18n().t('web.access.sendToAnyone')}</Text>
            <TextArea label={t('web.access.inviteLink')} readOnly rows={3} value={joinLink(api.baseUrl, issued.code ?? '')} onFocus={e => e.currentTarget.select()} />
            <div><Button size="sm" variant="secondary" icon="copy" label={t('web.access.copyLink')} onClick={() => void navigator.clipboard?.writeText(joinLink(api.baseUrl, issued.code ?? ''))} /></div>
            <Text as="p" variant="caption" tone="tertiary">Or give them the code on its own: {issued.code}</Text>
          </div>
        ) : (
          <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
            {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
            <Input label={t('web.hostedSecurity.email')} type="email" optional help={currentI18n().t('web.access.emailHelp')} autoFocus value={email} onChange={e => setEmail(e.target.value)} />
            <Select label={t('web.access.roleLabel')} value={role} onChange={e => setRole(e.target.value as AccessTier)} options={[{value: 'member', label: t('web.access.roleMember')}, {value: 'admin', label: t('web.access.roleAdmin')}]} />
          </div>
        )}
      </Dialog>
      <ListPager pages={read} />
    </SettingsGroup>
  );
}

/** An account's devices, in the one place they are managed: approve, block, and see when each was last here. */
export function AccountDevices({accountId}: {accountId: string}) {
  const {api} = useSession();
  const t = currentI18n().t;
  const read = useCursorList(async cursor => { const raw = await api.request(accessListPath('devices', cursor) + '&account=' + encodeURIComponent(accountId)); return {items: [...parseAccessDevicePage(raw).items], nextCursor: nextCursorOf(raw)}; }, [api, accountId]);
  const action = useAction();
  // BE-API-10: one ID per logical write; retries reuse it.
  const [opIds] = useState(createOperationIds);
  const trust = (d: AccessDevice, next: 'approved' | 'blocked') => void action.run(async () => { const key = JSON.stringify({id: d.id, revision: d.revision, trust: next}); await api.request(`/v1/admin/access/devices/${encodeURIComponent(d.id)}/trust`, 'PUT', {expectedRevision: d.revision, operationId: opIds.forPayload(key), trust: next}); opIds.release(); read.reload(); }, next === 'approved' ? 'Device approved.' : 'Device blocked.');
  const mine = read.data;
  const waiting = mine?.filter(d => d.trust === 'pending').length ?? 0;
  return (
    <SettingsGroup title={t('devices.title')} description={waiting ? t('web.access.devicesWaiting', {count: waiting}) : t('web.access.devicesLede')}>
      {read.error ? <div style={{padding: 12}}><Notice tone={read.data ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {action.error ? <div style={{padding: 12}}><Notice tone="error" compact>{action.error}</Notice></div> : null}
      {mine?.map(d => (
        <SettingsRow key={d.id} icon={d.platform === 'tvos' ? 'cast' : d.platform === 'web' ? 'globe' : 'profile'} label={d.name} help={t('web.access.deviceMeta', {platform: d.platform, when: since(d.lastSeenAt)})}
          state={<Status tone={d.trust === 'approved' ? 'healthy' : d.trust === 'blocked' ? 'danger' : 'warning'}>{d.trust === 'approved' ? t('web.access.trustApproved') : d.trust === 'blocked' ? t('web.access.trustBlocked') : t('web.access.trustWaiting')}</Status>}
          control={<div style={{display: 'flex', gap: 8}}>{d.trust !== 'approved' ? <Button size="sm" variant="secondary" label={t('web.devices.approve')} disabled={action.busy} onClick={() => trust(d, 'approved')} /> : null}{d.trust !== 'blocked' ? <Button size="sm" variant="ghost" label={t('web.access.block')} disabled={action.busy} onClick={() => trust(d, 'blocked')} /> : null}</div>} />
      ))}
      {mine && !mine.length ? <div style={{padding: 16}}><Text variant="caption" tone="tertiary">{t('web.devices.none')}</Text></div> : null}
      <ListPager pages={read} />
    </SettingsGroup>
  );
}

/** People › API keys: access for scripts and integrations, shown once when made. */
export function APIKeysPanel() {
  const {api} = useSession();
  const t = currentI18n().t;
  const read = useCursorList(async cursor => { const raw = await api.request(accessListPath('apiKeys', cursor)); return {items: items(raw, parseAccessAPIKey), nextCursor: nextCursorOf(raw)}; }, [api]);
  const action = useAction();
  // BE-API-10: one ID per logical write; retries reuse it.
  const [opIds] = useState(createOperationIds);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [scope, setScope] = useState<KeyScope>('read-only');
  const [issued, setIssued] = useState<AccessAPIKey>();
  const [revoking, setRevoking] = useState<AccessAPIKey>();
  const active = useMemo(() => read.data?.filter(k => !k.revoked) ?? [], [read.data]);
  return (
    <SettingsGroup title={t('web.access.apiKeys')} description={t('server.access.keyLede')}>
      {read.error ? <div style={{padding: 12}}><Notice tone={read.data ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {active.map(k => (
        <SettingsRow key={k.id} icon="lock" label={k.name} help={t('web.access.keyMeta', {access: scopeLabel[k.scope], hint: k.hint, when: since(k.lastUsedAt)})} control={<Button size="sm" variant="ghost" label={t('web.access.revoke')} onClick={() => { action.clear(); setRevoking(k); }} />} />
      ))}
      {read.data && !active.length ? <div style={{padding: 16}}><Text variant="caption" tone="tertiary">{t('web.access.noApiKeys')}</Text></div> : null}
      <div style={{padding: '8px 16px 16px'}}><Button size="sm" variant="secondary" icon="plus" label={t('web.access.createKey')} onClick={() => { setOpen(true); setIssued(undefined); setName(''); action.clear(); }} /></div>
      <Dialog open={open} onOpenChange={setOpen} dismissable={!issued} title={issued ? t('web.access.copyKeyTitle') : t('web.access.createKeyTitle')} width={480}
        actions={issued ? <Button variant="primary" label={t('web.access.copiedIt')} onClick={() => setOpen(false)} /> : <><Button variant="ghost" label={t('action.cancel')} onClick={() => setOpen(false)} /><Button variant="primary" label={t('web.saved.create')} disabled={!name.trim()} loading={action.busy} onClick={() => void action.run(async () => { const payload = {name: name.trim(), scope}; const key = JSON.stringify(payload); setIssued(parseAccessAPIKey(await api.request('/v1/admin/access/api-keys', 'POST', {operationId: opIds.forPayload(key), ...payload}))); opIds.release(); read.reload(); })} /></>}>
        {issued ? (
          <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
            <Notice tone="warning" compact>{t('web.access.keyShownOnce')}</Notice>
            <TextArea label={t('web.access.apiKey')} readOnly rows={2} value={issued.secret ?? ''} onFocus={e => e.currentTarget.select()} />
            <div><Button size="sm" variant="secondary" icon="copy" label={t('web.twoStep.copy')} onClick={() => void navigator.clipboard?.writeText(issued.secret ?? '')} /></div>
          </div>
        ) : (
          <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
            {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
            <Input label={t('profile.name')} help={t('web.access.keyNameHelp')} autoFocus maxLength={80} value={name} onChange={e => setName(e.target.value)} placeholder="Home Assistant" /> {/* lint-strings-allow: an example name, not copy */}
            <Select label={t('web.access.scopeLabel')} value={scope} onChange={e => setScope(e.target.value as KeyScope)} options={keyScopes.map(s => ({value: s, label: scopeLabel[s]}))} />
            <Text variant="caption" tone="tertiary">{scopeHelp[scope]}</Text>
          </div>
        )}
      </Dialog>
      <ConfirmDialog open={!!revoking} onOpenChange={o => !o && setRevoking(undefined)} title={t('web.access.revokeTitle', {name: revoking?.name ?? ''})} body={t('web.access.revokeBody')} confirmLabel={t('web.access.revoke')} busy={action.busy} error={action.error}
        onConfirm={() => void action.run(async () => { if (!revoking) return; const key = JSON.stringify({id: revoking.id, revision: revoking.revision}); await api.request(`/v1/admin/access/api-keys/${encodeURIComponent(revoking.id)}/revoke`, 'POST', {expectedRevision: revoking.revision, operationId: opIds.forPayload(key)}); opIds.release(); setRevoking(undefined); read.reload(); }, 'Key revoked.')} />
      <ListPager pages={read} />
    </SettingsGroup>
  );
}
