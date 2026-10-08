import React, {useEffect, useRef, useState} from 'react';
import {ActivityIndicator, BackHandler, Linking, StatusBar, TVEventControl, View} from 'react-native';
import {DarkTheme, NavigationContainer} from '@react-navigation/native';
import {createNativeStackNavigator} from '@react-navigation/native-stack';
import {SafeAreaProvider} from 'react-native-safe-area-context';
import {DataProvider, useClient} from './src/data/DataProvider';
import {SessionProvider, useSession} from './src/app/session';
import {ToastProvider} from './src/app/toast';
import {GlowProvider} from './src/app/glow';
import {PlayerProvider} from './src/player/PlayerProvider';
import {SelectionProvider} from './src/app/selection';
import {ActionsProvider, useActions} from './src/app/actions';
import {rootNav} from './src/nav/nav';
import type {AuthRoutes, RootRoutes} from './src/nav/routes';
import {openScreen, registerScreenHooks, SCREENS} from './src/nav/screens';
import {Shell} from './src/shell/Shell';
import * as A from './src/screens/auth/Auth';
import {colors} from './src/theme/tokens';
import {isTV} from './src/theme/layout';
import {Text} from './src/ui/Text';
import {Wordmark} from './src/shell/Brand';

const Root = createNativeStackNavigator<RootRoutes>();
const theme = {...DarkTheme, colors: {...DarkTheme.colors, background: 'transparent', card: colors.overlay, primary: colors.accent, text: colors.text, border: colors.line}};

/**
 * Portico Classic. Providers: data (the service layer), session, toasts, the
 * glow canvas, playback, multi-select and the title actions menu; then the
 * navigator with the sign-in flow or the signed-in shell.
 */
export default function App() {
  return (
    <SafeAreaProvider>
      <StatusBar barStyle="light-content" />
      <DataProvider>
        <SessionProvider>
          <ToastProvider>
            <GlowProvider>
              <PlayerProvider>
                <SelectionProvider>
                  <ActionsProvider>
                    <Navigator />
                  </ActionsProvider>
                </SelectionProvider>
              </PlayerProvider>
            </GlowProvider>
          </ToastProvider>
        </SessionProvider>
      </DataProvider>
    </SafeAreaProvider>
  );
}

function Navigator() {
  const session = useSession();
  const client = useClient();
  const actions = useActions();
  const [authParams, setAuthParams] = useState<object>();
  const pending = useRef<string | undefined>(undefined);
  useEffect(() => {
    registerScreenHooks({
      auth: (route, params) => {
        setAuthParams(params);
        session.setAuthStart(route);
        // An existing navigator keeps its state, so set the stack explicitly: Landing, then the screen.
        setTimeout(() => {
          if (!rootNav.isReady()) return;
          const routes = route === 'Landing' ? [{name: 'Landing'}] : [{name: 'Landing'}, {name: route, params}];
          rootNav.reset({index: routes.length - 1, routes} as never);
        }, 80);
      },
      enterProfile: id => session.enter(id),
      actions: () => actions as never,
    });
  }, [session, actions]);

  // Deep links for review: portico-classic://screen/<id>
  useEffect(() => {
    const handle = (url: string | null) => {
      const m = url?.match(/screen\/([\w-]+)/);
      if (!m) return;
      const def = SCREENS.find(s => s.id === m[1]);
      if (!def) return;
      if (def.group !== 'Getting in' && session.phase !== 'signedIn') { pending.current = def.id; return; }
      setTimeout(() => openScreen(def.id, client), 350);
    };
    void Linking.getInitialURL().then(handle);
    const sub = Linking.addEventListener('url', e => handle(e.url));
    return () => sub.remove();
  }, [client, session.phase]);
  // Development only: review tooling can post a screen id to a local file server (no URL prompts).
  useEffect(() => {
    if (!__DEV__) return;
    let last = '';
    const t = setInterval(() => {
      fetch(`http://localhost:8191/cmd.txt?t=${Date.now()}`).then(r => (r.ok ? r.text() : '')).then(txt => {
        const line = txt.trim();
        if (!line || line === last) return;
        const first = !last;
        last = line;
        if (first) return;
        const id = line.split('|')[0]!;
        const def = SCREENS.find(sc => sc.id === id);
        if (def) openScreen(def.id, client);
      }).catch(() => {});
    }, 1000);
    return () => clearInterval(t);
  }, [client]);
  useEffect(() => {
    if (session.phase === 'signedIn' && pending.current) {
      const id = pending.current;
      pending.current = undefined;
      setTimeout(() => openScreen(id, client), 900);
    }
  }, [session.phase, client]);

  // TV Menu key in the sign-in flow: go back while there is somewhere to go.
  const [authDepth, setAuthDepth] = useState(1);
  useEffect(() => {
    if (!isTV || session.phase !== 'signedOut') return;
    if (authDepth > 1) TVEventControl.enableTVMenuKey();
    else TVEventControl.disableTVMenuKey();
    const sub = BackHandler.addEventListener('hardwareBackPress', () => {
      if (rootNav.canGoBack()) { rootNav.goBack(); return true; }
      return false;
    });
    return () => sub.remove();
  }, [session.phase, authDepth]);

  if (session.phase === 'launch') return <Splash />;
  return (
    <NavigationContainer ref={rootNav} theme={theme} onStateChange={s => setAuthDepth(s?.routes.length ?? 1)}>
      <Root.Navigator key={session.phase === 'signedIn' ? 'app' : `auth-${session.authStart}`} screenOptions={{headerShown: false, contentStyle: {backgroundColor: 'transparent'}, animation: isTV ? 'fade' : 'default'}} initialRouteName={session.phase === 'signedIn' ? 'Main' : (session.authStart as keyof AuthRoutes)}>
        {session.phase === 'signedIn' ? (
          <Root.Screen name="Main" component={Shell} />
        ) : (
          <>
            <Root.Screen name="Landing" component={A.LandingScreen} />
            <Root.Screen name="AccountSignIn" component={A.AccountSignInScreen} initialParams={session.authStart === 'AccountSignIn' ? (authParams as never) : undefined} />
            <Root.Screen name="TwoStep" component={A.TwoStepScreen} />
            <Root.Screen name="DirectSignIn" component={A.DirectSignInScreen} />
            <Root.Screen name="ServerChooser" component={A.ServerChooserScreen} />
            <Root.Screen name="Profiles" component={A.ProfilesScreen} />
            <Root.Screen name="Join" component={A.JoinScreen} initialParams={session.authStart === 'Join' ? (authParams as never) : undefined} />
            <Root.Screen name="Activate" component={A.ActivateScreen} />
            <Root.Screen name="QuickConnect" component={A.QuickConnectScreen} />
            <Root.Screen name="Claim" component={A.ClaimScreen} />
            <Root.Screen name="Setup" component={A.SetupScreen} />
            <Root.Screen name="Restore" component={A.RestoreScreen} />
            <Root.Screen name="VerifyEmail" component={A.VerifyEmailScreen} />
            <Root.Screen name="ResetPassword" component={A.ResetPasswordScreen} />
            <Root.Screen name="ServerUnavailable" component={A.ServerUnavailableScreen} />
            <Root.Screen name="Offline" component={A.OfflineScreen} />
            <Root.Screen name="DeviceApproval" component={A.DeviceApprovalScreen} />
          </>
        )}
      </Root.Navigator>
    </NavigationContainer>
  );
}

function Splash() {
  return (
    <View style={{flex: 1, alignItems: 'center', justifyContent: 'center'}}>
      {/* Same size and place as the native launch screen, so the hand-over is seamless. */}
      <Wordmark height={isTV ? 84 : 48} />
      <View style={{position: 'absolute', top: '50%', marginTop: isTV ? 110 : 64, flexDirection: 'row', gap: isTV ? 20 : 10, alignItems: 'center'}}>
        <ActivityIndicator color={colors.accentStrong} />
        <Text variant="caption" tone="secondary">Opening Portico…</Text>
      </View>
    </View>
  );
}
