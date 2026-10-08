import React, {memo, useSyncExternalStore} from 'react';
import type {Group, GroupMember} from '@core/social-playback.ts';
import {useTogether} from '../app/together';
import {useI18n} from '../app/i18n';
import {Popover, Text} from '../ui';
import s from './GroupPill.module.css';

const noSubscribe = () => () => {};

/** FEAT-09: whether this viewer is in a room where only the host may play and pause. */
export function useHostControlsPlayback(): boolean {
  const {service} = useTogether();
  return useSyncExternalStore(service?.subscribe ?? noSubscribe, () => {
    const snap = service?.getSnapshot();
    return !!snap?.group && (snap.phase === 'live' || snap.phase === 'reconnecting') && !snap.group.permissions.canControl;
  });
}

type PillState = {tone: 'ok' | 'wait' | 'warn'; label: string};
function describe(group: Group, reconnecting: boolean, t: ReturnType<typeof useI18n>['t']): PillState {
  if (reconnecting) return {tone: 'warn', label: t('web.together.pill.reconnecting')};
  if (group.host.presence !== 'connected' && !group.permissions.isHost) return {tone: 'warn', label: t('web.together.pill.hostAway')};
  const waiting = joined(group).find(m => m.id !== group.viewerMemberId && m.readiness === 'buffering');
  if (waiting) return {tone: 'wait', label: t('web.together.pill.waitingFor', {name: waiting.displayName})};
  return {tone: 'ok', label: t('web.together.pill.inStep')};
}
const joined = (group: Group): GroupMember[] => group.members.filter(m => m.state === 'joined');
const initial = (name: string) => (name.trim()[0] ?? '?').toUpperCase();

/**
 * FEAT-09: in a Watch Together room the player shows the group: who is here, and whether
 * everyone is in step or someone is catching up. It subscribes on its own, so group changes
 * re-render only the pill.
 */
export const GroupPill = memo(function GroupPill() {
  const {state} = useTogether();
  const i18n = useI18n();
  const group = state.group;
  if (!group || (state.phase !== 'live' && state.phase !== 'reconnecting')) return null;
  const members = joined(group);
  const status = describe(group, state.phase === 'reconnecting', i18n.t);
  const shown = members.slice(0, 3);
  const trigger = (
    <button type="button" className={s.pill} aria-label={i18n.t('web.together.pill.label', {name: group.name, count: members.length, status: status.label})}>
      <span className={s.faces} aria-hidden>{shown.map(m => <span key={m.id} className={s.face}>{initial(m.displayName)}</span>)}{members.length > shown.length ? <span className={s.face}>+{members.length - shown.length}</span> : null}</span>
      <span className={s.copy}>
        <span className={s.name}>{group.name}</span>
        <span className={s[status.tone]}>{status.label}</span>
      </span>
    </button>
  );
  return (
    <Popover trigger={trigger} align="start">
      <div className={s.panel}>
        <Text as="p" variant="label" tone="secondary">{i18n.t('web.together.pill.people', {count: members.length})}</Text>
        <ul className={s.list}>
          {members.map(m => (
            <li key={m.id} className={s.member}>
              <span className={s.face} aria-hidden>{initial(m.displayName)}</span>
              <span className={s.memberName}>{m.displayName}{m.id === group.viewerMemberId ? ` ${i18n.t('web.together.pill.you')}` : ''}</span>
              <Text as="span" variant="caption" tone="tertiary">{m.role === 'host' ? i18n.t('web.together.pill.host') : m.readiness === 'buffering' ? i18n.t('web.together.pill.catchingUp') : m.presence !== 'connected' ? i18n.t('web.together.pill.away') : ''}</Text>
            </li>
          ))}
        </ul>
        {!group.permissions.canControl ? <Text as="p" variant="caption" tone="tertiary">{i18n.t('web.together.pill.hostControls')}</Text> : null}
      </div>
    </Popover>
  );
});
