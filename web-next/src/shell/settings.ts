import {createI18n, deviceRegion, resolveRegion} from '@i18n';
import {preferenceValue, type PreferenceSnapshot} from '@core/preferences.ts';
import {regionPreferences} from '@core/preference-effects.ts';
import {musicAudioEffects, musicQueueDefaults} from '@core/music-preferences.ts';
import type {SkipMode} from '../../../packages/player-core/src/index.ts';
import type {WebPlaybackSettings} from '../platform/playback';

export function shellLanguage(preferences: PreferenceSnapshot) {
  return createI18n(resolveRegion(regionPreferences(preferences), deviceRegion()));
}
/** Omit defaultQuality: the real server applies home/away defaults to the actual request. */
export function shellPlaybackSettings(preferences: PreferenceSnapshot, browser: Pick<Navigator, 'userAgent' | 'maxTouchPoints'> = navigator): WebPlaybackSettings {
  return {
    videoSpeed: () => preferenceValue<number>(preferences, 'playback.defaultSpeed'),
    audiobookSpeed: () => preferenceValue<number>(preferences, 'audiobooks.defaultSpeed'),
    skipModes: () => ({intro: preferenceValue<SkipMode>(preferences, 'playback.introSkip'), credits: preferenceValue<SkipMode>(preferences, 'playback.creditsSkip'), recap: preferenceValue<SkipMode>(preferences, 'playback.recapSkip')}),
    audioEffects: () => musicAudioEffects(preferences), musicDefaults: () => musicQueueDefaults(preferences),
    webVolume: () => !(/iPhone|iPad|iPod/.test(browser.userAgent) || /Macintosh/.test(browser.userAgent) && browser.maxTouchPoints > 1),
  };
}
