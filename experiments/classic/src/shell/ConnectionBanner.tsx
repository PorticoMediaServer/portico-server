import React, {useEffect, useRef, useState} from 'react';
import {ActivityIndicator, Animated, View} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {useClient, useTopic} from '../data/DataProvider';
import {useSession} from '../app/session';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {Text} from '../ui/Text';
import {Icon} from '../ui/Icon';
import {Button} from '../ui/Button';
import {PHONE_HEADER} from '../ui/Page';

/** "Reconnecting to Lighthouse…" and offline banners, with an automatic retry countdown. */
export function ConnectionBanner() {
  const client = useClient();
  useTopic(['session']);
  const {server} = useSession();
  const m = useMetrics();
  const insets = useSafeAreaInsets();
  const state = client.session.connectivity();
  const [seconds, setSeconds] = useState(8);
  const y = useRef(new Animated.Value(-80)).current;
  useEffect(() => {
    Animated.timing(y, {toValue: state === 'online' ? -120 : 0, duration: 260, useNativeDriver: true}).start();
    if (state === 'online') return;
    setSeconds(8);
    const t = setInterval(() => setSeconds(s => (s <= 1 ? 8 : s - 1)), 1000);
    return () => clearInterval(t);
  }, [state, y]);
  if (state === 'online') return null;
  const offline = state === 'offline';
  return (
    <Animated.View style={{position: 'absolute', left: m.isTV ? m.rail.collapsed + 40 : 12, right: m.isTV ? 40 : 12, top: m.isTV ? 28 : m.isPhone ? insets.top + PHONE_HEADER + 4 : Math.max(insets.top, 12), transform: [{translateY: y}]}}>
      <View style={{flexDirection: 'row', alignItems: 'center', gap: m.space.md, padding: m.space.md, borderRadius: m.radius.panel, backgroundColor: offline ? '#3a1c1f' : '#2a2413', borderWidth: 1, borderColor: offline ? colors.dangerDeep : '#6b5a2a'}}>
        {offline ? <Icon name="offline" size={m.icon} color={colors.danger} /> : <ActivityIndicator color={colors.amber} />}
        <View style={{flex: 1}}>
          <Text variant="bodyStrong">{offline ? 'This device is offline' : `Reconnecting to ${server?.name ?? 'your server'}`}</Text>
          <Text variant="caption" tone="secondary">{offline ? 'Check your connection. Downloads still play.' : `${server?.name ?? 'Your server'} isn’t answering right now. Trying again in ${seconds} s. Downloads and settings still work.`}</Text>
        </View>
        {m.isTV ? null : <Button size="sm" label="Try now" variant="secondary" onPress={() => client.session.setConnectivity('online')} />}
      </View>
    </Animated.View>
  );
}
