import {useEffect, useRef, useState, type CSSProperties} from 'react';
import {defaultI18n} from '@i18n';
import {generatedCoverTint} from '@design';
import {createReadingPublication, type ReaderContentsTarget} from '@portico/reader-core';
import {stageContentsTarget, retireContentsTarget} from './contents-handoff';
import {ReaderContents, type ReaderContentsEntry} from '../../kit/reader/ReaderPanel';
import type {ReadingManifest, ReadingProgress} from '@portico/client-core';
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
  const [detail, setDetail] = useState<Readonly<{manifest?: ReadingManifest; progress?: ReadingProgress; targets?: readonly ReaderContentsTarget[]; failed?: boolean}>>({});
  const publicationReady = useRef(false);
  const captureTarget = useRef<((index: number) => ReaderContentsTarget) | undefined>(undefined);
  const anchor = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const bindings = createReadBindings(captured.context); let active = true;
    let publication: Awaited<ReturnType<typeof createReadingPublication>> | undefined;
    publicationReady.current = false; captureTarget.current = undefined; setDetail({});
    const off = bindings.authority.subscribe(() => {if (active) {publicationReady.current = false; setDetail({failed: true});}});
    if (!sourceId) setDetail({failed: true});
    else void (async () => {
      const session = await bindings.open({itemId: eligibility.itemId, sourceId});
      const progress = await session.progress();
      publication = await createReadingPublication(session);
      if (!active || !bindings.authority.isCurrent()) {publication.dispose(); return;}
      if (active && bindings.authority.isCurrent()) {publicationReady.current = true;
        captureTarget.current = index => {if (!active || !bindings.authority.isCurrent()) throw new Error('source_changed'); return publication!.contentsTarget(index);};
        setDetail({manifest: session.manifest, progress,
        targets: session.manifest.format === 'epub' ? (session.manifest.contents ?? []).map((_, index) => publication!.contentsTarget(index)) : []});}
    })().catch(error => {if (active && bindings.authority.isCurrent()) {errorSink.current(error); setDetail({failed: true});} void bindings.close().catch(() => {});});
    return () => {active = false; publicationReady.current = false; publication?.dispose(); off(); void bindings.dispose().catch(() => {});};
  }, [captured.context, eligibility.itemId, sourceId, retry]);
  if (detail.failed) return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => setRetry(value => value + 1)}}/>;
  const manifest = detail.manifest, version = eligibility.versions.find(version => version.sourceId === sourceId);
  if (!manifest || !version || manifest.sourceId !== sourceId) return <Skeleton height={480}/>;
  const title = manifest.metadata.title ?? '', author = manifest.metadata.author;
  const tint = generatedCoverTint(title);
  const saved = detail.progress?.progress;
  const progression = saved && !saved.stale && saved.sourceRevision === manifest.revision ? saved.location.progression : undefined;
  const open = (index?: number) => {
    if (!captured.context.isCurrent() || !publicationReady.current) return;
    let target: ReaderContentsTarget | undefined;
    try {target = index === undefined ? undefined : captureTarget.current?.(index);}
    catch (error) {errorSink.current(error); return;}
    if (index !== undefined && !target) return;
    stageContentsTarget(captured.context, eligibility.itemId, manifest.sourceId, target);
    void router.navigate(readDestination(eligibility.itemId, manifest.sourceId)).catch(error => {
      retireContentsTarget(captured.context, target);
      if (captured.context.isCurrent()) errorSink.current(error);
    });
  };
  /** No Version control with one file (notes/read-web.md); the details keep format and size. */
  const chooseVersion = eligibility.versions.length > 1;
  const viewer = context.identity?.viewer, canEdit = viewer?.authority === 'local' && viewer.role === 'owner';
  const details = <ReadBookFacts heading={t('next.reader.details')} facts={[
    ...(author ? [{id: 'author', label: t('filter.field.author'), value: author}] : []),
    ...(manifest.metadata.language ? [{id: 'language', label: t('next.selection.language'), value: manifest.metadata.language}] : []),
    {id: 'format', label: t('next.reader.format'), value: manifest.format.toUpperCase()},
    {id: 'size', label: t('next.reader.fileSize'), value: defaultI18n.bytes(manifest.size)},
  ]}/>;
  return <><ReadBookHero title={title} author={author ? {name: author} : undefined} facts={[]}
    ambient={<span className={styles.ambient} style={{'--read-tint-first': tint.first, '--read-tint-second': tint.second} as CSSProperties} aria-hidden="true"/>}
    actions={progression !== undefined && progression > 0 ? <ResumeButton icon="i-book" progress={progression} onClick={() => open()}>{t('next.cards.read')}</ResumeButton>
      : <Button variant="primary" icon="i-book" onClick={() => open()}>{t('next.reader.read')}</Button>}
    under={<>{canEdit && <><Button ref={editorAnchor} size="small" variant="glass" icon="i-more" iconOnly aria-label={t('next.controls.more')} onClick={() => setMore(value => !value)}/><Menu open={more} onOpenChange={setMore} title={title} anchorRef={editorAnchor}><MenuItem onSelect={() => {if (context.isCurrent()) setEditing(true);}}>{t('entry.editMetadata')}</MenuItem></Menu></>}{chooseVersion ? <><Button ref={anchor} size="small" variant="glass" onClick={() => setMenu(value => !value)} aria-label={t('next.reader.version')}>
      {version.format.toUpperCase()}<Icon name="i-down" size={14}/></Button>
      <Menu open={menu} onOpenChange={setMenu} title={t('next.reader.version')} anchorRef={anchor}>{eligibility.versions.map(candidate => <MenuItem key={candidate.sourceId}
        selected={candidate.sourceId === sourceId} disabled={!candidate.inventoryAvailable} onSelect={() => setSourceId(candidate.sourceId)} description={defaultI18n.bytes(candidate.size)}>
        {candidate.format.toUpperCase()}</MenuItem>)}</Menu></> : null}</>}/>
    {editing && canEdit && <ReadMetadata context={context} reading={captured.context} sourceId={manifest.sourceId} format={manifest.format} itemId={eligibility.itemId} anchor={editorAnchor} onClose={() => {setEditing(false); setRetry(value => value + 1);}}/>}
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
