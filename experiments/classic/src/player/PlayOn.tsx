import React, {useEffect, useState} from 'react';
import {ActivityIndicator, View} from 'react-native';
import {Sheet} from '../ui/Sheet';
import {ListRow, Notice} from '../ui/Controls';
import {Text} from '../ui/Text';
import {Button} from '../ui/Button';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {formatTime, usePlayer, usePlayerState} from './PlayerProvider';

const devices = [
  {kind: 'portico' as const, name: 'Living Room Apple TV', sub: 'Portico · on this network', icon: 'tvDevice' as const},
  {kind: 'portico' as const, name: 'Bedroom SHIELD', sub: 'Portico · on this network', icon: 'tvDevice' as const},
  {kind: 'cast' as const, name: 'Kitchen Nest Hub', sub: 'Google Cast', icon: 'cast' as const},
  {kind: 'cast' as const, name: 'Office Chromecast', sub: 'Google Cast', icon: 'cast' as const},
];

/** Play on: AirPlay, Google Cast and other Portico devices, with the hand-over states. */
export function PlayOnSheet({visible, onClose}: {visible: boolean; onClose: () => void}) {
  const m = useMetrics();
  const s = usePlayerState();
  const player = usePlayer();
  const [phase, setPhase] = useState<{name: string; step: 'looking' | 'connecting' | 'approve' | 'done'}>();
  const [scanning, setScanning] = useState(true);
  const [stopped, setStopped] = useState<{name: string; at: number}>();
  useEffect(() => { if (visible) { setScanning(true); const t = setTimeout(() => setScanning(false), 1200); return () => clearTimeout(t); } }, [visible]);
  const choose = (d: (typeof devices)[number] | {kind: 'airplay'; name: string}) => {
    setPhase({name: d.name, step: 'connecting'});
    setTimeout(() => setPhase({name: d.name, step: d.kind === 'portico' ? 'approve' : 'done'}), 1100);
    setTimeout(() => { player.set({cast: {kind: d.kind, name: d.name}}); setPhase({name: d.name, step: 'done'}); }, d.kind === 'portico' ? 2600 : 1400);
  };
  return (
    <Sheet visible={visible} onClose={() => { setPhase(undefined); onClose(); }} title="Play on" subtitle="Send what’s playing to another screen or speaker." size="sm">
      {s.cast ? (
        <Notice tone="success" title={`Playing on ${s.cast.name}`} body="Use the other device’s remote or app to control it." action={{label: 'Stop', onPress: () => { setStopped({name: s.cast!.name, at: s.position}); player.set({cast: undefined, playing: false}); setPhase(undefined); }}} />
      ) : null}
      {stopped && !s.cast ? (
        <Notice tone="info" title={`Stopped playing on ${stopped.name}.`} body="Pick up here where the other screen left off." action={{label: `Continue here from ${formatTime(stopped.at)}`, onPress: () => { player.seek(stopped.at); player.set({playing: true}); setStopped(undefined); onClose(); }}} />
      ) : null}
      {phase && phase.step !== 'done' ? (
        <View style={{flexDirection: 'row', gap: 12, alignItems: 'center', padding: 8}}>
          <ActivityIndicator color={colors.accentStrong} />
          <View style={{flex: 1}}>
            <Text variant="bodyStrong">{phase.step === 'approve' ? `Approve this on ${phase.name}.` : `Connecting to ${phase.name}…`}</Text>
            <Text variant="caption" tone="tertiary">Playback carries on here until the other device has taken over.</Text>
          </View>
        </View>
      ) : null}
      {!m.isTV ? <ListRow icon="airplay" title="AirPlay" subtitle="TVs and speakers near you" onPress={() => choose({kind: 'airplay', name: 'Living Room HomePod'})} /> : null}
      <Text variant="label" tone="tertiary" style={{marginTop: 6}}>On this network</Text>
      {scanning ? <View style={{flexDirection: 'row', gap: 10, alignItems: 'center', padding: 10}}><ActivityIndicator color={colors.accentStrong} size="small" /><Text variant="caption" tone="tertiary">Looking on this network</Text></View> : devices.map(d => (
        <ListRow key={d.name} icon={d.icon} title={d.name} subtitle={d.sub} selected={s.cast?.name === d.name} onPress={() => choose(d)} chevron={false} />
      ))}
      {m.isTV ? <Button label="Close" variant="secondary" onPress={onClose} /> : null}
    </Sheet>
  );
}
