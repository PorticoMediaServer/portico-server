import type {RefObject} from 'react';
import {defaultI18n} from '@i18n';
import {Icon} from '../Icon';
import {BottomSheet, Popover} from '../overlays';
import type {PlayOnController} from '../../platform/playback/play-on';
import type {CastPlayerTranslate, PlayOnPanelProps} from './types';
import styles from './cast-player.module.css';

export function PlayOnButton({controller, available, buttonRef, onOpen, t = defaultI18n.t}: Readonly<{controller: PlayOnController; available: boolean; buttonRef?: RefObject<HTMLButtonElement | null>; onOpen?: () => void; t?: CastPlayerTranslate}>) {
  if (!available) return null;
  return <button type="button" ref={buttonRef} className={styles.playOnButton} aria-label={t('next.dock.playOnDevice')} aria-haspopup="menu"
    onClick={() => { onOpen?.(); controller.openPlayOn(); }}><Icon name="i-cast" size={24} /></button>;
}

/** Desktop f1s and the phone sheet, with exactly the binding browser rows and footnote. */
export function PlayOnPanel({controller, snapshot, phone, anchorRef, reportError, t = defaultI18n.t}: PlayOnPanelProps) {
  const open = snapshot.panel === 'play-on' && !snapshot.disposed;
  if (!snapshot.cast.available && !snapshot.airPlayAvailable) return null;
  const close = () => controller.closePanel();
  const rows = <>
    <div className={styles.deviceGroup}>
      {snapshot.airPlayAvailable && <button type="button" role={phone ? undefined : 'menuitem'} className={styles.deviceRow} onClick={() => controller.startAirPlay()}>
        <Icon name="i-airplay" size={20} /><span>{t('playOn.airplay')}<small>{t('playOn.airplaySubtitle')}</small></span></button>}
      {snapshot.cast.available && <button type="button" role={phone ? undefined : 'menuitem'} className={styles.deviceRow} onClick={() => { void controller.startCast().catch(reportError); }}>
        <Icon name="i-cast" size={20} /><span>{t('playOn.googleCast')}<small>{t('next.cast.browserDevices')}</small></span></button>}
    </div>
    <div className={styles.separator} role="separator" /><p className={styles.footnote}>{t('next.cast.browserFootnote')}</p>
  </>;
  if (phone) return <BottomSheet open={open} onOpenChange={close} title={t('next.dock.playOnDevice')} fitContent returnFocusRef={anchorRef} showCloseButton={false}
    className={styles.playOnSheet} bodyClassName={styles.sheetBody}>
    <button type="button" className={styles.done} onClick={close}>{t('next.controls.done')}</button>{rows}
  </BottomSheet>;
  return <Popover open={open} onOpenChange={close} title={t('next.dock.playOnDevice')} anchorRef={anchorRef} returnFocusRef={anchorRef} width={300}
    role="menu" className={styles.playOnPanel} sideOffset={12} onContentKeyDown={event => {
      if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
      const rows = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')];
      const at = rows.indexOf(document.activeElement as HTMLButtonElement);
      const target = event.key === 'Home' ? 0 : event.key === 'End' ? rows.length - 1 : (at + (event.key === 'ArrowUp' ? -1 : 1) + rows.length) % rows.length;
      event.preventDefault(); rows[target]?.focus();
    }}>
    <div className={styles.menuHeading}>{t('next.dock.playOnDevice')}</div>{rows}
  </Popover>;
}
