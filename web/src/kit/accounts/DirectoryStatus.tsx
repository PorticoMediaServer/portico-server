import type {ReactNode} from 'react';
import {createI18n, deviceRegion, resolveRegion} from '@i18n';
import {Icon, type IconName} from '../Icon';
import {Button} from '../controls';
import {ColourBackdrop} from '../artwork';
import {useSizeClass} from '../theme';
import styles from './directory-status.module.css';

const t = createI18n(resolveRegion(undefined, deviceRegion())).t;

/** connect 2: the six connection states between the directory and Home, on the
 * same state layout. Presentation only — the caller owns the phase, the retry
 * clock and every code that decides it. */
export type DirectoryStatusPhase = 'opening' | 'waiting' | 'unreachable' | 'account-waiting' | 'identity' | 'clock';

export function ServicesDirectoryStatus({header, phase, server, lastSeen, seconds, owner = false, onCheck, onChoose, onAccountHelp, busy = false, error}: {
  header: ReactNode;
  busy?: boolean;
  error?: ReactNode;
  phase: DirectoryStatusPhase;
  /** The server's own captured name. */
  server: string;
  /** The already formatted last-online time; absent means it was never seen. */
  lastSeen?: string;
  /** The caller's real retry countdown, in seconds. */
  seconds: number;
  owner?: boolean;
  onCheck: () => void;
  onChoose: () => void;
  /** The owner's way in while Portico Accounts are away (connect 2d). */
  onAccountHelp: () => void;
}) {
  const opening = phase === 'opening';
  const spinning = opening;
  const icon: IconName | undefined = phase === 'waiting' || phase === 'unreachable' ? 'i-srvoff'
    : phase === 'account-waiting' ? 'i-cloudoff' : phase === 'identity' ? 'i-shield' : phase === 'clock' ? 'i-clock' : undefined;
  const title = phase === 'opening' ? t('web.chooser.connecting', {server})
    : phase === 'waiting' ? t('web.chooser.waitingTitle', {server})
      : phase === 'unreachable' ? t('web.chooser.unreachableTitle', {server})
        : phase === 'account-waiting' ? t('web.chooser.accountWaitingTitle')
          : phase === 'identity' ? t('web.chooser.unconfirmedTitle', {server})
            : t('web.chooser.clockTitle', {server});
  const body = phase === 'opening' ? t('web.chooser.openingBody')
    : phase === 'waiting' ? t('web.chooser.waitingBody')
      : phase === 'unreachable' ? (lastSeen === undefined ? t('web.chooser.unreachableBodyNever') : t('web.chooser.unreachableBody', {date: lastSeen}))
        : phase === 'account-waiting' ? t('web.chooser.accountWaitingBody', {server})
          : phase === 'identity' ? t('web.chooser.unconfirmedBody')
            : t('web.chooser.clockBody');
  const counting = phase === 'waiting' || phase === 'account-waiting';
  // H11 / connect 2: the connection states sit on the round colour backdrop (top on phones), never a flat page.
  const compact = useSizeClass() === 'compact';
  return <div className={styles.page}>
    {header}<ColourBackdrop viewport shape={compact ? 'top' : 'round'} />
    <main className={styles.main}>
      <section className={styles.state}>
        <span className={styles.icon} data-tone={opening ? 'accent' : 'warning'} aria-hidden="true">
          {spinning ? <span className={styles.spinner} /> : icon && <Icon name={icon} size={28} />}
        </span>
        <h1>{title}</h1>
        <p className={styles.body}>{body}</p>
        {counting && <div className={styles.alive}>
          <svg className={styles.ring} viewBox="0 0 24 24" aria-hidden="true"><circle className={styles.ringTrack} cx="12" cy="12" r="9" /><circle className={styles.ringArc} cx="12" cy="12" r="9" /></svg>
          {t('web.chooser.checkingAgain', {seconds})}
        </div>}
        {error}
        <div className={styles.actions}>
          {!opening && <Button variant="primary" disabled={busy} onClick={onCheck}>{t(counting ? 'web.chooser.checkNow' : 'web.chooser.tryAgain')}</Button>}
          {opening ? <button type="button" className={styles.quiet} onClick={onChoose}>{t('web.chooser.otherServer')}</button>
            : phase === 'account-waiting' ? <button type="button" className={styles.quiet} disabled={busy} onClick={onAccountHelp}>{t('accounts.signIn.cannotUsePortico')}</button>
              : <Button onClick={onChoose}>{t('web.chooser.otherServer')}</Button>}
        </div>
        {phase === 'unreachable' && owner && <p className={styles.help}>{t('web.hostedAccount.offlineOwnerHint')}</p>}
      </section>
    </main>
  </div>;
}
