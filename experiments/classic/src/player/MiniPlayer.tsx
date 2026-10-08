import React from 'react';
import {View} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {Artwork} from '../ui/Artwork';
import {IconButton} from '../ui/Button';
import {Focusable} from '../ui/Focusable';
import {Text} from '../ui/Text';
import {ProgressBar} from '../ui/Controls';
import {usePlayer, usePlayerState} from './PlayerProvider';
import {PHONE_TABS} from '../ui/Page';

/** Music and audiobooks keep playing in a mini player docked above the tab bar. */
export function MiniPlayer() {
  const s = usePlayerState();
  const player = usePlayer();
  const m = useMetrics();
  const insets = useSafeAreaInsets();
  if (s.mode === 'none' || s.expanded || m.isTV) return null;
  const e = s.queue[s.index];
  if (!e) return null;
  return (
    <View style={{position: 'absolute', left: m.isPhone ? 8 : 16, right: m.isPhone ? 8 : 16, bottom: m.isPhone ? insets.bottom + PHONE_TABS + 6 : insets.bottom + 16, maxWidth: m.isPhone ? undefined : 560, alignSelf: 'center'}}>
      <Focusable onPress={player.expand} accessibilityLabel={`Open now playing, ${e.title}`} style={{borderRadius: 14, backgroundColor: colors.raisedBright, borderWidth: 1, borderColor: colors.lineStrong, overflow: 'hidden', shadowColor: '#000', shadowOpacity: 0.45, shadowRadius: 18, shadowOffset: {width: 0, height: 6}}}>
        <View style={{flexDirection: 'row', alignItems: 'center', gap: 10, padding: 8}}>
          <Artwork source={e.image} title={e.title} width={44} height={44} radius={8} />
          <View style={{flex: 1}}>
            <Text variant="bodyStrong" numberOfLines={1}>{e.title}</Text>
            <Text variant="caption" tone="tertiary" numberOfLines={1}>{s.cast ? `Playing on ${s.cast.name}` : e.subtitle}</Text>
          </View>
          <IconButton icon={s.playing ? 'pause' : 'play'} label={s.playing ? 'Pause' : 'Play'} size={40} onPress={player.toggle} />
          <IconButton icon="next" label="Next" size={40} onPress={player.next} disabled={s.mode === 'book'} />
          <IconButton icon="close" label="Close player" size={36} onPress={player.close} />
        </View>
        <ProgressBar value={s.duration ? s.position / s.duration : 0} height={3} track="transparent" style={{borderRadius: 0}} />
      </Focusable>
    </View>
  );
}
