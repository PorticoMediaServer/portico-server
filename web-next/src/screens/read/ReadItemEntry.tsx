import {ReadSavedAction} from './ReadSavedAction';
import {ReadMarkAction} from './ReadMarkAction';
import {useEffect, useLayoutEffect, useRef, useState, type CSSProperties} from 'react';
import {defaultI18n} from '@i18n';
import {generatedCoverTint} from '@design';
import {createReadingPublication, type ReaderContentsTarget} from '@portico/reader-core';
import {stageContentsTarget, retireContentsTarget} from './contents-handoff';
import {ReaderContents, type ReaderContentsEntry} from '../../kit/reader/ReaderPanel';
import type {ReadingManifest, ReadingProgress, ReadingSession} from '@portico/client-core';
import {saveReadingFile} from './save-file';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {Icon} from '../../kit/Icon';
import {Button, ResumeButton} from '../../kit/controls';
import {Menu, MenuItem} from '../../kit/overlays';
import {Skeleton, StateView} from '../../kit/states';
import {ReadBookFacts, ReadBookHero, ReadBookSections} from '../../kit/reader/ReadBookHero';
import {useLibraryScreenContext} from '../browse/screen-context';
import {ReadMetadata} from './ReadMetadata';
import {createReadBindings} from './bindings';
import {readDestination, type ReadItemEligibility} from './navigation';
import styles from './read-item.module.css';

type Captured = NonNullable<ReturnType<typeof useProtectedWebScreenContext>>;
type ReadAttempt = {readonly context: Captured['context']; readonly sourceId: string | undefined; readonly retry: number; readonly fileLife: AbortController; session?: ReadingSession; saving?: boolean; live: boolean; ready: boolean; target?: (index: number) => ReaderContentsTarget};
const {t} = defaultI18n;
/** The primary Item registration supplies a genuine Reading resource result. */
export function ReadItemEntry({itemId, eligibility}: {itemId: string; eligibility: ReadItemEligibility}) {
  const captured = useProtectedWebScreenContext();
  return captured && eligibility.itemId === itemId ? <BoundReadItem key={itemId} captured={captured} eligibility={eligibility}/> : null;
}
function BoundReadItem({captured, eligibility}: {captured: Captured; eligibility: ReadItemEligibility}) {
  const context = useLibraryScreenContext(captured);
  const [editing, setEditing] = useState(false), [more, setMore] = useState(false);
  const editorAnchor = useRef<HTMLButtonElement>(null);
  const errorSink = useRef(captured.binding.home.effects.reportError); errorSink.current = captured.binding.home.effects.reportError;
  const [sourceId, setSourceId] = useState(() => eligibility.versions.find(version => version.inventoryAvailable)?.sourceId);
  const [menu, setMenu] = useState(false), [retry, setRetry] = useState(0);
  const [saving, setSaving] = useState(false);
  const [attempt, setAttempt] = useState<ReadAttempt>();
  const attemptRef = useRef<ReadAttempt>();
  const renderIdentity = useRef({context: captured.context, sourceId, retry});
  renderIdentity.current = {context: captured.context, sourceId, retry};
  const [detail, setDetail] = useState<Readonly<{attempt?: ReadAttempt; manifest?: ReadingManifest; progress?: ReadingProgress; client?: ReturnType<typeof createReadBindings>['client']; targets?: readonly ReaderContentsTarget[]; failed?: boolean}>>({});
  const current = (value: ReadAttempt | undefined): value is ReadAttempt => !!value && value.live && attemptRef.current === value
    && renderIdentity.current.context === value.context && renderIdentity.current.sourceId === value.sourceId && renderIdentity.current.retry === value.retry && value.context.isCurrent();
  const retire = (value: ReadAttempt) => {value.fileLife.abort(); value.live = false; value.ready = false; value.target = undefined; if (attemptRef.current === value) attemptRef.current = undefined;};
  const refresh = (value: ReadAttempt | undefined) => {if (!current(value)) return; retire(value); setRetry(count => count + 1);};
  useLayoutEffect(() => {
    const value: ReadAttempt = {context: captured.context, sourceId, retry, fileLife: new AbortController(), live: true, ready: false};
    attemptRef.current = value; setAttempt(value); setSaving(false);
    return () => retire(value);
  }, [captured.context, sourceId, retry]);
  const anchor = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!current(attempt)) return;
    const original = attempt;
    const bindings = createReadBindings(original.context);
    let active = true;
    const live = () => active && current(original) && bindings.authority.isCurrent();
    let publication: Awaited<ReturnType<typeof createReadingPublication>> | undefined;
    const off = bindings.authority.subscribe(() => {if (active && attemptRef.current === original) {original.fileLife.abort(); original.ready = false; original.target = undefined; setDetail({attempt: original, failed: true});}});
    const requestedSource = original.sourceId;
    if (!requestedSource) setDetail({attempt: original, failed: true});
    else void (async () => {
      const session = await bindings.open({itemId: eligibility.itemId, sourceId: requestedSource});
      if (!live()) return;
      const progress = await session.progress();
      if (!live()) return;
      publication = await createReadingPublication(session);
      if (!live()) {publication.dispose(); return;}
      original.ready = true;
      original.session = session;
      original.target = index => {if (!live() || !original.ready) throw new Error('source_changed'); return publication!.contentsTarget(index);};
      setDetail({attempt: original, manifest: session.manifest, progress, client: bindings.client,
        targets: session.manifest.format === 'epub' ? (session.manifest.contents ?? []).map((_, index) => publication!.contentsTarget(index)) : []});
    })().catch(error => {if (live()) {errorSink.current(error); original.ready = false; original.target = undefined; setDetail({attempt: original, failed: true});} void bindings.close().catch(() => {});});
    return () => {active = false; original.ready = false; original.target = undefined; publication?.dispose(); off(); void bindings.dispose().catch(() => {});};
  }, [attempt, eligibility.itemId]);
  if (current(attempt) && detail.attempt === attempt && detail.failed) return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => refresh(attempt)}}/>;
  const manifest = current(attempt) && detail.attempt === attempt ? detail.manifest : undefined, version = eligibility.versions.find(version => version.sourceId === sourceId);
  if (!manifest || !version || manifest.sourceId !== sourceId) return <Skeleton height={480}/>;
  const title = manifest.metadata.title ?? '', author = manifest.metadata.author;
  const metadata = manifest.metadata, series = metadata.series;
  const original = detail.attempt;
  const factsCurrent = () => current(original) && original === attempt && original.ready && detail.manifest === manifest;
  const canSaveFile = manifest.canSaveFile === true && manifest.size <= 64 << 20;
  const saveFile = async () => {
    const session = original?.session;
    if (!original || !session || original.saving || original.fileLife.signal.aborted || !factsCurrent()
      || session.manifest !== manifest || !canSaveFile) return;
    original.saving = true; setSaving(true);
    const live = () => factsCurrent() && original.session === session && !original.fileLife.signal.aborted;
    try {await saveReadingFile(session, original.fileLife.signal, live);}
    catch (error) {if (live()) errorSink.current(error);}
    finally {original.saving = false; if (current(original)) setSaving(false);}
  };
  const seriesPresentation = series ? {label: series.number === undefined ? series.name : t('next.reader.seriesPosition', {series: series.name, number: defaultI18n.number(series.number)}), onOpen: () => {
    if (!factsCurrent()) return;
    const publicationType = metadata.publicationType?.value;
    void router.navigate({to: '/library/$libraryId', params: {libraryId: series.libraryId}, search: {view: 'series', entityId: series.id,
      ...(publicationType === 'magazine' ? {pivot: 'magazines'} : publicationType === 'newspaper' ? {pivot: 'newspapers'} : {})}}).catch(error => {if (factsCurrent()) errorSink.current(error);});
  }} : undefined;
  const tint = generatedCoverTint(title);
  const saved = detail.progress?.progress;
  const progression = saved && !saved.stale && saved.sourceId === manifest.sourceId && saved.sourceRevision === manifest.revision && saved.location.format === manifest.format ? saved.location.progression : undefined;
  const open = (index?: number) => {
    if (!factsCurrent()) return;
    let target: ReaderContentsTarget | undefined;
    try {target = index === undefined ? undefined : original?.target?.(index);}
    catch (error) {errorSink.current(error); return;}
    if (index !== undefined && !target) return;
    stageContentsTarget(captured.context, eligibility.itemId, manifest.sourceId, target);
    void router.navigate(readDestination(eligibility.itemId, manifest.sourceId)).catch(error => {
      retireContentsTarget(captured.context, target);
      if (factsCurrent()) errorSink.current(error);
    });
  };
  /** No Version control with one file (notes/read-web.md); the details keep format and size. */
  const chooseVersion = eligibility.versions.length > 1;
  const viewer = context.identity?.viewer, canEdit = viewer?.authority === 'local' && viewer.role === 'owner';
  const details = <ReadBookFacts heading={t('next.reader.details')} facts={[
    ...(author ? [{id: 'author', label: t('filter.field.author'), value: author}] : []),
    ...(manifest.metadata.language ? [{id: 'language', label: t('next.selection.language'), value: manifest.metadata.language}] : []),
    ...(metadata.wordCount !== undefined ? [{id: 'words', label: t('next.reader.wordsLabel'), value: defaultI18n.number(metadata.wordCount)}] : []),
    ...(manifest.format === 'cbz' && manifest.pageNames?.length ? [{id: 'pages', label: t('next.reader.pagesStrip'), value: defaultI18n.number(manifest.pageNames.length)}] : []),
    {id: 'format', label: t('next.reader.format'), value: manifest.format.toUpperCase()},
    {id: 'size', label: t('next.reader.fileSize'), value: defaultI18n.bytes(manifest.size)},
  ]}/>;
  return <><ReadBookHero cover={context.artwork.foreground(manifest.metadata.coverUrl, 784, 2 / 3)} title={title} author={author ? {name: author} : undefined} series={seriesPresentation} facts={[
      ...(metadata.wordCount !== undefined ? [{id: 'words', text: t('next.reader.wordCount', {count: metadata.wordCount}), role: 'context' as const}] : []),
      ...(metadata.pageCount !== undefined ? [{id: 'pages', text: t('next.reader.pageCount', {count: metadata.pageCount}), role: 'context' as const}] : []),
      ...(metadata.estimatedSeconds !== undefined && metadata.estimatedSeconds > 0 ? [{id: 'estimate', text: t('next.reader.aboutDuration', {duration: defaultI18n.duration(metadata.estimatedSeconds)}), role: 'context' as const}] : []),
    ]} opening={metadata.openingLines ? {label: t('next.reader.openingLines'), text: metadata.openingLines} : undefined}
    ambient={<span className={styles.ambient} style={{'--read-tint-first': tint.first, '--read-tint-second': tint.second} as CSSProperties} aria-hidden="true"/>}
    actions={<>{progression !== undefined && progression > 0 ? <ResumeButton icon="i-book" progress={progression} remaining={defaultI18n.percent(progression)} onClick={() => open()}>{t('next.cards.read')}</ResumeButton>
      : <Button variant="primary" icon="i-book" onClick={() => open()}>{t('next.reader.read')}</Button>}{detail.client && <ReadSavedAction client={detail.client} itemId={eligibility.itemId} isCurrent={factsCurrent} reportError={error => errorSink.current(error)}/>}{detail.client && <ReadMarkAction client={detail.client} itemId={eligibility.itemId} isCurrent={factsCurrent} reportError={error => errorSink.current(error)}/>}</>}
    under={<>{(canEdit || canSaveFile) && <><Button ref={editorAnchor} size="small" variant="glass" icon="i-more" iconOnly aria-label={t('next.controls.more')} onClick={() => setMore(value => !value)}/><Menu open={more} onOpenChange={setMore} title={title} anchorRef={editorAnchor}>{canEdit && <MenuItem onSelect={() => {if (context.isCurrent()) setEditing(true);}}>{t('entry.editMetadata')}</MenuItem>}{canSaveFile && <MenuItem disabled={saving} onSelect={() => {void saveFile();}}>{t('next.reader.saveFile')}</MenuItem>}</Menu></>}{chooseVersion ? <><Button ref={anchor} size="small" variant="glass" onClick={() => setMenu(value => !value)} aria-label={t('next.reader.version')}>
      {version.format.toUpperCase()}<Icon name="i-down" size={14}/></Button>
      <Menu open={menu} onOpenChange={setMenu} title={t('next.reader.version')} anchorRef={anchor}>{eligibility.versions.map(candidate => <MenuItem key={candidate.sourceId}
        selected={candidate.sourceId === sourceId} disabled={!candidate.inventoryAvailable} onSelect={() => {if (candidate.sourceId === sourceId || !factsCurrent() || !original) return; retire(original); setSourceId(candidate.sourceId);}} description={defaultI18n.bytes(candidate.size)}>
        {candidate.format.toUpperCase()}</MenuItem>)}</Menu></> : null}</>}/>
    {editing && canEdit && <ReadMetadata context={context} reading={captured.context} sourceId={manifest.sourceId} format={manifest.format} itemId={eligibility.itemId} anchor={editorAnchor} onClose={() => {if (!current(original)) return; setEditing(false); refresh(original);}}/>}
    <ReadBookSections contents={detail.targets?.length ? <><h2>{t('next.reader.contents')}</h2><ReaderContents entries={detailContents(manifest.contents ?? [], index => open(index))}/></> : <span hidden/>} details={details}/></>;
}

/** Preserve the file's hierarchy and original index; no inferred page or progress labels. */
function detailContents(contents: NonNullable<ReadingManifest['contents']>, navigate: (index: number) => void): readonly ReaderContentsEntry[] {
  type Branch = {id: string; label: string; onNavigate: () => void; children: Branch[]};
  const roots: Branch[] = [], ancestors: {depth: number; entry: Branch}[] = [];
  contents.forEach((item, index) => {
    while (ancestors.length && ancestors[ancestors.length - 1].depth >= item.depth) ancestors.pop();
    const entry: Branch = {id: String(index), label: item.title, onNavigate: () => navigate(index), children: []};
    const parent = ancestors[ancestors.length - 1];
    (parent ? parent.entry.children : roots).push(entry);
    ancestors.push({depth: item.depth, entry});
  });
  return roots;
}
