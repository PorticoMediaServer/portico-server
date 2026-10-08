import {useNavigate} from '@tanstack/react-router';
import {useQuery, useServices} from '../data/hooks';
import {useTogether} from '../app/together';
import {player} from '../app/player';
import {Icon} from '../ui/Icon';
import {Popover} from '../ui/Overlay';
import {Avatar} from '../ui/Media';

/** Watching with a group: who's here and in step, and a way back to the group. */
export function GroupPill() {
  const services = useServices();
  const navigate = useNavigate();
  const {groupId} = useTogether();
  const group = useQuery(groupId ? `together.${groupId}` : null, () => services.together.group(groupId!)).data;
  if (!group) return null;
  const behind = group.members.filter(m => m.status !== 'inStep').length;
  return (
    <Popover side="bottom" align="end" trigger={
      <button type="button" className="group-pill" aria-label={`Watching with ${group.name}`}>
        <span className="group-faces">{group.members.slice(0, 3).map(m => <Avatar key={m.id} name={m.name} color={m.color} size={22} />)}</span>
        <span>{group.name}</span>
        {behind ? <span className="group-wait">{behind} catching up</span> : <Icon name="check" size={14} />}
      </button>
    }>
      <div className="stack" style={{gap: 10}}>
        <strong>{group.name}</strong>
        <ul className="group-members">{group.members.map(m => <li key={m.id}><Avatar name={m.name} color={m.color} size={26} /><span>{m.name}{m.you ? ' (you)' : ''}{m.host ? ' · Host' : ''}</span><span className="faint">{m.status === 'inStep' ? 'In step' : m.status === 'buffering' ? 'Loading' : m.status === 'catchingUp' ? 'Catching up' : 'Away'}</span></li>)}</ul>
        <button type="button" className="btn btn-secondary btn-sm" onClick={() => { player.minimize(); void navigate({to: '/together', search: {group: group.id}}); }}><span className="btn-label">Open the group</span></button>
      </div>
    </Popover>
  );
}
