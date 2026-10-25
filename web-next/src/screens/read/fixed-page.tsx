import {isComicReadingFormat} from '@core/reading-format.ts';
import {useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore, type RefObject} from 'react';
import {defaultI18n} from '@i18n';
import type {ReadingPreferences} from '@portico/client-core';
import {createWindowedCollection, type WindowedCollection} from '@core/collections/index.ts';
import {WindowedGrid} from '../../kit/layout/WindowedGrid';
import {readerErrorCode, type ReadingController, type ReaderErrorCode, type ReaderFit, type ReaderFormat, type ReaderLocation, type ReaderZoom, type ReaderZoomState} from '@portico/reader-core';
import {Menu, MenuItem, MenuSeparator} from '../../kit/overlays';
import {loadReadPagePreview, type ReadPagePreview} from './preview';
import type {ReaderMountAuthority} from './mount';
import styles from './fixed-page.module.css';

const {t} = defaultI18n;

type Fixed = Readonly<{controller: ReadingController; authority: ReaderMountAuthority; format: ReaderFormat;
  reportError: (code: ReaderErrorCode) => void}>;

const fitLabel = (fit: ReaderFit): string => t(fit === 'page' ? 'next.reader.fitPage' : fit === 'width' ? 'next.reader.fitWidth'
  : fit === 'height' ? 'next.reader.fitHeight' : 'next.reader.fitSpread');

/** The observed fit or scale; nothing is claimed before the engine reports one. */
export function fixedPageZoomLabel(zoom: ReaderZoomState | null): string {
  if (!zoom) return t('next.reader.fit');
  return zoom.fit ? fitLabel(zoom.fit) : defaultI18n.percent(zoom.scale);
}

/**
 * Fixed pages (PDF, CBZ) draw the file's own pages in the reader's footer. A bounded
 * visible window is rendered from the current document, so a 300-page PDF never opens
 * every page to build it, and no second archive or preview cache lives here.
 */
export function FixedPageStrip({controller, authority, reportError, format}: Fixed) {
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const count = snapshot.metadata?.pageCount, location = snapshot.location;
  const current = location && location.format !== 'epub' ? location.page : undefined;
  if (format === 'epub' || !count || count < 2 || current === undefined || snapshot.status !== 'ready') return null;
  const first = Math.max(0, Math.min(current - 5, count - 11)), last = Math.min(count, first + 11);
  return <div className={styles.strip} role="group" aria-label={t('next.reader.pagesStrip')}>
    {Array.from({length: last - first}, (_, offset) => first + offset).map(index => <PageThumb key={index} controller={controller} authority={authority} reportError={reportError}
      format={format} index={index} count={count} current={index === current} pageLabel={t('next.reader.pageLabel', {page: defaultI18n.number(index + 1)})}
      onSelect={() => {
        const location: ReaderLocation = {format: isComicReadingFormat(format) ? format : 'pdf', page: index, progression: count === 1 ? 0 : index / (count - 1)};
        void controller.navigate({kind: 'location', location}).catch(error => {
          if (authority.isCurrent()) reportError(readerErrorCode(error));
        });
      }}/>)}
  </div>;
}

function PageThumb({controller, authority, reportError, format, index, count, current, pageLabel, large = false, signal, onSelect}: Fixed & Readonly<{
  index: number; count: number; current: boolean; pageLabel: string; large?: boolean; signal?: AbortSignal; onSelect: () => void;
}>) {
  const [preview, setPreview] = useState<ReadPagePreview>();
  useEffect(() => {
    const life = new AbortController();
    let loaded: ReadPagePreview | undefined, alive = true;
    void loadReadPagePreview({controller, index, width: large ? 180 : 96, authority, signal: signal ? AbortSignal.any([life.signal, signal]) : life.signal}).then(value => {
      if (!alive || signal?.aborted) {value.dispose(); return;}
      loaded = value; setPreview(value);
    }).catch(error => {if (alive && authority.isCurrent() && !life.signal.aborted && !signal?.aborted) reportError(readerErrorCode(error));});
    return () => {alive = false; life.abort(); loaded?.dispose();};
    // A preview belongs to one page of one authorized document.
  }, [controller, index, authority, reportError, large, signal]);
  return <button type="button" className={large ? `${styles.page} ${styles.large}` : styles.page} aria-current={current ? 'page' : undefined}
    aria-label={count > 0 ? t('next.reader.pageOf', {page: defaultI18n.number(index + 1), count: defaultI18n.number(count)}) : pageLabel}
    data-current={current || undefined} onClick={onSelect}>
    {preview ? <img src={preview.url} alt="" width={preview.width} height={preview.height} loading="lazy" decoding="async"/> : <span className={styles.blank} aria-hidden="true">{format.toUpperCase()}</span>}
    <span className={styles.number} aria-hidden="true">{defaultI18n.number(index + 1)}</span>
  </button>;
}

/** The actual document page count supplies ordinals; only visible previews mount. */
export function FixedPagesPanel(props: Fixed & Readonly<{onNavigate?: () => void}>) {
  const snapshot = useSyncExternalStore(props.controller.subscribe, props.controller.getSnapshot, props.controller.getSnapshot);
  const count = snapshot.metadata?.pageCount, location = snapshot.location;
  if (props.format === 'epub' || !count || !Number.isSafeInteger(count) || count < 1 || !location || location.format === 'epub'
    || snapshot.status !== 'ready' || !snapshot.sourceId || !snapshot.sourceRevision) return null;
  return <FixedPagesWindow key={JSON.stringify([snapshot.sourceId, snapshot.sourceRevision, count])} {...props}
    count={count} current={location.page} sourceId={snapshot.sourceId} sourceRevision={snapshot.sourceRevision}/>;
}
function FixedPagesWindow({controller, authority, reportError, format, count, current, sourceId, sourceRevision, onNavigate}: Fixed & Readonly<{
  count: number; current: number; sourceId: string; sourceRevision: string; onNavigate?: () => void;
}>) {
  const viewport = useRef<HTMLDivElement>(null);
  const [pageWindow, setPageWindow] = useState<Readonly<{collection: WindowedCollection<number>; life: AbortController}>>();
  const [life, setLife] = useState<AbortController>();
  const lifeRef = useRef<AbortController | undefined>(undefined);
  useLayoutEffect(() => {
    const captured = new AbortController(); lifeRef.current = captured; setLife(captured);
    return () => {captured.abort(); if (lifeRef.current === captured) lifeRef.current = undefined;};
  }, [controller, authority, sourceId, sourceRevision, count]);
  const pages = pageWindow && pageWindow.life === life ? pageWindow.collection : undefined;
  const [height, setHeight] = useState(0);
  const currentSource = () => {
    const snapshot = controller.getSnapshot();
    return authority.isCurrent() && snapshot.status === 'ready' && snapshot.sourceId === sourceId && snapshot.sourceRevision === sourceRevision
      && snapshot.metadata?.pageCount === count;
  };
  useEffect(() => {
    if (!life || lifeRef.current !== life || life.signal.aborted) return;
    const collection = createWindowedCollection<number>({pageSize: 24, maxResidentPages: 4, maxConcurrent: 1, prefetchPages: 0,
    keyOf: index => String(index), async fetchPage(start, size, signal) {
      const snapshot = controller.getSnapshot();
      if (signal.aborted || life.signal.aborted || lifeRef.current !== life || !authority.isCurrent() || snapshot.status !== 'ready' || snapshot.sourceId !== sourceId
        || snapshot.sourceRevision !== sourceRevision || snapshot.metadata?.pageCount !== count) throw new Error('source_changed');
      return {items: Array.from({length: Math.max(0, Math.min(size, count - start))}, (_, offset) => start + offset), total: count};
    },
    });
    setPageWindow({collection, life});
    return () => collection.dispose();
  }, [life, controller, authority, sourceId, sourceRevision, count]);
  useEffect(() => {
    const element = viewport.current; if (!element) return;
    const observer = new ResizeObserver(() => setHeight(element.clientHeight)); observer.observe(element); setHeight(element.clientHeight);
    return () => observer.disconnect();
  }, []);
  const navigate = (index: number) => {
    if (!life || lifeRef.current !== life || life.signal.aborted || !currentSource() || !Number.isSafeInteger(index) || index < 0 || index >= count) return;
    const target: ReaderLocation = {format: isComicReadingFormat(format) ? format : 'pdf', page: index, progression: count === 1 ? 0 : index / (count - 1)};
    void controller.navigate({kind: 'location', location: target}, life.signal).then(() => {if (lifeRef.current === life && !life.signal.aborted && currentSource()) onNavigate?.();}).catch(error => {
      if (lifeRef.current === life && !life.signal.aborted && currentSource()) reportError(readerErrorCode(error));
    });
  };
  return <div className={styles.panel}>
    <p className={styles.panelCount}>{t('next.reader.pageOf', {page: defaultI18n.number(current + 1), count: defaultI18n.number(count)})}</p>
    <div ref={viewport} className={styles.panelWindow}>{height > 0 && pages && <WindowedGrid initialIndex={current} collection={pages}
      label={t('next.reader.pagesStrip')} height={height} cardWidth={104} rowHeight={156} gap={12} rowGap={12} overscanRows={1}
      renderPlaceholder={() => <span className={styles.blank} aria-hidden="true"/>}
      renderItem={index => <PageThumb controller={controller} authority={authority} reportError={reportError} format={format} index={index} count={count}
        current={index === current} large signal={life?.signal} pageLabel={t('next.reader.pageLabel', {page: defaultI18n.number(index + 1)})} onSelect={() => navigate(index)}/>}/>}</div>
  </div>;
}

/**
 * Zoom, fit and page pairing for fixed pages. Each choice is a live engine override in
 * this session, exactly like the display panel: closing the book restores the defaults.
 */
export function FixedPageMenus({controller, authority, reportError, format, open, onOpenChange, anchorRef, directionRef}: Fixed & Readonly<{
  open: 'zoom' | 'direction' | undefined; onOpenChange: (open: 'zoom' | 'direction' | undefined) => void;
  anchorRef: RefObject<HTMLElement | null>; directionRef: RefObject<HTMLElement | null>;
}>) {
  useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const snapshot = controller.getSnapshot(), zoom = snapshot.zoom, preferences = snapshot.preferences;
  const failed = (error: unknown) => {if (authority.isCurrent()) reportError(readerErrorCode(error));};
  const apply = (value: ReaderZoom) => {if (authority.isCurrent()) void controller.setZoom(value).catch(failed);};
  const override = (patch: Partial<ReadingPreferences>) => {if (authority.isCurrent()) void controller.setOverride(patch).catch(failed);};
  const scale = zoom?.scale ?? 1;
  const fits: readonly ReaderFit[] = format === 'pdf' ? ['page', 'width', 'spread'] : ['page', 'width', 'height'];
  return <>
    <Menu open={open === 'zoom'} onOpenChange={value => onOpenChange(value ? 'zoom' : undefined)} title={t('next.reader.zoom')} anchorRef={anchorRef}>
      <MenuItem onSelect={() => apply({kind: 'scale', value: Math.max(0.05, Math.round((scale / 1.25) * 100) / 100)})} disabled={scale <= 0.05}>{t('next.reader.zoomOut')}</MenuItem>
      <MenuItem onSelect={() => apply({kind: 'scale', value: Math.min(8, Math.round(scale * 1.25 * 100) / 100)})} disabled={scale >= 8}>{t('next.reader.zoomIn')}</MenuItem>
      <MenuSeparator/>
      {fits.map(fit => <MenuItem key={fit} selected={zoom?.fit === fit} onSelect={() => apply({kind: 'fit', value: fit})}>{fitLabel(fit)}</MenuItem>)}
      <MenuSeparator/>
      <MenuItem selected={!preferences.twoPages} onSelect={() => override({twoPages: false})}>{t('next.reader.onePage')}</MenuItem>
      <MenuItem selected={preferences.twoPages} onSelect={() => override({twoPages: true})}>{t('next.reader.twoPages')}</MenuItem>
    </Menu>
    {isComicReadingFormat(format) && <Menu open={open === 'direction'} onOpenChange={value => onOpenChange(value ? 'direction' : undefined)} title={t('next.reader.direction')} anchorRef={directionRef}>
      {(['file', 'ltr', 'rtl'] as const).map(direction => <MenuItem key={direction} selected={preferences.comicDirection === direction}
        onSelect={() => override({comicDirection: direction})}>{t(direction === 'file' ? 'next.reader.directionFile' : direction === 'ltr' ? 'next.reader.directionLtr' : 'next.reader.directionRtl')}</MenuItem>)}
    </Menu>}
  </>;
}
