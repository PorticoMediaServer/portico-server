import {AccessibilityInfo} from 'react-native';
import {useEffect, useState} from 'react';
import {useClient, useTopic} from '../data/DataProvider';

let systemReduce = false;
AccessibilityInfo.isReduceMotionEnabled().then(v => (systemReduce = v)).catch(() => {});

/** Device-facing preferences the UI reads synchronously. */
export function usePrefs() {
  const client = useClient();
  useTopic(['prefs']);
  const [sys, setSys] = useState(systemReduce);
  useEffect(() => {
    const sub = AccessibilityInfo.addEventListener('reduceMotionChanged', setSys);
    return () => sub.remove();
  }, []);
  return {
    reduceMotion: sys || client.prefs.get('appearance.reduceMotion') === true,
    cardScale: Number(client.prefs.get('appearance.cardSizePercent') || 100) / 100,
    showBackdrops: client.prefs.get('appearance.showBackdrops') !== false,
    badges: client.prefs.get('notifications.badges') !== false,
    skipBack: Number(client.prefs.get('playback.skipBackSeconds') || 10),
    skipForward: Number(client.prefs.get('playback.skipForwardSeconds') || 30),
    autoplayNext: client.prefs.get('playback.autoplayNext') !== false,
    countdown: Number(client.prefs.get('playback.upNextCountdownSeconds') ?? 10),
    introSkip: String(client.prefs.get('playback.introSkip') || 'ask'),
    creditsSkip: String(client.prefs.get('playback.creditsSkip') || 'ask'),
    lyrics: client.prefs.get('playback.showSyncedLyrics') !== false,
  };
}
