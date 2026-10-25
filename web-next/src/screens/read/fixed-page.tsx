import {useEffect, useState, useSyncExternalStore, type RefObject} from 'react';
import {defaultI18n} from '@i18n';
import type {ReadingPreferences} from '@portico/client-core';
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
        const location: ReaderLocation = {format: format === 'pdf' ? 'pdf' : 'cbz', page: index, progression: count === 1 ? 0 : index / (count - 1)};
        void controller.navigate({kind: 'location', location}).catch(error => {
          if (authority.isCurrent()) reportError(readerErrorCode(error));
        });
      }}/>)}
  </div>;
}

function PageThumb({controller, authority, reportError, format, index, count, current, pageLabel, large = false, onSelect}: Fixed & Readonly<{
  index: number; count: number; current: boolean; pageLabel: string; large?: boolean; onSelect: () => void;
}>) {
  const [preview, setPreview] = useState<ReadPagePreview>();
  useEffect(() => {
    const life = new AbortController();
    let loaded: ReadPagePreview | undefined, alive = true;
    void loadReadPagePreview({controller, index, width: large ? 180 : 96, authority, signal: life.signal}).then(value => {
      if (!alive) {value.dispose(); return;}
      loaded = value; setPreview(value);
    }).catch(error => {if (alive && authority.isCurrent() && !life.signal.aborted) reportError(readerErrorCode(error));});
    return () => {alive = false; life.abort(); loaded?.dispose();};
    // A preview belongs to one page of one authorized document.
  }, [controller, index, authority, reportError, large]);
  return <button type="button" className={large ? `${styles.page} ${styles.large}` : styles.page} aria-current={current ? 'page' : undefined}
    aria-label={count > 0 ? t('next.reader.pageOf', {page: defaultI18n.number(index + 1), count: defaultI18n.number(count)}) : pageLabel}
    data-current={current || undefined} onClick={onSelect}>
    {preview ? <img src={preview.url} alt="" width={preview.width} height={preview.height} loading="lazy" decoding="async"/> : <span className={styles.blank} aria-hidden="true">{format.toUpperCase()}</span>}
    <span className={styles.number} aria-hidden="true">{defaultI18n.number(index + 1)}</span>
  </button>;
}

/** The same bounded page window as a panel, for fixed pages whose only contents are pages. */
export function FixedPagesPanel({controller, authority, reportError, format}: Fixed) {
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const count = snapshot.metadata?.pageCount, location = snapshot.location;
  const current = location && location.format !== 'epub' ? location.page : undefined;
  if (format === 'epub' || !count || count < 1 || current === undefined || snapshot.status !== 'ready') return null;
  const first = Math.max(0, Math.min(current - 6, count - 13)), last = Math.min(count, first + 13);
  const navigate = (index: number) => {
    const target: ReaderLocation = {format: format === 'pdf' ? 'pdf' : 'cbz', page: index, progression: count === 1 ? 0 : index / (count - 1)};
    void controller.navigate({kind: 'location', location: target}).catch(error => {
      if (authority.isCurrent()) reportError(readerErrorCode(error));
    });
  };
  return <div className={styles.panel}>
    <p className={styles.panelCount}>{t('next.reader.pageOf', {page: defaultI18n.number(current + 1), count: defaultI18n.number(count)})}</p>
    <div className={styles.panelGrid}>{Array.from({length: last - first}, (_, offset) => first + offset).map(index =>
      <PageThumb key={index} controller={controller} authority={authority} reportError={reportError} format={format} index={index} count={count}
        current={index === current} large pageLabel={t('next.reader.pageLabel', {page: defaultI18n.number(index + 1)})} onSelect={() => navigate(index)}/>)}</div>
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
    {format === 'cbz' && <Menu open={open === 'direction'} onOpenChange={value => onOpenChange(value ? 'direction' : undefined)} title={t('next.reader.direction')} anchorRef={directionRef}>
      {(['file', 'ltr', 'rtl'] as const).map(direction => <MenuItem key={direction} selected={preferences.comicDirection === direction}
        onSelect={() => override({comicDirection: direction})}>{t(direction === 'file' ? 'next.reader.directionFile' : direction === 'ltr' ? 'next.reader.directionLtr' : 'next.reader.directionRtl')}</MenuItem>)}
    </Menu>}
  </>;
}
