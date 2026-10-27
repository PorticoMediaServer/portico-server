import {defaultI18n} from '@i18n';
import type {WebPlaybackRuntimeSnapshot} from '../../../platform/playback';
import {Button, Segmented} from '../../../kit/controls';
import {StateView} from '../../../kit/states';
import {LyricsPanel} from './LyricsPanel';
import {ListeningQueue} from './ListeningQueue';
import {ListeningChapters} from './ListeningChapters';
import type {ListeningHost} from './use-listening-owner';
import {listeningSleepChoices} from './listening-owner';
import styles from './listening-panel.module.css';
const t = defaultI18n.t;
/** All callbacks capture the drawn host opening and the original service page. */
export function ListeningPanels({host, snapshot, reportError}: {host: ListeningHost; snapshot: WebPlaybackRuntimeSnapshot; reportError(error: unknown): void}) {
  const {owner, state} = host, run = (task: () => unknown) => {try {void Promise.resolve(task()).catch(error => {if (owner.canReport(state,true)) reportError(error);});} catch (error) {if (owner.canReport(state,true)) reportError(error);}};
  const list = state.queue;
  if (state.panel === 'lyrics') return <LyricsPanel snapshot={state.lyrics ?? {data: null, candidates: [], warnings: [], preview: null, busy: false, error: '', generation: 0}}
    positionSeconds={snapshot.presentation?.positionSeconds ?? 0} enabled={owner.lyricsOffered()} canSeek={snapshot.presentation?.controls.includes('seek') === true}
    onRetry={() => run(() => owner.lyricsRetry(state))} onChoose={state.lyrics?.data?.source&&state.lyrics.data.librarySelection===undefined?(shown,resource)=>run(()=>owner.chooseLyric(state,shown,resource)):undefined} onSeek={(shown, index) => run(() => owner.lyric(state, shown, index))}/>;
  if (state.panel === 'chapters') return <ListeningChapters host={host} snapshot={snapshot} reportError={reportError}/>;
  if (state.panel === 'speed') return <Segmented aria-label={t('player.opt.speed')} value={String(snapshot.presentation?.rate ?? 1)} items={owner.speeds().map(rate => ({value: String(rate), label: `${rate}×`}))} onValueChange={value => run(() => owner.setSpeed(state, snapshot, Number(value),true))}/>;
  if (state.panel === 'sleep') return <Segmented aria-label={t('player.opt.sleepTimer')} value={String(owner.sleepMinutes())} items={(owner.sleepOffered() ? listeningSleepChoices : []).map(minutes => ({value: String(minutes), label: minutes ? t('player.opt.minutes', {count: minutes}) : t('player.opt.off')}))} onValueChange={value => run(() => owner.setSleep(state, snapshot, Number(value),true))}/>;
  if(state.panel==='options')return <div className={styles.tabs}>
    {!!owner.speeds().length&&<Button onClick={()=>owner.show(state,'speed')}>{t('player.opt.speed')}</Button>}
    {owner.sleepOffered()&&<Button onClick={()=>owner.show(state,'sleep')}>{t('player.opt.sleepTimer')}</Button>}
    {owner.chaptersOffered()&&<Button onClick={()=>owner.show(state,'chapters')}>{t('player.opt.chapters')}</Button>}
  </div>;
  if (!owner.queueOffered() || !list?.current) return <StateView title={t('player.queueUnavailable')}/>;
  const policy = snapshot.queueView?.queue;
  return <ListeningQueue snapshot={list} policy={policy ? {repeat: policy.repeat, shuffled: policy.shuffled} : undefined} policyBusy={state.policyBusy}
    onPolicy={(shown, intent) => run(() => owner.policy(state, shown, intent))} onRefresh={() => run(() => owner.reloadQueue(state))}
    window={{snapshot: state.window, retained: state.retainedWindow, anchor: state.anchor, error: state.queueError, busy: state.policyBusy,
      onRetry: () => run(() => owner.reloadQueue(state)), onEdit: (page, entryId, action) => run(() => owner.queueEdit(state, page, entryId, action))}}
    save={{open: !!state.save, name: state.save?.name ?? '', progress: state.save?.progress ?? null, original: state.save?.list,
      busy: state.save?.busy, error: state.save?.error,
      openDialog: shown => owner.openSave(state, shown), close: () => owner.closeSave(state), changeName: name => owner.saveName(state, name), confirm: (shown, name) => run(() => owner.save(state, shown, name))}}/>;
}
export function ListeningTabs({host}: {host: ListeningHost}) {
  const {owner, state} = host;
  return <nav className={styles.tabs} aria-label={t('next.music.upNext')}>
    <Button size="small" variant="ghost" aria-pressed={state.panel === 'queue'} onClick={event => owner.show(state, 'queue',event.currentTarget)}>{t('next.music.upNext')}</Button>
    {owner.lyricsOffered() && <Button size="small" variant="ghost" aria-pressed={state.panel === 'lyrics'} onClick={event => owner.show(state, 'lyrics',event.currentTarget)}>{t('player.lyrics')}</Button>}
  </nav>;
}
