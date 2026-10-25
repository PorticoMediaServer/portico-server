import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls';
import {Icon} from '../../kit/Icon';
import styles from './reader-hint.module.css';

const {t} = defaultI18n;

/** phone-read 14: the one-time explanation of the page's own tap zones.
 * Only gestures the reader actually implements are named; swipe has no engine
 * signal, so it is not promised here. */
export function ReaderFirstRunHint({onDone}: {onDone: () => void}) {
  return <div className={styles.hint} role="dialog" aria-modal="true" aria-label={t('next.reader.hintTitle')}>
    <div className={styles.zones}>
      <div className={styles.zone}><Icon name="i-left" size={26}/><span>{t('next.reader.previous')}</span><small>{t('next.reader.hintLeftThird')}</small></div>
      <div className={`${styles.zone} ${styles.center}`}><Icon name="i-tap" size={24}/><span>{t('next.reader.hintShowControls')}</span><small>{t('next.reader.hintMiddle')}</small></div>
      <div className={styles.zone}><Icon name="i-right" size={26}/><span>{t('next.reader.next')}</span><small>{t('next.reader.hintRightThird')}</small></div>
    </div>
    <div className={styles.foot}>
      <div className={styles.instruction}><Icon name="i-hold" size={22}/><span>{t('next.reader.hintSelect')}</span></div>
      <Button variant="primary" onClick={onDone}>{t('next.reader.hintGotIt')}</Button>
    </div>
  </div>;
}
