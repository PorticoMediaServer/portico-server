import type {SelectionProfile} from '@core/session-selection.ts';
import {Icon} from '../../ui';
import s from './Auth.module.css';
import {useI18n} from '../../app/i18n';

/** The profile group centers as a whole, each card stays centered, and rows wrap as the set grows. */
export function ProfileGrid({items, busy, onPick}: {items: readonly SelectionProfile[]; busy: boolean; onPick: (p: SelectionProfile) => void}) {
  const {t} = useI18n();
  return (
    <div className={s.profiles}>
      {items.map(p => (
        <button key={p.id} type="button" className={s.profile} disabled={!p.eligible || busy} onClick={() => onPick(p)}>
          <span className={s.profileArt} style={{background: `var(--profile-art-${p.art ?? 'blue'})`}}>{p.name.slice(0, 1).toUpperCase()}</span>
          <span className={s.profileName}>{p.name}</span>
          <span className={s.profileNote}>{!p.eligible ? (p.unavailableReason === 'membership_required' ? t('web.profilePicker.notShared') : p.unavailableReason === 'membership_revoked' ? t('web.profilePicker.accessRemoved') : t('web.profilePicker.restricted')) : p.pinRequired ? <><Icon name="lock" size={11} /> {t('pin.entry')}</> : p.primary ? t('profile.main') : ''}</span>
        </button>
      ))}
    </div>
  );
}
