import {useEffect, useRef, useState} from 'react';
import type {AccountMoveCode} from '@core/account-move.ts';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls/Button';
import {Dialog} from '../../kit/overlays/Dialog';
import styles from './move-codes-sheet.module.css';

const t = defaultI18n.t;
/** The parent owns the one-time receipt and clears it on close/authority retirement.
 * This leaf never makes or recovers codes and puts no secret in a URL or storage. */
export function MoveCodesSheet({codes, serverName, address, isCurrent, onClose, onError}: {
  codes: readonly AccountMoveCode[]; serverName: string; address: string;
  isCurrent: () => boolean; onClose: () => void; onError: (error: unknown) => void;
}) {
  const [qr, setQR] = useState<{address: string; path: string; size: number}>();
  const alive = useRef(true), copying = useRef(false);
  useEffect(() => {alive.current = true; return () => {alive.current = false;};}, []);
  useEffect(() => {
    let active = true;
    // Only the public captured server address is encoded, never a person's code.
    void import('qrcode').then(encoder => {
      if (!active || !isCurrent()) return;
      const value = encoder.create(address, {errorCorrectionLevel: 'M'}), cells = value.modules;
      let path = '';
      for (let y = 0; y < cells.size; y++) for (let x = 0; x < cells.size; x++) if (cells.get(y, x)) path += `M${x} ${y}h1v1h-1z`;
      setQR({address, path, size: cells.size});
    }).catch(() => {if (active && isCurrent()) onError(new Error('move_codes_qr_failed'));});
    return () => {active = false;};
  }, [address, isCurrent, onError]);
  if (!isCurrent()) return null;
  const copy = async () => {
    if (!alive.current || !isCurrent() || copying.current) return;
    copying.current = true;
    try {
      await navigator.clipboard.writeText([serverName, address, ...codes.map(row => `${row.username}\t${row.code}\t${defaultI18n.dateTime(row.expiresAt)}`)].join('\n'));
    } catch {if (alive.current && isCurrent()) onError(new Error('move_codes_copy_failed'));}
    finally {copying.current = false;}
  };
  return <Dialog open title={t('accounts.move.sheetTitle')} width={760} className={styles.sheet}
    onOpenChange={value => {if (!value) onClose();}} footer={<>
      <Button onClick={() => {void copy();}}>{t('accounts.move.copyList')}</Button>
      <Button onClick={() => {if (alive.current && isCurrent()) window.print();}}>{t('accounts.move.print')}</Button>
      <Button variant="primary" onClick={onClose}>{t('accounts.move.done')}</Button>
    </>}>
    <article className={styles.paper}>
      <h2>{serverName}</h2><h3>{t('accounts.move.sheetHeading')}</h3>
      <p>{t('accounts.move.sheetHelp', {address})}</p>
      {qr?.address === address && <svg viewBox={`-4 -4 ${qr.size + 8} ${qr.size + 8}`} role="img" aria-label={t('qr.label')} shapeRendering="crispEdges"><path d={qr.path}/></svg>}
      <table><thead><tr><th>{t('accounts.move.sheetUsername')}</th><th>{t('accounts.move.code')}</th><th>{t('accounts.move.validUntil')}</th></tr></thead>
        <tbody>{codes.map(row => <tr key={row.accountId}><td>{row.username}</td><td className={styles.code}>{row.code}</td><td>{defaultI18n.dateTime(row.expiresAt)}</td></tr>)}</tbody></table>
    </article>
  </Dialog>;
}
