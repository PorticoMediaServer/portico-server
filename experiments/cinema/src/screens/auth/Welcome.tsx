import React, {useEffect, useRef, useState} from 'react';
import {Animated, Easing, StyleSheet, TVFocusGuideView, View} from 'react-native';
import {useRoute, type RouteProp} from '@react-navigation/native';
import {useQuery, useServices} from '../../data/hooks';
import type {Profile} from '../../data/types';
import {Wordmark} from '../../design/Brand';
import {Button} from '../../design/Button';
import {ToggleRow} from '../../design/Controls';
import {Focusable} from '../../design/Focusable';
import {glow} from '../../design/Glow';
import {Icon} from '../../design/Icon';
import {ui} from '../../design/overlays';
import {Text} from '../../design/Text';
import {colors, fonts, isTV, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {go, replaceAll, type Routes} from '../../nav/routes';
import {linkNavigatedRecently} from '../../nav/deepLinks';
import {useSession} from '../../state/session';

/** Launch: the mark lights up while Portico finds your servers, then opens the last one. */
export function LandingScreen() {
  const s = useServices();
  const session = useSession();
  const fade = useRef(new Animated.Value(0)).current;
  const beam = useRef(new Animated.Value(0)).current;
  const [status, setStatus] = useState('Finding your servers');
  useEffect(() => {
    glow.setTint(null);
    Animated.parallel([
      Animated.timing(fade, {toValue: 1, duration: 900, easing: Easing.out(Easing.cubic), useNativeDriver: true}),
      Animated.loop(Animated.sequence([Animated.timing(beam, {toValue: 1, duration: 1400, useNativeDriver: true}), Animated.timing(beam, {toValue: 0, duration: 1400, useNativeDriver: true})])),
    ]).start();
    let alive = true;
    s.auth.servers().then(servers => {
      if (!alive) return;
      setStatus(`Opening ${servers[0].name}`);
      setTimeout(() => alive && !linkNavigatedRecently() && replaceAll('Profiles'), 1300);
    });
    return () => {
      alive = false;
    };
  }, [s, fade, beam, session.server]);
  return (
    <View style={styles.center}>
      <Animated.View style={{opacity: fade, transform: [{scale: fade.interpolate({inputRange: [0, 1], outputRange: [0.92, 1]})}], alignItems: 'center'}}>
        <Animated.View style={{opacity: beam.interpolate({inputRange: [0, 1], outputRange: [0.75, 1]})}}>
          <Wordmark size={isTV ? 84 : 42} />
        </Animated.View>
        <Text variant="caption" tone="tertiary" style={{marginTop: u(30)}}>{status}…</Text>
      </Animated.View>
    </View>
  );
}

/** Who's watching: large profile portraits; a PIN pad opens for protected profiles. */
export function ProfilesScreen() {
  const s = useServices();
  const session = useSession();
  const layout = useLayout();
  const route = useRoute<RouteProp<Routes, 'Profiles'>>();
  const q = useQuery('profiles', () => s.account.profiles());
  const [remember, setRemember] = useState(true);
  const size = isTV ? 220 : layout.compact ? 118 : 150;
  const choose = (p: Profile) => {
    const enter = () => {
      session.setProfile(p);
      replaceAll('Home');
    };
    if (p.pin) openPinPad(s, p, enter);
    else enter();
  };
  return (
    <View style={styles.center}>
      <Wordmark size={isTV ? 40 : 24} />
      <Text variant="hero" center style={{marginTop: u(28)}}>Who’s watching?</Text>
      <Text variant="body" tone="tertiary" center style={{marginTop: u(6)}}>{session.server?.name}</Text>
      <TVFocusGuideView autoFocus style={[styles.grid, {gap: isTV ? 64 : 22}]}>
        {(q.data ?? []).map((p, i) => (
          <Focusable key={p.id} onPress={() => choose(p)} hasTVPreferredFocus={i === 0} scale={1.1} accessibilityLabel={`${p.name}${p.pin ? ', protected by a PIN' : ''}${p.kids ? ', kids profile' : ''}`} onFocus={() => glow.setTint(p.color)}>
            {({focused}) => (
              <View style={{alignItems: 'center', width: size + 20}}>
                <View style={[styles.avatar, {width: size, height: size, borderRadius: size / 2, backgroundColor: p.color}, focused && isTV && styles.avatarFocus]}>
                  <Text style={{fontFamily: fonts.display, fontSize: size * 0.42, color: colors.actionInk}}>{p.initials}</Text>
                  {p.kids ? <View style={styles.kids}><Text variant="micro" tone="ink">KIDS</Text></View> : null}
                </View>
                <View style={{flexDirection: 'row', alignItems: 'center', marginTop: u(12), gap: 6}}>
                  <Text variant="subheading" color={focused && isTV ? colors.white : colors.textSecondary}>{p.name}</Text>
                  {p.pin ? <Icon name="lock" size={13} color={colors.textTertiary} /> : null}
                </View>
                {p.id === session.profile?.id && route.params?.switching ? <Text variant="micro" tone="accent">CURRENT</Text> : null}
              </View>
            )}
          </Focusable>
        ))}
      </TVFocusGuideView>
      <View style={{flexDirection: 'row', gap: 12, marginTop: u(40), flexWrap: 'wrap', justifyContent: 'center'}}>
        <Button label="Manage profiles" icon="edit" onPress={() => go('SettingsProfiles')} />
        <Button label="Switch server" icon="server" variant="ghost" onPress={() => go('Chooser')} />
      </View>
      {!isTV ? <View style={{width: 320, marginTop: 10}}><ToggleRow label="Remember this profile on this device" value={remember} onChange={setRemember} /></View> : null}
    </View>
  );
}

export function openPinPad(s: ReturnType<typeof useServices>, p: Profile, onOk: () => void) {
  ui.sheet({title: `Enter the PIN for ${p.name}`, subtitle: 'Four digits', render: close => <PinPad s={s} profile={p} onOk={() => { close(); onOk(); }} />});
}

function PinPad({s, profile, onOk}: {s: ReturnType<typeof useServices>; profile: Profile; onOk: () => void}) {
  const [pin, setPin] = useState('');
  const [error, setError] = useState<string | null>(null);
  const press = (k: string) => {
    setError(null);
    if (k === 'del') return setPin(v => v.slice(0, -1));
    const next = (pin + k).slice(0, 4);
    setPin(next);
    if (next.length === 4) {
      s.auth.unlockProfile(profile.id, next).then(ok => {
        if (ok) onOk();
        else {
          setError('That PIN isn’t right. Try again.');
          setPin('');
        }
      });
    }
  };
  const keys = ['1', '2', '3', '4', '5', '6', '7', '8', '9', 'forgot', '0', 'del'];
  return (
    <View style={{alignItems: 'center'}}>
      <View style={{flexDirection: 'row', gap: u(14), marginVertical: u(18)}}>
        {[0, 1, 2, 3].map(i => <View key={i} style={[styles.pinDot, i < pin.length && {backgroundColor: colors.accentStrong, borderColor: colors.accentStrong}]} />)}
      </View>
      {error ? <Text variant="caption" tone="danger" style={{marginBottom: u(10)}}>{error}</Text> : <Text variant="caption" tone="tertiary" style={{marginBottom: u(10)}}>Hint for the demo: 2468</Text>}
      <View style={{width: isTV ? 480 : 270, flexDirection: 'row', flexWrap: 'wrap', justifyContent: 'center', gap: u(10)}}>
        {keys.map((k, i) => (
          <Focusable key={k} hasTVPreferredFocus={i === 0} onPress={() => (k === 'forgot' ? ui.toast('A forgotten PIN is reset with your password plus a two-step code.') : press(k))} scale={1.1} accessibilityLabel={k === 'del' ? 'Delete' : k === 'forgot' ? 'Forgot PIN' : k}>
            {({focused}) => (
              <View style={[styles.key, focused && isTV && {backgroundColor: colors.white}]}>
                {k === 'del' ? <Icon name="delete" size={20} color={focused && isTV ? colors.actionInk : colors.text} /> : k === 'forgot' ? <Text variant="micro" color={focused && isTV ? colors.actionInk : colors.textTertiary}>FORGOT</Text> : <Text variant="heading" color={focused && isTV ? colors.actionInk : colors.text}>{k}</Text>}
              </View>
            )}
          </Focusable>
        ))}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  center: {flex: 1, alignItems: 'center', justifyContent: 'center', padding: 20},
  grid: {flexDirection: 'row', flexWrap: 'wrap', justifyContent: 'center', marginTop: u(36), maxWidth: 1400},
  avatar: {alignItems: 'center', justifyContent: 'center'},
  avatarFocus: {borderWidth: 6, borderColor: colors.white, shadowColor: colors.white, shadowOpacity: 0.5, shadowRadius: 30},
  kids: {position: 'absolute', bottom: -6, backgroundColor: colors.white, borderRadius: 8, paddingHorizontal: 8, paddingVertical: 2},
  pinDot: {width: u(16), height: u(16), borderRadius: u(8), borderWidth: 2, borderColor: colors.lineStrong},
  key: {width: u(72), height: u(60), borderRadius: u(14), backgroundColor: 'rgba(237,245,250,0.08)', alignItems: 'center', justifyContent: 'center'},
});
