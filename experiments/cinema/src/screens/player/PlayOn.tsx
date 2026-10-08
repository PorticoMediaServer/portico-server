import React, {useEffect, useState} from 'react';
import {ActivityIndicator, View} from 'react-native';
import {Button} from '../../design/Button';
import {ListRow} from '../../design/Controls';
import {Icon} from '../../design/Icon';
import {ui} from '../../design/overlays';
import {Text} from '../../design/Text';
import {colors, u} from '../../design/tokens';
import {player, usePlayer} from '../../state/player';

const devices = [
  {id: 'atv', name: 'Living Room Apple TV', kind: 'portico' as const, detail: 'Portico · on this network'},
  {id: 'bedroom', name: 'Bedroom Chromecast', kind: 'cast' as const, detail: 'Google Cast'},
  {id: 'homepod', name: 'Kitchen HomePod', kind: 'airplay' as const, detail: 'AirPlay · speaker'},
  {id: 'lgtv', name: 'Den TV', kind: 'airplay' as const, detail: 'AirPlay'},
];

/** Play on: AirPlay, Google Cast and other Portico devices, with the hand-over states. */
export function openPlayOn(title: string) {
  ui.sheet({title: 'Play on', subtitle: title, render: close => <PlayOnBody close={close} />});
}

function PlayOnBody({close}: {close: () => void}) {
  const p = usePlayer();
  const [phase, setPhase] = useState<{device: string; step: 'connecting' | 'approve' | 'playing' | 'failed'} | null>(null);
  useEffect(() => {
    if (!phase || phase.step === 'playing' || phase.step === 'failed') return;
    const t = setTimeout(() => {
      if (phase.step === 'connecting') setPhase({...phase, step: phase.device === 'Living Room Apple TV' ? 'approve' : phase.device === 'Den TV' ? 'failed' : 'playing'});
      else {
        setPhase({...phase, step: 'playing'});
      }
    }, 1600);
    return () => clearTimeout(t);
  }, [phase]);
  useEffect(() => {
    if (phase?.step === 'playing') player.set({playOn: {device: phase.device, kind: 'portico'}});
  }, [phase]);
  if (p.playOn && !phase) {
    return (
      <View>
        <View style={{flexDirection: 'row', alignItems: 'center', marginBottom: u(12)}}>
          <Icon name="cast" size={20} color={colors.accentStrong} />
          <Text variant="bodyStrong" style={{marginLeft: 10}}>Playing on {p.playOn.device}</Text>
        </View>
        <Text variant="caption" tone="tertiary">Use the other device’s remote or app to control it.</Text>
        <Button label="Stop playing on the TV" icon="stop" style={{marginTop: 14}} onPress={() => { player.set({playOn: null}); ui.toast(`Stopped playing on the TV. Continue here from ${Math.floor(p.position / 60)} min.`); close(); }} hasTVPreferredFocus />
      </View>
    );
  }
  if (phase) {
    const msg = {connecting: `Connecting to ${phase.device}…`, approve: `Approve this on ${phase.device}.`, playing: `Playing on ${phase.device}.`, failed: `${phase.device} didn’t answer.`}[phase.step];
    return (
      <View style={{alignItems: 'center', paddingVertical: u(20)}}>
        {phase.step === 'playing' ? <Icon name="checkCircle" size={40} color={colors.healthy} /> : phase.step === 'failed' ? <Icon name="alertCircle" size={40} color={colors.danger} /> : <ActivityIndicator color={colors.accentStrong} size="large" />}
        <Text variant="bodyStrong" center style={{marginTop: 12}}>{msg}</Text>
        <Text variant="caption" tone="tertiary" center style={{marginTop: 4}}>{phase.step === 'failed' ? 'That device couldn’t take over playback. Check that it’s on with Portico open, then try again. Still playing here.' : phase.step === 'playing' ? 'Use the other device’s remote or app to control it.' : 'Playback carries on here until the other device has taken over.'}</Text>
        <Button label={phase.step === 'failed' ? 'Try again' : 'Done'} variant="primary" style={{marginTop: 16}} hasTVPreferredFocus onPress={() => (phase.step === 'failed' ? setPhase({...phase, step: 'connecting'}) : close())} />
      </View>
    );
  }
  return (
    <View>
      <ListRow icon="airplay" label="AirPlay" help="TVs and speakers near you" onPress={() => setPhase({device: 'Den TV', step: 'connecting'})} hasTVPreferredFocus />
      <Text variant="eyebrow" tone="tertiary" style={{marginTop: u(12), marginBottom: u(4), marginLeft: u(14)}}>On this network</Text>
      {devices.map(d => (
        <ListRow key={d.id} icon={d.kind === 'cast' ? 'cast' : d.kind === 'airplay' ? 'airplay' : 'tv'} label={d.name} help={d.detail} onPress={() => setPhase({device: d.name, step: 'connecting'})} />
      ))}
    </View>
  );
}
