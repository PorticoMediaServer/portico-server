import React from 'react';
import {Pressable, StyleSheet, View} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {useQuery, useServices} from '../../data/hooks';
import {Artwork} from '../../design/Artwork';
import {IconButton} from '../../design/Button';
import {Focusable} from '../../design/Focusable';
import {Icon} from '../../design/Icon';
import {Text} from '../../design/Text';
import {colors, isTV, radius} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {go} from '../../nav/routes';
import {MINI_PLAYER_HEIGHT} from '../../nav/Screen';
import {TAB_BAR_HEIGHT} from '../../nav/TabBar';
import {player, usePlayer} from '../../state/player';

/** The mini player: docked above the tabs on iPhone, floating on iPad, a Now Playing pill on TV. */
export function MiniPlayer() {
  const p = usePlayer();
  const s = useServices();
  const layout = useLayout();
  const insets = useSafeAreaInsets();
  const q = useQuery(p.itemId ? 'item:' + p.itemId : null, () => s.catalog.item(p.itemId!));
  const visible = p.active && p.kind !== 'live' && (p.kind !== 'video' || p.minimized);
  if (!visible || !q.data) return null;
  const it = q.data;
  const art = it.art.square ?? it.art.poster ?? it.art.still ?? it.art.backdrop;
  const sub = it.kind === 'track' ? it.artistName : it.kind === 'audiobook' ? it.author : it.kind === 'episode' ? `Episode ${it.number}` : '';
  const open = () => {
    if (p.kind === 'video') {
      player.set({minimized: false});
      go('Player', {id: it.id, from: p.position});
    } else go('NowPlaying');
  };
  const progress = p.duration ? p.position / p.duration : 0;
  if (isTV) {
    return (
      <View style={styles.tvWrap} pointerEvents="box-none">
        <Focusable onPress={open} accessibilityLabel={`Now playing: ${it.title}`} scale={1.06}>
          {({focused}) => (
            <View style={[styles.tvPill, focused && {backgroundColor: colors.white}]}>
              <Artwork art={art} tint={it.tint} style={{width: 56, height: 56, borderRadius: 10}} />
              <View style={{marginLeft: 16, maxWidth: 360}}>
                <Text variant="micro" color={focused ? colors.actionInk : colors.accentStrong}>{p.playing ? 'NOW PLAYING' : 'PAUSED'}</Text>
                <Text variant="captionStrong" numberOfLines={1} color={focused ? colors.actionInk : colors.text}>{it.title}</Text>
              </View>
              <View style={{marginLeft: 16}}><Icon name={p.playing ? 'pause' : 'play'} size={18} color={focused ? colors.actionInk : colors.text} fill /></View>
            </View>
          )}
        </Focusable>
      </View>
    );
  }
  const floating = !layout.compact;
  return (
    <View style={[floating ? [styles.floating, {bottom: insets.bottom + 16}] : [styles.docked, {bottom: TAB_BAR_HEIGHT + insets.bottom}]]}>
      <Pressable onPress={open} style={styles.body} accessibilityLabel={`Open now playing: ${it.title}`}>
        <Artwork art={art} tint={it.tint} style={styles.art} />
        <View style={{flex: 1, marginLeft: 12}}>
          <Text variant="captionStrong" numberOfLines={1}>{it.title}</Text>
          <Text variant="caption" tone="tertiary" numberOfLines={1}>{sub}</Text>
        </View>
        <IconButton icon={p.playing ? 'pause' : 'play'} label={p.playing ? 'Pause' : 'Play'} fill variant="ghost" onPress={player.toggle} size={40} />
        <IconButton icon="next" label="Next" variant="ghost" onPress={() => player.next()} size={40} />
      </Pressable>
      <View style={styles.track}><View style={[styles.fill, {width: `${progress * 100}%`}]} /></View>
    </View>
  );
}

const styles = StyleSheet.create({
  docked: {position: 'absolute', left: 8, right: 8, height: MINI_PLAYER_HEIGHT - 6, borderRadius: radius.lg, backgroundColor: colors.overlay, borderWidth: StyleSheet.hairlineWidth, borderColor: colors.lineStrong, zIndex: 25, overflow: 'hidden'},
  floating: {position: 'absolute', right: 20, width: 380, height: 68, borderRadius: radius.lg, backgroundColor: colors.overlay, borderWidth: StyleSheet.hairlineWidth, borderColor: colors.lineStrong, zIndex: 25, overflow: 'hidden', shadowColor: '#000', shadowOpacity: 0.4, shadowRadius: 20},
  body: {flex: 1, flexDirection: 'row', alignItems: 'center', paddingHorizontal: 8},
  art: {width: 42, height: 42, borderRadius: 8},
  track: {position: 'absolute', left: 0, right: 0, bottom: 0, height: 2, backgroundColor: 'rgba(255,255,255,0.1)'},
  fill: {height: 2, backgroundColor: colors.accentStrong},
  tvWrap: {position: 'absolute', top: 40, right: 64, zIndex: 25},
  tvPill: {flexDirection: 'row', alignItems: 'center', padding: 10, paddingRight: 24, borderRadius: 40, backgroundColor: 'rgba(19,38,52,0.92)', borderWidth: 1, borderColor: colors.lineStrong},
});
