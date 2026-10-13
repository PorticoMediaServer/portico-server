import {useEffect, useState} from 'react';
import {parseRatingSystems, type RatingSystem} from '@core/identity-restrictions.ts';
import {contentRatings, parseAccessDevicePage, parseInvitation, type AccessDevice, type AccessTier, type Invitation, type MemberLimits} from '@core/server-administration.ts';
import {accessListPath} from '@core/server-admin/panel-logic.ts';
export {accessListPath, accessLists} from '@core/server-admin/panel-logic.ts';
import {VIEWING_HOURS_MAX, dayChoiceOf, daysOfChoice, deviceFacts, newViewingHours, roleLabel, trustApplies, type DayChoice} from '@core/server-admin/people-words.ts';
import {clockMinutes, clockText} from '@core/server-admin/panel-logic.ts';
import {QUIET_READ_EVERY_MS, sendFresh, useAction} from '../../admin/console';
import {createOperationIds} from './operation-ids';
import {findInPages, nextCursorOf, useCursorList} from '../../admin/cursor-read';
import {ListPager} from './ListPager';
import {useSession} from '../../app/session';
import {Button, ConfirmDialog, Dialog, Input, Notice, Select, SettingsEmpty, SettingsFooter, SettingsGroup, SettingsLoading, SettingsNote, SettingsRow, Status, Switch, Text, TextArea, relative} from '../../ui';
import {useI18n} from '../../app/i18n';

const items = <T,>(raw: unknown, each: (x: unknown) => T): T[] => {
  const list = (raw as {items?: unknown})?.items;
  if (!Array.isArray(list)) throw new Error('The administration response does not match this server. Refresh before continuing.');
  return list.map(each);
};
/** The rating systems this server publishes (the one scale accounts and profiles share); empty until read, and when the read fails. */
function useRatingSystems(): readonly RatingSystem[] {
  const {api} = useSession();
  const [systems, setSystems] = useState<readonly RatingSystem[]>([]);
  useEffect(() => {
    const stop = new AbortController();
    api.request<{items?: unknown}>('/v1/rating-systems', 'GET', undefined, stop.signal).then(raw => setSystems(parseRatingSystems(raw?.items)), () => {});
    return () => stop.abort();
  }, [api]);
  return systems;
}

/** A person's limits as rows: streams, quality away from home, and what they may see. Zero or empty means no limit. */
export function LimitsFields({limits, onChange}: {limits: MemberLimits; onChange: (next: MemberLimits) => void}) {
  const {t} = useI18n();
  // The same scale a profile's limits use: a rating system, then a rating in it. A limit saved
  // before systems existed (a plain rating with no system) stays shown as it is until changed.
  const systems = useRatingSystems();
  const system = systems.find(s => s.id === limits.ratingSystem);
  const plain = !limits.ratingSystem && !!limits.maxContentRating;
  const ratings = system ? system.values.map(v => ({value: v.code, label: t('web.limits.ratingOption', {label: v.label, age: v.minimumAge})})) : !systems.length || plain ? contentRatings.map(r => ({value: r, label: r})) : [];
  return (
    <>
      <SettingsGroup title={t('web.access.limits')} description={t('web.access.zeroNoLimit')}>
        <SettingsRow label={t('web.access.maxStreams')} control={<Input hideLabel label={t('web.access.maxStreams')} type="number" min={0} max={1000} value={limits.maxStreams} onChange={e => onChange({...limits, maxStreams: Math.max(0, Number(e.target.value) || 0)})} style={{width: 100}} />} />
        <SettingsRow label={t('web.access.remoteQuality')} help={t('web.access.remoteQualityHelp')} control={<Input hideLabel label={t('web.access.remoteQuality')} type="number" min={0} max={200} value={Math.round(limits.remoteBitrateKbps / 1000)} onChange={e => onChange({...limits, remoteBitrateKbps: Math.max(0, Number(e.target.value) || 0) * 1000})} style={{width: 100}} />} />
        {systems.length ? <SettingsRow label={t('limits.ratingSystem')} control={<Select hideLabel label={t('limits.ratingSystem')} value={limits.ratingSystem} onChange={e => onChange({...limits, ratingSystem: e.target.value, maxContentRating: ''})} options={[...(limits.ratingSystem ? [] : [{value: '', label: t('server.people.ratingSystemChoose')}]), ...systems.map(s => ({value: s.id, label: t('web.limits.systemOption', {name: s.name, region: s.region})}))]} />} /> : null}
        <SettingsRow label={t('web.access.highestRating')} help={systems.length && !system && !plain ? t('server.people.ratingSystemFirst') : undefined} control={<Select hideLabel label={t('web.access.highestRating')} value={limits.maxContentRating} disabled={!ratings.length} onChange={e => onChange({...limits, maxContentRating: e.target.value})} options={[{value: '', label: t('limits.noLimit')}, ...ratings, ...(limits.maxContentRating && !ratings.some(r => r.value === limits.maxContentRating) ? [{value: limits.maxContentRating, label: limits.maxContentRating}] : [])]} />} />
        <SettingsRow label={t('web.limits.unrated')} control={<Switch checked={limits.allowUnrated} onCheckedChange={v => onChange({...limits, allowUnrated: v})} label={t('limits.allowUnrated')} />} />
        <SettingsRow label={t('web.limits.hiddenLabels')} help={t('web.access.hiddenLabelsHelp')} stack control={<Input hideLabel label={t('web.limits.hiddenLabels')} value={limits.tagPolicy.deniedLabels.join(', ')} onChange={e => onChange({...limits, tagPolicy: {deniedLabels: e.target.value.split(',').map(x => x.trim()).filter(Boolean)}})} style={{width: '100%'}} />} />
      </SettingsGroup>
      <ViewingHours limits={limits} onChange={onChange} />
    </>
  );
}

/**
 * An account's viewing hours: when this person can sign in and watch. Each set of hours is a
 * row: which days, from when, until when (an end at or before the start runs into the next
 * day). With none, any time. Saved with the rest of the account's limits by the page's Save.
 */
function ViewingHours({limits, onChange}: {limits: MemberLimits; onChange: (next: MemberLimits) => void}) {
  const i18n = useI18n();
  const {t} = i18n;
  const windows = limits.schedule.windows;
  const set = (next: MemberLimits['schedule']['windows']) => onChange({...limits, schedule: {...limits.schedule, windows: next}});
  const change = (index: number, patch: Partial<MemberLimits['schedule']['windows'][number]>) => set(windows.map((w, i) => (i === index ? {...w, ...patch} : w)));
  // 4 January 1970 was a Sunday: day 0 of the server's week.
  const dayName = (day: number) => i18n.formatDate(new Date(Date.UTC(1970, 0, 4 + day, 12)), {weekday: 'long', timeZone: 'UTC'});
  const dayOptions = (current: readonly number[]) => [
    ...(['all', 'weekdays', 'weekends'] as const).map(value => ({value, label: t(`server.people.days.${value}`)})),
    ...[1, 2, 3, 4, 5, 6, 0].map(day => ({value: String(day), label: dayName(day)})),
    ...(dayChoiceOf(current) === 'several' ? [{value: 'several', label: `${t('server.people.days.several')}: ${i18n.list([1, 2, 3, 4, 5, 6, 0].filter(d => current.includes(d)).map(dayName))}`}] : []),
  ];
  return (
    <SettingsGroup title={t('server.people.hours')} description={t('server.people.hoursLede')} action={<Button size="sm" variant="secondary" icon="plus" label={t('server.people.hoursAdd')} disabled={windows.length >= VIEWING_HOURS_MAX} onClick={() => set([...windows, newViewingHours()])} />}>
      {windows.map((w, i) => (
        <SettingsRow key={i} stack start label={<Select hideLabel label={t('server.people.hoursDays')} value={dayChoiceOf(w.days)} options={dayOptions(w.days)} onChange={e => change(i, {days: daysOfChoice(e.target.value as DayChoice, w.days)})} />}
          control={<div style={{display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center'}}>
            <Text variant="caption" tone="tertiary">{t('server.windows.from')}</Text>
            <Input hideLabel label={t('server.people.hoursFrom')} type="time" value={clockText(w.startMinute)} onChange={e => change(i, {startMinute: clockMinutes(e.target.value)})} style={{width: 120}} />
            <Text variant="caption" tone="tertiary">{t('server.people.hoursUntilWord')}</Text>
            <Input hideLabel label={t('server.people.hoursUntil')} type="time" value={clockText(w.endMinute)} onChange={e => change(i, {endMinute: clockMinutes(e.target.value)})} style={{width: 120}} />
            {w.endMinute <= w.startMinute ? <Text variant="caption" tone="tertiary">{t('server.people.hoursNextDay')}</Text> : null}
            <Button size="sm" variant="ghost" icon="trash" aria-label={t('server.people.hoursRemove')} onClick={() => set(windows.filter((_, n) => n !== i))} />
          </div>} />
      ))}
      {!windows.length ? <SettingsEmpty title={t('server.people.hoursNone')} /> : null}
      {windows.length ? <SettingsFooter><Text variant="caption" tone="tertiary">{limits.schedule.timezone ? t('server.people.hoursZone', {zone: limits.schedule.timezone}) : t('server.people.hoursServerZone')}</Text></SettingsFooter> : null}
    </SettingsGroup>
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
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  const page = async (cursor: string) => { const raw = await api.request(accessListPath('invitations', cursor)); return {items: items(raw, parseInvitation), nextCursor: nextCursorOf(raw)}; };
  // An invitation leaves this list when someone joins with it, and nothing on the feed says so.
  const read = useCursorList(page, [api], {every: QUIET_READ_EVERY_MS});
  const action = useAction();
  // BE-API-10: one ID per logical write; retries reuse it.
  const [opIds] = useState(createOperationIds);
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState('');
  const [porticoUsername, setPorticoUsername] = useState('');
  const [role, setRole] = useState<AccessTier>('member');
  const [issued, setIssued] = useState<Invitation>();
  const [revoking, setRevoking] = useState<Invitation>();
  const create = () => void action.run(async () => { const payload = {email: email.trim(), role, allowedLibraries: [], expiresInHours: 168, ...(porticoUsername.trim() ? {porticoUsername: porticoUsername.trim()} : {})}; const key = JSON.stringify(payload); const out = parseInvitation(await api.request('/v1/admin/access/invitations', 'POST', {operationId: opIds.forPayload(key), ...payload})); opIds.release(); setIssued(out); setEmail(''); setPorticoUsername(''); read.reload(); });
  const revoke = (invitation: Invitation) => action.run(async () => {
    await sendFresh(invitation,
      i => api.request(`/v1/admin/access/invitations/${encodeURIComponent(i.id)}/revoke`, 'POST', {expectedRevision: i.revision, operationId: opIds.forPayload(JSON.stringify({id: i.id, revision: i.revision}))}),
      () => findInPages(page, i => i.id === invitation.id),
      // Used or already revoked in the meantime: there is nothing left to revoke.
      now => (now.state === 'pending' ? true : 'done'));
    opIds.release(); setRevoking(undefined); read.reload();
  }, t('server.people.invitationRevoked'));
  const pending = read.data?.filter(i => i.state === 'pending') ?? [];
  const name = (i: Invitation) => i.email || i.porticoUsername || t('web.access.invitationUnlabelled');
  return (
    <SettingsGroup title={t('web.access.invitations')} description={t('web.access.invitationsLede')} action={<Button size="sm" variant="secondary" icon="plus" label={t('web.access.inviteSomeone')} onClick={() => { setOpen(true); setIssued(undefined); action.clear(); }} />}>
      {read.error ? <SettingsNote><Notice tone={read.data ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {action.notice ? <SettingsNote><Notice tone="success" compact>{action.notice}</Notice></SettingsNote> : null}
      {!read.data && read.loading ? <SettingsLoading thing={t('server.thing.invitations')} /> : null}
      {pending.map(i => (
        <SettingsRow key={i.id} icon="mail" label={name(i)} help={[t('web.access.inviteMeta', {role: roleLabel(t, i.role), date: i18n.date(i.expiresAt)}), i.porticoUsername ? t('server.people.inviteOnlyFor', {username: i.porticoUsername}) : ''].filter(Boolean).join(' · ')}
          control={<Button size="sm" variant="ghost" label={t('web.access.revoke')} onClick={() => { action.clear(); setRevoking(i); }} />} />
      ))}
      {read.data && !pending.length ? <SettingsEmpty title={t('web.access.noInvitations')} /> : null}
      <Dialog open={open} onOpenChange={setOpen} title={issued ? t('web.access.invitationReady') : t('web.access.inviteSomeone')} width={460}
        actions={issued ? <Button variant="primary" label={t('action.done')} onClick={() => setOpen(false)} /> : <><Button variant="ghost" label={t('action.cancel')} onClick={() => setOpen(false)} /><Button variant="primary" label={t('web.access.createInvitation')} disabled={!!email.trim() && !/^\S+@\S+\.\S+$/.test(email.trim())} loading={action.busy} onClick={create} /></>}>
        {issued ? (
          <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
            <Text as="p" variant="body" tone="secondary">{issued.email ? t('web.access.sendTo', {email: issued.email}) : t('web.access.sendToAnyone')}</Text>
            {issued.porticoUsername ? <Text as="p" variant="caption" tone="tertiary">{t('server.people.inviteOnlyFor', {username: issued.porticoUsername})}</Text> : null}
            <TextArea label={t('web.access.inviteLink')} readOnly rows={3} value={joinLink(api.baseUrl, issued.code ?? '')} onFocus={e => e.currentTarget.select()} />
            <div><Button size="sm" variant="secondary" icon="copy" label={t('web.access.copyLink')} onClick={() => void navigator.clipboard?.writeText(joinLink(api.baseUrl, issued.code ?? ''))} /></div>
            <Text as="p" variant="caption" tone="tertiary">{t('server.people.inviteCode', {code: issued.code ?? ''})}</Text>
          </div>
        ) : (
          <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
            {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
            <Input label={t('web.hostedSecurity.email')} type="email" optional help={t('web.access.emailHelp')} autoFocus value={email} onChange={e => setEmail(e.target.value)} />
            <Input label={t('server.people.inviteRecipient')} optional help={t('server.people.inviteRecipientHelp')} autoCapitalize="off" spellCheck={false} maxLength={64} value={porticoUsername} onChange={e => setPorticoUsername(e.target.value)} />
            <Select label={t('web.access.roleLabel')} value={role} onChange={e => setRole(e.target.value as AccessTier)} options={[{value: 'member', label: t('web.access.roleMember')}, {value: 'admin', label: t('web.access.roleAdmin')}]} />
          </div>
        )}
      </Dialog>
      <ConfirmDialog open={!!revoking} onOpenChange={o => !o && setRevoking(undefined)} title={t('server.people.revokeInvitation', {name: revoking ? name(revoking) : ''})} body={t('server.people.revokeInvitationBody')} confirmLabel={t('web.access.revoke')} busy={action.busy} error={action.error || undefined}
        onConfirm={() => { if (revoking) void revoke(revoking); }} />
      <ListPager pages={read} />
    </SettingsGroup>
  );
}

/** An account's devices, in the one place they are managed: approve, block, and see when each was last here. */
export function AccountDevices({accountId}: {/** Absent: everyone's devices, most recently seen first, each with whose it is. */ accountId?: string}) {
  const {t} = useI18n();
  const {api} = useSession();
  const page = async (cursor: string) => { const raw = await api.request(accessListPath('devices', cursor) + (accountId ? '&account=' + encodeURIComponent(accountId) : '')); return {items: [...parseAccessDevicePage(raw).items], nextCursor: nextCursorOf(raw)}; };
  // A device that has just signed in and waits for approval appears here by itself.
  const read = useCursorList(page, [api, accountId], {every: QUIET_READ_EVERY_MS});
  const action = useAction();
  // BE-API-10: one ID per logical write; retries reuse it.
  const [opIds] = useState(createOperationIds);
  const [blocking, setBlocking] = useState<AccessDevice>();
  const trust = (device: AccessDevice, next: 'approved' | 'blocked') => action.run(async () => {
    await sendFresh(device,
      d => api.request(`/v1/admin/access/devices/${encodeURIComponent(d.id)}/trust`, 'PUT', {expectedRevision: d.revision, operationId: opIds.forPayload(JSON.stringify({id: d.id, revision: d.revision, trust: next})), trust: next}),
      () => findInPages(page, d => d.id === device.id),
      now => trustApplies(now, next));
    opIds.release(); setBlocking(undefined); read.reload();
  }, t(next === 'approved' ? 'server.people.deviceApproved' : 'server.people.deviceBlocked'));
  const mine = read.data;
  const waiting = mine?.filter(d => d.trust === 'pending').length ?? 0;
  return (
    <SettingsGroup title={t('devices.title')} description={waiting ? t('web.access.devicesWaiting', {count: waiting}) : t('web.access.devicesLede')}>
      {read.error ? <SettingsNote><Notice tone={read.data ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {action.error && !blocking ? <SettingsNote><Notice tone="error" compact>{action.error}</Notice></SettingsNote> : null}
      {action.notice ? <SettingsNote><Notice tone="success" compact>{action.notice}</Notice></SettingsNote> : null}
      {!mine && read.loading ? <SettingsLoading thing={t('server.thing.devices')} /> : null}
      {mine?.map(d => (
        <SettingsRow key={d.id} icon={d.platform === 'tvos' || d.platform === 'androidtv' ? 'tvDevice' : d.platform === 'web' ? 'browser' : d.platform === 'ios' || d.platform === 'android' ? 'phone' : d.platform === 'api' ? 'lock' : 'profile'} label={d.name} help={deviceFacts(t, d, {showOwner: !accountId, relative: iso => relative(Date.now() - Date.parse(iso))})}
          state={<Status tone={d.trust === 'approved' ? 'healthy' : d.trust === 'blocked' ? 'danger' : 'warning'}>{d.trust === 'approved' ? t('web.access.trustApproved') : d.trust === 'blocked' ? t('web.access.trustBlocked') : t('web.access.trustWaiting')}</Status>}
          control={<div style={{display: 'flex', gap: 8}}>{d.trust !== 'approved' ? <Button size="sm" variant="secondary" label={t('web.devices.approve')} disabled={action.busy} onClick={() => void trust(d, 'approved')} /> : null}{d.trust !== 'blocked' ? <Button size="sm" variant="ghost" label={t('web.access.block')} disabled={action.busy} onClick={() => { action.clear(); setBlocking(d); }} /> : null}</div>} />
      ))}
      {mine && !mine.length ? <SettingsEmpty title={t('web.devices.none')} /> : null}
      <ConfirmDialog open={!!blocking} onOpenChange={o => !o && setBlocking(undefined)} title={t('server.people.blockTitle', {name: blocking?.name ?? ''})} body={t('server.people.blockBody')} confirmLabel={t('web.access.block')} busy={action.busy} error={action.error || undefined}
        onConfirm={() => { if (blocking) void trust(blocking, 'blocked'); }} />
      <ListPager pages={read} />
    </SettingsGroup>
  );
}
