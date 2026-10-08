import React, {useEffect, useMemo, useState} from 'react';
import {Linking, LogBox, StatusBar, StyleSheet, View} from 'react-native';
import {NavigationContainer, DarkTheme} from '@react-navigation/native';
import {createNativeStackNavigator} from '@react-navigation/native-stack';
import {SafeAreaProvider} from 'react-native-safe-area-context';
import {createFixtureServices} from './data/fixture/backend';
import {ServicesProvider} from './data/hooks';
import type {Profile, Server} from './data/types';
import {Glow} from './design/Glow';
import {OverlayHost} from './design/overlays';
import {colors, isTV} from './design/tokens';
import {useLayout} from './design/useLayout';
import {updateCurrentRoute, useCurrentRoute} from './nav/current';
import {handleDeepLink, setDeepLinkServices} from './nav/deepLinks';
import {setReconnectHook} from './nav/screenIndex';
import {Rail} from './nav/Rail';
import {chromeless, navigationRef, type Routes} from './nav/routes';
import {screens} from './nav/screens';
import {TabBar} from './nav/TabBar';
import {MiniPlayer} from './screens/player/MiniPlayer';
import {ReconnectBanner} from './screens/shared/ReconnectBanner';
import {setDurationResolver} from './state/player';
import {SessionProvider, useSession} from './state/session';

LogBox.ignoreAllLogs();
const Stack = createNativeStackNavigator<Routes>();

export default function App() {
  const services = useMemo(() => createFixtureServices(), []);
  const [boot, setBoot] = useState<null | {server: Server; profile: Profile}>(null);
  useEffect(() => {
    setDurationResolver(id => services.catalog.durationOf(id));
    setDeepLinkServices(services);
    Promise.all([services.auth.servers(), services.account.profiles()]).then(([servers, profiles]) => setBoot({server: servers[0], profile: profiles[0]}));
  }, [services]);
  return (
    <SafeAreaProvider>
      <ServicesProvider services={services}>
        {boot ? (
          <SessionProvider initial={{server: boot.server, profile: boot.profile, role: boot.server.role}}>
            <Root />
          </SessionProvider>
        ) : (
          <View style={styles.root} />
        )}
      </ServicesProvider>
    </SafeAreaProvider>
  );
}

function Chrome() {
  const route = useCurrentRoute();
  const layout = useLayout();
  const bare = chromeless.has(route.name);
  return (
    <>
      {!bare && (isTV || layout.tablet) ? <Rail /> : null}
      {!bare && layout.compact ? <TabBar /> : null}
      {!bare ? <MiniPlayer /> : null}
      <ReconnectBanner />
    </>
  );
}

function Root() {
  const session = useSession();
  useEffect(() => setReconnectHook(() => {
    session.setConnection('reconnecting');
    setTimeout(() => session.setConnection('online'), 8000);
  }), [session]);
  return (
    <View style={styles.root}>
      <StatusBar barStyle="light-content" backgroundColor="transparent" translucent />
      <Glow />
      <NavigationContainer
        ref={navigationRef}
        theme={{...DarkTheme, colors: {...DarkTheme.colors, background: 'transparent', card: colors.ink}}}
        onReady={() => {
          updateCurrentRoute();
          Linking.getInitialURL().then(u => u && setTimeout(() => handleDeepLink(u), 300));
        }}
        onStateChange={updateCurrentRoute}>
        <Stack.Navigator initialRouteName="Landing" screenOptions={{headerShown: false, contentStyle: {backgroundColor: 'transparent'}, animation: isTV ? 'fade' : 'default', fullScreenGestureEnabled: true}}>
          {screens.map(s => (
            <Stack.Screen key={s.name} name={s.name} component={s.component as React.ComponentType} options={s.options} />
          ))}
        </Stack.Navigator>
      </NavigationContainer>
      <Chrome />
      <OverlayHost />
      <LinkListener />
    </View>
  );
}

function LinkListener() {
  useEffect(() => {
    const sub = Linking.addEventListener('url', e => handleDeepLink(e.url));
    return () => sub.remove();
  }, []);
  return null;
}

const styles = StyleSheet.create({
  root: {flex: 1, backgroundColor: colors.ink},
});

