import React, {useState} from 'react';
import {View} from 'react-native';
import {useAction, useClient, useQuery, useTopic} from '../../data/DataProvider';
import {Screen} from '../../ui/Page';
import {Button} from '../../ui/Button';
import {Avatar, Badge, EmptyState, ListRow, Notice, Segmented, Surface, TextField} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Artwork} from '../../ui/Artwork';
import {ConfirmDialog} from '../../ui/Sheet';
import {useMetrics} from '../../theme/layout';
import {useSession} from '../../app/session';
import {useActions} from '../../app/actions';
import {useToast} from '../../app/toast';
import {root} from '../../nav/nav';
import {colors} from '../../theme/tokens';

/** Watch Together: start or join a group, then watch in step with a shared queue. */
export function TogetherScreen() {
  const m = useMetrics();
  const client = useClient();
  useTopic(['together']);
  const {profile} = useSession();
  const actions = useActions();
  const toast = useToast();
  const groups = useQuery(c => c.together.groups(), [], ['together']);
  const g = client.together.current();
  const [code, setCode] = useState('');
  const [name, setName] = useState(`${profile?.name}’s group`);
  const [control, setControl] = useState<'host-only' | 'anyone'>('host-only');
  const [confirm, setConfirm] = useState<'leave' | 'end'>();
  const {busy, run, error} = useAction();
  const wide = !m.isPhone;
  if (!g) {
    return (
      <Screen contentStyle={{gap: m.space.xl, paddingHorizontal: m.gutter}}>
        <View style={{gap: 6}}>
          <Text variant="label" tone="accent">Watch Together</Text>
          <Text variant="display">Watch in step with friends on this server</Text>
          <Text variant="body" tone="secondary" style={{maxWidth: 820}}>Everyone in the group plays, pauses and skips together, each on their own screen. The host keeps you together.</Text>
        </View>
        {error ? <Notice tone="error" body={error} /> : null}
        <View style={{flexDirection: wide ? 'row' : 'column', gap: m.space.lg}}>
          <Surface style={{flex: 1, gap: m.space.md}}>
            <Text variant="heading">Have a code?</Text>
            <Text variant="caption" tone="tertiary">Ask the host for the eight-character code.</Text>
            <TextField label="Code from the host" placeholder="ABCD EFGH" value={code} onChangeText={t => setCode(t.toUpperCase())} autoCapitalize="characters" autoCorrect={false} maxLength={9} />
            <Button label="Join" icon="together" variant="primary" disabled={code.replace(/\s/g, '').length !== 8} loading={busy} onPress={() => void run(async () => { await client.together.join(code); toast({message: 'You joined the group.'}); })} hasTVPreferredFocus />
          </Surface>
          <Surface style={{flex: 1, gap: m.space.md}}>
            <Text variant="heading">Start a group</Text>
            <Text variant="caption" tone="tertiary">You get a code to share. Then choose Watch with group from any title’s menu.</Text>
            <TextField label="Your name in the group" value={name} onChangeText={setName} help="Shown to the others" />
            <Text variant="captionStrong" tone="secondary">Who can play, pause and choose what’s on</Text>
            <Segmented value={control} onChange={setControl} options={[{value: 'host-only', label: 'Only me'}, {value: 'anyone', label: 'Everyone'}]} />
            <Button label="Start" icon="play" variant="primary" loading={busy} onPress={() => void run(async () => { await client.together.create(name, control); })} />
          </Surface>
        </View>
        {groups.data?.length ? (
          <View style={{gap: m.space.sm}}>
            <Text variant="heading">Groups you’re in</Text>
            {groups.data.map(x => <ListRow key={x.id} icon="together" title={x.name} subtitle={`${x.members.length} here · ${x.state === 'lobby' ? 'Waiting to start' : x.state === 'playing' ? 'Playing' : 'Ready'}${x.members.find(mm => mm.host) ? ` · Host ${x.members.find(mm => mm.host)!.name}` : ''}`} onPress={() => void run(async () => { await client.together.join(x.code); })} />)}
          </View>
        ) : null}
      </Screen>
    );
  }
  const me = g.members.find(x => x.you);
  const host = me?.host;
  const now = g.nowPlaying;
  return (
    <Screen contentStyle={{gap: m.space.lg, paddingHorizontal: m.gutter}}>
      <View style={{flexDirection: 'row', alignItems: 'flex-end', gap: m.space.md, flexWrap: 'wrap'}}>
        <View style={{flex: 1, minWidth: 260}}>
          <Text variant="label" tone="accent">Watch Together · {g.state === 'lobby' ? 'Waiting to start' : g.state === 'playing' ? 'Playing' : g.state === 'paused' ? 'Ready' : 'Ended'}</Text>
          <Text variant="display">{g.name}</Text>
          <Text variant="body" tone="secondary">{g.members.length === 1 ? 'Just you so far' : `${g.members.length} people`} · {g.control === 'host-only' ? 'The host picks what to watch.' : 'Everyone controls playback.'}</Text>
        </View>
        <Button label="Leave group" icon="signOut" variant="secondary" onPress={() => setConfirm(host ? 'leave' : 'leave')} />
        {host ? <Button label="End for everyone" icon="close" variant="danger" onPress={() => setConfirm('end')} /> : null}
      </View>
      <View style={{flexDirection: wide ? 'row' : 'column', gap: m.space.lg}}>
        <View style={{flex: 1.4, gap: m.space.lg}}>
          <Surface style={{gap: m.space.md}}>
            <Text variant="subheading">Now playing</Text>
            {now ? (
              <View style={{flexDirection: 'row', gap: m.space.md, alignItems: 'center'}}>
                <Artwork source={now.backdrop ?? now.image} title={now.title} width={m.isTV ? 320 : 180} height={m.isTV ? 180 : 101} radius={m.radius.card} />
                <View style={{flex: 1, gap: 6}}>
                  <Text variant="bodyStrong">{now.title}</Text>
                  <Text variant="caption" tone="tertiary">{now.subtitle}</Text>
                  <View style={{flexDirection: 'row', gap: 8, flexWrap: 'wrap'}}>
                    <Button size="sm" icon="play" label="Open player" variant="primary" onPress={() => void actions.play(now)} hasTVPreferredFocus />
                    {host || g.control === 'anyone' ? <Button size="sm" icon={g.state === 'playing' ? 'pause' : 'play'} label={g.state === 'playing' ? 'Pause for everyone' : 'Play for everyone'} onPress={() => void client.together.setPlaying(g.state !== 'playing')} /> : null}
                  </View>
                </View>
              </View>
            ) : <Text variant="body" tone="tertiary">Nothing chosen yet. {m.isTV ? 'Hold Select on any title and choose Watch with group.' : 'Open any title’s menu and choose Watch with group.'}</Text>}
          </Surface>
          <Surface style={{gap: m.space.sm}}>
            <View style={{flexDirection: 'row', alignItems: 'center'}}><Text variant="subheading" style={{flex: 1}}>Queue</Text><Button size="sm" icon="search" label="Find something" variant="ghost" onPress={() => root('Home')} /></View>
            {g.queue.length ? g.queue.map(q => <ListRow key={q.id} left={<Artwork source={q.image} title={q.title} width={m.isTV ? 60 : 40} height={m.isTV ? 90 : 60} radius={4} />} title={q.title} subtitle={q.subtitle} onPress={host ? () => void client.together.watch(q.id) : undefined} chevron={false} trailing={host ? <Button size="sm" icon="close" accessibilityLabel="Remove from queue" variant="ghost" onPress={() => void client.together.queueRemove(q.id)} /> : undefined} />) : <Text variant="caption" tone="tertiary">Nothing queued. Add titles from their menu with Add to group queue.</Text>}
          </Surface>
        </View>
        <View style={{flex: 1, gap: m.space.lg}}>
          <Surface style={{gap: m.space.sm}}>
            <Text variant="subheading">Invite people</Text>
            <View style={{padding: m.space.md, borderRadius: m.radius.control, backgroundColor: colors.raisedSoft, alignItems: 'center'}}><Text variant="display" style={{letterSpacing: 6}}>{g.code}</Text></View>
            <Text variant="caption" tone="tertiary">They choose Watch Together, then enter the code. It works for 15 minutes, for people with a profile on this server.</Text>
            <View style={{flexDirection: 'row', gap: 8, flexWrap: 'wrap'}}>
              {!m.isTV ? <Button size="sm" icon="share" label="Share code" variant="secondary" onPress={() => toast({message: `Join my Portico group with the code ${g.code}`, icon: 'share'})} /> : null}
              <Button size="sm" icon="refresh" label="New code" variant="ghost" onPress={() => void client.together.newCode()} />
            </View>
          </Surface>
          <Surface style={{gap: 4}}>
            <Text variant="subheading" style={{marginBottom: 6}}>Who’s here</Text>
            {g.members.map(x => (
              <ListRow key={x.id} left={<Avatar name={x.name} art={x.art} size={m.isTV ? 48 : 34} />} title={x.you ? `${x.name} (you)` : x.name} subtitle={x.host ? `Host · ${x.status === 'inStep' ? 'In step' : 'Catching up'}` : x.status === 'inStep' ? 'In step' : x.status === 'away' ? 'Away' : x.status === 'buffering' ? 'Buffering' : 'Catching up'} chevron={false}
                trailing={x.host ? <Badge label="Host" tone="accent" /> : host && !x.you ? <Button size="sm" label="Make host" variant="ghost" onPress={() => void client.together.makeHost(x.id)} /> : undefined} />
            ))}
          </Surface>
        </View>
      </View>
      <ConfirmDialog visible={confirm === 'leave'} onClose={() => setConfirm(undefined)} title={host ? 'Leave while you’re the host?' : 'Leave the group?'} body={host ? 'The group pauses after two minutes without a host, then ends. Make someone else the host first to keep it going.' : 'You can join again with the code.'} confirmLabel={host ? 'Leave anyway' : 'Leave'} onConfirm={() => void run(async () => { await client.together.leave(); setConfirm(undefined); })} />
      <ConfirmDialog visible={confirm === 'end'} onClose={() => setConfirm(undefined)} title="End the group for everyone?" body="Everyone’s playback carries on by itself, out of step." confirmLabel="End group" destructive onConfirm={() => void run(async () => { await client.together.end(); setConfirm(undefined); })} />
      {g.members.length === 1 ? <EmptyState icon="together" title="Waiting for people to join" body="Share the code. People appear here as they join." compact /> : null}
    </Screen>
  );
}
