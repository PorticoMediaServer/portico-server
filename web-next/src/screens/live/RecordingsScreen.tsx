import {useEffect, useMemo, useState, useSyncExternalStore, type CSSProperties, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {bytesLabel, dvrMessage, playableRecording, recordingReason, recordingStateLabel,
  type Recording, type RecordingDetail, type RecordingState} from '@core/dvr.ts';
import {channelTint} from '@portico/client-core/kit-model';
import {Button, Chip, Select} from '../../kit/controls';
import {ChannelMark} from '../../kit/artwork';
import {Icon} from '../../kit/Icon';
import {Banner, Dialog, Menu, SidePanel, type MenuEntry} from '../../kit/overlays';
import {StateView} from '../../kit/states';
import {RecordingsLayout, RecordingStorage} from '../../kit/dvr/RecordingsLayout';
import {RuleGap} from '../../kit/dvr/RuleCard';
import {LiveRecordingTabs} from './SeriesRules';
import {createRecordingsController, type RecordingsController, type RecordingsSort, type RecordingsView} from './recordings-controller';
import {createRecordedGroupReader, type RecordedGroupReader} from './recorded-group';
import {readRecordingDetail} from './recording-detail';
import {useLibraryScreenContext, type ProtectedLibraryScreen} from '../browse/screen-context';
import {screenLifetime, type ScreenContext} from '../detail/context';
import styles from './live.module.css';

const t = defaultI18n.t;
const randomHex = async () => Array.from(crypto.getRandomValues(new Uint8Array(24)), value => value.toString(16).padStart(2, '0')).join('');
const statePill = (state: RecordingState) => t(({scheduled: 'dvr.state.scheduled', conflicted: 'dvr.state.conflicted', 'waiting-source': 'dvr.state.waitingSource',
  'waiting-guide': 'dvr.state.waitingGuide', preparing: 'dvr.state.preparing', recording: 'dvr.state.recording', finalizing: 'dvr.state.finalizing',
  completed: 'dvr.state.completed', 'incomplete-playable': 'dvr.state.incompletePlayable', failed: 'dvr.state.failed', cancelled: 'dvr.state.cancelled',
  'pending-delete': 'dvr.state.pendingDelete', deleted: 'dvr.state.deleted'} as const)[state]);

function timeOf(value: string) { return new Intl.DateTimeFormat(defaultI18n.region.formatLocale, {hour: 'numeric', minute: '2-digit', timeZone: defaultI18n.region.timeZone ?? undefined}).format(Date.parse(value)); }
function dayOf(value: string, now: number) {
  const at = Date.parse(value);
  return new Date(at).toDateString() === new Date(now).toDateString() ? t('guide.today')
    : new Intl.DateTimeFormat(defaultI18n.region.formatLocale, {weekday: 'long', month: 'short', day: 'numeric'}).format(at);
}
function dateOf(value: string) { return new Intl.DateTimeFormat(defaultI18n.region.formatLocale, {weekday: 'long', month: 'long', day: 'numeric'}).format(Date.parse(value)); }
function rangeOf(recording: Recording) { return t('guide.cellTime', {start: timeOf(recording.start), end: timeOf(recording.end)}); }
function episodeOf(recording: Recording) {
  const info = recording.programme.episode;
  const number = info?.number ?? (info as {number?: number; episode?: number} | undefined)?.episode;
  return info?.display ?? (info?.season !== undefined && number !== undefined ? t('downloads.title.episode', {season: info.season, episode: number}) : undefined);
}
function channelMark(recording: Recording) {
  const channel = recording.channel;
  return channel ? {sourceId: '', id: channel.id, text: channel.mark?.text ?? channel.name.slice(0, 2).toLocaleUpperCase(), seed: channel.mark?.seed ?? channel.id} : undefined;
}
function RecordingArt({recording, large, badges, now}: {recording: Recording; large?: boolean; badges?: ReactNode; now?: number}) {
  const mark = channelMark(recording);
  const title = recording.programme.title.replace(/^The\s+/i, '');
  const start = Date.parse(recording.start), end = Date.parse(recording.end);
  const progress = !large && recording.state === 'recording' && now !== undefined && end > start ? Math.min(1, Math.max(0, (now - start) / (end - start))) : undefined;
  return <span className={large ? styles.recArtLarge : styles.recArt} style={{'--tint': channelTint(mark?.seed ?? recording.id)} as CSSProperties} aria-hidden="true">
    <span className={styles.recArtGlyph}>{(Array.from(title)[0] ?? '?').toLocaleUpperCase()}</span>
    {mark && <span className={styles.recArtChannel}><ChannelMark mark={mark} size={large ? 22 : 18}/>{large && <b>{recording.channel?.name}</b>}</span>}
    {badges && <span className={styles.recArtBadges}>{badges}</span>}
    {progress !== undefined && <span className={styles.recArtProgress}><i style={{width: `${(progress * 100).toFixed(2)}%`}}/></span>}
  </span>;
}
function StatePill({recording}: {recording: Recording}) {
  return <span className={styles.recState} data-state={recording.state}>{statePill(recording.state)}</span>;
}
function RecordingCopy({recording}: {recording: Recording}) {
  const episode = episodeOf(recording), mark = channelMark(recording);
  return <div className={styles.recCopy}>
    <b>{recording.programme.title}</b>
    {(episode || recording.programme.subtitle) && <small className={styles.recEpisode}>{episode}{episode && recording.programme.subtitle && <RuleGap/>}{recording.programme.subtitle}</small>}
    <small className={styles.recMeta}>{recording.programme.flags?.live && <span className={styles.recFlag} data-kind="live">{t('guide.live')}</span>}{recording.programme.flags?.new && <span className={styles.recFlag} data-kind="new">{t('guide.new')}</span>}{recording.channel && <span>{mark && <ChannelMark mark={mark} size={18}/>}{recording.channel.name}</span>}{recording.ruleId ? <span><Icon name="i-series" size={15}/>{t('next.live.seriesRule')}</span> : <span>{t('next.live.oneTime')}</span>}</small>
  </div>;
}
function RecordingRow({recording, now, onCancel, onDetails}: {recording: Recording; now: number; onCancel?: () => void; onDetails?: () => void}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const items: readonly MenuEntry[] = [
    ...(onCancel ? [{id: 'cancel', label: t('recordings.cancelAction'), icon: 'i-close' as const, destructive: true, onSelect: onCancel}] : []),
    ...(onDetails ? [{id: 'details', label: t('next.live.details'), icon: 'i-info' as const, onSelect: onDetails}] : [])];
  return <div className={styles.recRow} data-state={recording.state}>
    <div className={styles.recWhen}><b>{recording.state === 'recording' ? t('guide.now') : timeOf(recording.start)}</b><small>{rangeOf(recording)}</small></div>
    <RecordingArt recording={recording} now={now}/>
    <RecordingCopy recording={recording}/>
    <div className={styles.recRight}><StatePill recording={recording}/>
      {items.length > 0 && <Menu open={menuOpen} onOpenChange={open => setMenuOpen(open)} title={t('web.dvr.actionsFor', {title: recording.programme.title})}
        trigger={<Button className={styles.recMore} variant="ghost" size="small" iconOnly icon="i-more" aria-label={t('web.dvr.actionsFor', {title: recording.programme.title})}/>} items={items}/>}
    </div>
  </div>;
}

function ConflictBox({context, recording, detail: supplied, onResolve, onCancel, onDetails}: {context: ScreenContext; recording: Recording; detail?: RecordingDetail; onResolve: (detail?: RecordingDetail) => void; onCancel?: () => void; onDetails?: () => void}) {
  const [detail, setDetail] = useState<RecordingDetail | undefined>(supplied);
  useEffect(() => {
    if (supplied || !context.isCurrent()) return;
    const abort = new AbortController();
    void readRecordingDetail(context, recording.id, abort.signal).then(value => {if (!abort.signal.aborted && context.isCurrent()) setDetail(value);}, () => {});
    return () => abort.abort();
  }, [context, recording.id, supplied]);
  const tuners = detail?.capacity.known ? detail.capacity.effective : recording.conflicts[0]?.capacity;
  const window = recording.conflicts[0];
  const [menuOpen, setMenuOpen] = useState(false);
  const items: readonly MenuEntry[] = [
    ...(onCancel ? [{id: 'cancel', label: t('recordings.cancelAction'), icon: 'i-close' as const, destructive: true, onSelect: onCancel}] : []),
    ...(onDetails ? [{id: 'details', label: t('next.live.details'), icon: 'i-info' as const, onSelect: onDetails}] : [])];
  return <div className={styles.conflictBox}>
    <div className={styles.conflictHead}><Icon name="i-warn" size={22}/><div>
      <h3>{t('recordings.conflictTitle', {title: recording.programme.title})}</h3>
      {tuners !== undefined && <p>{t('next.live.conflictBody', {count: tuners, start: window ? timeOf(window.start) : timeOf(recording.start), end: window ? timeOf(window.end) : timeOf(recording.end)})}</p>}
    </div></div>
    <div className={styles.conflictRow}>
      <div className={styles.recWhen}><b>{timeOf(recording.start)}</b><small>{rangeOf(recording)}</small></div>
      <RecordingArt recording={recording}/>
      <RecordingCopy recording={recording}/>
      <div className={styles.recRight}><StatePill recording={recording}/><Button className={styles.conflictAction} variant="primary" onClick={() => onResolve(detail)}>{t('web.dvr.seeConflict')}</Button>
        {items.length > 0 && <Menu open={menuOpen} onOpenChange={open => setMenuOpen(open)} title={t('web.dvr.actionsFor', {title: recording.programme.title})}
          trigger={<Button className={styles.recMore} variant="ghost" size="small" iconOnly icon="i-more" aria-label={t('web.dvr.actionsFor', {title: recording.programme.title})}/>} items={items}/>}</div>
    </div>
    {detail && detail.overlaps.length > 0 && <div className={styles.conflictOthers}>
      <span>{t('next.live.usingTuners')}</span>
      {detail.overlaps.map(other => <span key={other.id}><b>{other.title}</b><em>{t('guide.cellTime', {start: timeOf(other.start), end: timeOf(other.end)})}</em></span>)}
    </div>}
  </div>;
}

function ConflictDialog({context, recording, controller, onClose}: {context: ScreenContext; recording?: Recording; controller: RecordingsController; onClose: () => void}) {
  const [detail, setDetail] = useState<RecordingDetail>();
  const [busy, setBusy] = useState<'raise' | 'skip'>();
  const [error, setError] = useState<string>();
  useEffect(() => {
    setDetail(undefined);setError(undefined);
    if (!recording || !context.isCurrent()) return;
    const abort = new AbortController();
    void readRecordingDetail(context, recording.id, abort.signal).then(value => {if (!abort.signal.aborted) setDetail(value);}, error => {if (!abort.signal.aborted) setError(dvrMessage(error));});
    return () => abort.abort();
  }, [context, recording]);
  const run = async (kind: 'raise' | 'skip') => {
    if (!recording || busy) return;
    setBusy(kind);setError(undefined);
    try {
      const priority = Math.min(1000, Math.max(recording.options.priority, ...(detail?.overlaps ?? []).map(other => other.priority)) + 1);
      const result = kind === 'skip' ? await controller.cancel(recording) : await controller.raisePriority(recording, priority);
      if (result.committed) {onClose();return;}
      if (!result.busy) setError(dvrMessage({status: 409}));
    } catch (failure) {setError(dvrMessage(failure));}
    finally {setBusy(undefined);}
  };
  if (!recording) return null;
  return <Dialog open={!!recording} onOpenChange={open => {if (!open) onClose();}} width={480}
    title={t('recordings.conflictTitle', {title: recording.programme.title})} description={t('recordings.conflictsBody')}
    footer={<><Button variant="primary" loading={busy === 'raise'} disabled={!!busy} onClick={() => {void run('raise');}}>{t('web.dvr.recordThis')}</Button>
      <Button variant="glass" loading={busy === 'skip'} disabled={!!busy} onClick={() => {void run('skip');}}>{t('web.dvr.skipThis')}</Button></>}>
    {error ? <Banner>{error}</Banner> : <p className={styles.dialogText}>{detail?.capacity.known ? t('recordings.conflictBodyTuners', {count: detail.capacity.effective}) : t('recordings.conflictsBody')}</p>}
  </Dialog>;
}

export function UpcomingView({context, controller, snapshot, now, onResolve, conflictDetails}: {context: ScreenContext; controller: RecordingsController; snapshot: ReturnType<RecordingsController['getSnapshot']>; now: number; onResolve: (recording: Recording, detail?: RecordingDetail) => void; conflictDetails?: Readonly<Record<string, RecordingDetail>>}) {
  const recordings = snapshot.page?.recordings ?? [];
  const conflicted = recordings.filter(recording => recording.state === 'conflicted');
  const current = recordings.filter(recording => ['recording', 'preparing', 'finalizing'].includes(recording.state) && recording.state !== 'conflicted');
  const later = recordings.filter(recording => ['scheduled', 'waiting-source', 'waiting-guide'].includes(recording.state));
  const [shown, setShown] = useState<Recording>();
  return <div className={styles.recBody}>
    {conflicted.length > 0 && <section className={styles.recSection}><h2>{t('recordings.conflicts')}<span>{snapshot.summary?.conflicts ?? conflicted.length}</span></h2>
      {conflicted.map(recording => <ConflictBox key={recording.id} context={context} recording={recording} detail={conflictDetails?.[recording.id]} onResolve={detail => onResolve(recording, detail)} onCancel={() => {void controller.cancel(recording);}} onDetails={() => setShown(recording)}/>)}</section>}
    {current.length > 0 && <section className={styles.recSection}><h2>{t('next.live.recordingNow')}</h2>
      {current.map(recording => <RecordingRow key={recording.id} recording={recording} now={now} onCancel={() => {void controller.cancel(recording);}}/>)}</section>}
    {later.length > 0 && <section className={styles.recSection}><h2>{t('next.live.laterToday')}<span>{dateOf(later[0]!.start)}</span></h2>
      {later.map(recording => <RecordingRow key={recording.id} recording={recording} now={now} onCancel={() => {void controller.cancel(recording);}} onDetails={() => setShown(recording)}/>)}</section>}
    {!conflicted.length && !current.length && !later.length && <StateView title={t('recordings.noneScheduled')} body={t('recordings.noneScheduledBody')}/>}
    <Dialog open={!!shown} onOpenChange={open => {if (!open) setShown(undefined);}} title={shown?.programme.title} description={shown ? rangeOf(shown) : undefined} width={440}
      footer={<Button variant="glass" onClick={() => setShown(undefined)}>{t('next.overlays.close')}</Button>}>{shown && <p className={styles.dialogText}>{recordingStateLabel(shown.state)}{shown.reason && <> · {recordingReason(shown.reason)}</>}</p>}</Dialog>
  </div>;
}

export function RecordedView({context, controller, snapshot}: {context: ScreenContext; controller: RecordingsController; snapshot: ReturnType<RecordingsController['getSnapshot']>}) {
  const [open, setOpen] = useState<{key: string; title: string}>();
  const sort = (snapshot.groupsSort ?? 'recent') as RecordingsSort;
  const groups = snapshot.groups?.groups ?? [];
  const change = (value: string) => {if (value === 'recent' || value === 'title' || value === 'oldest') void controller.readGroups(value).catch(() => {});};
  return <div className={styles.recBody}>
    <div className={styles.recordedHead}><p>{t('next.live.recordedHead', {shows: groups.length, recordings: snapshot.summary?.recorded ?? 0, unwatched: snapshot.summary?.unwatched ?? 0})}</p>
      <span className={styles.recordedSortLabel}>{t('channels.sort')}</span><Select className={styles.recordedSort} aria-label={t('channels.sort')} value={sort} options={[{value: 'recent', label: t('next.live.sortRecent')}, {value: 'title', label: t('next.live.sortTitle')}, {value: 'oldest', label: t('next.live.sortOldest')}]} onValueChange={change}/></div>
    {groups.length > 0 ? <div className={styles.recordedGrid}>{groups.map(group => <button type="button" className={styles.recordedCard} key={group.key} onClick={() => setOpen({key: group.key, title: group.title})}>
      <RecordingArt recording={group.recording} large={true} badges={<>{group.unwatched > 0 && <em className={styles.recArtCount}>{group.unwatched}</em>}{group.kept > 0 && <em>{t('recordings.kept')}</em>}{group.exceptions > 0 && <em>{t('dvr.state.incomplete')}</em>}</>}/>
      <span className={styles.recordedCopy}><b>{group.title}</b><small>{group.recording.channel?.name}</small>
        <small>{group.unwatched ? t('recordings.showCountUnwatched', {count: group.recordings, unwatched: group.unwatched}) : t('recordings.showCount', {count: group.recordings})}</small></span>
    </button>)}</div> : <StateView title={t('recordings.noneRecorded')} body={t('recordings.noneRecordedBody')}/>}
    {open && <RecordedGroupPanel context={context} group={open.key} title={open.title} sort={sort} onClose={() => setOpen(undefined)}/>}
  </div>;
}

function RecordedGroupPanel({context, group, title, sort, onClose}: {context: ScreenContext; group: string; title: string; sort: RecordingsSort; onClose: () => void}) {
  const reader = useMemo(() => createRecordedGroupReader(context, group, sort), [context, group, sort]);
  useEffect(() => () => reader.dispose(), [reader]);
  useEffect(() => { void reader.load().catch(() => {}); }, [reader]);
  const state = useSyncExternalStore(reader.subscribe, reader.getSnapshot, reader.getSnapshot);
  const [remove, setRemove] = useState<Recording>();
  const life = useMemo(() => screenLifetime(context), [context]);
  useEffect(() => () => life.dispose(), [life]);
  const play = (recording: Recording) => {if (!playableRecording(recording) || !life.current()) return;void context.effects.play({libraryId: recording.libraryId, itemId: recording.itemId, startSeconds: Math.max(0, (recording.positionMs ?? 0) / 1000)}, life.signal(), life.current).catch(context.effects.reportError);};
  return <SidePanel open onOpenChange={open => {if (!open) onClose();}} width={640} topInset={76} title={title}
    description={t('recordings.showCount', {count: state.recordings.length})} className={styles.groupPanel}
    footer={<Button variant="glass" onClick={onClose}>{t('next.overlays.close')}</Button>}>
    {state.error && <Banner>{state.error}</Banner>}
    {state.recordings.map(recording => <div className={styles.groupRow} key={recording.id}>
      <RecordingArt recording={recording}/>
      <div className={styles.groupCopy}><b>{recording.programme.subtitle || recording.programme.title}</b><small>{dayOf(recording.start, Date.now())}<RuleGap/>{rangeOf(recording)}</small>
        <small>{recording.keep ? t('recordings.kept') : recording.watched ? t('next.live.watched') : t('next.live.unwatched')}</small></div>
      <div className={styles.groupActions}>
        <Button variant="glass" size="small" icon="i-play" disabled={!playableRecording(recording)} onClick={() => play(recording)}>{t('next.live.play')}</Button>
        <Button variant="ghost" size="small" disabled={state.busy || !playableRecording(recording)} onClick={() => {void reader.keep(recording);}}>{recording.keep ? t('web.dvr.stopKeeping') : t('web.dvr.keep')}</Button>
        <Button variant="ghost" size="small" disabled={state.busy} onClick={() => setRemove(recording)}>{t('web.dvr.deleteRecording')}</Button>
      </div>
    </div>)}
    {state.nextCursor && <Button className={styles.groupMore} variant="glass" onClick={() => {void reader.more().catch(() => {});}}>{t('next.live.more')}</Button>}
    <Dialog open={!!remove} onOpenChange={open => {if (!open) setRemove(undefined);}} width={440} title={t('web.dvr.deleteTitle', {title: remove?.programme.title ?? ''})}
      description={t('web.dvr.deleteBody', {size: bytesLabel(remove?.bytes ?? 0)})} footer={<><Button variant="glass" onClick={() => setRemove(undefined)}>{t('action.cancel')}</Button>
        <Button variant="ghost" loading={state.busy} onClick={() => {if (remove) void reader.remove(remove).then(ok => {if (ok) setRemove(undefined);});}}>{t('web.dvr.deleteRecording')}</Button></>}>
      <p className={styles.dialogText}>{t('web.dvr.deleteBody', {size: bytesLabel(remove?.bytes ?? 0)})}</p>
    </Dialog>
  </SidePanel>;
}

function HistoryView({snapshot, now}: {snapshot: ReturnType<RecordingsController['getSnapshot']>; now: number}) {
  const recordings = snapshot.page?.recordings ?? [];
  return <div className={styles.recBody}>{recordings.length
    ? recordings.map(recording => <RecordingRow key={recording.id} recording={recording} now={now}/>)
    : <StateView title={t('recordings.noneRecorded')} body={t('recordings.noneRecordedBody')}/>}</div>;
}

function StorageView({snapshot}: {snapshot: ReturnType<RecordingsController['getSnapshot']>}) {
  const storage = snapshot.storage;
  if (!storage) return null;
  return <div className={styles.recBody}><div className={styles.storagePanel}>
    <RecordingStorage used={t('next.live.storageUsed', {amount: Math.round(storage.measurement.usedBytes / 1e9)})}
      free={t('next.live.storageFree', {amount: Math.round(storage.measurement.freeBytes / 1e9)})}
      remaining={t('recordings.hoursLeft', {count: storage.forecastHours})} label={t('recordings.storage')}
      fraction={storage.measurement.usedBytes / Math.max(1, storage.measurement.usedBytes + storage.measurement.freeBytes)}/>
    {storage.pendingDeleteBytes > 0 && <p>{t('recordings.pendingDelete', {size: `${Math.round(storage.pendingDeleteBytes / 1e9)} GB`})}</p>}
    {storage.policy.retentionDays > 0 && <p>{t('recordings.retention', {count: storage.policy.retentionDays})}</p>}
    {!storage.measurement.writeHealthy && <Banner>{t('recordings.unavailableBody')}</Banner>}
  </div></div>;
}

function Boundary({captured, view, onView, onGuide}: {captured: ProtectedLibraryScreen; view: RecordingsView; onView: (view: 'upcoming' | 'recorded' | 'rules' | 'history' | 'storage') => void; onGuide: () => void}) {
  const context = useLibraryScreenContext(captured);
  const [owned, setOwned] = useState<Readonly<{context: ScreenContext; controller: RecordingsController}>>();
  const [attempt, setAttempt] = useState(0);
  const now = useMemo(() => Date.now(), [attempt]);
  useEffect(() => {
    const life = screenLifetime(context);
    const controller = createRecordingsController({api: life.api, serverId: context.scope.serverId, requestId: randomHex, signal: life.signal(), isCurrent: life.current});
    setOwned({context, controller});
    const load = async () => {
      await controller.load({view});
      await controller.readSummary();
      if (view === 'recorded') await controller.readGroups('recent');
    };
    void load().catch(() => {});
    return () => {life.dispose();controller.dispose();};
  }, [context, view, attempt]);
  const snapshot = useSyncExternalStore(owned?.controller.subscribe ?? (() => () => {}), owned?.controller.getSnapshot ?? (() => undefined as never), owned?.controller.getSnapshot ?? (() => undefined as never));
  const summary = snapshot?.summary;
  const views = [
    {id: 'upcoming' as const, label: t('recordings.upcoming'), count: summary?.upcoming},
    {id: 'recorded' as const, label: t('recordings.recorded'), count: summary?.recorded},
    {id: 'rules' as const, label: t('recordings.rules'), count: summary?.rules},
    {id: 'history' as const, label: t('saved.tab.history')},
    {id: 'storage' as const, label: t('recordings.storage')},
  ];
  const [conflict, setConflict] = useState<{recording: Recording; detail?: RecordingDetail}>();
  const retry = () => setAttempt(value => value + 1);
  if (!owned || owned.context !== context || !snapshot) return null;
  const loading = snapshot.phase === 'loading' || snapshot.phase === 'idle';
  const body = loading ? <p role="status">{t('recordings.loading')}</p>
    : snapshot.error && !snapshot.page && !snapshot.storage ? <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: retry}}/>
      : view === 'upcoming' ? <UpcomingView context={context} controller={owned.controller} snapshot={snapshot} now={now} onResolve={(recording, detail) => setConflict({recording, detail})}/>
        : view === 'recorded' ? <RecordedView context={context} controller={owned.controller} snapshot={snapshot}/>
          : view === 'history' ? <HistoryView snapshot={snapshot} now={now}/>
            : <StorageView snapshot={snapshot}/>;
  return <RecordingsLayout title={t('next.home.shell.live')} sourceLabel={t('channels.source')} allChannels={t('channels.all')}
    sourceName={context.identity?.serverName ?? ''} navigation={undefined}>
    <LiveRecordingTabs view="recordings" conflicts={summary?.conflicts} onChange={value => {if (value === 'guide') onGuide();}}/>
    <div className={styles.recordingsBar}>
      <div className={styles.recordingsChips} role="group" aria-label={t('recordings.title')}>
        {views.map(item => <Chip className={styles.recordingsChip} key={item.id} selected={item.id === view} count={item.count} onClick={() => onView(item.id)}>{item.label}</Chip>)}
      </div>
      {snapshot.storage && <button type="button" className={styles.storageButton} aria-label={t('recordings.storage')} onClick={() => onView('storage')}>
        <RecordingStorage used={t('next.live.storageUsed', {amount: Math.round(snapshot.storage.measurement.usedBytes / 1e9)})}
          free={t('next.live.storageFree', {amount: Math.round(snapshot.storage.measurement.freeBytes / 1e9)})}
          remaining={t('recordings.hoursLeft', {count: snapshot.storage.forecastHours})} label={t('recordings.storage')}
          fraction={snapshot.storage.measurement.usedBytes / Math.max(1, snapshot.storage.measurement.usedBytes + snapshot.storage.measurement.freeBytes)}/>
      </button>}
    </div>
    {snapshot.error && <Banner className={styles.recordingsBanner} action={{label: t('next.states.tryAgain'), onAction: retry}}>{snapshot.error}</Banner>}
    {body}
    {conflict && <ConflictDialog context={context} recording={conflict.recording} controller={owned.controller} onClose={() => setConflict(undefined)}/>}
  </RecordingsLayout>;
}

export function RecordingsScreen({captured, view, onView, onGuide}: {captured: ProtectedLibraryScreen; view: RecordingsView; onView: (view: 'upcoming' | 'recorded' | 'rules' | 'history' | 'storage') => void; onGuide: () => void}) {
  return <Boundary captured={captured} view={view} onView={onView} onGuide={onGuide}/>;
}
