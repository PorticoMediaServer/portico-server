import React, {useState} from 'react';
import {View} from 'react-native';
import {useQuery} from '../data/DataProvider';
import type {Entry, Track} from '../data/types';
import {Sheet} from '../ui/Sheet';
import {ListRow, Notice, Stepper, Tabs} from '../ui/Controls';
import {Text} from '../ui/Text';
import {Button} from '../ui/Button';
import {useMetrics} from '../theme/layout';
import {usePlayer, usePlayerState} from './PlayerProvider';

export type OptionsTab = 'audio' | 'subtitles' | 'quality' | 'speed' | 'sleep' | 'info';
const defaultAudio: Track[] = [{id: 'a1', label: 'English', language: 'en', codec: 'AAC', channels: 'Stereo', default: true}, {id: 'a2', label: 'Japanese', language: 'ja', codec: 'AAC', channels: 'Stereo'}];
const defaultSubs: Track[] = [{id: 's1', label: 'English', language: 'en', codec: 'SRT', origin: 'sidecar'}, {id: 's2', label: 'English (SDH)', language: 'en', codec: 'ASS', origin: 'embedded'}, {id: 's3', label: 'Spanish', language: 'es', codec: 'SRT', origin: 'provider'}];

/** Audio · Subtitles · Quality · Speed · Sleep · Playback info, as a side panel. */
export function PlayerOptions({visible, onClose, entry, initialTab = 'audio', mode}: {visible: boolean; onClose: () => void; entry?: Entry; initialTab?: OptionsTab; mode: 'video' | 'music' | 'book' | 'live'}) {
  const m = useMetrics();
  const s = usePlayerState();
  const player = usePlayer();
  const [tab, setTab] = useState<OptionsTab>(initialTab);
  React.useEffect(() => { if (visible) setTab(initialTab); }, [visible, initialTab]);
  const detail = useQuery(c => (entry && c.catalog.entry(entry.id)?.kind === 'movie' ? c.catalog.movie(entry.id) : Promise.resolve(undefined)), [entry?.id]);
  const audio = detail.data?.audio ?? defaultAudio;
  const subs = detail.data?.subtitles ?? defaultSubs;
  const [finding, setFinding] = useState(false);
  const tabs: {value: OptionsTab; label: string}[] = mode === 'video' || mode === 'live'
    ? [{value: 'audio', label: 'Audio'}, {value: 'subtitles', label: 'Subtitles'}, {value: 'quality', label: 'Quality'}, ...(mode === 'video' ? [{value: 'speed' as const, label: 'Speed'}] : []), {value: 'sleep', label: 'Sleep'}, {value: 'info', label: 'Info'}]
    : [{value: 'speed', label: 'Speed'}, {value: 'sleep', label: 'Sleep timer'}, {value: 'quality', label: 'Quality'}, {value: 'info', label: 'Info'}];
  const qualities = [
    {id: 'auto', label: 'Automatic', sub: 'Best quality your connection allows'},
    {id: 'original', label: 'Original', sub: 'Plays the file as is · 4K HEVC · 42 Mbps'},
    {id: '1080', label: 'Convert to 1080p', sub: '10 Mbps'},
    {id: '720', label: 'Convert to 720p', sub: '4 Mbps'},
    {id: '480', label: 'Convert to 480p', sub: '1.5 Mbps'},
  ];
  return (
    <Sheet visible={visible} onClose={onClose} side title="Playback options" contentStyle={{gap: m.space.sm}}>
      <Tabs tabs={tabs} value={tab} onChange={setTab} />
      {tab === 'audio' ? (
        <View>
          {audio.map(t => <ListRow key={t.id} title={t.label} subtitle={[t.codec, t.channels, t.default ? 'Default' : undefined].filter(Boolean).join(' · ')} selected={s.audio === t.id} chevron={false} onPress={() => player.set({audio: t.id})} />)}
          <Text variant="caption" tone="tertiary" style={{marginTop: 8}}>Audio: mixed to stereo for this device.</Text>
        </View>
      ) : null}
      {tab === 'subtitles' ? (
        <View style={{gap: m.space.xs}}>
          <ListRow title="Off" selected={s.subtitles === 'off'} chevron={false} onPress={() => player.set({subtitles: 'off'})} />
          {subs.map(t => <ListRow key={t.id} title={t.label} subtitle={[t.forced ? 'Forced' : undefined, t.origin === 'embedded' ? 'In the file' : t.origin === 'sidecar' ? 'Beside the file' : 'Downloaded', t.codec === 'PGS' ? 'Converts the video · lower quality' : undefined].filter(Boolean).join(' · ')} selected={s.subtitles === t.id} chevron={false} onPress={() => player.set({subtitles: t.id})} />)}
          <ListRow icon="search" title={finding ? 'Looking for subtitles…' : 'Find subtitles…'} subtitle="Search OpenSubtitles for this title" onPress={() => { setFinding(true); setTimeout(() => setFinding(false), 1400); }} />
          <View style={{flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: m.isTV ? 22 : 14, paddingVertical: 8}}>
            <View><Text variant="body">Timing</Text><Text variant="caption" tone="tertiary">Move subtitles earlier or later</Text></View>
            <Stepper label="Subtitle timing" value={s.subtitleOffset} onChange={v => player.set({subtitleOffset: v})} min={-10} max={10} step={0.5} unit="s" />
          </View>
        </View>
      ) : null}
      {tab === 'quality' ? (
        <View>
          {qualities.map(q => <ListRow key={q.id} title={q.label} subtitle={q.sub} selected={s.quality === q.id} chevron={false} onPress={() => player.set({quality: q.id})} />)}
          <Text variant="caption" tone="tertiary" style={{marginTop: 8}}>Connection: home network · up to 4K.</Text>
        </View>
      ) : null}
      {tab === 'speed' ? (
        <View>
          {[0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, ...(mode === 'book' ? [2.5, 3] : [])].map(v => <ListRow key={v} title={v === 1 ? 'Normal' : `${v}×`} selected={s.speed === v} chevron={false} onPress={() => player.set({speed: v})} />)}
        </View>
      ) : null}
      {tab === 'sleep' ? (
        <View>
          {s.sleepEndsAt ? <Notice tone="info" body={`Sleep timer: ${s.sleepLabel}. Playback pauses at ${new Date(s.sleepEndsAt).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'})}.`} /> : null}
          <ListRow title="Off" selected={!s.sleepEndsAt} chevron={false} onPress={() => player.sleep(undefined)} />
          {[5, 10, 15, 30, 45, 60, 90].map(v => <ListRow key={v} title={`${v} minutes`} selected={s.sleepLabel === `${v} min`} chevron={false} onPress={() => player.sleep(v)} />)}
          <ListRow title={mode === 'book' ? 'End of chapter' : 'End of this title'} selected={s.sleepLabel === 'End of chapter'} chevron={false} onPress={() => player.sleep('end')} />
        </View>
      ) : null}
      {tab === 'info' ? (
        <View style={{gap: 6}}>
          {[
            ['Playback', s.quality === 'auto' || s.quality === 'original' ? 'Playing the file as it is' : `Converted to ${s.quality}p for this device`],
            ['Video', mode === 'music' || mode === 'book' ? '—' : s.quality === 'original' || s.quality === 'auto' ? 'HEVC Main 10 · 3840×2160 · Dolby Vision' : `H.264 · ${s.quality}p · converted by graphics hardware (VideoToolbox)`],
            ['Audio', mode === 'music' ? 'FLAC 24-bit/96 kHz · gapless' : 'E-AC-3 5.1 → AAC Stereo (mixed for this device)'],
            ['Subtitles', s.subtitles === 'off' ? 'Off' : 'SRT · unchanged'],
            ['Bitrate', s.quality === 'original' ? '42 Mbps' : '10 Mbps'],
            ['Connection', 'Home network · Lighthouse (192.168.1.20)'],
            ['Buffer', '38 s ahead'],
          ].map(([k, v]) => <View key={k} style={{flexDirection: 'row', gap: 12, paddingVertical: 4}}><Text variant="captionStrong" tone="tertiary" style={{width: m.isTV ? 170 : 100}}>{k}</Text><Text variant="caption" style={{flex: 1}}>{v}</Text></View>)}
        </View>
      ) : null}
      {m.isTV ? <Button label="Done" variant="secondary" onPress={onClose} /> : null}
    </Sheet>
  );
}
