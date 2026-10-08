import React from 'react';
import {View} from 'react-native';
import {useQuery, useServices} from '../../data/hooks';
import {Focusable} from '../../design/Focusable';
import {Icon} from '../../design/Icon';
import {ui} from '../../design/overlays';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {plural} from '../../design/format';

/** Watch Together pill: who's watching and whether everyone is in step. */
export function GroupPill() {
  const s = useServices();
  const q = useQuery('together:groups', () => s.together.groups());
  const g = (q.data ?? []).find(x => x.state !== 'ended' && x.state !== 'lobby') ?? (q.data ?? [])[0];
  if (!g) return null;
  const waiting = g.members.find(m => m.status !== 'inStep');
  const status = g.state === 'hostReconnecting' ? 'Host reconnecting' : waiting ? `Waiting for ${waiting.name}…` : 'In step';
  const open = () =>
    ui.sheet({
      title: g.name,
      subtitle: `${plural(g.members.length, 'person', 'people')} watching · ${g.control === 'host' ? 'The host controls playback' : 'Everyone controls playback'}`,
      render: () => (
        <View>
          {g.members.map(m => (
            <View key={m.id} style={{flexDirection: 'row', alignItems: 'center', paddingVertical: u(8)}}>
              <View style={{width: u(30), height: u(30), borderRadius: u(15), backgroundColor: m.color, alignItems: 'center', justifyContent: 'center'}}><Text variant="captionStrong" tone="ink">{m.name[0]}</Text></View>
              <Text variant="bodyStrong" style={{marginLeft: u(10), flex: 1}}>{m.name}{m.you ? ' (you)' : ''}{m.host ? ' · Host' : ''}</Text>
              <Text variant="caption" color={m.status === 'inStep' ? colors.healthy : colors.amber}>{{inStep: 'In step', buffering: 'Buffering', catchingUp: 'Catching up', away: 'Away'}[m.status]}</Text>
            </View>
          ))}
        </View>
      ),
    });
  return (
    <Focusable onPress={open} scale={1.06} accessibilityLabel={`${g.name}: ${plural(g.members.length, 'person', 'people')} watching, ${status}. Show who’s here`} style={{marginRight: u(8)}}>
      {({focused}) => (
        <View style={{flexDirection: 'row', alignItems: 'center', height: u(34), paddingHorizontal: u(12), borderRadius: radius.pill, backgroundColor: focused && isTV ? colors.white : 'rgba(7,16,24,0.6)', borderWidth: 1, borderColor: waiting ? colors.amber : colors.healthy}}>
          <Icon name="together" size={14} color={focused && isTV ? colors.actionInk : colors.text} />
          <Text variant="captionStrong" color={focused && isTV ? colors.actionInk : colors.text} style={{marginLeft: 6}}>{g.members.length}</Text>
          <Text variant="caption" color={focused && isTV ? colors.actionInk : waiting ? colors.amber : colors.healthy} style={{marginLeft: 6}}>{status}</Text>
        </View>
      )}
    </Focusable>
  );
}
