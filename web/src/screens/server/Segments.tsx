import {useState} from 'react';
import {approveSegments, editSegmentMarker, parseSegmentClock, readSegmentStatus, readSegmentTitle, readSegmentTitles, redetectSegments, segmentClock, segmentKinds, type SegmentKind, type SegmentReviewEntry, type SegmentReviewMarker, type SegmentReviewSeason, type SegmentTitlePage, type SegmentTitleSummary} from '@core/segment-review.ts';
import {problem, useAction, useRead} from '../../admin/console';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {usePlayerActions} from '../../player/PlayerContext';
import {Badge, Button, Dialog, Input, Loading, Metric, MetricGrid, Notice, Select, SettingsGroup, SettingsNote, SettingsRow, Status, Text} from '../../ui';

/**
 * Server › Intros and credits: what detection has done and is doing, each video library's mode,
 * and the owner's review of one show or film at a time. Single markers change through the title
 * analysis marker route with the targets the review carries; a season's detections can be
 * approved together, and a title measured again without losing the owner's decisions.
 */
export function SegmentsPanel() {
  const {t} = useI18n();
  const {api} = useSession();
  const status = useRead(() => readSegmentStatus(api), [api], {every: 15000});
  const [library, setLibrary] = useState<string>('');
  const [title, setTitle] = useState<string>('');
  const s = status.data;
  const unreadable = s ? s.files.failed + s.files.unsupported : 0;
  if (title) return <SegmentReview id={title} onBack={() => setTitle('')} />;
  return (
    <>
      <SettingsGroup title={t('web.segments.progress')}>
        {status.error ? <SettingsNote><Notice tone="warning" compact action={{label: t('action.tryAgain'), onClick: status.reload}}>{status.error}</Notice></SettingsNote> : null}
        {!s && !status.error ? <SettingsNote><Loading label={t('web.segments.loading')} /></SettingsNote> : null}
        {s ? <SettingsNote>
          <MetricGrid>
            <Metric label={t('web.segments.measured')} value={s.files.complete} />
            <Metric label={t('web.segments.waiting')} value={s.files.pending} sub={s.showsWaiting ? t('web.segments.showsWaiting', {count: s.showsWaiting}) : undefined} />
            <Metric label={t('web.segments.unreadable')} value={unreadable} />
          </MetricGrid>
        </SettingsNote> : null}
        {s ? <SettingsRow label={t('web.segments.status')} control={<Status tone={s.mode === 'off' ? 'neutral' : s.files.pending || s.showsWaiting || !s.catchUpComplete ? 'accent' : 'healthy'}>{t(s.mode === 'off' ? 'web.segments.stateOff' : s.files.pending || s.showsWaiting || !s.catchUpComplete ? 'web.segments.stateWorking' : 'web.segments.stateDone')}</Status>} />
          : null}
        {s?.failures.map(f => <SettingsRow key={f.itemId + f.updatedMS} icon="warning" label={f.title || t('web.segments.untitled')} help={t('web.segments.failure', {state: f.state})} />)}
      </SettingsGroup>
      {s && s.libraries.length ? <SettingsGroup title={t('web.segments.libraries')}>
        {s.libraries.map(l => (
          <SettingsRow key={l.libraryId} label={l.name} help={`${t(l.itemKind === 'movie' ? 'web.segments.libraryModeFilms' : 'web.segments.libraryMode', {mode: l.mode})}${l.choice === 'server' ? ' · ' + t('web.segments.followsServer') : ''}`}
            control={<Button size="sm" variant={library === l.libraryId ? 'secondary' : 'ghost'} label={t('web.segments.review')} onClick={() => setLibrary(library === l.libraryId ? '' : l.libraryId)} />} />
        ))}
      </SettingsGroup> : null}
      {library ? <SegmentTitles library={library} name={s?.libraries.find(l => l.libraryId === library)?.name ?? ''} onOpen={setTitle} /> : null}
    </>
  );
}

function SegmentTitles({library, name, onOpen}: {library: string; name: string; onOpen: (id: string) => void}) {
  const {t} = useI18n();
  const {api} = useSession();
  const [more, setMore] = useState<SegmentTitlePage[]>([]);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const first = useRead(() => { setMore([]); return readSegmentTitles(api, library); }, [api, library]);
  const pages = first.data ? [first.data, ...more] : [];
  const titles = pages.flatMap(p => p.titles);
  const cursor = pages.length ? pages[pages.length - 1]!.nextCursor : '';
  const loadMore = () => {
    if (!cursor || loadingMore) return;
    setLoadingMore(true);
    readSegmentTitles(api, library, cursor).then(p => setMore(m => [...m, p]), e => setError(problem(e))).finally(() => setLoadingMore(false));
  };
  const line = (x: SegmentTitleSummary) => [
    t('web.segments.titleFiles', {measured: x.files.complete, total: x.files.complete + x.files.pending + x.files.failed + x.files.unsupported}),
    x.markers.automatic ? t('web.segments.titleAutomatic', {count: x.markers.automatic}) : '',
    x.markers.suggested ? t('web.segments.titleSuggested', {count: x.markers.suggested}) : '',
    x.markers.approved ? t('web.segments.titleApproved', {count: x.markers.approved}) : '',
  ].filter(Boolean).join(' · ');
  return (
    <SettingsGroup title={t('web.segments.titlesIn', {library: name})}>
      {first.error || error ? <SettingsNote><Notice tone="warning" compact action={{label: t('action.tryAgain'), onClick: first.reload}}>{first.error || error}</Notice></SettingsNote> : null}
      {!first.data && !first.error ? <SettingsNote><Loading label={t('web.segments.loading')} /></SettingsNote> : null}
      {first.data && !titles.length ? <SettingsNote><Text tone="tertiary">{t('web.segments.noTitles')}</Text></SettingsNote> : null}
      {titles.map(x => <SettingsRow key={x.id} label={x.title || t('web.segments.untitled')} help={line(x)} onClick={() => onOpen(x.id)} control={x.markers.suggested ? <Badge tone="accent">{t('web.segments.toReview', {count: x.markers.suggested})}</Badge> : undefined} />)}
      {cursor ? <SettingsNote><Button size="sm" variant="ghost" label={t('web.segments.showMore')} disabled={loadingMore} onClick={loadMore} /></SettingsNote> : null}
    </SettingsGroup>
  );
}

type Editing = Readonly<{entry: SegmentReviewEntry; marker?: SegmentReviewMarker}>;

function SegmentReview({id, onBack}: {id: string; onBack: () => void}) {
  const {t} = useI18n();
  const {api} = useSession();
  const player = usePlayerActions();
  const read = useRead(() => readSegmentTitle(api, id), [api, id]);
  const action = useAction();
  const [editing, setEditing] = useState<Editing | null>(null);
  // What a batch did ("6 markers approved"): counted by the server, so said after it answers.
  const [note, setNote] = useState('');
  const review = read.data;
  const change = (fn: () => Promise<unknown>) => { setNote(''); void action.run(async () => { try { await fn(); } finally { read.reload(); } }); };
  const seasonTitle = (season: SegmentReviewSeason) => season.title || (season.number === 0 ? t('web.segments.specials') : season.number < 0 ? t('web.segments.episodes') : t('web.segments.season', {number: season.number}));
  const trust = (m: SegmentReviewMarker) => m.approved ? 'approved' : !m.viewerVisible ? 'hidden' : m.automaticSafe ? 'automatic' : 'suggested';
  const markerLine = (m: SegmentReviewMarker) => [t('web.segments.range', {start: segmentClock(m.startUS), end: segmentClock(m.endUS)}), t('web.segments.basis', {basis: m.basis}), m.basis === 'owner' ? '' : t('web.segments.confidence', {percent: Math.round(m.confidence * 100)})].filter(Boolean).join(' · ');
  return (
    <>
      <SettingsGroup title={review?.title ?? t('web.segments.reviewTitle')}
        action={<Button size="sm" variant="ghost" icon="back" label={t('web.segments.allTitles')} onClick={onBack} />}>
        {read.error ? <SettingsNote><Notice tone="warning" compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
        {action.error ? <SettingsNote><Notice tone="error" compact>{action.error}</Notice></SettingsNote> : null}
        {note && !action.error ? <SettingsNote><Notice tone="success" compact>{note}</Notice></SettingsNote> : null}
        {!review && !read.error ? <SettingsNote><Loading label={t('web.segments.loading')} /></SettingsNote> : null}
        {review ? <SettingsRow label={t('web.segments.detectAgain')} help={t('web.segments.detectAgainHelp')} control={<Button size="sm" variant="secondary" icon="refresh" label={t('web.segments.detectAgain')} disabled={action.busy} onClick={() => change(async () => { const n = await redetectSegments(api, id); setNote(t('web.segments.detectAgainQueued', {count: n})); })} />} />
          : null}
      </SettingsGroup>
      {review?.seasons.map(season => (
        <SettingsGroup key={season.number} title={seasonTitle(season)}
          action={season.entries.some(e => e.markers.some(m => !m.approved && !m.edited && m.basis !== 'owner')) ? <Button size="sm" variant="ghost" icon="check" label={t('web.segments.approveSeason')} disabled={action.busy} onClick={() => change(async () => { const n = await approveSegments(api, id, ['intro', 'recap', 'credits', 'outro'], review.kind === 'show' ? season.number : undefined); setNote(t('web.segments.approvedCount', {count: n})); })} /> : undefined}>
          {season.entries.map(entry => (
            <div key={entry.itemId}>
              <SettingsRow label={review.kind === 'show' ? t('web.segments.episode', {number: entry.number, title: entry.title}) : entry.title}
                help={entry.markers.length ? undefined : t('web.segments.entryState', {state: entry.state})}
                control={entry.sourceId ? <Button size="sm" variant="ghost" icon="plus" label={t('web.segments.add')} disabled={action.busy} onClick={() => setEditing({entry})} /> : undefined} />
              {entry.markers.map(m => (
                <SettingsRow key={m.id} stack label={t('web.segments.kind', {kind: m.kind})} help={markerLine(m)}
                  state={<Status tone={trust(m) === 'automatic' || trust(m) === 'approved' ? 'healthy' : trust(m) === 'hidden' ? 'neutral' : 'accent'}>{t('web.segments.trust', {trust: trust(m)})}</Status>}
                  control={<>
                    <Button size="sm" variant="ghost" icon="play" label={t('web.segments.watch')} onClick={() => player.play(entry.itemId, Math.max(0, Number(m.startUS) / 1e6 - 3))} />
                    {!m.approved ? <Button size="sm" variant="ghost" icon="check" label={t('web.segments.approve')} disabled={action.busy} onClick={() => change(() => editSegmentMarker(api, entry, {action: 'approve', markerId: m.id}))} /> : null}
                    <Button size="sm" variant="ghost" icon="edit" label={t('web.segments.adjust')} disabled={action.busy} onClick={() => setEditing({entry, marker: m})} />
                    <Button size="sm" variant="ghost" icon="close" label={t('action.dismiss')} disabled={action.busy} onClick={() => change(() => editSegmentMarker(api, entry, {action: 'dismiss', markerId: m.id}))} />
                  </>} />
              ))}
            </div>
          ))}
        </SettingsGroup>
      ))}
      {editing ? <MarkerDialog editing={editing} busy={action.busy} onClose={() => setEditing(null)} onSave={(kind, startUS, endUS) => { const e = editing; setEditing(null); change(() => editSegmentMarker(api, e.entry, e.marker ? {action: 'edit', markerId: e.marker.id, kind, startUS, endUS} : {action: 'create', kind, startUS, endUS})); }} /> : null}
    </>
  );
}

function MarkerDialog({editing, busy, onClose, onSave}: {editing: Editing; busy: boolean; onClose: () => void; onSave: (kind: SegmentKind, startUS: string, endUS: string) => void}) {
  const {t} = useI18n();
  const [kind, setKind] = useState<SegmentKind>(editing.marker?.kind ?? 'intro');
  const [start, setStart] = useState(editing.marker ? segmentClock(editing.marker.startUS) : '');
  const [end, setEnd] = useState(editing.marker ? segmentClock(editing.marker.endUS) : '');
  const startUS = parseSegmentClock(start), endUS = parseSegmentClock(end);
  const duration = Number(editing.entry.durationUS || 0);
  const valid = startUS !== null && endUS !== null && Number(startUS) < Number(endUS) && (!duration || Number(endUS) <= duration);
  return (
    <Dialog open onOpenChange={open => { if (!open) onClose(); }} title={t(editing.marker ? 'web.segments.adjustTitle' : 'web.segments.addTitle')} width={420}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        <Select label={t('web.segments.kindLabel')} value={kind} onChange={e => setKind(e.target.value as SegmentKind)} options={segmentKinds.map(k => ({value: k, label: t('web.segments.kind', {kind: k})}))} />
        <Input label={t('web.segments.start')} value={start} onChange={e => setStart(e.target.value)} placeholder="0:00.0" />
        <Input label={t('web.segments.end')} value={end} onChange={e => setEnd(e.target.value)} placeholder="1:30.0" />
        <Text variant="caption" tone="tertiary">{t('web.segments.timeHelp')}</Text>
        <div style={{display: 'flex', gap: 8, justifyContent: 'flex-end'}}>
          <Button variant="ghost" label={t('action.cancel')} onClick={onClose} />
          <Button variant="primary" label={t('action.save')} disabled={!valid || busy} onClick={() => { if (valid) onSave(kind, startUS!, endUS!); }} />
        </div>
      </div>
    </Dialog>
  );
}
