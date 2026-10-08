import React, {useState} from 'react';
import {View} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {useClient, useTopic} from '../data/DataProvider';
import {Focusable} from '../ui/Focusable';
import {Text} from '../ui/Text';
import {Avatar, ListRow, Badge} from '../ui/Controls';
import {Sheet} from '../ui/Sheet';
import {Button} from '../ui/Button';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {push} from '../nav/nav';
import {PHONE_HEADER} from '../ui/Page';

/** Watch Together pill: who's here and whether everyone is in step. */
export function GroupPill({inline}: {inline?: boolean}) {
  const client = useClient();
  useTopic(['together']);
  const m = useMetrics();
  const insets = useSafeAreaInsets();
  const [open, setOpen] = useState(false);
  const g = client.together.current();
  if (!g) return null;
  const waiting = g.members.find(x => x.status !== 'inStep');
  const host = g.members.find(x => x.host);
  const pill = (
    <Focusable onPress={() => setOpen(true)} accessibilityLabel={`${g.name}: ${g.members.length} people watching. Show who’s here`}
      style={({focused}) => ({flexDirection: 'row', alignItems: 'center', gap: 8, paddingLeft: 6, paddingRight: 14, height: m.controlSmall, borderRadius: 999, backgroundColor: focused ? colors.focus : colors.overlay, borderWidth: 1, borderColor: colors.line})}>
      {({focused}) => (
        <>
          <View style={{flexDirection: 'row'}}>
            {g.members.slice(0, 3).map((mem, i) => <View key={mem.id} style={{marginLeft: i ? -10 : 0}}><Avatar name={mem.name} art={mem.art} size={m.controlSmall - 12} /></View>)}
          </View>
          <Text variant="captionStrong" color={focused ? colors.accentInk : colors.text}>{waiting ? `Waiting for ${waiting.name}…` : 'In step'}</Text>
        </>
      )}
    </Focusable>
  );
  return (
    <>
      {inline ? pill : m.isTV ? null : <View style={{position: 'absolute', right: 14, top: m.isPhone ? insets.top + PHONE_HEADER + 6 : Math.max(insets.top, 16) + 4}}>{pill}</View>}
      <Sheet visible={open} onClose={() => setOpen(false)} title={g.name} subtitle={`${g.members.length} people watching · ${g.control === 'host-only' ? 'The host controls playback' : 'Everyone controls playback'}`} size="sm">
        {g.members.map(mem => <ListRow key={mem.id} left={<Avatar name={mem.name} art={mem.art} size={m.isTV ? 48 : 34} />} title={mem.you ? `${mem.name} (you)` : mem.name} subtitle={mem.host ? `Host · ${mem.status === 'inStep' ? 'In step' : 'Catching up'}` : mem.status === 'inStep' ? 'In step' : mem.status === 'away' ? 'Away' : mem.status === 'buffering' ? 'Buffering' : 'Catching up'} trailing={mem.host ? <Badge label="Host" tone="accent" /> : undefined} />)}
        <Text variant="caption" tone="tertiary">Host: {host?.name}. Code {g.code}.</Text>
        <Button label="Open Watch Together" variant="secondary" onPress={() => { setOpen(false); push('Together'); }} />
      </Sheet>
    </>
  );
}
