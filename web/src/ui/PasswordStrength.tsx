import {passwordStrength} from '@core/password-strength.ts';
import {cx} from './cx';
import {currentI18n} from '../app/i18n';
import s from './PasswordStrength.module.css';

/**
 * Advisory strength for a new password: four segments, a label and one hint.
 * It never blocks. The only rule is 8 characters (`passwordStrength().acceptable`),
 * which the form checks itself.
 */
export function PasswordStrength({password, id}: {password: string; id?: string}) {
  if (!password) return <p id={id} className={s.hint}>{currentI18n().t('web.password.hint')}</p>;
  const r = passwordStrength(password);
  const tone = r.level <= 1 ? s.weak : r.level === 2 ? s.fair : s.good;
  return (
    <div id={id} className={s.root} aria-live="polite">
      <div className={s.bars} aria-hidden>
        {[1, 2, 3, 4].map(n => <span key={n} className={cx(s.bar, n <= r.level && tone)} />)}
      </div>
      <p className={s.hint}><span className={s.label}>{r.label}</span>{r.hint ? ` · ${r.hint}` : ''}</p>
    </div>
  );
}
