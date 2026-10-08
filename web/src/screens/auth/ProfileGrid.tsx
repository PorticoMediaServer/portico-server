import type {SelectionProfile} from '@core/session-selection.ts';
import {currentI18n} from '../../app/i18n';
import {Icon} from '../../ui';
import s from './Auth.module.css';

/** The profile group centers as a whole, each card stays centered, and rows wrap as the set grows. */
export function ProfileGrid({items, busy, onPick}: {items: readonly SelectionProfile[]; busy: boolean; onPick: (p: SelectionProfile) => void}) {
  return (
    <div className={s.profiles}>
      {items.map(p => (
        <button key={p.id} type="button" className={s.profile} disabled={!p.eligible || busy} onClick={() => onPick(p)}>
          <span className={s.profileArt} style={{background: `var(--profile-art-${p.art ?? 'blue'})`}}>{p.name.slice(0, 1).toUpperCase()}</span>
          <span className={s.profileName}>{p.name}</span>
          <span className={s.profileNote}>{!p.eligible ? (p.unavailableReason === 'membership_required' ? 'Not shared' : p.unavailableReason === 'membership_revoked' ? 'Access removed' : 'Restricted') : p.pinRequired ? <><Icon name="lock" size={11} /> {currentI18n().t('pin.entry')}</> : p.primary ? 'Primary' : ''}</span>
        </button>
      ))}
    </div>
  );
}
