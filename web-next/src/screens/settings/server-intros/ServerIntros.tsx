import {useEffect, useState, useSyncExternalStore, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import type {FactPart} from '@core/presentation/kit-model';
import type {SegmentKind, SegmentReviewEntry, SegmentReviewMarker, SegmentTitleReview, SegmentTitleSummary} from '@core/segment-review.ts';
import {Button} from '../../../kit/controls';
import {Icon} from '../../../kit/Icon';
import {SegmentWindows, StatTiles, type SegmentEdit, type SegmentMark} from '../../../kit/shell';
import type {SegmentIntrosSnapshot, SegmentIntrosSource} from './source';
import styles from './server-intros.module.css';

const t = defaultI18n.t;
const seconds = (us: string): number => {const value = Number(us); return Number.isFinite(value) && value > 0 ? value / 1e6 : 0;};
const isOpening = (kind: SegmentKind) => kind === 'intro' || kind === 'recap' || kind === 'commercial';
const windowKind = (kind: SegmentKind): SegmentMark['kind'] => kind === 'credits' || kind === 'outro' ? 'credits' : kind === 'recap' || kind === 'commercial' ? 'recap' : 'intro';
const waiting = (marker: SegmentReviewMarker) => !marker.approved && !marker.edited && marker.basis !== 'owner';
const trustText = (marker: SegmentReviewMarker) => t('web.segments.trust', {trust: marker.approved ? 'approved' : !marker.viewerVisible ? 'hidden' : marker.automaticSafe ? 'automatic' : 'suggested'});
const mark = (marker: SegmentReviewMarker): SegmentMark => {
  const facts: FactPart[] = [];
  if (marker.basis !== 'owner') facts.push({id: 'confidence', text: t('web.segments.confidence', {percent: Math.round(marker.confidence * 100)}), role: 'supplement'});
  facts.push({id: 'basis', text: t('web.segments.basis', {basis: marker.basis}), role: 'context'});
  return {id: marker.id, kind: windowKind(marker.kind), label: t('web.segments.kind', {kind: marker.kind}), start: seconds(marker.startUS), end: seconds(marker.endUS),
    suggested: waiting(marker), trust: trustText(marker), trustTone: marker.approved || !marker.viewerVisible ? 'neutral' : marker.automaticSafe ? 'automatic' : 'suggested', facts};
};
const titleLine = (title: SegmentTitleSummary) => [
  t('web.segments.titleFiles', {measured: title.files.complete, total: title.files.complete + title.files.pending + title.files.failed + title.files.unsupported}),
  title.markers.suggested ? t('web.segments.titleSuggested', {count: title.markers.suggested}) : '',
  title.markers.automatic ? t('web.segments.titleAutomatic', {count: title.markers.automatic}) : '',
  title.markers.approved ? t('web.segments.titleApproved', {count: title.markers.approved}) : '',
].filter(Boolean).join(' · ');

/** The owner's review under the drawn mode row and progress line: the status
 * the server reports, each video library, its shows and films, and one title's
 * markers with the drawn zoomed windows. Every number is the server's own. */
export function ServerIntros({source, modeRows}: {source: SegmentIntrosSource; modeRows: ReactNode}) {
  const snapshot = useSyncExternalStore(source.subscribe, source.getSnapshot, source.getSnapshot);
  const [edit, setEdit] = useState<{entryId: string; marker: SegmentReviewMarker; start: number; end: number}>();
  useEffect(() => {void source.load();}, [source]);
  // The drawn page starts on the first real video library; the owner can switch.
  useEffect(() => {
    const first = snapshot.status?.libraries[0];
    if (first && !snapshot.libraryId) void source.openLibrary(first.libraryId);
  }, [source, snapshot.status, snapshot.libraryId]);
  useEffect(() => {setEdit(undefined);}, [snapshot.titleId, snapshot.season]);
  const status = snapshot.status;
  const stateText = !status ? '' : status.mode === 'off' ? t('web.segments.stateOff') : status.files.pending || status.showsWaiting || !status.catchUpComplete ? t('web.segments.stateWorking') : t('web.segments.stateDone');
  const busy = !!snapshot.busy;
  const title = snapshot.title;
  const season = title?.kind === 'show' ? title.seasons.find(value => value.number === snapshot.season) ?? title.seasons[0] : undefined;
  const entries = title ? title.kind === 'show' ? season?.entries ?? [] : title.seasons.flatMap(value => value.entries) : [];
  return <>
    {modeRows}
    {snapshot.phase === 'error' && !status
      ? <ReviewNotice tone="bad" onRetry={() => {void source.load();}}>{t('web.segments.error')}</ReviewNotice>
      : status ? <StatTiles variant="segments" items={[
        {id: 'measured', label: t('web.segments.measured'), value: defaultI18n.number(status.files.complete)},
        {id: 'waiting', label: t('web.segments.waiting'), value: defaultI18n.number(status.files.pending)},
        {id: 'unreadable', label: t('web.segments.unreadable'), value: defaultI18n.number(status.files.failed + status.files.unsupported)},
        {id: 'status', label: t('web.segments.status'), value: stateText},
      ]}/> : <p className={styles.loading} role="status">{t('web.segments.loading')}</p>}

    <section className={styles.review} id="segments.review" aria-labelledby="segments-review-title">
      <header className={styles.reviewHead}>
        <h3 id="segments-review-title">{t('web.segments.review')}</h3>
        <div className={styles.legend} aria-hidden="true">
          <span data-kind="intro">{t('web.segments.kind', {kind: 'intro'})}</span>
          <span data-kind="recap">{t('web.segments.kind', {kind: 'recap'})}</span>
          <span data-kind="credits">{t('web.segments.kind', {kind: 'credits'})}</span>
          <span data-kind="waiting">{t('web.segments.titleSuggested', {count: 1}).replace('1', '…')}</span>
        </div>
      </header>
      {!status?.libraries.length ? <p className={styles.empty}>{t('web.segments.noLibraries')}</p> : <div className={styles.columns}>
        <div className={styles.shows} id="segments.libraries">
          <div className={styles.libraryTabs} role="group" aria-label={t('web.segments.libraries')}>
            {status.libraries.map(library => <button key={library.libraryId} type="button" aria-pressed={snapshot.libraryId === library.libraryId}
              className={styles.libraryTab} onClick={() => {void source.openLibrary(library.libraryId);}}>
              <b>{library.name}</b>
              <small>{t(library.itemKind === 'movie' ? 'web.segments.libraryModeFilms' : 'web.segments.libraryMode', {mode: library.mode})}{library.choice === 'server' ? ` · ${t('web.segments.followsServer')}` : ''}</small>
            </button>)}
          </div>
          {snapshot.titlesLoading && !snapshot.titles.length ? <p className={styles.loading} role="status">{t('web.segments.loading')}</p> : null}
          {snapshot.titlesError ? <ReviewNotice tone="bad">{t('web.segments.error')}</ReviewNotice> : null}
          {!snapshot.titlesLoading && status && snapshot.libraryId && !snapshot.titles.length ? <p className={styles.empty}>{t('web.segments.noTitles')}</p> : null}
          {snapshot.titles.map(value => <button key={value.id} type="button" className={styles.show} aria-current={snapshot.titleId === value.id || undefined} onClick={() => {void source.openTitle(value.id);}}>
            <b>{value.title || t('web.segments.untitled')}</b>
            <small>{titleLine(value)}</small>
            {value.markers.suggested ? <span className={styles.todo}>{t('web.segments.toReview', {count: value.markers.suggested})}</span> : null}
          </button>)}
          {snapshot.titlesCursor ? <Button variant="ghost" size="small" onClick={() => {void source.moreTitles();}} disabled={snapshot.titlesLoading}>{t('web.segments.showMore')}</Button> : null}
        </div>

        <div className={styles.pane}>
          {!title ? <p className={styles.empty}>{t('web.segments.chooseTitle')}</p> : <Review title={title} season={snapshot.season} entries={entries} snapshot={snapshot} edit={edit}
            onSeason={value => source.selectSeason(value)} onApproveAll={() => {void source.approve();}} onDetect={() => {void source.detectAgain();}}
            onRetryTitle={() => {if (snapshot.titleId) void source.openTitle(snapshot.titleId);}}
            onEdit={(entry, marker) => setEdit({entryId: entry.itemId, marker, start: seconds(marker.startUS), end: seconds(marker.endUS)})}
            onChange={(entry, value) => setEdit(current => current && current.entryId === entry.itemId ? {...current, start: value.start, end: value.end} : current)}
            onSave={(entry) => {if (!edit) return; void source.saveMarker(entry, edit.marker, edit.marker.kind, String(Math.round(edit.start * 1e6)), String(Math.round(edit.end * 1e6))); setEdit(undefined);}}
            onCancel={() => setEdit(undefined)}
            onApprove={(entry, marker) => {void source.approveMarker(entry, marker);}}
            busy={busy} />}
        </div>
      </div>}
    </section>
  </>;
}

function Review({title, season, entries, snapshot, edit, onSeason, onApproveAll, onDetect, onRetryTitle, onEdit, onChange, onSave, onCancel, onApprove, busy}: {
  title: SegmentTitleReview; season?: number; entries: readonly SegmentReviewEntry[]; snapshot: SegmentIntrosSnapshot;
  edit?: {entryId: string; marker: SegmentReviewMarker; start: number; end: number};
  onSeason: (value: number) => void; onApproveAll: () => void; onDetect: () => void; onRetryTitle: () => void;
  onEdit: (entry: SegmentReviewEntry, marker: SegmentReviewMarker) => void;
  onChange: (entry: SegmentReviewEntry, value: SegmentEdit) => void;
  onSave: (entry: SegmentReviewEntry) => void; onCancel: () => void;
  onApprove: (entry: SegmentReviewEntry, marker: SegmentReviewMarker) => void; busy: boolean;
}) {
  return <>
    <header className={styles.reviewTitle}>
      <div><b>{title.title}</b>{title.kind === 'show' && <label className={styles.season}>{t('web.segments.season', {number: season ?? 1})}
        <select aria-label={t('web.segments.season', {number: season ?? 1})} value={season ?? title.seasons[0]?.number} onChange={event => onSeason(Number(event.currentTarget.value))}>
          {title.seasons.map(value => <option key={value.number} value={value.number}>{value.title || t('web.segments.season', {number: value.number})}</option>)}
        </select>
        <Icon name="i-down" size={14} />
      </label>}</div>
      <div className={styles.actions}>
        <Button variant="glass" size="small" icon="i-check" onClick={onApproveAll} disabled={busy}>{t('web.segments.approveSeason')}</Button>
        <Button variant="ghost" size="small" icon="i-refresh" onClick={onDetect} disabled={busy}>{t('web.segments.detectAgain')}</Button>
      </div>
    </header>
    {snapshot.note ? <p className={styles.note} role="status">{snapshot.note}</p> : null}
    {snapshot.actionError ? <ReviewNotice tone="bad">{t('web.segments.actionError')}</ReviewNotice> : null}
    {snapshot.titleLoading ? <p className={styles.loading} role="status">{t('web.segments.loading')}</p> : null}
    {snapshot.titleError ? <ReviewNotice tone="bad" onRetry={onRetryTitle}>{t('web.segments.error')}</ReviewNotice> : null}
    {!snapshot.titleLoading && !entries.length ? <p className={styles.empty}>{t('web.segments.entryState', {state: 'complete'})}</p> : null}
    {entries.map(entry => <Episode key={entry.itemId} entry={entry} title={title} edit={edit?.entryId === entry.itemId ? edit : undefined} busy={busy}
      onEdit={onEdit} onChange={onChange} onSave={onSave} onCancel={onCancel} onApprove={onApprove} />)}
  </>;
}

function Episode({entry, title, edit, busy, onEdit, onChange, onSave, onCancel, onApprove}: {
  entry: SegmentReviewEntry; title: SegmentTitleReview; edit?: {entryId: string; marker: SegmentReviewMarker; start: number; end: number}; busy: boolean;
  onEdit: (entry: SegmentReviewEntry, marker: SegmentReviewMarker) => void;
  onChange: (entry: SegmentReviewEntry, value: SegmentEdit) => void;
  onSave: (entry: SegmentReviewEntry) => void; onCancel: () => void;
  onApprove: (entry: SegmentReviewEntry, marker: SegmentReviewMarker) => void;
}) {
  const duration = seconds(entry.durationUS);
  const opening = entry.markers.filter(marker => isOpening(marker.kind)).map(mark);
  const ending = entry.markers.filter(marker => !isOpening(marker.kind)).map(mark);
  const todo = entry.markers.filter(waiting).length;
  return <article className={styles.episode}>
    <header className={styles.episodeHead}>
      {title.kind === 'show' && <span className={styles.number}>{entry.number}</span>}
      <b>{entry.title || t('web.segments.untitled')}</b>
      {duration > 0 && <span className={styles.duration}>{Math.round(duration / 60)} min</span>}
      <span className={styles.state} data-todo={todo > 0 || undefined}>{todo > 0 ? t('web.segments.titleSuggested', {count: todo}) : t('web.segments.entryState', {state: 'complete'})}</span>
    </header>
    {duration > 0 && (opening.length || ending.length)
      ? <SegmentWindows duration={duration} opening={opening} ending={ending} edit={edit ? {id: edit.marker.id, start: edit.start, end: edit.end} : undefined}
          onEdit={value => {const marker = entry.markers.find(candidate => candidate.id === value.id); if (marker) onEdit(entry, marker);}}
          onChange={value => onChange(entry, value)} onSave={() => onSave(entry)} onCancel={onCancel}
          onApprove={value => {const marker = entry.markers.find(candidate => candidate.id === value.id); if (marker && !busy) onApprove(entry, marker);}} />
      : <p className={styles.empty}>{t('web.segments.entryState', {state: entry.state})}</p>}
  </article>;
}

function ReviewNotice({children, tone, onRetry}: {children: ReactNode; tone: 'bad'; onRetry?: () => void}) {
  return <div className={styles.notice} role="alert" data-tone={tone}><Icon name="i-info" size={16}/><span>{children}</span>{onRetry && <button type="button" onClick={onRetry}>{t('action.tryAgain')}</button>}</div>;
}
