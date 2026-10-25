import {useEffect, useLayoutEffect, useRef, useSyncExternalStore, useState, type ReactNode} from 'react';
import type {AccountMove} from '@core/account-move.ts';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls/Button';
import type {SettingsOwnerSwitch} from './owner-switch';
import styles from './move-in-progress.module.css';

const t = defaultI18n.t;

/** One committed panel scope: the exact source, move revision and lifetime ticket
 * the buttons were drawn with. A replacement retires it before layout cleanup. */
type Scope = {readonly source: SettingsOwnerSwitch; readonly move: AccountMove; readonly controller: AbortController; retired: boolean};

const retire = (scope: Scope) => {if (!scope.retired) {scope.retired = true; scope.controller.abort();}};

/** One subscription for the real move in flight: the same owner-fenced read the
 * switch rows consult. A refusal or outage leaves `undefined`, never a guess. */
export function SignInMoveWatch({source, children}: {
  source?: SettingsOwnerSwitch; children: (move: AccountMove | undefined) => ReactNode;
}) {
  const move = useSyncExternalStore(source?.subscribeMove ?? (() => () => {}), source?.getMove ?? (() => null));
  // One read per captured context; refreshMove itself refuses when the surface is gone.
  const read = useRef('');
  useEffect(() => { if (source && read.current !== source.contextId) { read.current = source.contextId; source.refreshMove(); } }, [source]);
  return <>{children(move ?? undefined)}</>;
}

/** Frame 24c: the one move in flight, with the server's own counts and the two
 * real actions. It makes no codes, admits nobody and holds no credential. */
export function MoveInProgress({move, source, isCurrent, onPeople, onError}: {
  move: AccountMove; source: SettingsOwnerSwitch; isCurrent: () => boolean; onPeople: () => void; onError: (error: unknown) => void;
}) {
  const [busy, setBusy] = useState(false);
  const scopeRef = useRef<Scope | undefined>(undefined);
  // Install this render's scope, retiring a replaced one immediately.
  let scope = scopeRef.current;
  if (scope && (scope.move.id !== move.id || scope.move.revision !== move.revision || scope.source.contextId !== source.contextId)) {
    retire(scope); scope = undefined; scopeRef.current = undefined;
  }
  if (!scope) {scope = {source, move, controller: new AbortController(), retired: false}; scopeRef.current = scope;}
  const current = scope;
  const live = (target: Scope) => target === scopeRef.current && !target.retired && !target.controller.signal.aborted && isCurrent();
  // Exactly this scope's ticket dies with it; no disposed controller is reused.
  useLayoutEffect(() => {setBusy(false); return () => retire(current);}, [current]);
  const moved = move.people - move.waiting, percent = move.people > 0 ? Math.round((Math.max(0, moved) / move.people) * 100) : 0;
  const finish = async (target: Scope) => {
    if (busy || !live(target)) return;
    setBusy(true);
    try { await target.source.finishMove(target.move, {isCurrent, signal: target.controller.signal}); }
    catch (error) { if (live(target)) onError(error); }
    finally { if (target === scopeRef.current) setBusy(false); }
  };
  return <section className={styles.panel} aria-live="polite">
    <h3>{t(move.to === 'local' ? 'accounts.settings.movingToLocal' : 'accounts.settings.movingToPortico')}</h3>
    <p className={styles.count}>{t('accounts.settings.waiting', {waiting: move.waiting, total: move.people})}</p>
    <div className={styles.bar} role="progressbar" aria-valuemin={0} aria-valuemax={move.people} aria-valuenow={Math.max(0, moved)}
      aria-label={t('accounts.settings.moved', {moved: Math.max(0, moved), total: move.people})}><span style={{width: `${percent}%`}} /></div>
    <p className={styles.help}>{t('accounts.settings.finishNowHelp')}</p>
    <div className={styles.actions}>
      <Button size="small" variant="glass" onClick={onPeople}>{t('accounts.settings.openPeople')}</Button>
      <Button size="small" variant="primary" loading={busy} disabled={busy} onClick={() => void finish(current)}>{t('accounts.settings.finishNow')}</Button>
    </div>
  </section>;
}
