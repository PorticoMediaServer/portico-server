import {useId} from 'react';
import {createI18n} from '@i18n';
import {timelineFacts, type FactPart, type PlaybackIdentityModel} from '@core/presentation/kit-model';
import {Icon, type IconName} from '../Icon';
import {Popover} from '../overlays/Popover';
import type {OverlayChange} from '../overlays/types';
import {ServerMark, type ServerRole, type ShellTranslate} from './ServerIdentity';
import {ProfileAvatarFace, type ProfileTone} from './AccountSurfaces';
import {counterProgress} from './model';
import {ShellScrim} from './ShellScrim';
import styles from './activity.module.css';

const tDefault = createI18n().t;
export type ActivityStreamView = Readonly<{
  id: string; identity: PlaybackIdentityModel; name: string; device: string; initial: string; art?: ProfileTone;
  positionMs?: number; durationMs?: number; paused: boolean; conversion?: FactPart; rate?: FactPart; onOpen: () => void;
}>;
export type ActivityWorkView = Readonly<{
  id: string; title: string; detail?: string; current?: string; icon: IconName; processed?: number; total?: number;
  discovered?: number; progressText?: string; onOpen: () => void;
}>;
export type ActivityPanelView = Readonly<{
  serverName: string; health?: 'normal' | 'attention'; streamCount?: number; taskCount?: number;
  streams: readonly ActivityStreamView[]; scans: readonly ActivityWorkView[]; tasks: readonly ActivityWorkView[];
  update?: Readonly<{available: boolean; latest?: string; current?: string; onOpen: () => void}>;
  lastScan?: string; phase: 'ready' | 'loading' | 'error'; onRetry: () => void; onDashboard: (section?: 'streams' | 'activity') => void;
}>;

/** No store is created here. The owner shell supplies its existing freshness subscription's real summary. */
export function TopBarActivity({viewerRole, view, open, onOpenChange, t = tDefault}: {viewerRole: ServerRole; view: ActivityPanelView; open: boolean; onOpenChange: OverlayChange; t?: ShellTranslate}) {
  if (viewerRole !== 'owner') return null;
  return <OwnerActivity view={view} open={open} onOpenChange={onOpenChange} t={t} />;
}
function OwnerActivity({view, open, onOpenChange, t}: {view: ActivityPanelView; open: boolean; onOpenChange: OverlayChange; t: ShellTranslate}) {
  const tip = useId(), known = view.phase === 'ready';
  const streamCount = known ? view.streamCount : undefined, update = known && view.update?.available;
  const busy = known && ((streamCount ?? 0) > 0 || view.scans.length > 0 || (view.taskCount ?? 0) > 0);
  const label = streamCount === undefined ? t('next.shell.activity') : t(update ? 'next.shell.activityLabelUpdate' : 'next.shell.activityLabel', {count: streamCount});
  const trigger = <button type="button" className={styles.trigger} data-busy={busy || undefined} data-open={open || undefined} data-streams={(streamCount ?? 0) > 0 || undefined} aria-label={label} aria-haspopup="dialog" aria-expanded={open} aria-describedby={update && !open ? tip : undefined}>
    <Icon name="i-act" size={22} />{(streamCount ?? 0) > 0 && <span className={styles.streamCount}>{streamCount}</span>}{update && <span className={styles.updateDot} aria-hidden="true" />}
  </button>;
  return <span className={styles.triggerWrap}><ShellScrim open={open} context="activity" /><Popover open={open} onOpenChange={onOpenChange} trigger={trigger} title={t('next.shell.activity')} width={420} sideOffset={14} alignOffset={-72} className={styles.popover} modal>
    <ActivityPanel viewerRole="owner" view={view} t={t} />
  </Popover>{update && !open && <span id={tip} role="tooltip" className={styles.tooltip}>{t('next.shell.updateAvailable')}</span>}</span>;
}

export function ActivityPanel({viewerRole, view, t = tDefault}: {viewerRole: ServerRole; view: ActivityPanelView; t?: ShellTranslate}) {
  if (viewerRole !== 'owner') return null;
  const streams = view.streams.slice(0, 4), tasks = view.tasks.slice(0, 3);
  const known = view.phase === 'ready';
  const quiet = known && view.streamCount === 0 && view.scans.length === 0 && view.taskCount === 0;
  return <div className={styles.panel}>
    <header className={styles.header}><ServerMark size={38} /><div><b>{view.serverName}</b>{known && view.health !== undefined && <span className={styles.health} data-health={view.health}>{t(view.health === 'normal' ? 'next.shell.health.normal' : 'next.shell.health.attention')}</span>}</div></header>
    <button type="button" className={styles.dashboard} onClick={() => view.onDashboard()}><Icon name="i-pulse" size={20} />{t('next.shell.dashboard')}{known && view.streamCount !== undefined && <span>{t('next.shell.streams', {count: view.streamCount})}</span>}<Icon name="i-right" size={16} /></button>
    <div className={styles.divider} />
    {view.phase === 'loading' ? <div className={styles.loading} role="status" aria-label={t('next.shell.loadingActivity')}><i /><i /><i /></div> : view.phase === 'error' ? <div className={styles.error}><p>{t('next.shell.activityError')}</p><button type="button" onClick={view.onRetry}>{t('next.shell.tryAgain')}</button></div> : <>
      {quiet ? <div className={styles.quiet}><span className={styles.quietLead}><Icon name="i-act" size={18} /></span><div><b>{t('next.shell.activityQuiet')}</b>{view.lastScan && <p>{t('next.shell.lastScan', {when: view.lastScan})}</p>}</div></div> : <>
        {(view.streamCount ?? 0) > 0 && <section className={styles.group}><h4>{t('next.shell.nowPlaying')}</h4>{streams.map(stream => <ActivityStream key={stream.id} stream={stream} />)}{view.streamCount !== undefined && view.streamCount > streams.length && <More count={view.streamCount - streams.length} onClick={() => view.onDashboard('streams')} t={t} />}</section>}
        {view.scans.length > 0 && <section className={styles.group}><h4>{t('next.shell.libraryScans')}</h4>{view.scans.map(scan => <ActivityWork key={scan.id} work={scan} scan t={t} />)}</section>}
        {(view.taskCount ?? 0) > 0 && <section className={styles.group}><h4>{t('next.shell.backgroundTasks')}</h4>{tasks.map(task => <ActivityWork key={task.id} work={task} t={t} />)}{view.taskCount !== undefined && view.taskCount > tasks.length && <More count={view.taskCount - tasks.length} onClick={() => view.onDashboard('activity')} t={t} />}</section>}
      </>}
      {view.update?.available && <><div className={styles.divider} /><button type="button" className={styles.work} data-update="true" onClick={view.update.onOpen}><span className={styles.updateLead}><Icon name="i-arrow-up" size={18} /></span><span className={styles.workText}><span className={styles.line1}><b>{t('next.shell.updateAvailable')}</b></span><span className={styles.line2}>{view.update.latest && <span>{t('next.shell.version', {version: view.update.latest})}</span>}{view.update.current && <span>{t('next.shell.youHaveVersion', {version: view.update.current})}</span>}</span></span><Icon name="i-right" size={16} className={styles.chevron} /></button></>}
    </>}
  </div>;
}
function More({count, onClick, t}: {count: number; onClick: () => void; t: ShellTranslate}) { return <button type="button" className={styles.more} onClick={onClick}>{t('next.shell.moreDashboard', {count})}</button>; }
function ActivityStream({stream}: {stream: ActivityStreamView}) {
  const fraction = timelineFacts(stream.durationMs, stream.positionMs).progress;
  return <button type="button" className={styles.work} data-stream="true" onClick={stream.onOpen}><ProfileAvatarFace initial={stream.initial} art={stream.art} size={32} appearance="session" /><span className={styles.workText}>
    <span className={styles.line1}><b>{stream.identity.title}</b>{stream.identity.subtitle.map(part => <span key={part.id} data-code={part.id === 'episode-label' || undefined}>{part.text}</span>)}</span>
    <span className={styles.line2}><em>{stream.name}</em><span className={styles.device}>{stream.device}</span>{stream.conversion && <span className={styles.conversion}><Icon name="i-swap" size={13} />{stream.conversion.text}</span>}{stream.rate && <span>{stream.rate.text}</span>}</span>
    {fraction !== undefined && <span className={styles.progress} data-paused={stream.paused || undefined} aria-hidden="true"><span style={{transform: `scaleX(${fraction})`}} /></span>}
  </span></button>;
}
function ActivityWork({work, scan = false, t}: {work: ActivityWorkView; scan?: boolean; t: ShellTranslate}) {
  const fraction = counterProgress(work.processed, work.total);
  const detail = scan ? fraction !== undefined ? t('next.shell.filesOf', {processed: work.processed!, total: work.total!}) : work.discovered !== undefined ? t('next.shell.filesFound', {count: work.discovered}) : work.detail : work.detail;
  const bottom = scan ? work.current : work.progressText;
  return <button type="button" className={styles.work} onClick={work.onOpen}><ProgressRing icon={work.icon} fraction={fraction} /><span className={styles.workText}><span className={styles.line1}><b>{work.title}</b>{detail && <span>{detail}</span>}</span>{bottom && <span className={styles.line2}><span className={styles.file}>{bottom}</span></span>}</span>{fraction !== undefined && <span className={styles.percentage}>{Math.round(fraction * 100)}%</span>}</button>;
}
export function ProgressRing({fraction, icon}: {fraction?: number; icon: IconName}) {
  const known = fraction !== undefined && Number.isFinite(fraction) && fraction >= 0 && fraction <= 1;
  return <span className={styles.ring} aria-hidden="true"><svg width={32} height={32} viewBox="0 0 32 32"><circle className={styles.ringTrack} cx={16} cy={16} r={14} />{known && <circle className={styles.ringValue} cx={16} cy={16} r={14} pathLength={100} strokeDasharray={`${fraction * 100} 100`} />}</svg><Icon name={icon} size={15} /></span>;
}
