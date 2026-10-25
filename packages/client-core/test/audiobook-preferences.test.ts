import test from 'node:test';
import assert from 'node:assert/strict';
import {audiobookPreferences,defaultAudiobookPreferences} from '../src/audiobook-preferences.ts';
import type {PreferenceSnapshot} from '../src/preferences.ts';
const snapshot=(effective:Record<string,unknown>)=>({effective,registry:{fields:[]}} as unknown as PreferenceSnapshot);
test('audiobook controls use independent current profile choices and only declared old-server defaults',()=>{
 assert.deepEqual(audiobookPreferences(snapshot({})),defaultAudiobookPreferences);
 assert.deepEqual(audiobookPreferences(snapshot({'audiobooks.skipBackMs':60000,'audiobooks.skipForwardMs':90000,'audiobooks.resumeRewindMs':5000,'playback.skipBackMs':10000})),{skipBackMs:60000,skipForwardMs:90000,resumeRewindMs:5000});
 for(const key of ['skipBackMs','skipForwardMs','resumeRewindMs'])assert.throws(()=>audiobookPreferences(snapshot({[`audiobooks.${key}`]:7000})),/invalid_audiobook_preference/);
});
