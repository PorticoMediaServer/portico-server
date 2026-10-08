import React, {useState} from 'react';
import {View} from 'react-native';
import {useAction, useClient, useQuery} from '../data/DataProvider';
import {useSession} from '../app/session';
import {ActionMenu, Sheet} from '../ui/Sheet';
import {Avatar, Badge, ListRow, Loading, Notice, PinPad} from '../ui/Controls';
import {Text} from '../ui/Text';
import {Focusable} from '../ui/Focusable';
import {Icon} from '../ui/Icon';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {openFromChrome, root} from '../nav/nav';
import {serverLine} from '../app/format';
import {useActions} from '../app/actions';
import {useToast} from '../app/toast';
import type {Profile} from '../data/types';

/** Who and where you are, with switch profile / server, account, feedback, help and sign out. */
export function ProfileMenu({visible, onClose}: {visible: boolean; onClose: () => void}) {
  const {profile, server, isAdmin, signOut} = useSession();
  const actions = useActions();
  const toast = useToast();
  const m = useMetrics();
  const [sheet, setSheet] = useState<'profile' | 'server'>();
  const header = (
    <View style={{flexDirection: 'row', alignItems: 'center', gap: m.space.md}}>
      <Avatar name={profile?.name ?? '?'} art={profile?.art} size={m.isTV ? 64 : 44} />
      <View style={{flex: 1}}>
        <Text variant="subheading">{profile?.name}</Text>
        <Text variant="caption" tone="tertiary">{server?.name} · Portico Account (justin)</Text>
      </View>
    </View>
  );
  return (
    <>
      <ActionMenu visible={visible} onClose={onClose} items={[
        {id: 'switch', label: 'Switch profile…', icon: 'switch'},
        {id: 'server', label: 'Switch server…', icon: 'server'},
        {id: 'account', label: 'Account', icon: 'user'},
        {id: 'settings', label: 'Settings', icon: 'settings', hidden: !m.isPhone},
        {id: 'admin', label: 'Manage server', icon: 'console', hidden: !m.isPhone || !isAdmin},
        {id: 'downloads', label: 'Downloads', icon: 'download', hidden: !m.isPhone},
        {id: 'together', label: 'Watch Together', icon: 'together', hidden: !m.isPhone},
        {id: 'feedback', label: 'Send feedback…', icon: 'flag', separatorBefore: true},
        {id: 'help', label: 'Help and support', icon: 'external'},
        {id: 'signOut', label: 'Sign out', icon: 'signOut', destructive: true, separatorBefore: true},
      ]} onSelect={id => {
        onClose();
        if (id === 'switch') setTimeout(() => setSheet('profile'), 250);
        else if (id === 'server') setTimeout(() => setSheet('server'), 250);
        else if (id === 'account') openFromChrome('Account', undefined, m.isPhone);
        else if (id === 'settings') openFromChrome('Settings', undefined, m.isPhone);
        else if (id === 'admin') openFromChrome('Admin', undefined, m.isPhone);
        else if (id === 'downloads') openFromChrome('Downloads', undefined, m.isPhone);
        else if (id === 'together') openFromChrome('Together', undefined, m.isPhone);
        else if (id === 'feedback') setTimeout(() => actions.feedback(), 250);
        else if (id === 'help') toast({message: 'Help and support: getportico.tv/help', icon: 'external'});
        else if (id === 'signOut') void signOut();
      }}>
        <View style={{padding: m.space.sm, paddingBottom: m.space.md}}>{header}</View>
      </ActionMenu>
      <ProfileSwitcher visible={sheet === 'profile'} onClose={() => setSheet(undefined)} />
      <ServerSwitcher visible={sheet === 'server'} onClose={() => setSheet(undefined)} onChosen={() => setTimeout(() => setSheet('profile'), 250)} />
    </>
  );
}

/** "Who's watching?" grid with PIN entry, reused by the sign-in flow. */
export function ProfileGrid({onChosen, serverId}: {onChosen?: () => void; serverId?: string}) {
  const {enter, profile: current} = useSession();
  const m = useMetrics();
  const profiles = useQuery(c => c.session.profiles(serverId), [serverId], ['account']);
  const [pinFor, setPinFor] = useState<Profile>();
  const [pinError, setPinError] = useState<string>();
  const {run, busy} = useAction();
  const choose = (p: Profile, pin?: string) => void run(async () => {
    try {
      await enter(p.id, pin);
      setPinFor(undefined);
      onChosen?.();
      root('Home');
    } catch (e) {
      setPinError(e instanceof Error ? e.message : 'That PIN isn’t right. Try again.');
    }
  });
  if (profiles.loading && !profiles.data) return <Loading label="Loading profiles" />;
  const size = m.isTV ? 150 : m.isPhone ? 84 : 110;
  if (pinFor) {
    return (
      <View style={{alignItems: 'center', gap: m.space.lg}}>
        <Avatar name={pinFor.name} art={pinFor.art} size={size * 0.8} />
        <Text variant="heading">Enter profile PIN</Text>
        <PinPad onComplete={pin => { setPinError(undefined); choose(pinFor, pin); }} error={pinError} />
        <Text variant="caption" tone="tertiary" center>Forgot the PIN? The main profile can change it in Settings › Profiles.</Text>
        <Focusable onPress={() => { setPinFor(undefined); setPinError(undefined); }} style={({focused}) => ({padding: 10, borderRadius: 10, backgroundColor: focused ? colors.focus : 'transparent'})}>{({focused}) => <Text variant="bodyStrong" color={focused ? colors.accentInk : colors.accentStrong}>Other profiles</Text>}</Focusable>
      </View>
    );
  }
  return (
    <View style={{flexDirection: 'row', flexWrap: 'wrap', justifyContent: 'center', gap: m.isTV ? 48 : 24}}>
      {(profiles.data ?? []).map((p, i) => (
        <Focusable key={p.id} onPress={() => (p.pin ? setPinFor(p) : choose(p))} focusScale={1.1} hasTVPreferredFocus={i === 0} disabled={busy} accessibilityLabel={`${p.name}${p.pin ? ', protected by a PIN' : ''}`} style={{alignItems: 'center', gap: 10, width: size + 20}}>
          {({focused}) => (
            <>
              <View style={{borderRadius: size, padding: 4, borderWidth: 3, borderColor: focused ? colors.focus : current?.id === p.id ? colors.accent : 'transparent'}}>
                <Avatar name={p.name} art={p.art} size={size} />
                {p.pin ? <View style={{position: 'absolute', right: 2, bottom: 2, width: size * 0.3, height: size * 0.3, borderRadius: size, backgroundColor: colors.overlay, alignItems: 'center', justifyContent: 'center', borderWidth: 1, borderColor: colors.line}}><Icon name="lock" size={size * 0.16} color={colors.textSecondary} /></View> : null}
              </View>
              <Text variant="bodyStrong" center>{p.name}</Text>
              <View style={{height: m.isTV ? 30 : 22, justifyContent: 'center'}}>
                {p.kids ? <Badge label="Kids" tone="accent" /> : p.role !== 'member' ? <Text variant="micro" tone="tertiary">{p.role === 'owner' ? 'Owner' : 'Admin'}</Text> : null}
              </View>
            </>
          )}
        </Focusable>
      ))}
    </View>
  );
}

export function ProfileSwitcher({visible, onClose}: {visible: boolean; onClose: () => void}) {
  return (
    <Sheet visible={visible} onClose={onClose} title="Switch profile" subtitle="Each profile keeps its own history, Watchlist and preferences." size="md">
      {visible ? <ProfileGrid onChosen={onClose} /> : null}
    </Sheet>
  );
}

export function ServerSwitcher({visible, onClose, onChosen}: {visible: boolean; onClose: () => void; onChosen: () => void}) {
  const client = useClient();
  const {server, setAuthStart} = useSession();
  const m = useMetrics();
  const servers = useQuery(c => c.session.servers(), [visible]);
  const [error, setError] = useState<string>();
  const {run, busy} = useAction();
  const choose = (id: string, name: string) => void run(async () => {
    const r = await client.session.chooseServer(id);
    if (r.status !== 'online') { setError(`Portico can’t reach ${name}. It may be turned off, asleep or offline. Check that it’s on, then try again.`); return; }
    onClose();
    onChosen();
  });
  return (
    <Sheet visible={visible} onClose={onClose} title="Switch server" subtitle="Servers on your Portico Account and ones you signed in to directly." size="md">
      {error ? <Notice tone="error" title="Can’t reach that server" body={error} /> : null}
      {servers.loading && !servers.data ? <Loading label="Loading your servers" /> : null}
      <Text variant="label" tone="tertiary">Servers on your Portico Account</Text>
      {servers.data?.map((s, i) => (
        <ListRow key={s.id} icon="server" title={s.name} subtitle={serverLine(s)} hasTVPreferredFocus={i === 0}
          trailing={<Badge label={s.id === server?.id ? 'Current server' : s.status === 'online' ? 'Online' : 'Can’t reach'} tone={s.id === server?.id ? 'accent' : s.status === 'online' ? 'good' : 'bad'} />} onPress={() => choose(s.id, s.name)} disabled={busy} />
      ))}
      <Text variant="label" tone="tertiary" style={{marginTop: m.space.sm}}>Signed in directly</Text>
      <ListRow icon="link" title="Sign in directly to another server" subtitle="Use an account that lives on that server" onPress={() => { onClose(); setAuthStart('DirectSignIn'); }} />
    </Sheet>
  );
}
