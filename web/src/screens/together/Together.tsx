import React, {useEffect, useMemo, useState} from 'react';
import type {Group, GroupHostAuthority, GroupMember, GroupQueueEntry, MediaItem} from '@core/index.ts';
import {groupTargetPositionUs} from '@core/index.ts';
import {useSession} from '../../app/session';
import {takePendingTogether, useTogether} from '../../app/together';
import {takePendingTogetherCode} from '../../app/link-fragment';
import {usePlayerOptional} from '../../player/engine';
import {defaultI18n} from '@i18n';
import {Artwork, Badge, Button, ConfirmDialog, IconButton, Input, Inset, ListRow, Loading, Notice, Page, PageHeader, Segmented, StateView, Surface, Text} from '../../ui';

const t = defaultI18n.t;
const authorityOptions = () => [{id: 'host-only' as const, label: t('web.together.onlyMe')}, {id: 'anyone' as const, label: t('web.together.everyone')}];
function stateLabel(state: Group['state']): string {
  switch (state) {
    case 'lobby': return t('web.together.state.lobby');
    case 'preparing': return t('web.together.state.preparing');
    case 'ready': return t('web.together.state.ready');
    case 'playing': case 'degraded': return t('web.together.state.playing');
    case 'paused': return t('web.downloads.state.paused');
    case 'host-reconnecting-playing': case 'host-reconnecting-paused': return t('web.together.state.hostReconnecting');
    case 'ending': return t('web.together.state.ending');
    default: return t('web.together.state.ended');
  }
}
function endedReason(reason: string): string {
  return reason === 'host-ended' ? t('web.together.ended.host') : reason === 'host-unavailable' ? t('web.together.ended.away') : reason === 'empty' ? t('web.together.ended.empty') : t('web.together.ended.other');
}
const clock = (seconds: number) => { const s = Math.max(0, Math.floor(seconds)); const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60); return `${h ? h + ':' + String(m).padStart(2, '0') : m}:${String(s % 60).padStart(2, '0')}`; };

/** Titles for the ids a group carries. A group never sends titles: what each member may see
 * differs, so every member names entries from their own library. */
function useTitles(ids: readonly string[]) {
  const {api} = useSession();
  const [items, setItems] = useState<Record<string, MediaItem | null>>({});
  const key = [...new Set(ids)].sort().join(',');
  useEffect(() => {
    let live = true;
    for (const id of key ? key.split(',') : []) {
      if (id in items) continue;
      api.item(id).then(item => { if (live) setItems(prev => ({...prev, [id]: item})); }, () => { if (live) setItems(prev => ({...prev, [id]: null})); });
    }
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, key]);
  return items;
}

export function TogetherScreen() {
  const {service, state} = useTogether();
  useEffect(() => { void service?.refreshDirectory(); }, [service]);
  const inRoom = state.group && state.phase !== 'idle' && state.phase !== 'left' && state.phase !== 'joining';
  // WEB-TOGETHER-01: a title's "Watch Together" arrives here; it plays once the group has started.
  const [pending, setPending] = useState(takePendingTogether);
  useEffect(() => {
    if (!pending || !inRoom || !state.group?.permissions.canManageQueue) return;
    setPending(undefined);
    void service?.watch(pending.itemId);
  }, [pending, inRoom, state.group?.permissions.canManageQueue, service]);
  if (!service) return null;
  return (
    <Page>
      {/* WEB-TOGETHER-01 (layout-preserving): a plain subtitle instead of an all-caps eyebrow sentence. */}
      <PageHeader title={inRoom ? state.group!.name : t('web.settings.together')} eyebrow={inRoom ? t('web.settings.together') : undefined} subtitle={inRoom ? undefined : t('web.together.subtitle')} />
      <Inset>
        <div style={{display: 'flex', flexDirection: 'column', gap: 20, maxWidth: 860}}>
          {state.error ? <Notice tone="warning" action={state.refused && state.group?.permissions.isHost ? {label: t('web.together.goAhead'), onClick: () => void service.transport(state.refused!.command, {...state.refused!.input, allowUnavailable: true})} : undefined}>{state.refused ? t('web.together.refused') : state.error}</Notice> : null}
          {pending && !inRoom ? <Notice tone="info" compact>{t('web.together.pendingTitle', {title: pending.title})}</Notice> : null}
          {inRoom ? <Room /> : <Lobby />}
        </div>
      </Inset>
    </Page>
  );
}

function Lobby() {
  const {service, state} = useTogether();
  const {session, hosted, local} = useSession();
  const profileId = session?.viewer?.profileId;
  const mine = hosted?.profiles.find(p => p.id === profileId)?.name ?? local?.profiles.find(p => p.id === profileId)?.name ?? '';
  const [name, setName] = useState(mine ? t('web.together.defaultName', {name: mine}) : '');
  const [who, setWho] = useState(mine);
  const [authority, setAuthority] = useState<GroupHostAuthority>('host-only');
  const [code, setCode] = useState(() => takePendingTogetherCode() ?? '');
  useEffect(() => { if (mine && !who) setWho(mine); }, [mine, who]);
  const groupName = name.trim() || (who.trim() ? t('web.together.defaultName', {name: who.trim()}) : t('web.settings.together'));
  if (!service) return null;
  const joining = state.phase === 'joining' || state.busy;
  return (
    <>
      <Surface>
        <div style={{display: 'flex', flexDirection: 'column', gap: 16}}>
          <Text variant="heading">{t('web.together.yourName')}</Text>
          <Input label={t('web.together.shownToOthers')} value={who} maxLength={64} onChange={e => setWho(e.target.value)} />
        </div>
      </Surface>
      <div style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))', gap: 20}}>
        <Surface>
          <form style={{display: 'flex', flexDirection: 'column', gap: 16}} onSubmit={e => { e.preventDefault(); if (code.trim()) void service.join(code, who); }}>
            <Text variant="heading">{t('web.together.joinTitle')}</Text>
            <Text variant="caption" tone="tertiary">{t('web.together.joinHelp')}</Text>
            <Input label={t('web.twoStep.code')} value={code} autoCapitalize="characters" autoComplete="off" spellCheck={false} maxLength={12} placeholder={t('web.together.codeExample')} onChange={e => setCode(e.target.value.toUpperCase())} style={{letterSpacing: '0.18em', fontVariantNumeric: 'tabular-nums'}} />
            <div><Button type="submit" variant="secondary" label={t('web.together.join')} loading={joining} disabled={code.replace(/[\s-]/g, '').length < 4 || joining} /></div>
          </form>
        </Surface>
        <Surface>
          <form style={{display: 'flex', flexDirection: 'column', gap: 16}} onSubmit={e => { e.preventDefault(); void service.create({name: groupName, displayName: who, hostAuthority: authority}); }}>
            <Text variant="heading">{t('web.together.startTitle')}</Text>
            <Text variant="caption" tone="tertiary">{t('web.together.startHelp')}</Text>
            <Input label={t('profile.name')} value={name} maxLength={80} onChange={e => setName(e.target.value)} />
            <div style={{display: 'flex', flexDirection: 'column', gap: 8}}>
              <Text variant="captionStrong" tone="secondary">{t('web.together.whoControls')}</Text>
              <Segmented label={t('web.together.whoControlsShort')} options={authorityOptions()} value={authority} onChange={setAuthority} />
            </div>
            <div><Button type="submit" variant="primary" label={t('web.together.start')} loading={joining} disabled={joining} /></div>
          </form>
        </Surface>
      </div>
      {state.directoryPhase === 'loading' ? <Loading label={t('status.loadingThing', {thing: t('web.together.groupsThing')})} /> : null}
      {state.directoryError && !state.directory.length ? <Notice tone="info">{state.directoryError}</Notice> : null}
      {state.directory.length ? (
        <div style={{display: 'flex', flexDirection: 'column', gap: 8}}>
          <Text variant="label" tone="secondary">{t('web.together.yourGroups')}</Text>
          <Surface padless>
            {state.directory.map(g => <ListRow key={g.id} icon="people" title={g.name} subtitle={t('web.together.groupLine', {count: g.members.filter(m => m.state === 'joined').length, state: stateLabel(g.state)})} meta={g.permissions.isHost ? t('web.together.youHost') : undefined} trailingIcon="forward" onClick={() => void service.open(g.id)} />)}
          </Surface>
        </div>
      ) : null}
    </>
  );
}

function Room() {
  const {service, state} = useTogether();
  const engine = usePlayerOptional();
  const group = state.group!;
  const [confirm, setConfirm] = useState<'end' | 'leave' | null>(null);
  const [, tick] = useState(0);
  useEffect(() => { const id = setInterval(() => tick(n => n + 1), 1000); return () => clearInterval(id); }, []);
  useEffect(() => { void service?.refreshQueue(); }, [service, group.queueRevision]);
  const queue = state.queue;
  const ids = useMemo(() => [group.timeline.itemId, ...(queue?.entries.map(e => e.itemId ?? '') ?? [])].filter(Boolean), [group.timeline.itemId, queue]);
  const titles = useTitles(ids);
  if (!service) return null;
  const ended = state.phase === 'ended';
  const can = group.permissions;
  const playing = group.timeline.state === 'playing';
  const now = group.timeline.itemId ? titles[group.timeline.itemId] : undefined;
  const position = group.timeline.itemId ? Number(groupTargetPositionUs(group.timeline, Date.parse(group.timeline.anchorAt), service.serverNow()) / 1000000n) : 0;
  const away = group.host.presence !== 'connected';
  const deadline = group.host.pauseAt && Date.parse(group.host.pauseAt) > service.serverNow() ? {pauses: true, at: group.host.pauseAt} : group.host.endAt ? {pauses: false, at: group.host.endAt} : null;
  const members = group.members.filter(m => m.state === 'joined');
  const me = members.find(m => m.id === group.viewerMemberId);
  const blocked = queue?.eligibility?.members.filter(m => m.blockedEntryIds.length) ?? [];
  if (ended) {
    return <StateView icon="people" title={t('web.together.endedTitle')} body={endedReason(group.endedReason)} action={{label: t('web.together.backToLobby'), onClick: () => service.dismiss()}} />;
  }
  return (
    <>
      {state.phase === 'reconnecting' ? <Notice tone="info" compact>{t('web.together.reconnecting')}</Notice> : null}
      {away ? <Notice tone="warning" action={me && !can.isHost ? {label: t('web.together.becomeHost'), onClick: () => void service.makeHost(me.id)} : undefined}>{[t('web.together.hostDropped'), deadline ? t(deadline.pauses ? 'web.together.pausesIn' : 'web.together.endsIn', {time: clock((Date.parse(deadline.at) - service.serverNow()) / 1000)}) : '', me && !can.isHost ? t('web.together.takeOver') : ''].filter(Boolean).join(' ')}</Notice> : null}

      <Surface>
        <div style={{display: 'flex', gap: 16, alignItems: 'center', flexWrap: 'wrap'}}>
          {/* FEAT-10: the poster goes through the artwork store (the raw path never loaded). */}
          {group.timeline.itemId ? <div style={{width: 64, flexShrink: 0}}><Artwork path={now?.posterUrl} shape="poster" icon="film" /></div> : null}
          <div style={{flex: '1 1 260px', minWidth: 0, display: 'flex', flexDirection: 'column', gap: 8}}>
            <div style={{display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap'}}>
              <Badge tone={playing ? 'healthy' : 'neutral'}>{stateLabel(group.state)}</Badge>
              {group.timeline.itemId ? <Text variant="caption" tone="tertiary" style={{fontVariantNumeric: 'tabular-nums'}}>{now?.duration ? t('web.together.positionOf', {position: clock(position), duration: clock(now.duration)}) : clock(position)}</Text> : null}
            </div>
            <Text variant="title" clamp={2}>{group.timeline.itemId ? (now === null ? t('web.together.hiddenTitle') : now?.title ?? t('web.together.loadingTitle')) : t('web.together.nothingChosen')}</Text>
            <Text variant="caption" tone="tertiary">{group.timeline.itemId ? readinessLine(group) : can.canManageQueue ? t('web.together.pickHelp') : t('web.together.hostPicks')}</Text>
          </div>
          <div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>
            {can.canControl && group.timeline.itemId ? <Button variant="primary" icon={playing ? 'pause' : 'play'} label={playing ? t('web.together.pauseAll') : group.readiness.aggregate === 'ready' ? t('web.together.playAll') : t('web.together.playAnyway')} disabled={state.busy} onClick={() => void service.transport(playing ? 'pause' : 'play')} /> : null}
            {group.timeline.itemId && engine?.state.itemId === group.timeline.itemId ? <Button variant="secondary" icon="maximize" label={t('web.together.openPlayer')} onClick={engine.expand} /> : null}
          </div>
        </div>
      </Surface>

      <div style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))', gap: 20, alignItems: 'start'}}>
        <div style={{display: 'flex', flexDirection: 'column', gap: 8}}>
          <Text variant="label" tone="secondary">{members.length === 1 ? t('web.together.justYou') : t('web.together.people', {count: members.length})}</Text>
          <Surface padless>
            {members.map(m => <MemberRow key={m.id} member={m} mine={m.id === group.viewerMemberId} canPromote={can.isHost && m.role !== 'host' && !state.busy} onPromote={() => void service.makeHost(m.id)} />)}
          </Surface>
          {can.isHost ? <Invite /> : null}
        </div>
        <div style={{display: 'flex', flexDirection: 'column', gap: 8}}>
          <Text variant="label" tone="secondary">{t('web.together.queue')}</Text>
          {blocked.length ? <Notice tone="warning" compact>{t(blocked.length === 1 && blocked[0]!.blockedEntryIds.length === 1 ? 'web.together.blockedOne' : 'web.together.blockedSome', {names: new Intl.ListFormat(undefined, {type: 'conjunction'}).format(blocked.map(b => b.displayName))})}</Notice> : null}
          {queue?.entries.length ? (
            <Surface padless>
              {queue.entries.map(e => <QueueRow key={e.entryId} entry={e} item={e.itemId ? titles[e.itemId] : null} current={e.entryId === group.timeline.currentEntryId} can={can.canManageQueue && !state.busy} onLoad={() => void service.transport('load', {entryId: e.entryId})} onRemove={() => void service.queueRemove(e.entryId)} />)}
            </Surface>
          ) : <Surface><Text variant="caption" tone="tertiary">{can.canManageQueue ? t('web.together.emptyQueueHost') : t('web.together.emptyQueue')}</Text></Surface>}
        </div>
      </div>

      <div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>
        <Button variant="ghost" label={t('web.together.leave')} onClick={() => (can.isHost && members.length > 1 ? setConfirm('leave') : void service.leave())} />
        {can.isHost ? <Button variant="ghost" label={t('web.together.endAll')} onClick={() => setConfirm('end')} /> : null}
      </div>
      <ConfirmDialog open={confirm === 'end'} onOpenChange={o => !o && setConfirm(null)} title={t('confirm.endGroup.title')} body={t('confirm.endGroup.body')} confirmLabel={t('confirm.endGroup.action')} onConfirm={() => { setConfirm(null); void service.end(); }} />
      <ConfirmDialog open={confirm === 'leave'} onOpenChange={o => !o && setConfirm(null)} title={t('confirm.leaveAsHost.title')} body={t('confirm.leaveAsHost.body')} confirmLabel={t('confirm.leaveAsHost.action')} onConfirm={() => { setConfirm(null); void service.leave(); }} />
    </>
  );
}

function readinessLine(group: Group): string {
  const r = group.readiness;
  if (r.aggregate === 'ready') return r.memberCount > 1 ? t('web.together.allReady') : t('web.together.state.ready');
  return t('web.together.waiting', {ready: r.ready, total: r.memberCount, count: r.buffering + r.lagging + r.stale});
}

function MemberRow({member, mine, canPromote, onPromote}: {member: GroupMember; mine: boolean; canPromote: boolean; onPromote: () => void}) {
  const status = member.presence !== 'connected' ? t('web.together.away') : member.readiness === 'ready' ? t('web.together.inStep') : member.readiness === 'buffering' ? t('web.together.loading') : t('web.together.catchingUp');
  return (
    <div style={{display: 'flex', alignItems: 'center', gap: 12, padding: '12px 16px', borderTop: '1px solid var(--line-soft)'}}>
      {/* FEAT-10: every member gets an avatar (their initial). */}
      <div style={{width: 32, flexShrink: 0}}><Artwork shape="circle" initial={member.displayName.trim()[0]?.toUpperCase()} icon="person" alt="" /></div>
      <div style={{flex: 1, minWidth: 0}}>
        <Text variant="bodyStrong" clamp={1}>{mine ? t('web.together.you', {name: member.displayName}) : member.displayName}</Text>
        <Text variant="caption" tone="tertiary">{member.role === 'host' ? t('web.together.hostStatus', {status}) : status}</Text>
      </div>
      {canPromote ? <Button size="sm" variant="ghost" label={t('web.together.makeHost')} onClick={onPromote} /> : null}
    </div>
  );
}

function QueueRow({entry, item, current, can, onLoad, onRemove}: {entry: GroupQueueEntry; item: MediaItem | null | undefined; current: boolean; can: boolean; onLoad: () => void; onRemove: () => void}) {
  const title = entry.unavailable || item === null ? t('web.together.notAvailable') : item?.title ?? t('web.together.loadingTitle');
  return (
    <div style={{display: 'flex', alignItems: 'center', gap: 12, padding: '12px 16px', borderTop: '1px solid var(--line-soft)', opacity: entry.unavailable ? 0.6 : 1}}>
      <Text variant="caption" tone="tertiary" style={{width: 20, textAlign: 'right', fontVariantNumeric: 'tabular-nums'}}>{entry.position + 1}</Text>
      <div style={{flex: 1, minWidth: 0}}>
        <Text variant="bodyStrong" clamp={1}>{title}</Text>
        {current ? <Text variant="caption" tone="accent">{t('web.together.nowPlaying')}</Text> : entry.unavailable ? <Text variant="caption" tone="tertiary">{t('web.together.hiddenEntry')}</Text> : null}
      </div>
      {can && !current && !entry.unavailable ? <Button size="sm" variant="ghost" label={t('web.together.watchThis')} onClick={onLoad} /> : null}
      {can ? <IconButton size="sm" variant="ghost" name="close" label={t('web.together.removeFromQueue')} onClick={onRemove} /> : null}
    </div>
  );
}

function Invite() {
  const {service, state} = useTogether();
  const [copied, setCopied] = useState(false);
  const invite = state.invite;
  const live = invite && Date.parse(invite.expiresAt) > Date.now();
  if (!service) return null;
  return (
    <Surface>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        <Text variant="bodyStrong">{t('web.together.invite')}</Text>
        {live ? (
          <>
            <Text variant="display" style={{letterSpacing: '0.2em', fontVariantNumeric: 'tabular-nums'}}>{invite.code.slice(0, 4)} {invite.code.slice(4)}</Text>
            <Text variant="caption" tone="tertiary">{t('web.together.inviteHelp', {count: invite.maxUses, time: new Date(invite.expiresAt).toLocaleTimeString([], {hour: 'numeric', minute: '2-digit'})})}</Text>
            <div style={{display: 'flex', gap: 8}}>
              <Button size="sm" variant="secondary" icon="copy" label={copied ? t('web.together.copied') : t('web.together.copyCode')} onClick={() => void navigator.clipboard?.writeText(invite.code).then(() => { setCopied(true); setTimeout(() => setCopied(false), 2000); })} />
              <Button size="sm" variant="ghost" label={t('web.together.newCode')} disabled={state.busy} onClick={() => void service.invite()} />
            </div>
          </>
        ) : (
          <>
            <Text variant="caption" tone="tertiary">{t('web.together.codeHelp')}</Text>
            <div><Button size="sm" variant="secondary" label={t('web.together.getCode')} loading={state.busy} onClick={() => void service.invite()} /></div>
          </>
        )}
      </div>
    </Surface>
  );
}
