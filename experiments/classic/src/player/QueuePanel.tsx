import React, {useState} from 'react';
import {View} from 'react-native';
import {useAction, useClient} from '../data/DataProvider';
import {Sheet} from '../ui/Sheet';
import {Artwork} from '../ui/Artwork';
import {IconButton, Button} from '../ui/Button';
import {Focusable} from '../ui/Focusable';
import {Text} from '../ui/Text';
import {Icon} from '../ui/Icon';
import {TextField} from '../ui/Controls';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {formatTime, usePlayer, usePlayerState} from './PlayerProvider';
import {useToast} from '../app/toast';

/** Up next: the play queue with reorder, remove, jump and Save as playlist. */
export function QueuePanel({visible, onClose}: {visible: boolean; onClose: () => void}) {
  const s = usePlayerState();
  const player = usePlayer();
  const client = useClient();
  const toast = useToast();
  const m = useMetrics();
  const [saving, setSaving] = useState(false);
  const [name, setName] = useState('Saved queue');
  const {busy, run} = useAction();
  const upcoming = s.queue.map((e, i) => ({e, i})).filter(x => x.i >= s.index).slice(0, 60);
  return (
    <Sheet visible={visible} onClose={onClose} side title="Up next" subtitle={`${Math.max(0, s.queue.length - s.index - 1)} more after this`} contentStyle={{gap: 2}}
      footer={saving ? (
        <View style={{flex: 1, gap: 8}}>
          <TextField label="Playlist name" value={name} onChangeText={setName} />
          <View style={{flexDirection: 'row', justifyContent: 'flex-end', gap: 8}}>
            <Button label="Cancel" variant="ghost" onPress={() => setSaving(false)} />
            <Button label="Save" variant="primary" loading={busy} onPress={() => void run(async () => { await client.saved.createPlaylist(name, s.queue.map(e => e.id)); setSaving(false); toast({message: 'Saved as a playlist.', icon: 'playlist'}); })} />
          </View>
        </View>
      ) : <Button label="Save as playlist" icon="playlist" variant="secondary" onPress={() => setSaving(true)} />}>
      {upcoming.map(({e, i}) => {
        const now = i === s.index;
        return (
          <Focusable key={`${e.id}-${i}`} onPress={() => player.jump(i)} hasTVPreferredFocus={i === s.index + 1} accessibilityLabel={now ? `Now playing, ${e.title}` : `Play ${e.title}`}
            style={({focused}) => ({flexDirection: 'row', alignItems: 'center', gap: m.space.sm, padding: m.isTV ? 12 : 8, borderRadius: m.radius.control, backgroundColor: focused ? colors.raisedBright : now ? 'rgba(64,159,212,0.14)' : 'transparent'})}>
            <Artwork source={e.image} title={e.title} width={e.shape === 'wide' || e.kind === 'episode' ? (m.isTV ? 128 : 88) : (m.isTV ? 72 : 48)} height={m.isTV ? 72 : 48} radius={6} />
            <View style={{flex: 1}}>
              <Text variant="bodyStrong" numberOfLines={1} color={now ? colors.accentStrong : colors.text}>{e.title}</Text>
              <Text variant="caption" tone="tertiary" numberOfLines={1}>{now ? 'Now playing' : [e.caption ?? e.subtitle, e.duration ? formatTime(e.duration) : undefined].filter(Boolean).join(' · ')}</Text>
            </View>
            {now ? <Icon name="audio" size={m.icon} color={colors.accentStrong} /> : (
              <View style={{flexDirection: 'row'}}>
                <IconButton icon="chevronUp" label="Move up" size={m.controlSmall} disabled={i <= s.index + 1} onPress={() => player.move(i, -1)} />
                <IconButton icon="chevronDown" label="Move down" size={m.controlSmall} disabled={i >= s.queue.length - 1} onPress={() => player.move(i, 1)} />
                <IconButton icon="close" label="Remove from queue" size={m.controlSmall} onPress={() => player.remove(i)} />
              </View>
            )}
          </Focusable>
        );
      })}
      {s.queue.length - s.index <= 1 ? <Text variant="caption" tone="tertiary" style={{padding: 8}}>Nothing is queued after this.</Text> : null}
    </Sheet>
  );
}
