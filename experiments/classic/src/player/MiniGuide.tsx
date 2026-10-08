import React from 'react';
import {View} from 'react-native';
import {useQuery} from '../data/DataProvider';
import {Sheet} from '../ui/Sheet';
import {Focusable} from '../ui/Focusable';
import {Artwork} from '../ui/Artwork';
import {Text} from '../ui/Text';
import {ProgressBar} from '../ui/Controls';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {usePlayer, usePlayerState} from './PlayerProvider';

const time = (t: number) => new Date(t).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});

/** The in-player mini guide: channels with what's on now and next; select to switch. */
export function MiniGuide({visible, onClose}: {visible: boolean; onClose: () => void}) {
  const s = usePlayerState();
  const player = usePlayer();
  const m = useMetrics();
  const now = Date.now();
  const data = useQuery(async c => {
    const chans = await c.live.channels();
    const guide = await c.live.guide(now, now + 3 * 3600000, chans.map(x => x.id));
    return chans.map(ch => ({ch, progs: guide[ch.id] ?? []}));
  }, [visible]);
  return (
    <Sheet visible={visible} onClose={onClose} side title="Guide" subtitle="Now and next on every channel" contentStyle={{gap: 4}}>
      {(data.data ?? []).map(({ch, progs}) => {
        const cur = progs[0];
        const nxt = progs[1];
        const active = ch.id === s.liveChannelId;
        return (
          <Focusable key={ch.id} hasTVPreferredFocus={active} onPress={() => { player.playLive(ch.id, {id: ch.id, kind: 'channel', title: cur?.title ?? ch.name, subtitle: `${ch.number} · ${ch.name}`, image: cur?.image, shape: 'wide', caption: cur?.episodeTitle}); onClose(); }}
            style={({focused}) => ({flexDirection: 'row', alignItems: 'center', gap: m.space.sm, padding: m.isTV ? 12 : 8, borderRadius: m.radius.control, backgroundColor: focused ? colors.raisedBright : active ? 'rgba(241,121,121,0.12)' : 'transparent'})}>
            <View style={{width: m.isTV ? 110 : 70, alignItems: 'center', gap: 2}}>
              <Artwork source={ch.logo} title={ch.name} width={m.isTV ? 96 : 60} height={m.isTV ? 54 : 34} radius={4} resizeMode="contain" style={{backgroundColor: 'transparent'}} />
              <Text variant="micro" tone="tertiary">{ch.number}</Text>
            </View>
            <View style={{flex: 1, gap: 2}}>
              <Text variant="bodyStrong" numberOfLines={1}>{cur?.title ?? 'No information'}</Text>
              {cur ? <ProgressBar value={(now - cur.start) / (cur.end - cur.start)} height={3} /> : null}
              <Text variant="caption" tone="tertiary" numberOfLines={1}>{nxt ? `Next · ${time(nxt.start)}  ${nxt.title}` : ''}</Text>
            </View>
          </Focusable>
        );
      })}
    </Sheet>
  );
}
