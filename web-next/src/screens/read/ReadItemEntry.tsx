import {useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {ReadingManifest, ReadingProgress} from '@portico/client-core';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {Icon} from '../../kit/Icon';
import {Button, ResumeButton} from '../../kit/controls';
import {Menu, MenuItem} from '../../kit/overlays';
import {Skeleton, StateView} from '../../kit/states';
import {ReadBookFacts, ReadBookHero, ReadBookSections} from '../../kit/reader/ReadBookHero';
import {createReadBindings} from './bindings';
import {readDestination, type ReadItemEligibility} from './navigation';

type Captured = NonNullable<ReturnType<typeof useProtectedWebScreenContext>>;
const {t} = defaultI18n;
/** The primary Item registration supplies a genuine Reading resource result. */
export function ReadItemEntry({itemId, eligibility}: {itemId: string; eligibility: ReadItemEligibility}) {
  const captured = useProtectedWebScreenContext();
  return captured && eligibility.itemId === itemId ? <BoundReadItem key={itemId} captured={captured} eligibility={eligibility}/> : null;
}
function BoundReadItem({captured, eligibility}: {captured: Captured; eligibility: ReadItemEligibility}) {
  const [sourceId, setSourceId] = useState(() => eligibility.versions.find(version => version.inventoryAvailable)?.sourceId);
  const [menu, setMenu] = useState(false), [retry, setRetry] = useState(0);
  const [detail, setDetail] = useState<Readonly<{manifest?: ReadingManifest; progress?: ReadingProgress; failed?: boolean}>>({});
  const anchor = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const bindings = createReadBindings(captured.context); let active = true;
    setDetail({});
    const off = bindings.authority.subscribe(() => {if (active) setDetail({failed: true});});
    if (!sourceId) setDetail({failed: true});
    else void (async () => {
      const session = await bindings.open({itemId: eligibility.itemId, sourceId});
      const progress = await session.progress();
      await bindings.close().catch(captured.binding.home.effects.reportError);
      if (active && bindings.authority.isCurrent()) setDetail({manifest: session.manifest, progress});
    })().catch(error => {if (active && bindings.authority.isCurrent()) {captured.binding.home.effects.reportError(error); setDetail({failed: true});} void bindings.close().catch(() => {});});
    return () => {active = false; off(); void bindings.dispose().catch(() => {});};
  }, [captured.context, eligibility.itemId, sourceId, retry]);
  if (detail.failed) return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => setRetry(value => value + 1)}}/>;
  const manifest = detail.manifest, version = eligibility.versions.find(version => version.sourceId === sourceId);
  if (!manifest || !version) return <Skeleton height={480}/>;
  const saved = detail.progress?.progress;
  const progression = saved && !saved.stale && saved.sourceRevision === manifest.revision ? saved.location.progression : undefined;
  const open = () => {if (captured.context.isCurrent()) void router.navigate(readDestination(eligibility.itemId, manifest.sourceId)).catch(captured.binding.home.effects.reportError);};
  return <><ReadBookHero title={manifest.metadata.title ?? ''} author={manifest.metadata.author ? {name: manifest.metadata.author} : undefined} facts={[]}
    actions={progression !== undefined && progression > 0 ? <ResumeButton icon="i-book" progress={progression} onClick={open}>{t('next.cards.read')}</ResumeButton>
      : <Button variant="primary" icon="i-book" onClick={open}>{t('next.reader.read')}</Button>}
    under={<><Button ref={anchor} size="small" variant="glass" onClick={() => setMenu(value => !value)} aria-label={t('next.reader.version')}>
      {version.format.toUpperCase()}<Icon name="i-down" size={14}/></Button>
      <Menu open={menu} onOpenChange={setMenu} title={t('next.reader.version')} anchorRef={anchor}>{eligibility.versions.map(candidate => <MenuItem key={candidate.sourceId}
        selected={candidate.sourceId === sourceId} disabled={!candidate.inventoryAvailable} onSelect={() => setSourceId(candidate.sourceId)} description={defaultI18n.bytes(candidate.size)}>
        {candidate.format.toUpperCase()}</MenuItem>)}</Menu></>}/>
    <ReadBookSections details={<ReadBookFacts heading={t('next.reader.details')} facts={[
      {id: 'format', label: t('next.reader.format'), value: manifest.format.toUpperCase()},
      {id: 'size', label: t('next.reader.fileSize'), value: defaultI18n.bytes(manifest.size)},
    ]}/>}/></>;
}
