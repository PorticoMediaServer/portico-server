import {useEffect, useSyncExternalStore, useState, type ReactNode} from 'react';
import type {AccountMove} from '@core/account-move.ts';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls/Button';
import type {SettingsOwnerSwitch} from './owner-switch';
import styles from './move-in-progress.module.css';

const t = defaultI18n.t;

const noMove = () => null;
const noSubscribe = () => () => {};

/** One subscription for the real move in flight: the same owner-fenced read the
 * switch rows consult. A refusal or outage leaves `undefined`, never a guess. */
export function SignInMoveWatch({source, children}: {
  source?: SettingsOwnerSwitch; children: (move: AccountMove | undefined) => ReactNode;
}) {
  const move = useSyncExternalStore(source?.subscribeMove ?? noSubscribe, source?.getMove ?? noMove);
  // One read per captured source; refreshMove itself refuses when the surface is gone.
  useEffect(() => { source?.refreshMove(); }, [source]);
  return <>{children(move ?? undefined)}</>;
}

/** Frame 24c: the one move in flight, with the server's own counts and the two
 * real actions. It makes no codes, admits nobody and holds no credential. */
export function MoveInProgress({move, source, isCurrent, onPeople, onError}: {
  move: AccountMove; source: SettingsOwnerSwitch; isCurrent: () => boolean; onPeople: () => void; onError: (error: unknown) => void;
}) {
  const [busy, setBusy] = useState(false);
  const moved = Math.max(0, move.people - move.waiting), percent = move.people > 0 ? Math.round((moved / move.people) * 100) : 0;
  const finish = async () => {
    if (busy || !isCurrent()) return;
    setBusy(true);
    try { await source.finishMove(); }
    catch (error) { if (isCurrent()) onError(error); }
    finally { if (isCurrent()) setBusy(false); }
  };
  return <section className={styles.panel} aria-live="polite">
    <h3>{t(move.to === 'local' ? 'accounts.settings.movingToLocal' : 'accounts.settings.movingToPortico')}</h3>
    <p className={styles.count}>{t('accounts.settings.waiting', {waiting: move.waiting, total: move.people})}</p>
    <div className={styles.bar} role="progressbar" aria-valuemin={0} aria-valuemax={move.people} aria-valuenow={moved}
      aria-label={t('accounts.settings.moved', {moved, total: move.people})}><span style={{width: `${percent}%`}} /></div>
    <p className={styles.help}>{t('accounts.settings.finishNowHelp')}</p>
    <div className={styles.actions}>
      <Button size="small" variant="glass" onClick={onPeople}>{t('accounts.settings.openPeople')}</Button>
      <Button size="small" variant="primary" loading={busy} disabled={busy} onClick={() => void finish()}>{t('accounts.settings.finishNow')}</Button>
    </div>
  </section>;
}
