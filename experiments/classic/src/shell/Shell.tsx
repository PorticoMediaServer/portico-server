import React, {useEffect, useRef, useState} from 'react';
import {BackHandler, TVEventControl, View} from 'react-native';
import {createNativeStackNavigator} from '@react-navigation/native-stack';
import {useMetrics} from '../theme/layout';
import type {AppRoutes} from '../nav/routes';
import {Rail} from './Rail';
import {PhoneHeader, TabBar} from './PhoneChrome';
import {ProfileMenu} from './ProfileMenu';
import {SelectionBar} from '../app/selection';
import {useActions} from '../app/actions';
import {ConnectionBanner} from './ConnectionBanner';
import {MiniPlayer} from '../player/MiniPlayer';
import {PlayerHost} from '../player/PlayerHost';
import {GroupPill} from '../player/GroupPill';
import {ReminderWatcher} from './ReminderWatcher';
import {back, contentNav} from '../nav/nav';
import {useDestination} from './useDestination';
import * as S from '../screens';
import {devlog} from '../dev/devlog';

const Stack = createNativeStackNavigator<AppRoutes>();

function ContentStack() {
  const m = useMetrics();
  return (
    <Stack.Navigator initialRouteName="Home" screenOptions={{headerShown: false, contentStyle: {backgroundColor: 'transparent'}, animation: m.isTV ? 'fade' : 'default', gestureEnabled: !m.isTV}}>
      <Stack.Screen name="Home" component={S.HomeScreen} />
      <Stack.Screen name="HomeRow" component={S.HomeRowScreen} />
      <Stack.Screen name="Library" component={S.LibraryScreen} />
      <Stack.Screen name="Entity" component={S.EntityScreen} />
      <Stack.Screen name="Movie" component={S.MovieScreen} />
      <Stack.Screen name="HomeVideo" component={S.MovieScreen} />
      <Stack.Screen name="Show" component={S.ShowScreen} />
      <Stack.Screen name="Episode" component={S.EpisodeScreen} />
      <Stack.Screen name="Person" component={S.PersonScreen} />
      <Stack.Screen name="Artist" component={S.ArtistScreen} />
      <Stack.Screen name="Album" component={S.AlbumScreen} />
      <Stack.Screen name="Audiobook" component={S.AudiobookScreen} />
      <Stack.Screen name="PhotoAlbum" component={S.PhotoAlbumScreen} />
      <Stack.Screen name="Resource" component={S.ResourceScreen} />
      <Stack.Screen name="Search" component={S.SearchScreen} />
      <Stack.Screen name="Saved" component={S.SavedScreen} />
      <Stack.Screen name="Live" component={S.LiveScreen} />
      <Stack.Screen name="Together" component={S.TogetherScreen} />
      <Stack.Screen name="Downloads" component={S.DownloadsScreen} />
      <Stack.Screen name="Settings" component={S.SettingsScreen} />
      <Stack.Screen name="SettingsSection" component={S.SettingsSectionScreen} />
      <Stack.Screen name="Account" component={S.AccountScreen} />
      <Stack.Screen name="Admin" component={S.AdminScreen} />
      <Stack.Screen name="NotFound" component={S.NotFoundScreen} />
    </Stack.Navigator>
  );
}

/**
 * The signed-in frame. TV: an icon rail that expands on focus. iPad: a
 * persistent, collapsible sidebar. iPhone: a header, a tab bar and a mini
 * player docked above the tabs. Player surfaces sit above everything.
 */
export function Shell() {
  const m = useMetrics();
  const actions = useActions();
  const [menu, setMenu] = useState(false);
  const [sidebar, setSidebar] = useState(m.width >= 1100);
  const railRef = useRef<View>(null);
  const [railFocused, setRailFocused] = useState(false);
  const [railRequest, setRailRequest] = useState(0);
  const dest = useDestination();
  useEffect(() => devlog(m.isTV ? 'shell-ready-tv' : 'shell-ready'), [m.isTV]);

  // TV Menu key: back through the stack, then to the rail, then out of the app.
  useEffect(() => {
    if (!m.isTV) return;
    const canBack = dest.depth > 1;
    const apply = () => (canBack || !railFocused ? TVEventControl.enableTVMenuKey() : TVEventControl.disableTVMenuKey());
    apply();
    // The native remote handler may attach after the first render; assert the policy again.
    const again = setTimeout(apply, 1200);
    const sub = BackHandler.addEventListener('hardwareBackPress', () => {
      if (contentNav()?.canGoBack()) { back(); return true; }
      if (!railFocused) { (railRef.current as unknown as {requestTVFocus?: () => void})?.requestTVFocus?.(); setRailRequest(n => n + 1); return true; }
      return false;
    });
    return () => { clearTimeout(again); sub.remove(); };
  }, [m.isTV, dest.depth, railFocused]);

  const bar = <SelectionBar onAddTo={actions.addTo} onEdit={actions.editMetadata} onDelete={actions.deleteMedia} />;
  if (m.isTV) {
    return (
      <View style={{flex: 1}}>
        <View style={{flex: 1, marginLeft: m.rail.collapsed}}><ContentStack /></View>
        <Rail variant="tv" onProfile={() => setMenu(true)} onFocusChange={setRailFocused} railRef={railRef} focusRequest={railRequest} />
        <ConnectionBanner />
        {bar}
        <GroupPill />
        <ReminderWatcher />
        <ProfileMenu visible={menu} onClose={() => setMenu(false)} />
        <PlayerHost />
      </View>
    );
  }
  if (m.isTablet) {
    return (
      <View style={{flex: 1, flexDirection: 'row'}}>
        <Rail variant="tablet" expanded={sidebar} onToggle={() => setSidebar(s => !s)} onProfile={() => setMenu(true)} />
        <View style={{flex: 1}}>
          <ContentStack />
          <ConnectionBanner />
          <MiniPlayer />
        </View>
        {bar}
        <GroupPill />
        <ReminderWatcher />
        <ProfileMenu visible={menu} onClose={() => setMenu(false)} />
        <PlayerHost />
      </View>
    );
  }
  return (
    <View style={{flex: 1}}>
      <ContentStack />
      <PhoneHeader onProfile={() => setMenu(true)} />
      <ConnectionBanner />
      <MiniPlayer />
      <TabBar />
      {bar}
      <GroupPill />
      <ReminderWatcher />
      <ProfileMenu visible={menu} onClose={() => setMenu(false)} />
      <PlayerHost />
    </View>
  );
}
