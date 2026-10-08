import React, {useState} from 'react';
import {Share, StyleSheet, View} from 'react-native';
import {useRoute, type RouteProp} from '@react-navigation/native';
import {useAction, useQuery, useServices} from '../../data/hooks';
import type {TogetherGroup, TogetherMember} from '../../data/types';
import {Artwork} from '../../design/Artwork';
import {Button, IconButton} from '../../design/Button';
import {Badge, ListRow, Panel, Segmented, TextField} from '../../design/Controls';
import {ui} from '../../design/overlays';
import {Banner, EmptyState} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {Screen} from '../../nav/Screen';
import {go, push, replaceAll, type Routes} from '../../nav/routes';
import {useItems} from '../shared/useItems';
import {plural} from '../../design/format';

const stateLabel: Record<TogetherGroup['state'], string> = {lobby: 'Waiting to start', playing: 'Playing', hostReconnecting: 'Host reconnecting', ended: 'Ended'};
const memberStatus: Record<TogetherMember['status'], string> = {inStep: 'In step', buffering: 'Buffering', catchingUp: 'Catching up', away: 'Away'};
const fmtCode = (c: string) => `${c.slice(0, 4)} ${c.slice(4)}`;

export function TogetherScreen() {
  const s = useServices();
  const layout = useLayout();
  const groups = useQuery('together:groups', () => s.together.groups());
  const [name, setName] = useState('Justin’s group');
  const [control, setControl] = useState<'host' | 'everyone'>('host');
  const [code, setCode] = useState('');
  const [error, setError] = useState<string | null>(null);
  const {busy, run} = useAction();
  return (
    <Screen heading="Watch Together" eyebrow="In step with other people on this server">
      <Text variant="body" tone="secondary" style={{marginBottom: u(16), maxWidth: 900}}>Everyone in the group plays, pauses and skips together, each on their own screen. The host keeps you together.</Text>
      {(groups.data ?? []).filter(g => g.state !== 'ended').length ? (
        <Panel title="Groups you’re in">
          {(groups.data ?? []).filter(g => g.state !== 'ended').map((g, i) => (
            <ListRow key={g.id} icon="together" label={g.name} help={`${g.members.length} here · ${stateLabel[g.state]}${g.members.find(m => m.you)?.host ? ' · You host' : ''}`} onPress={() => push('TogetherGroup', {id: g.id})} hasTVPreferredFocus={i === 0} />
          ))}
        </Panel>
      ) : null}
      <View style={{flexDirection: layout.compact ? 'column' : 'row', gap: u(16)}}>
        <Panel title="Start a group" lede="You get a code to share. Then choose Watch with group from any title’s menu." style={{flex: 1}}>
          <TextField label="Group name" value={name} onChangeText={setName} autoCapitalize="words" />
          <Text variant="captionStrong" tone="secondary" style={{marginBottom: 6}}>Who can play, pause and choose what’s on</Text>
          <Segmented value={control} onChange={setControl} options={[{value: 'host', label: 'Only me'}, {value: 'everyone', label: 'Everyone'}]} />
          <Button label="Start" icon="play" variant="primary" busy={busy} style={{marginTop: 14, alignSelf: 'flex-start'}} onPress={() => run(async () => { const g = await s.together.create(name, control); push('TogetherGroup', {id: g.id}); })} />
        </Panel>
        <Panel title="Have a code?" lede="Ask the host for the eight-character code." style={{flex: 1}}>
          <TextField label="Code from the host" value={code} onChangeText={v => setCode(v.toUpperCase())} placeholder="ABCD EFGH" error={error} />
          <Button label="Join" icon="login" busy={busy} style={{alignSelf: 'flex-start'}} onPress={() => run(async () => { const g = await s.together.join(code); if (g) push('TogetherGroup', {id: g.id}); else setError('That code didn’t work.'); })} />
        </Panel>
      </View>
    </Screen>
  );
}

export function TogetherGroupScreen() {
  const route = useRoute<RouteProp<Routes, 'TogetherGroup'>>();
  const s = useServices();
  const layout = useLayout();
  const q = useQuery('together:' + route.params.id, () => s.together.group(route.params.id));
  const g = q.data;
  const items = useItems(g ? [...g.queue, ...(g.nowPlaying ? [g.nowPlaying] : [])] : undefined);
  const [codeShown, setCodeShown] = useState(false);
  if (!g) return <Screen><EmptyState icon="together" title="Loading group" /></Screen>;
  const me = g.members.find(m => m.you);
  const host = !!me?.host;
  const ready = g.members.filter(m => m.ready).length;
  if (g.state === 'ended') {
    return (
      <Screen>
        <EmptyState icon="together" title="This group has ended" body="The host ended the group." action={{label: 'Back to Watch Together', onPress: () => go('Together')}} />
      </Screen>
    );
  }
  const now = g.nowPlaying ? items.lookup(g.nowPlaying) : undefined;
  return (
    <Screen title={g.name}>
      <Text variant="eyebrow" tone="accent">{stateLabel[g.state]} · {g.members.length} people · {g.control === 'host' ? 'The host controls playback' : 'Everyone controls playback'}</Text>
      <Text variant="hero">{g.name}</Text>
      {g.state === 'hostReconnecting' ? <Banner icon="refresh" tone="warn" title="The host’s connection dropped." body="The group pauses in 2:00 unless they return. You can take over as host." action={{label: 'Become host', onPress: () => s.together.update(g.id, {state: 'playing', members: g.members.map(m => ({...m, host: m.you}))})}} /> : null}
      <View style={{flexDirection: layout.compact ? 'column' : 'row', gap: u(16), marginTop: u(14)}}>
        <View style={{flex: 1.3}}>
          <Panel title="Now playing">
            {now ? (
              <View style={{flexDirection: 'row', alignItems: 'center'}}>
                <Artwork art={now.art.poster} tint={now.tint} style={{width: u(70), height: u(105), borderRadius: 8}} />
                <View style={{flex: 1, marginLeft: 12}}>
                  <Text variant="bodyStrong">{now.title}</Text>
                  <Text variant="caption" tone="tertiary">{Math.floor(g.position / 60)}:{String(g.position % 60).padStart(2, '0')} · everyone in step</Text>
                </View>
              </View>
            ) : (
              <Text variant="body" tone="tertiary">{host ? 'Nothing chosen yet. Open any title’s menu and choose Watch with group.' : 'The host picks what to watch.'}</Text>
            )}
            <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 10, marginTop: 12}}>
              {host ? <Button label={g.state === 'playing' ? 'Pause for everyone' : 'Play for everyone'} icon={g.state === 'playing' ? 'pause' : 'play'} variant="primary" hasTVPreferredFocus onPress={() => s.together.update(g.id, {state: g.state === 'playing' ? 'lobby' : 'playing', nowPlaying: g.nowPlaying ?? g.queue[0] ?? null})} /> : null}
              {g.nowPlaying || g.queue[0] ? <Button label="Open player" icon="tv" onPress={() => go('Player', {id: g.nowPlaying ?? g.queue[0]})} /> : null}
            </View>
            <Text variant="caption" tone={ready === g.members.length ? 'healthy' : 'amber'} style={{marginTop: 10}}>{ready === g.members.length ? 'Everyone is ready.' : `${ready} of ${g.members.length} ready · waiting for ${g.members.length - ready === 1 ? 'one person' : `${plural(g.members.length - ready, 'person', 'people')}`}`}</Text>
            {ready < g.members.length && host ? <Button label="Go ahead without them" size="sm" variant="ghost" onPress={() => s.together.update(g.id, {members: g.members.map(m => ({...m, ready: true}))})} /> : null}
          </Panel>
          <Panel title="Queue" lede={host ? 'Add titles from their menu with Add to group queue.' : undefined}>
            {!g.queue.length ? <Text variant="body" tone="tertiary">Nothing queued.</Text> : null}
            {g.queue.map((id, i) => {
              const it = items.lookup(id);
              return <ListRow key={id} label={`${i + 1}. ${it?.title ?? 'Loading title'}`} help={it ? String(it.year ?? '') : undefined} compact chevron={false} onPress={() => ui.menu({title: it?.title ?? 'Title', actions: [{label: 'Watch this', icon: 'play', onPress: () => s.together.update(g.id, {nowPlaying: id, state: 'playing'})}, {label: 'Remove from queue', icon: 'x', destructive: true, onPress: () => s.together.update(g.id, {queue: g.queue.filter(x => x !== id)})}]})} />;
            })}
          </Panel>
        </View>
        <View style={{flex: 1}}>
          <Panel title="People" lede={g.members.length === 1 ? 'Just you so far' : `${plural(g.members.length, 'person', 'people')}`}>
            {g.members.map(m => (
              <View key={m.id} style={styles.member}>
                <View style={[styles.avatar, {backgroundColor: m.color}]}><Text variant="captionStrong" tone="ink">{m.name[0]}</Text></View>
                <View style={{flex: 1, marginLeft: 10}}>
                  <Text variant="bodyStrong">{m.you ? `${m.name} (you)` : m.name}</Text>
                  <Text variant="caption" color={m.status === 'inStep' ? colors.healthy : colors.amber}>{m.host ? `Host · ${memberStatus[m.status]}` : memberStatus[m.status]}</Text>
                </View>
                {host && !m.you ? <Button label="Make host" size="sm" variant="ghost" onPress={() => s.together.update(g.id, {members: g.members.map(x => ({...x, host: x.id === m.id}))})} /> : null}
              </View>
            ))}
          </Panel>
          <Panel title="Invite people" lede="They choose Watch Together, then enter the code. Works 5 times until 9:40 PM. They need a profile on this server.">
            <Text style={{fontFamily: 'Menlo', fontSize: isTV ? 56 : 30, color: colors.text, letterSpacing: 4}}>{codeShown ? fmtCode(g.code) : '•••• ••••'}</Text>
            <View style={{flexDirection: 'row', gap: 8, marginTop: 10, flexWrap: 'wrap'}}>
              <Button label={codeShown ? 'Copy code' : 'Get a code'} icon={codeShown ? 'copy' : 'key'} size="sm" onPress={() => (codeShown ? ui.toast('Copied') : setCodeShown(true))} />
              {!isTV && codeShown ? <Button label="Share code" icon="share" size="sm" onPress={() => Share.share({message: `Join my Portico group with the code ${fmtCode(g.code)}`})} /> : null}
              {codeShown ? <Button label="New code" icon="refresh" size="sm" variant="ghost" onPress={() => ui.toast('New code made. The old one stops working.')} /> : null}
            </View>
          </Panel>
          <View style={{flexDirection: 'row', gap: 10}}>
            <Button label="Leave group" icon="logout" onPress={() => ui.confirm({title: host ? 'Leave as host?' : 'Leave this group?', body: host ? 'Someone else becomes host and the group carries on.' : 'You can rejoin with the code while the group is open.', confirmLabel: 'Leave group', onConfirm: () => s.together.leave(g.id).then(() => go('Together'))})} />
            {host ? <Button label="End for everyone" icon="stop" variant="danger" onPress={() => ui.confirm({title: 'End the group for everyone?', body: 'Everyone’s playback stops and the code stops working.', confirmLabel: 'End for everyone', destructive: true, onConfirm: () => s.together.end(g.id)})} /> : null}
          </View>
        </View>
      </View>
    </Screen>
  );
}

export function TogetherJoinScreen() {
  const route = useRoute<RouteProp<Routes, 'TogetherJoin'>>();
  const s = useServices();
  const [name, setName] = useState('Justin');
  const {busy, run} = useAction();
  const code = route.params?.code ?? 'KXWP7M2Q';
  return (
    <Screen chromeless>
      <View style={{flex: 1, justifyContent: 'center', alignItems: 'center'}}>
        <View style={[styles.joinCard, {width: isTV ? 900 : '100%'}]}>
          <Text variant="eyebrow" tone="accent">Watch Together</Text>
          <Text variant="display">Join Friday Movie Night</Text>
          <Text variant="body" tone="secondary" style={{marginVertical: 8}}>Justin invited you with the code {fmtCode(code)}.</Text>
          <TextField label="Your name in the group" value={name} onChangeText={setName} help="Shown to the others" />
          <View style={{flexDirection: 'row', gap: 10}}>
            <Button label="Join" variant="primary" busy={busy} hasTVPreferredFocus onPress={() => run(async () => { const g = await s.together.join(code); if (g) replaceAll('TogetherGroup', {id: g.id}); })} />
            <Button label="Not now" onPress={() => replaceAll('Home')} />
          </View>
        </View>
      </View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  member: {flexDirection: 'row', alignItems: 'center', paddingVertical: u(6)},
  avatar: {width: u(34), height: u(34), borderRadius: u(17), alignItems: 'center', justifyContent: 'center'},
  joinCard: {padding: u(24), borderRadius: radius.xl, backgroundColor: 'rgba(16,33,45,0.85)', borderWidth: StyleSheet.hairlineWidth, borderColor: colors.line},
});

export {Badge, IconButton};
