import {useCallback, useEffect, useMemo, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import {useChannelGuide} from './guide';
import {activeRecording, countUnwatched, mutableRecording, playableRecording, type DeletePreview, type DVRPage, type DVRView as View, type Recording, type RecordingDetail, type RecordingRule, type RecordingStorage, type RuleConfig} from '@core/dvr.ts';
import {formatBytes} from '@core/presentation/index.ts';
import {usePlayerActions} from '../../player/PlayerContext';
import {useSession} from '../../app/session';
import {errorText} from '../../app/errors';
import {Badge, Button, ConfirmDialog, Dialog, Inset, ListRow, Loading, Menu, Notice, ProgressBar, Segmented, Select, SettingsGroup, SettingsRow, StateView, Surface, Text, type MenuItem} from '../../ui';
import {useDVRClient, useGuideFormat, localTimezone} from './guide';
import {groupRecordings, isExceptionBadge, dvrPaddings, dvrLimits} from './dvr-groups';
import {ListPager} from '../server/ListPager';

const t = defaultI18n.t;
const views = (): {id: View; label: string}[] => [{id: 'upcoming', label: t('web.dvr.upcoming')}, {id: 'recorded', label: t('web.dvr.recorded')}, {id: 'rules', label: t('web.dvr.rules')}, {id: 'history', label: t('web.dvr.history')}, {id: 'storage', label: t('web.dvr.storage')}];
const statusOf = (e: unknown) => (e && typeof e === 'object' && typeof (e as {status?: unknown}).status === 'number' ? (e as {status: number}).status : 0);

/** DVR failures in viewer language: a changed recording, an unsupported choice, or the shared presenter. */
export function dvrError(e: unknown): string {
  const status = statusOf(e);
  if (status === 401 || status === 403) return t('web.dvr.error.access');
  if (status === 409) return t('web.dvr.error.changed');
  if (status === 422) return t('web.dvr.error.unsupported');
  return errorText(e, 'live', 'action');
}

function stateLabel(state: Recording['state']): string {
  switch (state) {
    case 'scheduled': return t('web.dvr.state.scheduled');
    case 'conflicted': return t('web.dvr.state.conflicted');
    case 'waiting-source': return t('web.dvr.state.waitingSource');
    case 'waiting-guide': return t('web.dvr.state.waitingGuide');
    case 'preparing': return t('web.dvr.state.preparing');
    case 'recording': return t('web.dvr.state.recording');
    case 'finalizing': return t('web.dvr.state.finalizing');
    case 'completed': return t('web.dvr.state.completed');
    case 'incomplete-playable': return t('web.dvr.state.incomplete');
    case 'failed': return t('web.dvr.state.failed');
    case 'cancelled': return t('web.dvr.state.cancelled');
    case 'pending-delete': return t('web.dvr.state.pendingDelete');
    case 'deleted': return t('web.dvr.state.deleted');
    // Unknown states degrade to title case (hide the badge); they never fail the screen.
    default: return String(state).replace(/-/g, ' ').replace(/^./, c => c.toUpperCase());
  }
}

function reasonLabel(reason: string): string {
  switch (reason) {
    case '': return '';
    case 'capture-window-missed': return t('web.dvr.reason.missed');
    case 'tuner-conflict': return t('web.dvr.reason.conflict');
    case 'storage-floor': return t('web.dvr.reason.storageFloor');
    case 'storage-cap': return t('web.dvr.reason.storageCap');
    case 'owner-cancelled': return t('web.dvr.reason.cancelled');
    case 'source-disabled': return t('web.live.reason.sourceOff');
    case 'source-changed': return t('web.dvr.reason.sourceChanged');
    case 'guide-occurrence-missing': return t('web.dvr.reason.guideMissing');
    case 'partial-capture': case 'recovered-partial': return t('web.dvr.reason.partial');
    case 'physical-reader-active': case 'active-reader-grace': return t('web.dvr.reason.inUse');
    case 'artifact-remove-unavailable': return t('web.dvr.reason.removeRetry');
    default: return '';
  }
}

type Confirm = {kind: 'cancel'; recording: Recording} | {kind: 'delete'; recording: Recording; preview: DeletePreview} | {kind: 'rule'; rule: RecordingRule};

/**
 * DVR: recordings, series rules, history and storage as distinct views, all through the shared
 * DVRClient (FEAT-02 / CD-04). Failures are explained in plain language; deletion is confirmed
 * with its impact.
 */
export function DVRView() {
  const dvr = useDVRClient();
  const {owner} = useSession();
  const player = usePlayerActions();
  const navigate = useNavigate();
  const format = useGuideFormat(localTimezone());
  const [view, setView] = useState<View>('upcoming');
  const [page, setPage] = useState<DVRPage>();
  const [storage, setStorage] = useState<RecordingStorage>();
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState<Confirm | null>(null);
  const [editing, setEditing] = useState<RecordingRule | null>(null);
  const [conflict, setConflict] = useState<Recording | null>(null);
  // PERF-S15: the list pages by the server's cursor (Previous / Next), so every recording is reachable.
  const [cursors, setCursors] = useState<readonly string[]>(['']);
  const cursor = cursors[cursors.length - 1] ?? '';
  const load = useCallback((next: View, at = '') => {
    if (!dvr) return;
    setError('');
    if (next === 'storage') dvr.storage().then(setStorage, e => setError(dvrError(e)));
    else dvr.list(next, at).then(setPage, e => setError(dvrError(e)));
  }, [dvr]);
  useEffect(() => { setCursors(['']); }, [view]);
  useEffect(() => { setPage(undefined); load(view, cursor); }, [load, view, cursor]);
  const act = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError('');
    try {
      await fn();
      load(view, cursor);
    } catch (e) {
      setError(dvrError(e));
    } finally {
      setBusy(false);
      setConfirm(null);
    }
  };
  const askDelete = async (r: Recording) => {
    if (!dvr) return;
    setError('');
    try { setConfirm({kind: 'delete', recording: r, preview: await dvr.deletePreview(r)}); } catch (e) { setError(dvrError(e)); }
  };
  const recordings = page?.recordings ?? [];
  const rules = page?.rules ?? [];
  const loaded = view === 'storage' ? !!storage : !!page;
  const stateTone = (r: Recording) => (r.state === 'recording' ? 'record' : r.state === 'failed' || r.state === 'conflicted' ? 'danger' : r.state === 'completed' || r.state === 'incomplete-playable' ? 'healthy' : 'neutral');
  // FEAT-08: show rows group by programme.seriesId and expand to episodes.
  // Series without an id (movies, one-offs) stay as single rows.
  const [expanded, setExpanded] = useState<readonly string[]>([]);
  useEffect(() => { setExpanded([]); }, [view]);
  const groups = useMemo(() => (view !== 'recorded' ? null : groupRecordings(recordings)), [view, recordings]);
  // Part 2.2: episode subtitle with the channel name when the server provides it.
  const episodeSubtitle = (r: Recording) => {
    const start = Date.parse(r.start), end = Date.parse(r.end);
    const duration = Number.isFinite(start) && Number.isFinite(end) && end > start ? t('web.dvr.minutes', {count: Math.max(1, Math.round((end - start) / 60000))}) : '';
    return [r.channel?.name || '', start && end ? `${format.dayLabel(start)}, ${format.clock(start)}` : '', duration, r.bytes > 0 && !activeRecording(r) ? formatBytes(r.bytes) : '', reasonLabel(r.reason)].filter(Boolean).join(' · ');
  };
  // Part 2.2: grouped show rows count only watched===false as unwatched; null is neither.
  const groupSubtitle = (items: readonly Recording[]) => {
    const unwatched = countUnwatched(items);
    return unwatched > 0 ? t('web.dvr.showCountUnwatched', {count: items.length, unwatched}) : t('web.dvr.showCount', {count: items.length});
  };
  const exceptionBadge = (r: Recording) => isExceptionBadge(r);
  const recordingItems = (r: Recording): MenuItem[] => {
    const items: MenuItem[] = [];
    if (playableRecording(r)) items.push({id: 'play', label: t('action.play'), icon: 'play'});
    if (r.state === 'conflicted') items.push({id: 'conflict', label: t('web.dvr.seeConflict'), icon: 'warning'});
    if (['completed', 'incomplete-playable', 'failed'].includes(r.state)) items.push({id: 'keep', label: r.keep ? t('web.dvr.stopKeeping') : t('web.dvr.keep'), icon: r.keep ? 'bookmarkFilled' : 'bookmark'});
    // FEAT-08: an in-progress recording can be stopped (cancel keeps what was captured).
    if (mutableRecording(r) || activeRecording(r)) items.push({id: 'cancel', label: t('web.dvr.cancelRecording'), icon: 'close', destructive: true, separatorBefore: items.length > 0});
    if (page && page.deletionAvailable && !activeRecording(r)) items.push({id: 'delete', label: t('web.dvr.deleteRecording'), icon: 'trash', destructive: true, separatorBefore: true});
    return items;
  };
  const onPickItem = (r: Recording, id: string) => {
    if (id === 'play') player.play(r.itemId, 0);
    else if (id === 'conflict') setConflict(r);
    else if (id === 'cancel') setConfirm({kind: 'cancel', recording: r});
    else if (id === 'delete') void askDelete(r);
    else if (id === 'keep' && dvr) void act(() => dvr.keep(r));
  };
  const recordingBadges = (r: Recording) => {
    // FEAT-08: Recorded shows badges only for exceptions (Failed, Incomplete, Kept).
    if (view === 'recorded' && !exceptionBadge(r) && !r.keep) return null;
    return <span style={{display: 'inline-flex', gap: 8}}>{r.keep ? <Badge tone="neutral">{t('web.dvr.kept')}</Badge> : null}<Badge tone={stateTone(r)} dot={r.state === 'recording'} live={r.state === 'recording'}>{stateLabel(r.state)}</Badge></span>;
  };
  const recordingSubtitle = (r: Recording) => {
    if (view === 'recorded') return episodeSubtitle(r);
    const reason = reasonLabel(r.reason);
    return [r.channel?.name || '', format.dayLabel(Date.parse(r.start)), format.range(Date.parse(r.start), Date.parse(r.end)), r.bytes > 0 && !activeRecording(r) ? formatBytes(r.bytes) : '', reason].filter(Boolean).join(' · ');
  };
  const RecordingRow = ({r, nested}: {r: Recording; nested?: boolean}) => {
    const items = recordingItems(r);
    const row = (
      <ListRow {...(r.itemId ? {art: {path: `/v1/items/${encodeURIComponent(r.itemId)}/art/poster`, icon: 'dvr' as const}, artShape: 'poster' as const} : {icon: 'dvr' as const})} title={r.programme.title} subtitle={recordingSubtitle(r)} meta={recordingBadges(r)} onClick={playableRecording(r) ? () => player.play(r.itemId, 0) : r.state === 'conflicted' ? () => setConflict(r) : undefined} actions={items.length ? <Menu label={t('web.dvr.actionsFor', {title: r.programme.title})} trigger={<Button variant="ghost" icon="more" aria-label={t('web.dvr.actionsFor', {title: r.programme.title})} size="sm" />} items={items} onSelect={id => onPickItem(r, id)} /> : undefined} />
    );
    return nested ? <div style={{paddingLeft: 40}}>{row}</div> : row;
  };
  // FEAT-08: Upcoming groups under day section headers.
  const UpcomingDays = () => {
    const order: string[] = [];
    const byDay = new Map<string, Recording[]>();
    for (const r of recordings) {
      const day = format.dayLabel(Date.parse(r.start));
      if (!byDay.has(day)) { byDay.set(day, []); order.push(day); }
      byDay.get(day)!.push(r);
    }
    return (
      <>
        {order.map(day => (
          <div key={day}>
            <div style={{padding: '12px 16px 4px'}}><Text variant="bodyStrong" tone="secondary">{day}</Text></div>
            {byDay.get(day)!.map(r => <RecordingRow key={r.id} r={r} />)}
          </div>
        ))}
      </>
    );
  };
  const openGuide = {label: t('web.dvr.openGuide'), onClick: () => void navigate({to: '/channels'})};
  return (
    <>
      <Inset><div style={{display: 'flex', flexWrap: 'wrap', gap: 12, alignItems: 'center', justifyContent: 'space-between'}}>
        <Segmented label={t('web.dvr.view')} options={views()} value={view} onChange={setView} />
        {page && view !== 'storage' && page.usage.recordings > 0 ? <Text variant="caption" tone="tertiary">{t('web.dvr.usage', {count: page.usage.recordings, size: formatBytes(page.usage.bytes)})}</Text> : null}
      </div></Inset>
      {page && !page.captureAvailable && view === 'upcoming' ? <Inset><Notice tone="info">{t('web.dvr.captureOff')}</Notice></Inset> : null}
      {error ? <Inset><Notice tone="error" action={{label: t('action.tryAgain'), onClick: () => load(view, cursor)}}>{error}</Notice></Inset> : null}
      {!loaded && !error ? <Inset><Loading label={t('status.loadingThing', {thing: view === 'storage' ? t('web.dvr.storageThing') : t('web.dvr.recordingsThing')})} /></Inset> : null}
      {page && (view === 'upcoming' || view === 'recorded' || view === 'history') && !recordings.length ? <NothingRecorded view={view}><StateView icon="dvr" title={view === 'upcoming' ? t('web.dvr.empty.upcoming') : view === 'recorded' ? t('web.dvr.empty.recorded') : t('web.dvr.empty.history')} body={view === 'upcoming' ? t('web.dvr.empty.upcomingBody') : t('web.dvr.empty.recordedBody')} action={view === 'upcoming' ? openGuide : undefined} /></NothingRecorded> : null}
      {page && view === 'rules' && !rules.length ? <StateView icon="repeat" title={t('web.dvr.empty.rules')} body={t('web.dvr.empty.rulesBody')} action={openGuide} /> : null}
      {page && view !== 'rules' && view !== 'storage' && recordings.length ? (
        <Inset><Surface padless>
          {view === 'recorded' && groups ? groups.map(g => {
            const first = g.items[0]!;
            const open = expanded.includes(g.key);
            if (g.items.length === 1) return <RecordingRow key={first.id} r={first} />;
            return (
              <div key={g.key}>
                <ListRow {...(first.itemId ? {art: {path: `/v1/items/${encodeURIComponent(first.itemId)}/art/poster`, icon: 'dvr' as const}, artShape: 'poster' as const} : {icon: 'dvr' as const})} title={g.title} subtitle={groupSubtitle(g.items)} meta={<Button variant="ghost" icon={open ? 'chevronDown' : 'forward'} aria-label={t('web.dvr.actionsFor', {title: g.title})} size="sm" />} onClick={() => setExpanded(e => (open ? e.filter(k => k !== g.key) : [...e, g.key]))} />
                {open ? g.items.map(r => <RecordingRow key={r.id} r={r} nested />) : null}
              </div>
            );
          }) : view === 'upcoming' ? <UpcomingDays /> : recordings.map(r => <RecordingRow key={r.id} r={r} />)}
        </Surface></Inset>
      ) : null}
      {page && view === 'rules' && rules.length ? (
        <Inset><Surface padless>
          {rules.map(rule => {
            const help = [rule.config.episodes === 'new' ? t('web.dvr.newEpisodes') : t('web.dvr.allEpisodes'), rule.config.options.episodeLimit ? t('web.dvr.keepLatest', {count: rule.config.options.episodeLimit}) : '', rule.config.keywords.join(', ')].filter(Boolean).join(' · ');
            return <ListRow key={rule.id} icon="repeat" title={rule.config.name} subtitle={help} meta={<Badge tone={rule.config.enabled ? 'healthy' : 'neutral'}>{rule.config.enabled ? t('web.dvr.on') : t('web.dvr.paused')}</Badge>} onClick={() => setEditing(rule)} actions={<Menu label={t('web.dvr.actionsFor', {title: rule.config.name})} trigger={<Button variant="ghost" icon="more" aria-label={t('web.dvr.actionsFor', {title: rule.config.name})} size="sm" />} items={[{id: 'edit', label: t('web.dvr.editRule'), icon: 'edit'}, {id: 'toggle', label: rule.config.enabled ? t('web.dvr.pauseRule') : t('web.dvr.resumeRule'), icon: rule.config.enabled ? 'pause' : 'play'}, {id: 'delete', label: t('web.dvr.deleteRule'), icon: 'trash', destructive: true, separatorBefore: true}]} onSelect={id => {
              if (id === 'edit') setEditing(rule);
              else if (id === 'toggle' && dvr) void act(() => dvr.saveRule(rule.id, rule.revision, {...rule.config, enabled: !rule.config.enabled}));
              else if (id === 'delete') setConfirm({kind: 'rule', rule});
            }} />} />;
          })}
        </Surface></Inset>
      ) : null}
      {page && view !== 'storage' ? <ListPager pages={{page: cursors.length, canPrevious: cursors.length > 1, canNext: !!page.nextCursor, loading: false, previous: () => setCursors(c => c.slice(0, -1)), next: () => { if (page.nextCursor) setCursors(c => [...c, page.nextCursor].slice(-1000)); }}} /> : null}
      {view === 'storage' && storage ? <StorageView storage={storage} owner={owner} /> : null}
      <RecordingConfirm confirm={confirm} busy={busy} onClose={() => setConfirm(null)} onConfirm={(future) => {
        if (!confirm || !dvr) return;
        if (confirm.kind === 'cancel') void act(() => dvr.cancel(confirm.recording));
        else if (confirm.kind === 'delete') void act(() => dvr.delete(confirm.recording, confirm.preview));
        else void act(() => dvr.deleteRule(confirm.rule, future ?? 'keep'));
      }} />
      {editing ? <RuleDialog rule={editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); load(view, cursor); }} /> : null}
      {conflict ? <ConflictDialog recording={conflict} onClose={() => setConflict(null)} onChanged={() => { setConflict(null); load(view, cursor); }} /> : null}
    </>
  );
}

function RecordingConfirm({confirm, busy, onClose, onConfirm}: {confirm: Confirm | null; busy: boolean; onClose: () => void; onConfirm: (future?: 'keep' | 'cancel') => void}) {
  const [future, setFuture] = useState<'keep' | 'cancel'>('keep');
  useEffect(() => setFuture('keep'), [confirm]);
  if (!confirm) return null;
  if (confirm.kind === 'rule') {
    // WEB-LIVE-03: deleting a rule asks first, and says what happens to what it already scheduled.
    return (
      <Dialog open onOpenChange={o => !o && onClose()} title={t('web.dvr.deleteRuleTitle', {name: confirm.rule.config.name})} width={440} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="danger" label={t('web.dvr.deleteRuleAction')} loading={busy} onClick={() => onConfirm(future)} /></>}>
        <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
          <Text as="p" variant="body" tone="secondary">{t('web.dvr.deleteRuleBody')}</Text>
          <Segmented label={t('web.dvr.upcomingChoice')} value={future} onChange={v => setFuture(v as 'keep' | 'cancel')} options={[{id: 'keep', label: t('web.dvr.keepUpcoming')}, {id: 'cancel', label: t('web.dvr.cancelUpcoming')}]} />
        </div>
      </Dialog>
    );
  }
  const title = confirm.recording.programme.title;
  const body = confirm.kind === 'cancel' ? t('web.dvr.cancelBody')
    : confirm.preview.activeReaders ? t('web.dvr.deleteBodyInUse', {size: formatBytes(confirm.preview.bytes)})
    : confirm.preview.keep ? t('web.dvr.deleteBodyKept', {size: formatBytes(confirm.preview.bytes)})
    : t('web.dvr.deleteBody', {size: formatBytes(confirm.preview.bytes)});
  return <ConfirmDialog open onOpenChange={o => !o && onClose()} title={confirm.kind === 'delete' ? t('web.dvr.deleteTitle', {title}) : t('web.dvr.cancelTitle', {title})} body={body} confirmLabel={confirm.kind === 'delete' ? t('web.dvr.deleteRecording') : t('web.dvr.cancelRecording')} cancelLabel={confirm.kind === 'cancel' ? t('web.dvr.keepIt') : undefined} destructive busy={busy} onConfirm={() => onConfirm()} />;
}

/** FEAT-02: a series rule's episodes, how many to keep and its padding, edited in one place. */
function RuleDialog({rule, onClose, onSaved}: {rule: RecordingRule; onClose: () => void; onSaved: () => void}) {
  const dvr = useDVRClient();
  const [config, setConfig] = useState<RuleConfig>(rule.config);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [matches, setMatches] = useState<number | null>(null);
  useEffect(() => {
    if (!dvr) return;
    // FEAT-02: the live preview updates at most once per 400 ms of edits, never per keystroke.
    let live = true;
    const id = setTimeout(() => { dvr.preview(config).then(p => { if (live) setMatches(p.matches); }, () => { if (live) setMatches(null); }); }, 400);
    return () => { live = false; clearTimeout(id); };
  }, [dvr, config]);
  const options = (patch: Partial<RuleConfig['options']>) => setConfig(c => ({...c, options: {...c.options, ...patch}}));
  const minutes = (seconds: number) => seconds ? t('web.dvr.minutes', {count: seconds / 60}) : t('web.dvr.none');
  const save = async () => {
    if (!dvr) return;
    setBusy(true); setError('');
    try { await dvr.saveRule(rule.id, rule.revision, config); onSaved(); } catch (e) { setError(dvrError(e)); } finally { setBusy(false); }
  };
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={config.name} description={matches === null ? undefined : t('web.dvr.matches', {count: matches})} width={480} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" label={t('action.save')} loading={busy} onClick={() => void save()} /></>}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 16}}>
        {error ? <Notice tone="error" compact>{error}</Notice> : null}
        <Segmented label={t('web.dvr.record')} value={config.episodes} onChange={v => setConfig(c => ({...c, episodes: v as 'all' | 'new'}))} options={[{id: 'new', label: t('web.dvr.newEpisodes')}, {id: 'all', label: t('web.dvr.allEpisodes')}]} />
        <Select label={t('web.dvr.keepLabel')} value={String(config.options.episodeLimit)} onChange={e => options({episodeLimit: Number(e.target.value)})} options={dvrLimits.map(n => ({value: String(n), label: n ? t('web.dvr.keepLatest', {count: n}) : t('web.dvr.keepAll')}))} />
        <Select label={t('web.dvr.startEarly')} value={String(config.options.beforeSeconds)} onChange={e => options({beforeSeconds: Number(e.target.value)})} options={dvrPaddings.map(n => ({value: String(n), label: minutes(n)}))} />
        <Select label={t('web.dvr.endLate')} value={String(config.options.afterSeconds)} onChange={e => options({afterSeconds: Number(e.target.value)})} options={dvrPaddings.map(n => ({value: String(n), label: minutes(n)}))} />
      </div>
    </Dialog>
  );
}

/** FEAT-02: a tuner conflict names what overlaps and lets the viewer pick which one records. */
function ConflictDialog({recording, onClose, onChanged}: {recording: Recording; onClose: () => void; onChanged: () => void}) {
  const dvr = useDVRClient();
  const format = useGuideFormat(localTimezone());
  const [detail, setDetail] = useState<RecordingDetail>();
  const [busy, setBusy] = useState<'raise' | 'skip' | null>(null);
  const [error, setError] = useState('');
  useEffect(() => {
    if (!dvr) return;
    dvr.detail(recording.id).then(setDetail, e => setError(dvrError(e)));
  }, [dvr, recording.id]);
  const run = async (kind: 'raise' | 'skip') => {
    if (!dvr) return;
    setBusy(kind); setError('');
    try {
      if (kind === 'skip') await dvr.cancel(recording);
      else await dvr.update(recording, {...recording.options, priority: Math.min(1000, Math.max(recording.options.priority, ...(detail?.overlaps ?? []).map(o => o.priority)) + 1)});
      onChanged();
    } catch (e) { setError(dvrError(e)); } finally { setBusy(null); }
  };
  const tuners = detail?.capacity.known ? detail.capacity.effective : undefined;
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={t('web.dvr.conflictTitle', {title: recording.programme.title})} width={480} actions={<><Button variant="ghost" label={t('action.close')} onClick={onClose} /><Button variant="secondary" label={t('web.dvr.skipThis')} loading={busy === 'skip'} disabled={!!busy} onClick={() => void run('skip')} /><Button variant="primary" label={t('web.dvr.recordThis')} loading={busy === 'raise'} disabled={!!busy || !detail} onClick={() => void run('raise')} /></>}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        {error ? <Notice tone="error" compact>{error}</Notice> : null}
        {!detail && !error ? <Loading label={t('status.loadingThing', {thing: t('web.dvr.conflictThing')})} /> : null}
        {detail ? <Text as="p" variant="body" tone="secondary">{tuners !== undefined ? t('web.dvr.conflictBodyTuners', {count: tuners}) : t('web.dvr.conflictBody')}</Text> : null}
        {detail?.overlaps.length ? (
          <Surface padless>
            {detail.overlaps.map(o => <ListRow key={o.id} icon="dvr" title={o.title} subtitle={`${format.dayLabel(Date.parse(o.start))} · ${format.range(Date.parse(o.start), Date.parse(o.end))}`} />)}
          </Surface>
        ) : null}
      </div>
    </Dialog>
  );
}

/** FEAT-02: how much recording space is used and roughly how much is left. */
function StorageView({storage, owner}: {storage: RecordingStorage; owner: boolean}) {
  const {usedBytes, freeBytes} = storage.measurement;
  const total = usedBytes + freeBytes;
  return (
    <Inset>
      <SettingsGroup title={t('web.dvr.storage')}>
        <SettingsRow icon="storage" label={t('web.dvr.used', {used: formatBytes(usedBytes), free: formatBytes(freeBytes)})} help={storage.forecastHours > 0 ? t('web.dvr.hoursLeft', {count: Math.floor(storage.forecastHours)}) : undefined} full control={total > 0 ? <div style={{width: '100%', maxWidth: 360}}><ProgressBar value={usedBytes / total} /></div> : undefined} />
        {storage.pendingDeleteBytes > 0 ? <SettingsRow icon="trash" label={t('web.dvr.pendingDelete', {size: formatBytes(storage.pendingDeleteBytes)})} /> : null}
        {!storage.measurement.writeHealthy ? <SettingsRow icon="warning" label={t('web.dvr.writeUnhealthy')} help={owner ? t('web.dvr.writeUnhealthyOwner') : undefined} /> : null}
        {owner && storage.policy.retentionDays ? <SettingsRow icon="clock" label={t('web.dvr.retention', {count: storage.policy.retentionDays})} /> : null}
      </SettingsGroup>
    </Inset>
  );
}

/**
 * With no tuner or playlist set up, DVR shows the same setup state as the guide instead of
 * sending people to a guide that has nothing in it (visual pass, 22 Sep).
 */
function NothingRecorded({view, children}: {view: View; children: React.ReactNode}) {
  const guide = useChannelGuide('live-source');
  const {owner} = useSession();
  const navigate = useNavigate();
  if (view !== 'upcoming' || guide.guide?.state !== 'no-sources') return <>{children}</>;
  return <StateView icon="live" title={owner ? t('web.live.empty.noSourcesOwnerTitle') : t('web.live.empty.noSourcesTitle')} body={owner ? t('web.live.empty.noSourcesOwner') : t('web.live.empty.noSourcesMember')} action={owner ? {label: t('web.live.setUp'), onClick: () => void navigate({to: '/settings/$section', params: {section: 'server-live'}, search: {}})} : undefined} />;
}
