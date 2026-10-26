import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls';

const t = defaultI18n.t;

function clock(remainingSeconds: number) {
  const whole = Math.max(0, Math.floor(remainingSeconds));
  const minutes = Math.floor(whole / 60), seconds = whole % 60;
  return `${minutes}:${String(seconds).padStart(2, '0')}`;
}

/**
 * auth20f: the one action on the expired and rate-limited reset-code states.
 * The owner supplies the real deadline through remainingSeconds and keeps the
 * countdown, the request and the saved code; this button only renders the drawn
 * label, the tabular figures the frame gives it, and stays disabled while busy or
 * while the countdown still runs.
 */
export function OwnerResetRewriteButton({remainingSeconds, busy, primary = false, onRewrite}: {
  remainingSeconds: number;
  busy: boolean;
  primary?: boolean;
  onRewrite: () => void;
}) {
  const waiting = Number.isFinite(remainingSeconds) && remainingSeconds > 0;
  return <Button variant={primary ? 'primary' : 'ghost'} size="small" disabled={busy || waiting} onClick={onRewrite}
    style={{fontVariantNumeric: 'tabular-nums'}}>{waiting ? t('accounts.recovery.writeNewIn', {time: clock(remainingSeconds)}) : t('accounts.recovery.rewrite')}</Button>;
}
