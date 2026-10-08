import React, {useEffect, useRef} from 'react';
import {Animated, StyleSheet, View} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {Button} from '../../design/Button';
import {Icon} from '../../design/Icon';
import {Text} from '../../design/Text';
import {colors, isTV, u} from '../../design/tokens';
import {useSession} from '../../state/session';

/** Global banner while the server isn't answering (web: shell/Connection). */
export function ReconnectBanner() {
  const session = useSession();
  const insets = useSafeAreaInsets();
  const show = session.connection !== 'online';
  const y = useRef(new Animated.Value(-200)).current;
  useEffect(() => {
    Animated.spring(y, {toValue: show ? 0 : -200, useNativeDriver: true, speed: 14, bounciness: 4}).start();
  }, [show, y]);
  return (
    <Animated.View pointerEvents={show ? 'box-none' : 'none'} style={[styles.wrap, {top: isTV ? 30 : insets.top + 4, transform: [{translateY: y}]}]}>
      <View style={styles.banner}>
        <Icon name={session.connection === 'offline' ? 'wifiOff' : 'refresh'} size={17} color={colors.amber} />
        <View style={{flexShrink: 1, marginLeft: u(10)}}>
          <Text variant="captionStrong">{session.connection === 'offline' ? 'You’re offline' : `Reconnecting to ${session.server?.name ?? 'your server'}…`}</Text>
          <Text variant="caption" tone="secondary" numberOfLines={2}>Portico keeps trying and brings this page back as soon as it reconnects. Settings, your account and switching servers still work.</Text>
        </View>
        <Button label="Dismiss" size="sm" variant="ghost" onPress={() => session.setConnection('online')} style={{marginLeft: u(8)}} />
      </View>
    </Animated.View>
  );
}

const styles = StyleSheet.create({
  wrap: {position: 'absolute', left: 0, right: 0, alignItems: 'center', zIndex: 50},
  banner: {flexDirection: 'row', alignItems: 'center', maxWidth: isTV ? 1100 : 560, marginHorizontal: 12, padding: u(12), borderRadius: 18, backgroundColor: colors.overlay, borderWidth: 1, borderColor: 'rgba(232,188,106,0.5)', shadowColor: '#000', shadowOpacity: 0.4, shadowRadius: 18},
});
